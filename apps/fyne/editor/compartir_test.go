// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type portapapelesFalso struct{ s string }

func (p *portapapelesFalso) Content() string     { return p.s }
func (p *portapapelesFalso) SetContent(s string) { p.s = s }

// servicioCompartir contesta como `/permissions` y `/shares` del servicio.
type servicioCompartir struct {
	mu        sync.Mutex
	prohibido bool
	permisos  string
	vistos    []string
}

func (s *servicioCompartir) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cuerpo, _ := io.ReadAll(r.Body)
	s.vistos = append(s.vistos, r.Method+" "+r.URL.Path+" "+string(cuerpo))
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/documents/d1/permissions":
		if s.prohibido {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, `{"permissions":[`+s.permisos+`]}`)
	case "GET /api/v1/documents/d1/shares":
		_, _ = io.WriteString(w, `{"shares":[]}`)
	case "POST /api/v1/documents/d1/shares":
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"share":{"id":"s1","role":"viewer","token":"abc"}}`)
	case "POST /api/v1/directory:resolve":
		// El directorio de Sytue ID: luis y ana son de la organización; nadie más.
		switch {
		case strings.Contains(string(cuerpo), "luis@x.eu"):
			_, _ = io.WriteString(w, `{"user_id":"sub-luis","email":"luis@x.eu","name":"Luis"}`)
		case strings.Contains(string(cuerpo), "sub-ana"):
			_, _ = io.WriteString(w, `{"user_id":"sub-ana","email":"ana@x.eu","name":"Ana Pérez"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func compartirDePrueba(t *testing.T, s *servicioCompartir) (*Compartir, *portapapelesFalso) {
	t.Helper()
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	pp := &portapapelesFalso{}
	c := &Compartir{
		Svc: api.NewDocs(srv.URL, nil, srv.Client()), ID: "d1", Titulo: "Acta",
		Enlace: "https://docus.sytue.eu/docus/edit?id=d1", EnlacePublico: func(tok string) string { return "https://docus.sytue.eu/docus/s/" + tok },
		Portapapeles: pp, Lanzar: func(f func()) { f() }, Avisos: ui.NuevaPilaAvisos(),
		Dir: api.NewDirectorio(srv.URL, nil, srv.Client()),
	}
	return c, pp
}

// TestGestionarAcceso: el diálogo pide quién tiene acceso, «Toda mi organización» da
// acceso de lector al tenant, al propietario no se le ofrece quitarlo y crear un enlace
// lo copia. Y quien no es propietario ve la frase de la web, no un diálogo vacío.
func TestGestionarAcceso(t *testing.T) {
	s := &servicioCompartir{permisos: `{"principal_kind":"user","principal_id":"sub-ana","role":"owner"}`}
	c, pp := compartirDePrueba(t, s)
	lienzo := software.NewCanvas()
	lienzo.Resize(fyne.NewSize(1280, 800))
	c.Gestionar(lienzo)
	if c.cargando || c.denegado || len(c.permisos) != 1 {
		t.Fatalf("tras cargar: cargando %v, denegado %v, %d permisos", c.cargando, c.denegado, len(c.permisos))
	}
	if c.toda() {
		t.Error("sin permiso de tenant, «Solo las personas que yo elija» es la elegida")
	}
	c.alcance(true)
	c.crearEnlace()
	if pp.s != "https://docus.sytue.eu/docus/s/abc" || c.nuevo == nil {
		t.Errorf("crear un enlace lo copia: %q", pp.s)
	}
	c.quien.PonerTexto("  luis@x.eu ")
	c.papel.Elegida = 2
	c.conceder()
	s.mu.Lock()
	vistos := strings.Join(s.vistos, "\n")
	s.mu.Unlock()
	for _, quiero := range []string{
		`PUT /api/v1/documents/d1/permissions {"principal_id":"","principal_kind":"tenant","role":"viewer"}`,
		`POST /api/v1/documents/d1/shares {"role":"viewer"}`,
		// La dirección se guarda por su `sub`: con la dirección tal cual, el acceso no abría nada.
		`PUT /api/v1/documents/d1/permissions {"principal_id":"sub-luis","principal_kind":"user","role":"editor"}`,
	} {
		if !strings.Contains(vistos, quiero) {
			t.Errorf("falta %s en:\n%s", quiero, vistos)
		}
	}
	lienzo.Capture() // un token de color inventado entra en pánico AL PINTAR
	if c.nombres["sub-ana"].Nombre != "Ana Pérez" {
		t.Errorf("con quién está compartido se enseña por su nombre: %+v", c.nombres)
	}
	// Una dirección que no es de nadie de la organización no se concede.
	antes := len(s.vistos)
	c.quien.PonerTexto("extrana@y.eu")
	c.conceder()
	s.mu.Lock()
	for _, v := range s.vistos[antes:] {
		if strings.HasPrefix(v, "PUT ") {
			t.Errorf("se ha concedido a quien no es de la organización: %s", v)
		}
	}
	s.mu.Unlock()

	otro, _ := compartirDePrueba(t, &servicioCompartir{prohibido: true})
	otro.Gestionar(lienzo)
	if !otro.denegado {
		t.Error("con un 403 se dice que solo el propietario puede cambiar el acceso")
	}
}

// TestElPanelCopiaElEnlace: «Copiar enlace» copia la dirección del documento en la web.
func TestElPanelCopiaElEnlace(t *testing.T) {
	c, pp := compartirDePrueba(t, &servicioCompartir{})
	panel := c.Panel(func() {}, func() {})
	var copiar *ui.Boton
	var ir func(o fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == "share.copy" {
			copiar = b
		}
		if k, ok := o.(*fyne.Container); ok {
			for _, h := range k.Objects {
				ir(h)
			}
		}
	}
	ir(panel)
	if copiar == nil {
		t.Fatal("el panel no tiene «Copiar enlace»")
	}
	copiar.Tapped(nil)
	if pp.s != "https://docus.sytue.eu/docus/edit?id=d1" || copiar.Texto != "share.copied" {
		t.Errorf("copiado %q, el botón dice %q", pp.s, copiar.Texto)
	}
}
