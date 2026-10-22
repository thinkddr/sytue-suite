// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Los atajos de teclado (MailApp.tsx, «Atajos de teclado»)
//
// Los que cualquiera trae de Gmail y Apple Mail: sin ellos hay que ir al ratón para
// CADA mensaje. Van por `SetOnTypedRune`, que Fyne solo dispara cuando NO hay nada
// enfocado —teclear «e» en el asunto no archiva nada, que es `esAtajoDeApp`—, y con un
// diálogo o un menú abiertos no hacen nada: el teclado es de la capa.

// AtajoEnviar es `ATAJO_ENVIAR`: el de enviar con los símbolos del teclado, que no hay
// que traducir.
func AtajoEnviar() string { return ui.AtajoDelSistema("Ctrl+↵") }

// AtajoPersonalizado atiende atajos con modificador (p. ej. Cmd+, para ajustes).
func (b *Buzon) AtajoPersonalizado(s fyne.Shortcut) bool {
	if cs, ok := s.(*desktop.CustomShortcut); ok {
		if cs.KeyName == fyne.KeyComma && cs.Modifier&fyne.KeyModifierShortcutDefault != 0 {
			b.P.Menu.AjustesAbiertos = true
			b.P.Refrescar()
			return true
		}
	}
	return false
}

// Atajos engancha el teclado de la web al lienzo de la ventana.
func (b *Buzon) Atajos(c fyne.Canvas) {
	c.SetOnTypedRune(func(r rune) { b.Atajo(c, r) })
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierShortcutDefault}, func(s fyne.Shortcut) {
		b.AtajoPersonalizado(s)
	})
}

// Atajo atiende una tecla suelta. Devuelve si era suya.
func (b *Buzon) Atajo(c fyne.Canvas, r rune) bool {
	if b.redaccion != nil || (c != nil && c.Overlays().Top() != nil) {
		return false
	}
	abierta := b.P.Lector.Conversacion
	hay := len(abierta.Mensajes) > 0 && b.hiloID != ""
	a := b.P.Acciones
	switch r {
	case '?':
		b.verAtajos()
	case 'c':
		b.Redactar()
	case '/':
		b.P.Bandeja.EnfocarBusqueda()
	case 'j', 'k':
		b.moverCursor(r == 'j')
	case 'u': // volver a la lista; Escape ya es de los diálogos y los menús
		if !hay {
			return false
		}
		b.cerrarHilo()
		b.P.Refrescar()
	case 'r', 'a', 'f':
		if !hay {
			return false
		}
		modo := map[rune]Respuesta{'r': Responder, 'a': ResponderATodos, 'f': Reenviar}[r]
		b.responder(abierta.Mensajes[0], modo)
	case 's', 'e', '#':
		h := map[rune]func(Conversacion){'s': a.Destacar, 'e': a.Archivar, '#': a.Papelera}[r]
		if !hay || h == nil {
			return false
		}
		h(abierta)
	default:
		return false
	}
	return true
}

// moverCursor es j/k: la conversación siguiente o la anterior. Sin nada abierto se
// retoma donde estaba el cursor: tras archivar, esa posición ya la ocupa la siguiente
// (como Gmail).
func (b *Buzon) moverCursor(siguiente bool) {
	convs := b.P.Bandeja.Conversaciones
	if len(convs) == 0 {
		return
	}
	actual := -1
	for i, c := range convs {
		if c.HiloID == b.hiloID && b.hiloID != "" {
			actual = i
		}
	}
	destino := min(b.cursorAtajo, len(convs)-1)
	switch {
	case actual >= 0 && siguiente:
		destino = min(actual+1, len(convs)-1)
	case actual >= 0:
		destino = max(actual-1, 0)
	}
	b.cursorAtajo = destino
	if convs[destino].HiloID != b.hiloID {
		b.Abrir(convs[destino])
	}
	b.P.Bandeja.VerFila(destino)
}

// ---- La hoja de atajos (`ShortcutsModal.tsx`) ----

// grupoAtajos es un apartado de la hoja.
type grupoAtajos struct {
	titulo string
	atajos []struct{ teclas, descripcion string }
}

// verAtajos abre la hoja. Es un diálogo `md` con los tres grupos de la web y el botón
// «Cerrar» en el pie.
func (b *Buzon) verAtajos() {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return
	}
	t := b.P.T
	fila := func(teclas, clave string) struct{ teclas, descripcion string } {
		return struct{ teclas, descripcion string }{teclas, t.T(clave, nil)}
	}
	grupos := []grupoAtajos{
		{t.T("shortcuts.group.move", nil), []struct{ teclas, descripcion string }{
			fila("j", "shortcuts.next"), fila("k", "shortcuts.prev"),
			fila("u", "shortcuts.backToList"), fila("/", "shortcuts.search"),
		}},
		{t.T("shortcuts.group.write", nil), []struct{ teclas, descripcion string }{
			fila("c", "shortcuts.compose"), fila("r", "shortcuts.reply"), fila("a", "shortcuts.replyAll"),
			fila("f", "shortcuts.forward"), fila(AtajoEnviar(), "shortcuts.send"),
		}},
		{t.T("shortcuts.group.triage", nil), []struct{ teclas, descripcion string }{
			fila("e", "shortcuts.archive"), fila("#", "shortcuts.trash"),
			fila("s", "shortcuts.star"), fila("?", "shortcuts.help"),
		}},
	}
	ancho := AnchoCuerpoDialogo(ui.DialogoMD, lienzo.Size().Width)
	secciones := make([]fyne.CanvasObject, 0, len(grupos))
	for _, g := range grupos {
		secciones = append(secciones, nuevaSeccionAtajos(g, ancho))
	}
	var d *ui.Dialogo
	cerrar := ui.NuevoBoton(t.T("ui.close", nil), func() { d.Cerrar() })
	d = ui.NuevoDialogo(t.T("shortcuts.title", nil), container.New(ui.Columna{Hueco: huecoGruposAtajos}, secciones...)).
		Con(ui.DialogoMD, cerrar)
	d.RotuloCerrar = t.T("ui.close", nil)
	d.Mostrar(lienzo)
}

