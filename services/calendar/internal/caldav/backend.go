// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package caldav implementa el servidor CalDAV (RFC 4791) de Sytue Calendar
// sobre emersion/go-webdav/caldav (BSD) y emersion/go-ical (MIT), respaldado por
// el store en PostgreSQL. Reutiliza la autenticacion de Sytue ID (JWT via JWKS):
// el principal/sub y el tenant viajan en el contexto inyectados por el
// middleware authn de platform.
//
// Clean-room: implementacion original sobre las especificaciones CalDAV
// (RFC 4791), scheduling (RFC 6638), sync-collection (RFC 6578) e iCalendar
// (RFC 5545), usando librerias permisivas. No deriva de codigo AGPL.
package caldav

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// httpError construye un error HTTP de WebDAV con el codigo dado. go-webdav lo
// serializa al cuerpo de error apropiado (RFC 4918) en la respuesta.
func httpError(code int, err error) error {
	return webdav.NewHTTPError(code, err)
}

// Backend implementa caldav.Backend sobre el store de Sytue Calendar.
type Backend struct {
	// AlGuardar recibe cada evento guardado con los asistentes que tenía antes (vacío
	// si es nuevo): con él se invita a los que se han añadido. Nil: no hace nada.
	AlGuardar func(ctx context.Context, ev domain.Event, antes []domain.Attendee)

	store  *store.Store
	prefix string // prefijo de montaje (p. ej. "/dav")
	// cache es la cache efímera compartida (Dragonfly). Guarda la LISTA DE
	// COLECCIONES del usuario, que es lo primero que pide cada sincronización
	// CalDAV. NIL es un estado normal: sin ella se consulta PostgreSQL como antes.
	cache *cache.Client
}

// UsarCache cablea la cache efímera compartida (nil la deshabilita).
func (b *Backend) UsarCache(c *cache.Client) { b.cache = c }

// colTTL es de un minuto. La lista de calendarios de una persona cambia muy poco
// —se crea uno cada varios meses—, pero el TTL se queda corto porque el precio de
// equivocarse es que un cliente no vea una colección recién creada, y eso se
// percibe como que la aplicación no funciona.
const colTTL = time.Minute

// colKey lleva el TENANT y el usuario. El tenant no es decorativo: dos personas de
// organizaciones distintas pueden compartir `sub` si algún día cambia el emisor, y
// servir la lista de calendarios de una a la otra sería un fallo de aislamiento.
func colKey(tenant, sub string) string { return "caldav:cols:" + tenant + ":" + sub }

// NewBackend construye el backend CalDAV.
func NewBackend(st *store.Store, prefix string) *Backend {
	if prefix == "" {
		prefix = "/dav"
	}
	return &Backend{store: st, prefix: strings.TrimRight(prefix, "/")}
}

// Handler construye el *caldav.Handler listo para montar tras el middleware de
// autenticacion (que inyecta claims + tenant en el contexto).
func (b *Backend) Handler() *caldav.Handler {
	// El Prefix DEBE coincidir con la ruta de montaje: emersion clasifica el tipo
	// de recurso por el nº de segmentos TRAS quitar el prefijo. Sin él, el home-set
	// /dav/calendars/<sub>/ se toma como un calendario y devuelve 404.
	return &caldav.Handler{Backend: b, Prefix: b.prefix}
}

// --- Rutas / principal -------------------------------------------------------

// principal devuelve el sub del usuario autenticado del contexto.
func principal(ctx context.Context) (string, error) {
	claims, ok := authn.FromContext(ctx)
	if !ok || claims.Subject == "" {
		return "", fmt.Errorf("caldav: contexto sin principal autenticado")
	}
	return claims.Subject, nil
}

// principalPath es la ruta del principal CalDAV del usuario.
func (b *Backend) principalPath(sub string) string {
	return path.Join(b.prefix, "principals", urlSafe(sub)) + "/"
}

// homeSetPath es la ruta del calendar-home-set del usuario (RFC 4791 6.2.1).
func (b *Backend) homeSetPath(sub string) string {
	return path.Join(b.prefix, "calendars", urlSafe(sub)) + "/"
}

