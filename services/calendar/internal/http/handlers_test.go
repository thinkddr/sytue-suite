// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// --- Fake del servicio -------------------------------------------------------

// fakeService implementa calendarService con salidas y errores programables por
// método. Registra la última entrada relevante para aserciones.
type fakeService struct {
	objects []domain.CalendarObject

	ensureCal domain.Calendar
	ensureErr error

	calendars []domain.Calendar
	listErr   error

	cal    domain.Calendar
	calErr error

	createdCal domain.Calendar
	createErr  error
	gotNewCal  store.NewCalendar

	updatedCal domain.Calendar
	updateErr  error

	deleteErr     error
	gotDeleteID   uuid.UUID
	gotDeleteSend bool

	subCal      domain.Calendar
	subErr      error
	gotSubURL   string
	gotSubName  string
	gotSubColor string

	refreshedCal domain.Calendar
	refreshErr   error
	gotRefreshID uuid.UUID

	event      domain.Event
	eventErr   error
	gotEventIn service.EventInput
	reminders  []int

	instances []domain.Instance
	instErr   error
	gotFrom   time.Time
	gotTo     time.Time

	rangeEvents []domain.Event
	rangeErr    error
	gotRangeCal uuid.UUID

	searchEvents []domain.Event
	searchErr    error
	gotSearch    store.EventSearchQuery

	attendee   domain.Attendee
	partErr    error
	gotPartEml string

	periods []domain.FreeBusyPeriod
	fbErr   error
	fbUser  string

	slots       []domain.FreeSlot
	slotsErr    error
	gotSlotUser []string
	gotSlotDur  time.Duration
	gotSlotWH   domain.WorkingHours

	inbox    []domain.Invitation
	inboxErr error
	gotPend  bool

	inv       domain.Invitation
	respErr   error
	gotStatus domain.InvitationStatus

	dueReminders []store.DueReminder
	dueErr       error
	gotDueSub    string
	gotDueBefore *store.ReminderCursor

	gotIMIP string
	imipErr error

	dismissErr    error
	gotDismissID  uuid.UUID
	gotDismissSub string

	amendObj  domain.CalendarObject
	amendErr  error
	gotAmend  string // "<uid>|<mode>"
	gotAmendR time.Time

	// Compartir calendarios con ACL.
	share                      domain.CalendarShare
	shareErr                   error
	gotShareEml                string
	gotSharePrm                domain.SharePermission
	gotShareOwn                string // ownerSub
	shares                     []domain.CalendarShare
	sharesErr                  error
	revokeErr                  error
	gotRevoke                  uuid.UUID
	sharedCals                 []store.SharedCalendar
	sharedErr                  error
	gotSharedSub, gotSharedEml string

	// Recursos reservables.
	resources    []store.ResourceAvailability
	resourcesErr error
	resource     domain.Resource
	resourceErr  error
	gotNewRes    store.NewResource
	gotResPatch  store.ResourcePatch
	gotResAdmin  bool
	gotResSub    string
	booking      store.BookingResult
	bookingErr   error
	gotBookSpec  service.BookingSpec
	gotBookResID uuid.UUID
	releaseErr   error
	gotRelUID    string
	gotRelAdmin  bool
}

func (f *fakeService) ListResources(_ context.Context, from, to time.Time, _ bool) ([]store.ResourceAvailability, error) {
	f.gotFrom, f.gotTo = from, to
	return f.resources, f.resourcesErr
}

func (f *fakeService) GetResource(context.Context, uuid.UUID) (domain.Resource, error) {
	return f.resource, f.resourceErr
}

func (f *fakeService) CreateResource(_ context.Context, nr store.NewResource) (domain.Resource, error) {
	f.gotNewRes = nr
	return f.resource, f.resourceErr
}

func (f *fakeService) UpdateResource(_ context.Context, _ uuid.UUID, sub string, isAdmin bool, p store.ResourcePatch) (domain.Resource, error) {
	f.gotResSub, f.gotResAdmin, f.gotResPatch = sub, isAdmin, p
	return f.resource, f.resourceErr
}

func (f *fakeService) DeleteResource(_ context.Context, _ uuid.UUID, sub string, isAdmin bool) error {
	f.gotResSub, f.gotResAdmin = sub, isAdmin
	return f.resourceErr
}

func (f *fakeService) BookResource(_ context.Context, resourceID uuid.UUID, spec service.BookingSpec) (store.BookingResult, error) {
	f.gotBookResID, f.gotBookSpec = resourceID, spec
	return f.booking, f.bookingErr
}

