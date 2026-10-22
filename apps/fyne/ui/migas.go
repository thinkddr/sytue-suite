// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las migas de pan (`nav[aria-label="Ruta"]` del explorador de Sytue Drive), medidas en
// producción:
//
//	nav       `flex items-center gap-0.5 text-base` → 541,75 × 26,25 en la captura
//	segmento  26,25 de alto, `px-2 py-1`, radio 6, texto 12,25/19,25
//	           peso 600 el actual (`aria-current`), 500 los demás
//	galón     `chevronRight` de 14, en el color del texto al 50 %
//
// Los 26,25 son la caja de línea de `text-base` (19,25) más `py-1` por los dos lados.
var (
	migaPX   = tema.Espacio(2)   // px-2
	migaPY   = tema.Espacio(1)   // py-1
	huecoMig = tema.Espacio(0.5) // gap-0.5, entre el galón y lo que tiene a los lados
)

// AltoMiga es lo que ocupa un segmento de la ruta: 26,25.
var AltoMiga = tema.TextoBase.Linea + 2*migaPY

// LadoGalonMiga es el `size={14}` del separador.
const LadoGalonMiga float32 = 14

// EstiloMigas son las tres cosas que cambian entre las rutas de la suite, y nada más:
// el cuerpo de letra, el fondo al pasar por encima y la tinta del galón. El cero es la
// ruta del cromo del explorador (`text-base`, `surface-sunken`, `text/50`).
type EstiloMigas struct {
	// Escalon cero es `text-base`.
	Escalon tema.Escalon
	// FondoEncima es el token del hover; vacío, `surface-sunken`.
	FondoEncima string
	// Galon es el token del separador; vacío, el texto al 50 %.
	Galon string
	// Iguales pinta TODOS los segmentos igual, también el actual: `font-medium` y la
	// tinta del texto. Es la ruta del selector de la suite (SuitePicker.tsx), cuyo
	// `aria-current` no cambia ni el peso ni el color.
	Iguales bool
}

// Las dos variantes de Drive además de la del cromo.
var (
	// MigasEnDialogo es la ruta sobre la superficie clara de un diálogo («Mover a…»):
	// `hover:bg-bg-subtle` y el galón en `text-subtle`.
	MigasEnDialogo = EstiloMigas{FondoEncima: "bg-subtle", Galon: "text-subtle"}
	// MigasEnBarra es la de la barra de «Compartido conmigo», sobre `bg-bg-subtle`:
	// `text-sm`, `hover:bg-bg` y el galón en `text-subtle`.
	MigasEnBarra = EstiloMigas{Escalon: tema.TextoSM, FondoEncima: "bg", Galon: "text-subtle"}
	// MigasDelSelector es la del selector de la suite: la del diálogo, con todos los
	// segmentos iguales (medido en el arnés: «Mi unidad» actual en 500).
	MigasDelSelector = EstiloMigas{FondoEncima: "bg-subtle", Galon: "text-subtle", Iguales: true}
)

func (e EstiloMigas) escalon() tema.Escalon {
	if e.Escalon.Tam == 0 {
		return tema.TextoBase
	}
	return e.Escalon
}

// Alto es lo que mide un segmento con este estilo: su caja de línea y el `py-1`.
func (e EstiloMigas) Alto() float32 {
	alto := e.escalon().Linea + 2*migaPY
	if e == MigasEnDialogo {
		// Las del diálogo de «Mover» miden 44 con el dedo (`pointer-coarse:min-h-11`):
		// eran el blanco más pequeño de la hoja (30). Las del cromo no cambian.
		return sueloTactil(alto)
	}
	return alto
}

// Miga es un segmento pulsable de la ruta.
type Miga struct {
	pulsable

	// Rotulo es el nombre de la carpeta.
	Rotulo string
	// Actual es `aria-current="page"`: el último, que va en semi-negrita y con la
	// tinta normal en vez de la apagada.
	Actual bool
	// Estilo es el de la ruta a la que pertenece.
	Estilo EstiloMigas
}

