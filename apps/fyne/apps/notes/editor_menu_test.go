// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"io/fs"
	"math"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// LOS GUARDIANES DEL MENÚ CONTEXTUAL Y DEL DOBLE TOQUE (editor_menu.go): sin ellos, en un
// teléfono no había forma de copiar ni de pegar dentro de una nota.

// puntoDeLetra es el centro de la letra de esa posición, en px de la zona.
func puntoDeLetra(t *testing.T, ed *EditorNota, p Pos) fyne.Position {
	t.Helper()
	m := ed.zona.maquetado()
	i := ed.lineaDe(m, p)
	if i < 0 {
		t.Fatalf("no hay línea para %v", p)
	}
	l := m.Lineas[i]
	return fyne.NewPos(PXZonaEd+(l.XDe(p.O)+l.XDe(p.O+1))/2, PYZonaEd+l.Y+l.Alto/2)
}

// opcionesDelMenu son los rótulos del menú abierto, en orden, y sus acciones.
func opcionesDelMenu(c fyne.Canvas) ([]string, map[string]*ui.ElementoMenu) {
	var rotulos []string
	por := map[string]*ui.ElementoMenu{}
	recorrerInterno(c.Overlays().Top(), func(o fyne.CanvasObject) {
		if e, ok := o.(*ui.ElementoMenu); ok {
			rotulos = append(rotulos, e.Texto)
			por[e.Texto] = e
		}
	})
	return rotulos, por
}

// TestElDobleToqueSeleccionaLaPalabra: dos toques seguidos y en el mismo sitio
// seleccionan la palabra, con sus acentos; dos lejos en el tiempo o en la pantalla son
// dos toques sueltos, que ponen el cursor.
//
// VISTO EN ROJO (2026-11-05): sin `dobleToque` en `Tapped`, el segundo toque deja el
// cursor dentro de «mundo» y no selecciona nada.
func TestElDobleToqueSeleccionaLaPalabra(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer("<p>hola mundo, adiós</p>"), "")
	w := test.NewWindow(ed)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 200))
	tocar := func(p fyne.Position) { ed.zona.Tapped(&fyne.PointEvent{Position: p}) }

	mundo := puntoDeLetra(t, ed, Pos{0, 7})
	tocar(mundo)
	tocar(mundo)
	if got := ed.Seleccionado(); got != "mundo" {
		t.Fatalf("el doble toque en «mundo» selecciona %q", got)
	}
	// El tercero es un toque suelto; el cuarto, cerca y a tiempo, otro doble.
	adios := puntoDeLetra(t, ed, Pos{0, 16})
	tocar(adios)
	tocar(adios.Add(fyne.NewPos(4, 2))) // el dedo no cae en el mismo píxel
	if got := ed.Seleccionado(); got != "adiós" {
		t.Fatalf("el doble toque en «adiós» selecciona %q", got)
	}

	tocar(mundo)
	ed.zona.contexto.ultimo = time.Now().Add(-time.Second)
	tocar(mundo)
	if ed.haySeleccion() {
		t.Fatalf("dos toques separados por un segundo seleccionan %q", ed.Seleccionado())
	}
	tocar(puntoDeLetra(t, ed, Pos{0, 1}))
	if ed.haySeleccion() || ed.cur.O > 2 {
		t.Fatalf("un toque lejos del anterior no es doble: deja %q y el cursor en %v", ed.Seleccionado(), ed.cur)
	}
}

// TestPalabraEn: lo que selecciona el doble clic según dónde cae, como el navegador.
func TestPalabraEn(t *testing.T) {
	casos := []struct {
		texto     string
		o, desde  int
		seleccion string
	}{
		{"hola mundo", 7, 0, "mundo"},
		{"hola mundo", 4, 0, "hola"},   // detrás de su última letra
		{"hola mundo", 10, 0, "mundo"}, // al final del bloque
		{"a , b", 2, 0, ","},           // un signo, solo
		{"uno   dos", 4, 0, "   "},     // los blancos, juntos
		{"l'aigua és", 1, 0, "l'aigua"},
		{"don’t stop", 2, 0, "don’t"},
		{"uno\n\ndos", 4, 0, ""}, // una línea vacía
		{" pan", 0, 1, "pan"},    // una tarea: el blanco de la casilla no se pisa
		{"", 0, 0, ""},
	}
	for _, c := range casos {
		r := []rune(c.texto)
		a, b := palabraEn(r, c.o, c.desde)
		if got := string(r[a:b]); got != c.seleccion {
			t.Errorf("en %q, %d: %q, y es %q", c.texto, c.o, got, c.seleccion)
		}
	}
}

