// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Las acciones del día (`Launcher.tsx`, forma Marco)
//
// Bajo el saludo, los accesos rápidos de siempre en píldoras: «Escribir un correo» es la
// principal —el `Button` primario de talla `sm`, con su icono— y las demás son la
// píldora secundaria de abajo. La última es «Más», que despliega lo que no es de todos
// los días. «Buscar en toda la suite» no está: ya lo lleva la barra a cualquier ancho.

// AccionesDelDia son las píldoras, en su orden (`ACCIONES_DEL_DIA`): la primera es la
// principal. AccionesEnMas es lo que queda bajo «Más» (`ACCIONES_EN_MAS`).
var (
	AccionesDelDia = []string{"quick.compose", "quick.newDoc", "quick.upload", "quick.newEvent"}
	AccionesEnMas  = []string{"quick.download", "quick.devices", "quick.data"}
)

// Medidas de la píldora secundaria, de la constante `PILDORA` de la web:
// `rounded-control border-border bg-surface text-text hover:border-border-strong
// inline-flex h-8 items-center gap-1 border px-3.5 text-sm font-semibold shadow-xs`.
const (
	// AltoPildoraDia es el `h-8`, con el borde dentro.
	AltoPildoraDia float32 = 8 * 0.25 * tema.Rem
	// PildoraDiaPX es el `px-3.5` y HuecoGalonPildora el `gap-1` hasta el galón.
	PildoraDiaPX      float32 = 3.5 * 0.25 * tema.Rem
	HuecoGalonPildora float32 = 1 * 0.25 * tema.Rem
	// LadoGalonPildora es el `<Icon name="chevronDown" size={14} />` de «Más».
	LadoGalonPildora float32 = 14
)

// PildoraDia es la píldora secundaria: superficie blanca con borde, del radio de control.
type PildoraDia struct {
	ui.Pulsable

	Rotulo string
	// Galon le pone el `chevronDown` detrás: es «Más», que despliega un menú.
	Galon bool
}

var _ fyne.Widget = (*PildoraDia)(nil)

// NuevaPildoraDia crea la píldora.
func NuevaPildoraDia(rotulo string, alPulsar func()) *PildoraDia {
	p := &PildoraDia{Rotulo: rotulo}
	p.Nombre, p.AlPulsar = rotulo, alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// MinSize es el rótulo con su `px-3.5` y su borde, y el galón con su hueco si lo lleva.
func (p *PildoraDia) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	ancho := 2*tema.Borde + 2*PildoraDiaPX + ui.Medir(p.Rotulo, tema.TextoSM, ui.SemiNegrita).Width
	if p.Galon {
		ancho += HuecoGalonPildora + LadoGalonPildora
	}
	alto := AltoPildoraDia
	if ui.EsTactil() { // `pointer-coarse:min-h-[max(44px,2.75rem)]`
		alto = max(alto, ui.SueloTactilBarra)
	}
	return fyne.NewSize(ancho, alto)
}

// borde es `border-border` y, con el puntero encima, `hover:border-border-strong`.
func (p *PildoraDia) borde(v tema.Variante) color.NRGBA {
	nombre := "border"
	if p.Encima() {
		nombre = "border-strong"
	}
	return ui.BordeSobre(nombre, "surface", v)
}

// CreateRenderer compone la sombra, la caja, el rótulo y el galón.
func (p *PildoraDia) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	radio := tema.Forma("radius-control")
	r := &pildoraDiaRenderer{
		p:      p,
		sombra: ui.NuevaSombra(tema.SombraXS, radio).SinInterior(),
		aro:    ui.NuevoAro(radio, ui.SinHueco, v),
		caja:   ui.NuevaCaja(tema.Color("surface", v), p.borde(v), radio).ConTrazo(),
		rotulo: ui.Texto(p.Rotulo, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v)),
	}
	r.objetos = append([]fyne.CanvasObject{r.sombra.Objeto()}, r.aro.Objetos()...)
	r.objetos = append(r.objetos, r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.rotulo)
	if p.Galon {
		r.galon = iconos.Trazo(iconos.ChevronAbajo, LadoGalonPildora, "text", v)
		r.objetos = append(r.objetos, r.galon.Objeto())
	}
	r.Refresh() // regla 7 de `ui.go`
	return r
}

type pildoraDiaRenderer struct {
	p       *PildoraDia
	v       tema.Variante
	sombra  *ui.Sombra
	aro     *ui.Aro
	caja    *ui.Caja
	rotulo  *canvas.Text
	galon   *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *pildoraDiaRenderer) MinSize() fyne.Size           { return r.p.MinSize() }