var _ fyne.Widget = (*Miga)(nil)

// NuevaMiga crea un segmento.
func NuevaMiga(rotulo string, alPulsar func()) *Miga {
	m := &Miga{Rotulo: rotulo}
	m.Nombre, m.AlPulsar = rotulo, alPulsar
	m.ExtendBaseWidget(m)
	return m
}

// AccessibilityLabel es el nombre de la carpeta.
func (m *Miga) AccessibilityLabel() string { return m.Rotulo }

func (m *Miga) peso() Peso {
	if m.Actual && !m.Estilo.Iguales {
		return SemiNegrita // font-semibold
	}
	return Medio // font-medium
}

// MedidaMiga es el rótulo más su `px-2`, por los 26,25 de alto.
func MedidaMiga(m *Miga) fyne.Size {
	return fyne.NewSize(Medir(m.Rotulo, m.Estilo.escalon(), m.peso()).Width+2*migaPX, m.Estilo.Alto())
}

// CreateRenderer construye fondo y rótulo.
func (m *Miga) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &migaRenderer{
		m:      m,
		aro:    NuevoAro(tema.Forma("radius-control"), SinHueco, v).Abierto(), // transparente en reposo
		caja:   NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control")),
		rotulo: Texto(m.Rotulo, m.Estilo.escalon(), m.peso(), tema.Color("text-muted", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.rotulo)
	return r
}

type migaRenderer struct {
	m       *Miga
	aro     *Aro
	caja    *Caja
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *migaRenderer) MinSize() fyne.Size           { return MedidaMiga(r.m) }
func (r *migaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *migaRenderer) Destroy()                     {}

// Layout centra el rótulo en la caja, recortándolo si no cabe.
func (r *migaRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	e := r.m.Estilo.escalon()
	r.rotulo.Text = recorta(r.m.Rotulo, e, r.m.peso(), t.Width-2*migaPX)
	ColocarTexto(r.rotulo, e, migaPX, (t.Height-e.Linea)/2)
}

// Refresh pinta los estados: el actual con la tinta normal, el resto apagados, y
// `hover:bg-surface-sunken` por debajo.
func (r *migaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	texto, fondo := tema.Color("text-muted", v), color.NRGBA{}
	if r.m.Actual || r.m.Estilo.Iguales {
		texto = tema.Color("text", v)
	}
	switch {
	case r.m.pulsado: // active:bg-surface-sunken
		fondo = tema.Color("surface-sunken", v)
	case r.m.encima && r.m.Estilo.FondoEncima != "":
		fondo, texto = tema.Color(r.m.Estilo.FondoEncima, v), tema.Color("text", v)
	case r.m.encima:
		fondo, texto = tema.Color("surface-sunken", v), tema.Color("text", v)
	}
	r.aro.Ver(r.m.enfocado)
	r.aro.Pintar(v)
	r.caja.Pintar(fondo, color.NRGBA{})
	r.rotulo.Color = texto
	r.rotulo.Refresh()
}

// Migas es la ruta entera: los segmentos con sus galones.
//
// El último es el actual y los demás llevan a su carpeta. Cuando no caben, los
// segmentos se estrechan TODOS en la misma proporción y se recortan con puntos
// suspensivos, que es lo que hace el `flex` con `min-w-0` y `truncate` de la web.
type Migas struct {
	pulsable

	// Ruta son los nombres, de la raíz al actual.
	Ruta []string
	// AlIr recibe el índice del segmento pulsado.
	AlIr func(int)
	// Estilo cero es la ruta del cromo; ver MigasEnDialogo y MigasEnBarra.
	Estilo EstiloMigas

	segmentos []*Miga
	galones   []*iconos.Dibujo
}

var _ fyne.Widget = (*Migas)(nil)

// NuevasMigas crea la ruta. `nombre` es el `aria-label` del `<nav>`, ya traducido.
func NuevasMigas(nombre string, ruta []string, alIr func(int)) *Migas {
	m := &Migas{Ruta: ruta, AlIr: alIr}
	m.Nombre = nombre
	m.ExtendBaseWidget(m)
	return m
}

// AccessibilityRole: la ruta agrupa; lo pulsable es cada segmento.
func (m *Migas) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MedidaMigas es lo que ocupa la ruta entera con sus galones y sus huecos.
func MedidaMigas(m *Migas) fyne.Size {
	ancho := float32(0)
	for i, nombre := range m.Ruta {
		if i > 0 {
			ancho += 2*huecoMig + LadoGalonMiga
		}
		ancho += MedidaMiga(&Miga{Rotulo: nombre, Actual: i == len(m.Ruta)-1, Estilo: m.Estilo}).Width
	}
	return fyne.NewSize(ancho, m.Estilo.Alto())
}

// CreateRenderer construye un segmento por nombre y un galón entre cada dos.
func (m *Migas) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &migasRenderer{m: m}
	m.segmentos, m.galones = nil, nil
	for i, nombre := range m.Ruta {
		s := NuevaMiga(nombre, func() {
			if m.AlIr != nil {
				m.AlIr(i)
			}
		})
		s.Actual, s.Estilo = i == len(m.Ruta)-1, m.Estilo
		m.segmentos = append(m.segmentos, s)
		if i > 0 {
			g := iconos.TrazoConTinta(iconos.ChevronDerecha, LadoGalonMiga, m.tintaGalon(v))
			m.galones = append(m.galones, g)
			r.objetos = append(r.objetos, g.Objeto())
		}
		r.objetos = append(r.objetos, s)
	}
	return r
}

