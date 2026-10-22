// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Tabs.tsx

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las pestañas de Tabs.tsx: versalitas con aire, subrayado de cobre en la activa y una
// línea de un píxel que recorre toda la tira.
//
//	pestaña   `px-3 py-2.5 text-sm font-semibold uppercase tracking-[0.06em] border-b-2`
//	          → 15,75 de caja de línea + 17,5 de relleno + 2 de subrayado = 35,25
//	tira      `flex gap-1 border-b` → 3,5 entre pestañas y 1 de línea: 36,25 en total
//	panel     `pt-4` → 14
//
// El subrayado de la pestaña y la línea de la tira SE APILAN, igual que en el
// navegador: el `border-b` es del contenedor y se pinta debajo de sus hijos, así que en
// la activa se ven los dos. Quitar uno de los dos deja la tira flotando.
var (
	pestanaPX   = tema.Espacio(3)   // px-3
	pestanaPY   = tema.Espacio(2.5) // py-2.5
	huecoTira   = tema.Espacio(1)   // gap-1
	rellenoPane = tema.Espacio(4)   // pt-4 del panel
)

// SubrayadoPestana es el `border-b-2` que lleva TODA pestaña: en la activa es cobre y
// en las demás es transparente, que es lo que hace que cambiar de pestaña no mueva ni
// un píxel del rótulo.
const SubrayadoPestana float32 = 2

// AltoPestana es lo que ocupa una pestaña: `py-2.5` sobre la caja de línea de
// `text-sm`, más su subrayado. Con el rem de 14 son 35,25.
var AltoPestana = tema.TextoSM.Linea + 2*pestanaPY + SubrayadoPestana

// AltoTiraPestanas es la tira entera, con la línea de un píxel de debajo: 36,25.
var AltoTiraPestanas = AltoPestana + tema.Borde

// altoPestana y altoTiraPestanas son los de arriba con el suelo del dedo: 44 con
// `pointer-coarse:min-h-11`, como la web (medían 40 en el teléfono).
func altoPestana() float32      { return sueloTactil(AltoPestana) }
func altoTiraPestanas() float32 { return altoPestana() + tema.Borde }

// Pestana es una pestaña. Se declara desde Pestanas y no suelta, pero es pública
// porque una pantalla puede querer su medida.
type Pestana struct {
	pulsable

	// Rotulo va en versalitas; el nombre accesible es el rótulo tal cual se escribió.
	Rotulo string
	// Icono es opcional, a la izquierda del rótulo con `gap-2`.
	Icono fyne.CanvasObject
	// Activa es `aria-selected`.
	Activa bool
	// AlMover recibe −1 o +1 con las flechas, y 0 o el último con Inicio y Fin. Lo
	// pone la tira: las flechas mueven el foco entre pestañas, que es lo que un
	// `tablist` tiene que hacer.
	AlMover func(delta int, extremo bool)
}

var _ fyne.Widget = (*Pestana)(nil)

