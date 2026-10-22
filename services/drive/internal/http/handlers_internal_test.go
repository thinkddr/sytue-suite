// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests de la puerta interna de la cuota compartida (PUT /internal/quota/external/{app})
// a través del router COMPLETO, para que el secreto se compruebe donde se comprueba
// —en el middleware— y no en un handler llamado a mano. Sin PostgreSQL: un fake del
// store recuerda lo que le llega.
package drivehttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
)

type fakeExternalUsage struct {
	err    error
	calls  int
	tenant string
	app    string
	bytes  int64
}

func (f *fakeExternalUsage) SetExternalUsage(ctx context.Context, app string, bytes int64) error {
	f.calls++
	f.tenant = tenancy.MustFrom(ctx).Slug
	f.app, f.bytes = app, bytes
	return f.err
}

func internalRouter(secreto string, st externalUsageStore) http.Handler {
	return NewRouter(Deps{Cfg: config.Config{PublicURL: "https://drive.sytue.eu", CastInternalSecret: secreto}, ExternalUsage: st})
}

func declarar(r http.Handler, bearer, app, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/internal/quota/external/"+app, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestExternalUsageOK(t *testing.T) {
	f := &fakeExternalUsage{}
	rec := declarar(internalRouter("s3creto", f), "s3creto", "cast", `{"tenant":"acme","bytes":300}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("código = %d, quiero 204: %s", rec.Code, rec.Body)
	}
	if f.calls != 1 || f.tenant != "acme" || f.app != "cast" || f.bytes != 300 {
		t.Fatalf("el store recibió %+v", f)
	}
	// Cero es un valor: la organización ha borrado todos sus vídeos.
	if rec := declarar(internalRouter("s3creto", f), "s3creto", "cast", `{"tenant":"acme","bytes":0}`); rec.Code != http.StatusNoContent || f.bytes != 0 {
		t.Fatalf("bytes=0 => %d (%d): tiene que aceptarse", rec.Code, f.bytes)
	}
}

// El secreto ES la autorización: sin él, con otro, o con el servicio sin secreto
// configurado, nada pasa y el store no se toca.
func TestExternalUsageSecreto(t *testing.T) {
	casos := []struct {
		nombre, configurado, dado string
	}{
		{"sin cabecera", "s3creto", ""},
		{"secreto equivocado", "s3creto", "otro"},
		{"secreto vacío en el servicio y vacío en la petición", "", ""},
		{"secreto vacío en el servicio y algo en la petición", "", "s3creto"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := &fakeExternalUsage{}
			rec := declarar(internalRouter(c.configurado, f), c.dado, "cast", `{"tenant":"acme","bytes":300}`)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("código = %d, quiero 401", rec.Code)
			}
			if f.calls != 0 {
				t.Fatal("con el secreto rechazado el store no debe tocarse")
			}
		})
	}
}

// Entrada mala: 400 y el store no se toca. La app es un conjunto cerrado; el tenant,
// un slug; los bytes, un total (no negativo).
func TestExternalUsageEntradaInvalida(t *testing.T) {
	casos := []struct{ nombre, app, body string }{
		{"app desconocida", "photos", `{"tenant":"acme","bytes":1}`},
		{"tenant inválido", "cast", `{"tenant":"Acme Corp","bytes":1}`},
		{"tenant vacío", "cast", `{"bytes":1}`},
		{"bytes negativos", "cast", `{"tenant":"acme","bytes":-1}`},
		{"JSON roto", "cast", `{"tenant":`},
		{"campo desconocido", "cast", `{"tenant":"acme","bytes":1,"delta":true}`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := &fakeExternalUsage{}
			rec := declarar(internalRouter("s3creto", f), "s3creto", c.app, c.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("código = %d, quiero 400: %s", rec.Code, rec.Body)
			}
			if f.calls != 0 {
				t.Fatal("con la entrada rechazada el store no debe tocarse")
			}
		})
	}
}

func TestExternalUsageErrorDelStore(t *testing.T) {
	f := &fakeExternalUsage{err: errors.New("postgres caído")}
	rec := declarar(internalRouter("s3creto", f), "s3creto", "cast", `{"tenant":"acme","bytes":300}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("código = %d, quiero 500", rec.Code)
	}
}

// Sin store la puerta no existe: 404, como el resto de planos opcionales de Drive.
// Y con store, la ruta está registrada con el método y el patrón del contrato.
func TestExternalUsageMontaje(t *testing.T) {
	if rec := declarar(internalRouter("s3creto", nil), "s3creto", "cast", `{"tenant":"acme","bytes":1}`); rec.Code != http.StatusNotFound {
		t.Fatalf("sin store => %d, quiero 404", rec.Code)
	}
	r, ok := internalRouter("s3creto", &fakeExternalUsage{}).(chi.Routes)
	if !ok {
		t.Fatal("NewRouter no devuelve un router de chi")
	}
	registrada := false
	if err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodPut && route == "/internal/quota/external/{app}" {
			registrada = true
		}
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
	if !registrada {
		t.Fatal("falta PUT /internal/quota/external/{app}")
	}
}
