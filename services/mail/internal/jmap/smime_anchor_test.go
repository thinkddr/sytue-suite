// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/smime"
)

// --- ancla S/MIME ---

/** Una petición con tenant en contexto, que es lo que inyecta RequireTenant. */
func conTenant(r *http.Request) *http.Request {
	t, err := tenancy.New("acme")
	if err != nil {
		panic(err)
	}
	return r.WithContext(tenancy.Into(r.Context(), t))
}

type anclaFalsa struct {
	pem string
	err error
}

func (a anclaFalsa) AnchorPEM(context.Context, string) (string, error) { return a.pem, a.err }

// Import no lo ejercita ningún test de esta tanda: lo que hay que probar de él
// —qué material se acepta y cuál no— se prueba en el paquete smime, contra
// openssl, que es donde vive la decisión.
func (a anclaFalsa) Import(
	context.Context, string, uuid.UUID, string, string, string,
) (*smime.Identity, error) {
	return nil, a.err
}

// Sin S/MIME configurado la ruta EXISTE y dice que no está disponible: un
// despliegue sin firma no es un error, y un 404 haría pensar que falta la ruta.
func TestSMIMEAnchor_sinServicio(t *testing.T) {
	a := &API{}
	rec := httptest.NewRecorder()
	a.getSMIMEAnchor(rec, conTenant(httptest.NewRequest(http.MethodGet, "/api/v1/smime/anchor", http.NoBody)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d, se esperaba 503", rec.Code)
	}
}

// El ancla sale como fichero PEM: se abre con el llavero del sistema, que es
// quien la instala. En JSON habría que extraerla a mano.
func TestSMIMEAnchor_sirveElPEMComoFichero(t *testing.T) {
	const pem = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	a := &API{smime: anclaFalsa{pem: pem}}
	rec := httptest.NewRecorder()
	a.getSMIMEAnchor(rec, conTenant(httptest.NewRequest(http.MethodGet, "/api/v1/smime/anchor", http.NoBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/x-pem-file" {
		t.Fatalf("content-type: %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "sytue-smime-ca.pem") {
		t.Fatalf("content-disposition: %q", got)
	}
	if rec.Body.String() != pem {
		t.Fatalf("cuerpo: %q", rec.Body.String())
	}
}

// Una organización que aún no ha firmado nada no tiene autoridad: 404, no 500.
func TestSMIMEAnchor_sinMaterial(t *testing.T) {
	a := &API{smime: anclaFalsa{err: smime.ErrNoMaterial}}
	rec := httptest.NewRecorder()
	a.getSMIMEAnchor(rec, conTenant(httptest.NewRequest(http.MethodGet, "/api/v1/smime/anchor", http.NoBody)))
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("un tenant sin autoridad no es un error del servidor: code=%d", rec.Code)
	}
}

// --- ancla pública ---

// conOrg pone el parámetro de ruta {org}, que en producción inyecta chi.
func conOrg(r *http.Request, org string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add("org", org)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

func pedirPublico(a *API, org string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	// La ruta es fija: el handler lee la organización del parámetro de chi, no
	// de la URL, y así el caso "con espacio" no rompe httptest.NewRequest.
	req := httptest.NewRequest(http.MethodGet, "/.well-known/smime/x.pem", http.NoBody)
	a.getSMIMEAnchorPublico(rec, conOrg(req, org))
	return rec
}

// Sin sesión y desde fuera: es el caso para el que existe.
func TestSMIMEAnchorPublico_sirveSinSesion(t *testing.T) {
	const pem = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	rec := pedirPublico(&API{smime: anclaFalsa{pem: pem}}, "acme")
	if rec.Code != http.StatusOK || rec.Body.String() != pem {
		t.Fatalf("code=%d cuerpo=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "public") {
		t.Fatalf("debería ser cacheable por quien lo pide de fuera: %q", got)
	}
}

// NO SE PUEDE ENUMERAR: los cuatro caminos que no sirven certificado contestan
// exactamente lo mismo. Distinguirlos diría qué organizaciones existen.
func TestSMIMEAnchorPublico_noDistingueLosMotivos(t *testing.T) {
	casos := map[string]*API{
		"sin servicio":      {},
		"sin autoridad":     {smime: anclaFalsa{err: smime.ErrNoMaterial}},
		"error del almacén": {smime: anclaFalsa{err: errors.New("boom")}},
		"certificado vacío": {smime: anclaFalsa{pem: ""}},
	}
	cuerpos := make([]string, 0, len(casos))
	for nombre, a := range casos {
		rec := pedirPublico(a, "acme")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: code=%d, se esperaba 404 en todos", nombre, rec.Code)
		}
		cuerpos = append(cuerpos, rec.Body.String())
	}
	for _, c := range cuerpos[1:] {
		if c != cuerpos[0] {
			t.Fatal("los motivos se distinguen por el cuerpo: eso permite enumerar organizaciones")
		}
	}
}

// Un slug inválido ni llega a consultar el almacén.
func TestSMIMEAnchorPublico_slugInvalido(t *testing.T) {
	for _, org := range []string{"", "../otra", "MAYUS", "con espacio"} {
		if rec := pedirPublico(&API{smime: anclaFalsa{pem: "x"}}, org); rec.Code != http.StatusNotFound {
			t.Fatalf("org=%q: code=%d", org, rec.Code)
		}
	}
}