// calendarPath es la ruta de una coleccion-calendario.
func (b *Backend) calendarPath(sub, name string) string {
	return path.Join(b.prefix, "calendars", urlSafe(sub), name) + "/"
}

// objectPath es la ruta de un recurso de calendario.
func (b *Backend) objectPath(sub, calName, resource string) string {
	return path.Join(b.prefix, "calendars", urlSafe(sub), calName, resource)
}

// CurrentUserPrincipal devuelve la ruta del principal actual (RFC 5397).
func (b *Backend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	sub, err := principal(ctx)
	if err != nil {
		return "", err
	}
	return b.principalPath(sub), nil
}

// CalendarHomeSetPath devuelve el calendar-home-set del usuario.
func (b *Backend) CalendarHomeSetPath(ctx context.Context) (string, error) {
	sub, err := principal(ctx)
	if err != nil {
		return "", err
	}
	// Garantiza al menos el calendario por defecto al descubrir el home set.
	if _, err := b.store.EnsureDefaultCalendar(ctx, sub); err != nil {
		return "", err
	}
	return b.homeSetPath(sub), nil
}

// --- Colecciones -------------------------------------------------------------

// ListCalendars lista las colecciones-calendario del usuario.
func (b *Backend) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	sub, err := principal(ctx)
	if err != nil {
		return nil, err
	}
	// La cache se consulta ANTES del auto-aprovisionamiento, y eso es lo que más
	// ahorra: `EnsureDefaultCalendar` es un UPSERT, o sea una ESCRITURA en la base
	// por cada PROPFIND. Los clientes de escritorio y móvil sincronizan cada pocos
	// minutos y empiezan siempre listando colecciones, así que con tres
	// dispositivos por persona eso es escritura constante sin que nadie esté
	// usando la aplicación.
	//
	// Saltárselo es correcto justamente porque hay entrada en cache: si la hay, es
	// que esta lista ya se sirvió antes, y el calendario por defecto ya existe.
	claims, _ := authn.FromContext(ctx)
	if v, ok := b.colsCacheadas(ctx, claims.Tenant, sub); ok {
		return v, nil
	}
	// Auto-aprovisiona un calendario por defecto en el primer acceso (como Mail con
	// sus buzones), para que el usuario tenga siempre una colección utilizable. Idempotente.
	if _, err := b.store.EnsureDefaultCalendar(ctx, sub); err != nil {
		return nil, err
	}
	cals, err := b.store.ListCalendars(ctx, sub)
	if err != nil {
		return nil, err
	}
	out := make([]caldav.Calendar, 0, len(cals))
	for _, c := range cals {
		out = append(out, b.toCalDAVCalendar(sub, c))
	}
	b.cachearCols(ctx, claims.Tenant, sub, out)
	return out, nil
}

// colsCacheadas devuelve la lista guardada, si la hay.
func (b *Backend) colsCacheadas(ctx context.Context, tenant, sub string) ([]caldav.Calendar, bool) {
	if !b.cache.Enabled() || tenant == "" {
		return nil, false
	}
	raw, found, err := b.cache.Get(ctx, colKey(tenant, sub))
	if err != nil || !found {
		return nil, false
	}
	var cals []caldav.Calendar
	if json.Unmarshal([]byte(raw), &cals) != nil {
		// Un valor ilegible se ignora: se sigue por el camino de siempre, que es
		// lo que pasaría sin cache.
		return nil, false
	}
	return cals, true
}

// cachearCols guarda la lista. Un fallo aquí no cambia la respuesta.
func (b *Backend) cachearCols(ctx context.Context, tenant, sub string, cals []caldav.Calendar) {
	if !b.cache.Enabled() || tenant == "" {
		return
	}
	blob, err := json.Marshal(cals)
	if err != nil {
		return
	}
	_ = b.cache.SetTTL(ctx, colKey(tenant, sub), string(blob), colTTL)
}

// invalidarCols tira la lista cacheada del usuario. Se llama en TODO camino que
// cree, renombre o borre una colección: sin esto, el TTL manda y el cliente no ve
// durante un minuto el calendario que acaba de crear.
func (b *Backend) invalidarCols(ctx context.Context, sub string) {
	if !b.cache.Enabled() {
		return
	}
	claims, _ := authn.FromContext(ctx)
	if claims.Tenant == "" {
		return
	}
	_ = b.cache.Del(ctx, colKey(claims.Tenant, sub))
}

