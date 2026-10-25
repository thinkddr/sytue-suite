// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`). El servicio es el de
// las secciones con una raíz que tiene de todo, y contesta vacío a lo demás: al pulsar
// se piden cosas que ningún otro test pide, y eso no es un fallo de la app.
func TestRecorridoConSesion(t *testing.T) {
	secciones := servicioSecciones(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/folders/raiz/children":
			_, _ = io.WriteString(w, `{"folders":[{"id":"c1","name":"Actas","is_directory":true,"path":"/Actas"}],`+
				`"files":[{"id":"f1","name":"acta.md","path":"/acta.md","size_bytes":10},{"id":"f2","name":"b.pdf","path":"/b.pdf","size_bytes":2048}]}`)
		case "GET /api/v1/folders/root", "GET /api/v1/favorites", "GET /api/v1/recent",
			"GET /api/v1/shared-with-me", "GET /api/v1/documents":
			secciones.Config.Handler.ServeHTTP(w, r)
		case "GET /api/v1/quota":
			_, _ = io.WriteString(w, `{"used_bytes":1048576,"quota_bytes":5368709120}`)
		case "GET /api/v1/trash", "GET /api/v1/team-drives", "GET /api/v1/storage/external":
			_, _ = io.WriteString(w, `{"items":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	p := pantallaDePrueba(t, srv)
	p.win.SetContent(p.Objeto())
	p.Arrancar(true)
	montaje.ComprobarRotacion(t, p.win, "drive", iconos.Drive, true)
	recorrido.Comprobar(t, p.win, 400)
}
