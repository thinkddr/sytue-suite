// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/soltarfuente"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// «Soltar la fuente» de punta a punta sobre el esquema real de Bases: congelada,
// las apps nativas van a Matrix; soltada con confirmación y reautenticación, vuelve a los
// datos de antes de congelar y escribe otra vez.
func TestSoltarLaFuenteDeBases(t *testing.T) {
	e := nuevo(t)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	scope := storage.Scope{Tenant: "acme", Subject: "ana", Product: "bases"}
	epoch := uuid.New()
	if state, err := e.s.BeginPrivateStorageMigration(ctx, "ana", epoch); err != nil || state.Mode != "frozen" {
		t.Fatal("no se congeló", state, err)
	}
	// Congelada, la base que salió con ana ya no admite escrituras en el servidor
	// (migración 0036); soltar es lo que las devuelve a la API.
	if _, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Durante"); !errors.Is(PrivateStorageError(err), ErrPrivateStorageFrozen) {
		t.Fatal("la fuente congelada admite escrituras", err)
	}
	if _, err := e.s.CrearBase(e.ctx, ana, "Nueva", ""); !errors.Is(PrivateStorageError(err), ErrPrivateStorageFrozen) {
		t.Fatal("una base nueva escapa de la fuente congelada", err)
	}
	var avisos []hubnotify.Notification
	handler, err := soltarfuente.Handler(soltarfuente.Config{
		Product: "bases", Soltar: e.s.ReleasePrivateStorage,
		Avisar: func(_ context.Context, n hubnotify.Notification) error { avisos = append(avisos, n); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	no := false
	soltar := func(body string, authTime time.Time) *httptest.ResponseRecorder {
		claims := authn.Claims{Subject: "ana", Tenant: "acme", AuthTime: authTime, AMR: []string{"pwd"}, MFAEnabled: &no}
		r := httptest.NewRequest(http.MethodPost, soltarfuente.Ruta, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		encoded, _ := storage.EncodeScopeHeader(scope)
		r.Header.Set(storage.ScopeHeader, encoded)
		r = r.WithContext(authn.Into(ctx, claims))
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	confirmado := `{"epoch":"` + epoch.String() + `","discard_private_writes":true}`
	if w := soltar(confirmado, time.Now().Add(-10*time.Minute)); w.Code != http.StatusUnauthorized || !strings.Contains(w.Header().Get("WWW-Authenticate"), "max_age=300") {
		t.Fatal("sin identificación reciente", w.Code, w.Body.String())
	}
	if w := soltar(`{"epoch":"`+epoch.String()+`"}`, time.Now()); w.Code != http.StatusBadRequest {
		t.Fatal("sin confirmación", w.Code, w.Body.String())
	}
	if w := soltar(`{"epoch":"`+uuid.NewString()+`","discard_private_writes":true}`, time.Now()); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "private_storage_release_stale") {
		t.Fatal("época vieja", w.Code, w.Body.String())
	}
	if state, _ := e.s.PrivateStorageState(ctx, "ana"); state.Mode != "frozen" || len(avisos) != 0 {
		t.Fatal("un intento rechazado cambió algo", state.Mode, avisos)
	}
	if w := soltar(confirmado, time.Now().Add(-time.Minute)); w.Code != http.StatusOK {
		t.Fatal("no se soltó", w.Code, w.Body.String())
	}
	if state, err := e.s.PrivateStorageState(ctx, "ana"); err != nil || state.Mode != "legacy" {
		t.Fatal("no volvió a legacy", state, err)
	}
	if _, err := e.s.CrearTabla(e.ctx, ana, e.base.ID, "Después"); err != nil {
		t.Fatal("la fuente soltada no admite escrituras", err)
	}
	tablas, err := e.s.ListarTablas(e.ctx, ana, e.base.ID)
	if err != nil || len(tablas) != 2 {
		t.Fatal("los datos de antes de congelar no siguen ahí", len(tablas), err)
	}
	if len(avisos) != 1 || avisos[0].Type != soltarfuente.TipoAviso || avisos[0].Subject != "ana" {
		t.Fatal("sin aviso a la persona", avisos)
	}
}
