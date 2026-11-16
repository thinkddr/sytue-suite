// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// FranjaLlamada es el aviso de «llamada en curso» que la web pone bajo la cabecera del
// canal (`ChatApp.tsx`, el `role="status"` antes de `MessageView`): la cámara, «Llamada
// en curso · N participantes» y «Unirse». Sin ella, quien abre un canal donde ya están
// hablando no se entera hasta que alguien escribe; con el punto del carril sabe QUE hay
// llamada, y aquí, cuántos son y cómo entrar.
//
// `flex items-center gap-2 border-b bg-selected px-4 py-1.5 text-sm text-primary-text`,
// con el `VideoGlyph` de 16, el texto que se estira truncado y el botón `sm` secundario.
type FranjaLlamada struct {
	widget.BaseWidget

	T *i18n.Translator
	// Participantes son los presentes; con 0 la columna no la enseña.
	Participantes int
	// AlUnirse es «Unirse»: `startChannelCall`, la misma puerta que «Llamar».
	AlUnirse func()

	fondo, filete *canvas.Rectangle
	glifo         *iconos.Dibujo
	texto         *canvas.Text
	unirse        *ui.Boton
}

var (
	_ fyne.Widget     = (*FranjaLlamada)(nil)
	_ fyne.Accessible = (*FranjaLlamada)(nil)
)

var (
	// RellenoFranjaLlamadaY es su `py-1.5`; el `px-4` es el canalón del historial.
	RellenoFranjaLlamadaY = tema.Espacio(1.5)
	// HuecoFranjaLlamada es el `gap-2` entre la cámara, el texto y el botón.
	HuecoFranjaLlamada = tema.Espacio(2)
	// GlifoFranjaLlamada es el `<VideoGlyph />` de 16.
	GlifoFranjaLlamada float32 = 16
)

// NuevaFranjaLlamada crea la franja; «Unirse» llama a AlUnirse en el momento de pulsar,
// así que se puede poner después.
func NuevaFranjaLlamada(t *i18n.Translator) *FranjaLlamada {
	f := &FranjaLlamada{T: t}
	f.unirse = ui.NuevoBoton(t.T("action.join", nil), func() {
		if f.AlUnirse != nil {
			f.AlUnirse()
		}
	}).Con(ui.Secundario, ui.TallaSM)
	f.ExtendBaseWidget(f)
	return f
}

// Rotulo es «Llamada en curso · N participantes», con su plural.
func (f *FranjaLlamada) Rotulo() string {
	return f.T.T("call.inProgress", map[string]any{"count": f.Participantes})
}

// Unirse es el botón, para los guardianes y el recorrido con el teclado.
func (f *FranjaLlamada) Unirse() *ui.Boton { return f.unirse }

// AccessibilityRole la anuncia como texto (`role="status"`); lo que se pulsa es «Unirse».
func (f *FranjaLlamada) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// AccessibilityLabel es el propio aviso.
func (f *FranjaLlamada) AccessibilityLabel() string { return f.Rotulo() }

// MedidaFranjaLlamada es su alto: el `py-1.5` de los dos lados, el botón `sm` —más alto
// que la línea de `text-sm`, y con el dedo de 44 px por el `pointer-coarse` de
// Button.tsx— y el `border-b`.
func MedidaFranjaLlamada() fyne.Size {
	return fyne.NewSize(0, 2*RellenoFranjaLlamadaY+max(tema.TextoSM.Linea, ui.SueloTactil(tema.Espacio(8)))+tema.Borde)
}

// MinSize es la medida declarada.
func (f *FranjaLlamada) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return MedidaFranjaLlamada()
}

// CreateRenderer compone el fondo, su filete, la cámara, el texto y el botón.
func (f *FranjaLlamada) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = canvas.NewRectangle(fondoFranjaLlamada(v))
	f.filete = canvas.NewRectangle(tema.Color("border", v))
	f.glifo = dibujo(glifoVideo, GlifoFranjaLlamada, "primary-text", v)
	f.texto = ui.Texto(f.Rotulo(), tema.TextoSM, ui.Normal, tema.Color("primary-text", v))
	r := &franjaLlamadaRenderer{f: f}
	r.Refresh()
	return r
}

// fondoFranjaLlamada es `bg-selected`, que es translúcido, sobre la `surface` del canal.
func fondoFranjaLlamada(v tema.Variante) color.NRGBA {
	return tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
}

type franjaLlamadaRenderer struct{ f *FranjaLlamada }

func (r *franjaLlamadaRenderer) MinSize() fyne.Size { return MedidaFranjaLlamada() }
func (r *franjaLlamadaRenderer) Destroy()           {}

func (r *franjaLlamadaRenderer) Objects() []fyne.CanvasObject {
	f := r.f
	return []fyne.CanvasObject{f.fondo, f.filete, f.glifo.Objeto(), f.texto, f.unirse}
}

// Layout: la cámara y el texto a la izquierda, «Unirse» empujado a la derecha.
func (r *franjaLlamadaRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Move(fyne.NewPos(0, 0))
	f.fondo.Resize(t)
	f.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	f.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	linea := t.Height - tema.Borde

	m := f.unirse.MinSize()
	derecha := t.Width - CanalonHistorial - m.Width
	f.unirse.Move(fyne.NewPos(derecha, (linea-m.Height)/2))
	f.unirse.Resize(m)

	f.glifo.Colocar(CanalonHistorial, (linea-GlifoFranjaLlamada)/2, GlifoFranjaLlamada)
	x := CanalonHistorial + GlifoFranjaLlamada + HuecoFranjaLlamada
	ui.Truncar(f.texto, f.Rotulo(), tema.TextoSM, ui.Normal, max(derecha-HuecoFranjaLlamada-x, 0))
	ui.ColocarTexto(f.texto, tema.TextoSM, x, (linea-tema.TextoSM.Linea)/2)
}

