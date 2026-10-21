// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"reflect"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// EditorPlano es el motor de edición de Fyne con TODO su cromo apagado, para quien ya
// tiene pintada la caja donde se escribe.
//
// # Por qué no vale `Entrada`
//
// `Entrada` es el campo de FORMULARIO completo: caja, borde, radio y aro. Hay dos
// sitios de la suite donde eso sobra porque la caja es de otro:
//
//   - la celda en edición de una hoja de cálculo, que es una casilla de la cuadrícula
//     —meter ahí una `Entrada` pinta un segundo borde dentro de la celda—, y
//   - la barra de fórmulas, que en la web es `bg-transparent` y sin borde
//     (`class="… h-5 w-full rounded-sm bg-transparent px-1 text-sm"`).
//
// Lo que se reaprovecha, y es lo caro, es lo MEDIDO en campo.go: apagándole el cromo
// por tema, `widget.Entry` pinta su texto en el mismo píxel que un `canvas.Text`
// nuestro del mismo escalón (desvío 0,000 px) y hay que imponerle el interlineado de
// CSS. Ese tema es `temaEditor` y ya está escrito; esto es la misma pieza sin la caja.
// Vale también aquí el aviso de campo.go: **no hay IME en escritorio**.
type EditorPlano struct {
	editor
	env *container.ThemeOverride
	th  *temaEditor

	// Tecla ve las teclas ANTES que el editor y devuelve true si se las queda. Es lo
	// que necesita una hoja de cálculo —Intro confirma y baja, Escape cancela, el
	// tabulador confirma y pasa a la derecha—, y sin el gancho habría que reescribir
	// `widget.Entry` entero para cambiar cuatro teclas.
	Tecla func(*fyne.KeyEvent) bool
	// Tabulador hace que el tabulador llegue a Tecla en vez de pasar el foco al
	// siguiente control: la celda de una hoja de cálculo confirma con él y salta a la
	// de al lado. Sin esto, Fyne se lo come antes (`fyne.Tabbable`).
	Tabulador bool
	// Atajo ve las COMBINACIONES —Ctrl/⌘ + tecla— antes que el editor y devuelve true
	// si se las queda. Hace falta aparte de Tecla porque Fyne no las entrega como
	// tecla: las manda como atajo al campo enfocado (`triggersShortcut` del driver), y
	// un atajo de la ventana no llega nunca mientras el foco está en un campo. Es el
	// ⌘/Ctrl + Intro que envía en el redactor de Sytue Mail.
	Atajo func(fyne.Shortcut) bool

	mayus bool
	// alDesplazar y enganchado son el aviso de AlDesplazar y el desplazador al que ya
	// se le ha puesto.
	alDesplazar func()
	enganchado  uintptr
}

var (
	_ fyne.Widget    = (*EditorPlano)(nil)
	_ fyne.Focusable = (*EditorPlano)(nil)
)

// NuevoEditorPlano monta el editor con ese escalón de la escala. `alEnfocar` puede ser
// nil.
func NuevoEditorPlano(e tema.Escalon, alEnfocar func(bool)) *EditorPlano {
	return nuevoEditorPlano(e, alEnfocar, false, false)
}

// NuevoEditorPlanoDesplazable es el campo de UNA línea que corre el texto para que el
// cursor se vea, como un `<input>` —la barra de fórmulas de una hoja—, y sin barra de
// desplazamiento, que un `<input>` no enseña. Es un constructor por lo mismo que el de
// varias líneas: el renderer decide al nacer si lleva desplazador.
func NuevoEditorPlanoDesplazable(e tema.Escalon, alEnfocar func(bool)) *EditorPlano {
	return nuevoEditorPlano(e, alEnfocar, false, true)
}

// NuevoEditorPlanoMultilinea es el mismo motor para un `<textarea>` sin caja propia —las
// notas del ponente de Diapos—: varias líneas, reparto por palabras y desplazamiento
// vertical.
//
// Es un constructor y no tres campos que poner después, y no por comodidad: el tema del
// envoltorio crea el renderer de `widget.Entry` al montarse, y ese renderer decide EN
// ESE MOMENTO si lleva desplazador. Con los campos puestos después, el primer `Layout`
// usa un desplazador sin contenido y entra en pánico (visto en el frontal el
// 2026-09-22, `scroller.go:419`).
func NuevoEditorPlanoMultilinea(e tema.Escalon, alEnfocar func(bool)) *EditorPlano {
	return nuevoEditorPlano(e, alEnfocar, true, false)
}