// TestElMenuContextualCortaCopiaYPegaSinCerrarLaNota: el clic derecho ofrece lo de
// `widget.Entry` —deshacer y rehacer si hay qué, Cortar y Copiar con algo seleccionado,
// Pegar y Seleccionar todo—, en el puntero, y cada opción hace lo mismo que su atajo:
// Cortar deja su paso de deshacer. Ni abrir el menú ni elegir en él cierran la nota. Y
// los rótulos van en el idioma de la app (aquí, español), no en el del sistema.
//
// VISTO EN ROJO (2026-11-06): con los rótulos de `lang.L`, que siguen el idioma del
// sistema, el menú ofrecía «Cut», «Copy», «Paste» y «Select all» en una app en español.
//
// VISTO EN ROJO (2026-11-05), tres veces: sin el ancla entre los objetos de la zona, el
// menú se abre en la esquina de la ventana ({8 8}); si `comprobarSalida` no mira las
// capas, abrir el menú cierra la nota; y con Cortar mandando el atajo de copiar, el texto
// se queda y no hay paso de deshacer.
func TestElMenuContextualCortaCopiaYPegaSinCerrarLaNota(t *testing.T) {
	cuaderno, notas := cuadernoDePrueba("list")
	p, _ := pantallaConServicio(t, nil, notas...)
	p.activo, p.cuadernos = cuaderno, []Cuaderno{cuaderno}
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 844))
	p.win = w
	w.SetContent(p.Objeto())
	n, ok := p.nota("n2")
	if !ok {
		t.Fatal("falta la nota de texto del cuaderno de prueba")
	}
	p.abrirEditor(n)
	s := p.edicion
	if s == nil {
		t.Fatal("no se ha abierto la nota para escribir")
	}
	ed, c := s.editor, w.Canvas()
	portapapeles := fyne.CurrentApp().Clipboard()
	abrir := func(quiero ...string) map[string]*ui.ElementoMenu {
		t.Helper()
		test.TapSecondaryAt(ed.zona, fyne.NewPos(PXZonaEd+10, PYZonaEd+5))
		rotulos, por := opcionesDelMenu(c)
		if !slices.Equal(rotulos, quiero) {
			t.Fatalf("el menú ofrece %q y son %q", rotulos, quiero)
		}
		// Con el menú abierto el foco está en él: no es haberse ido de la nota. Es la
		// comprobación que el driver real encola tras mover el foco.
		ed.zona.comprobarSalida()
		if p.edicion != s {
			t.Fatal("abrir el menú cierra la nota")
		}
		return por
	}
	elegir := func(por map[string]*ui.ElementoMenu, rotulo string) {
		t.Helper()
		test.Tap(por[rotulo])
		if len(c.Overlays().List()) != 0 {
			t.Fatalf("elegir %s no recoge el menú", rotulo)
		}
		ed.zona.comprobarSalida()
		if p.edicion != s || p.lienzo.Editor != ed || c.Focused() != ed.Zona() {
			t.Fatalf("elegir %s cierra la nota o le quita el foco (%T)", rotulo, c.Focused())
		}
	}

	ed.ancla, ed.cur = Pos{0, 0}, Pos{0, 6} // «Veinte»
	ed.repintar()
	por := abrir("Cortar", "Copiar", "Pegar", "Seleccionar todo")
	// El panel cuelga del puntero, no de la esquina de la ventana ni del pie de la nota.
	d := fyne.CurrentApp().Driver()
	puntero := d.AbsolutePositionForObject(ed.zona).Add(fyne.NewPos(PXZonaEd+10, PYZonaEd+5))
	var menu *ui.Menu
	recorrerInterno(c.Overlays().Top(), func(o fyne.CanvasObject) {
		if m, ok := o.(*ui.Menu); ok {
			menu = m
		}
	})
	if menu == nil {
		t.Fatal("el clic derecho no abre un `ui.Menu`")
	}
	if en := d.AbsolutePositionForObject(menu); math.Abs(float64(en.X-puntero.X)) > 1 ||
		math.Abs(float64(en.Y-puntero.Y-ui.SeparacionMenu)) > 1 {
		t.Fatalf("el menú se abre en %v y el puntero está en %v", en, puntero)
	}
	elegir(por, "Copiar")
	if portapapeles.Content() != "Veinte" || ed.HTML() != "<p>Veinte grandes.</p>" {
		t.Fatalf("Copiar deja %q en el portapapeles y %s en la nota", portapapeles.Content(), ed.HTML())
	}
	pasos := len(ed.deshacer)
	elegir(abrir("Cortar", "Copiar", "Pegar", "Seleccionar todo"), "Cortar")
	if portapapeles.Content() != "Veinte" || ed.HTML() != "<p> grandes.</p>" || len(ed.deshacer) != pasos+1 {
		t.Fatalf("Cortar deja %q en el portapapeles, %s en la nota y %d pasos de deshacer (eran %d)",
			portapapeles.Content(), ed.HTML(), len(ed.deshacer), pasos)
	}
	portapapeles.SetContent("Treinta")
	elegir(abrir("Deshacer", "Pegar", "Seleccionar todo"), "Pegar")
	if ed.HTML() != "<p>Treinta grandes.</p>" {
		t.Fatalf("Pegar deja %s", ed.HTML())
	}
	elegir(abrir("Deshacer", "Pegar", "Seleccionar todo"), "Deshacer")
	if ed.HTML() != "<p> grandes.</p>" {
		t.Fatalf("Deshacer desde el menú deja %s", ed.HTML())
	}
	elegir(abrir("Deshacer", "Rehacer", "Pegar", "Seleccionar todo"), "Seleccionar todo")
	if got := ed.Seleccionado(); got != " grandes." {
		t.Fatalf("Seleccionar todo selecciona %q", got)
	}
}