func (r *pildoraDiaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pildoraDiaRenderer) Destroy()                     {}

func (r *pildoraDiaRenderer) Layout(t fyne.Size) {
	r.sombra.Ajustar(fyne.NewPos(0, 0), t, r.v)
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	x := tema.Borde + PildoraDiaPX
	ui.ColocarTexto(r.rotulo, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
	if r.galon != nil {
		x += ui.Medir(r.p.Rotulo, tema.TextoSM, ui.SemiNegrita).Width + HuecoGalonPildora
		r.galon.Colocar(x, (t.Height-LadoGalonPildora)/2, LadoGalonPildora)
	}
}

// Refresh repinta con el estado del puntero y del foco.
func (r *pildoraDiaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.v = v
	radio := tema.Forma("radius-control") // la forma se cambia en caliente
	r.sombra.CambiarRadio(radio)
	r.aro.CambiarRadio(radio)
	r.caja.CambiarRadio(radio)
	r.caja.Pintar(tema.Color("surface", v), r.p.borde(v))
	r.aro.Pintar(v)
	r.aro.Ver(r.p.Enfocado())
	r.rotulo.Color = tema.Color("text", v)
	r.rotulo.Refresh()
	if r.galon != nil {
		r.galon.Tenir("text", v)
	}
	r.Layout(r.p.Size())
}

// accion es un acceso rápido: su clave del catálogo, su dibujo y lo que hace. Es la
// misma lista para la tarjeta de Carril y para las píldoras de Marco.
type accion struct {
	clave  string
	glifo  GlifoAcceso
	pulsar func()
}

// accionDe busca una por su clave.
func accionDe(acciones []accion, clave string) (accion, bool) {
	for _, a := range acciones {
		if a.clave == clave {
			return a, true
		}
	}
	return accion{}, false
}

// pildorasDe son las acciones del día: la principal, las otras tres y «Más».
func (r *lanzadorRenderer) pildorasDe(v tema.Variante) []fyne.CanvasObject {
	t, acciones := r.l.T, r.acciones()
	var out []fyne.CanvasObject
	for i, clave := range AccionesDelDia {
		a, ok := accionDe(acciones, clave)
		if !ok {
			continue
		}
		if i > 0 {
			out = append(out, NuevaPildoraDia(t.T(clave, nil), a.pulsar))
			continue
		}
		// La principal: `<Button size="sm" leadingIcon={qa.icon()}>`.
		b := ui.NuevoBoton(t.T(clave, nil), a.pulsar).Con(ui.Primario, ui.TallaSM)
		b.Glifo, b.TamIcono = a.glifo.dibujo(tema.Color("on-primary", v)), LadoGlifo
		out = append(out, b)
	}
	mas := NuevaPildoraDia(t.T("launcher.more", nil), nil)
	mas.Galon = true
	mas.AlPulsar = func() { r.menu = r.menuMas(mas, acciones) }
	return append(out, mas)
}

// repartirPildoras las pone en líneas de `ancho` (`flex flex-wrap gap-2`) y devuelve el
// grupo y lo que ocupa.
func repartirPildoras(pildoras []fyne.CanvasObject, ancho float32) (fyne.CanvasObject, float32) {
	piezas := make([]ui.PiezaFlujo, 0, len(pildoras))
	var linea float32
	for _, p := range pildoras {
		m := p.MinSize()
		linea = max(linea, m.Height)
		piezas = append(piezas, ui.PiezaFlujo{O: p, Ancho: m.Width, Alto: m.Height, Sep: HuecoPildoras})
	}
	return ui.Fluir(piezas, ancho, linea, HuecoPildoras)
}

// menuMas despliega lo que queda bajo «Más», alineado con su borde izquierdo
// (`align="start"`). El menú lleva el foco a su primera opción y Escape lo devuelve a la
// píldora, como el `Dropdown` de la web.
func (r *lanzadorRenderer) menuMas(disparador fyne.CanvasObject, acciones []accion) *ui.Menu {
	t := r.l.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	var elementos []fyne.CanvasObject
	for _, clave := range AccionesEnMas {
		a, ok := accionDe(acciones, clave)
		if !ok {
			continue
		}
		e := ui.NuevoElementoMenu(t.T(clave, nil), iconos.Ninguno, a.pulsar)
		e.Glifo, e.TamIcono = a.glifo.dibujo(tema.Color("text-muted", v)), LadoGlifo // el de un `MenuItem`
		elementos = append(elementos, e)
	}
	m := ui.NuevoMenu(elementos...)
	m.Rotulo, m.AlInicio = t.T("launcher.more", nil), true
	m.Mostrar(disparador)
	return m
}
