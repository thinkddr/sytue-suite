// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Migas.tsx

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
	// Etiqueta es el `aria-label`, ya traducido, cuando el rótulo no dice qué hace: el
	// «…» de una ruta plegada. Vacía: el rótulo.
	Etiqueta string
}

var _ fyne.Widget = (*Miga)(nil)

// NuevaMiga crea un segmento.
func NuevaMiga(rotulo string, alPulsar func()) *Miga {
	m := &Miga{Rotulo: rotulo}
	m.Nombre, m.AlPulsar = rotulo, alPulsar
	m.ExtendBaseWidget(m)
	return m
}

// AccessibilityLabel es el nombre de la carpeta, o la Etiqueta si la hay.
func (m *Miga) AccessibilityLabel() string {
	if m.Etiqueta != "" {
		return m.Etiqueta
	}
	return m.Rotulo
}

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
// El último es el actual y los demás llevan a su carpeta.
//
// CUANDO NO CABE, PLIEGA, como Migas.tsx: con cuatro niveles o más, si la ruta entera
// no entra en su hueco se queda en «raíz › … › padre › actual», y el «…» abre un menú
// con los niveles que esconde. Antes se estrechaban todos a la vez y una ruta de ocho
// carpetas en un teléfono eran ocho «Pro…» ilegibles. Si plegada aún no cabe, lo que
// queda se estrecha en la misma proporción y se recorta con puntos suspensivos, que
// es lo que hace el `flex` con `min-w-0` y `truncate` de la web.
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
	// puntos es el «…» de la ruta plegada; nil si la ruta es demasiado corta.
	puntos *Miga
	// plegada dice si el último Layout escondió los niveles intermedios.
	plegada bool
	// menu es el de los niveles ocultos, mientras está abierto.
	menu *Menu
}

// MinimoParaPlegar: con menos de cuatro niveles no hay nada entre la raíz y el padre.
const MinimoParaPlegar = 4

// PliegueDeMigas es `pliegueDeMigas` de Migas.tsx: qué índices se ven de una ruta de
// `n` niveles y cuáles esconde el «…». Plegada es raíz, padre y actual; sin plegar,
// todos.
func PliegueDeMigas(n int, plegada bool) (visibles, ocultos []int) {
	for i := range n {
		visibles = append(visibles, i)
	}
	if !plegada || n < MinimoParaPlegar {
		return visibles, nil
	}
	return []int{0, n - 2, n - 1}, visibles[1 : n-2]
}

// Plegada dice si la ruta enseña ahora el «…» en vez de los niveles intermedios.
func (m *Migas) Plegada() bool { return m.plegada }

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
	m.puntos = nil
	if len(m.Ruta) >= MinimoParaPlegar {
		m.puntos = NuevaMiga("…", m.abrirOcultos)
		m.puntos.Etiqueta, m.puntos.Estilo = textoComun("ui.migas.ocultas"), m.Estilo
		m.puntos.Hide()
		r.objetos = append(r.objetos, m.puntos)
	}
	return r
}

// abrirOcultos despliega el menú de los niveles que el «…» esconde, alineado con su
// borde izquierdo (`align="start"`), como el Dropdown de la web.
func (m *Migas) abrirOcultos() {
	if m.puntos == nil || (m.menu != nil && m.menu.Abierto()) {
		return
	}
	_, ocultos := PliegueDeMigas(len(m.Ruta), true)
	elementos := make([]fyne.CanvasObject, 0, len(ocultos))
	for _, i := range ocultos {
		elementos = append(elementos, NuevoElementoMenu(m.Ruta[i], iconos.Ninguno, func() {
			if m.AlIr != nil {
				m.AlIr(i)
			}
		}))
	}
	m.menu = NuevoMenu(elementos...)
	m.menu.AlInicio, m.menu.Rotulo = true, m.puntos.Etiqueta
	m.menu.Mostrar(m.puntos)
}

// MenuOcultos es el menú del «…» mientras está abierto (nil si no).
func (m *Migas) MenuOcultos() *Menu {
	if m.menu == nil || !m.menu.Abierto() {
		return nil
	}
	return m.menu
}

