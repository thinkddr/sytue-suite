// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// Cadencia del prober de uptime. El worker despierta cada uptimeTick y DRENA en
// bucle: reclama lotes de checks vencidos (uptimeBatch, el tope del store) y los
// sondea con concurrencia acotada, re-reclamando mientras el lote venga lleno. El
// throughput deja así de estar atado a la cadencia del tick (era ~batch/tick,
// ~10/s/réplica) y pasa a estarlo a la concurrencia y la latencia del sondeo. El
// intervalo EFECTIVO de cada check lo gobierna su interval_seconds (>=30s); el
// tick es solo la granularidad con la que se despierta a buscar backlog.
const (
	uptimeTick       = 10 * time.Second
	uptimeBatch      = 500 // tope de ClaimDueChecks; menos round-trips por drenaje
	probeTimeout     = 10 * time.Second
	probeConcurrency = 200 // sondeos HTTP en paralelo por lote (acota goroutines/sockets)
	probeUserAgent   = "sytue-cloud/uptime-prober"
)

// RunUptimeProber arranca el worker de monitorización de uptime (sin estado,
// cross-réplica): cada tick RECLAMA checks vencidos con FOR UPDATE SKIP LOCKED
// (reparte entre réplicas), los sondea por HTTP GET con guard anti-SSRF en el
// dial y registra el resultado; en la transición up↔down emite un evento al bus.
// Bloquea hasta que se cancele ctx. Lo arranca cmd/cloud-api.
func (s *Service) RunUptimeProber(ctx context.Context) error {
	p := &uptimeProber{st: s.pstore, client: newProbeClient()}
	return p.run(ctx)
}

type uptimeProber struct {
	st     *store.Store
	client *http.Client
}

func (p *uptimeProber) run(ctx context.Context) error {
	ticker := time.NewTicker(uptimeTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := p.tick(ctx); err != nil {
				log.From(ctx).Warn("tick del prober de uptime falló", "error", err)
			}
		}
	}
}

// tick DRENA el backlog de checks vencidos: reclama un lote, lo sondea y, mientras
// venga lleno (había más vencidos que el tope del lote), re-reclama de inmediato
// sin esperar al próximo tick. ClaimDueChecks arrienda cada fila hacia delante
// (next_run_at += interval, >=30s) antes de devolverla, así que la re-reclamación
// nunca re-selecciona lo ya tomado y el bucle termina en cuanto no queda vencido.
// Sin este drenaje el throughput quedaba fijado en ~uptimeBatch/uptimeTick por
// réplica, muy por debajo de la escala objetivo.
//
// ponytail: drenaje síncrono lote a lote; el techo es probeConcurrency/latencia
// por réplica (repartido cross-réplica por SKIP LOCKED). Si una sola réplica no
// vacía el backlog, subir concurrencia/réplicas o desacoplar sondeo de reclamación.
func (p *uptimeProber) tick(ctx context.Context) error {
	for {
		claimed, err := p.st.ClaimDueChecks(ctx, time.Now(), uptimeBatch)
		if err != nil {
			return err
		}
		if len(claimed) == 0 {
			return nil
		}
		p.probeBatch(ctx, claimed)
		if len(claimed) < uptimeBatch || ctx.Err() != nil {
			//nolint:nilerr // la cancelación del ctx NO es un fallo del sondeo: se sale limpio y se reanuda en el próximo tick.
			return nil // lote incompleto (backlog vacío) o cancelación: espera al próximo tick
		}
	}
}

// probeBatch sondea un lote reclamado de forma concurrente (acotada por
// probeConcurrency) para que un objetivo colgado no bloquee al resto del lote
// durante todo el probeTimeout, y registra cada resultado.
func (p *uptimeProber) probeBatch(ctx context.Context, claimed []store.ClaimedCheck) {
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for _, c := range claimed {
		wg.Add(1)
		sem <- struct{}{}
		go func(c store.ClaimedCheck) {
			defer wg.Done()
			defer func() { <-sem }()
			status, latency := p.probe(ctx, c.TargetURL)
			if err := p.st.RecordCheckResult(ctx, c, status, latency, time.Now()); err != nil {
				log.From(ctx).Warn("registrar resultado de uptime falló",
					"check", c.ID, "tenant", c.Tenant, "error", err)
			}
		}(c)
	}
	wg.Wait()
}

// probe hace un HTTP GET al objetivo y devuelve el estado (up/down) y la latencia
// en ms. up = respuesta 2xx/3xx dentro del timeout; cualquier error de red, 4xx/5xx
// o timeout = down. El cliente lleva el guard anti-SSRF en el dial (safeDialControl,
// tras resolver DNS: cierra el rebinding a IP interna); además se revalida la URL
// por si el estado de la fila quedó obsoleto respecto a la política vigente.
func (p *uptimeProber) probe(ctx context.Context, target string) (string, int) {
	if err := domain.ValidateUptimeTarget(target); err != nil {
		return domain.UptimeStatusDown, 0
	}
	reqCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return domain.UptimeStatusDown, 0
	}
	req.Header.Set("User-Agent", probeUserAgent)
	start := time.Now()
	resp, err := p.client.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return domain.UptimeStatusDown, latency
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return domain.UptimeStatusUp, latency
	}
	return domain.UptimeStatusDown, latency
}

