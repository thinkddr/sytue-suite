// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Las dos filas de la tarjeta «Hoy» y el enlace de acción de una cabecera de sección.

// ---- Fila de evento ----

// Medidas de la fila de evento (`flex items-baseline gap-3 px-4 py-2`):
//
//	hora     42 de ancho (`w-12 shrink-0`), 10,5/14 `text-xs tabular-nums`
//	resumen  11,375/15,75 `text-sm truncate`
//	alto     29,75 = 14 de `py-2` + 15,75
//
// La hora NO lleva `whitespace-nowrap`: lo que no cabe en su columna baja de línea, y
// «Todo el día» son dos («Todo el» y «día»). El resumen se queda arriba, con la primera.
//
// Un evento ya terminado va al 60 % (`opacity-60`), aplanado contra la superficie por
// lo mismo que el `disabled:opacity-50` del botón.
const (
	// AnchoHora es el `w-12` de la columna de la hora.
	AnchoHora float32 = 12 * 0.25 * tema.Rem
	// MargenEvento es el `px-4` y AltoEvento el `py-2`.
	MargenEvento float32 = 4 * 0.25 * tema.Rem
	AltoEvento   float32 = 2 * 0.25 * tema.Rem
	// HuecoEvento es el `gap-3`.
	HuecoEvento float32 = 3 * 0.25 * tema.Rem
)

// altoEvento es el `py-2` de la fila y, en la forma Marco, su `marco:py-1`: «Hoy» y
// «Reciente» tienen que caber enteras a 1280×800.
func altoEvento() float32 {
	if tema.EsMarco() {
		return 1 * 0.25 * tema.Rem
	}
	return AltoEvento
}

// sangriaHora es lo que baja la hora respecto al resumen para compartir línea de base
// (ver `Layout`).
func sangriaHora() float32 { return (tema.TextoSM.Linea - tema.TextoXS.Linea) / 2 }

// MedidaEvento es el alto de una fila de evento cuya hora ocupa esas líneas: con una
// manda el resumen; con dos, la hora (41 en Marco, medido en la web).
func MedidaEvento(lineasHora int) float32 {
	return 2*altoEvento() + max(tema.TextoSM.Linea, sangriaHora()+float32(lineasHora)*tema.TextoXS.Linea)
}

// FilaEvento es un evento del día. No es pulsable: en la web es un `<li>` sin enlace.
type FilaEvento struct {
	ui.Pulsable

	Hora    string
	Resumen string
	// Pasado es `opacity-60`.
	Pasado bool
}

var _ fyne.Widget = (*FilaEvento)(nil)

// lineasHora parte la hora en lo que cabe en su `w-12`.
func (f *FilaEvento) lineasHora() []string {
	return ui.Repartir(f.Hora, tema.TextoXS, ui.Normal, AnchoHora)
}

// nuevaFilaEvento construye la fila a partir del evento y de la hora actual.
func nuevaFilaEvento(v tema.Variante, t *i18n.Translator, e EventoDeHoy, ahora time.Time) *FilaEvento {
	hora := i18n.FormatTime(t.Locale(), e.Inicio)
	if e.TodoElDia {
		hora = t.T("today.allDay", nil)
	}
	resumen := e.Resumen
	if resumen == "" {
		resumen = t.T("today.untitled", nil)
	}
	f := &FilaEvento{Hora: hora, Resumen: resumen, Pasado: HaTerminado(e, ahora)}
	f.Nombre = hora + " " + resumen
	f.Inerte = true
	f.ExtendBaseWidget(f)
	_ = v
	return f
}

// AccessibilityRole: es texto, no un control.
func (f *FilaEvento) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// CreateRenderer compone hora y resumen.
func (f *FilaEvento) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &eventoRenderer{f: f, resumen: ui.Texto(f.Resumen, tema.TextoSM, ui.Normal, f.tinta(v, "text"))}
	r.objetos = []fyne.CanvasObject{r.resumen}
	for _, l := range f.lineasHora() {
		h := ui.Texto(l, tema.TextoXS, ui.Normal, f.tinta(v, "text-muted"))
		r.hora = append(r.hora, h)
		r.objetos = append(r.objetos, h)
	}
	return r
}

func (f *FilaEvento) tinta(v tema.Variante, token string) color.NRGBA {
	c := tema.Color(token, v)
	if !f.Pasado {
		return c
	}
	return tokens.Aplanar(c, 0.6, tema.Color("surface", v))
}

type eventoRenderer struct {
	f       *FilaEvento
	hora    []*canvas.Text // una por línea
	resumen *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *eventoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, MedidaEvento(len(r.hora)))
}
func (r *eventoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *eventoRenderer) Destroy()                     {}

// Layout alinea las dos cajas por su LÍNEA DE BASE (`items-baseline`), que es lo que
// hace que la hora y el resumen se lean en la misma línea aunque midan distinto.
//
// ponytail: aquí se aproxima alineando los centros de las dos cajas de línea. La línea
// de base de verdad pide la métrica de la fuente, que Fyne no expone por fuera de
// `canvas.Text`; con 10,5 y 11,375 px de tamaño la diferencia es de medio píxel.
func (r *eventoRenderer) Layout(t fyne.Size) {
	y := altoEvento()
	for i, h := range r.hora {
		ui.ColocarTexto(h, tema.TextoXS, MargenEvento, y+sangriaHora()+float32(i)*tema.TextoXS.Linea)
	}
	x := MargenEvento + AnchoHora + HuecoEvento
	ui.Truncar(r.resumen, r.f.Resumen, tema.TextoSM, ui.Normal, t.Width-x-MargenEvento)
	ui.ColocarTexto(r.resumen, tema.TextoSM, x, y)
}

