// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestLaTablaDeAtajosEsLaDeLaWeb: las mismas filas, en el mismo orden y con las mismas
// descripciones, que `GALERIA` y `VISOR` de `lib/atajos.ts`. Si una plataforma gana un
// atajo y la otra no, la hoja de una de las dos miente.
func TestLaTablaDeAtajosEsLaDeLaWeb(t *testing.T) {
	fuente, err := os.ReadFile("../../../frontend/apps/photos/src/lib/atajos.ts")
	if err != nil {
		t.Fatal(err)
	}
	tabla := func(nombre string) []string {
		bloque := regexp.MustCompile(`(?s)const ` + nombre + `: [^=]+= \[(.*?)\n\];`).FindSubmatch(fuente)
		if bloque == nil {
			t.Fatalf("atajos.ts ya no declara %s: el espejo hay que rehacerlo", nombre)
		}
		ms := regexp.MustCompile(`descripcion: '([^']+)'`).FindAllSubmatch(bloque[1], -1)
		claves := make([]string, 0, len(ms))
		for _, m := range ms {
			claves = append(claves, string(m[1]))
		}
		return claves
	}
	claves := func(filas []filaAtajo) []string {
		out := make([]string, len(filas))
		for i, f := range filas {
			out[i] = f.clave
		}
		return out
	}
	if web, aqui := tabla("GALERIA"), claves(atajosGaleria); !slices.Equal(web, aqui) {
		t.Errorf("la galería: web %v, Fyne %v", web, aqui)
	}
	if web, aqui := tabla("VISOR"), claves(atajosVisor); !slices.Equal(web, aqui) {
		t.Errorf("el visor: web %v, Fyne %v", web, aqui)
	}
}

// TestCadaAtajoTieneSuAccion: lo que llega como tecla y lo que llega como runa (en
// mayúscula también: el bloqueo de mayúsculas) encuentra su fila.
func TestCadaAtajoTieneSuAccion(t *testing.T) {
	casos := []struct {
		tabla  []filaAtajo
		tecla  string
		quiero accionAtajo
	}{
		{atajosGaleria, teclaDeRuna('u'), atajoSubir},
		{atajosGaleria, teclaDeRuna('U'), atajoSubir},
		{atajosGaleria, string(fyne.KeyEscape), atajoSalirDeSeleccion},
		{atajosGaleria, teclaDeRuna('?'), atajoAyuda},
		{atajosGaleria, string(fyne.KeyLeft), atajoNinguno},
		{atajosVisor, string(fyne.KeyLeft), atajoAnterior},
		{atajosVisor, string(fyne.KeyRight), atajoSiguiente},
		{atajosVisor, string(fyne.KeyHome), atajoPrimera},
		{atajosVisor, string(fyne.KeyEnd), atajoUltima},
		{atajosVisor, teclaDeRuna(' '), atajoPase},
		{atajosVisor, teclaDeRuna('I'), atajoFicha},
		{atajosVisor, teclaDeRuna('z'), atajoTamanyoReal},
		{atajosVisor, string(fyne.KeyDelete), atajoPapelera},
		{atajosVisor, string(fyne.KeyBackspace), atajoPapelera},
		{atajosVisor, string(fyne.KeyEscape), atajoCerrar},
		{atajosVisor, teclaDeRuna('?'), atajoAyuda},
		// El nombre de tecla de Fyne de la «I» no es la runa: no se cruzan.
		{atajosVisor, string(fyne.KeyI), atajoNinguno},
		{atajosVisor, teclaDeRuna('u'), atajoNinguno},
	}
	for _, c := range casos {
		if got := accionDe(c.tabla, c.tecla); got != c.quiero {
			t.Errorf("%q: acción %d, tocaba %d", c.tecla, got, c.quiero)
		}
	}
}

// hojaDeAtajos es la hoja abierta encima de todo, o nil.
func hojaDeAtajos(a *App, w fyne.Window) *ui.Dialogo {
	var hoja *ui.Dialogo
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Dialogo:
			if x.Titulo == a.v.Traduce("shortcuts.title") {
				hoja = x
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				buscar(h)
			}
		}
	}
	if top := w.Canvas().Overlays().Top(); top != nil {
		buscar(top)
	}
	return hoja
}

// botonAtajosDelVisor es el mando «Atajos» de la cabecera del visor, o nil.
func botonAtajosDelVisor(a *App) *ui.BotonIcono {
	var b *ui.BotonIcono
	var buscar func(o fyne.CanvasObject)
	buscar = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.BotonIcono:
			if x.Rotulo == a.v.Traduce("shortcuts.help") {
				b = x
			}
		case *fyne.Container:
			for _, h := range x.Objects {
				buscar(h)
			}
		case *container.Scroll:
			buscar(x.Content)
		case *container.ThemeOverride:
			buscar(x.Content)
		}
	}
	buscar(a.vis.capa.dentro)
	return b
}