// NuevaPestana crea una pestaña con su rótulo.
func NuevaPestana(rotulo string, alPulsar func()) *Pestana {
	p := &Pestana{Rotulo: rotulo}
	p.Nombre, p.AlPulsar = rotulo, alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityLabel es el rótulo sin pasar por las versalitas: lo que se lee en voz
// alta no tiene por qué ir en mayúsculas, que es solo la presentación.
func (p *Pestana) AccessibilityLabel() string { return p.Rotulo }

// TypedKey añade a Intro las flechas, Inicio y Fin del patrón `tablist` de la WAI.
func (p *Pestana) TypedKey(e *fyne.KeyEvent) {
	if p.AlMover != nil {
		switch e.Name {
		case fyne.KeyRight:
			p.AlMover(1, false)
			return
		case fyne.KeyLeft:
			p.AlMover(-1, false)
			return
		case fyne.KeyHome:
			p.AlMover(0, true)
			return
		case fyne.KeyEnd:
			p.AlMover(1, true)
			return
		default: // las demás teclas las resuelve la cáscara: Intro activa
		}
	}
	p.pulsable.TypedKey(e)
}

// MedidaPestana es lo que ocupa: el rótulo en versalitas con su tracking, el `px-3` y,
// si lo lleva, el icono con su hueco.
func MedidaPestana(p *Pestana) fyne.Size {
	ancho := MedirVersal(p.Rotulo, tema.TextoSM, SemiNegrita) + 2*pestanaPX
	if p.Icono != nil {
		ancho += CajaIcono + huecoFila
	}
	return fyne.NewSize(ancho, altoPestana())
}

// CreateRenderer construye rótulo y subrayado.
func (p *Pestana) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pestanaRenderer{
		p:         p,
		aro:       NuevoAro(0, SinHueco, v).Abierto(), // no tiene caja
		subrayado: canvas.NewRectangle(color.NRGBA{}),
	}
	r.rotulo = NuevoVersal(p.Rotulo, tema.TextoSM, SemiNegrita, color.NRGBA{})
	r.objetos = append(r.aro.Objetos(), r.subrayado)
	if p.Icono != nil {
		r.objetos = append(r.objetos, p.Icono)
	}
	r.objetos = append(r.objetos, r.rotulo.Objetos()...)
	// Se pinta YA, antes de devolverlo. El rótulo nace con el color a cero y lo pone
	// `Refresh`, y Fyne NO llama a `Refresh` antes del primer pintado: sin esta línea
	// la pestaña sale INVISIBLE hasta que algo la repinte. Medido: 0 píxeles de tinta
	// recién montada, 1 623 después de un `Refresh`. Es la regla 7 del paquete.
	r.Refresh()
	return r
}

type pestanaRenderer struct {
	p         *Pestana
	aro       *Aro
	subrayado *canvas.Rectangle
	rotulo    *Versal
	objetos   []fyne.CanvasObject
}

func (r *pestanaRenderer) MinSize() fyne.Size           { return MedidaPestana(r.p) }
func (r *pestanaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pestanaRenderer) Destroy()                     {}

// Layout coloca icono y rótulo centrados en el hueco que deja el subrayado, que es lo
// que hace `items-center` con un `border-b-2`: el contenido se centra en la caja SIN
// contar el borde.
func (r *pestanaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.subrayado.Move(fyne.NewPos(0, t.Height-SubrayadoPestana))
	r.subrayado.Resize(fyne.NewSize(t.Width, SubrayadoPestana))

	interior := t.Height - SubrayadoPestana
	x := pestanaPX
	if r.p.Icono != nil {
		r.p.Icono.Move(fyne.NewPos(x, (interior-CajaIcono)/2))
		r.p.Icono.Resize(fyne.NewSize(CajaIcono, CajaIcono))
		x += CajaIcono + huecoFila
	}
	r.rotulo.Colocar(x, (interior-tema.TextoSM.Linea)/2)
}

// Refresh pinta los tres estados de Tabs.tsx: activa, al pasar por encima y en reposo.
func (r *pestanaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	texto, linea := tema.Color("text-muted", v), color.NRGBA{}
	switch {
	case r.p.Activa:
		// El color lo lleva el SUBRAYADO; si también lo llevara el texto saldrían dos
		// colores en una misma palabra.
		texto, linea = tema.Color("text", v), tema.Color("indicator", v)
	case r.p.encima:
		texto, linea = tema.Color("text", v), tema.Color("border-strong", v)
	}
	r.aro.Ver(r.p.enfocado)
	r.aro.Pintar(v)
	r.subrayado.FillColor = linea
	r.subrayado.Refresh()
	r.rotulo.Pintar(texto)
}

// Pestanas es Tabs.tsx entero: la tira con su línea y, debajo, el contenido de la
// pestaña elegida con el `pt-4`.
type Pestanas struct {
	pulsable

	// Rotulos son las pestañas, en orden.
	Rotulos []string
	// Paneles es el contenido de cada una, en el mismo orden. Puede tener menos
	// entradas que rótulos —o ninguna— si la pantalla pinta el contenido por su
	// cuenta.
	Paneles []fyne.CanvasObject
	// Activa es el índice de la elegida.
	Activa int
	// AlCambiar avisa del cambio. La pestaña se activa igual sin él.
	AlCambiar func(int)

	pestanas []*Pestana
}

var _ fyne.Widget = (*Pestanas)(nil)

