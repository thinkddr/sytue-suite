// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/MessageView.tsx (`priorityBadge`
// y el bloque del acuse de recibo, Ola 12).
//
// La prioridad va ENCIMA del texto, como una píldora con PALABRAS («Urgente»): el color
// solo la refuerza (WCAG 1.4.1). El acuse va DEBAJO de las reacciones: «Confirmar» si
// quien mira aún no lo ha dado, «Confirmado» si ya, y «N acuses» si alguien lo ha dado.

// Pildora es una etiqueta en píldora con relleno, borde opcional y texto: la de
// `priorityBadge` (`rounded-pill sytue-overline border px-2 py-0.5`) y la de
// «Confirmado» (`rounded-pill px-2 py-0.5 text-xs font-medium`).
type Pildora struct {
	widget.BaseWidget

	Texto string
	// Fondo, Borde y Tinta son tokens; Borde vacío, sin filete.
	Fondo, Borde, Tinta string
	// Versal es `sytue-overline`: versalitas y semi-negrita. Sin ella, `font-medium`.
	Versal bool

	caja  *ui.Caja
	texto *canvas.Text
}

// NuevaPildora crea la píldora.
func NuevaPildora(texto, fondo, borde, tinta string, versal bool) *Pildora {
	p := &Pildora{Texto: texto, Fondo: fondo, Borde: borde, Tinta: tinta, Versal: versal}
	p.ExtendBaseWidget(p)
	return p
}

func (p *Pildora) rotulo() (string, ui.Peso) {
	if p.Versal {
		return ui.Versalitas(p.Texto), ui.SemiNegrita
	}
	return p.Texto, ui.Medio
}

// MedidaPildora es `px-2 py-0.5` alrededor de la caja de línea de `text-xs`, con su
// filete si lo lleva (el de la prioridad: 19,5 de alto).
func MedidaPildora(p *Pildora) fyne.Size {
	s, peso := p.rotulo()
	borde := float32(0)
	if p.Borde != "" {
		borde = tema.Borde
	}
	return fyne.NewSize(ui.Medir(s, tema.TextoXS, peso).Width+2*tema.Espacio(2)+2*borde,
		tema.TextoXS.Linea+2*tema.Espacio(0.5)+2*borde)
}

// AccessibilityLabel es su texto.
func (p *Pildora) AccessibilityLabel() string { return p.Texto }

// AccessibilityRole: es texto.
func (p *Pildora) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

func (p *Pildora) colores(v tema.Variante) (fondo, borde, tinta color.NRGBA) {
	if p.Borde != "" {
		borde = tema.Color(p.Borde, v)
	}
	return tema.Color(p.Fondo, v), borde, tema.Color(p.Tinta, v)
}

func (p *Pildora) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := p.colores(v)
	s, peso := p.rotulo()
	p.caja = ui.NuevaCaja(fondo, borde, tema.RadioPill)
	p.texto = ui.Texto(s, tema.TextoXS, peso, tinta)
	return &pildoraRenderer{p: p, objs: append(p.caja.Objetos(), p.texto)}
}

type pildoraRenderer struct {
	p    *Pildora
	objs []fyne.CanvasObject
}

func (r *pildoraRenderer) MinSize() fyne.Size           { return MedidaPildora(r.p) }
func (r *pildoraRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *pildoraRenderer) Destroy()                     {}

func (r *pildoraRenderer) Layout(t fyne.Size) {
	ui.MoverCaja(r.p.caja, fyne.Position{}, t)
	ui.ColocarTexto(r.p.texto, tema.TextoXS, (t.Width-ui.Medir(r.p.texto.Text, tema.TextoXS, r.pesoDe()).Width)/2,
		(t.Height-tema.TextoXS.Linea)/2)
}

func (r *pildoraRenderer) pesoDe() ui.Peso {
	_, peso := r.p.rotulo()
	return peso
}

func (r *pildoraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := r.p.colores(v)
	r.p.caja.Pintar(fondo, borde)
	r.p.texto.Color = tinta
	r.p.texto.Refresh()
	r.Layout(r.p.Size())
}

// NuevaPildoraPrioridad es `priorityBadge`: urgente en peligro con su filete de
// peligro, importante en aviso con el filete normal.
func NuevaPildoraPrioridad(t *i18n.Translator, p Prioridad) *Pildora {
	if p == Urgente {
		return NuevaPildora(t.T("priority.urgent", nil), "danger-bg", "danger", "danger-text", true)
	}
	return NuevaPildora(t.T("priority.important", nil), "warning-bg", "border", "warning-text", true)
}

// AireAcuse es el `mt-1.5` de la fila del acuse y HuecoAcuse su `gap-2`; AirePrioridad
// es el `mb-0.5` bajo la píldora de la prioridad.
var (
	AireAcuse     = tema.Espacio(1.5)
	HuecoAcuse    = tema.Espacio(2)
	AirePrioridad = tema.Espacio(0.5)
)

// montarAcuse pone las piezas de la fila del acuse de un mensaje que lo pide.
func (f *FilaMensaje) montarAcuse() {
	m := f.Mensaje
	if !m.PideAcuse || f.EnHilo {
		return
	}
	t := f.T
	switch {
	case m.Acusado:
		f.acuse = append(f.acuse, NuevaPildora(t.T("ack.done", nil), "success-bg", "", "success-text", false))
	case f.AlAcusar != nil:
		f.acuse = append(f.acuse, ui.NuevoBoton(t.T("ack.confirm", nil), func() { f.AlAcusar(f.Mensaje) }).
			Con(ui.Secundario, ui.TallaSM))
	}
	if m.Acuses > 0 {
		f.acuse = append(f.acuse, ui.NuevoRotulo(t.T("ack.byCount", i18n.Vars{"count": m.Acuses}), escalonMeta, ui.Normal, "text-subtle"))
	}
	f.objetos = append(f.objetos, f.acuse...)
}

// altoAcuse es lo que suma la fila del acuse, con su `mt-1.5`, o cero.
func (f *FilaMensaje) altoAcuse() float32 {
	if len(f.acuse) == 0 {
		return 0
	}
	alto := float32(0)
	for _, o := range f.acuse {
		alto = max(alto, o.MinSize().Height)
	}
	return AireAcuse + alto
}

// altoPrioridad es lo que suma la píldora de la prioridad sobre el texto, o cero.
func (f *FilaMensaje) altoPrioridad() float32 {
	if f.prioridad == nil {
		return 0
	}
	return MedidaPildora(f.prioridad).Height + AirePrioridad
}