// TestMantenerPulsadoEnElTelefonoSeleccionaLaPalabra: en el teléfono, mantener pulsado
// sin nada seleccionado selecciona la palabra del dedo y ofrece copiarla, como el menú
// del sistema; en el escritorio, el clic derecho no toca la selección.
//
// VISTO EN ROJO (2026-11-05): sin seleccionar antes la palabra, mantener pulsado en
// «mundo» no selecciona nada y el menú del teléfono no ofrece Copiar.
func TestMantenerPulsadoEnElTelefonoSeleccionaLaPalabra(t *testing.T) {
	for _, tactil := range []bool{true, false} {
		t.Run(fmt.Sprint("tactil=", tactil), func(t *testing.T) {
			t.Cleanup(ui.TactilDePrueba(tactil))
			ed := NuevoEditorNota(trInterno(t), Leer("<p>hola mundo</p>"), "")
			w := test.NewWindow(ed)
			t.Cleanup(w.Close)
			w.Resize(fyne.NewSize(390, 600))
			w.Canvas().Focus(ed.Zona())
			test.TapSecondaryAt(ed.zona, puntoDeLetra(t, ed, Pos{0, 7}))
			_, por := opcionesDelMenu(w.Canvas())
			copiar := por["Copiar"]
			if !tactil {
				if ed.haySeleccion() || copiar != nil {
					t.Fatalf("en el escritorio el clic derecho selecciona %q", ed.Seleccionado())
				}
				return
			}
			if ed.Seleccionado() != "mundo" || copiar == nil {
				t.Fatalf("mantener pulsado en «mundo» selecciona %q", ed.Seleccionado())
			}
			test.Tap(copiar)
			if got := fyne.CurrentApp().Clipboard().Content(); got != "mundo" {
				t.Fatalf("Copiar deja %q en el portapapeles", got)
			}
		})
	}
}

