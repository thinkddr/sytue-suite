// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/config"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/intents"
	"git.sytue.eu/sytue/one/services/hub/internal/interpretar"
	"git.sytue.eu/sytue/one/services/hub/internal/notify"
)

// Las siguientes interfaces son la superficie MÍNIMA que los handlers consumen de
// cada servicio de aplicación. En producción las satisfacen *search.Service,
// *notify.Service, *presence.Service y *store.Store respectivamente; declararlas
// como interfaces permite inyectar fakes en los tests HTTP sin PostgreSQL ni
// conectores upstream, sin alterar el cableado de NewRouter.

// searcher ejecuta la búsqueda federada y la actividad reciente cross-app.
type searcher interface {
	Search(ctx context.Context, token string, q domain.SearchQuery) (domain.SearchResponse, error)
	Recent(ctx context.Context, token string, sources []domain.Source, limit int) domain.SearchResponse
}

// notifier crea notificaciones y expone el contador de no leídas (handshake SSE).
type notifier interface {
	Create(ctx context.Context, rec domain.NewNotification) (domain.Notification, error)
	CreateHistorico(ctx context.Context, h domain.NewHistorico) (string, error)
	UnreadCount(ctx context.Context, subjects []string) (int, error)
	PublishEvent(ctx context.Context, tenant, subject, event string, data json.RawMessage) error
}

// notifStore es la superficie de persistencia de notificaciones que usan los
// handlers directamente.
type notifStore interface {
	ListNotifications(ctx context.Context, subjects []string, onlyUnread bool, before *domain.Antes, limit int) ([]domain.Notification, error)
	UnreadCount(ctx context.Context, subjects []string) (int, error)
	MarkRead(ctx context.Context, id uuid.UUID, subjects []string) error
	MarkAllRead(ctx context.Context, subjects []string) (int64, error)
	ListNotificationPrefs(ctx context.Context, subject string) ([]domain.NotificationPref, error)
	SetNotificationPref(ctx context.Context, subject string, p domain.NotificationPref) error
	OlvidarBuzon(ctx context.Context, subject string) (int64, error)
}

// presenceSvc gestiona la presencia de usuarios.
type presenceSvc interface {
	Heartbeat(ctx context.Context, subject string, state domain.PresenceState, status string) error
	Offline(ctx context.Context, subject string) error
	Query(ctx context.Context, subjects []string) ([]domain.Presence, error)
	TTL() time.Duration
}

// Handlers agrupa los manejadores HTTP de Sytue Hub.
type Handlers struct {
	cfg      config.Config
	search   searcher
	notify   notifier
	hub      *notify.Hub
	store    notifStore
	presence presenceSvc
	intents  *intents.Resolver
	// interprete traduce una frase a parámetros de búsqueda. Puede ser nil o
	// estar inactivo: sin servicio de IA la búsqueda funciona como siempre.
	interprete *interpretar.Cliente
	// instaladores sirve al actualizador de las apps de escritorio; nil sin bucket.
	instaladores catalogoInstaladores
	// correo cuenta los correos sin leer de la persona en Sytue Mail; nil sin Mail.
	correo buzonSinLeer
	// orgs da el nombre visible de las organizaciones de fuera; nil sin Sytue ID.
	orgs nombresOrgs
}

// buzonSinLeer es lo que se usa del conector de Mail (connectors.MailConnector).
type buzonSinLeer interface {
	SinLeer(ctx context.Context, token string) (int64, error)
}

// Apps responde qué productos levanta ESTA instalación.
//
//	GET /api/v1/apps  →  {"apps":["one","mail","drive",…]}
//
// El conmutador de aplicaciones y el Launcher del portal lo consultan para no
// ofrecer lo que no está: en el perfil piloto no se despliega Sytue Cloud, y su
// tesela seguía llevando a un `cloud.<dominio>` inexistente. Un acceso que solo
// puede acabar en error parece una avería del producto.
//
// LISTA VACÍA = «no se sabe», y el frontal presenta la suite entera. Es la
// degradación correcta: como mucho se ofrece algo que no está —lo de siempre—,
// en vez de esconder media suite porque una variable no llegó o porque el Hub
// aún no se ha actualizado.
//
// Va aquí y no en cada aplicación porque el Hub es el único servicio que las
// once ya conocen (`hubBaseUrl` está en la shell). Y quién lo levanta lo sabe el
// DESPLIEGUE, así que se lee de la configuración en vez de sondear dominios: una
// sonda diría «no responde» ante un corte de red y apagaría accesos que sí
// existen.
func (h *Handlers) Apps(w http.ResponseWriter, r *http.Request) {
	apps := h.cfg.AppsDesplegadas
	if apps == nil {
		apps = []string{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"apps": apps})
}

