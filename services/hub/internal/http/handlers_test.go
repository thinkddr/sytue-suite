// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/connectors"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
	"git.sytue.eu/sytue/one/services/hub/internal/intents"
	"git.sytue.eu/sytue/one/services/hub/internal/interpretar"
	"git.sytue.eu/sytue/one/services/hub/internal/notify"
)

// --- fakes ---

type fakeSearch struct {
	resp domain.SearchResponse
	err  error
	// got es la consulta TAL COMO LLEGÓ al servicio. Comprobar solo el código de
	// respuesta no basta: con el término vacío este fake contesta 200 igual, que
	// es justo cómo se coló el fallo de QUERY.
	got domain.SearchQuery
	// gotRecent son los orígenes con los que se pidió la actividad reciente.
	gotRecent []domain.Source
}

func (f *fakeSearch) Search(_ context.Context, _ string, q domain.SearchQuery) (domain.SearchResponse, error) {
	f.got = q
	f.resp.Query = q.Text
	return f.resp, f.err
}

func (f *fakeSearch) Recent(_ context.Context, _ string, src []domain.Source, _ int) domain.SearchResponse {
	f.gotRecent = src
	return f.resp
}

type fakeNotifier struct {
	created domain.NewNotification
	err     error
	unread  int
}

func (f *fakeNotifier) Create(_ context.Context, rec domain.NewNotification) (domain.Notification, error) {
	f.created = rec
	if f.err != nil {
		return domain.Notification{}, f.err
	}
	return domain.Notification{ID: uuid.New(), Subject: rec.Subject, Title: rec.Title, Source: rec.Source}, nil
}

func (f *fakeNotifier) UnreadCount(_ context.Context, _ []string) (int, error) { return f.unread, nil }

func (f *fakeNotifier) CreateHistorico(context.Context, domain.NewHistorico) (string, error) {
	return domain.HistoricoCreado, nil
}

func (f *fakeNotifier) PublishEvent(context.Context, string, string, string, json.RawMessage) error {
	return f.err
}

type fakeStore struct {
	olvidado string
	items    []domain.Notification
	unread   int
	updated  int64
	markErr  error
	listErr  error
	markedID uuid.UUID
	// Preferencias de notificación: las que devuelve al listar y la última guardada.
	prefs     []domain.NotificationPref
	prefsErr  error
	savedPref *domain.NotificationPref
}

func (f *fakeStore) ListNotifications(_ context.Context, _ []string, _ bool, _ *domain.Antes, _ int) ([]domain.Notification, error) {
	return f.items, f.listErr
}
func (f *fakeStore) UnreadCount(_ context.Context, _ []string) (int, error) { return f.unread, nil }
func (f *fakeStore) MarkRead(_ context.Context, id uuid.UUID, _ []string) error {
	f.markedID = id
	return f.markErr
}

func (f *fakeStore) MarkAllRead(_ context.Context, _ []string) (int64, error) {
	return f.updated, f.markErr
}

func (f *fakeStore) ListNotificationPrefs(_ context.Context, _ string) ([]domain.NotificationPref, error) {
	return f.prefs, f.prefsErr
}

func (f *fakeStore) SetNotificationPref(_ context.Context, _ string, p domain.NotificationPref) error {
	f.savedPref = &p
	return f.prefsErr
}

func (f *fakeStore) OlvidarBuzon(_ context.Context, subject string) (int64, error) {
	f.olvidado = subject
	return 0, nil
}

type fakePresence struct {
	items []domain.Presence
	err   error
	ttl   time.Duration
}

func (f *fakePresence) Heartbeat(_ context.Context, _ string, state domain.PresenceState, _ string) error {
	if state != "" && !domain.ValidState(state) {
		return domain.ErrInvalidInput
	}
	return f.err
}
func (f *fakePresence) Offline(_ context.Context, _ string) error { return f.err }
func (f *fakePresence) Query(_ context.Context, _ []string) ([]domain.Presence, error) {
	return f.items, f.err
}

func (f *fakePresence) TTL() time.Duration {
	if f.ttl == 0 {
		return 45 * time.Second
	}
	return f.ttl
}

func newHandlers(s *fakeSearch, n *fakeNotifier, st *fakeStore, p *fakePresence, hub *notify.Hub) *Handlers {
	return &Handlers{
		search:   s,
		notify:   n,
		store:    st,
		presence: p,
		hub:      hub,
		intents:  intents.NewResolver(intents.Targets{Drive: "https://drive", Mail: "https://mail", Calendar: "https://cal", Chat: "https://chat", Contacts: "https://contacts"}),
	}
}

