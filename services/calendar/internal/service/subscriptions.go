// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
	"git.sytue.eu/sytue/one/services/calendar/internal/subscriptions"
)

// Suscripción a calendarios ICS externos (el «Añadir calendario por URL»). Una
// suscripción es un calendario de SOLO LECTURA cuyos VEVENT se obtienen de una
// URL http(s), SSRF-safe, con GET condicional, y se cachean como objetos/eventos
// normales — reutilizando render/CalDAV/RRULE/TZ. Un worker cross-réplica
// refresca cada feed según su cadencia (respetando Cache-Control).

const (
	// maxFeedObjects acota cuántos recursos (UIDs) se cachean por feed: defensa
	// frente a feeds abusivos. Un calendario de festivos/equipo cabe de sobra.
	maxFeedObjects = 10000
	// maxRefreshBatch acota cuántas suscripciones refresca el worker por ciclo.
	// ponytail: procesado secuencial por ciclo; el reparto entre réplicas lo da el
	// claim con SKIP LOCKED. Si hay muchísimos feeds, subir el batch o el nº de
	// réplicas del worker (cada feed queda arrendado de forma independiente).
	maxRefreshBatch = 20
)

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// SubscribeICS da de alta una suscripción a un feed ICS externo: valida la URL
// (esquema/host; la barrera SSRF completa se aplica en cada fetch), crea el
// calendario de solo lectura y su fila de suscripción, y deja que el worker haga
// el primer refresco (next_refresh_at=now()). Devuelve el calendario creado.
func (s *Service) SubscribeICS(ctx context.Context, ownerSub, rawURL, displayName, color string) (domain.Calendar, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !domain.ValidSubscriptionURL(rawURL) {
		return domain.Calendar{}, domain.ErrInvalidURL
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = "Calendario suscrito"
	}
	if !hexColorRe.MatchString(color) {
		color = "" // color no válido => el store aplica el por defecto de la paleta
	}
	ns := store.NewSubscription{
		OwnerSub:        ownerSub,
		URL:             rawURL,
		DisplayName:     displayName,
		Color:           color,
		RefreshInterval: s.cfg.SubscriptionRefreshInterval,
	}
	// Slug único por dueño (segmento de ruta CalDAV). Reintenta con sufijo si choca.
	base := subscriptionSlug(displayName)
	c, err := s.store.CreateSubscription(ctx, base, ns)
	if err == domain.ErrConflict { //nolint:errorlint // el store devuelve el centinela exacto
		c, err = s.store.CreateSubscription(ctx, base+"-"+shortID(), ns)
	}
	if err != nil {
		return domain.Calendar{}, err
	}
	s.publish(ctx, "calendar.subscription.created", map[string]any{"calendar_id": c.ID, "url": rawURL})
	return c, nil
}

// RefreshSubscription refresca AHORA la suscripción de un calendario (acción
// «actualizar ahora» del usuario) y devuelve el calendario con su estado nuevo.
// El tenant ya va en el contexto de la petición. Misma verja que el resto de la
// API primaria: dueño|writer|manage; ajeno → 404 (el worker no pasa por aquí).
func (s *Service) RefreshSubscription(ctx context.Context, calendarID uuid.UUID) (domain.Calendar, error) {
	if err := s.requireCalendarWrite(ctx, calendarID); err != nil {
		return domain.Calendar{}, err
	}
	job, err := s.store.SubscriptionForCalendar(ctx, calendarID)
	if err != nil {
		return domain.Calendar{}, err
	}
	s.refreshJob(ctx, job)
	return s.store.GetCalendar(ctx, calendarID)
}

// RefreshDueSubscriptions es el ciclo del worker: reclama las suscripciones
// vencidas (cross-réplica, con lease) y refresca cada una bajo su tenant.
// Satisface subscriptions.Refresher. Devuelve cuántas procesó.
func (s *Service) RefreshDueSubscriptions(ctx context.Context) (int, error) {
	jobs, err := s.store.ClaimDueSubscriptions(ctx, maxRefreshBatch)
	if err != nil {
		return 0, err
	}
	for _, job := range jobs {
		// Cada feed se refresca bajo el tenant de su fila (el claim es cross-tenant).
		tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: job.Tenant})
		s.refreshJob(tctx, job)
	}
	return len(jobs), nil
}

