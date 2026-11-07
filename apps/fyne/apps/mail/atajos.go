// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

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
	// Escape va por la ranura de teclas sueltas, la que reciben las teclas sin nadie
	// enfocado; lo que hubiera en ella sigue detrás. Las capas la envuelven al abrirse,
	// así que con un diálogo o un menú puesto Escape es suyo antes que de la selección.
	previa := c.OnTypedKey()
	c.SetOnTypedKey(func(k *fyne.KeyEvent) {
		if !b.TeclaSuelta(c, k) && previa != nil {
			previa(k)
		}
	})
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
	case 'x', 'l', 'v':
		return b.atajoDeSeleccion(r, hay)
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

// atajoDeSeleccion son «x» (marcar la del cursor), «l» (Etiquetar) y «v» (Mover a…).
// Los dos menús son del hilo abierto y del buzón propio.
func (b *Buzon) atajoDeSeleccion(r rune, hay bool) bool {
	switch {
	case r == 'x':
		return b.marcarCursor()
	case !hay || b.P.Bandeja.SoloLectura:
		return false
	case r == 'l':
		return b.P.Lector.AbrirEtiquetar()
	default:
		return b.P.Lector.AbrirMover()
	}
}

// TeclaSuelta atiende Escape: vacía la selección, y solo si hay algo marcado; sin
// selección la tecla sigue su camino. Devuelve si era suya.
func (b *Buzon) TeclaSuelta(c fyne.Canvas, k *fyne.KeyEvent) bool {
	if k == nil || k.Name != fyne.KeyEscape || b.redaccion != nil ||
		(c != nil && c.Overlays().Top() != nil) || b.P.Bandeja.CuantasMarcadas() == 0 {
		return false
	}
	b.quitarSeleccion()
	return true
}

// marcarCursor es «x»: la casilla de la conversación del cursor —la abierta, o la fila
// donde se quedó j/k—, como un clic en ella, ancla del rango incluida.
func (b *Buzon) marcarCursor() bool {
	convs := b.P.Bandeja.Conversaciones
	if b.P.Bandeja.SoloLectura || len(convs) == 0 {
		return false
	}
	i := min(b.cursorAtajo, len(convs)-1)
	if b.hiloID != "" {
		// La abierta manda; si no está en la lista (abierta desde un aviso del Hub),
		// no hay casilla que marcar, como en la web.
		i = slices.IndexFunc(convs, func(c Conversacion) bool { return c.HiloID == b.hiloID })
		if i < 0 {
			return false
		}
	}
	b.marcarFila(convs[i], i)
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

// verAtajos abre la hoja (`ui.MostrarHojaAtajos`) con los tres grupos de la web.
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
			fila("s", "shortcuts.star"), fila("x", "shortcuts.select"),
			fila("Esc", "shortcuts.clearSelection"), fila("l", "shortcuts.label"),
			fila("v", "shortcuts.move"), fila("?", "shortcuts.help"),
		}},
	}
	hoja := make([]ui.GrupoDeAtajos, 0, len(grupos))
	for _, g := range grupos {
		h := ui.GrupoDeAtajos{Titulo: g.titulo}
		for _, a := range g.atajos {
			h.Atajos = append(h.Atajos, ui.AtajoDeTeclado{Teclas: []string{a.teclas}, Descripcion: a.descripcion})
		}
		hoja = append(hoja, h)
	}
	ui.MostrarHojaAtajos(lienzo, t.T("shortcuts.title", nil), t.T("ui.close", nil), hoja)
}
