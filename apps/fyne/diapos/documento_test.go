// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLosEnlacesDeEditarDanElDocumento: el esquema propio y la ruta de la web con su
// prefijo llevan al mismo documento; otro esquema u otra ruta, a ninguno.
func TestLosEnlacesDeEditarDanElDocumento(t *testing.T) {
	casos := map[string]string{
		"sytue-slides:/edit?id=abc":          "abc",
		"sytue-slides:/diapos/edit?id=abc":   "abc",
		"sytue-docs:/edit?id=abc":            "",
		"sytue-slides:/open-from-drive?id=x": "",
	}
	for enlace, quiere := range casos {
		u, _ := url.Parse(enlace)
		if got := IDDeEnlace(u); got != quiere {
			t.Errorf("%s da %q y tocaba %q", enlace, got, quiere)
		}
	}
}

// TestLosErroresDeAbrirSonSuPantalla: sin red, «sin conexión»; un 403, «sin permiso»;
// el resto, «no se pudo abrir».
func TestLosErroresDeAbrirSonSuPantalla(t *testing.T) {
	casos := []struct {
		err    error
		quiere editor.Estado
	}{
		{&api.Error{Status: http.StatusForbidden}, editor.SinPermiso},
		{&api.Error{Status: http.StatusNotFound}, editor.Error},
		{&url.Error{Op: "Get", Err: errors.New("dial tcp: connection refused")}, editor.SinConexion},
		{nil, editor.Listo},
	}
	for _, c := range casos {
		if got := EstadoDe(c.err); got != c.quiere {
			t.Errorf("%v da el estado %v y tocaba %v", c.err, got, c.quiere)
		}
	}
}

// TestAbrirLeeElRolYElContenido contra un servicio de mentira con las respuestas de
// services/docs: un «viewer» abre en solo lectura y el contenido es la cadena de
// dentro del registro.
func TestAbrirLeeElRolYElContenido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/state"):
			_, _ = w.Write([]byte(`{"seq":3,"state":{"lamport":7,"registers":{"doc:slides":` +
				`{"value":"{\"slides\":[]}","dot":{"a":"x","c":1},"lamport":7}}}}`))
		default:
			_, _ = w.Write([]byte(`{"document":{"id":"d1","kind":"presentation","title":"Trimestre"},"role":"viewer"}`))
		}
	}))
	defer srv.Close()
	ap, err := Abrir(context.Background(), api.NewSlides(srv.URL, nil, srv.Client()), "d1")
	if err != nil {
		t.Fatal(err)
	}
	if ap.Titulo != "Trimestre" || ap.Escribir || ap.Contenido != `{"slides":[]}` {
		t.Errorf("abrir da título %q, escribir %v, contenido %q", ap.Titulo, ap.Escribir, ap.Contenido)
	}
}