func (f *fakeService) ReleaseResource(_ context.Context, sub, _ string, isAdmin bool, _ uuid.UUID, sourceUID string) error {
	f.gotResSub, f.gotRelAdmin, f.gotRelUID = sub, isAdmin, sourceUID
	return f.releaseErr
}

func (f *fakeService) ShareCalendar(_ context.Context, _ uuid.UUID, ownerSub, _, email string, perm domain.SharePermission) (domain.CalendarShare, error) {
	f.gotShareOwn, f.gotShareEml, f.gotSharePrm = ownerSub, email, perm
	return f.share, f.shareErr
}

func (f *fakeService) ListCalendarShares(_ context.Context, _ uuid.UUID, ownerSub string) ([]domain.CalendarShare, error) {
	f.gotShareOwn = ownerSub
	return f.shares, f.sharesErr
}

func (f *fakeService) RevokeCalendarShare(_ context.Context, _, shareID uuid.UUID, ownerSub string) error {
	f.gotRevoke, f.gotShareOwn = shareID, ownerSub
	return f.revokeErr
}

func (f *fakeService) SharedWithMe(_ context.Context, sub, email string) ([]store.SharedCalendar, error) {
	f.gotSharedSub, f.gotSharedEml = sub, email
	return f.sharedCals, f.sharedErr
}

func (f *fakeService) EnsureDefaultCalendar(context.Context, string) (domain.Calendar, error) {
	return f.ensureCal, f.ensureErr
}

func (f *fakeService) ListCalendars(context.Context, string) ([]domain.Calendar, error) {
	return f.calendars, f.listErr
}

func (f *fakeService) GetCalendar(context.Context, uuid.UUID) (domain.Calendar, error) {
	return f.cal, f.calErr
}

func (f *fakeService) CreateCalendar(_ context.Context, nc store.NewCalendar) (domain.Calendar, error) {
	f.gotNewCal = nc
	return f.createdCal, f.createErr
}

func (f *fakeService) UpdateCalendar(context.Context, uuid.UUID, store.CalendarPatch) (domain.Calendar, error) {
	return f.updatedCal, f.updateErr
}
func (f *fakeService) DeleteCalendar(context.Context, uuid.UUID) error { return f.deleteErr }

func (f *fakeService) SubscribeICS(_ context.Context, _, url, displayName, color string) (domain.Calendar, error) {
	f.gotSubURL, f.gotSubName, f.gotSubColor = url, displayName, color
	return f.subCal, f.subErr
}

func (f *fakeService) RefreshSubscription(_ context.Context, id uuid.UUID) (domain.Calendar, error) {
	f.gotRefreshID = id
	return f.refreshedCal, f.refreshErr
}

func (f *fakeService) CreateEvent(_ context.Context, in service.EventInput) (domain.Event, error) {
	f.gotEventIn = in
	return f.event, f.eventErr
}

func (f *fakeService) GetEvent(context.Context, uuid.UUID) (domain.Event, error) {
	return f.event, f.eventErr
}

func (f *fakeService) EventReminders(context.Context, domain.Event) ([]int, error) {
	return f.reminders, nil
}

func (f *fakeService) UpdateEvent(_ context.Context, in service.EventInput) (domain.Event, error) {
	f.gotEventIn = in
	return f.event, f.eventErr
}
func (f *fakeService) DeleteEvent(context.Context, uuid.UUID) error { return f.deleteErr }
func (f *fakeService) DeleteEventNotifying(_ context.Context, id uuid.UUID, send bool) error {
	f.gotDeleteID, f.gotDeleteSend = id, send
	return f.deleteErr
}

func (f *fakeService) ListInstances(_ context.Context, _ uuid.UUID, from, to time.Time) ([]domain.Instance, error) {
	f.gotFrom, f.gotTo = from, to
	return f.instances, f.instErr
}

