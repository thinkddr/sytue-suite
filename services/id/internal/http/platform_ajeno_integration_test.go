// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

// Quien administra SU organización no administra la plataforma.
//
// Es la frontera que separa a un cliente de quien opera el producto: el owner de
// una organización manda sobre sus usuarios, sus grupos y su facturación, y no
// tiene por qué poder listar las demás organizaciones, suspenderlas ni regalarles
// plazas. Sin esta prueba, un cambio en `requirePlatformAdmin` que confundiera «rol
// de organización» con «alcance de plataforma» pasaría el resto de la suite entera
// sin que nadie lo notara.
package http

import (
	"net/http"
	"strings"
	"testing"
)

// TestPlatformAjeno_UnAdminDeOrganizacionNoEntra comprueba las dos mitades del
// requisito, y la segunda es la que se olvida.
//
// La primera es obvia: el owner de una organización cliente recibe 403 en la API de
// plataforma. La segunda es que ese 403 tiene que venir del PRIMER candado —«no
// estás en la allowlist»— y no de los otros dos.
//
// Importa porque los tres candados devuelven 403 y a simple vista son iguales. Si
// alguien acabara en `activation_required`, el test seguiría en verde mientras
// significa lo contrario de lo que prueba: que ese owner SÍ pasó la allowlist y
// solo le falta elevarse. Es decir, que sí es admin de plataforma.
func TestPlatformAjeno_UnAdminDeOrganizacionNoEntra(t *testing.T) {
	l := newLiveServer(t)

	// Un usuario normal, dueño de su organización, que NUNCA se añade a
	// `platform_admins`. Con MFA en el token: así lo que se prueba es el alcance,
	// no la falta de segundo factor.
	ajeno := l.mkUser(t, "dueño-de-su-org@cliente.test", "")
	conMFA := []string{"pwd", "mfa"}

	// Se recorren varias rutas a propósito: el gate cuelga del grupo, y una ruta
	// que quedara fuera del grupo por descuido no daría 403 aquí aunque el
	// middleware funcionara.
	rutas := []struct {
		metodo string
		ruta   string
		cuerpo string
	}{
		{http.MethodGet, "/api/v1/platform/orgs", ""},
		{http.MethodGet, "/api/v1/platform/usage", ""},
		{http.MethodGet, "/api/v1/platform/admins", ""},
		{http.MethodGet, "/api/v1/platform/audit", ""},
		{
			http.MethodPost, "/api/v1/platform/orgs/00000000-0000-0000-0000-000000000000/seat-grants",
			`{"seats":5,"kind":"goodwill","reason":"prueba"}`,
		},
	}

	for _, r := range rutas {
		rec := l.pimReq(t, r.metodo, r.ruta, ajeno.ID, conMFA, r.cuerpo)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: código=%d, quiero 403 (cuerpo=%s)", r.metodo, r.ruta, rec.Code, rec.Body)
			continue
		}
		// Y del candado correcto. `activation_required` significaría que este
		// usuario está en la allowlist y solo le falta elevarse, que es justo lo
		// contrario de lo que se quiere probar.
		if strings.Contains(rec.Body.String(), "activation_required") {
			t.Errorf("%s %s: el 403 dice «te falta elevarte», o sea que este usuario SÍ pasó la allowlist: %s",
				r.metodo, r.ruta, rec.Body)
		}
		// `mfa_required` tampoco vale: el token ya lleva MFA, así que ese código
		// señalaría un fallo en el orden de los candados.
		if strings.Contains(rec.Body.String(), "mfa_required") {
			t.Errorf("%s %s: el 403 pide MFA y el token ya la lleva: %s", r.metodo, r.ruta, rec.Body)
		}
	}
}

// Y sin segundo factor tampoco, aunque estuviera en la allowlist.
//
// Es el orden de los candados lo que se fija aquí: la administración de plataforma
// exige MFA SIEMPRE, tenga o no rol de organización quien llama.
func TestPlatformAjeno_SinSegundoFactorTampocoEntra(t *testing.T) {
	l := newLiveServer(t)
	ajeno := l.mkUser(t, "sin-mfa@cliente.test", "")

	rec := l.pimReq(t, http.MethodGet, "/api/v1/platform/orgs", ajeno.ID, []string{"pwd"}, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("código=%d, quiero 403 (cuerpo=%s)", rec.Code, rec.Body)
	}
}