// toAPIError traduce errores de dominio a *httpx.Error canónicos.
func toAPIError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.ErrBadRequest.WithMessage("Invalid parameters.")
	case errors.Is(err, domain.ErrUnknownSource):
		return httpx.ErrBadRequest.WithMessage("Unknown search source.")
	case errors.Is(err, domain.ErrUnknownIntent):
		return httpx.ErrBadRequest.WithMessage("Intent no soportado.")
	case errors.Is(err, domain.ErrForbidden):
		return httpx.ErrForbidden
	case errors.Is(err, domain.ErrGone):
		return httpx.NewError(http.StatusGone, "gone", "This maintenance operation is closed.")
	case errors.Is(err, domain.ErrTooMany):
		return httpx.ErrTooManyRequests.WithMessage("Too many notifications for this recipient.")
	default:
		return httpx.ErrInternal.Wrap(err)
	}
}

// personales es el contexto de la petición con los buzones PERSONALES de quien llama
// (ADR 0053): con ellos lee también sus avisos de otras organizaciones.
func personales(ctx context.Context) context.Context {
	c, _ := authn.FromContext(ctx)
	return domain.ConBuzonesPersonales(ctx, buzonesPersonalesDel(c), c.Emitido)
}

// buzonesPersonalesDel token: ninguno si es DELEGADO (`act`): lo que es de la persona
// en todas sus organizaciones no se le abre a quien actúa en su nombre; se queda con
// lo de la organización del token (ADR 0053).
func buzonesPersonalesDel(c authn.Claims) []string {
	if c.Delegado {
		return nil
	}
	return domain.BuzonesPersonales(c.Subject, append([]string{c.Mailbox}, c.Mailboxes...)...)
}

// buzones son los destinatarios de avisos del usuario autenticado: su `sub` y sus
// direcciones (ver domain.Destinatarios).
func buzones(r *http.Request) []string {
	c, _ := authn.FromContext(r.Context())
	return domain.Destinatarios(c.Subject, c.Email, c.Mailbox)
}

// subFromCtx devuelve el subject del usuario autenticado.
func subFromCtx(r *http.Request) string {
	if c, ok := authn.FromContext(r.Context()); ok {
		return c.Subject
	}
	return ""
}

// bearerToken extrae el JWT crudo de la cabecera Authorization para reenviarlo a
// los servicios upstream en la búsqueda federada (llamadas en nombre del usuario).
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// ---------------------------------------------------------------------------
// Búsqueda federada
// ---------------------------------------------------------------------------

// Search ejecuta la búsqueda federada:
// GET /api/v1/search?q=…&sources=a,b&limit=n&offset=n&from=…&to=…&kinds=file,message
//
// `offset` pagina sobre el AGREGADO ordenado por relevancia (no por origen: el orden
// que le importa a quien busca es el de todos los resultados juntos). `from`/`to` son
// instantes RFC 3339 y `kinds` un CSV de tipos.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	// `httpx.Consulta` Y NO `r.URL.Query()`, porque esta ruta está montada con
	// `GetYQuery`: por QUERY el término viaja en el CUERPO y la query string
	// llega vacía. Leyéndola de la URL, `q` salía siempre vacío, el servicio
	// devolvía `ErrInvalidInput` y el buscador global de la suite entera
	// respondía «No se pudo completar la búsqueda» — medido en producción el
	// 2026-09-14: 400 `bad_request` sobre `https://hub.sytue.eu/api/v1/search`
	// SIN query string. El cliente tampoco se recuperaba: solo cae a GET ante
	// 404, 405 o 501, y esto era un 400.
	p, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sources, err := parseSources(p)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	from, hasFrom, err := parseInstant(p, "from")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	to, hasTo, err := parseInstant(p, "to")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := domain.SearchQuery{
		Text:    p.Get("q"),
		Sources: sinSoloOrganizacion(r, sources),
		Limit:   atoiDefault(p.Get("limit"), 0),
		Offset:  atoiDefault(p.Get("offset"), 0),
		Kinds:   splitCSV(p.Get("kinds")),
	}
	if hasFrom {
		q.From = &from
	}
	if hasTo {
		q.To = &to
	}

	// `ia=1` pide entender la frase antes de buscarla. Se hace ANTES del fan-out
	// porque lo que cambia son los parámetros del fan-out.
	var entendido *domain.Interpreted
	if p.Get("ia") == "1" {
		q, entendido = h.interpretarConsulta(r, q)
	}

	resp, err := h.search.Search(r.Context(), bearerToken(r), q)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	resp.Interpreted = entendido
	httpx.JSON(w, r, http.StatusOK, resp)
}

