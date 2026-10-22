// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compartir

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

type sesionDoble struct{}

func (sesionDoble) AccessToken(context.Context) (string, error) { return "tok", nil }
func (sesionDoble) Invalidate(string)                           {}

// servicio hace de Sytue ID y del servicio del recurso, y apunta las escrituras.
type servicio struct {
	mu         sync.Mutex
	escrituras []string
}

func (s *servicio) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/albums/a1/people":
			_, _ = io.WriteString(w, `{"people":[{"subject":"sub-luis","permission":"read"}]}`)
			return
		case "POST /api/v1/directory:resolve":
			switch {
			case strings.Contains(string(cuerpo), "nadie"):
				w.WriteHeader(http.StatusNotFound)
			case strings.Contains(string(cuerpo), "luis"):
				_, _ = io.WriteString(w, `{"user_id":"sub-luis","email":"luis@acme.test"}`)
			default:
				_, _ = io.WriteString(w, `{"user_id":"sub-ana","email":"ana@acme.test","name":"Ana"}`)
			}
			return
		}
		s.mu.Lock()
		s.escrituras = append(s.escrituras, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(cuerpo)))
		s.mu.Unlock()
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, `{"subject":"sub-ana","permission":"write","created":true}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func abrir(t *testing.T, niveles ...string) (*Dialogo, *servicio, *[]string) {
	t.Helper()
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	s := &servicio{}
	srv := s.servidor(t)
	w := test.NewWindow(container.NewStack())
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	var avisos []string
	enLinea := func(f func()) { f() }
	d := Abrir(w.Canvas(), Opciones{
		T: tr, V: tema.Claro, Nombre: "Verano", Niveles: niveles,
		Personas:   api.NuevasPersonas(api.New(srv.URL+"/api/v1", sesionDoble{}, nil), "/albums/a1/people"),
		Directorio: api.NewDirectorio(srv.URL, sesionDoble{}, nil),
		Avisar:     func(_ bool, texto string) { avisos = append(avisos, texto) },
		Lanzar:     enLinea, EnHilo: enLinea,
	})
	return d, s, &avisos
}

// La lista dice con quién está compartido (con la cara que da el directorio); un correo
// de fuera se queda escrito junto al campo y no llega al servicio; compartir manda el
// `sub` que da Sytue ID con el nivel elegido; y revocar lo quita de la lista.
func TestCompartirConUnaPersonaYRevocar(t *testing.T) {
	d, s, avisos := abrir(t, api.NivelVer, api.NivelEditar)
	if len(d.Personas()) != 1 || d.nombre(d.Personas()[0]) != "luis@acme.test" {
		t.Fatalf("la lista no llega o no resuelve a la persona: %+v %+v", d.Personas(), d.caras)
	}

	d.correo.PonerTexto("nadie@fuera.test")
	d.Compartir()
	if d.errorCorreo != d.t("sharePeople.unknown") {
		t.Errorf("un correo de fuera deja %q junto al campo", d.errorCorreo)
	}

	d.correo.PonerTexto("ana@acme.test")
	d.nivel.Elegida = 1
	d.Compartir()
	if len(d.Personas()) != 2 || d.Personas()[1] != (api.PersonaCompartida{Sub: "sub-ana", Permiso: api.NivelEditar}) {
		t.Fatalf("tras compartir, la lista es %+v", d.Personas())
	}
	d.Quitar(d.Personas()[0])
	if len(d.Personas()) != 1 || d.Personas()[0].Sub != "sub-ana" {
		t.Fatalf("tras revocar, la lista es %+v", d.Personas())
	}
	quiero := []string{
		`PUT /api/v1/albums/a1/people/sub-ana {"permission":"write"}`,
		`DELETE /api/v1/albums/a1/people/sub-luis`,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.Equal(s.escrituras, quiero) {
		t.Errorf("al servicio ha llegado\n%s\ny se esperaba\n%s", strings.Join(s.escrituras, "\n"), strings.Join(quiero, "\n"))
	}
	if len(*avisos) != 2 {
		t.Errorf("avisos: %q", *avisos)
	}
}

// Con un solo nivel (un álbum se comparte para ver) no se pregunta: la fila es el correo
// y el botón, como la web con `niveles={['read']}`.
func TestConUnSoloNivelNoSePregunta(t *testing.T) {
	d, _, _ := abrir(t, api.NivelVer)
	fila := d.cuerpo.Objects[0].(*fyne.Container).Objects[1].(*fyne.Container)
	if n := len(fila.Objects); n != 2 {
		t.Errorf("la fila del formulario lleva %d piezas y con un nivel son 2 (correo y botón)", n)
	}
}