// TestElInterroganteAbreLaHojaDeAtajos: «?» abre la hoja en la galería (con sus dos
// grupos) y en el visor (con el suyo, encima del visor, que sigue debajo); el botón
// «Atajos» del visor abre la misma.
func TestElInterroganteAbreLaHojaDeAtajos(t *testing.T) {
	s := &servicioFalso{fotos: fotosDelServicio(5), porPagina: 100}
	a, w := appDePrueba(t, s)
	a.Arrancar()

	a.runaSuelta('?')
	hoja := hojaDeAtajos(a, w)
	if hoja == nil {
		t.Fatal("«?» en la galería no abre la hoja de atajos")
	}
	guardar(t, "atajos-galeria", w.Canvas().Capture())
	grupos := gruposDeAtajos(a.v, false, true, true)
	if len(grupos) != 2 || grupos[0].Titulo != a.v.Traduce("shortcuts.group.gallery") {
		t.Errorf("la hoja de la galería no lleva sus dos grupos: %+v", grupos)
	}
	// Con la hoja delante, «u» no abre el selector de ficheros por debajo.
	a.runaSuelta('u')
	if hojaDeAtajos(a, w) != hoja {
		t.Error("con la hoja abierta, «u» ha hecho algo")
	}
	hoja.Cerrar()
	if w.Canvas().Overlays().Top() != nil {
		t.Fatal("cerrar la hoja deja una capa puesta")
	}

	a.abrirEnGaleria(a.g.fotos[1].ID)
	capa := w.Canvas().Overlays().Top()
	a.runaSuelta('?')
	hoja = hojaDeAtajos(a, w)
	if hoja == nil || a.vis == nil {
		t.Fatal("«?» en el visor no abre la hoja encima del visor")
	}
	visor := gruposDeAtajos(a.v, true, true, true)
	if len(visor) != 1 || visor[0].Titulo != a.v.Traduce("shortcuts.group.viewer") {
		t.Errorf("la hoja del visor no lleva solo el suyo: %+v", visor)
	}
	// Una segunda «?» con la hoja ya delante no apila otra.
	a.runaVisor('?')
	if n := len(w.Canvas().Overlays().List()); n != 2 {
		t.Errorf("con la hoja abierta, otra «?» deja %d capas y tocaban 2", n)
	}
	hoja.Cerrar()
	if w.Canvas().Overlays().Top() != capa {
		t.Fatal("cerrar la hoja no deja el visor debajo")
	}

	guardar(t, "visor-con-atajos", w.Canvas().Capture())
	b := botonAtajosDelVisor(a)
	if b == nil {
		t.Fatal("el visor no ofrece el botón «Atajos»")
	}
	b.Tapped(nil)
	if hojaDeAtajos(a, w) == nil {
		t.Error("el botón «Atajos» no abre la hoja")
	}
	guardar(t, "atajos-visor", w.Canvas().Capture())
}

// TestLaHojaDelVisorNoPrometeLoQueNoHay: sin lista no hay Inicio ni Fin, y sin papelera
// no hay Supr.
func TestLaHojaDelVisorNoPrometeLoQueNoHay(t *testing.T) {
	a, _ := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(1), porPagina: 100})
	descripciones := func(saltos, papelera bool) []string {
		atajos := gruposDeAtajos(a.v, true, saltos, papelera)[0].Atajos
		out := make([]string, 0, len(atajos))
		for _, f := range atajos {
			out = append(out, f.Descripcion)
		}
		return out
	}
	sinNada := descripciones(false, false)
	for _, clave := range []string{"shortcuts.first", "shortcuts.last", "viewer.delete"} {
		if slices.Contains(sinNada, a.v.Traduce(clave)) {
			t.Errorf("sin saltos ni papelera, la hoja aún cuenta %q", clave)
		}
	}
	if !slices.Contains(sinNada, a.v.Traduce("shortcuts.info")) {
		t.Error("la hoja ha perdido «i»")
	}
	todo := gruposDeAtajos(a.v, true, true, true)[0].Atajos
	papelera := todo[len(todo)-3]
	if want := []string{a.v.Traduce("shortcuts.keyDelete"), "⌫"}; !slices.Equal(papelera.Teclas, want) {
		t.Errorf("Supr se escribe %v y tocaba %v", papelera.Teclas, want)
	}
	if todo[5].Teclas[0] != "I" || todo[4].Teclas[0] != a.v.Traduce("shortcuts.keySpace") {
		t.Errorf("las letras y el Espacio se escriben %v y %v", todo[5].Teclas, todo[4].Teclas)
	}
}
