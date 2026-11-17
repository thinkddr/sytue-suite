// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Las cabeceras A–Z (CONT-05). Los vectores son los MISMOS que recorre la web
// (apps/frontend/apps/contacts/src/client/iniciales.test.ts): si un lado agrupa distinto,
// falla el suyo.
//
// VISTO EN ROJO (2026-11-12): con la primera letra sin tilde a secas, la Ñ caía en la N
// y la Å sueca en la A; sin la prueba contra la A–Z, la Ø inglesa tenía cabecera propia;
// con la etiqueta «nb-NO» tal cual, x/text metía la Å noruega en la A; sin `ponerCabecera`
// ni `filaDe`, ↓ desde el último de la A enfocaba la cabecera B. Deshecho.

type vectoresIniciales struct {
	Casos []struct {
		Nombre  string   `json:"nombre"`
		Locale  string   `json:"locale"`
		Nombres []string `json:"nombres"`
		Grupos  []struct {
			Inicial string   `json:"inicial"`
			Nombres []string `json:"nombres"`
		} `json:"grupos"`
	} `json:"casos"`
}

func TestInicialesSonLasDeLaWeb(t *testing.T) {
	datos, err := os.ReadFile("testdata/iniciales.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresIniciales
	if err := json.Unmarshal(datos, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 10 {
		t.Fatalf("hay %d vectores; se esperaban al menos 10", len(v.Casos))
	}
	for _, c := range v.Casos {
		t.Run(c.Nombre, func(t *testing.T) {
			if !i18n.IsLocale(c.Locale) {
				t.Fatalf("idioma desconocido %q", c.Locale)
			}
			grupos := GruposPorInicial(c.Nombres, i18n.Locale(c.Locale))
			if len(grupos) != len(c.Grupos) {
				t.Fatalf("salen %d grupos (%v); se esperaban %d", len(grupos), grupos, len(c.Grupos))
			}
			for i, g := range grupos {
				var nombres []string
				for _, j := range g.Indices {
					nombres = append(nombres, c.Nombres[j])
				}
				if g.Inicial != c.Grupos[i].Inicial || !slices.Equal(nombres, c.Grupos[i].Nombres) {
					t.Errorf("grupo %d: %q %v; se esperaba %q %v", i, g.Inicial, nombres,
						c.Grupos[i].Inicial, c.Grupos[i].Nombres)
				}
			}
		})
	}
}

// appConNombres monta la app con estos contactos cargados.
func appConNombres(t *testing.T, nombres ...string) (*App, fyne.Canvas) {
	t.Helper()
	cs := make([]api.Contact, len(nombres))
	for i, n := range nombres {
		cs[i] = persona(n, n)
	}
	s := &servicioFalso{contactos: cs}
	a := appDePrueba(t, s)
	a.Cargar()
	s.correrCola()
	c := a.lienzo()
	if c == nil {
		t.Fatal("la app no está montada")
	}
	c.SetOnTypedRune(a.Atajo)
	return a, c
}

// cabecerasDeLista son las cabeceras de la lista virtual, en orden.
func cabecerasDeLista(p *Pantalla) []string {
	var v []string
	for _, f := range p.filasLista {
		if f.cabecera != "" {
			v = append(v, f.cabecera)
		}
	}
	return v
}

func TestListaAgrupaPorInicialConElOrdenDeLaColacion(t *testing.T) {
	a, _ := appConNombres(t, "Zoe", "123 Taxi", "Ñoño", "Ana", "Álvaro", "Nuria", "beatriz")
	p := a.Pantalla
	if got, want := cabecerasDeLista(p), []string{"A", "B", "N", "Ñ", "Z", "#"}; !slices.Equal(got, want) {
		t.Fatalf("cabeceras %v; se esperaban %v", got, want)
	}
	// Lo que no empieza por letra, al final también en `Contactos`: el cursor recorre
	// lo que se ve en el orden en que se ve.
	if got, want := nombres(p.Contactos), []string{"Álvaro", "Ana", "beatriz", "Nuria", "Ñoño", "Zoe", "123 Taxi"}; !slices.Equal(got, want) {
		t.Fatalf("orden %v; se esperaba %v", got, want)
	}
	// Cada contacto sabe en qué fila de la lista está.
	for i, fila := range p.filaDe {
		if p.filasLista[fila].cabecera != "" || p.filasLista[fila].contacto != i {
			t.Fatalf("el contacto %d apunta a la fila %d, que es %+v", i, fila, p.filasLista[fila])
		}
	}
	// Y la lista pinta la cabecera con su alto, sin casilla ni avatar.
	var cabecera *FilaContacto
	for _, o := range filasPintadas(a) {
		if o.Cabecera == "A" {
			cabecera = o
		}
	}
	if cabecera == nil {
		t.Fatal("la cabecera A no está pintada")
	}
	if h := cabecera.Size().Height; h != AltoCabeceraLista {
		t.Errorf("la cabecera mide %v; se esperaban %v", h, AltoCabeceraLista)
	}
	if !cabecera.Disabled() {
		t.Error("la cabecera no debe estar en el recorrido del tabulador")
	}
	cabecera.Tapped(&fyne.PointEvent{Position: fyne.NewPos(150, 10)})
	cabecera.TappedSecondary(&fyne.PointEvent{})
	if p.Seleccionado != nil || len(a.marcados) != 0 {
		t.Error("tocar la cabecera no debe abrir ni marcar nada")
	}
}

func TestListaSinCabecerasConBusquedaOFiltro(t *testing.T) {
	a, _ := appConNombres(t, "Ana", "Bea", "Carla")
	p := a.Pantalla
	if len(cabecerasDeLista(p)) != 3 {
		t.Fatalf("sin filtro debe haber tres cabeceras; hay %v", cabecerasDeLista(p))
	}
	p.AlBuscar("a")
	if got := cabecerasDeLista(p); len(got) != 0 || len(p.filasLista) != 3 {
		t.Fatalf("con búsqueda no debe haber cabeceras; hay %v en %d filas", got, len(p.filasLista))
	}
	p.AlBuscar("")
	for _, c := range p.Contactos {
		a.favoritos[api.CleanUID(c.UID)] = true
	}
	p.SoloDestacados = true
	a.repintar()
	if got := cabecerasDeLista(p); len(got) != 0 || len(p.filasLista) != 3 {
		t.Fatalf("con «Destacados» no debe haber cabeceras; hay %v en %d filas", got, len(p.filasLista))
	}
}

func TestListaElTecladoSeSaltaLasCabeceras(t *testing.T) {
	a, c := appConNombres(t, "Ana", "Alba", "Bea")
	p := a.Pantalla
	teclear(c, 'j')
	if f := filaEnfocada(c); f == nil || f.Nombre != "Alba" {
		t.Fatalf("la primera j debe enfocar el primer contacto, no la cabecera; enfocado %v", c.Focused())
	}
	pulsar(c, fyne.KeyDown)
	pulsar(c, fyne.KeyDown)
	if f := filaEnfocada(c); f == nil || f.Nombre != "Bea" || p.CursorFila() != 2 {
		t.Fatalf("↓ desde el último de la A debe ir a Bea, saltándose la B; cursor %d, enfocado %v",
			p.CursorFila(), c.Focused())
	}
	teclear(c, 'k')
	if f := filaEnfocada(c); f == nil || f.Nombre != "Ana" {
		t.Fatalf("k debe volver a Ana; enfocado %v", c.Focused())
	}
}

// filasPintadas son las filas que la lista tiene a la vista.
func filasPintadas(a *App) []*FilaContacto {
	var v []*FilaContacto
	var recorrer func(o fyne.CanvasObject)
	recorrer = func(o fyne.CanvasObject) {
		if f, ok := o.(*FilaContacto); ok {
			if f.Visible() {
				v = append(v, f)
			}
			return
		}
		switch w := o.(type) {
		case *fyne.Container:
			for _, h := range w.Objects {
				recorrer(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(w).Objects() {
				recorrer(h)
			}
		}
	}
	recorrer(a.Pantalla.virtual)
	return v
}

// VISTO EN ROJO: con `AlMover` a nil en la cabecera, ↓ no hacía nada desde una fila
// enfocada que la rueda había reciclado en cabecera.
func TestListaLaCabeceraRecicladaConFocoSigueMoviendo(t *testing.T) {
	a, c := appConNombres(t, "Ana", "Alba", "Bea")
	p := a.Pantalla
	teclear(c, 'j')
	f := filaEnfocada(c)
	if f == nil || f.Nombre != "Alba" {
		t.Fatalf("la primera j debe enfocar a Alba; enfocado %v", c.Focused())
	}
	fyne.DoAndWait(func() { p.ponerCabecera(f, "A") })
	pulsar(c, fyne.KeyDown)
	if p.CursorFila() != 1 {
		t.Fatalf("↓ desde la cabecera reciclada debe llevar el cursor a Ana; cursor %d", p.CursorFila())
	}
}