// medidaPlegada es lo que ocupa la ruta plegada: raíz, «…», padre y actual con sus
// tres galones.
func (m *Migas) medidaPlegada() float32 {
	n := len(m.segmentos)
	ancho := MedidaMiga(m.segmentos[0]).Width + MedidaMiga(m.puntos).Width
	ancho += MedidaMiga(m.segmentos[n-2]).Width + MedidaMiga(m.segmentos[n-1]).Width
	return ancho + 3*(2*huecoMig+LadoGalonMiga)
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

// piezaMiga es un segmento de la fila que se pinta, con el galón que lleva delante
// (nil la raíz) y si encoge cuando la ruta no cabe.
type piezaMiga struct {
	galon  *iconos.Dibujo
	miga   *Miga
	encoge bool
}

// fila es lo que se pinta de la ruta, de izquierda a derecha, plegada o no.
func (m *Migas) fila() []piezaMiga {
	visibles, _ := PliegueDeMigas(len(m.segmentos), m.plegada)
	var fila []piezaMiga
	for _, i := range visibles {
		if i == 0 {
			fila = append(fila, piezaMiga{miga: m.segmentos[0], encoge: true})
			if m.plegada {
				// El «…» no encoge (`shrink-0`): es lo único que lleva a lo escondido.
				fila = append(fila, piezaMiga{galon: m.galones[0], miga: m.puntos})
			}
			continue
		}
		fila = append(fila, piezaMiga{galon: m.galones[i-1], miga: m.segmentos[i], encoge: true})
	}
	return fila
}

// verSolo enseña lo que va en la fila y esconde lo demás. Cambia la visibilidad solo
// de lo que cambia: cada Show o Hide refresca, y el Layout corre en cada cambio de
// tamaño de la ventana.
func (m *Migas) verSolo(fila []piezaMiga) {
	vistos := map[fyne.CanvasObject]bool{}
	for _, p := range fila {
		vistos[p.miga] = true
		if p.galon != nil {
			vistos[p.galon.Objeto()] = true
		}
	}
	todos := make([]fyne.CanvasObject, 0, len(m.segmentos)+len(m.galones)+1)
	for _, s := range m.segmentos {
		todos = append(todos, s)
	}
	for _, g := range m.galones {
		todos = append(todos, g.Objeto())
	}
	if m.puntos != nil {
		todos = append(todos, m.puntos)
	}
	for _, o := range todos {
		if v := vistos[o]; v && !o.Visible() {
			o.Show()
		} else if !v && o.Visible() {
			o.Hide()
		}
	}
}

// Layout pone segmento, galón, segmento…; pliega los intermedios en «…» si la ruta
// no cabe, y encoge por igual lo que quede si ni así cabe.
func (r *migasRenderer) Layout(t fyne.Size) {
	m := r.m
	pedido := MedidaMigas(m).Width
	m.plegada = m.puntos != nil && pedido > t.Width
	fila := m.fila()
	m.verSolo(fila)
	if m.plegada {
		pedido = m.medidaPlegada()
	}

	factor := float32(1)
	if pedido > t.Width && pedido > 0 {
		// Los galones y los huecos no encogen (`shrink-0`), ni el «…»: el déficit se
		// reparte entre los rótulos, en proporción a lo que cada uno pedía.
		fijo := float32(len(fila)-1) * (2*huecoMig + LadoGalonMiga)
		if m.plegada {
			fijo += MedidaMiga(m.puntos).Width
		}
		factor = max(t.Width-fijo, 0) / max(pedido-fijo, 1)
	}
	x := float32(0)
	alto := m.Estilo.Alto()
	for _, p := range fila {
		if p.galon != nil {
			x += huecoMig
			p.galon.Colocar(x, (t.Height-LadoGalonMiga)/2, LadoGalonMiga)
			x += LadoGalonMiga + huecoMig
		}
		ancho := MedidaMiga(p.miga).Width
		if p.encoge {
			ancho *= factor
		}
		p.miga.Move(fyne.NewPos(x, (t.Height-alto)/2))
		p.miga.Resize(fyne.NewSize(ancho, alto))
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
