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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioEquipos contesta las unidades de equipo y apunta las escrituras.
type servicioEquipos struct {
	mu         sync.Mutex
	escrituras []string
}

func (s *servicioEquipos) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/team-drives":
			_, _ = io.WriteString(w, `{"items":[{"id":"u1","name":"Ventas","role":"viewer"},{"id":"u2","name":"Legal","role":"manager","archived_at":"2026-09-01T10:00:00Z"}]}`)
			return
		case "GET /api/v1/team-drives/u1/children":
			_, _ = io.WriteString(w, `{"role":"viewer","can_write":false,"folders":[{"id":"c1","name":"Ofertas"}],"files":[{"id":"f1","name":"a.pdf","size_bytes":3}]}`)
			return
		case "GET /api/v1/team-drives/u2/children":
			w.WriteHeader(http.StatusNotFound) // el acceso se revocó mientras se miraba
			return
		case "GET /api/v1/team-drives/u2/members":
			_, _ = io.WriteString(w, `{"members":[{"subject":"ana","role":"manager"}]}`)
			return
		}
		s.mu.Lock()
		s.escrituras = append(s.escrituras, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(cuerpo)))
		s.mu.Unlock()
		if r.URL.Path == "/api/v1/team-drives/u2/members" && strings.Contains(string(cuerpo), "viewer") {
			w.WriteHeader(http.StatusConflict) // se quedaría sin gestor
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestLasUnidadesDeEquipoYaNoSeQuedanCargando: la sección cargaba para siempre. Ahora
// lista las unidades con su rol, entra en una y enseña su contenido, y un lector no ve
// ni borrar ni subir. Un 404 dentro es «sin acceso a esta unidad».
//
// VISTO EN ROJO (2026-09-23, en el frontal): quitando el `case UnidadesDeEquipo` de
// `cargadorDe`, la sección no tiene carga.
func TestLasUnidadesDeEquipoYaNoSeQuedanCargando(t *testing.T) {
	s := &servicioEquipos{}
	p := pantallaDePrueba(t, s.servidor(t))
	q := p.sec.equipos
	carga := p.cargadorDe(UnidadesDeEquipo)
	if carga == nil {
		t.Fatal("las unidades de equipo no tienen carga")
	}
	carga()
	if q.Fase != Exito || len(q.Unidades) != 2 || q.Unidades[1].ArchivedAt == nil {
		t.Fatalf("la lista: %d %+v", q.Fase, q.Unidades)
	}

	lector := q.Unidades[0]
	q.Abierta = &lector
	p.cargarUnidad()
	if q.FaseDetalle != Exito || len(q.Contenido) != 2 || !q.Contenido[0].Carpeta {
		t.Fatalf("el contenido: %d %+v", q.FaseDetalle, q.Contenido)
	}
	for _, el := range q.Contenido {
		for _, b := range q.filaContenido(el, false, tema.Claro).Botones {
			if b.Cuerpo == glifoPapelera {
				t.Errorf("un lector puede borrar «%s»", el.Nombre)
			}
		}
	}

	gestor := q.Unidades[1]
	q.Abierta = &gestor
	p.cargarUnidad()
	if !q.Denegada {
		t.Error("un 404 dentro de la unidad no se lee como acceso retirado")
	}
}

// TestLosMiembrosSeGestionan: añadir con su rol, y el 409 de «se quedaría sin gestor»
// dice eso y no un fallo genérico.
func TestLosMiembrosSeGestionan(t *testing.T) {
	s := &servicioEquipos{}
	p := pantallaDePrueba(t, s.servidor(t))
	q := p.sec.equipos
	q.Abierta = &api.UnidadEquipo{ID: "u2", Name: "Legal", Role: api.RolGestionar}
	dm := p.pedirMiembros()
	if dm == nil || len(dm.miembros) != 1 {
		t.Fatalf("los miembros no llegan: %+v", dm)
	}
	dm.sujeto.PonerTexto("luis@acme.test")
	dm.rol.Elegida = 1
	dm.agregar()
	dm.poner("ana", api.RolVer, func(err error) {
		if codigo(err) != http.StatusConflict {
			t.Errorf("dejar la unidad sin gestor no llega como 409: %v", err)
		}
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	quiero := []string{
		`PUT /api/v1/team-drives/u2/members {"role":"editor","subject":"luis@acme.test"}`,
		`PUT /api/v1/team-drives/u2/members {"role":"viewer","subject":"ana"}`,
	}
	if !slices.Equal(s.escrituras, quiero) {
		t.Errorf("al servicio ha llegado %q", s.escrituras)
	}
}

// TestLasFilasDeEquipoMidenLoDeLaWeb: una unidad de la lista es `py-2.5` alrededor del
// nombre con su meta (22+18) → 60; una fila de contenido, `py-2` y su `border-b` → 57.
func TestLasFilasDeEquipoMidenLoDeLaWeb(t *testing.T) {
	tr := traductor(t)
	q := NuevaVistaEquipos(tr)
	q.Fase, q.Unidades = Exito, []api.UnidadEquipo{{ID: "u1", Name: "Ventas", Role: api.RolEditar}}
	f := q.filaContenido(Elemento{ID: "f1", Nombre: "a.pdf", Tamano: 3}, true, tema.Claro)
	casi(t, "la fila de contenido", f.AltoFilaPlana(), 57)
	unidad := NuevaFilaPlana("Ventas")
	unidad.Glifo = f.Glifo
	unidad.LadoGlifo, unidad.CajaGlifo, unidad.Meta = LadoGlifoEquipo, CajaGlifoEquipo, "x"
	unidad.RellenoY, unidad.Filete = RellenoYFilaUnidad, SinFilete
	casi(t, "la primera unidad de la lista", unidad.AltoFilaPlana(), 60)
	casi(t, "la cabecera de una unidad", AltoCabeceraEquipo, 53)
}

// TestLasUnidadesSePintan pinta la lista y el detalle en sus fases y en los dos temas.
func TestLasUnidadesSePintan(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	tr := traductor(t)
	for _, nombre := range []string{"claro", "oscuro"} {
		tema.Elegir(nombre)
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		for _, fase := range []Fase{Cargando, Exito, Vacia, Fallo, SinSesion, SinConexion} {
			q := NuevaVistaEquipos(tr)
			q.Fase, q.Ventana = fase, 1280
			q.Unidades = []api.UnidadEquipo{{ID: "u1", Name: "Ventas", Role: api.RolGestionar}}
			pintar(t, q.Objeto(v))
			q.Abierta = &api.UnidadEquipo{ID: "u1", Name: "Ventas", Role: api.RolGestionar}
			q.FaseDetalle, q.HayMas = fase, true
			q.Contenido = []Elemento{{ID: "c1", Nombre: "Ofertas", Carpeta: true, Tamano: -1}, {ID: "f1", Nombre: "a.pdf", Tamano: 3}}
			pintar(t, q.Objeto(v))
		}
	}
}

func pintar(t *testing.T, o fyne.CanvasObject) {
	t.Helper()
	w := test.NewWindow(o)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1020, 744))
	w.Canvas().Capture()
	w.Close()
}

// TestLaMetaDeLaUnidadPasaDeLineaEnElMovil: lo que se puede hacer en una unidad
// (`sytue-meta block`, sin `truncate`) parte en varias líneas cuando no cabe, y la fila
// crece con ellas; a ancho de escritorio cabe en una.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `MetaParte`, la fila mide lo mismo a
// 360 que a 1280 y la meta sale cortada con puntos suspensivos.
func TestLaMetaDeLaUnidadPasaDeLineaEnElMovil(t *testing.T) {
	ancho := float32(0)
	f := NuevaFilaPlana("Ventas")
	f.Meta = strings.Repeat("Puede ver, comentar y descargar lo que hay dentro ", 2)
	f.MetaParte, f.AnchoDe = true, func() float32 { return ancho }
	f.Botones = []BotonFila{{Texto: "Abrir"}}
	ancho = 1200
	escritorio := f.MinSize().Height
	ancho = 300
	movil := f.MinSize().Height
	if movil < escritorio+tema.TextoXS.Linea {
		t.Fatalf("a 300 la fila mide %v y a 1200 %v: la meta no pasa de línea", movil, escritorio)
	}
	w := test.NewWindow(f)
	defer w.Close()
	w.Resize(fyne.NewSize(300, movil))
	f.Resize(fyne.NewSize(300, movil))
	w.Canvas().Capture()
}