func nuevoEditorPlano(e tema.Escalon, alEnfocar func(bool), multilinea, horizontal bool) *EditorPlano {
	p := &EditorPlano{}
	p.alEnfocar = func(f bool) {
		if alEnfocar != nil {
			alEnfocar(f)
		}
	}
	// `ExtendBaseWidget` con el EXTERIOR, y por eso no se usa `nuevoEditor`: Fyne se
	// queda con el primero que se le declara, así que si lo extendiera el `editor` de
	// dentro, el TypedKey de aquí no se llamaría nunca y el gancho sería decorativo.
	p.ExtendBaseWidget(p)
	p.Scroll = fyne.ScrollNone
	p.Wrapping = fyne.TextWrapOff
	if multilinea {
		p.MultiLine, p.Wrapping, p.Scroll = true, fyne.TextWrapWord, fyne.ScrollVerticalOnly
	}
	if horizontal {
		p.Scroll = fyne.ScrollHorizontalOnly
	}
	p.th = &temaEditor{escalon: e, sinBarras: horizontal}
	p.env = container.NewThemeOverride(p, p.th)
	return p
}

// TypedKey ofrece la tecla al gancho antes que al editor.
func (p *EditorPlano) TypedKey(k *fyne.KeyEvent) {
	if p.Tecla != nil && p.Tecla(k) {
		return
	}
	if SubirEscape(p, k) {
		return
	}
	p.Entry.TypedKey(k)
}

// AcceptsTab se queda con el tabulador si lo pide Tabulador (o si es de varias líneas,
// que es lo que hace `widget.Entry`).
func (p *EditorPlano) AcceptsTab() bool { return p.Tabulador || p.MultiLine }

// TypedShortcut ofrece la combinación al gancho antes que al editor.
func (p *EditorPlano) TypedShortcut(s fyne.Shortcut) {
	if p.Atajo != nil && p.Atajo(s) {
		return
	}
	p.Entry.TypedShortcut(s)
}

// KeyDown apunta si se pulsa Mayús y se lo pasa al editor, que lo usa para
// seleccionar. `fyne.KeyEvent` no trae modificadores, y el gancho `Tecla` lo necesita
// para distinguir Intro de Mayús+Intro, que en el compositor de un chat son «enviar»
// y «salto de línea».
func (p *EditorPlano) KeyDown(k *fyne.KeyEvent) {
	if k.Name == desktop.KeyShiftLeft || k.Name == desktop.KeyShiftRight {
		p.mayus = true
	}
	p.Entry.KeyDown(k)
}

// KeyUp es la otra mitad de KeyDown.
func (p *EditorPlano) KeyUp(k *fyne.KeyEvent) {
	if k.Name == desktop.KeyShiftLeft || k.Name == desktop.KeyShiftRight {
		p.mayus = false
	}
	p.Entry.KeyUp(k)
}

// Mayus dice si hay una tecla Mayús pulsada ahora mismo.
func (p *EditorPlano) Mayus() bool { return p.mayus }

// Objeto es lo que va en el `Objects()` del renderer que lo aloja: el editor DENTRO de
// su tema, no el editor pelado. Colocarlo sin el envoltorio se lleva por delante el
// interlineado y el tamaño de letra.
func (p *EditorPlano) Objeto() fyne.CanvasObject { return p.env }

// Colocar pone el editor en esa caja.
func (p *EditorPlano) Colocar(pos fyne.Position, tam fyne.Size) {
	p.env.Move(pos)
	p.env.Resize(tam)
	p.enganchar() // el desplazador nace con el renderer, que ya existe tras el Resize
}

// AlDesplazar avisa cuando el editor desplazable corre su texto sin que cambie lo
// escrito —la rueda sobre el campo—, para quien pinta debajo algo alineado con las
// letras: el tinte de la barra de fórmulas, que si no se quedaba atrás hasta la
// siguiente tecla.
func (p *EditorPlano) AlDesplazar(f func()) {
	p.alDesplazar = f
	p.enganchado = 0
	p.enganchar()
}