// CreateCalendar crea una coleccion-calendario (MKCALENDAR, RFC 4791 5.3.1).
// El nombre de la coleccion se toma del ultimo segmento de la ruta; el nombre a
// mostrar y la descripcion, de las propiedades del cuerpo. El store valida el
// nombre y aisla por tenant (RLS); los conflictos se mapean a 409.
func (b *Backend) CreateCalendar(ctx context.Context, cal *caldav.Calendar) error {
	if cal == nil {
		return httpError(http.StatusBadRequest, fmt.Errorf("caldav: calendario vacio"))
	}
	sub, calName, _, err := b.parsePath(ctx, cal.Path)
	if err != nil {
		return err
	}
	if calName == "" {
		return httpError(http.StatusBadRequest, fmt.Errorf("caldav: ruta de calendario invalida"))
	}
	components := cal.SupportedComponentSet
	if len(components) == 0 {
		components = []string{string(domain.CompEvent)}
	}
	if _, err := b.store.CreateCalendar(ctx, store.NewCalendar{
		OwnerSub:    sub,
		Name:        calName,
		DisplayName: cal.Name,
		Description: cal.Description,
		Components:  components,
	}); err != nil {
		return mapErr(err)
	}
	// La lista cacheada acaba de quedarse corta: sin esto, el cliente que acaba de
	// crear el calendario no lo vería hasta que venciera el TTL, y eso se percibe
	// como que el MKCALENDAR no funcionó.
	b.invalidarCols(ctx, sub)
	return nil
}

// GetCalendar devuelve una coleccion-calendario por su ruta.
func (b *Backend) GetCalendar(ctx context.Context, urlPath string) (*caldav.Calendar, error) {
	sub, calName, _, err := b.parsePath(ctx, urlPath)
	if err != nil {
		return nil, err
	}
	c, err := b.store.GetCalendarByName(ctx, sub, calName)
	if err != nil {
		return nil, mapErr(err)
	}
	out := b.toCalDAVCalendar(sub, c)
	return &out, nil
}

func (b *Backend) toCalDAVCalendar(sub string, c domain.Calendar) caldav.Calendar {
	desc := c.Description
	return caldav.Calendar{
		Path:                  b.calendarPath(sub, c.Name),
		Name:                  orStr(c.DisplayName, c.Name),
		Description:           desc,
		SupportedComponentSet: c.Components,
		MaxResourceSize:       10 * 1024 * 1024,
	}
}

// --- Recursos ----------------------------------------------------------------

// GetCalendarObject devuelve un recurso de calendario por su ruta.
func (b *Backend) GetCalendarObject(ctx context.Context, urlPath string, _ *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	sub, calName, resource, err := b.parsePath(ctx, urlPath)
	if err != nil {
		return nil, err
	}
	if resource == "" {
		return nil, httpError(http.StatusNotFound, fmt.Errorf("caldav: recurso vacio"))
	}
	cal, err := b.store.GetCalendarByName(ctx, sub, calName)
	if err != nil {
		return nil, mapErr(err)
	}
	obj, err := b.store.GetObjectByResource(ctx, cal.ID, resource)
	if err != nil {
		return nil, mapErr(err)
	}
	return b.toCalDAVObject(sub, calName, obj)
}

// ListCalendarObjects lista los recursos de una coleccion.
func (b *Backend) ListCalendarObjects(ctx context.Context, urlPath string, _ *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	sub, calName, _, err := b.parsePath(ctx, urlPath)
	if err != nil {
		return nil, err
	}
	cal, err := b.store.GetCalendarByName(ctx, sub, calName)
	if err != nil {
		return nil, mapErr(err)
	}
	objs, err := b.store.ListObjects(ctx, cal.ID)
	if err != nil {
		return nil, err
	}
	out := make([]caldav.CalendarObject, 0, len(objs))
	for _, o := range objs {
		co, err := b.toCalDAVObject(sub, calName, o)
		if err != nil {
			continue // recurso ilegible: lo omitimos en el listado
		}
		out = append(out, *co)
	}
	return out, nil
}

