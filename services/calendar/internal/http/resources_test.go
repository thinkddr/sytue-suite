// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// adminClaims son claims con rol admin (para alta de recursos).
func adminClaims() *authn.Claims {
	return &authn.Claims{Subject: "user-1", Email: "u@ex.com", Tenant: "acme", Roles: []string{"admin"}}
}

func TestListResourcesOK(t *testing.T) {
	f := &fakeService{resources: []store.ResourceAvailability{{
		Resource: domain.Resource{Name: "Sala A", Kind: domain.ResourceRoom},
		Busy:     []domain.FreeBusyPeriod{},
	}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListResources(rec, req(http.MethodGet, "/", "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if _, ok := decodeEnvelope(t, rec)["resources"]; !ok {
		t.Errorf("falta clave 'resources'")
	}
}

func TestListResourcesUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).ListResources(rec, req(http.MethodGet, "/", "", nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

// Con ?start&end se pide disponibilidad; el rango se propaga al servicio.
func TestListResourcesWithRange(t *testing.T) {
	f := &fakeService{resources: []store.ResourceAvailability{}}
	rec := httptest.NewRecorder()
	target := "/?start=2026-07-01T00:00:00Z&end=2026-07-02T00:00:00Z"
	newHandlers(f).ListResources(rec, req(http.MethodGet, target, "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotFrom.IsZero() || f.gotTo.IsZero() {
		t.Errorf("el rango de disponibilidad no se propagó: from=%v to=%v", f.gotFrom, f.gotTo)
	}
}

// Dar de alta un recurso SIN rol admin → 403 (no llega al servicio).
func TestCreateResourceForbiddenWithoutAdmin(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	body := `{"kind":"room","name":"Sala A","capacity":8}`
	newHandlers(f).CreateResource(rec, req(http.MethodPost, "/", body, authClaims(), nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403 (sin rol admin)", rec.Code)
	}
	if f.gotNewRes.Name != "" {
		t.Errorf("no debe llamar al servicio sin rol admin")
	}
}

// Con rol admin, el alta pasa y el creador queda como created_by.
func TestCreateResourceAdminOK(t *testing.T) {
	f := &fakeService{resource: domain.Resource{Name: "Sala A"}}
	rec := httptest.NewRecorder()
	body := `{"kind":"room","name":"Sala A","capacity":8,"location":"P2","email":"sala-a@ex.com"}`
	newHandlers(f).CreateResource(rec, req(http.MethodPost, "/", body, adminClaims(), nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotNewRes.Kind != domain.ResourceRoom || f.gotNewRes.Name != "Sala A" || f.gotNewRes.CreatedBy != "user-1" {
		t.Errorf("NewResource mal mapeado: %+v", f.gotNewRes)
	}
}

func TestCreateResourceUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).CreateResource(rec, req(http.MethodPost, "/", `{}`, nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

// Datos inválidos (tipo desconocido) → ErrInvalidResource → 400.
func TestCreateResourceInvalid(t *testing.T) {
	f := &fakeService{resourceErr: domain.ErrInvalidResource}
	rec := httptest.NewRecorder()
	body := `{"kind":"vehicle","name":"Coche"}`
	newHandlers(f).CreateResource(rec, req(http.MethodPost, "/", body, adminClaims(), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// Editar sin ser gestor: el servicio devuelve ErrNotManager → 403.
func TestUpdateResourceNotManager(t *testing.T) {
	f := &fakeService{resourceErr: domain.ErrNotManager}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateResource(rec, req(http.MethodPatch, "/", `{"name":"Nueva"}`,
		authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403 (no gestor)", rec.Code)
	}
	if f.gotResSub != "user-1" || f.gotResAdmin {
		t.Errorf("update debe pasar sub=user-1, isAdmin=false: sub=%q admin=%v", f.gotResSub, f.gotResAdmin)
	}
}

func TestUpdateResourceAdminFlag(t *testing.T) {
	f := &fakeService{resource: domain.Resource{Name: "Nueva"}}
	rec := httptest.NewRecorder()
	newHandlers(f).UpdateResource(rec, req(http.MethodPatch, "/", `{"active":false}`,
		adminClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if !f.gotResAdmin {
		t.Errorf("isAdmin debe ser true con rol admin")
	}
	if f.gotResPatch.Active == nil || *f.gotResPatch.Active {
		t.Errorf("patch active debe ser false: %+v", f.gotResPatch.Active)
	}
}

func TestDeleteResourceOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	newHandlers(f).DeleteResource(rec, req(http.MethodDelete, "/", "", authClaims(),
		map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotResSub != "user-1" {
		t.Errorf("delete debe pasar el sub del JWT")
	}
}

// Reservar una sala LIBRE → accepted.
func TestBookResourceAccepted(t *testing.T) {
	f := &fakeService{booking: store.BookingResult{Status: domain.PartAccepted, Resource: domain.Resource{Name: "Sala A"}}}
	rec := httptest.NewRecorder()
	body := `{"uid":"evt-1","summary":"Reunión","start":"2026-07-01T09:00:00Z","end":"2026-07-01T10:00:00Z"}`
	newHandlers(f).BookResource(rec, req(http.MethodPost, "/", body, authClaims(),
		map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if got := decodeEnvelope(t, rec)["status"]; got != "accepted" {
		t.Errorf("status = %v, want accepted", got)
	}
	// El organizer por defecto es el email del JWT y el UID viaja en el spec.
	if f.gotBookSpec.Organizer != "u@ex.com" || f.gotBookSpec.SourceUID != "evt-1" {
		t.Errorf("spec mal mapeado: %+v", f.gotBookSpec)
	}
}

// Reservar una sala OCUPADA → declined + el periodo en conflicto.
func TestBookResourceDeclined(t *testing.T) {
	f := &fakeService{booking: store.BookingResult{
		Status:   domain.PartDeclined,
		Conflict: &domain.FreeBusyPeriod{Type: domain.FBBusy},
	}}
	rec := httptest.NewRecorder()
	body := `{"uid":"evt-2","summary":"x","start":"2026-07-01T09:00:00Z","end":"2026-07-01T10:00:00Z"}`
	newHandlers(f).BookResource(rec, req(http.MethodPost, "/", body, authClaims(),
		map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	m := decodeEnvelope(t, rec)
	if m["status"] != "declined" {
		t.Errorf("status = %v, want declined", m["status"])
	}
	if _, ok := m["conflict"]; !ok {
		t.Errorf("una reserva rechazada debe incluir el periodo en conflicto")
	}
}

func TestBookResourceUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).BookResource(rec, req(http.MethodPost, "/", `{}`, nil,
		map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

// Reservar un recurso desactivado → ErrResourceInactive → 409.
func TestBookResourceInactive(t *testing.T) {
	f := &fakeService{bookingErr: domain.ErrResourceInactive}
	rec := httptest.NewRecorder()
	body := `{"uid":"evt-3","start":"2026-07-01T09:00:00Z","end":"2026-07-01T10:00:00Z"}`
	newHandlers(f).BookResource(rec, req(http.MethodPost, "/", body, authClaims(),
		map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", rec.Code)
	}
}

// Liberar sin ?source_uid → 400.
func TestReleaseResourceMissingUID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).ReleaseResource(rec, req(http.MethodDelete, "/", "", authClaims(),
		map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

func TestReleaseResourceOK(t *testing.T) {
	f := &fakeService{}
	rec := httptest.NewRecorder()
	newHandlers(f).ReleaseResource(rec, req(http.MethodDelete, "/?source_uid=evt-9", "", authClaims(),
		map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotRelUID != "evt-9" || f.gotResSub != "user-1" {
		t.Errorf("release mal enrutado: uid=%q sub=%q", f.gotRelUID, f.gotResSub)
	}
}

// Liberar sin permiso (ni gestor ni quien reservó) → ErrNotManager → 403.
func TestReleaseResourceForbidden(t *testing.T) {
	f := &fakeService{releaseErr: domain.ErrNotManager}
	rec := httptest.NewRecorder()
	newHandlers(f).ReleaseResource(rec, req(http.MethodDelete, "/?source_uid=evt-9", "", authClaims(),
		map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rec.Code)
	}
}

// TestBookResourceIgnoresOrganizerFromBody: el organizador de una reserva es QUIEN LA
// HACE. Antes se tomaba del cuerpo y solo se caía al usuario real si venía vacío, así
// que cualquiera podía reservar una sala a nombre de otra persona. Y no era cosmético:
// liberar la reserva se autoriza comparando ese organizador con quien llama, de modo que
// un campo del cuerpo decidía quién mandaba sobre la reserva.
func TestBookResourceIgnoresOrganizerFromBody(t *testing.T) {
	f := &fakeService{booking: store.BookingResult{Status: domain.PartAccepted}}
	rec := httptest.NewRecorder()
	body := `{"uid":"evt-1","organizer":"ceo@ex.com","start":"2026-07-01T09:00:00Z","end":"2026-07-01T10:00:00Z"}`
	newHandlers(f).BookResource(rec, req(http.MethodPost, "/", body, authClaims(),
		map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if f.gotBookSpec.Organizer != "u@ex.com" {
		t.Fatalf("organizer = %q: el del cuerpo debe IGNORARSE y usarse el del JWT", f.gotBookSpec.Organizer)
	}
}