// enganchar pone el aviso en el `OnScrolled` del desplazador interno de `widget.Entry`,
// que Fyne no publica: se llega por `reflect` como en Desplazamiento y se escribe con
// `unsafe`, porque el campo es de un valor no exportado. `widget.Entry` no usa ese
// `OnScrolled`, así que no se pisa nada. Si Fyne lo renombra, no se engancha y
// `TestLaRuedaAvisaDelDesplazamiento` se pone rojo.
func (p *EditorPlano) enganchar() {
	if p.alDesplazar == nil {
		return
	}
	sc := reflect.ValueOf(&p.Entry).Elem().FieldByName("scroll")
	if !sc.IsValid() || sc.Kind() != reflect.Pointer || sc.IsNil() || sc.Pointer() == p.enganchado {
		return
	}
	cb := sc.Elem().FieldByName("OnScrolled")
	aviso := reflect.ValueOf(func(fyne.Position) { p.alDesplazar() })
	if !cb.IsValid() || cb.Type() != aviso.Type() {
		return
	}
	reflect.NewAt(cb.Type(), unsafe.Pointer(cb.UnsafeAddr())).Elem().Set(aviso) //nolint:gosec // campo no exportado de Fyne, ver arriba
	p.enganchado = sc.Pointer()
}

// AltoFuente es la caja de línea de la FUENTE del escalón, que es lo que el editor va a
// ocupar de alto. Quien lo coloca la necesita para centrarlo dentro de la caja de línea
// de CSS, igual que hace `ColocarTexto` con un texto suelto.
func (p *EditorPlano) AltoFuente() float32 { return altoFuente(p.th.escalon) }

// Texto es lo escrito.
func (p *EditorPlano) Texto() string { return p.Text }

// PonerTexto escribe y deja el cursor AL FINAL, que es donde lo quiere quien abre una
// celda para seguir escribiendo.
func (p *EditorPlano) PonerTexto(s string) {
	p.SetText(s)
	p.CursorColumn = len([]rune(s))
}

// AlCambiar se llama con el texto nuevo en cada pulsación.
func (p *EditorPlano) AlCambiar(f func(string)) { p.OnChanged = f }

// SoloLectura es el `readOnly` del `<input>`: se ve y se puede copiar, pero no se
// escribe.
func (p *EditorPlano) SoloLectura(si bool) {
	if si {
		p.Disable()
		return
	}
	p.Enable()
}

// Tinta fija la tinta del texto y la del cursor; nil deja la del tema. Es para dos
// casos que el tema no puede saber: el editor que escribe sobre el PAPEL de una hoja de
// cálculo —blanco también en tema oscuro, así que la tinta del tema lo dejaría blanco
// sobre blanco— y el que deja su texto TRANSPARENTE porque otro lo pinta debajo en
// colores (las referencias de una fórmula), conservando el cursor.
func (p *EditorPlano) Tinta(texto, cursor color.Color) {
	if p.th.tintaTexto == texto && p.th.tintaCursor == cursor {
		return
	}
	p.th.tintaTexto, p.th.tintaCursor = texto, cursor
	p.env.Refresh()
}

// Escalon cambia el escalón sin reconstruir el editor.
func (p *EditorPlano) Escalon(e tema.Escalon) {
	p.th.escalon = e
	p.env.Refresh()
}

// Desplazamiento es cuánto ha corrido a la izquierda su texto el editor desplazable para
// que el cursor se vea. Quien pinta DEBAJO el texto a trozos de colores —la fórmula
// tintada de una hoja, como el `scrollLeft` que la web copia del `<input>` a su fondo—
// lo necesita para que los trozos caigan bajo las letras.
//
// `widget.Entry` no lo publica: se lee con `reflect` de su campo `scroll` (solo se lee,
// y un valor, no un puntero). Si Fyne lo renombra, esto da 0 y
// `TestElDesplazamientoSeLee` se pone rojo.
func (p *EditorPlano) Desplazamiento() float32 {
	sc := reflect.ValueOf(&p.Entry).Elem().FieldByName("scroll")
	if !sc.IsValid() || sc.Kind() != reflect.Pointer || sc.IsNil() {
		return 0
	}
	x := sc.Elem().FieldByName("Offset").FieldByName("X")
	if !x.IsValid() || !x.CanFloat() {
		return 0
	}
	return float32(x.Float())
}

// Cara pone la letra con la que se escribe: la cara `r` —una familia que la suite no
// trae, o una de Figtree ya elegida— o, con nil, la de la suite con el estilo `s`. Es
// la celda de una hoja, que la web edita con su familia, su negrita y su cursiva. Pasa
// por `tema.EstiloDeCara` y no por el tema del envoltorio, o el cursor se colocaría
// midiendo otra letra.
func (p *EditorPlano) Cara(r fyne.Resource, s fyne.TextStyle) {
	if r != nil {
		s = tema.EstiloDeCara(r)
	}
	if p.TextStyle == s {
		return
	}
	p.TextStyle = s
	p.Refresh()
}
