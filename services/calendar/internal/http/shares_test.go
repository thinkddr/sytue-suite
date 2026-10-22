// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendarhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// --- Compartir: gestión (solo el dueño) --------------------------------------

func TestCreateShareOK(t *testing.T) {
	calID := uuid.New()
	f := &fakeService{share: domain.CalendarShare{CalendarID: calID, Permission: domain.PermReader}}
	rec := httptest.NewRecorder()
	body := `{"email":"Grantee@Example.com","permission":"reader"}`
	newHandlers(f).CreateShare(rec, req(http.MethodPost, "/", body, authClaims(), map[string]string{"id": calID.String()}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201", rec.Code)
	}
	// El handler pasa el sub del JWT como dueño (la verja de dueño la aplica el
	// servicio) y el correo tal cual (la normalización vive en el servicio).
	if f.gotShareOwn != "user-1" {
		t.Errorf("ownerSub = %q, want user-1", f.gotShareOwn)
	}
	if f.gotShareEml != "Grantee@Example.com" || f.gotSharePrm != domain.PermReader {
		t.Errorf("share mal mapeado: email=%q perm=%q", f.gotShareEml, f.gotSharePrm)
	}
}

func TestCreateShareUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).CreateShare(rec, req(http.MethodPost, "/", `{}`, nil, map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

// Un correo/permiso inválido → ErrInvalidShare → 400 (IsValidationError).
func TestCreateShareInvalid(t *testing.T) {
	f := &fakeService{shareErr: domain.ErrInvalidShare}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateShare(rec, req(http.MethodPost, "/", `{"email":"nope","permission":"reader"}`,
		authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// Compartir consigo mismo → ErrShareSelf → 400.
func TestCreateShareSelf(t *testing.T) {
	f := &fakeService{shareErr: domain.ErrShareSelf}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateShare(rec, req(http.MethodPost, "/", `{"email":"u@ex.com","permission":"writer"}`,
		authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// Un no-dueño (el servicio devuelve ErrNotFound) → 404, sin filtrar existencia.
func TestCreateShareNotOwner(t *testing.T) {
	f := &fakeService{shareErr: domain.ErrNotFound}
	rec := httptest.NewRecorder()
	newHandlers(f).CreateShare(rec, req(http.MethodPost, "/", `{"email":"a@b.com","permission":"reader"}`,
		authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
}

func TestListSharesOK(t *testing.T) {
	f := &fakeService{shares: []domain.CalendarShare{{Permission: domain.PermWriter}}}
	rec := httptest.NewRecorder()
	newHandlers(f).ListShares(rec, req(http.MethodGet, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if _, ok := decodeEnvelope(t, rec)["shares"]; !ok {
		t.Errorf("falta clave 'shares'")
	}
}

// Sin shares, la lista es [] y nunca null (contrato del cliente).
func TestListSharesEmptyNotNull(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).ListShares(rec, req(http.MethodGet, "/", "", authClaims(), map[string]string{"id": uuid.NewString()}))
	if got := rec.Body.String(); got == "" || got == `{"shares":null}` {
		t.Errorf("shares debe ser [], no null: %q", got)
	}
}

func TestRevokeShareOK(t *testing.T) {
	shareID := uuid.New()
	f := &fakeService{}
	rec := httptest.NewRecorder()
	newHandlers(f).RevokeShare(rec, req(http.MethodDelete, "/", "", authClaims(),
		map[string]string{"id": uuid.NewString(), "shareId": shareID.String()}))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code = %d, want 204", rec.Code)
	}
	if f.gotRevoke != shareID || f.gotShareOwn != "user-1" {
		t.Errorf("revoke mal mapeado: share=%v owner=%q", f.gotRevoke, f.gotShareOwn)
	}
}

func TestRevokeShareBadShareID(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).RevokeShare(rec, req(http.MethodDelete, "/", "", authClaims(),
		map[string]string{"id": uuid.NewString(), "shareId": "nope"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
}

// --- Compartido conmigo ------------------------------------------------------

func TestSharedWithMeOK(t *testing.T) {
	f := &fakeService{sharedCals: []store.SharedCalendar{{
		Calendar:   domain.Calendar{OwnerSub: "owner-9", Name: "trabajo"},
		Permission: domain.PermReader,
		SharedBy:   "owner-9",
	}}}
	rec := httptest.NewRecorder()
	newHandlers(f).SharedWithMe(rec, req(http.MethodGet, "/", "", authClaims(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if f.gotSharedSub != "user-1" || f.gotSharedEml != "u@ex.com" {
		t.Errorf("SharedWithMe usa sub/email del JWT: sub=%q email=%q", f.gotSharedSub, f.gotSharedEml)
	}
}

func TestSharedWithMeEmptyNotNull(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).SharedWithMe(rec, req(http.MethodGet, "/", "", authClaims(), nil))
	if got := rec.Body.String(); got == `{"calendars":null}` {
		t.Errorf("calendars debe ser [], no null: %q", got)
	}
}

func TestSharedWithMeUnauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandlers(&fakeService{}).SharedWithMe(rec, req(http.MethodGet, "/", "", nil, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}

// --- Acceso a un calendario compartido (verja) -------------------------------
