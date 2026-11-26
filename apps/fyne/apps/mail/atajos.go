// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"runtime"
	"slices"
	"time"

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
			b.verAjustes()
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
	// «g» y la letra de la carpeta, antes que los atajos sueltos («a» responde a todos
	// sola, pero tras la «g» es Archivo). Vale también desde los ajustes.
	if ir, ok := b.atajoDeIr(r); ok {
		return ir
	}
	// En la página de ajustes no hay lista ni lector: j/k, archivar o destacar
	// actuarían sobre una conversación que no se ve. Quedan la ayuda y redactar,
	// como en la web (`enAjustes` en MailApp.tsx).
	if b.P.Ajustes != nil && r != '?' && r != 'c' {
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
		return b.atajoDeRespuesta(r, abierta, hay)
	case 'x', 'l', 'v':
		return b.atajoDeSeleccion(r, hay)
	case 'I', 'U', '!', 'z':
		return b.atajoDeTriaje(r, abierta, hay)
	case 's', 'e', '#':
		return atajoDelHilo(r, a, abierta, hay)
	default:
		return false
	}
	return true
}

// atajoDelHilo son «s» (destacar), «e» (archivar) y «#» (a la papelera) sobre el hilo
// abierto: sin hilo, o sin la acción en esta vista, no son nada.
func atajoDelHilo(r rune, a *Acciones, abierta Conversacion, hay bool) bool {
	h := map[rune]func(Conversacion){'s': a.Destacar, 'e': a.Archivar, '#': a.Papelera}[r]
	if !hay || h == nil {
		return false
	}
	h(abierta)
	return true
}

// RolDeIr es `ROL_DE_IR` de la web: la letra tras «g» y el rol de la carpeta a la que
// lleva (por rol, no por nombre: «Recibidos» se llama distinto en cada cuenta).
var RolDeIr = map[rune]string{'i': "inbox", 't': "sent", 'd': "drafts", 'a': "archive"}

// VentanaIr es `VENTANA_IR_MS`: lo que se espera la segunda tecla.
const VentanaIr = 1500 * time.Millisecond

// atajoDeIr atiende la «g» y la tecla que la sigue. El segundo valor dice si la tecla
// era de «ir a» (y el primero, si se usó); si no, sigue su camino como atajo suelto.
func (b *Buzon) atajoDeIr(r rune) (usada, era bool) {
	ahora := time.Now()
	pendiente := b.irPendiente
	b.irPendiente = time.Time{}
	if rol, ok := RolDeIr[r]; ok && !pendiente.IsZero() && ahora.Sub(pendiente) <= VentanaIr {
		c, hay := b.carpetaDeRol(rol)
		if hay {
			b.ElegirCarpeta(c)
		}
		return hay, true
	}
	if r == 'g' {
		b.irPendiente = ahora
		return true, true
	}
	return false, false
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

// atajoDeRespuesta es «r», «a» y «f»: responder, responder a todos y reenviar el
// último mensaje de la conversación abierta.
func (b *Buzon) atajoDeRespuesta(r rune, abierta Conversacion, hay bool) bool {
	if !hay {
		return false
	}
	modo := map[rune]Respuesta{'r': Responder, 'a': ResponderATodos, 'f': Reenviar}[r]
	b.responder(abierta.Mensajes[0], modo)
	return true
}

// atajoDeTriaje es `atajoDeTriaje` de la web (utils.ts): `Mayús+I` marca leída, `Mayús+U`
// no leída, «!» manda a spam —la abierta o, sin nada abierto, la fila del cursor de
// j/k— y «z» deshace el último triaje. Marcar leída la que ya lo está, o no leída la que
// no, no es atajo: conmutar haría lo contrario de lo pedido. En solo lectura no hay
// triaje (las Acciones ya están a nil, pero «z» no pasa por ellas).
func (b *Buzon) atajoDeTriaje(r rune, abierta Conversacion, hay bool) bool {
	if b.P.Bandeja.SoloLectura {
		return false
	}
	if r == 'z' {
		if b.ultimoTriaje == nil {
			return false
		}
		b.deshacerTriaje(b.ultimoTriaje)
		return true
	}
	c := abierta
	if !hay {
		convs := b.P.Bandeja.Conversaciones
		if len(convs) == 0 {
			return false
		}
		c = convs[min(b.cursorAtajo, len(convs)-1)]
	}
	a := b.P.Acciones
	switch {
	case r == '!' && a.Spam != nil:
		a.Spam(c)
	case (r == 'I' && c.SinLeer || r == 'U' && !c.SinLeer) && a.Leido != nil:
		a.Leido(c)
	default:
		return false
	}
	return true
}

// teclaMayus es la de la web: «⇧» en un Mac y el nombre de la tecla en su idioma en
// los demás.
func (b *Buzon) teclaMayus() string {
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		return "⇧"
	}
	return b.P.T.T("shortcuts.key.shift", nil)
}