// Las medidas de la hoja: grupos con `gap-5`, el rótulo en `sytue-overline mb-2`, filas
// `flex items-center gap-4 py-1.5` separadas por `divide-y` a `border/60`, y cada tecla
// `h-6 min-w-6 px-1.5 rounded border bg-bg-subtle text-xs` con `gap-1` entre ellas.
const (
	huecoGruposAtajos float32 = 5 * 0.25 * tema.Rem
	bajoRotuloAtajos  float32 = 2 * 0.25 * tema.Rem
	rellenoFilaAtajo  float32 = 1.5 * 0.25 * tema.Rem
	huecoFilaAtajo    float32 = 4 * 0.25 * tema.Rem
	ladoTecla         float32 = 6 * 0.25 * tema.Rem
)

// AltoFilaAtajo es una fila: su `py-1.5` y la tecla de 21 (que manda sobre el texto).
var AltoFilaAtajo = 2*rellenoFilaAtajo + ladoTecla

// seccionAtajos es un grupo de la hoja.
type seccionAtajos struct {
	widget.BaseWidget
	g     grupoAtajos
	ancho float32
}

func nuevaSeccionAtajos(g grupoAtajos, ancho float32) *seccionAtajos {
	s := &seccionAtajos{g: g, ancho: ancho}
	s.ExtendBaseWidget(s)
	return s
}

func (s *seccionAtajos) MinSize() fyne.Size {
	n := float32(len(s.g.atajos))
	return fyne.NewSize(s.ancho, tema.TextoXS.Linea+bajoRotuloAtajos+n*AltoFilaAtajo+max(n-1, 0)*tema.Borde)
}

func (s *seccionAtajos) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &seccionAtajosRenderer{s: s, rotulo: ui.NuevoVersal(s.g.titulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))}
	r.objetos = append(r.objetos, r.rotulo.Objetos()...)
	for i, a := range s.g.atajos {
		if i > 0 {
			f := canvas.NewRectangle(veloDe(tema.Color("border", v), 0.6))
			r.filetes = append(r.filetes, f)
			r.objetos = append(r.objetos, f)
		}
		tecla := ui.NuevaTecla(a.teclas, tema.TextoXS, ui.Normal)
		tecla.Fondo, tecla.Tinta, tecla.Borde = "bg-subtle", "text-muted", "border"
		tecla.RellenoX, tecla.Radio = tema.Espacio(1.5), tema.RadioSM
		tecla.Minimo = fyne.NewSize(ladoTecla, ladoTecla)
		texto := ui.Texto(a.descripcion, tema.TextoSM, ui.Normal, tema.Color("text", v))
		r.teclas, r.textos = append(r.teclas, tecla), append(r.textos, texto)
		r.objetos = append(r.objetos, tecla, texto)
	}
	r.Refresh()
	return r
}

type seccionAtajosRenderer struct {
	s       *seccionAtajos
	rotulo  *ui.Versal
	filetes []*canvas.Rectangle
	teclas  []*ui.Tecla
	textos  []*canvas.Text
	objetos []fyne.CanvasObject
}

func (r *seccionAtajosRenderer) MinSize() fyne.Size           { return r.s.MinSize() }
func (r *seccionAtajosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *seccionAtajosRenderer) Destroy()                     {}
func (r *seccionAtajosRenderer) Layout(t fyne.Size) {
	r.rotulo.Colocar(0, 0)
	y := tema.TextoXS.Linea + bajoRotuloAtajos
	for i := range r.teclas {
		if i > 0 {
			r.filetes[i-1].Move(fyne.NewPos(0, y))
			r.filetes[i-1].Resize(fyne.NewSize(t.Width, tema.Borde))
			y += tema.Borde
		}
		m := r.teclas[i].MinSize()
		r.teclas[i].Move(fyne.NewPos(0, y+rellenoFilaAtajo))
		r.teclas[i].Resize(m)
		ui.ColocarTexto(r.textos[i], tema.TextoSM, m.Width+huecoFilaAtajo, y+(AltoFilaAtajo-tema.TextoSM.Linea)/2)
		y += AltoFilaAtajo
	}
}

func (r *seccionAtajosRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.rotulo.Pintar(tema.Color("text-subtle", v))
	for _, f := range r.filetes {
		f.FillColor = veloDe(tema.Color("border", v), 0.6)
	}
	for _, x := range r.textos {
		x.Color = tema.Color("text", v)
	}
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// veloDe es un color con su opacidad multiplicada: el `/60` de Tailwind.
func veloDe(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(float32(c.A) * a)
	return c
}
