// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// TestLoQueSeEscribeEnUnDocumentoAjenoAvisaASuPropietario (ADR 0078): una escritura en
// `/documents/{id}` suma a la señal de cambios al propietario y a las personas con
// permiso (no a un grupo), y solo dentro de una escritura con señal. VISTO EN ROJO sin el
// idr.Use(h.conQuienesVen).
func TestLoQueSeEscribeEnUnDocumentoAjenoAvisaASuPropietario(t *testing.T) {
	fs := &fakeStore{doc: &domain.Document{ID: "d1", Kind: domain.KindDocument, OwnerSub: "ana"}}
	fs.perms = []domain.Permission{
		{PrincipalKind: domain.PrincipalUser, PrincipalID: "bea", Role: domain.RoleEditor},
		{PrincipalKind: domain.PrincipalGroup, PrincipalID: "ventas", Role: domain.RoleViewer},
	}
	h := newHandler(fs)
	llega := false
	siguiente := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { llega = true })
	r := req(t, http.MethodPatch, "/v1/documents/d1", ownerClaims(), map[string]string{"id": "d1"}, "")
	ctx, avisados := hubnotify.EscrituraDePrueba(r.Context())
	h.conQuienesVen(siguiente).ServeHTTP(httptest.NewRecorder(), r.WithContext(ctx))
	if got := avisados(); !llega || !slices.Equal(got, []string{"ana", "bea"}) {
		t.Fatalf("la escritura avisa a %v (llega %v); quiero a ana y bea", got, llega)
	}
	// Y la ruta lo lleva puesto, tras pasar a la organización anfitriona.
	if !regexpRuta(t, "idr.Use(h.enAnfitrion)", "idr.Use(h.conQuienesVen)") {
		t.Fatal("/documents/{id} no suma a quienes ven el documento después de enAnfitrion")
	}
}

// regexpRuta dice si handler.go monta `antes` y, poco después, `despues`.
func regexpRuta(t *testing.T, antes, despues string) bool {
	t.Helper()
	fuente, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatal(err)
	}
	_, resto, ok := strings.Cut(string(fuente), antes)
	i := strings.Index(resto, despues)
	return ok && i > 0 && i < 200
}

// TestLaEdicionEnVivoAvisaUnaVezPorVentana: las operaciones de la edición en vivo (por
// REST y por el WebSocket) mandan la señal de su producto a quien escribe, al propietario
// y a las personas con permiso, pero una sola vez por documento y VentanaEnVivo: a ritmo
// de tecla serían cientos. VISTO EN ROJO sin el avisarEnVivo de aplicarOps.
func TestLaEdicionEnVivoAvisaUnaVezPorVentana(t *testing.T) {
	var mu sync.Mutex
	var llegan []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e struct {
			Tenant  string `json:"tenant"`
			Subject string `json:"subject"`
			Data    struct {
				Producto string `json:"producto"`
			} `json:"data"`
		}
		_ = json.NewDecoder(r.Body).Decode(&e)
		mu.Lock()
		llegan = append(llegan, e.Tenant+" "+e.Data.Producto+" "+e.Subject)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	recibido := func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := slices.Clone(llegan)
		slices.Sort(out)
		return out
	}

	fs := &fakeStore{doc: &domain.Document{ID: "d1", Kind: domain.KindSpreadsheet, OwnerSub: "ana"}}
	fs.perms = []domain.Permission{{PrincipalKind: domain.PrincipalUser, PrincipalID: "owner", Role: domain.RoleEditor}}
	h := newHandler(fs)
	h.hubNotifier = hubnotify.New(srv.URL, "s")
	h.enVivo = hubnotify.NuevaUnaVez(VentanaEnVivo)
	acme, err := tenancy.New("acme")
	if err != nil {
		t.Fatal(err)
	}
	ops := `{"ops":[{"kind":"insert","dot":{"a":"x","c":1},"value":"\"H\"","lamport":1}]}`
	for range 5 {
		r := req(t, http.MethodPost, "/v1/documents/d1/ops", ownerClaims(), map[string]string{"id": "d1"}, ops)
		rec := httptest.NewRecorder()
		h.postOps(rec, r.WithContext(tenancy.Into(r.Context(), acme)))
		if rec.Code != http.StatusOK {
			t.Fatalf("ops = %d", rec.Code)
		}
	}
	quiero := []string{"acme sheets ana", "acme sheets owner"}
	for limite := time.Now().Add(5 * time.Second); !slices.Equal(recibido(), quiero); {
		if time.Now().After(limite) {
			t.Fatalf("llega %v, quiero %v", recibido(), quiero)
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	if got := recibido(); !slices.Equal(got, quiero) {
		t.Fatalf("cinco lotes seguidos mandan %v; quiero una sola vez %v", got, quiero)
	}
}
