// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// La llamada entrante: lo que se ve —y, en un directo, lo que suena— cuando alguien
// empieza una llamada en un canal que NO estás mirando. La trama `call_started` ya
// llegaba (franjallamada.go enciende el punto del carril); sin esto, a quien te llamaba
// por un directo no lo veías hasta abrir la conversación.

// AvisoEntrante es lo que hay que pintar cuando alguien llama (`AvisoEntrante` de
// `lib/llamadaEntrante.ts`).
type AvisoEntrante struct {
	CanalID  string
	Etiqueta string
	// Directo además SUENA; una llamada de canal solo se ve.
	Directo bool
}

// SituacionLlamada es lo que hace falta para decidir si una llamada avisa.
type SituacionLlamada struct {
	// CanalID es donde ha empezado la llamada; Abierto, el canal que se está mirando.
	CanalID, Abierto string
	// EnLlamada dice si quien mira ya está dentro de una llamada.
	EnLlamada bool
	// Canales son los suyos, para el nombre, el tipo y el silencio.
	Canales []Canal
}

// AvisoDeLlamada es `avisoDeLlamada` de la web: el aviso, o false si esta llamada no
// debe interrumpir. No se avisa cuando
//   - es el canal abierto (la franja de «Llamada en curso» ya lo cuenta);
//   - ya se está en otra llamada (dos timbres a la vez no ayudan);
//   - el canal está silenciado (ya dijo que no quiere que le interrumpan);
//   - el canal no está en su lista (no es suyo, o aún no ha cargado).
//
// Es una función y no un método porque son cuatro reglas que se rompen en silencio:
// quitar una no da error, solo suena cuando no debía.
func AvisoDeLlamada(s SituacionLlamada) (AvisoEntrante, bool) {
	if s.CanalID == "" || s.CanalID == s.Abierto || s.EnLlamada {
		return AvisoEntrante{}, false
	}
	i := slices.IndexFunc(s.Canales, func(c Canal) bool { return c.ID == s.CanalID })
	if i < 0 || s.Canales[i].Silenciado {
		return AvisoEntrante{}, false
	}
	c := s.Canales[i]
	return AvisoEntrante{CanalID: c.ID, Etiqueta: c.Nombre, Directo: c.Clase == Directo}, true
}

// LlamadaEntrante es la tarjeta de IncomingCall.tsx: el nombre, «te está llamando» (o
// «Ha empezado una llamada en este canal») y «Contestar» y «Rechazar».
//
// `sytue-popover-surface fixed top-4 right-4 w-72 rounded-lg border p-3 shadow-lg`. Va
// arriba a la derecha y NO es un diálogo modal a propósito: una llamada no puede
// secuestrar lo que estabas escribiendo, así que no pide el foco.
type LlamadaEntrante struct {
	widget.BaseWidget

	T     *i18n.Translator
	Aviso AvisoEntrante
	// AlContestar y AlRechazar se llaman al pulsar; se pueden poner después.
	AlContestar, AlRechazar func()

	sombra              *ui.Sombra
	caja                *ui.Caja
	nombre, meta        *canvas.Text
	contestar, rechazar *ui.Boton
}

var (
	_ fyne.Widget     = (*LlamadaEntrante)(nil)
	_ fyne.Accessible = (*LlamadaEntrante)(nil)
)

var (
	// AnchoLlamadaEntrante es su `w-72`.
	AnchoLlamadaEntrante = tema.Espacio(72)
	// MargenLlamadaEntrante es el `top-4 right-4` hasta el borde de la ventana.
	MargenLlamadaEntrante = tema.Espacio(4)
	// RellenoLlamadaEntrante es el `p-3`.
	RellenoLlamadaEntrante = tema.Espacio(3)
	// Las separaciones del rótulo: `mt-0.5` y `mb-2.5`; entre botones, `gap-2`.
	sepMetaEntrante    = tema.Espacio(0.5)
	sepBotonesEntrante = tema.Espacio(2.5)
	huecoEntrante      = tema.Espacio(2)
)

// NuevaLlamadaEntrante crea la tarjeta, escondida hasta que haya a quién avisar.
func NuevaLlamadaEntrante(t *i18n.Translator) *LlamadaEntrante {
	l := &LlamadaEntrante{T: t, sombra: ui.NuevaSombra(tema.SombraLG, tema.RadioLG)}
	// «Contestar» primero y a la izquierda: es lo que se busca con prisa.
	l.contestar = ui.NuevoBoton(t.T("call.answer", nil), func() {
		if l.AlContestar != nil {
			l.AlContestar()
		}
	}).Con(ui.Primario, ui.TallaSM)
	l.rechazar = ui.NuevoBoton(t.T("call.decline", nil), func() {
		if l.AlRechazar != nil {
			l.AlRechazar()
		}
	}).Con(ui.Secundario, ui.TallaSM)
	l.ExtendBaseWidget(l)
	l.Hide()
	return l
}