// TeclaSuelta atiende Escape: vacía la selección, y solo si hay algo marcado; sin
// selección la tecla sigue su camino. Devuelve si era suya.
func (b *Buzon) TeclaSuelta(c fyne.Canvas, k *fyne.KeyEvent) bool {
	if k == nil || k.Name != fyne.KeyEscape || b.redaccion != nil || b.P.Ajustes != nil ||
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

// moverCursor es j/k y los botones «Más reciente»/«Más antigua» del lector: la
// conversación siguiente o la anterior. Sin nada abierto se retoma donde estaba el
// cursor: tras archivar, esa posición ya la ocupa la siguiente (como Gmail). Bajar
// desde la última cargada, si quedan páginas, pide la siguiente y abre la que llega
// (`seguirTrasPagina`), como `pasoDelCursor` de la web.
func (b *Buzon) moverCursor(siguiente bool) {
	convs := b.P.Bandeja.Conversaciones
	if len(convs) == 0 {
		return
	}
	actual := b.indiceAbierta()
	if siguiente && actual == len(convs)-1 && b.P.Bandeja.HayMas {
		b.avanceTrasCargar = b.hiloID
		b.MasPagina()
		b.ponerNavegacion()
		b.P.Lector.Refresh()
		return
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

// indiceAbierta es la fila de la conversación abierta en la lista; -1 si no hay
// ninguna o no está a la vista (se llegó por enlace, o el filtro la dejó fuera).
func (b *Buzon) indiceAbierta() int {
	if b.hiloID == "" {
		return -1
	}
	return slices.IndexFunc(b.P.Bandeja.Conversaciones, func(c Conversacion) bool { return c.HiloID == b.hiloID })
}

// ponerNavegacion enciende o apaga los botones del lector: la más reciente no tiene
// nada encima, y la más antigua solo tiene algo debajo si quedan páginas
// (`vecinasDeLaAbierta` de la web).
func (b *Buzon) ponerNavegacion() {
	l, i, n := b.P.Lector, b.indiceAbierta(), len(b.P.Bandeja.Conversaciones)
	l.HayAnterior = i > 0
	l.HaySiguiente = i >= 0 && (i < n-1 || b.P.Bandeja.HayMas)
	l.CargandoSiguiente = b.avanceTrasCargar != "" && b.pidiendo
}

// seguirTrasPagina termina el «Más antigua» que pidió la página siguiente: cuando
// llega (o falla), abre la conversación de detrás de la que estaba abierta, si la
// hay. Si entretanto se abrió otra, o la página no trajo nada nuevo, se olvida.
func (b *Buzon) seguirTrasPagina() {
	pendiente := b.avanceTrasCargar
	if pendiente == "" || b.pidiendo {
		return
	}
	b.avanceTrasCargar = ""
	convs := b.P.Bandeja.Conversaciones
	if i := b.indiceAbierta(); b.hiloID == pendiente && i >= 0 && i+1 < len(convs) {
		b.cursorAtajo = i + 1
		b.Abrir(convs[i+1])
		b.P.Bandeja.VerFila(i + 1)
		return
	}
	b.ponerNavegacion()
	b.P.Lector.Refresh()
}

// ---- La hoja de atajos (`ShortcutsModal.tsx`) ----

// grupoAtajos es un apartado de la hoja.
type grupoAtajos struct {
	titulo string
	atajos []filaAtajo
}

// filaAtajo es un atajo de la hoja: sus teclas, una pastilla cada una, y qué hace.
type filaAtajo struct {
	teclas      []string
	descripcion string
}

// verAtajos abre la hoja (`ui.MostrarHojaAtajosDeSuite`) con los tres grupos de la web.
func (b *Buzon) verAtajos() {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return
	}
	t := b.P.T
	fila := func(tecla, clave string) filaAtajo {
		return filaAtajo{[]string{tecla}, t.T(clave, nil)}
	}
	mayus := func(tecla, clave string) filaAtajo {
		return filaAtajo{[]string{b.teclaMayus(), tecla}, t.T(clave, nil)}
	}
	ir := func(tecla, clave string) filaAtajo {
		return filaAtajo{[]string{"g", tecla}, t.T(clave, nil)}
	}
	grupos := []grupoAtajos{
		{t.T("shortcuts.group.move", nil), []filaAtajo{
			fila("j", "shortcuts.next"), fila("k", "shortcuts.prev"),
			fila("u", "shortcuts.backToList"), fila("/", "shortcuts.search"),
			ir("i", "shortcuts.goInbox"), ir("t", "shortcuts.goSent"),
			ir("d", "shortcuts.goDrafts"), ir("a", "shortcuts.goArchive"),
		}},
		{t.T("shortcuts.group.write", nil), []filaAtajo{
			fila("c", "shortcuts.compose"), fila("r", "shortcuts.reply"), fila("a", "shortcuts.replyAll"),
			fila("f", "shortcuts.forward"), fila(AtajoEnviar(), "shortcuts.send"),
		}},
		{t.T("shortcuts.group.triage", nil), []filaAtajo{
			fila("e", "shortcuts.archive"), fila("#", "shortcuts.trash"),
			fila("!", "shortcuts.spam"), fila("z", "shortcuts.undo"),
			mayus("I", "shortcuts.markRead"), mayus("U", "shortcuts.markUnread"),
			fila("s", "shortcuts.star"), fila("x", "shortcuts.select"),
			fila("Esc", "shortcuts.clearSelection"), fila("l", "shortcuts.label"),
			fila("v", "shortcuts.move"), fila("?", "shortcuts.help"),
		}},
	}
	hoja := make([]ui.GrupoDeAtajos, 0, len(grupos))
	for _, g := range grupos {
		h := ui.GrupoDeAtajos{Titulo: g.titulo}
		for _, a := range g.atajos {
			h.Atajos = append(h.Atajos, ui.AtajoDeTeclado{Teclas: a.teclas, Descripcion: a.descripcion})
		}
		hoja = append(hoja, h)
	}
	// La hoja es la de la suite: los de Mail y, detrás, ⌘K, «?» y Esc.
	ui.MostrarHojaAtajosDeSuite(lienzo, t.Locale(), hoja)
}