func (f *fakeService) ListObjects(_ context.Context, calID uuid.UUID) ([]domain.CalendarObject, error) {
	var out []domain.CalendarObject
	for _, o := range f.objects {
		if o.CalendarID == calID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakeService) ListEventsInRange(_ context.Context, calID uuid.UUID, from, to time.Time) ([]domain.Event, error) {
	f.gotFrom, f.gotTo, f.gotRangeCal = from, to, calID
	return f.rangeEvents, f.rangeErr
}

func (f *fakeService) EsInvitadoDeEvento(context.Context, string) (bool, error) { return false, nil }

func (f *fakeService) SearchEvents(_ context.Context, q store.EventSearchQuery) ([]domain.Event, error) {
	f.gotSearch = q
	return f.searchEvents, f.searchErr
}

func (f *fakeService) SetAttendeePartStat(_ context.Context, _ uuid.UUID, email string, _ domain.ParticipationStatus) (domain.Attendee, error) {
	f.gotPartEml = email
	return f.attendee, f.partErr
}

func (f *fakeService) FreeBusy(_ context.Context, user string, _, _ time.Time) ([]domain.FreeBusyPeriod, error) {
	f.fbUser = user
	return f.periods, f.fbErr
}

func (f *fakeService) SuggestSlots(_ context.Context, users []string, _, _ time.Time, dur time.Duration, _ *time.Location, wh domain.WorkingHours, _ int) ([]domain.FreeSlot, error) {
	f.gotSlotUser = users
	f.gotSlotDur = dur
	f.gotSlotWH = wh
	return f.slots, f.slotsErr
}

func (f *fakeService) PedirEdicion(context.Context, uuid.UUID, peticiones.Peticion, string) (bool, error) {
	return false, nil
}

func (f *fakeService) PeticionesEdicion(context.Context, uuid.UUID, string) ([]peticiones.Peticion, error) {
	return nil, nil
}

func (f *fakeService) ResolverPeticion(context.Context, uuid.UUID, string, string, string, bool) error {
	return nil
}

func (f *fakeService) ListInbox(_ context.Context, _ string, onlyPending bool, _ int) ([]domain.Invitation, error) {
	f.gotPend = onlyPending
	return f.inbox, f.inboxErr
}

func (f *fakeService) RespondInvitation(_ context.Context, _ uuid.UUID, status domain.InvitationStatus) (domain.Invitation, error) {
	f.gotStatus = status
	return f.inv, f.respErr
}

func (f *fakeService) RecibirRespuesta(ctx context.Context, remitente, ics string) error {
	t, _ := tenancy.From(ctx)
	f.gotIMIP = t.Slug + "|" + remitente + "|" + ics
	return f.imipErr
}

func (f *fakeService) DueReminders(_ context.Context, ownerSub string, _ time.Time, before *store.ReminderCursor, _ int) ([]store.DueReminder, error) {
	f.gotDueSub = ownerSub
	f.gotDueBefore = before
	return f.dueReminders, f.dueErr
}

func (f *fakeService) DismissReminder(_ context.Context, ownerSub string, id uuid.UUID) error {
	f.gotDismissSub, f.gotDismissID = ownerSub, id
	return f.dismissErr
}

func (f *fakeService) AmendRecurrence(_ context.Context, _ uuid.UUID, uid string, recurrenceID time.Time, mode string, _ uuid.UUID) (domain.CalendarObject, error) {
	f.gotAmend = uid + "|" + mode
	f.gotAmendR = recurrenceID
	return f.amendObj, f.amendErr
}

// --- Helpers de test ---------------------------------------------------------

func newHandlers(f *fakeService) *Handlers {
	return &Handlers{svc: f, cfg: config.Config{
		MaxFreeBusyWindow: 366 * 24 * time.Hour,
		MaxRangeWindow:    366 * 24 * time.Hour,
	}}
}

// req construye una petición con claims inyectados y params de ruta chi.
func req(method, target, body string, claims *authn.Claims, params map[string]string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, target, http.NoBody)
	}
	ctx := r.Context()
	if claims != nil {
		ctx = authn.Into(ctx, *claims)
	}
	if params != nil {
		rc := chi.NewRouteContext()
		for k, v := range params {
			rc.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rc)
	}
	return r.WithContext(ctx)
}

func authClaims() *authn.Claims {
	return &authn.Claims{Subject: "user-1", Email: "u@ex.com", Tenant: "acme"}
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body no es JSON: %v (%q)", err, rec.Body.String())
	}
	return m
}

// --- Calendarios -------------------------------------------------------------

func TestListCalendarsUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).ListCalendars(rec, req(http.MethodGet, "/", "", nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
}

func TestListCalendarsOK(t *testing.T) {
	f := &fakeService{calendars: []domain.Calendar{{Name: "work"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListCalendars(rec, req(http.MethodGet, "/", "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	m := decodeEnvelope(t, rec)
	if _, ok := m["calendars"]; !ok {
		t.Errorf("falta clave 'calendars': %v", m)
	}
}

func TestListCalendarsEnsureError(t *testing.T) {
	f := &fakeService{ensureErr: domain.ErrConflict}
	rec := httptest.NewRecorder()
	newHandlers(f).ListCalendars(rec, req(http.MethodGet, "/", "", authClaims(), nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

func TestCreateCalendarOK(t *testing.T) {
	f := &fakeService{createdCal: domain.Calendar{Name: "n"}}
	rec := httptest.NewRecorder()
	body := `{"name":"n","display_name":"N","timezone":"UTC"}`
	newHandlers(f).CreateCalendar(rec, req(http.MethodPost, "/", body, authClaims(), nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201", rec.Code)
	}
	if f.gotNewCal.OwnerSub != "user-1" || f.gotNewCal.Name != "n" {
		t.Errorf("NewCalendar mal mapeado: %+v", f.gotNewCal)
	}
}

func TestCreateCalendarUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).CreateCalendar(rec, req(http.MethodPost, "/", `{}`, nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestCreateCalendarBadJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).CreateCalendar(rec, req(http.MethodPost, "/", `{bad`, authClaims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestCreateCalendarValidationError(t *testing.T) {
	f := &fakeService{createErr: domain.ErrInvalidName}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateCalendar(rec, req(http.MethodPost, "/", `{"name":""}`, authClaims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Suscripciones ICS -------------------------------------------------------

func TestSubscribeOK(t *testing.T) {
	f := &fakeService{subCal: domain.Calendar{Name: "ics-festivos", ReadOnly: true}}
	rec := httptest.NewRecorder()
	body := `{"url":"https://example.com/holidays.ics","display_name":"Festivos","color":"#0F7052"}`
	newHandlers(f).Subscribe(rec, req(http.MethodPost, "/", body, authClaims(), nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201", rec.Code)
	}
	if f.gotSubURL != "https://example.com/holidays.ics" || f.gotSubName != "Festivos" || f.gotSubColor != "#0F7052" {
		t.Errorf("Subscribe mal mapeado: url=%q name=%q color=%q", f.gotSubURL, f.gotSubName, f.gotSubColor)
	}
}

func TestSubscribeUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).Subscribe(rec, req(http.MethodPost, "/", `{}`, nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestSubscribeInvalidURL(t *testing.T) {
	f := &fakeService{subErr: domain.ErrInvalidURL}
	rec := httptest.NewRecorder()
	newHandlers(f).Subscribe(rec, req(http.MethodPost, "/", `{"url":"file:///etc/passwd"}`, authClaims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400 (ErrInvalidURL)", rec.Code)
	}
}

func TestRefreshSubscriptionOK(t *testing.T) {
	id := uuid.New()
	f := &fakeService{refreshedCal: domain.Calendar{ID: id, ReadOnly: true}}
	rec := httptest.NewRecorder()
	newHandlers(f).RefreshSubscription(rec, req(http.MethodPost, "/", "", authClaims(), map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotRefreshID != id {
		t.Errorf("RefreshSubscription id = %v, want %v", f.gotRefreshID, id)
	}
}

// TestUpdateEventReadOnly comprueba que ErrReadOnly (una escritura sobre un
// calendario de suscripción) mapea a 403 en la capa HTTP.
func TestUpdateEventReadOnly(t *testing.T) {
	f := &fakeService{eventErr: domain.ErrReadOnly}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateEvent(rec, req(http.MethodPut, "/", `{"summary":"z","start":"2026-01-01T09:00:00Z","end":"2026-01-01T10:00:00Z"}`,
		authClaims(), map[string]string{"id": uuid.New().String()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403 (ErrReadOnly)", rec.Code)
	}
}

func TestGetCalendarBadID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).GetCalendar(rec, req(http.MethodGet, "/", "", authClaims(), map[string]string{"id": "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestGetCalendarNotFound(t *testing.T) {
	f := &fakeService{calErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).GetCalendar(rec, req(http.MethodGet, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestGetCalendarOK(t *testing.T) {
	f := &fakeService{cal: domain.Calendar{Name: "c"}}
	rec := httptest.NewRecorder()
	newHandlers(f).GetCalendar(rec, req(http.MethodGet, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestUpdateCalendarOK(t *testing.T) {
	f := &fakeService{updatedCal: domain.Calendar{Name: "u"}}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateCalendar(rec, req(http.MethodPatch, "/", `{"display_name":"X"}`, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestUpdateCalendarBadID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).UpdateCalendar(rec, req(http.MethodPatch, "/", `{}`, authClaims(), map[string]string{"id": "x"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestDeleteCalendarDefaultImmutable(t *testing.T) {
	f := &fakeService{deleteErr: domain.ErrDefaultImmutable}
	rec := httptest.NewRecorder()
	newHandlers(f).DeleteCalendar(rec, req(http.MethodDelete, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestDeleteCalendarOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).DeleteCalendar(rec, req(http.MethodDelete, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

// --- Eventos -----------------------------------------------------------------

func TestCreateEventOK(t *testing.T) {
	f := &fakeService{event: domain.Event{UID: "x"}}
	rec := httptest.NewRecorder()
	body := `{"summary":"s","start":"2026-01-01T10:00:00Z","end":"2026-01-01T11:00:00Z"}`
	newHandlers(f).CreateEvent(rec, req(http.MethodPost, "/", body, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	// Sin organizer en el cuerpo se usa el email del token.
	if f.gotEventIn.Organizer != "u@ex.com" {
		t.Errorf("Organizer = %q, want u@ex.com", f.gotEventIn.Organizer)
	}
}

func TestCreateEventBadCalendarID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).CreateEvent(rec, req(http.MethodPost, "/", `{}`, authClaims(), map[string]string{"id": "bad"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// Los tres de abajo cubren la verja de un calendario COMPARTIDO en las rutas
// normales, que son las únicas que quedan: `requireCalendarWrite` devuelve
// ErrReadOnly a un reader y ErrNotFound a quien no tiene acceso, y aquí se
// comprueba que la capa HTTP los traduzca a 403 y 404. Vivían en las rutas
// paralelas `/shared-events` y `/shared-instances`, retiradas por duplicadas;
// la cobertura se trasladó con ellas en vez de irse por el desagüe.
//
// El 404 —y no un 403— cuando no hay acceso es deliberado: un 403 confirmaría
// que ese calendario existe, y con ids ajenos eso es enumerar.

// Un reader que intenta CREAR en un calendario compartido → 403.
func TestCreateEventReadOnly(t *testing.T) {
	f := &fakeService{eventErr: domain.ErrReadOnly}
	rec := httptest.NewRecorder()
	body := `{"summary":"z","start":"2026-01-01T09:00:00Z","end":"2026-01-01T10:00:00Z"}`
	newHandlers(f).CreateEvent(rec, req(http.MethodPost, "/", body, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403 (reader no escribe)", rec.Code)
	}
}

// Crear en un calendario que no es tuyo ni te han compartido → 404, no 403.
func TestCreateEventNoAccess(t *testing.T) {
	f := &fakeService{eventErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	body := `{"summary":"z","start":"2026-01-01T09:00:00Z","end":"2026-01-01T10:00:00Z"}`
	newHandlers(f).CreateEvent(rec, req(http.MethodPost, "/", body, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404 (sin filtrar existencia)", rec.Code)
	}
}

// Pintar los eventos de un calendario sin acceso → 404.
func TestListInstancesNoAccess(t *testing.T) {
	f := &fakeService{instErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	r := req(http.MethodGet, "/?start=2026-01-01T00:00:00Z&end=2026-01-02T00:00:00Z", "",
		authClaims(), map[string]string{"id": uuid.NewString()})
	newHandlers(f).ListInstances(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestCreateEventValidationError(t *testing.T) {
	f := &fakeService{eventErr: domain.ErrInvalidRange}
	rec := httptest.NewRecorder()
	body := `{"start":"2026-01-01T11:00:00Z","end":"2026-01-01T10:00:00Z"}`
	newHandlers(f).CreateEvent(rec, req(http.MethodPost, "/", body, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestGetEventNotFound(t *testing.T) {
	f := &fakeService{eventErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).GetEvent(rec, req(http.MethodGet, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestUpdateEventOK(t *testing.T) {
	f := &fakeService{event: domain.Event{UID: "orig", Organizer: "org@ex.com", CalendarID: uuid.New()}}
	rec := httptest.NewRecorder()
	body := `{"uid":"ignored","summary":"s","start":"2026-01-01T10:00:00Z","end":"2026-01-01T11:00:00Z"}`
	newHandlers(f).UpdateEvent(rec, req(http.MethodPut, "/", body, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	// El UID del cuerpo se ignora en favor del actual.
	if f.gotEventIn.UID != "orig" {
		t.Errorf("UID = %q, want orig (inmutable)", f.gotEventIn.UID)
	}
}

// Los recordatorios viajan en el cuerpo REST: ausentes no se tocan (nil), y una lista
// —aunque vacía— los sustituye. La diferencia entre las dos es lo que protege los que
// la persona puso en la web cuando la edita un cliente que no sabe de ellos.
func TestEventReqLlevaLosRecordatorios(t *testing.T) {
	casos := map[string]*[]int{
		`{"summary":"s","start":"2026-01-01T10:00:00Z","end":"2026-01-01T11:00:00Z"}`:                       nil,
		`{"summary":"s","start":"2026-01-01T10:00:00Z","end":"2026-01-01T11:00:00Z","reminders":null}`:      nil,
		`{"summary":"s","start":"2026-01-01T10:00:00Z","end":"2026-01-01T11:00:00Z","reminders":[]}`:        {},
		`{"summary":"s","start":"2026-01-01T10:00:00Z","end":"2026-01-01T11:00:00Z","reminders":[10,1440]}`: {10, 1440},
	}
	for body, want := range casos {
		f := &fakeService{event: domain.Event{UID: "x", CalendarID: uuid.New()}}
		rec := httptest.NewRecorder()
		newHandlers(f).UpdateEvent(rec, req(http.MethodPut, "/", body, authClaims(), map[string]string{"id": uuid.NewString()}))
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, body=%s", rec.Code, rec.Body.String())
		}
		got := f.gotEventIn.Reminders
		if (got == nil) != (want == nil) || (got != nil && !reflect.DeepEqual(*got, *want)) {
			t.Errorf("%s: Reminders = %v, quiero %v", body, got, want)
		}
	}
}

// El evento suelto trae sus recordatorios (siempre la lista, aunque esté vacía).
func TestGetEventTraeSusRecordatorios(t *testing.T) {
	for _, tc := range []struct {
		rec  []int
		want string
	}{{[]int{15, 60}, `"reminders":[15,60]`}, {[]int{}, `"reminders":[]`}} {
		f := &fakeService{event: domain.Event{UID: "u-1", Summary: "s"}, reminders: tc.rec}
		rec := httptest.NewRecorder()
		newHandlers(f).GetEvent(rec, req(http.MethodGet, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) || !strings.Contains(rec.Body.String(), `"uid":"u-1"`) {
			t.Fatalf("code = %d, body=%s, quiero %s y el evento entero", rec.Code, rec.Body.String(), tc.want)
		}
	}
}

func TestUpdateEventNotFound(t *testing.T) {
	f := &fakeService{eventErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateEvent(rec, req(http.MethodPut, "/", `{}`, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestDeleteEventOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).DeleteEvent(rec, req(http.MethodDelete, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
}

// --- Instancias / rango ------------------------------------------------------

func TestListInstancesMissingRange(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).ListInstances(rec, req(http.MethodGet, "/?start=", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestListInstancesBadStart(t *testing.T) {
	rec := httptest.NewRecorder()
	target := "/?start=notadate&end=2026-01-02T00:00:00Z"
	newHandlers(&fakeService{}).ListInstances(rec, req(http.MethodGet, target, "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestListInstancesEndBeforeStart(t *testing.T) {
	rec := httptest.NewRecorder()
	target := "/?start=2026-01-02T00:00:00Z&end=2026-01-01T00:00:00Z"
	newHandlers(&fakeService{}).ListInstances(rec, req(http.MethodGet, target, "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestListInstancesTooWide(t *testing.T) {
	h := &Handlers{svc: &fakeService{}, cfg: config.Config{MaxRangeWindow: time.Hour}}
	rec := httptest.NewRecorder()
	target := "/?start=2026-01-01T00:00:00Z&end=2026-06-01T00:00:00Z"
	h.ListInstances(rec, req(http.MethodGet, target, "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestListInstancesOK(t *testing.T) {
	f := &fakeService{instances: []domain.Instance{{Event: domain.Event{UID: "a"}}}}
	rec := httptest.NewRecorder()
	target := "/?start=2026-01-01T00:00:00Z&end=2026-01-02T00:00:00Z"
	newHandlers(f).ListInstances(rec, req(http.MethodGet, target, "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	m := decodeEnvelope(t, rec)
	if _, ok := m["instances"]; !ok {
		t.Errorf("falta 'instances': %v", m)
	}
}

// --- RSVP --------------------------------------------------------------------

func TestSetPartStatDefaultsToOwnEmail(t *testing.T) {
	f := &fakeService{attendee: domain.Attendee{Email: "u@ex.com"}}
	rec := httptest.NewRecorder()
	newHandlers(f).SetPartStat(rec, req(http.MethodPut, "/", `{"partstat":"ACCEPTED"}`, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotPartEml != "u@ex.com" {
		t.Errorf("email = %q, want u@ex.com (default)", f.gotPartEml)
	}
}

// TestSetPartStatIgnoresBodyEmail verifica la verja anti-spoof del RSVP: el
// email del cuerpo se IGNORA y siempre se responde como el usuario del JWT
// (nadie sobrescribe el PARTSTAT de otro asistente por este endpoint).
func TestSetPartStatIgnoresBodyEmail(t *testing.T) {
	f := &fakeService{attendee: domain.Attendee{Email: "u@ex.com"}}
	rec := httptest.NewRecorder()
	newHandlers(f).SetPartStat(rec, req(http.MethodPut, "/",
		`{"email":"victima@ex.com","partstat":"DECLINED"}`, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotPartEml != "u@ex.com" {
		t.Errorf("email = %q, want u@ex.com (el del JWT, nunca el del cuerpo)", f.gotPartEml)
	}
}

func TestSetPartStatBadID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).SetPartStat(rec, req(http.MethodPut, "/", `{}`, authClaims(), map[string]string{"id": "x"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Free/Busy ---------------------------------------------------------------

func TestFreeBusyDefaultsToSelf(t *testing.T) {
	f := &fakeService{periods: []domain.FreeBusyPeriod{}}
	rec := httptest.NewRecorder()
	target := "/?start=2026-01-01T00:00:00Z&end=2026-01-02T00:00:00Z"
	newHandlers(f).FreeBusy(rec, req(http.MethodGet, target, "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if f.fbUser != "user-1" {
		t.Errorf("user = %q, want user-1 (self)", f.fbUser)
	}
}

func TestFreeBusyExplicitUser(t *testing.T) {
	f := &fakeService{periods: []domain.FreeBusyPeriod{}}
	rec := httptest.NewRecorder()
	target := "/?user=peer&start=2026-01-01T00:00:00Z&end=2026-01-02T00:00:00Z"
	newHandlers(f).FreeBusy(rec, req(http.MethodGet, target, "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.fbUser != "peer" {
		t.Errorf("user = %q, want peer", f.fbUser)
	}
}

func TestFreeBusyNoUserNoAuth(t *testing.T) {
	rec := httptest.NewRecorder()
	target := "/?start=2026-01-01T00:00:00Z&end=2026-01-02T00:00:00Z"
	newHandlers(&fakeService{}).FreeBusy(rec, req(http.MethodGet, target, "", nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestFreeBusyBadRange(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).FreeBusy(rec, req(http.MethodGet, "/", "", authClaims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// TestSuggestSlotsIncludesSelfAndParams: «buscar hueco» incluye SIEMPRE al
// organizador (self) ademas de los ?user, y parsea duracion/franja laboral.
func TestSuggestSlotsIncludesSelfAndParams(t *testing.T) {
	f := &fakeService{slots: []domain.FreeSlot{{}}}
	rec := httptest.NewRecorder()
	target := "/?user=peer&start=2026-01-01T00:00:00Z&end=2026-01-08T00:00:00Z" +
		"&duration=45&work_start=08:30&work_end=18:00&weekdays=true"
	newHandlers(f).SuggestSlots(rec, req(http.MethodGet, target, "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if len(f.gotSlotUser) != 2 || f.gotSlotUser[0] != "peer" || f.gotSlotUser[1] != "user-1" {
		t.Errorf("users = %v, want [peer user-1]", f.gotSlotUser)
	}
	if f.gotSlotDur != 45*time.Minute {
		t.Errorf("duration = %s, want 45m", f.gotSlotDur)
	}
	if f.gotSlotWH.StartMin != 8*60+30 || f.gotSlotWH.EndMin != 18*60 || !f.gotSlotWH.WeekdaysOnly {
		t.Errorf("working hours = %+v, want 08:30-18:00 weekdays", f.gotSlotWH)
	}
}

func TestSuggestSlotsNoAuth(t *testing.T) {
	rec := httptest.NewRecorder()
	target := "/?start=2026-01-01T00:00:00Z&end=2026-01-02T00:00:00Z"
	newHandlers(&fakeService{}).SuggestSlots(rec, req(http.MethodGet, target, "", nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

// --- Invitaciones ------------------------------------------------------------

func TestListInboxNoEmailForbidden(t *testing.T) {
	rec := httptest.NewRecorder()
	claims := &authn.Claims{Subject: "u"} // sin email
	newHandlers(&fakeService{}).ListInbox(rec, req(http.MethodGet, "/", "", claims, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

func TestListInboxPendingFilter(t *testing.T) {
	f := &fakeService{inbox: []domain.Invitation{}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListInbox(rec, req(http.MethodGet, "/?pending=true", "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if !f.gotPend {
		t.Error("onlyPending debe ser true con ?pending=true")
	}
}

func TestRespondInvitationOK(t *testing.T) {
	f := &fakeService{inv: domain.Invitation{Status: domain.InvAccepted}}
	rec := httptest.NewRecorder()
	newHandlers(f).RespondInvitation(rec, req(http.MethodPost, "/", `{"status":"accepted"}`, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotStatus != domain.InvAccepted {
		t.Errorf("status = %q, want accepted", f.gotStatus)
	}
}

func TestRespondInvitationBadID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).RespondInvitation(rec, req(http.MethodPost, "/", `{}`, authClaims(), map[string]string{"id": "x"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestRespondInvitationConflict(t *testing.T) {
	f := &fakeService{respErr: domain.ErrConflict}
	rec := httptest.NewRecorder()
	newHandlers(f).RespondInvitation(rec, req(http.MethodPost, "/", `{"status":"declined"}`, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

// --- Recordatorios in-app ----------------------------------------------------

func TestListDueRemindersOK(t *testing.T) {
	f := &fakeService{dueReminders: []store.DueReminder{{ID: uuid.New(), UID: "ev-1", Summary: "Standup"}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListDueReminders(rec, req(http.MethodGet, "/", "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	m := decodeEnvelope(t, rec)
	if _, ok := m["reminders"]; !ok {
		t.Errorf("falta 'reminders': %v", m)
	}
	if f.gotDueSub != "user-1" {
		t.Errorf("ownerSub = %q, want user-1 (self)", f.gotDueSub)
	}
	// Con 1 < limit (50) no hay siguiente página.
	if m["next_cursor"] != "" {
		t.Errorf("next_cursor debe ser vacío sin página siguiente: %v", m["next_cursor"])
	}
}

func TestListDueRemindersUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).ListDueReminders(rec, req(http.MethodGet, "/", "", nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

func TestListDueRemindersBadCursor(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).ListDueReminders(rec, req(http.MethodGet, "/?cursor=@@notbase64@@", "", authClaims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestDismissReminderOK(t *testing.T) {
	f := &fakeService{}
	id := uuid.New()
	rec := httptest.NewRecorder()
	newHandlers(f).DismissReminder(rec, req(http.MethodPost, "/", "", authClaims(), map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotDismissID != id || f.gotDismissSub != "user-1" {
		t.Errorf("dismiss mal enrutado: id=%s sub=%s", f.gotDismissID, f.gotDismissSub)
	}
}

func TestDismissReminderNotFound(t *testing.T) {
	f := &fakeService{dismissErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).DismissReminder(rec, req(http.MethodPost, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestDismissReminderBadID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).DismissReminder(rec, req(http.MethodPost, "/", "", authClaims(), map[string]string{"id": "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestReminderCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 17, 8, 30, 15, 123, time.UTC)
	id := uuid.New()
	cur, err := parseReminderCursor(encodeReminderCursor(ts, id))
	if err != nil {
		t.Fatalf("parseReminderCursor: %v", err)
	}
	if !cur.TriggerAt.Equal(ts) || cur.ID != id {
		t.Errorf("round-trip = (%s, %s), want (%s, %s)", cur.TriggerAt, cur.ID, ts, id)
	}
	if c, err := parseReminderCursor(""); err != nil || c != nil {
		t.Errorf("cursor vacío debe ser (nil, nil), got (%v, %v)", c, err)
	}
}

// --- Amend recurrence --------------------------------------------------------

func TestAmendRecurrenceOK(t *testing.T) {
	f := &fakeService{amendObj: domain.CalendarObject{UID: "series-1"}}
	rec := httptest.NewRecorder()
	body := `{"uid":"series-1","recurrence_id":"2026-07-22T09:00:00Z","mode":"exclude"}`
	newHandlers(f).AmendRecurrence(rec, req(http.MethodPost, "/", body, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotAmend != "series-1|exclude" {
		t.Errorf("amend mal enrutado: %q", f.gotAmend)
	}
	if !f.gotAmendR.Equal(time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("recurrence_id = %s, want 2026-07-22T09:00:00Z", f.gotAmendR)
	}
}

func TestAmendRecurrenceBadMode(t *testing.T) {
	rec := httptest.NewRecorder()
	body := `{"uid":"x","recurrence_id":"2026-07-22T09:00:00Z","mode":"bogus"}`
	newHandlers(&fakeService{}).AmendRecurrence(rec, req(http.MethodPost, "/", body, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestAmendRecurrenceMissingFields(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).AmendRecurrence(rec, req(http.MethodPost, "/", `{"mode":"until"}`, authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- fail() mapping ----------------------------------------------------------

func TestFailPreconditionFailed(t *testing.T) {
	f := &fakeService{calErr: domain.ErrPreconditionFailed}
	rec := httptest.NewRecorder()
	newHandlers(f).GetCalendar(rec, req(http.MethodGet, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("code = %d, want 412", rec.Code)
	}
}

func TestFailInternalDefault(t *testing.T) {
	f := &fakeService{calErr: context.DeadlineExceeded} // error genérico => 500
	rec := httptest.NewRecorder()
	newHandlers(f).GetCalendar(rec, req(http.MethodGet, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
}
