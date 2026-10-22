// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syphotos/db"
	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
	"git.sytue.eu/sytue/one/services/syphotos/internal/drive"
	"git.sytue.eu/sytue/one/services/syphotos/internal/service"
	"git.sytue.eu/sytue/one/services/syphotos/internal/store"
)

// driveFalso contesta como Drive: snake_case, 404 para lo que no conoce y 500 para
// «roto».
func driveFalso(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/api/v1/files/") {
		case "d-roto", "d-papelera":
			_, _ = w.Write([]byte(`{"id":"x","name":"x.png","content_type":"image/png","size_bytes":4321}`))
		case "d-caido":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sinMetadatos(driveID string) domain.Activo {
	return domain.Activo{Duenyo: "u1", DriveID: driveID, Nombre: driveID + ".png", Ancho: 10, Alto: 10, Orientacion: 1}
}

func TestCompletarMetadatos_rellenaSinSacarDeLaPapelera(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "t1"})

	roto, err := st.Ingerir(ctx, sinMetadatos("d-roto"))
	if err != nil {
		t.Fatal(err)
	}
	borrada, err := st.Ingerir(ctx, sinMetadatos("d-papelera"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ALaPapelera(ctx, borrada.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Ingerir(ctx, sinMetadatos("d-ya-no-esta")); err != nil {
		t.Fatal(err)
	}
	bien := sinMetadatos("d-bien")
	bien.Tipo, bien.Bytes = "image/jpeg", 99
	if _, err := st.Ingerir(ctx, bien); err != nil {
		t.Fatal(err)
	}

	in := service.NuevaIngesta(st, drive.Nuevo(driveFalso(t).URL, 5*time.Second), nil, 0, nil)
	c, err := in.CompletarMetadatos(ctx, "tok", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if c.Arreglados != 2 || c.SinOriginal != 1 {
		t.Errorf("pasada = %+v, se esperaban 2 arregladas y 1 sin original", c)
	}

	a, err := st.Activo(ctx, roto.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if a.Tipo != "image/png" || a.Bytes != 4321 {
		t.Errorf("la foto rota quedó con tipo %q y %d bytes", a.Tipo, a.Bytes)
	}
	// Reparar no restaura: `Ingerir` sí lo haría, y por eso no se usa.
	if _, err := st.Activo(ctx, borrada.ID, "u1"); err == nil {
		t.Error("completar los metadatos sacó una foto de la papelera")
	}
	papelera, err := st.Papelera(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(papelera) != 1 || papelera[0].Tipo != "image/png" || papelera[0].Bytes != 4321 {
		t.Errorf("papelera = %+v", papelera)
	}

	// Una segunda pasada no toca nada: solo queda la que no tiene original.
	c, err = in.CompletarMetadatos(ctx, "tok", "u1")
	if err != nil || c.Arreglados != 0 || c.SinOriginal != 1 {
		t.Errorf("segunda pasada = %+v, %v", c, err)
	}
	// Y lo de otra persona no se ve.
	c, err = in.CompletarMetadatos(ctx, "tok", "u2")
	if err != nil || c != (service.Completado{}) {
		t.Errorf("pasada de otra persona = %+v, %v", c, err)
	}
}

// Con Drive caído o el token caducado fallarían todas: la pasada se para en la
// primera en vez de martillear.
func TestCompletarMetadatos_seParaAnteUnErrorDeDrive(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "t1"})
	if _, err := st.Ingerir(ctx, sinMetadatos("d-caido")); err != nil {
		t.Fatal(err)
	}

	in := service.NuevaIngesta(st, drive.Nuevo(driveFalso(t).URL, 5*time.Second), nil, 0, nil)
	if _, err := in.CompletarMetadatos(ctx, "tok", "u1"); err == nil {
		t.Error("un 500 de Drive no paró la pasada")
	}
}