// newProbeClient es el cliente HTTP del prober: mismo guard anti-SSRF en el dial
// que el emisor de webhooks (safeDialControl) y sin seguir redirecciones (un 3xx
// a un destino interno NO se persigue). Timeout total probeTimeout.
func newProbeClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: safeDialControl}
	return &http.Client{
		Timeout: probeTimeout,
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// ---------------------------------------------------------------------------
// API de aplicación: uptime checks
// ---------------------------------------------------------------------------

// CreateUptimeCheckInput son los datos de creación desde la API. Enabled nil = true.
type CreateUptimeCheckInput struct {
	ProjectID       uuid.UUID
	Name            string
	TargetURL       string
	IntervalSeconds int
	Enabled         *bool
}

// CreateUptimeCheck valida (SSRF crítico) y crea un check. Si TargetURL viene
// vacío, usa por defecto la URL del deployment de producción activo del proyecto.
func (s *Service) CreateUptimeCheck(ctx context.Context, in CreateUptimeCheckInput) (domain.UptimeCheck, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return domain.UptimeCheck{}, fmt.Errorf("%w: name requerido", domain.ErrInvalidInput)
	}
	target := strings.TrimSpace(in.TargetURL)
	if target == "" {
		prod, err := s.pstore.GetActiveProduction(ctx, in.ProjectID)
		if err != nil {
			return domain.UptimeCheck{}, fmt.Errorf(
				"%w: sin target_url y el proyecto no tiene un deployment de producción activo", domain.ErrInvalidInput,
			)
		}
		target = prod.URL
	}
	if err := domain.ValidateUptimeTarget(target); err != nil {
		return domain.UptimeCheck{}, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	return s.pstore.CreateUptimeCheck(ctx, store.NewUptimeCheckRecord{
		ProjectID:       in.ProjectID,
		Name:            name,
		TargetURL:       target,
		IntervalSeconds: domain.NormalizeInterval(in.IntervalSeconds),
		Enabled:         enabled,
	})
}

// ListUptimeChecks lista los checks de un proyecto.
func (s *Service) ListUptimeChecks(ctx context.Context, projectID uuid.UUID) ([]domain.UptimeCheck, error) {
	return s.pstore.ListUptimeChecks(ctx, projectID)
}

// GetUptimeCheck devuelve un check por ID.
func (s *Service) GetUptimeCheck(ctx context.Context, id uuid.UUID) (domain.UptimeCheck, error) {
	return s.pstore.GetUptimeCheck(ctx, id)
}

// UpdateUptimeCheck valida (SSRF en target) y aplica una actualización parcial.
func (s *Service) UpdateUptimeCheck(ctx context.Context, id uuid.UUID, upd store.UpdateUptimeCheckRecord) (domain.UptimeCheck, error) {
	if upd.Name != nil {
		n := strings.TrimSpace(*upd.Name)
		if n == "" {
			return domain.UptimeCheck{}, fmt.Errorf("%w: name vacío", domain.ErrInvalidInput)
		}
		upd.Name = &n
	}
	if upd.TargetURL != nil {
		t := strings.TrimSpace(*upd.TargetURL)
		if err := domain.ValidateUptimeTarget(t); err != nil {
			return domain.UptimeCheck{}, err
		}
		upd.TargetURL = &t
	}
	if upd.IntervalSeconds != nil {
		v := domain.NormalizeInterval(*upd.IntervalSeconds)
		upd.IntervalSeconds = &v
	}
	return s.pstore.UpdateUptimeCheck(ctx, id, upd)
}

// DeleteUptimeCheck elimina un check.
func (s *Service) DeleteUptimeCheck(ctx context.Context, id uuid.UUID) error {
	return s.pstore.DeleteUptimeCheck(ctx, id)
}

// ---------------------------------------------------------------------------
// API de aplicación: alert policies
// ---------------------------------------------------------------------------

// CreateAlertPolicyInput son los datos de creación desde la API. Enabled nil = true.
type CreateAlertPolicyInput struct {
	ProjectID  *uuid.UUID
	Event      string
	ChannelURL string
	Enabled    *bool
}

// CreateAlertPolicy valida el evento y la URL de canal (anti-SSRF) y crea la policy.
// Si el canal es `mailto:`, la alerta nace sin confirmar (`Confirmed=false`),
// guarda el hash SHA-256 de un token aleatorio y envía un correo de confirmación
// al destinatario para evitar usar Sytue Cloud como amplificador de correo hacia
// direcciones de terceros sin consentimiento (CLOUD-19).
func (s *Service) CreateAlertPolicy(ctx context.Context, in CreateAlertPolicyInput) (domain.AlertPolicy, error) {
	if !domain.ValidAlertEvent(in.Event) {
		return domain.AlertPolicy{}, fmt.Errorf("%w: event no soportado", domain.ErrInvalidInput)
	}
	channel := strings.TrimSpace(in.ChannelURL)
	if err := domain.ValidateAlertChannel(channel); err != nil {
		return domain.AlertPolicy{}, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	rec := store.NewAlertPolicyRecord{
		ProjectID:  in.ProjectID,
		Event:      in.Event,
		ChannelURL: channel,
		Enabled:    enabled,
		Confirmed:  true,
	}
	addr, isMail := domain.MailChannelAddress(channel)
	var rawTok string
	if isMail {
		tok, hash, err := newAlertConfirmToken()
		if err != nil {
			return domain.AlertPolicy{}, err
		}
		rawTok = tok
		rec.Confirmed = false
		rec.ConfirmTokenHash = &hash
	}
	pol, err := s.store.CreateAlertPolicy(ctx, rec)
	if err != nil {
		return domain.AlertPolicy{}, err
	}
	if isMail {
		s.sendAlertConfirmEmail(ctx, addr, in.Event, rawTok)
	}
	return pol, nil
}

// ListAlertPolicies lista las policies del tenant.
func (s *Service) ListAlertPolicies(ctx context.Context) ([]domain.AlertPolicy, error) {
	return s.pstore.ListAlertPolicies(ctx)
}

// UpdateAlertPolicy valida (evento/URL si vienen) y aplica una actualización parcial.
// Si se cambia channel_url a un canal `mailto:`, reinicia el estado de confirmación
// y envía un nuevo enlace al destinatario (CLOUD-19).
func (s *Service) UpdateAlertPolicy(ctx context.Context, id uuid.UUID, upd store.UpdateAlertPolicyRecord) (domain.AlertPolicy, error) {
	if upd.Event != nil && !domain.ValidAlertEvent(*upd.Event) {
		return domain.AlertPolicy{}, fmt.Errorf("%w: event no soportado", domain.ErrInvalidInput)
	}
	var mailAddr, rawTok string
	if upd.ChannelURL != nil {
		c := strings.TrimSpace(*upd.ChannelURL)
		if err := domain.ValidateAlertChannel(c); err != nil {
			return domain.AlertPolicy{}, err
		}
		upd.ChannelURL = &c
		if addr, ok := domain.MailChannelAddress(c); ok {
			tok, hash, err := newAlertConfirmToken()
			if err != nil {
				return domain.AlertPolicy{}, err
			}
			mailAddr = addr
			rawTok = tok
			upd.ResetConfirm = true
			upd.ConfirmTokenHash = &hash
		}
	}
	pol, err := s.store.UpdateAlertPolicy(ctx, id, upd)
	if err != nil {
		return domain.AlertPolicy{}, err
	}
	if mailAddr != "" {
		s.sendAlertConfirmEmail(ctx, mailAddr, pol.Event, rawTok)
	}
	return pol, nil
}

// DeleteAlertPolicy elimina una policy.
func (s *Service) DeleteAlertPolicy(ctx context.Context, id uuid.UUID) error {
	return s.pstore.DeleteAlertPolicy(ctx, id)
}

// ConfirmAlertPolicy confirma una alerta de correo a partir del token en claro
// recibido en el enlace de confirmación (CLOUD-19).
func (s *Service) ConfirmAlertPolicy(ctx context.Context, rawToken string) (domain.AlertPolicy, error) {
	tok := strings.TrimSpace(rawToken)
	if tok == "" {
		return domain.AlertPolicy{}, fmt.Errorf("%w: token de confirmación requerido", domain.ErrInvalidInput)
	}
	sum := sha256.Sum256([]byte(tok))
	hash := hex.EncodeToString(sum[:])
	return s.store.ConfirmAlertPolicyByToken(ctx, hash)
}

func newAlertConfirmToken() (rawToken, tokenHash string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("generar token de confirmación de alerta: %w", err)
	}
	rawToken = hex.EncodeToString(b[:])
	sum := sha256.Sum256([]byte(rawToken))
	return rawToken, hex.EncodeToString(sum[:]), nil
}

func (s *Service) sendAlertConfirmEmail(ctx context.Context, to, event, rawToken string) {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.PublicURL), "/")
	if base == "" {
		base = "https://cloud.sytue.eu/cloud"
	} else if !strings.HasSuffix(base, "/cloud") {
		base += "/cloud"
	}
	link := base + "/confirm-alert?token=" + url.QueryEscape(rawToken)
	subject := "[Sytue Cloud] Confirma la recepción de alertas (" + event + ")"
	body := "Alguien ha configurado esta dirección (" + to + ") como destinataria de alertas de Sytue Cloud para el evento «" + event + "».\n\n" +
		"Para activar el envío de alertas a este buzón, confirma la suscripción visitando el siguiente enlace:\n\n" +
		"  " + link + "\n\n" +
		"Si no has solicitado estas alertas, ignora este mensaje: no se enviará ningún aviso mientras no confirmes el enlace."
	if err := sendAlertMail(ctx, s.cfg, to, subject, body); err != nil {
		log.From(ctx).Warn("enviar correo de confirmación de alerta falló", "to", logsafe.Text(to), "error", err)
	}
}