// interpretarConsulta cambia la consulta por la que entendió la IA, o la deja
// como estaba.
//
// UN FALLO AQUÍ NO ROMPE LA BÚSQUEDA, y esa es la regla de la que cuelga todo lo
// demás: si el servicio de IA no está, tarda de más o contesta cualquier cosa,
// se busca literalmente igual que siempre. Interpretar es una mejora, no un
// requisito; convertirlo en un requisito sería atar el buscador de la suite
// entera a que un modelo conteste a tiempo.
func (h *Handlers) interpretarConsulta(r *http.Request, q domain.SearchQuery) (domain.SearchQuery, *domain.Interpreted) {
	if !h.interprete.Activo() || !interpretar.MereceInterpretarse(q.Text) {
		return q, nil
	}
	in, err := h.interprete.Interpretar(r.Context(), bearerToken(r), q.Text, time.Now())
	if err != nil {
		// Se registra y se sigue: quien busca no tiene por qué enterarse de que
		// el asistente no contestó, porque va a recibir sus resultados igual.
		log.From(r.Context()).Warn("no se pudo interpretar la consulta", "error", err)
		return q, nil
	}
	// SIN TÉRMINO NO HAY BÚSQUEDA, y por eso una interpretación que se queda sin
	// texto se tira entera. Pasa con las frases que son TODO filtro —«los correos
	// de la semana pasada»—: el modelo deja `texto` vacío, que es lo correcto por
	// su parte, pero la búsqueda federada exige término y devolvería un 400.
	//
	// Se descarta en vez de rellenarlo con la frase original porque eso sería lo
	// peor de las dos cosas: buscar literalmente «los correos de la semana
	// pasada» Y ADEMÁS acotado al rango, o sea cero resultados garantizados.
	//
	// ponytail: el fan-out sin término ya existe (`Recent`), pero no admite
	// `kinds` ni rango. El día que `Search` acepte una consulta solo de filtros,
	// esto se cae y estas frases empiezan a funcionar.
	if strings.TrimSpace(in.Texto) == "" {
		return q, nil
	}

	// Lo que la persona pidió A MANO manda sobre lo que dedujo el modelo: quien
	// ha marcado «solo Drive» en la interfaz no quiere que una frase se lo
	// cambie por debajo.
	nueva := q
	nueva.Text = in.Texto
	if len(q.Sources) == 0 {
		nueva.Sources = in.Fuentes
	}
	if len(q.Kinds) == 0 {
		nueva.Kinds = in.Tipos
	}
	if q.From == nil {
		nueva.From = in.Desde
	}
	if q.To == nil {
		nueva.To = in.Hasta
	}
	return nueva, &domain.Interpreted{
		Text:    nueva.Text,
		Sources: nueva.Sources,
		Kinds:   nueva.Kinds,
		From:    nueva.From,
		To:      nueva.To,
	}
}

// Recent expone la ACTIVIDAD RECIENTE cross-app del usuario:
// GET /api/v1/recent?sources=a,b&limit=n. Para cada origen devuelve sus ítems más
// recientes SIN término de búsqueda, agregados y ordenados por recencia.
// Reutiliza el fan-out federado (en nombre del usuario vía Bearer, degradación
// elegante: un origen sin recientes o caído no rompe el resto).
func (h *Handlers) Recent(w http.ResponseWriter, r *http.Request) {
	// GET a secas —`/recent` no está montada con QUERY—, así que sus parámetros
	// son los de la URL.
	sources, err := parseSources(r.URL.Query())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	limit := atoiDefault(r.URL.Query().Get("limit"), 0)
	resp := h.search.Recent(r.Context(), bearerToken(r), sinSoloOrganizacion(r, sources), limit)
	httpx.JSON(w, r, http.StatusOK, resp)
}

// sinSoloOrganizacion quita SyERP, Pages y Bases de la federación POR DEFECTO cuando quien
// pregunta es una cuenta personal. Son productos de organizaciones: sus servicios le
// contestan 403, y el Hub lo contaba como «Sytue Web y Sytue ERP no han respondido»
// en cada búsqueda, un aviso que no puede arreglar nadie. Solo si no pidió orígenes
// concretos: quien nombra `sources=pages` lo está pidiendo y ve lo que pase.
func sinSoloOrganizacion(r *http.Request, sources []domain.Source) []domain.Source {
	if len(sources) > 0 {
		return sources
	}
	if claims, ok := authn.FromContext(r.Context()); !ok || !claims.Personal() {
		return sources
	}
	return slices.DeleteFunc(domain.AllSources(), func(s domain.Source) bool {
		return s == domain.SourceERP || s == domain.SourcePages || s == domain.SourceBases
	})
}

