// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// El menú de la pestaña de hoja (`MenuPestana.tsx` y `pestana.ts`): renombrar, duplicar,
// mover un puesto, el color de la pestaña y eliminar. Antes renombrar solo se alcanzaba
// con doble clic y el color se leía y se exportaba, pero no se podía poner.
//
// En la web los colores son una rejilla de muestras dentro del mismo menú; el menú de
// aquí es una columna de acciones, así que «Color de pestaña» abre un segundo menú con
// las doce de la paleta de contenido y «Quitar color», cada una con su punto.

// ladoIconoMenu es el `<EditorIcon size="sm">` de las opciones: 14.
const ladoIconoMenu float32 = 14

// OpcionesPestana son las opciones vivas del menú (`opcionesDePestana`).
type OpcionesPestana struct {
	Renombrar, Duplicar, Color, Izquierda, Derecha, Eliminar bool
}

// OpcionesPestana dice qué se puede hacer con la hoja i desde su menú. Con el libro
// protegido (o en solo lectura) todo es estructura —también el color, como en Excel—
// y sale inerte; mover depende además del sitio, y eliminar de que quede otra hoja.
func (c *Lienzo) OpcionesPestana(i int) OpcionesPestana {
	n := len(c.Libro.Hojas())
	libre := !c.Rejilla.SoloLectura && c.Libro.ProteccionLibro() == nil && i >= 0 && i < n
	return OpcionesPestana{
		Renombrar: libre, Duplicar: libre, Color: libre,
		Izquierda: libre && i > 0, Derecha: libre && i < n-1, Eliminar: libre && n > 1,
	}
}

// AbrirMenuHoja activa la hoja i, si no lo estaba, y despliega su menú colgando del ▾.
func (p *Pestanas) AbrirMenuHoja(i int) {
	lz := p.lz
	if i < 0 || i >= len(lz.Libro.Hojas()) || (p.menu != nil && p.menu.Abierto()) {
		return
	}
	lz.CambiarHoja(i)
	p.Refresh() // el ▾ se coloca junto a la pestaña que ahora es la activa
	op := lz.OpcionesPestana(i)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	accion := func(clave string, ico iconos.Ed, vivo bool, fn func()) *ui.ElementoMenu {
		e := ui.NuevoElementoMenu(lz.t(clave, nil), iconos.Ninguno, fn)
		e.Denso, e.Inerte = true, !vivo // `size="sm"`, `disabled`
		e.Glifo, e.TamIcono = iconos.TrazoEd(ico, ladoIconoMenu, "text-muted", v), ladoIconoMenu
		return e
	}
	color := ui.NuevoElementoMenu(lz.t("sheet.tabColor", nil), iconos.Ninguno, func() { p.abrirMenuColores(i) })
	color.Denso, color.Inerte = true, !op.Color
	color.TamIcono = ladoIconoMenu
	if c, ok := editor.Hex(lz.Libro.ColorPestana(i)); ok {
		color.PuntoColor = c
	} else {
		color.Glifo = iconos.TrazoEd(iconos.EdColorTexto, ladoIconoMenu, "text-muted", v)
	}
	borrar := accion("sheet.delete", iconos.EdPapelera, op.Eliminar, lz.PedirBorrarHoja)
	borrar.Peligro = true
	borrar.Glifo = iconos.TrazoEd(iconos.EdPapelera, ladoIconoMenu, "danger-text", v)
	p.mostrarMenu(ui.NuevoMenu(
		accion("sheet.rename", iconos.EdLapiz, op.Renombrar, func() { lz.PedirRenombrarHoja(i) }),
		accion("sheet.duplicate", iconos.EdCopiar, op.Duplicar, lz.DuplicarHoja),
		accion("sheet.moveLeft", iconos.EdChevronIzq, op.Izquierda, func() { lz.MoverHoja(i, i-1) }),
		accion("sheet.moveRight", iconos.EdChevronDer, op.Derecha, func() { lz.MoverHoja(i, i+1) }),
		ui.NuevoSeparadorMenu(),
		color,
		ui.NuevoSeparadorMenu(),
		borrar,
	), lz.t("sheet.tab.menu", i18n.Vars{"name": lz.Libro.Hojas()[i]}))
}

// abrirMenuColores es la paleta de la pestaña: «Quitar color» y las doce de contenido,
// con un tic en la vigente. La vigente se compara sin mayúsculas: lo que llega de un
// .xlsx o de un libro guardado antes puede venir en minúsculas (`#dc2626`).
func (p *Pestanas) abrirMenuColores(i int) {
	lz := p.lz
	actual := lz.Libro.ColorPestana(i)
	ninguno := ui.NuevoElementoMenu(lz.t("palette.none", nil), iconos.Cerrar, func() { lz.PonerColorPestana(i, "") })
	ninguno.Denso, ninguno.TamIcono, ninguno.Marcada = true, ui.LadoPuntoMenu, actual == ""
	elems := make([]fyne.CanvasObject, 0, 1+len(editor.PaletaContenido))
	elems = append(elems, ninguno)
	for _, hex := range editor.PaletaContenido {
		e := ui.NuevoElementoMenu(lz.t("palette.color", i18n.Vars{"hex": hex}), iconos.Ninguno,
			func() { lz.PonerColorPestana(i, hex) })
		e.Denso, e.TamIcono, e.Marcada = true, ui.LadoPuntoMenu, strings.EqualFold(actual, hex)
		e.PuntoColor, _ = editor.Hex(hex)
		elems = append(elems, e)
	}
	p.mostrarMenu(ui.NuevoMenu(elems...), lz.t("sheet.tabColor", nil))
}

