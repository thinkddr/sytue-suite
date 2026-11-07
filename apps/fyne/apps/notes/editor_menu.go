// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// El menú contextual del cuerpo y la selección por palabras.
//
// En la web el cuerpo es un `contenteditable` y esto lo pone el navegador: el clic derecho
// —o mantener pulsado en el teléfono— ofrece Cortar, Copiar, Pegar y Seleccionar todo, y
// el doble clic selecciona la palabra. El editor propio no tenía nada de eso: en un
// teléfono, sin Ctrl+C ni Ctrl+V, no había forma de copiar ni de pegar dentro de una nota,
// y en el escritorio el clic derecho no hacía nada.
//
// Las acciones son los MISMOS atajos que llegan del teclado (`zonaEdicion.TypedShortcut`):
// Cortar deja su paso de deshacer y Pegar pasa antes por `Atajo`, que es donde el redactor
// de Sytue Mail pega con formato. Los rótulos son del catálogo de Notepads y no los de
// `widget.Entry` (`lang.L`): esos siguen el idioma del SISTEMA, y con la suite en español
// sobre un sistema en inglés —o en uno de los idiomas que Fyne no trae— el menú salía en
// inglés en medio de una app en español. El panel es `ui.Menu` y no el
// `PopUpMenu` de Fyne: una capa abierta por fuera de `ui` no se cierra con Escape sin foco
// ni con «atrás» en el teléfono (TestInventarioDeModales).
//
// # Por qué el doble toque se cuenta a mano
//
// Con `fyne.DoubleTappable`, Fyne retrasa CADA toque suelto hasta descartar el segundo:
// 300 ms en el escritorio y 500 en el teléfono. En el teléfono el cursor y el teclado
// llegarían medio segundo tarde a cada toque; en el escritorio el `Tapped` atrasado
// devolvería el cursor al punto pulsado por encima de lo que ya se hubiera escrito, o
// seleccionado con Mayús, entretanto. Es lo mismo que decidió `ui.Deslizador`: así cada
// toque llega en el acto y el segundo, si cae a tiempo y cerca, selecciona la palabra.

var _ fyne.SecondaryTappable = (*zonaEdicion)(nil)

// holguraDobleToque es lo que puede caer el segundo toque del primero, en px: lo que se
// mueve un dedo entre dos toques (el ratón se mueve menos).
const holguraDobleToque = 12

// contextoZona es lo que la zona recuerda para el doble toque y el menú contextual.
type contextoZona struct {
	// ultimo y donde son el toque anterior: cuándo y en qué punto.
	ultimo time.Time
	donde  fyne.Position
	// ancla es el punto del que cuelga el menú: un rectángulo transparente y sin tamaño
	// entre los objetos de la zona, porque `ui.Menu` se coloca respecto a algo que esté
	// en pantalla. Nace con el primer menú.
	ancla *canvas.Rectangle
}

// dobleToque apunta un toque y dice si es el segundo de un doble; entonces selecciona
// la palabra que hay debajo.
func (z *zonaEdicion) dobleToque(p fyne.Position) bool {
	c := &z.contexto
	ahora := time.Now()
	dx, dy := p.X-c.donde.X, p.Y-c.donde.Y
	if !c.ultimo.IsZero() && ahora.Sub(c.ultimo) <= fyne.CurrentApp().Driver().DoubleTapDelay() &&
		dx*dx+dy*dy <= holguraDobleToque*holguraDobleToque {
		c.ultimo = time.Time{} // el tercero vuelve a ser un toque suelto
		if q, ok := z.posEn(p); ok {
			z.e.seleccionarPalabra(q)
		}
		return true
	}
	c.ultimo, c.donde = ahora, p
	return false
}

// TappedSecondary es el clic derecho del escritorio y mantener pulsado en el teléfono:
// el menú, en el punto. En el teléfono, sin nada seleccionado, antes selecciona la
// palabra del dedo, como el menú del sistema; en el escritorio la selección y el cursor
// se quedan como estaban, que es lo que hace `widget.Entry`.
func (z *zonaEdicion) TappedSecondary(ev *fyne.PointEvent) {
	c := fyne.CurrentApp().Driver().CanvasForObject(z)
	if c == nil {
		return
	}
	c.Focus(z)
	if ui.EsTactil() && !z.e.haySeleccion() {
		if p, ok := z.posEn(ev.Position); ok {
			z.e.seleccionarPalabra(p)
		}
	}
	z.menuContextual(ev.Position).Mostrar(z)
}