// Refresh repinta con los tokens del tema actual.
func (r *eventoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for _, h := range r.hora {
		h.Color = r.f.tinta(v, "text-muted")
		h.Refresh()
	}
	r.resumen.Color = r.f.tinta(v, "text")
	r.resumen.Refresh()
}

// ---- Pie de correo ----

// El pie de la tarjeta «Hoy»: `flex items-center gap-3 border-t px-4 py-2.5 text-sm`
// con la marca de Mail de 20. Medido: 38,5 = 1 de filete + 17,5 de `py-2.5` + 20.
const (
	// AltoPieCorreo es el `py-2.5`.
	AltoPieCorreo float32 = 2.5 * 0.25 * tema.Rem
)

// altoPieCorreo es el `py-2.5` del pie y, en la forma Marco, su `marco:py-2`.
func altoPieCorreo() float32 {
	if tema.EsMarco() {
		return 2 * 0.25 * tema.Rem
	}
	return AltoPieCorreo
}

// MedidaCorreo es el alto del pie, su filete incluido.
func MedidaCorreo() float32 {
	return tema.Borde + 2*altoPieCorreo() + max(iconos.MarcaXS, tema.TextoSM.Linea)
}

// FilaCorreo es el pie de «Hoy»: la marca de Mail y los no leídos.
type FilaCorreo struct {
	ui.Pulsable

	Texto string
}

var _ fyne.Widget = (*FilaCorreo)(nil)

// NuevaFilaCorreo crea el pie.
func NuevaFilaCorreo(texto string, alPulsar func()) *FilaCorreo {
	f := &FilaCorreo{Texto: texto}
	f.Nombre = texto
	f.AlPulsar = alPulsar
	f.ExtendBaseWidget(f)
	return f
}

// CreateRenderer compone filete, marca y texto.
func (f *FilaCorreo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &correoRenderer{
		f:      f,
		fondo:  canvas.NewRectangle(f.fondo(v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		aro:    ui.NuevoAro(0, ui.SinHueco, v).Abierto(),
		marca:  iconos.Marca(iconos.Mail, iconos.MarcaXS, v),
		texto:  ui.Texto(f.Texto, tema.TextoSM, ui.Normal, tema.Color("text", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.fondo, r.filete, r.marca.Objeto(), r.texto)
	return r
}

func (f *FilaCorreo) fondo(v tema.Variante) color.NRGBA {
	if f.Encima() || f.Enfocado() {
		return tema.Color("bg-subtle", v)
	}
	return color.NRGBA{}
}

type correoRenderer struct {
	f             *FilaCorreo
	fondo, filete *canvas.Rectangle
	aro           *ui.Aro
	marca         *iconos.Dibujo
	texto         *canvas.Text
	objetos       []fyne.CanvasObject
}

func (r *correoRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, MedidaCorreo()) }
func (r *correoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *correoRenderer) Destroy()                     {}

// Layout pone el filete arriba —`border-t` va DENTRO del alto, como en `border-box`—
// y la marca y el texto centrados en lo que queda.
func (r *correoRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.fondo.Resize(t)
	r.filete.Move(fyne.NewPos(0, 0))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	medio := tema.Borde + (t.Height-tema.Borde)/2
	r.marca.Colocar(MargenEvento, medio-iconos.MarcaXS/2, iconos.MarcaXS)
	x := MargenEvento + iconos.MarcaXS + HuecoEvento
	ui.Truncar(r.texto, r.f.Texto, tema.TextoSM, ui.Normal, t.Width-x-MargenEvento)
	ui.ColocarTexto(r.texto, tema.TextoSM, x, medio-tema.TextoSM.Linea/2)
}

// Refresh repinta con el estado del puntero.
func (r *correoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = r.f.fondo(v)
	r.fondo.Refresh()
	r.filete.FillColor = tema.Color("border", v)
	r.filete.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.f.Enfocado())
	r.texto.Color = tema.Color("text", v)
	r.texto.Refresh()
}

// ---- El enlace de acción de una cabecera ----

// SangriaEnlace es el `py-1` que el enlace de acción añade a cada lado para llegar a
// los 26 px de diana de WCAG 2.5.8 sin mover la cabecera (`-my-1`).
const SangriaEnlace float32 = 1 * 0.25 * tema.Rem

// enlace es el `<a class="text-primary-text text-sm hover:underline">` que va a la
// derecha de una cabecera de sección. El peso NO es el mismo en los dos de
// Launcher.tsx: «Ver el calendario» va en `font-medium` y «Ver todo →» en
// `font-semibold`. Es `ui.Enlace` con esas clases.
func enlace(texto string, p ui.Peso, alPulsar func()) *ui.Enlace {
	e := ui.NuevoEnlace(texto, tema.TextoSM, p, "primary-text", alPulsar)
	e.Sangria = SangriaEnlace
	return e
}
