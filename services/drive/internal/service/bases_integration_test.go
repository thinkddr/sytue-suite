// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

const tokenDeBases = "sbc_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// Exportar una vista de Sytue Bases a una carpeta, contra un Bases de mentira que exige lo
// que el de verdad: token, consumidor «drive» y la organización de quien pide.
func TestExportarVistaDeBases(t *testing.T) {
	var revocada atomic.Bool
	var lecturas atomic.Int32
	bases := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lecturas.Add(1)
		if revocada.Load() || r.Header.Get("Authorization") != "Bearer "+tokenDeBases ||
			r.Header.Get("X-Sytue-Consumer") != "drive" || r.Header.Get("X-Sytue-Tenant") != "acme" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"columns":[{"name":"Cliente","type":"text"},{"name":"Importe","type":"number"}],
			"rows":[["=cmd|' /C calc'!A0",-120.5],["Bea",85]],
			"provenance":{"source":"sytue-bases","consumer":"drive","connection_id":"c1","version":2,
			 "base_name":"Tienda","table_name":"Pedi/dos","view_name":"../Grandes\u0007","granted_by":"ana",
			 "granted_at":"2026-10-01T10:00:00Z","expires_at":"2026-12-30T10:00:00Z","read_at":"2026-10-05T08:00:00Z"}}`))
	}))
	defer bases.Close()

	svc, _, blobs, ctx := newChannelService(t)
	ctx = store.WithOwner(ctx, "bob")
	svc.cfg.BasesURL = bases.URL
	root, err := svc.Root(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Lo que no es un token de conexión no sale hacia Bases; lo que Bases rechaza, tampoco entra.
	for _, malo := range []string{"", "eyJ.un.jwt", "sbc_corto"} {
		if _, _, err := svc.ExportarVistaDeBases(ctx, root.ID, malo, "bob"); !errors.Is(err, ErrConexionBases) {
			t.Errorf("token %q: %v", malo, err)
		}
	}
	if lecturas.Load() != 0 {
		t.Error("un secreto que no es token de conexión salió hacia Sytue Bases")
	}
	// Otra organización con el mismo token: Bases la rechaza (el de mentira, como el de verdad).
	if _, _, err := svc.ExportarVistaDeBases(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "globex"}), root.ID, tokenDeBases, "gus"); !errors.Is(err, ErrConexionBases) {
		t.Errorf("otra organización: %v", err)
	}

	f, p, err := svc.ExportarVistaDeBases(ctx, root.ID, tokenDeBases, "bob")
	if err != nil {
		t.Fatal(err)
	}
	// El nombre sale de la vista, saneado: sin separadores de ruta ni caracteres de control.
	if f.Name != "Pedi dos - .. Grandes.csv" || !domain.ValidName(f.Name) || f.OwnerSub != "bob" || f.FolderID != root.ID {
		t.Errorf("fichero: %+v", f)
	}
	// El CSV neutraliza la fórmula del texto y deja el número negativo como número.
	if got := string(blobs.putBody); got != "Cliente,Importe\n'=cmd|' /C calc'!A0,-120.5\nBea,85\n" {
		t.Errorf("CSV subido:\n%q", got)
	}
	if p.Source != "sytue-bases" || p.Version != f.Version || strings.Contains(string(p.Detail), tokenDeBases) ||
		!strings.Contains(string(p.Detail), `"granted_by":"ana"`) || !strings.Contains(string(p.Detail), `"base_name":"Tienda"`) {
		t.Errorf("procedencia: %+v %s", p, p.Detail)
	}

	// Reexportar es una versión nueva del mismo fichero, y cada una guarda de dónde vino.
	f2, _, err := svc.ExportarVistaDeBases(ctx, root.ID, tokenDeBases, "bob")
	if err != nil || f2.ID != f.ID || f2.Version != f.Version+1 {
		t.Fatalf("reexportar: %+v %v", f2, err)
	}
	ps, err := svc.ProcedenciaDeFichero(ctx, f.ID)
	if err != nil || len(ps) != 2 || ps[0].Version != f2.Version || ps[0].CreatedBy != "bob" {
		t.Errorf("procedencia del fichero: %+v %v", ps, err)
	}
	// La procedencia no cruza organizaciones, ni existe para un fichero que no existe.
	if _, err := svc.ProcedenciaDeFichero(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "globex"}), f.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("procedencia desde otra organización: %v", err)
	}
	if _, err := svc.ProcedenciaDeFichero(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("procedencia de un fichero inexistente: %v", err)
	}

	// Revocar la conexión en Bases impide volver a exportar; lo ya exportado se queda.
	revocada.Store(true)
	if _, _, err := svc.ExportarVistaDeBases(ctx, root.ID, tokenDeBases, "bob"); !errors.Is(err, ErrConexionBases) {
		t.Errorf("tras revocar: %v", err)
	}
	if ps, _ := svc.ProcedenciaDeFichero(ctx, f.ID); len(ps) != 2 {
		t.Errorf("revocar cambió lo ya exportado: %d", len(ps))
	}
}
