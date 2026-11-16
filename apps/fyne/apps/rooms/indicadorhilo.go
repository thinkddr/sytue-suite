// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Contrapartida de IndicadorHilo.tsx: el hilo visto desde el canal (D-ROOM-05). El
// botón con avatares y «Última respuesta hace…» es EnlaceHilo (hilo.go); aquí van la
// hora de esa última respuesta y la línea «Respondió en un hilo · Ver el hilo» de una
// respuesta mandada también al canal.

// relojHilo es el «ahora» de «Última respuesta hace 5 min»; los tests lo congelan.
var relojHilo = time.Now

// rotuloUltimaRespuesta es «Última respuesta hace 5 min», o "" si el servicio no dio la
// hora (una raíz de antes del indicador, o un mensaje sin hilo).
func (f *FilaMensaje) rotuloUltimaRespuesta() string {
	if f.Mensaje.UltimaRespuesta.IsZero() {
		return ""
	}
	return f.T.T("thread.lastReply", i18n.Vars{
		"when": i18n.FormatTimeAgo(f.Locale, f.Mensaje.UltimaRespuesta, relojHilo()),
	})
}

// conLineaEnHilo dice si la fila lleva «Respondió en un hilo»: una respuesta mandada
// también al canal, vista en el canal y con quien abra el hilo.
func (f *FilaMensaje) conLineaEnHilo() bool {
	return !f.EnHilo && f.Mensaje.RaizHilo != "" && f.AlAbrirHilo != nil
}

// montarLineaEnHilo pone «Respondió en un hilo» en la fila si le toca.
func (f *FilaMensaje) montarLineaEnHilo() {
	if !f.conLineaEnHilo() {
		return
	}
	f.enHilo = NuevaLineaEnHilo(f.T, func() { f.AlAbrirHilo(f.Mensaje) })
	f.enHilo.alEncima = f.hijoEncima
	f.objetos = append(f.objetos, f.enHilo)
}

// altoEnHilo es lo que ocupa esa línea con su `mb-0.5`, o cero.
func (f *FilaMensaje) altoEnHilo() float32 {
	if !f.conLineaEnHilo() {
		return 0
	}
	return MedidaLineaEnHilo(f.T).Height + AireLineaEnHilo
}

// La línea de `RespondioEnHilo`: `sytue-meta mb-0.5 flex items-center gap-1`, con el
// glifo de hilos de 12, el texto, un «·» y el botón «Ver el hilo» en `text-primary-text
// font-medium`. HOJA.
var (
	AireLineaEnHilo  = tema.Espacio(0.5)
	HuecoLineaEnHilo = tema.Espacio(1)
)

// LineaEnHilo es «Respondió en un hilo · Ver el hilo». Solo «Ver el hilo» se pulsa,
// como en la web, donde el resto es texto.
type LineaEnHilo struct {
	widget.BaseWidget
	T *i18n.Translator
	// alEncima avisa a la fila: sobre el enlace el mensaje sigue «encima».
	alEncima func(bool)

	glifo        *iconos.Dibujo
	texto, punto *canvas.Text
	ver          *ui.Enlace
	objetos      []fyne.CanvasObject
}

var _ fyne.Widget = (*LineaEnHilo)(nil)

// NuevaLineaEnHilo crea la línea; alVer abre el hilo.
func NuevaLineaEnHilo(t *i18n.Translator, alVer func()) *LineaEnHilo {
	l := &LineaEnHilo{T: t}
	l.ver = ui.NuevoEnlace(t.T("thread.viewThread", nil), escalonMeta, ui.Medio, "primary-text", alVer)
	l.ver.Boton = true
	l.ver.AlCambiarEstado(func() {
		if l.alEncima != nil {
			l.alEncima(l.ver.Encima())
		}
	})
	l.ExtendBaseWidget(l)
	return l
}

// MedidaLineaEnHilo es lo que ocupa la línea. Es la única cuenta.
func MedidaLineaEnHilo(t *i18n.Translator) fyne.Size {
	ancho := LadoGlifoSiguiendo + HuecoLineaEnHilo +
		ui.Medir(t.T("thread.repliedInThread", nil), escalonMeta, ui.Normal).Width + HuecoLineaEnHilo +
		ui.Medir("·", escalonMeta, ui.Normal).Width + HuecoLineaEnHilo +
		ui.Medir(t.T("thread.viewThread", nil), escalonMeta, ui.Medio).Width
	return fyne.NewSize(ancho, escalonMeta.Linea)
}

// CreateRenderer compone la línea.
func (l *LineaEnHilo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l.glifo = dibujo(glifoHilos, LadoGlifoSiguiendo, "text-subtle", v)
	l.texto = ui.Texto(l.T.T("thread.repliedInThread", nil), escalonMeta, ui.Normal, tema.Color("text-subtle", v))
	l.punto = ui.Texto("·", escalonMeta, ui.Normal, tema.Color("text-subtle", v))
	l.objetos = []fyne.CanvasObject{l.glifo.Objeto(), l.texto, l.punto, l.ver}
	return &lineaEnHiloRenderer{l: l}
}

type lineaEnHiloRenderer struct{ l *LineaEnHilo }

func (r *lineaEnHiloRenderer) MinSize() fyne.Size           { return MedidaLineaEnHilo(r.l.T) }
func (r *lineaEnHiloRenderer) Objects() []fyne.CanvasObject { return r.l.objetos }
func (r *lineaEnHiloRenderer) Destroy()                     {}

func (r *lineaEnHiloRenderer) Layout(t fyne.Size) {
	l := r.l
	alto := escalonMeta.Linea
	l.glifo.Colocar(0, (alto-LadoGlifoSiguiendo)/2, LadoGlifoSiguiendo)
	x := LadoGlifoSiguiendo + HuecoLineaEnHilo
	ui.ColocarTexto(l.texto, escalonMeta, x, 0)
	x += ui.Medir(l.texto.Text, escalonMeta, ui.Normal).Width + HuecoLineaEnHilo
	ui.ColocarTexto(l.punto, escalonMeta, x, 0)
	x += ui.Medir("·", escalonMeta, ui.Normal).Width + HuecoLineaEnHilo
	m := ui.MedidaEnlace(l.ver)
	l.ver.Move(fyne.NewPos(x, 0))
	l.ver.Resize(fyne.NewSize(min(m.Width, max(t.Width-x, 0)), alto))
}

func (r *lineaEnHiloRenderer) Refresh() {
	l := r.l
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	l.glifo.Tenir("text-subtle", v)
	for _, c := range []*canvas.Text{l.texto, l.punto} {
		c.Color = tema.Color("text-subtle", v)
		c.Refresh()
	}
	l.ver.Refresh()
	r.Layout(l.Size())
}
