// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"errors"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// errorDelServicio es la respuesta de error tal como la manda `httpx.Fail`.
func errorDelServicio(estado int, cuerpo string) error {
	return &api.Error{Status: estado, Body: cuerpo}
}

// La tabla es la de la web, fila a fila: una que se amplíe sola deja a la otra
// enseñando «algo ha ido mal» donde su gemela ya dice qué pasa.
func TestMensajeDeErrorTablaComoLaWeb(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/erp/src/lib/api.ts")
	ini := strings.Index(web, "export const ERRORES")
	if ini < 0 {
		t.Fatal("api.ts ya no tiene ERRORES: revisar Errores")
	}
	bloque, _, cerrado := strings.Cut(web[ini:], "\n};")
	if !cerrado {
		t.Fatal("ERRORES de api.ts sin cerrar")
	}
	deLaWeb := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^ {2}(\w+): '([^']+)',$`).FindAllStringSubmatch(bloque, -1) {
		deLaWeb[m[1]] = m[2]
	}
	if len(deLaWeb) < 40 {
		t.Fatalf("solo %d filas leídas de la web: ¿cambió el formato?", len(deLaWeb))
	}
	if !maps.Equal(deLaWeb, Errores) {
		for _, c := range slices.Sorted(maps.Keys(deLaWeb)) {
			if Errores[c] != deLaWeb[c] {
				t.Errorf("%s: web %q, Fyne %q", c, deLaWeb[c], Errores[c])
			}
		}
		for c := range Errores {
			if _, ok := deLaWeb[c]; !ok {
				t.Errorf("%s está en Fyne y no en la web", c)
			}
		}
	}
}

// Toda clave de la tabla existe en el catálogo de cada idioma: sin ella, `T` devuelve
// la propia clave y la pantalla diría «error.code.periodClosed».
func TestMensajeDeErrorCatalogos(t *testing.T) {
	claves := slices.Collect(maps.Values(Errores))
	claves = append(claves, "error.code.actionNotAllowed", "error.code.checkFields",
		"error.code.sessionExpired", "error.generic")
	en, err := Traductor("en")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range i18n.Locales {
		tr, err := Traductor(l)
		if err != nil {
			t.Fatal(err)
		}
		for _, clave := range claves {
			if tr.T(clave, nil) == clave {
				t.Errorf("%s: falta %s", l, clave)
			}
		}
		if l == "es" && tr.T("error.code.periodClosed", nil) == en.T("error.code.periodClosed", nil) {
			t.Error("es: error.code.periodClosed sale en inglés")
		}
	}
}

func TestMensajeDeError(t *testing.T) {
	tr := traductor(t)
	casos := []struct {
		nombre string
		err    error
		quiere string
	}{
		{"por su código", errorDelServicio(409, `{"error":{"code":"periodo_cerrado","message":"That accounting period is closed."}}`), "error.code.periodClosed"},
		{"sin permiso", errorDelServicio(403, `{"error":{"code":"sin_permiso","message":"You do not have access."}}`), "error.sinPermiso"},
		{"certificado", errorDelServicio(422, `{"error":{"code":"certificado_invalido","message":"invalid"}}`), "vf.error.invalidCertificate"},
		{"familia sin_verbo_", errorDelServicio(403, `{"error":{"code":"sin_verbo_asentar","message":"Your role…"}}`), "error.code.actionNotAllowed"},
		{"validación con campos", errorDelServicio(422, `{"error":{"code":"unprocessable","message":"fecha: YYYY-MM-DD","details":{"fields":[{"field":"fecha","message":"YYYY-MM-DD"}]}}}`), "error.code.checkFields"},
		{"validación sin campos", errorDelServicio(422, `{"error":{"code":"unprocessable","message":"bad"}}`), "error.code.unprocessable"},
		{"código sin traducir", errorDelServicio(409, `{"error":{"code":"codigo_nuevo","message":"Something new."}}`), "error.generic"},
		{"sesión que no renueva", &api.Error{Status: http.StatusUnauthorized}, "error.code.sessionExpired"},
		{"no es del servicio", errors.New("dial tcp: connection refused"), "error.generic"},
	}
	for _, c := range casos {
		got := MensajeDeError(c.err, tr)
		if want := tr.T(c.quiere, nil); got != want {
			t.Errorf("%s: %q, quería %q", c.nombre, got, want)
		}
		if strings.Contains(got, "Something new") || strings.Contains(got, "accounting period") {
			t.Errorf("%s: enseña el respaldo inglés del servicio: %q", c.nombre, got)
		}
	}
}