// parseInstant lee un parámetro de fecha en RFC 3339. Devuelve `ok=false` cuando el
// parámetro NO venía (sin acotar), que es distinto de que viniera mal —eso es un error—:
// filtrar por una fecha que el servidor no entendió devolvería resultados fuera del
// rango pedido sin decirlo. Los tres valores evitan el `nil, nil` que hace ambiguo si
// «sin fecha» fue una decisión o un fallo que nadie miró.
func parseInstant(p url.Values, name string) (t time.Time, ok bool, err error) {
	v := strings.TrimSpace(p.Get(name))
	if v == "" {
		return time.Time{}, false, nil
	}
	t, err = time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false, httpx.ErrBadRequest.WithMessage(
			"Parameter " + name + " must be an RFC 3339 date (2026-07-29T10:00:00Z).",
		)
	}
	return t, true, nil
}

// parseSources lee y valida el parámetro CSV `sources` (vacío = todos los
// orígenes registrados). Devuelve un *httpx.Error si algún origen es desconocido.
func parseSources(p url.Values) ([]domain.Source, error) {
	var sources []domain.Source
	for _, s := range splitCSV(p.Get("sources")) {
		src := domain.Source(s)
		if !domain.ValidSource(src) {
			return nil, httpx.ErrBadRequest.WithMessage("Unknown search source: " + s)
		}
		sources = append(sources, src)
	}
	return sources, nil
}

// ---------------------------------------------------------------------------
// Notificaciones
// ---------------------------------------------------------------------------

// ListNotifications: GET /api/v1/notifications?unread=true&before=<rfc3339>&limit=n.
func (h *Handlers) ListNotifications(w http.ResponseWriter, r *http.Request) {
	onlyUnread := r.URL.Query().Get("unread") == "true"
	limit := atoiDefault(r.URL.Query().Get("limit"), 50)

	var before *domain.Antes
	if v := r.URL.Query().Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Parameter 'before' must be RFC3339."))
			return
		}
		before = &domain.Antes{En: t}
		// `before_id`, el id del último de la página: sin él, dos del mismo instante en
		// el corte se perdían.
		if id := r.URL.Query().Get("before_id"); id != "" {
			if before.ID, err = uuid.Parse(id); err != nil {
				httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Parameter 'before_id' must be a UUID."))
				return
			}
		}
	}

	items, err := h.store.ListNotifications(personales(r.Context()), buzones(r), onlyUnread, before, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if t, err := tenancy.From(r.Context()); err == nil {
		for i := range items {
			items[i] = items[i].VistoDesde(t.Slug)
		}
		h.conNombres(r.Context(), items)
	}
	if items == nil {
		items = []domain.Notification{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"notifications": items})
}