// TestElMenuPegaPorElMismoCaminoQueElTeclado: quien aloja el editor y se queda el pegado
// del teclado —el redactor de Sytue Mail, que pega con formato— se queda también el del
// menú.
//
// VISTO EN ROJO (2026-11-05): con Pegar llamando a `atajo` en vez de a `TypedShortcut`,
// el menú pega texto plano por debajo de `Atajo`.
func TestElMenuPegaPorElMismoCaminoQueElTeclado(t *testing.T) {
	ed := NuevoEditorNota(trInterno(t), Leer("<p>hola</p>"), "")
	var pegado string
	ed.Atajo = func(s fyne.Shortcut) bool {
		x, ok := s.(*fyne.ShortcutPaste)
		if ok {
			pegado = x.Clipboard.Content()
		}
		return ok
	}
	w := test.NewWindow(ed)
	defer w.Close()
	w.Resize(fyne.NewSize(400, 200))
	w.Canvas().Focus(ed.Zona())
	fyne.CurrentApp().Clipboard().SetContent("<b>rico</b>")
	test.TapSecondaryAt(ed.zona, fyne.NewPos(PXZonaEd+4, PYZonaEd+4))
	_, por := opcionesDelMenu(w.Canvas())
	test.Tap(por["Pegar"])
	if pegado != "<b>rico</b>" || ed.HTML() != "<p>hola</p>" {
		t.Fatalf("el menú pega %q por `Atajo` y la nota queda %s", pegado, ed.HTML())
	}
}

// TestElMenuContextualTieneSusRotulosEnCadaApp: el editor de notas es también el cuerpo
// del redactor de Sytue Mail, que lo monta con el catálogo de Mail. Los rótulos del menú
// tienen que estar en los dos catálogos y en los 26 idiomas; si falta uno, el menú enseña
// la clave («editor.paste») en vez del texto.
//
// VISTO EN ROJO (2026-11-06): con las claves solo en el catálogo de Notepads, Mail daba
// «editor.cut», «editor.copy», «editor.paste» y «editor.selectAll» en los 26 idiomas.
func TestElMenuContextualTieneSusRotulosEnCadaApp(t *testing.T) {
	claves := []string{"editor.undo", "editor.redo", "editor.cut", "editor.copy", "editor.paste", "editor.selectAll"}
	for app, catalogo := range map[string]func() fs.FS{"notes": catalogos.Notes, "mail": catalogos.Mail} {
		for _, l := range i18n.Locales {
			tr, err := i18n.New(l, i18n.Common(), catalogo())
			if err != nil {
				t.Fatal(err)
			}
			for _, k := range claves {
				if tr.T(k, nil) == k {
					t.Errorf("%s/%s: falta %s", app, l, k)
				}
			}
		}
	}
}

// TestEnElTelefonoLaBarraVaEnUnaFila: por debajo de `md` la barra de formato es UNA fila
// que se desplaza de lado (`FILA_UNICA_EN_MOVIL` de la web); en el escritorio envuelve
// como siempre y no se desplaza.
//
// VISTO EN ROJO (2026-11-06): en el driver móvil, a 393 de ancho, la barra envolvía en
// cuatro filas y se comía media pantalla.
func TestEnElTelefonoLaBarraVaEnUnaFila(t *testing.T) {
	medir := func(ancho float32) (alto float32, desplaza bool) {
		ed := NuevoEditorNota(trInterno(t), Leer("<p>hola</p>"), "")
		w := test.NewWindow(ed)
		defer w.Close()
		w.Resize(fyne.NewSize(ancho, 600))
		lado := ed.barraLado
		return lado.AltoPara(lado.Size().Width), lado.AnchoContenido() > lado.Size().Width+1
	}
	altoAncha, desplazaAncha := medir(1280)
	altoTelefono, desplazaTelefono := medir(390)
	if desplazaAncha {
		t.Error("a 1280 la barra se desplaza de lado")
	}
	if altoTelefono != altoAncha || !desplazaTelefono {
		t.Fatalf("a 390 la barra mide %v (a 1280, %v) y se desplaza=%v: quiero una fila que se desplace",
			altoTelefono, altoAncha, desplazaTelefono)
	}
}