// mostrarMenu lo cuelga del ▾, hacia arriba (la barra está al pie) y alineado a su
// izquierda, como `<Dropdown align="start" side="top">`.
func (p *Pestanas) mostrarMenu(m *ui.Menu, rotulo string) {
	m.AlInicio, m.Arriba, m.Rotulo = true, true, rotulo
	p.menu = m
	m.Mostrar(p.tira.chevron)
}

// MenuHoja es el menú de la pestaña si está abierto (para las pruebas y el cromo).
func (p *Pestanas) MenuHoja() *ui.Menu {
	if p.menu == nil || !p.menu.Abierto() {
		return nil
	}
	return p.menu
}

// ---- el ▾ ----

// chevronPestana es el botón ▾ que sigue a la pestaña activa. Con el dedo mide 44 de
// ancho (`pointer-coarse:min-w-[max(44px,2.75rem)]`): es lo único que abre el menú en
// un teléfono, donde no hay clic derecho que valga.
type chevronPestana struct {
	widget.BaseWidget

	tira             *tiraHojas
	encima, enfocado bool
}

var (
	_ fyne.Tappable     = (*chevronPestana)(nil)
	_ fyne.Focusable    = (*chevronPestana)(nil)
	_ fyne.Accessible   = (*chevronPestana)(nil)
	_ desktop.Hoverable = (*chevronPestana)(nil)
)

func (ch *chevronPestana) Tapped(*fyne.PointEvent) {
	ch.tira.p.AbrirMenuHoja(ch.tira.p.lz.Libro.Activa())
}
func (ch *chevronPestana) FocusGained()                   { ch.enfocado = true; ch.Refresh() }
func (ch *chevronPestana) FocusLost()                     { ch.enfocado = false; ch.Refresh() }
func (ch *chevronPestana) MouseIn(*desktop.MouseEvent)    { ch.encima = true; ch.Refresh() }
func (ch *chevronPestana) MouseMoved(*desktop.MouseEvent) {}
func (ch *chevronPestana) MouseOut()                      { ch.encima = false; ch.Refresh() }

// TypedRune y TypedKey: Espacio e Intro pulsan, como en cualquier `<button>`.
func (ch *chevronPestana) TypedRune(r rune) {
	if r == ' ' {
		ch.Tapped(nil)
	}
}

func (ch *chevronPestana) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(ch, e) {
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		ch.Tapped(nil)
	}
}

// AccessibilityLabel es el `aria-label`: «Opciones de la hoja: Ventas».
func (ch *chevronPestana) AccessibilityLabel() string {
	lz := ch.tira.p.lz
	return lz.t("sheet.tab.menu", i18n.Vars{"name": lz.Libro.Hoja()})
}

// AccessibilityRole es botón.
func (ch *chevronPestana) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

// MinSize es el `w-5`, con el suelo táctil de ancho.
func (ch *chevronPestana) MinSize() fyne.Size {
	ch.ExtendBaseWidget(ch)
	return fyne.NewSize(ui.SueloTactil(AnchoChevronPestana), altoContenidoTira)
}

// CreateRenderer compone el aro y el ▾.
func (ch *chevronPestana) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &chevronRenderer{
		ch:    ch,
		aro:   ui.NuevoAro(tema.RadioSM, ui.SinHueco, v), // `sytue-focus-ring rounded-sm`
		icono: iconos.Trazo(iconos.ChevronAbajo, LadoIconoChevron, "text-muted", v),
	}
	r.objetos = r.aro.Antes(r.icono.Objeto())
	r.Refresh()
	return r
}

type chevronRenderer struct {
	ch      *chevronPestana
	aro     *ui.Aro
	icono   *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *chevronRenderer) MinSize() fyne.Size           { return r.ch.MinSize() }
func (r *chevronRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *chevronRenderer) Destroy()                     {}

func (r *chevronRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.icono.Colocar((t.Width-LadoIconoChevron)/2, (t.Height-LadoIconoChevron)/2, LadoIconoChevron)
}

func (r *chevronRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// `text-text-muted hover:text-text`
	token := "text-muted"
	if r.ch.encima {
		token = "text"
	}
	r.icono.Tenir(token, v)
	r.aro.Pintar(v)
	r.aro.Ver(r.ch.enfocado)
	r.Layout(r.ch.Size())
	canvas.Refresh(r.icono.Objeto())
}