// UnreadCount: GET /api/v1/notifications/unread_count.
func (h *Handlers) UnreadCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.store.UnreadCount(personales(r.Context()), buzones(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	out := map[string]any{"unread": count}
	// Y el correo sin leer, que es ESTADO y no aviso: el que ya esperaba antes de
	// que Mail avisara no dejó ninguno. Va aquí y no en cada app porque la campana
	// está en las trece y solo la de Mail puede preguntar a su BFF. Sin Mail, o si
	// no contesta, el campo no sale y la campana sigue con los avisos.
	if h.correo != nil {
		if n, err := h.correo.SinLeer(r.Context(), bearerToken(r)); err == nil {
			out["mail_unread"] = n
		}
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// createNotificationRequest es el cuerpo para emitir una notificación. El
// destinatario (subject) puede diferir del emisor (un servicio notifica a otro
// usuario); requiere el scope notifications:write.
type createNotificationRequest struct {
	Subject string            `json:"subject"`
	Source  string            `json:"source"`
	Type    string            `json:"type"`
	Level   string            `json:"level"`
	Title   string            `json:"title"`
	Body    string            `json:"body"`
	URL     string            `json:"url"`
	Params  map[string]string `json:"params"`
}

// CreateNotification: POST /api/v1/notifications.
func (h *Handlers) CreateNotification(w http.ResponseWriter, r *http.Request) {
	claims, _ := authn.FromContext(r.Context())
	var req createNotificationRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Notificar a otro usuario exige scope explícito; a uno mismo siempre vale.
	target := strings.TrimSpace(req.Subject)
	if target == "" {
		target = claims.Subject
	}
	if target != claims.Subject && !claims.HasScope("notifications:write") {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The notifications:write scope is required to notify other users."))
		return
	}

	n, err := h.notify.Create(r.Context(), domain.NewNotification{
		Subject: target,
		Source:  domain.Source(req.Source),
		Type:    req.Type,
		Level:   domain.NotificationLevel(req.Level),
		Title:   req.Title,
		Body:    req.Body,
		URL:     req.URL,
		Params:  req.Params,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// A OTRA persona, 202 y sin cuerpo, se entregue o no: con el aviso de vuelta —o
	// sin él cuando lo tiene silenciado— quien lo manda sabría qué ha callado esa
	// persona, también en sus otras organizaciones (ADR 0053).
	if target != claims.Subject {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, n)
}

// MarkRead: POST /api/v1/notifications/{id}/read.
func (h *Handlers) MarkRead(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.MarkRead(personales(r.Context()), id, buzones(r)); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// MarkAllRead: POST /api/v1/notifications/read_all.
func (h *Handlers) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	n, err := h.store.MarkAllRead(personales(r.Context()), buzones(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"updated": n})
}

// ---------------------------------------------------------------------------
// Presencia
// ---------------------------------------------------------------------------

// heartbeatRequest es el cuerpo del heartbeat de presencia.
type heartbeatRequest struct {
	State  string `json:"state"`
	Status string `json:"status"`
}

// Heartbeat: PUT /api/v1/presence.
func (h *Handlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var req heartbeatRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.presence.Heartbeat(r.Context(), subFromCtx(r), domain.PresenceState(req.State), req.Status); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// Sugerir al cliente el intervalo de renovación (~TTL/2).
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"ok":                true,
		"heartbeat_seconds": int(h.presence.TTL().Seconds()) / 2,
	})
}

// Offline: DELETE /api/v1/presence.
func (h *Handlers) Offline(w http.ResponseWriter, r *http.Request) {
	if err := h.presence.Offline(r.Context(), subFromCtx(r)); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// QueryPresence: GET /api/v1/presence?subjects=a,b,c.
func (h *Handlers) QueryPresence(w http.ResponseWriter, r *http.Request) {
	subjects := splitCSV(r.URL.Query().Get("subjects"))
	if len(subjects) == 0 {
		// Sin lista explícita, devolver la propia presencia.
		subjects = []string{subFromCtx(r)}
	}
	if len(subjects) > 200 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Too many subjects (200 maximum)."))
		return
	}
	items, err := h.presence.Query(r.Context(), subjects)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if items == nil {
		items = []domain.Presence{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"presence": items})
}

// ---------------------------------------------------------------------------
// Intents cross-app
// ---------------------------------------------------------------------------

// ResolveIntent: POST /api/v1/intents -> devuelve deep-links.
func (h *Handlers) ResolveIntent(w http.ResponseWriter, r *http.Request) {
	var req domain.IntentRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	res, err := h.intents.Resolve(req)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseID extrae y valida un UUID de la ruta.
func parseID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithMessage("Invalid identifier.")
	}
	return id, nil
}

// atoiDefault parsea un entero con default ante error/valor vacío.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// splitCSV divide un valor separado por comas ignorando vacíos.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Preferencias de notificación
// ---------------------------------------------------------------------------

// ListNotificationPrefs: GET /api/v1/notifications/prefs — las excepciones del usuario.
// Devuelve solo lo que ha cambiado: la ausencia de una regla significa «que llegue».
func (h *Handlers) ListNotificationPrefs(w http.ResponseWriter, r *http.Request) {
	prefs, err := h.store.ListNotificationPrefs(r.Context(), subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if prefs == nil {
		prefs = []domain.NotificationPref{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"prefs": prefs})
}

type notificationPrefRequest struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Muted  bool   `json:"muted"`
}

// SetNotificationPref: PUT /api/v1/notifications/prefs — silencia (o vuelve a permitir)
// un origen, o un tipo concreto dentro de él. Siempre sobre el usuario del token: nadie
// configura las notificaciones de otro.
func (h *Handlers) SetNotificationPref(w http.ResponseWriter, r *http.Request) {
	var req notificationPrefRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	src := domain.Source(strings.TrimSpace(req.Source))
	if !domain.ValidNotificationSource(src) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Origen desconocido: "+req.Source))
		return
	}
	pref := domain.NotificationPref{Source: src, Kind: strings.TrimSpace(req.Kind), Muted: req.Muted}
	if err := h.store.SetNotificationPref(r.Context(), subFromCtx(r), pref); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, pref)
}
