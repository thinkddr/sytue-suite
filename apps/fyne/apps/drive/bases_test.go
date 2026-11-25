// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// TestNuevoLlevaLaVistaDeBases: «Nuevo» trae, tras su separador, «Vista de Sytue Bases»,
// el último como en el `Dropdown` de FileBrowser.tsx.
func TestNuevoLlevaLaVistaDeBases(t *testing.T) {
	p := NuevaPantalla(traductor(t), nil, nil, nil)
	var filas []string
	for _, o := range p.elementosNuevo() {
		switch x := o.(type) {
		case *ui.ElementoMenu:
			filas = append(filas, x.Texto)
		case *ui.SeparadorMenu:
			filas = append(filas, "—")
		}
	}
	t1 := func(k string) string { return p.tr.T(k, nil) }
	quiero := []string{
		t1("files.upload"), t1("files.new.folder"), "—",
		t1("files.new.document"), t1("files.new.spreadsheet"), t1("files.new.presentation"), "—",
		t1("files.new.bases"),
	}
	if !slices.Equal(filas, quiero) {
		t.Errorf("«Nuevo» trae %q y la web trae %q", filas, quiero)
	}
}

// servicioBases contesta la exportación según el token, y apunta lo que le llega.
type servicioBases struct {
	mu      sync.Mutex
	cuerpos []string
	rutas   []string
}

func (s *servicioBases) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
			return
		case "GET /api/v1/folders/raiz/children", "GET /api/v1/folders/c1/children":
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.rutas = append(s.rutas, r.Method+" "+r.URL.Path)
		s.cuerpos = append(s.cuerpos, string(b))
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(string(b), "revocado"):
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"code":"bases_connection_invalid","message":"no"}`)
		case strings.Contains(string(b), "lleno"):
			w.WriteHeader(http.StatusInsufficientStorage)
			_, _ = io.WriteString(w, `{"code":"quota_exceeded","message":"no"}`)
		case strings.Contains(string(b), "roto"):
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"file":{"id":"f9","folder_id":"c1","name":"Clientes.csv"},"provenance":{}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *servicioBases) lista() ([]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.rutas), slices.Clone(s.cuerpos)
}

// ultimoAviso deja antes que el aviso llegue a la pila: `avisar` lo encola con `fyne.Do`.
func ultimoAviso(p *Pantalla) *ui.Aviso {
	esperar()
	a := p.avisos.Avisos()
	if len(a) == 0 {
		return &ui.Aviso{}
	}
	return a[len(a)-1]
}

// TestLaVistaDeBasesSeExportaALaCarpeta: el token va en el cuerpo a la carpeta abierta
// (la raíz si no hay ninguna) y se avisa con el nombre del CSV; un 422 dice que la
// conexión no vale, un 507 que no cabe, y lo demás que no se pudo, como la web.
func TestLaVistaDeBasesSeExportaALaCarpeta(t *testing.T) {
	s := &servicioBases{}
	srv := s.servidor(t)
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)

	p.exportarVistaDeBases("c1", "sbc_123")
	a := ultimoAviso(p)
	if a.Tono != ui.AvisoExito || a.Titulo != p.tr.T("files.bases.done", i18n.Vars{"name": "Clientes.csv"}) {
		t.Errorf("exportar avisa %+v", a)
	}
	p.exportarVistaDeBases("", "sbc_456")
	rutas, cuerpos := s.lista()
	if !slices.Equal(rutas, []string{"POST /api/v1/folders/c1/imports/bases", "POST /api/v1/folders/raiz/imports/bases"}) {
		t.Errorf("al servicio ha llegado %q", rutas)
	}
	if len(cuerpos) != 2 || cuerpos[0] != `{"token":"sbc_123"}` {
		t.Errorf("el token tiene que ir en el cuerpo, y va %q", cuerpos)
	}

	for token, clave := range map[string]string{
		"revocado": "files.bases.invalid",
		"lleno":    "files.bases.quota",
		"roto":     "files.bases.failed",
	} {
		// Una pantalla por caso: la pila enseña tres avisos y encola el resto.
		p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)
		p.exportarVistaDeBases("c1", token)
		if a := ultimoAviso(p); a.Tono != ui.AvisoPeligro || a.Titulo != p.tr.T(clave, nil) {
			t.Errorf("con %q avisa %q y tenía que avisar %q", token, a.Titulo, p.tr.T(clave, nil))
		}
	}
}

// TestLaVistaDeBasesPideElTokenEnSecreto: el elemento del menú abre la pregunta con su
// título, la frase de la web por etiqueta y el campo de contraseña; aceptar exporta a la
// carpeta abierta.
func TestLaVistaDeBasesPideElTokenEnSecreto(t *testing.T) {
	s := &servicioBases{}
	srv := s.servidor(t)
	w := test.NewWindow(nil)
	defer w.Close()
	w.Resize(fyne.NewSize(1280, 800))
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, w)
	w.SetContent(p.Objeto())

	p.pedirVistaDeBases()
	dialogo := w.Canvas().Overlays().Top()
	if dialogo == nil {
		t.Fatal("«Vista de Sytue Bases» no abre ningún diálogo")
	}
	if !contieneTexto(dialogo, p.tr.T("files.new.bases", nil)) {
		t.Error("el diálogo no lleva el título «Vista de Sytue Bases»")
	}
	campo, ok := w.Canvas().Focused().(*ui.Entrada)
	if !ok {
		t.Fatalf("el foco ha caído en %T y tiene que caer en el campo del token", w.Canvas().Focused())
	}
	if !campo.EsContrasena() {
		t.Error("el token se ve mientras se escribe")
	}
	campo.PonerTexto("  sbc_789  ")
	campo.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	limite := time.Now().Add(5 * time.Second)
	for time.Now().Before(limite) {
		if _, c := s.lista(); len(c) > 0 {
			break
		}
		esperar()
	}
	if _, c := s.lista(); !slices.Equal(c, []string{`{"token":"sbc_789"}`}) {
		t.Errorf("aceptar ha mandado %q", c)
	}
}