// menuContextual es el de `widget.Entry.TappedSecondary`: deshacer y rehacer si hay qué,
// y Cortar y Copiar solo con algo seleccionado. Pegar sale siempre, como allí: saber si
// el portapapeles tiene algo es leerlo, y en iOS leerlo avisa a la persona de que la app
// ha pegado.
func (z *zonaEdicion) menuContextual(en fyne.Position) *ui.Menu {
	e := z.e
	portapapeles := fyne.CurrentApp().Clipboard()
	var elementos []fyne.CanvasObject
	opcion := func(clave string, s fyne.Shortcut) {
		elementos = append(elementos, ui.NuevoElementoMenu(e.tr.T(clave, nil), iconos.Ninguno, func() {
			z.TypedShortcut(s)
			// En el teléfono `ui` no devuelve el foco a lo que no es un campo al cerrar
			// la capa; aquí se sigue escribiendo, como tras el menú de bloque.
			e.reenfocar()
		}))
	}
	if len(e.deshacer) > 0 {
		opcion("editor.undo", &fyne.ShortcutUndo{})
	}
	if len(e.rehacer) > 0 {
		opcion("editor.redo", &fyne.ShortcutRedo{})
	}
	if len(elementos) > 0 {
		elementos = append(elementos, ui.NuevoSeparadorMenu())
	}
	if e.haySeleccion() {
		opcion("editor.cut", &fyne.ShortcutCut{Clipboard: portapapeles})
		opcion("editor.copy", &fyne.ShortcutCopy{Clipboard: portapapeles})
	}
	opcion("editor.paste", &fyne.ShortcutPaste{Clipboard: portapapeles})
	opcion("editor.selectAll", &fyne.ShortcutSelectAll{})
	m := ui.NuevoMenu(elementos...)
	m.AlInicio, m.Ancla = true, z.anclaEn(en)
	return m
}

// anclaEn pone el ancla del menú en ese punto de la zona.
func (z *zonaEdicion) anclaEn(p fyne.Position) fyne.CanvasObject {
	if z.contexto.ancla == nil {
		z.contexto.ancla = canvas.NewRectangle(color.Transparent)
	}
	z.contexto.ancla.Move(p)
	z.Refresh() // la zona la mete entre sus objetos: `ui.Menu` la busca en pantalla
	return z.contexto.ancla
}

// seleccionarPalabra selecciona la palabra de esa posición (ver palabraEn).
func (e *EditorNota) seleccionarPalabra(p Pos) {
	if p.B < 0 || p.B >= len(e.doc.Bloques) || !editable(e.doc.Bloques[p.B]) {
		return
	}
	bl := e.doc.Bloques[p.B]
	a, b := palabraEn([]rune(bl.Texto()), p.O, inicioEditable(bl))
	e.ancla, e.cur = Pos{p.B, a}, Pos{p.B, b}
	e.pendiente, e.escribiendo, e.hayX = nil, false, false
	e.repintar()
}

// claseLetra es lo que junta el doble clic: letras de palabra seguidas, blancos seguidos,
// y cualquier otra cosa —un signo, una imagen— sola.
type claseLetra uint8

const (
	deOtra claseLetra = iota
	dePalabra
	deBlanco
)

func claseDe(r rune) claseLetra {
	switch {
	case r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.Is(unicode.Mn, r):
		return dePalabra
	case r != '\n' && unicode.IsSpace(r):
		return deBlanco
	}
	return deOtra
}

// palabraEn son los extremos [a, b) de la palabra de las runas `r` en el punto `o`, sin
// salir de `desde` (el principio escribible del bloque). El punto es un hueco entre dos
// letras —el clic en la mitad derecha de una letra cae detrás de ella—, así que se mira
// la de delante y, si esa no es de palabra y la de detrás sí, la de detrás. Un apóstrofo
// entre dos letras es de la palabra («l'aigua», «don't»), como en el navegador. Sobre un
// salto de línea no hay nada que seleccionar: devuelve el punto.
func palabraEn(r []rune, o, desde int) (int, int) {
	o = min(max(o, desde), len(r))
	if len(r) <= desde {
		return o, o
	}
	clase := func(i int) claseLetra {
		if (r[i] == '\'' || r[i] == '’') && i > desde && i+1 < len(r) &&
			claseDe(r[i-1]) == dePalabra && claseDe(r[i+1]) == dePalabra {
			return dePalabra
		}
		return claseDe(r[i])
	}
	i := o
	switch {
	case i < len(r) && clase(i) == dePalabra:
	case i > desde && clase(i-1) == dePalabra, i == len(r):
		i--
	}
	c := clase(i)
	switch {
	case r[i] == '\n':
		return o, o
	case c == deOtra:
		return i, i + 1
	}
	a, b := i, i+1
	for a > desde && clase(a-1) == c {
		a--
	}
	for b < len(r) && clase(b) == c {
		b++
	}
	return a, b
}