// Contestar y Rechazar son los botones, para los guardianes.
func (l *LlamadaEntrante) Contestar() *ui.Boton { return l.contestar }

// Rechazar es «Rechazar».
func (l *LlamadaEntrante) Rechazar() *ui.Boton { return l.rechazar }

// Rotulo es lo que dice el lector de pantalla: «Llamada de Ana» o «Llamada en equipo».
func (l *LlamadaEntrante) Rotulo() string {
	clave := "call.incomingChannel"
	if l.Aviso.Directo {
		clave = "call.incomingDirect"
	}
	return l.T.T(clave, i18n.Vars{"name": l.Aviso.Etiqueta})
}

// Detalle es la línea de debajo del nombre.
func (l *LlamadaEntrante) Detalle() string {
	if l.Aviso.Directo {
		return l.T.T("call.incomingDirectShort", nil)
	}
	return l.T.T("call.incomingChannelShort", nil)
}

// AccessibilityRole: un grupo con nombre (`role="alertdialog"` sin mover el foco).
func (l *LlamadaEntrante) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

// AccessibilityLabel es el propio aviso.
func (l *LlamadaEntrante) AccessibilityLabel() string { return l.Rotulo() }

// altoBotonesEntrante es la fila de botones `sm`, con el dedo si toca.
func (l *LlamadaEntrante) altoBotones() float32 {
	return max(l.contestar.MinSize().Height, l.rechazar.MinSize().Height)
}

// MinSize es la tarjeta: bordes, `p-3`, el nombre, la línea de debajo y los botones.
func (l *LlamadaEntrante) MinSize() fyne.Size {
	l.ExtendBaseWidget(l)
	alto := 2*tema.Borde + 2*RellenoLlamadaEntrante + tema.TextoSM.Linea + sepMetaEntrante +
		tema.TextoXS.Linea + sepBotonesEntrante + l.altoBotones()
	return fyne.NewSize(AnchoLlamadaEntrante, alto)
}