// tintaGalon es `text-text/50` en el cromo y `text-text-subtle` dentro de un diálogo.
func (m *Migas) tintaGalon(v tema.Variante) color.NRGBA {
	if m.Estilo.Galon != "" {
		return tema.Color(m.Estilo.Galon, v)
	}
	return medioTono(tema.Color("text", v))
}

// medioTono es el `text-text/50` del galón: el color del texto a media opacidad.
func medioTono(c color.NRGBA) color.NRGBA {
	c.A /= 2
	return c
}

type migasRenderer struct {
	m       *Migas
	objetos []fyne.CanvasObject
}

func (r *migasRenderer) MinSize() fyne.Size           { return MedidaMigas(r.m) }
func (r *migasRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *migasRenderer) Destroy()                     {}

// Layout pone segmento, galón, segmento… y encoge todos por igual si la ruta no cabe.
func (r *migasRenderer) Layout(t fyne.Size) {
	pedido := MedidaMigas(r.m).Width
	factor := float32(1)
	if pedido > t.Width && pedido > 0 {
		// Los galones y los huecos no encogen (`shrink-0`): el déficit se reparte
		// entre los rótulos, en proporción a lo que cada uno pedía.
		fijo := float32(len(r.m.galones)) * (2*huecoMig + LadoGalonMiga)
		factor = max(t.Width-fijo, 0) / max(pedido-fijo, 1)
	}
	x := float32(0)
	for i, s := range r.m.segmentos {
		if i > 0 {
			x += huecoMig
			r.m.galones[i-1].Colocar(x, (t.Height-LadoGalonMiga)/2, LadoGalonMiga)
			x += LadoGalonMiga + huecoMig
		}
		ancho := MedidaMiga(s).Width * factor
		alto := r.m.Estilo.Alto()
		s.Move(fyne.NewPos(x, (t.Height-alto)/2))
		s.Resize(fyne.NewSize(ancho, alto))
		x += ancho
	}
}

// Refresh repinta los galones con el tono del tema actual.
func (r *migasRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for _, g := range r.m.galones {
		g.TenirCon(r.m.tintaGalon(v))
	}
}