// Refresh repinta con los tokens del tema y el número de ahora.
func (r *franjaLlamadaRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo.FillColor = fondoFranjaLlamada(v)
	f.filete.FillColor = tema.Color("border", v)
	f.glifo.Tenir("primary-text", v)
	f.texto.Color = tema.Color("primary-text", v)
	r.Layout(f.Size())
	for _, o := range []fyne.CanvasObject{f.fondo, f.filete, f.texto} {
		o.Refresh()
	}
	f.unirse.Refresh()
}

// ---- En la Sala ----

// CadaLlamadaActiva es el `useSondeo(refrescarLlamadaActiva, 60_000)` de la web. Que
// HAY llamada lo dicen al instante las tramas `call_started`/`call_ended`; el sondeo
// solo ajusta cuántos hay dentro, y para eso un minuto basta.
const CadaLlamadaActiva = time.Minute

// alEmpezarOAcabarLlamada es la trama `call` de la web: enciende o apaga el punto de la
// fila de ese canal y, si es el abierto, la franja —con 1 al empezar, que el sondeo
// corrige; a 0 en seco al acabar—. Sea del canal que sea: el agujero era justo el de
// otro canal, que no se veía hasta abrirlo.
func (sa *Sala) alEmpezarOAcabarLlamada(canal string, empieza bool) {
	cambia := sa.marcarLlamadaViva(canal, empieza)
	if canal == sa.abierto {
		n := 0
		if empieza {
			n = max(sa.P.Canal.Llamada.Participantes, 1)
		}
		sa.ponerLlamadaActiva(n)
	}
	if cambia {
		sa.P.Refrescar()
	}
}

// marcarLlamadaViva apunta (o borra) la llamada del canal y vuelve a pasar los canales
// al carril si cambia; dice si cambió, para que quien llama monte el carril de nuevo.
func (sa *Sala) marcarLlamadaViva(canal string, viva bool) bool {
	if sa.llamadasVivas[canal] == viva {
		return false
	}
	if viva {
		if sa.llamadasVivas == nil {
			sa.llamadasVivas = map[string]bool{}
		}
		sa.llamadasVivas[canal] = true
	} else {
		delete(sa.llamadasVivas, canal)
	}
	sa.pintarCanales()
	return true
}

// abrirLlamadaActiva es el efecto de `activeId` de la web: al cambiar de canal la franja
// parte de lo que se sabe —el punto de su fila— y se pregunta en el acto cuántos son.
func (sa *Sala) abrirLlamadaActiva() {
	n := 0
	if sa.llamadasVivas[sa.abierto] {
		n = 1
	}
	sa.ponerLlamadaActiva(n)
	sa.refrescarLlamadaActiva()
}

// ponerLlamadaActiva deja ese número de presentes en la franja del canal abierto. El
// sondeo casi siempre trae lo mismo, y entonces no se repinta nada.
func (sa *Sala) ponerLlamadaActiva(n int) {
	if f := sa.P.Canal.Llamada; f.Participantes != n {
		f.Participantes = n
		sa.pintarFranjaLlamada()
	}
}

// pintarFranjaLlamada la repinta: también cambia al entrar o salir de una llamada.
func (sa *Sala) pintarFranjaLlamada() {
	sa.P.Canal.DentroDeLlamada = sa.ventanaLlamada != nil
	sa.P.Canal.Refresh()
}

// pararSondeoLlamada quita el sondeo programado, si lo hay.
func (sa *Sala) pararSondeoLlamada() {
	if sa.pararLlamadaActiva != nil {
		sa.pararLlamadaActiva()
		sa.pararLlamadaActiva = nil
	}
}

// refrescarLlamadaActiva pregunta cuántos hay en la llamada del canal abierto y se
// vuelve a programar. Solo con un canal abierto y la app a la vista: con la ventana de
// fondo eran 60 consultas a la hora que no leía nadie. Un fallo deja la franja como
// estaba (la web degrada a «sin banner» al abrir, y aquí eso ya lo hizo
// abrirLlamadaActiva); una respuesta de un canal que ya no es el abierto se tira.
func (sa *Sala) refrescarLlamadaActiva() {
	sa.pararSondeoLlamada()
	vigia, ok := sa.S.(VigiaLlamada)
	canal := sa.abierto
	if !ok || canal == "" || sa.enFondo {
		return
	}
	pedir(sa, func(ctx context.Context) (int, error) { return vigia.LlamadaActiva(ctx, canal) },
		func(n int, err error) {
			if canal != sa.abierto {
				return
			}
			if err == nil {
				// El sondeo también es la verdad del punto del carril: si la trama de
				// fin se perdió en un corte, aquí se apaga.
				if sa.marcarLlamadaViva(canal, n > 0) {
					sa.P.Refrescar()
				}
				sa.ponerLlamadaActiva(n)
			}
			if sa.enFondo {
				return
			}
			sa.pararSondeoLlamada()
			sa.pararLlamadaActiva = sa.Esperar(CadaLlamadaActiva, sa.refrescarLlamadaActiva)
		})
}