// CreateRenderer compone la caja, los dos textos y los dos botones.
func (l *LlamadaEntrante) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l.caja = ui.NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), tema.RadioLG)
	l.nombre = ui.Texto(l.Aviso.Etiqueta, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	// `.sytue-meta`: `text-xs` en `text-subtle`.
	l.meta = ui.Texto(l.Detalle(), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	r := &llamadaEntranteRenderer{l: l}
	r.Refresh()
	return r
}

type llamadaEntranteRenderer struct{ l *LlamadaEntrante }

func (r *llamadaEntranteRenderer) MinSize() fyne.Size { return r.l.MinSize() }
func (r *llamadaEntranteRenderer) Destroy()           {}

func (r *llamadaEntranteRenderer) Objects() []fyne.CanvasObject {
	l := r.l
	return append(l.caja.Objetos(), l.nombre, l.meta, l.contestar, l.rechazar)
}

// Layout: el nombre truncado, su línea y los dos botones a su ancho, de izquierda a
// derecha.
func (r *llamadaEntranteRenderer) Layout(t fyne.Size) {
	l := r.l
	l.caja.Redimensionar(t)
	x := tema.Borde + RellenoLlamadaEntrante
	cabe := max(t.Width-2*x, 0)
	y := tema.Borde + RellenoLlamadaEntrante
	ui.Truncar(l.nombre, l.Aviso.Etiqueta, tema.TextoSM, ui.SemiNegrita, cabe)
	ui.ColocarTexto(l.nombre, tema.TextoSM, x, y)
	y += tema.TextoSM.Linea + sepMetaEntrante
	ui.Truncar(l.meta, l.Detalle(), tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(l.meta, tema.TextoXS, x, y)
	y += tema.TextoXS.Linea + sepBotonesEntrante
	alto := l.altoBotones()
	for _, b := range []*ui.Boton{l.contestar, l.rechazar} {
		m := b.MinSize()
		b.Move(fyne.NewPos(x, y))
		b.Resize(fyne.NewSize(m.Width, alto))
		x += m.Width + huecoEntrante
	}
}

// Refresh repinta con los tokens del tema y el aviso de ahora.
func (r *llamadaEntranteRenderer) Refresh() {
	l := r.l
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l.caja.Pintar(tema.Color("menu-surface", v), tema.Color("border", v))
	l.nombre.Color = tema.Color("text", v)
	l.meta.Color = tema.Color("text-subtle", v)
	r.Layout(l.Size())
	l.nombre.Refresh()
	l.meta.Refresh()
	l.contestar.Refresh()
	l.rechazar.Refresh()
}

// esquinaEntrante coloca la tarjeta (y su sombra) arriba a la derecha de la ventana.
//
// Va de HERMANO del contenido, como la pila de avisos (ver ui/aviso.go): una capa de
// Fyne dejaría la app entera sin poder pulsar mientras suena.
type esquinaEntrante struct{ l *LlamadaEntrante }

func (e esquinaEntrante) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (e esquinaEntrante) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	l := e.l
	if !l.Visible() {
		l.sombra.Objeto().Hide()
		return
	}
	ancho := min(AnchoLlamadaEntrante, max(t.Width-2*MargenLlamadaEntrante, 0))
	tam := fyne.NewSize(ancho, l.MinSize().Height)
	pos := fyne.NewPos(t.Width-MargenLlamadaEntrante-ancho, MargenLlamadaEntrante)
	l.Move(pos)
	l.Resize(tam)
	l.sombra.Objeto().Show()
	l.sombra.Ajustar(pos, tam, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
}

// nuevaCapaEntrante es el hueco transparente del tamaño de la ventana con la tarjeta.
func nuevaCapaEntrante(l *LlamadaEntrante) *fyne.Container {
	return container.New(esquinaEntrante{l: l}, l.sombra.Objeto(), l)
}

// ---- En la Pantalla ----

// MostrarLlamadaEntrante enseña (o cambia) el aviso.
func (p *Pantalla) MostrarLlamadaEntrante(a AvisoEntrante) {
	l := p.Entrante
	l.Aviso = a
	l.Show()
	l.Refresh()
	p.capaEntrante.Refresh()
}

// QuitarLlamadaEntrante lo retira.
func (p *Pantalla) QuitarLlamadaEntrante() {
	p.Entrante.Aviso = AvisoEntrante{}
	p.Entrante.Hide()
	p.capaEntrante.Refresh()
}

// ---- En la Sala ----

// alLlamadaEntrante es la otra mitad de la trama `call` de la web: con `call_started`
// de otro canal saca el aviso si le toca (y, en un directo, el timbre y el aviso del
// sistema con la ventana sin foco); con `call_ended` del suyo lo retira.
func (sa *Sala) alLlamadaEntrante(canal string, empieza bool) {
	if !empieza {
		if sa.entrante.CanalID == canal {
			sa.retirarEntrante()
		}
		return
	}
	a, ok := AvisoDeLlamada(SituacionLlamada{
		CanalID: canal, Abierto: sa.abierto, EnLlamada: sa.ventanaLlamada != nil, Canales: sa.P.Lateral.Canales,
	})
	if !ok {
		return
	}
	sa.retirarEntrante() // una llamada nueva sustituye a la anterior, con su timbre
	sa.entrante = a
	sa.P.MostrarLlamadaEntrante(a)
	// Solo suena un DIRECTO: una llamada de canal se ve, pero no interrumpe a los
	// treinta que están en él. Por lo mismo, solo un directo sale al sistema.
	if !a.Directo {
		return
	}
	if sa.Timbre != nil {
		sa.pararTimbre = sa.Timbre()
	}
	// «Pausar avisos» calla el del sistema, como calla los del Hub (BarraSuite).
	if sa.enFondo && sa.AvisarSistema != nil && !sa.AvisosEnPausa() {
		sa.AvisarSistema(sa.P.Entrante.Rotulo(), sa.P.Entrante.Detalle())
	}
}

// retirarEntrante quita el aviso y para el timbre. El timbre va atado al aviso, no a
// cada sitio que lo apaga: contestar, rechazar, que cuelguen y salir de la sesión son
// cuatro caminos, y aquí solo hay uno que lo para.
func (sa *Sala) retirarEntrante() {
	if sa.pararTimbre != nil {
		sa.pararTimbre()
		sa.pararTimbre = nil
	}
	if sa.entrante.CanalID == "" {
		return
	}
	sa.entrante = AvisoEntrante{}
	sa.P.QuitarLlamadaEntrante()
}

// ContestarEntrante es «Contestar»: abre esa conversación y entra en la llamada por la
// misma puerta que «Llamar» de la cabecera.
func (sa *Sala) ContestarEntrante() {
	id := sa.entrante.CanalID
	sa.retirarEntrante()
	i := slices.IndexFunc(sa.P.Lateral.Canales, func(c Canal) bool { return c.ID == id })
	if i < 0 {
		return
	}
	sa.Elegir(sa.P.Lateral.Canales[i])
	sa.LlamarEnCanal()
}

// RechazarEntrante es «Rechazar»: el aviso se va y deja de sonar. La llamada sigue para
// los demás, y el punto del carril dice que está ahí por si se cambia de idea.
func (sa *Sala) RechazarEntrante() { sa.retirarEntrante() }