// refreshJob obtiene el feed, sincroniza los eventos si cambió y persiste el
// desenlace (estado + validadores condicionales + próximo refresco). Best-effort:
// un fallo se registra en la fila (last_status='error') y se reintenta al vencer.
func (s *Service) refreshJob(ctx context.Context, job store.SubJob) {
	res := s.fetchAndSync(ctx, job)
	if err := s.store.MarkSubscriptionResult(ctx, job, res); err != nil {
		log.From(ctx).Warn("calendar: marcar suscripción", "calendar_id", job.CalendarID, "error", err)
	}
}

// fetchAndSync hace el GET condicional SSRF-safe, y si el feed cambió lo parsea
// (reusa el parser ICS/RRULE) y reemplaza el caché de eventos. Nunca ejecuta el
// contenido: es dato. Devuelve el resultado a persistir.
func (s *Service) fetchAndSync(ctx context.Context, job store.SubJob) store.SubscriptionResult {
	interval := domain.ClampRefreshInterval(job.RefreshInterval)

	fr, err := s.subClient.Fetch(ctx, job.URL, subscriptions.Conditional{
		ETag:         job.ETag,
		LastModified: job.LastModified,
	})
	if err != nil {
		log.From(ctx).Warn("calendar: fetch de suscripción", "calendar_id", job.CalendarID, "error", err)
		return store.SubscriptionResult{
			Status: "error", ETag: job.ETag, LastModified: job.LastModified,
			LastError: truncErr(err.Error()), NextRefresh: time.Now().Add(interval),
		}
	}
	// 304: el feed no cambió — no se retrae ni se re-parsea (condicional).
	if fr.NotModified {
		return store.SubscriptionResult{
			Status: "not_modified", ETag: job.ETag, LastModified: job.LastModified,
			NextRefresh: nextRefresh(interval, 0),
		}
	}

	objs, err := caldav.DecodeICSFeed(fr.Body, maxFeedObjects)
	if err != nil {
		log.From(ctx).Warn("calendar: parsear suscripción", "calendar_id", job.CalendarID, "error", err)
		return store.SubscriptionResult{
			Status: "error", ETag: job.ETag, LastModified: job.LastModified,
			LastError: truncErr(err.Error()), NextRefresh: time.Now().Add(interval),
		}
	}
	subObjs := make([]store.SubscriptionObject, 0, len(objs))
	for _, po := range objs {
		subObjs = append(subObjs, store.SubscriptionObject{Master: po.Master, Overrides: po.Overrides})
	}
	if err := s.store.SyncSubscriptionObjects(ctx, job.CalendarID, subObjs); err != nil {
		log.From(ctx).Warn("calendar: sincronizar suscripción", "calendar_id", job.CalendarID, "error", err)
		return store.SubscriptionResult{
			Status: "error", ETag: fr.ETag, LastModified: fr.LastModified,
			LastError: truncErr(err.Error()), NextRefresh: time.Now().Add(interval),
		}
	}
	return store.SubscriptionResult{
		Status: "ok", ETag: fr.ETag, LastModified: fr.LastModified,
		NextRefresh: nextRefresh(interval, fr.MaxAge),
	}
}

// nextRefresh calcula el próximo refresco: no antes de la cadencia del feed y, si
// el servidor pidió un Cache-Control mayor, lo respeta (no re-fetchea antes).
// Acotado a [Min, Max].
func nextRefresh(interval, cacheMaxAge time.Duration) time.Time {
	after := interval
	if cacheMaxAge > after {
		after = cacheMaxAge
	}
	return time.Now().Add(domain.ClampRefreshInterval(after))
}

// subscriptionSlug deriva un segmento de ruta CalDAV URL-safe del nombre visible:
// minúsculas ASCII y guiones. Vacío => un id corto.
func subscriptionSlug(displayName string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(displayName) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	if slug == "" {
		return "ics-" + shortID()
	}
	return "ics-" + slug
}

// shortID devuelve un sufijo aleatorio corto (para desambiguar slugs).
func shortID() string { return strings.SplitN(uuid.NewString(), "-", 2)[0] }

// truncErr acota el texto de error persistido (columna informativa).
func truncErr(s string) string {
	const maxLen = 500
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