// QueryCalendarObjects ejecuta un calendar-query (RFC 4791 7.8): filtra por
// rango temporal si se especifica un time-range en el filtro de componentes.
func (b *Backend) QueryCalendarObjects(ctx context.Context, urlPath string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	objs, err := b.ListCalendarObjects(ctx, urlPath, nil)
	if err != nil {
		return nil, err
	}
	if query == nil {
		return objs, nil
	}
	start, end, ok := compFilterRange(query.CompFilter)
	if !ok {
		return objs, nil
	}
	// Filtra por solapamiento del rango sobre el VEVENT serializado.
	var filtered []caldav.CalendarObject
	for _, o := range objs {
		if o.Data == nil {
			continue
		}
		if objectOverlaps(o.Data, start, end) {
			filtered = append(filtered, o)
		}
	}
	return filtered, nil
}

// PutCalendarObject crea o sustituye un recurso (PUT CalDAV). Respeta las
// precondiciones If-Match/If-None-Match via opts. Devuelve el recurso resultante.
func (b *Backend) PutCalendarObject(ctx context.Context, urlPath string, calendar *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	sub, calName, resource, err := b.parsePath(ctx, urlPath)
	if err != nil {
		return nil, err
	}
	cal, err := b.store.GetCalendarByName(ctx, sub, calName)
	if err != nil {
		return nil, mapErr(err)
	}

	data, err := encodeCalendar(calendar)
	if err != nil {
		return nil, httpError(http.StatusBadRequest, err)
	}
	po, err := DecodeCalendarObject(data, cal.ID.String())
	if err != nil {
		return nil, httpError(http.StatusBadRequest, err)
	}
	in := po.Master
	in.CalendarID = cal.ID
	in.ICalData = data
	if resource != "" {
		in.ResourceName = resource
	}

	// Determina create vs update segun If-None-Match / existencia previa.
	create := opts != nil && opts.IfNoneMatch.IsWildcard()
	expectETag := uuid.Nil
	if opts != nil {
		if et, eerr := opts.IfMatch.ETag(); eerr == nil {
			if parsed, perr := uuid.Parse(strings.Trim(et, `"`)); perr == nil {
				expectETag = parsed
			}
		}
	}

	var antes []domain.Attendee
	if !create {
		// Si no existe el objeto, crea; si existe, actualiza.
		if prev, gerr := b.store.GetObjectByUID(ctx, cal.ID, in.UID); gerr != nil {
			create = true
		} else if po, derr := DecodeCalendarObject(prev.Data, cal.ID.String()); derr == nil {
			antes = po.Master.Attendees
		}
	}

	var (
		ev  domain.Event
		obj domain.CalendarObject
	)
	if create {
		ev, obj, err = b.store.CreateEvent(ctx, in)
	} else {
		ev, obj, err = b.store.UpdateEvent(ctx, in, expectETag)
	}
	if err != nil {
		return nil, mapErr(err)
	}
	if b.AlGuardar != nil {
		b.AlGuardar(ctx, ev, antes)
	}
	co, err := b.toCalDAVObject(sub, calName, obj)
	if err != nil {
		return nil, err
	}
	return co, nil
}

// DeleteCalendarObject borra un recurso de calendario.
func (b *Backend) DeleteCalendarObject(ctx context.Context, urlPath string) error {
	sub, calName, resource, err := b.parsePath(ctx, urlPath)
	if err != nil {
		return err
	}
	cal, err := b.store.GetCalendarByName(ctx, sub, calName)
	if err != nil {
		return mapErr(err)
	}
	obj, err := b.store.GetObjectByResource(ctx, cal.ID, resource)
	if err != nil {
		return mapErr(err)
	}
	if err := b.store.DeleteEvent(ctx, cal.ID, obj.UID, uuid.Nil); err != nil {
		return mapErr(err)
	}
	return nil
}

// --- Conversion de recursos --------------------------------------------------

func (b *Backend) toCalDAVObject(sub, calName string, o domain.CalendarObject) (*caldav.CalendarObject, error) {
	cal, err := ical.NewDecoder(strings.NewReader(o.Data)).Decode()
	if err != nil {
		return nil, err
	}
	return &caldav.CalendarObject{
		Path:          b.objectPath(sub, calName, o.ResourceName),
		ModTime:       o.UpdatedAt,
		ContentLength: int64(o.SizeBytes),
		ETag:          o.ETag.String(),
		Data:          cal,
	}, nil
}