// NuevasPestanas crea la tira con sus rótulos.
func NuevasPestanas(rotulos []string, paneles []fyne.CanvasObject) *Pestanas {
	p := &Pestanas{Rotulos: rotulos, Paneles: paneles}
	p.Nombre = "pestañas"
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityRole: la tira agrupa; quien se pulsa es cada pestaña.
func (p *Pestanas) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// Elegir cambia de pestaña y avisa.
func (p *Pestanas) Elegir(i int) {
	if i < 0 || i >= len(p.Rotulos) || i == p.Activa {
		return
	}
	p.Activa = i
	for j, t := range p.pestanas {
		t.Activa = j == i
		t.Refresh()
	}
	p.Refresh()
	if p.AlCambiar != nil {
		p.AlCambiar(i)
	}
}

// MedidaPestanas es la tira más, si hay panel, su `pt-4` y el alto del contenido.
func MedidaPestanas(p *Pestanas) fyne.Size {
	ancho := float32(0)
	for i, r := range p.Rotulos {
		if i > 0 {
			ancho += huecoTira
		}
		t := &Pestana{Rotulo: r}
		ancho += MedidaPestana(t).Width
	}
	alto := altoTiraPestanas()
	if panel := p.panel(); panel != nil {
		alto += rellenoPane + panel.MinSize().Height
		ancho = max(ancho, panel.MinSize().Width)
	}
	return fyne.NewSize(ancho, alto)
}

func (p *Pestanas) panel() fyne.CanvasObject {
	if p.Activa < 0 || p.Activa >= len(p.Paneles) {
		return nil
	}
	return p.Paneles[p.Activa]
}

// CreateRenderer construye las pestañas y la línea de la tira.
func (p *Pestanas) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pestanasRenderer{p: p, linea: canvas.NewRectangle(tema.Color("border", v))}
	p.pestanas = nil
	for i, rotulo := range p.Rotulos {
		t := NuevaPestana(rotulo, nil)
		t.Activa = i == p.Activa
		t.AlPulsar = func() { p.Elegir(i) }
		t.AlMover = func(delta int, extremo bool) {
			destino := i + delta
			if extremo {
				destino = 0
				if delta > 0 {
					destino = len(p.Rotulos) - 1
				}
			}
			// Circular, como las flechas de un `tablist`.
			destino = (destino + len(p.Rotulos)) % len(p.Rotulos)
			p.Elegir(destino)
			if c := fyne.CurrentApp().Driver().CanvasForObject(p); c != nil {
				enfocar(c, p.pestanas[destino])
			}
		}
		p.pestanas = append(p.pestanas, t)
	}
	r.rehacer()
	return r
}

type pestanasRenderer struct {
	p       *Pestanas
	linea   *canvas.Rectangle
	panel   fyne.CanvasObject
	objetos []fyne.CanvasObject
}

func (r *pestanasRenderer) rehacer() {
	r.panel = r.p.panel()
	r.objetos = []fyne.CanvasObject{r.linea}
	for _, t := range r.p.pestanas {
		r.objetos = append(r.objetos, t)
	}
	if r.panel != nil {
		r.objetos = append(r.objetos, r.panel)
	}
}

func (r *pestanasRenderer) MinSize() fyne.Size           { return MedidaPestanas(r.p) }
func (r *pestanasRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pestanasRenderer) Destroy()                     {}

// Layout pone las pestañas en fila con su `gap-1`, la línea debajo de todas y el panel
// a partir del `pt-4`.
func (r *pestanasRenderer) Layout(t fyne.Size) {
	x := float32(0)
	for _, p := range r.p.pestanas {
		m := MedidaPestana(p)
		p.Move(fyne.NewPos(x, 0))
		p.Resize(m)
		x += m.Width + huecoTira
	}
	r.linea.Move(fyne.NewPos(0, altoPestana()))
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	if r.panel != nil {
		y := altoTiraPestanas() + rellenoPane
		r.panel.Move(fyne.NewPos(0, y))
		r.panel.Resize(fyne.NewSize(t.Width, max(t.Height-y, 0)))
	}
}

// Refresh repinta la línea y cambia el panel visible.
func (r *pestanasRenderer) Refresh() {
	r.linea.FillColor = tema.Color("border", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.linea.Refresh()
	if r.panel != r.p.panel() {
		r.rehacer()
	}
	r.Layout(r.p.Size()) // cambiar de pestaña no cambia el tamaño de la tira
}