func request(t *testing.T, method, target string, claims *authn.Claims, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	if claims != nil {
		r = r.WithContext(authn.Into(r.Context(), *claims))
	}
	return r
}

func userClaims() *authn.Claims { return &authn.Claims{Subject: "u", Tenant: "acme"} }

// withURLParam inyecta un path param de chi en el contexto del request.
func withURLParam(r *http.Request, key, val string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

func TestSearch(t *testing.T) {
	h := newHandlers(&fakeSearch{resp: domain.SearchResponse{}}, nil, nil, nil, nil)

	// Source desconocido -> 400.
	rec := httptest.NewRecorder()
	h.Search(rec, request(t, "GET", "/api/v1/search?q=hola&sources=bogus", userClaims(), ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("source inválido: code=%d", rec.Code)
	}

	// Feliz -> 200.
	rec = httptest.NewRecorder()
	h.Search(rec, request(t, "GET", "/api/v1/search?q=hola&sources=drive,mail&limit=5", userClaims(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("search: code=%d", rec.Code)
	}

	// Error de dominio (entrada inválida) -> 400.
	h = newHandlers(&fakeSearch{err: domain.ErrInvalidInput}, nil, nil, nil, nil)
	rec = httptest.NewRecorder()
	h.Search(rec, request(t, "GET", "/api/v1/search?q=", userClaims(), ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("search vacío: code=%d", rec.Code)
	}
}

// Una cuenta personal no tiene SyERP ni Pages: por defecto no se les pregunta (su 403 se
// contaba como «no ha respondido»); una organización los conserva, y pedirlos por su
// nombre se respeta.
func TestCuentaPersonalNoBuscaEnProductosDeOrganizacion(t *testing.T) {
	personal := &authn.Claims{Subject: "u", Tenant: "yo", OrgKind: authn.OrgKindPersonal}
	tiene := slices.Contains[[]domain.Source]

	fs := &fakeSearch{}
	h := newHandlers(fs, nil, nil, nil, nil)
	h.Search(httptest.NewRecorder(), request(t, "GET", "/api/v1/search?q=hola", personal, ""))
	if len(fs.got.Sources) == 0 || tiene(fs.got.Sources, domain.SourcePages) || tiene(fs.got.Sources, domain.SourceERP) || tiene(fs.got.Sources, domain.SourceBases) {
		t.Errorf("personal: orígenes por defecto = %v", fs.got.Sources)
	}
	if !tiene(fs.got.Sources, domain.SourceMail) {
		t.Errorf("personal: debe seguir buscando en Mail: %v", fs.got.Sources)
	}

	fs = &fakeSearch{}
	h = newHandlers(fs, nil, nil, nil, nil)
	h.Search(httptest.NewRecorder(), request(t, "GET", "/api/v1/search?q=hola", userClaims(), ""))
	if len(fs.got.Sources) != 0 {
		t.Errorf("organización: sin filtrar (vacío = todos), got %v", fs.got.Sources)
	}

	fs = &fakeSearch{}
	h = newHandlers(fs, nil, nil, nil, nil)
	h.Search(httptest.NewRecorder(), request(t, "GET", "/api/v1/search?q=hola&sources=pages", personal, ""))
	if len(fs.got.Sources) != 1 || fs.got.Sources[0] != domain.SourcePages {
		t.Errorf("personal pidiendo pages: got %v", fs.got.Sources)
	}

	fs = &fakeSearch{}
	h = newHandlers(fs, nil, nil, nil, nil)
	h.Recent(httptest.NewRecorder(), request(t, "GET", "/api/v1/recent", personal, ""))
	if len(fs.gotRecent) == 0 || tiene(fs.gotRecent, domain.SourcePages) || tiene(fs.gotRecent, domain.SourceERP) {
		t.Errorf("personal, recientes: %v", fs.gotRecent)
	}
}

func TestRecent(t *testing.T) {
	resp := domain.SearchResponse{Hits: []domain.SearchHit{
		{Source: domain.SourceDrive, ID: "f1", Title: "Informe"},
	}}
	h := newHandlers(&fakeSearch{resp: resp}, nil, nil, nil, nil)

	// Feliz -> 200 con los hits agregados.
	rec := httptest.NewRecorder()
	h.Recent(rec, request(t, "GET", "/api/v1/recent?limit=6", userClaims(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("recent: code=%d", rec.Code)
	}
	var body domain.SearchResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Hits) != 1 || body.Hits[0].ID != "f1" {
		t.Errorf("hits inesperados: %+v", body.Hits)
	}

	// Source desconocido -> 400 (misma validación que la búsqueda).
	rec = httptest.NewRecorder()
	h.Recent(rec, request(t, "GET", "/api/v1/recent?sources=bogus", userClaims(), ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("source inválido: code=%d", rec.Code)
	}
}

func TestListNotifications(t *testing.T) {
	st := &fakeStore{items: []domain.Notification{{ID: uuid.New(), Title: "hola"}}}
	h := newHandlers(nil, nil, st, nil, nil)

	// before inválido -> 400.
	rec := httptest.NewRecorder()
	h.ListNotifications(rec, request(t, "GET", "/api/v1/notifications?before=ayer", userClaims(), ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("before inválido: code=%d", rec.Code)
	}

	// Feliz -> 200 con clave notifications.
	rec = httptest.NewRecorder()
	h.ListNotifications(rec, request(t, "GET", "/api/v1/notifications?unread=true&limit=10&before=2026-01-01T00:00:00Z", userClaims(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: code=%d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, ok := body["notifications"]; !ok {
		t.Error("falta clave notifications")
	}
}

func TestUnreadCount(t *testing.T) {
	h := newHandlers(nil, nil, &fakeStore{unread: 7}, nil, nil)
	rec := httptest.NewRecorder()
	h.UnreadCount(rec, request(t, "GET", "/api/v1/notifications/unread_count", userClaims(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("unread: code=%d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["unread"] != float64(7) {
		t.Errorf("unread = %v", body["unread"])
	}
}

func TestCreateNotification(t *testing.T) {
	n := &fakeNotifier{}
	h := newHandlers(nil, n, nil, nil, nil)

	// JSON inválido -> 400.
	rec := httptest.NewRecorder()
	h.CreateNotification(rec, request(t, "POST", "/api/v1/notifications", userClaims(), `{bad`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("json inválido: code=%d", rec.Code)
	}

	// A uno mismo -> 201, sin exigir scope.
	rec = httptest.NewRecorder()
	h.CreateNotification(rec, request(t, "POST", "/api/v1/notifications", userClaims(), `{"source":"drive","type":"t","title":"Hola"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("propia: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if n.created.Subject != "u" {
		t.Errorf("subject por defecto = %q", n.created.Subject)
	}

	// A otro usuario sin scope -> 403.
	rec = httptest.NewRecorder()
	h.CreateNotification(rec, request(t, "POST", "/api/v1/notifications", userClaims(), `{"subject":"otro","source":"drive","type":"t","title":"Hola"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a otro sin scope: code=%d", rec.Code)
	}

	// A otro usuario CON scope -> 202 y SIN cuerpo: el aviso de vuelta diría si esa
	// persona lo tiene silenciado (ADR 0053).
	rec = httptest.NewRecorder()
	claims := &authn.Claims{Subject: "u", Tenant: "acme", Scopes: []string{"notifications:write"}}
	h.CreateNotification(rec, request(t, "POST", "/api/v1/notifications", claims, `{"subject":"otro","source":"drive","type":"t","title":"Hola"}`))
	if rec.Code != http.StatusAccepted || rec.Body.Len() != 0 {
		t.Fatalf("a otro con scope: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if n.created.Subject != "otro" {
		t.Errorf("subject destino = %q", n.created.Subject)
	}

	// Notificación inválida (source desconocido) -> 400.
	n2 := &fakeNotifier{err: domain.ErrUnknownSource}
	h = newHandlers(nil, n2, nil, nil, nil)
	rec = httptest.NewRecorder()
	h.CreateNotification(rec, request(t, "POST", "/api/v1/notifications", userClaims(), `{"source":"nope","type":"t","title":"X"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("source desconocido: code=%d", rec.Code)
	}
}

func TestMarkReadAndAll(t *testing.T) {
	st := &fakeStore{updated: 3}
	h := newHandlers(nil, nil, st, nil, nil)

	// MarkRead con id inválido -> 400.
	rec := httptest.NewRecorder()
	r := request(t, "POST", "/api/v1/notifications/xxx/read", userClaims(), "")
	r = withURLParam(r, "id", "not-a-uuid")
	h.MarkRead(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("id inválido: code=%d", rec.Code)
	}

	// MarkRead feliz -> 204.
	id := uuid.New()
	rec = httptest.NewRecorder()
	r = request(t, "POST", "/api/v1/notifications/"+id.String()+"/read", userClaims(), "")
	r = withURLParam(r, "id", id.String())
	h.MarkRead(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("markRead: code=%d", rec.Code)
	}

	// MarkAllRead -> 200 con updated.
	rec = httptest.NewRecorder()
	h.MarkAllRead(rec, request(t, "POST", "/api/v1/notifications/read_all", userClaims(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("markAll: code=%d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["updated"] != float64(3) {
		t.Errorf("updated = %v", body["updated"])
	}
}

func TestPresenceHandlers(t *testing.T) {
	p := &fakePresence{items: []domain.Presence{{Subject: "u", State: domain.StateOnline}}}
	h := newHandlers(nil, nil, nil, p, nil)

	// Heartbeat estado inválido -> 400 (el fake devuelve ErrInvalidInput).
	rec := httptest.NewRecorder()
	h.Heartbeat(rec, request(t, "PUT", "/api/v1/presence", userClaims(), `{"state":"teleporting"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("estado inválido: code=%d", rec.Code)
	}

	// Heartbeat feliz -> 200 con heartbeat_seconds.
	rec = httptest.NewRecorder()
	h.Heartbeat(rec, request(t, "PUT", "/api/v1/presence", userClaims(), `{"state":"online","status":"aquí"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("heartbeat: code=%d", rec.Code)
	}

	// Offline -> 204.
	rec = httptest.NewRecorder()
	h.Offline(rec, request(t, "DELETE", "/api/v1/presence", userClaims(), ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("offline: code=%d", rec.Code)
	}

	// QueryPresence con lista -> 200.
	rec = httptest.NewRecorder()
	h.QueryPresence(rec, request(t, "GET", "/api/v1/presence?subjects=a,b,c", userClaims(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("query: code=%d", rec.Code)
	}

	// QueryPresence con demasiados subjects -> 400.
	rec = httptest.NewRecorder()
	many := make([]string, 201)
	for i := range many {
		many[i] = "s"
	}
	h.QueryPresence(rec, request(t, "GET", "/api/v1/presence?subjects="+strings.Join(many, ","), userClaims(), ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("demasiados subjects: code=%d", rec.Code)
	}
}

func TestResolveIntent(t *testing.T) {
	h := newHandlers(nil, nil, nil, nil, nil)

	// JSON inválido -> 400.
	rec := httptest.NewRecorder()
	h.ResolveIntent(rec, request(t, "POST", "/api/v1/intents", userClaims(), `{bad`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("json inválido: code=%d", rec.Code)
	}

	// Intent desconocido -> 400.
	rec = httptest.NewRecorder()
	h.ResolveIntent(rec, request(t, "POST", "/api/v1/intents", userClaims(), `{"type":"nope"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("intent desconocido: code=%d", rec.Code)
	}

	// Intent válido (attach-drive-to-mail) -> 200 con URL.
	rec = httptest.NewRecorder()
	h.ResolveIntent(rec, request(t, "POST", "/api/v1/intents", userClaims(),
		`{"type":"attach-drive-to-mail","ids":["f1"],"params":{"to":"a@b.com"}}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("intent válido: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var res domain.IntentResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.URL == "" || res.Target != domain.SourceMail {
		t.Errorf("resultado inesperado: %+v", res)
	}
}

func TestStreamRequiresTenant(t *testing.T) {
	hub := notify.NewHub(4)
	h := newHandlers(nil, &fakeNotifier{unread: 2}, nil, nil, hub)

	// Claims sin tenant en el contexto -> 403 (falta tenant), antes del bucle SSE.
	rec := httptest.NewRecorder()
	h.Stream(rec, request(t, "GET", "/api/v1/notifications/stream", &authn.Claims{Subject: "u"}, ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("stream sin tenant debe dar 403, code=%d", rec.Code)
	}
}

func TestStreamHandshakeAndClose(t *testing.T) {
	hub := notify.NewHub(4)
	h := newHandlers(nil, &fakeNotifier{unread: 2}, nil, nil, hub)

	// El httptest.ResponseRecorder implementa http.Flusher, así que el handler
	// entra en el bucle SSE. Cancelamos el contexto (simula desconexión del
	// cliente) para que Stream retorne tras emitir el handshake "ready".
	ctx, cancel := context.WithCancel(context.Background())
	claims := &authn.Claims{Subject: "u", Tenant: "acme"}
	r := request(t, "GET", "/api/v1/notifications/stream", claims, "")
	r = r.WithContext(tenancy.Into(authn.Into(ctx, *claims), tenancy.Tenant{Slug: "acme"}))
	cancel() // cliente desconectado antes de arrancar el bucle

	rec := httptest.NewRecorder()
	h.Stream(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("stream: code=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("content-type = %q", ct)
	}
	// El handshake debe incluir el contador inicial de no leídas.
	if !strings.Contains(rec.Body.String(), "event: ready") || !strings.Contains(rec.Body.String(), `"unread":2`) {
		t.Errorf("falta handshake ready con unread: %q", rec.Body.String())
	}
	if hub.SubscriberCount() != 0 {
		t.Errorf("la suscripción SSE debe cerrarse al salir, quedan %d", hub.SubscriberCount())
	}
}

// El conmutador de aplicaciones pregunta al Hub qué levanta esta instalación.
// Lo que se guarda aquí es la degradación: sin lista configurada NO se esconde
// media suite, se presenta entera, que es como se ha comportado siempre.
func TestApps(t *testing.T) {
	t.Parallel()

	t.Run("devuelve lo que declara el despliegue", func(t *testing.T) {
		h := newHandlers(nil, nil, nil, nil, nil)
		h.cfg.AppsDesplegadas = []string{"one", "mail", "docs"}
		rr := httptest.NewRecorder()
		h.Apps(rr, request(t, http.MethodGet, "/api/v1/apps", userClaims(), ""))
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d", rr.Code)
		}
		var cuerpo struct {
			Apps []string `json:"apps"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &cuerpo); err != nil {
			t.Fatalf("cuerpo ilegible: %v", err)
		}
		if len(cuerpo.Apps) != 3 || cuerpo.Apps[0] != "one" {
			t.Fatalf("apps = %v, quiero [one mail docs]", cuerpo.Apps)
		}
	})

	t.Run("sin configurar contesta lista vacía, no nula", func(t *testing.T) {
		// `null` en JSON obligaría a cada cliente a distinguirlo de `[]`, y el
		// que no lo hiciera escondería la suite entera. Con `[]` el contrato es
		// uno solo: vacío = no se sabe = preséntalo todo.
		h := newHandlers(nil, nil, nil, nil, nil)
		rr := httptest.NewRecorder()
		h.Apps(rr, request(t, http.MethodGet, "/api/v1/apps", userClaims(), ""))
		if cuerpo := strings.TrimSpace(rr.Body.String()); cuerpo != `{"apps":[]}` {
			t.Fatalf("cuerpo = %s, quiero {\"apps\":[]}", cuerpo)
		}
	})
}

// --- búsqueda con IA ---

// servidorIA levanta un servicio de IA de mentira que contesta lo que se le diga.
func servidorIA(t *testing.T, texto string, codigo int) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if codigo != http.StatusOK {
			w.WriteHeader(codigo)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":` + strconv.Quote(texto) + `}`))
	}))
	t.Cleanup(s.Close)
	return s
}

// El caso que justifica la función: la frase se convierte en filtros.
// EL BUSCADOR DE LA SUITE ENTERA SE ROMPIÓ POR NO TENER ESTE TEST.
//
// `/search` está montada con `httpx.GetYQuery`, así que atiende GET y QUERY. Por
// QUERY el término viaja en el CUERPO y la query string llega VACÍA; el handler
// lo leía de `r.URL.Query()`, así que `q` salía siempre vacío y el servicio
// devolvía `ErrInvalidInput`. Resultado en producción (2026-09-14): un 400
// `bad_request` en cada pulsación y «No se pudo completar la búsqueda» en las
// once apps. El cliente tampoco lo salvaba: solo cae a GET ante 404, 405 o 501.
//
// Los tests que había cubrían solo GET, que es exactamente por lo que no saltó
// nada. Este manda un QUERY de verdad —cuerpo urlencoded, sin `?` en la URL— y
// comprueba que el término LLEGA al servicio, no solo que responda 200: con `q`
// vacío el fake también contestaría 200 y el test pasaría sin cazar nada.
func TestSearchPorQueryLeeElCuerpo(t *testing.T) {
	fs := &fakeSearch{resp: domain.SearchResponse{}}
	h := newHandlers(fs, nil, nil, nil, nil)

	rec := httptest.NewRecorder()
	r := request(t, httpx.MethodQuery, "/api/v1/search", userClaims(),
		"q=factura+Acme&sources=drive,mail&limit=5")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.Search(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("QUERY: code=%d, cuerpo=%s", rec.Code, rec.Body.String())
	}
	if fs.got.Text != "factura Acme" {
		t.Fatalf("el término no llegó al servicio: %q", fs.got.Text)
	}
	if fs.got.Limit != 5 {
		t.Fatalf("limit no llegó: %d", fs.got.Limit)
	}
	if len(fs.got.Sources) != 2 {
		t.Fatalf("sources no llegaron: %v", fs.got.Sources)
	}
}

func TestSearch_IAInterpretaLaFrase(t *testing.T) {
	ia := servidorIA(t, `{"texto":"factura Acme","fuentes":["drive"],"tipos":["file"],"desde":"2026-08-01T00:00:00Z"}`, http.StatusOK)
	fs := &fakeSearch{resp: domain.SearchResponse{}}
	h := newHandlers(fs, nil, nil, nil, nil)
	h.interprete = interpretar.NewCliente(ia.URL, 5*time.Second)

	rec := httptest.NewRecorder()
	h.Search(rec, request(t, "GET", "/api/v1/search?ia=1&q=facturas+de+Acme+del+mes+pasado+en+Drive", userClaims(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	var resp domain.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Interpreted == nil {
		t.Fatal("la respuesta debe decir qué entendió, o la interfaz no puede enseñarlo")
	}
	if resp.Interpreted.Text != "factura Acme" {
		t.Fatalf("texto: %q", resp.Interpreted.Text)
	}
	if len(resp.Interpreted.Sources) != 1 || resp.Interpreted.Sources[0] != domain.SourceDrive {
		t.Fatalf("fuentes: %v", resp.Interpreted.Sources)
	}
	if resp.Interpreted.From == nil {
		t.Fatal("el rango debería haberse aplicado")
	}
}

// LA REGLA DE LA QUE CUELGA TODO: si la IA falla, se busca literal igual que
// siempre. Interpretar es una mejora, no un requisito.
func TestSearch_IACaidaNoRompeLaBusqueda(t *testing.T) {
	ia := servidorIA(t, "", http.StatusInternalServerError)
	fs := &fakeSearch{resp: domain.SearchResponse{}}
	h := newHandlers(fs, nil, nil, nil, nil)
	h.interprete = interpretar.NewCliente(ia.URL, 5*time.Second)

	rec := httptest.NewRecorder()
	h.Search(rec, request(t, "GET", "/api/v1/search?ia=1&q=facturas+de+Acme+del+mes+pasado", userClaims(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("una IA caída no puede tumbar la búsqueda: code=%d", rec.Code)
	}
	var resp domain.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Interpreted != nil {
		t.Fatal("sin interpretación no se anuncia ninguna")
	}
	if resp.Query != "facturas de Acme del mes pasado" {
		t.Fatalf("se debería haber buscado lo literal: %q", resp.Query)
	}
}

// Sin servicio configurado, la búsqueda es la de siempre y no se pierde tiempo.
func TestSearch_SinServicioDeIA(t *testing.T) {
	fs := &fakeSearch{resp: domain.SearchResponse{}}
	h := newHandlers(fs, nil, nil, nil, nil)
	h.interprete = interpretar.NewCliente("", time.Second)

	rec := httptest.NewRecorder()
	h.Search(rec, request(t, "GET", "/api/v1/search?ia=1&q=facturas+de+Acme+del+mes+pasado", userClaims(), ""))
	if rec.Code != http.StatusOK || fs.resp.Query != "facturas de Acme del mes pasado" {
		t.Fatalf("code=%d query=%q", rec.Code, fs.resp.Query)
	}
}

// Lo que la persona marcó A MANO manda: quien ha elegido «solo Mail» no quiere
// que una frase se lo cambie por debajo.
func TestSearch_ElFiltroDelUsuarioManda(t *testing.T) {
	ia := servidorIA(t, `{"texto":"acta","fuentes":["drive"]}`, http.StatusOK)
	fs := &fakeSearch{resp: domain.SearchResponse{}}
	h := newHandlers(fs, nil, nil, nil, nil)
	h.interprete = interpretar.NewCliente(ia.URL, 5*time.Second)

	rec := httptest.NewRecorder()
	h.Search(rec, request(t, "GET", "/api/v1/search?ia=1&sources=mail&q=el+acta+de+la+reunion+pasada", userClaims(), ""))
	var resp domain.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Interpreted == nil || len(resp.Interpreted.Sources) != 1 || resp.Interpreted.Sources[0] != domain.SourceMail {
		t.Fatalf("el origen elegido a mano no debe pisarse: %+v", resp.Interpreted)
	}
}

// Una palabra suelta no se le manda al modelo: no hay nada que interpretar y
// sería pagar por cada tecla.
func TestSearch_UnaPalabraNoLlamaAlModelo(t *testing.T) {
	var llamadas int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		llamadas++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"{}"}`))
	}))
	defer s.Close()
	h := newHandlers(&fakeSearch{resp: domain.SearchResponse{}}, nil, nil, nil, nil)
	h.interprete = interpretar.NewCliente(s.URL, 5*time.Second)

	h.Search(httptest.NewRecorder(), request(t, "GET", "/api/v1/search?ia=1&q=factura", userClaims(), ""))
	if llamadas != 0 {
		t.Fatalf("no debería haber llamado al modelo, llamadas=%d", llamadas)
	}
}

// Una frase que es TODO filtro deja el término vacío, y la búsqueda federada
// exige término: se descarta la interpretación en vez de devolver un 400.
func TestSearch_SinTerminoSeDescartaLaInterpretacion(t *testing.T) {
	ia := servidorIA(t, `{"texto":"","fuentes":["mail"],"desde":"2026-09-01T00:00:00Z"}`, http.StatusOK)
	fs := &fakeSearch{resp: domain.SearchResponse{}}
	h := newHandlers(fs, nil, nil, nil, nil)
	h.interprete = interpretar.NewCliente(ia.URL, 5*time.Second)

	rec := httptest.NewRecorder()
	h.Search(rec, request(t, "GET", "/api/v1/search?ia=1&q=los+correos+de+la+semana+pasada", userClaims(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	var resp domain.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Interpreted != nil {
		t.Fatalf("no se anuncia una interpretación que no se ha aplicado: %+v", resp.Interpreted)
	}
	if resp.Query != "los correos de la semana pasada" {
		t.Fatalf("debería haberse buscado lo literal: %q", resp.Query)
	}
}

func TestInternalNotification_AuthAndCreate(t *testing.T) {
	n := &fakeNotifier{}
	h := newHandlers(nil, n, nil, nil, nil)
	h.cfg.InternalSecret = "test-internal-secret"

	// 1. Sin secret -> 401
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/notifications", strings.NewReader(`{"tenant":"acme","subject":"user1","title":"Test"}`))
	req.Header.Set("Content-Type", "application/json")
	h.internalAuth(http.HandlerFunc(h.CreateInternalNotification)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401 sin secret, obtuve %d", rec.Code)
	}

	// 2. Secret inválido -> 401
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/internal/notifications", strings.NewReader(`{"tenant":"acme","subject":"user1","title":"Test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-secret")
	h.internalAuth(http.HandlerFunc(h.CreateInternalNotification)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperaba 401 con secret erróneo, obtuve %d", rec.Code)
	}

	// 3. Secret válido -> 201 Created y notificación creada
	rec = httptest.NewRecorder()
	payload := `{"tenant":"acme","subject":"user1","source":"rooms","type":"rooms.mention","title":"Ana te mencionó","body":"hola @user1","url":"https://rooms.sytue.eu/channel/123"}`
	req = httptest.NewRequest(http.MethodPost, "/internal/notifications", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-internal-secret")
	h.internalAuth(http.HandlerFunc(h.CreateInternalNotification)).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("esperaba 201 Created, obtuve %d: %s", rec.Code, rec.Body.String())
	}
	if n.created.Subject != "user1" || n.created.Title != "Ana te mencionó" || n.created.Source != domain.SourceRooms {
		t.Fatalf("notificación creada no coincide con el payload: %+v", n.created)
	}
}

// Lo que llega por el Ingress no entra en /internal, ni con el secreto bueno. Era
// alcanzable desde internet con el secreto de desarrollo, que es público.
func TestInternalNotification_NoSeAlcanzaDesdeElIngress(t *testing.T) {
	n := &fakeNotifier{}
	h := newHandlers(nil, n, nil, nil, nil)
	h.cfg.InternalSecret = "test-internal-secret"
	payload := `{"tenant":"acme","subject":"user1","source":"rooms","type":"rooms.mention","title":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/notifications", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-internal-secret")
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	rec := httptest.NewRecorder()
	h.internalAuth(http.HandlerFunc(h.CreateInternalNotification)).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound || n.created.Subject != "" {
		t.Fatalf("desde el Ingress: code=%d creada=%+v", rec.Code, n.created)
	}
}

// Un token DELEGADO (`act`) no abre lo que es de la persona en todas sus
// organizaciones (ADR 0053); uno propio, sí, con sus direcciones de todas.
func TestDelegadoNoAbreLosAvisosDeLaPersona(t *testing.T) {
	propio := authn.Claims{Subject: "bea", Mailbox: "bea@globex.test", Mailboxes: []string{"bea@acme.test"}}
	if b := buzonesPersonalesDel(propio); len(b) != 3 {
		t.Fatalf("propio: %v", b)
	}
	delegado := propio
	delegado.Delegado, delegado.Actor = true, "app-x"
	if b := buzonesPersonalesDel(delegado); len(b) != 0 {
		t.Fatalf("delegado: %v", b)
	}
}

// El contador de la campana trae además el correo sin leer de Mail —con el token de
// quien pregunta y sin Spam, Papelera, Enviados ni Borradores—, y si Mail no contesta
// el campo no sale y los avisos siguen contando.
func TestUnreadCountTraeElCorreoSinLeer(t *testing.T) {
	mail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/mailboxes" || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"mailboxes":[{"role":"inbox","unread_emails":3},{"role":"","unread_emails":2},` +
			`{"role":"archive","unread_emails":1},{"role":"junk","unread_emails":9},{"role":"trash","unread_emails":9},` +
			`{"role":"sent","unread_emails":9},{"role":"drafts","unread_emails":9}]}`))
	}))
	defer mail.Close()

	pedir := func(base string) map[string]any {
		h := newHandlers(nil, nil, &fakeStore{unread: 1}, nil, nil)
		h.correo = connectors.NewMailConnector(base, "https://mail", time.Second)
		req := request(t, "GET", "/api/v1/notifications/unread_count", userClaims(), "")
		req.Header.Set("Authorization", "Bearer tok")
		rec := httptest.NewRecorder()
		h.UnreadCount(rec, req)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return body
	}
	if b := pedir(mail.URL); b["unread"] != float64(1) || b["mail_unread"] != float64(6) {
		t.Fatalf("contador %v, quiero unread=1 y mail_unread=6", b)
	}
	mail.Close()
	if b := pedir(mail.URL); b["unread"] != float64(1) || b["mail_unread"] != nil {
		t.Fatalf("con Mail caído: %v, quiero unread=1 y sin mail_unread", b)
	}
}

// fakeNombres responde con nombres fijos y apunta por qué slugs le preguntaron.
type fakeNombres struct {
	nombres map[string]string
	pedidos [][]string
}

func (f *fakeNombres) Nombres(_ context.Context, slugs []string) map[string]string {
	f.pedidos = append(f.pedidos, slugs)
	out := map[string]string{}
	for _, s := range slugs {
		if n, ok := f.nombres[s]; ok {
			out[s] = n
		}
	}
	return out
}

// Un aviso de otra organización sale con su nombre visible, y solo se pregunta por
// las organizaciones de los avisos de la persona (decisión del dueño, 17/10/2026).
func TestAvisosDeFueraConElNombreDeSuOrganizacion(t *testing.T) {
	st := &fakeStore{items: []domain.Notification{
		{ID: uuid.New(), Tenant: "acme", Title: "propio"},
		{ID: uuid.New(), Tenant: "otra", Title: "de fuera"},
		{ID: uuid.New(), Tenant: "otra", Title: "de fuera, otra vez"},
		{ID: uuid.New(), Tenant: "sin-nombre", Title: "de una que Sytue ID no nombra"},
	}}
	h := newHandlers(nil, nil, st, nil, nil)
	nombres := &fakeNombres{nombres: map[string]string{"otra": "Acme Ingeniería S.L.", "acme": "La propia"}}
	h.orgs = nombres
	r := request(t, "GET", "/api/v1/notifications", userClaims(), "")
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
	rec := httptest.NewRecorder()
	h.ListNotifications(rec, r)
	var body struct {
		Notifications []domain.Notification `json:"notifications"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Notifications) != 4 {
		t.Fatalf("lista: %d %s", rec.Code, rec.Body)
	}
	got := map[string][2]string{}
	for _, n := range body.Notifications {
		got[n.Title] = [2]string{n.Organization, n.OrganizationName}
	}
	if got["propio"] != [2]string{} || got["de fuera"] != [2]string{"otra", "Acme Ingeniería S.L."} ||
		got["de fuera, otra vez"] != got["de fuera"] || got["de una que Sytue ID no nombra"] != [2]string{"sin-nombre", ""} {
		t.Fatalf("organizaciones: %v", got)
	}
	if len(nombres.pedidos) != 1 || !slices.Equal(nombres.pedidos[0], []string{"otra", "sin-nombre"}) {
		t.Fatalf("se pregunta solo por las de fuera de sus avisos, una vez cada una: %v", nombres.pedidos)
	}
}