// --- Helpers -----------------------------------------------------------------

// parsePath descompone una ruta CalDAV en (sub, calendario, recurso). Valida que
// el principal de la ruta coincida con el usuario autenticado (aislamiento).
func (b *Backend) parsePath(ctx context.Context, urlPath string) (sub, calName, resource string, err error) {
	authSub, err := principal(ctx)
	if err != nil {
		return "", "", "", err
	}
	trimmed := strings.TrimPrefix(urlPath, b.prefix)
	trimmed = strings.Trim(trimmed, "/")
	segs := strings.Split(trimmed, "/")
	// Formato esperado: calendars/<sub>/<calendar>[/<resource>]
	if len(segs) >= 2 && segs[0] == "calendars" {
		pathSub := segs[1]
		if pathSub != urlSafe(authSub) && pathSub != authSub {
			return "", "", "", httpError(http.StatusForbidden,
				fmt.Errorf("caldav: principal de ruta no coincide con el autenticado"))
		}
		if len(segs) >= 3 {
			calName = segs[2]
		}
		if len(segs) >= 4 {
			resource = strings.Join(segs[3:], "/")
		}
		return authSub, calName, resource, nil
	}
	return authSub, "", "", nil
}

// mapErr traduce errores de dominio a errores CalDAV con su codigo HTTP.
func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case isDomain(err, domain.ErrNotFound):
		return httpError(http.StatusNotFound, err)
	case isDomain(err, domain.ErrConflict):
		return httpError(http.StatusConflict, err)
	case isDomain(err, domain.ErrPreconditionFailed):
		return httpError(http.StatusPreconditionFailed, err)
	case isDomain(err, domain.ErrInvalidEvent), isDomain(err, domain.ErrInvalidRRULE),
		isDomain(err, domain.ErrInvalidName), isDomain(err, domain.ErrInvalidRange):
		return httpError(http.StatusBadRequest, err)
	case isDomain(err, domain.ErrDefaultImmutable):
		return httpError(http.StatusForbidden, err)
	default:
		return err
	}
}

// encodeCalendar serializa un *ical.Calendar a texto iCalendar.
func encodeCalendar(cal *ical.Calendar) (string, error) {
	if cal == nil {
		return "", fmt.Errorf("caldav: VCALENDAR vacio")
	}
	var sb strings.Builder
	if err := ical.NewEncoder(&sb).Encode(cal); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// objectOverlaps indica si algun VEVENT del recurso solapa [start, end).
func objectOverlaps(cal *ical.Calendar, start, end time.Time) bool {
	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		evt := &ical.Event{Component: child}
		s, _, err := propTime(evt.Props, ical.PropDateTimeStart)
		if err != nil {
			continue
		}
		e, _, err := propTime(evt.Props, ical.PropDateTimeEnd)
		if err != nil {
			e = s
		}
		if s.Before(end) && start.Before(e) {
			return true
		}
	}
	return false
}

// compFilterRange extrae el time-range del filtro de componentes de un
// calendar-query, si lo hay (busca en VCALENDAR > VEVENT).
func compFilterRange(cf caldav.CompFilter) (time.Time, time.Time, bool) {
	var walk func(caldav.CompFilter) (time.Time, time.Time, bool)
	walk = func(c caldav.CompFilter) (time.Time, time.Time, bool) {
		if !c.Start.IsZero() || !c.End.IsZero() {
			start, end := c.Start, c.End
			if start.IsZero() {
				start = time.Unix(0, 0)
			}
			if end.IsZero() {
				end = start.AddDate(10, 0, 0)
			}
			return start.UTC(), end.UTC(), true
		}
		for _, child := range c.Comps {
			if s, e, ok := walk(child); ok {
				return s, e, ok
			}
		}
		return time.Time{}, time.Time{}, false
	}
	return walk(cf)
}

func isDomain(err, target error) bool {
	return errors.Is(err, target)
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// urlSafe convierte un sub en un segmento de ruta seguro (sin separadores).
func urlSafe(s string) string {
	return strings.NewReplacer("/", "_", " ", "_").Replace(s)
}
