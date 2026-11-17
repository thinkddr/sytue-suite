// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// MarcoAjustes y SeccionAjustes son `SettingsShell` y `SettingsSection` de
// `@sytue/ui` (SettingsShell.tsx): el marco de una página de AJUSTES y sus secciones.
//
// Vivían en el Hub (`apps/hub/ajustes.go`, `bloques.go`) y Drive, Calendar y Mail se
// hacían su versión: §2.8 de la hoja dice que la configuración guiada no se
// reimplementa por app, así que el marco es de la suite (CONVENTIONS §13.2) y todas
// esas pantallas usan este.
//
// Medido en producción en «Tus datos» del portal (`medidas/portal/datos.*.json`): a
// 1280 la caja empieza en x = 213 y mide 854 (`max-w-5xl` de 896 menos el `sm:px-6`); a
// 390, en x = 14 y mide 362 (`px-4`). El título (24,5/31,5, 700) cae 21 por debajo del
// borde de arriba, la entrada 5,25 por debajo del título y la primera sección 21 más
// abajo. Cada sección es una `Card` con `shadow-sm`: 1 de borde, 14 de relleno y el `h2`
// de 15,75/22,75 arriba, con `gap-4` (14) hasta lo de dentro.
const (
	// AnchoMarcoAjustes es el `max-w-5xl`: 64rem, 1024 px.
	AnchoMarcoAjustes float32 = 64 * tema.Rem
	// MargenMarcoAjustesY es el `py-6`, y también el `mt-6` y el `gap-6` de las secciones.
	MargenMarcoAjustesY float32 = 6 * 0.25 * tema.Rem
	// MargenMarcoAjustesX es el `px-4` de la caja; desde `sm:`, MargenMarcoAjustesXSM.
	MargenMarcoAjustesX   float32 = 4 * 0.25 * tema.Rem
	MargenMarcoAjustesXSM float32 = 6 * 0.25 * tema.Rem
	// MargenEntradaAjustes es el `mt-1.5` de la descripción de la página.
	MargenEntradaAjustes float32 = 1.5 * 0.25 * tema.Rem
	// HuecoCabeceraAjustes es el `gap-4` entre el texto de la cabecera y sus acciones.
	HuecoCabeceraAjustes float32 = 4 * 0.25 * tema.Rem
	// MargenAvisoAjustes es el `mt-4` del aviso permanente (`banner`).
	MargenAvisoAjustes float32 = 4 * 0.25 * tema.Rem
	// AnchoLecturaAjustes es el `max-w-2xl` de las descripciones.
	AnchoLecturaAjustes float32 = 42 * tema.Rem
	// HuecoAccionesAjustes es el `gap-2` entre las acciones de una cabecera.
	HuecoAccionesAjustes float32 = 2 * 0.25 * tema.Rem

	// RellenoSeccionAjustes es el `p-4` del `CardBody` y HuecoSeccionAjustes su `gap-4`.
	RellenoSeccionAjustes float32 = 4 * 0.25 * tema.Rem
	HuecoSeccionAjustes   float32 = 4 * 0.25 * tema.Rem
	// HuecoSeccionPlana es el `gap-3` de la sección sin tarjeta, que es también el de
	// la cabecera de cualquier sección entre el título y sus acciones.
	HuecoSeccionPlana float32 = 3 * 0.25 * tema.Rem
	// MargenDescSeccionAjustes es el `mt-1` de la descripción de una sección.
	MargenDescSeccionAjustes float32 = 1 * 0.25 * tema.Rem
)

// Los interlineados de estas pantallas que no son los de la escala: `leading-relaxed`
// (1,625) sobre `text-base` y `text-sm`.
var (
	baseRelajada = tema.Escalon{Tam: tema.TextoBase.Tam, Linea: 1.625 * tema.TextoBase.Tam}
	smRelajada   = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
)

// CajaAjustes es dónde empieza y cuánto mide la caja de un `SettingsShell` para un
// ancho de ventana. El `sm:` mide la VENTANA, como todos los cortes.
func CajaAjustes(ventana float32) (x, ancho float32) { return cajaAjustes(ventana, ventana) }

// cajaAjustes es la caja en un sitio de ese ancho cuando la ventana mide otra cosa: la
// página de Drive o de Mail vive junto al menú lateral, y el `sm:px-6` sigue mirando la
// ventana, no la hoja.
func cajaAjustes(sitio, ventana float32) (x, ancho float32) {
	margen := MargenMarcoAjustesX
	if ventana >= CorteSM {
		margen = MargenMarcoAjustesXSM
	}
	caja := min(sitio, AnchoMarcoAjustes)
	return (sitio-caja)/2 + margen, max(caja-2*margen, 0)
}

// InteriorSeccionAjustes es lo que le queda a lo de dentro de una sección CON TARJETA
// de ese ancho: el borde y el `p-4` a cada lado.
func InteriorSeccionAjustes(ancho float32) float32 {
	return max(ancho-2*(tema.Borde+RellenoSeccionAjustes), 0)
}

// ---- La cabecera, común al marco y a la sección ----

// cabeceraAjustes es el `flex flex-wrap items-start` de las dos cabeceras: el título y
// su descripción en una columna `min-w-0 flex-1` y las acciones a la derecha
// (`shrink-0 items-center gap-2`). Como la columna es `flex-1` con base 0, nunca obliga
// a partir la fila: las acciones se quedan siempre a la derecha.
type cabeceraAjustes struct {
	titulo, desc *Parrafo
	acciones     []fyne.CanvasObject
	// hueco es el `gap` entre la columna y las acciones; margen, el `mt-*` de la
	// descripción.
	hueco, margen float32
}

func (c *cabeceraAjustes) anchoAcciones() (w, h float32) {
	for i, a := range c.acciones {
		m := a.MinSize()
		if i > 0 {
			w += HuecoAccionesAjustes
		}
		w += m.Width
		h = max(h, m.Height)
	}
	return w, h
}

func (c *cabeceraAjustes) columna(ancho float32) float32 {
	if len(c.acciones) == 0 {
		return ancho
	}
	w, _ := c.anchoAcciones()
	return max(ancho-w-c.hueco, 0)
}

func (c *cabeceraAjustes) altoTexto(col float32) float32 {
	h := AltoParrafo(c.titulo, col)
	if c.desc != nil && c.desc.Texto != "" {
		h += c.margen + AltoParrafo(c.desc, min(col, AnchoLecturaAjustes))
	}
	return h
}

func (c *cabeceraAjustes) alto(ancho float32) float32 {
	_, h := c.anchoAcciones()
	return max(c.altoTexto(c.columna(ancho)), h)
}

// poner coloca la cabecera y devuelve dónde acaba.
func (c *cabeceraAjustes) poner(x, y, ancho float32) float32 {
	col := c.columna(ancho)
	h := AltoParrafo(c.titulo, col)
	c.titulo.Move(fyne.NewPos(x, y))
	c.titulo.Resize(fyne.NewSize(col, h))
	if c.desc != nil && c.desc.Texto != "" {
		w := min(col, AnchoLecturaAjustes)
		c.desc.Move(fyne.NewPos(x, y+h+c.margen))
		c.desc.Resize(fyne.NewSize(w, AltoParrafo(c.desc, w)))
	}
	wa, ha := c.anchoAcciones()
	ax := x + ancho - wa
	for _, a := range c.acciones {
		m := a.MinSize()
		a.Move(fyne.NewPos(ax, y+(ha-m.Height)/2))
		a.Resize(m)
		ax += m.Width + HuecoAccionesAjustes
	}
	return y + c.alto(ancho)
}

func (c *cabeceraAjustes) objetos() []fyne.CanvasObject {
	out := []fyne.CanvasObject{c.titulo}
	if c.desc != nil && c.desc.Texto != "" {
		out = append(out, c.desc)
	}
	return append(out, c.acciones...)
}

// pintar pone el texto y la tinta del tema en curso.
func (c *cabeceraAjustes) pintar(titulo, desc string, v tema.Variante) {
	c.titulo.Texto, c.titulo.Tinta = titulo, tema.Color("text", v)
	c.titulo.Refresh()
	c.desc.Texto, c.desc.Tinta = desc, tema.Color("text-muted", v)
	c.desc.Refresh()
}

// ---- El marco de la página ----

// MarcoAjustes es `SettingsShell`: la caja centrada de `max-w-5xl` con el título de la
// página (`sytue-h2`), su descripción llana (`text-base leading-relaxed`, `max-w-2xl`),
// la acción principal a la derecha, un aviso permanente opcional y las secciones en
// columna con `gap-6`.
//
// Ocupa el ancho de la ventana —es la página— y coloca la caja él: quien lo usa no
// calcula márgenes. Su alto depende del ancho (`AltoPara`), así que va dentro de un
// desplazamiento con `AlAnchoFluido` o directamente como contenido de una vista.
//
// ponytail: el índice de secciones (`indice`) de la web no está: ninguna pantalla nativa
// tiene aún nueve secciones. Se añade cuando la primera lo pida.
type MarcoAjustes struct {
	widget.BaseWidget

	Titulo      string
	Descripcion string
	// Acciones es la acción principal de la página, a la derecha del título.
	Acciones []fyne.CanvasObject
	// Aviso es el `banner`: un aviso permanente sobre el contenido. Nil, ninguno.
	Aviso fyne.CanvasObject
	// Secciones son las de la página, normalmente `SeccionAjustes`.
	Secciones []fyne.CanvasObject
	// Ventana es el ancho de la VENTANA cuando el marco no la ocupa entera (va en la
	// hoja, junto al menú): es la que decide el `sm:` del margen. Cero, la del marco.
	Ventana float32

	cab *cabeceraAjustes
}

var (
	_ fyne.Widget     = (*MarcoAjustes)(nil)
	_ fyne.Accessible = (*MarcoAjustes)(nil)
)

// NuevoMarcoAjustes crea el marco con su título, su descripción y sus secciones.
func NuevoMarcoAjustes(titulo, descripcion string, secciones ...fyne.CanvasObject) *MarcoAjustes {
	m := &MarcoAjustes{Titulo: titulo, Descripcion: descripcion, Secciones: secciones}
	m.ExtendBaseWidget(m)
	return m
}

// AccessibilityRole: un contenedor; lo que se lee son sus hijos.
func (m *MarcoAjustes) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AccessibilityLabel es el título de la página.
func (m *MarcoAjustes) AccessibilityLabel() string { return m.Titulo }

// MedidaMarcoAjustes es lo que ocupa el marco en una ventana de ese ancho: todo el
// ancho y el alto de lo que lleva. Es la única cuenta (regla 2 de ui.go).
func MedidaMarcoAjustes(m *MarcoAjustes, ventana float32) fyne.Size {
	return fyne.NewSize(ventana, m.colocar(ventana, false))
}

// AltoPara es el alto a ese ancho de ventana, para `AltoA` y los desplazamientos.
func (m *MarcoAjustes) AltoPara(ventana float32) float32 {
	return MedidaMarcoAjustes(m, ventana).Height
}

// cabecera es la cabecera del marco con los textos y las acciones de ahora. Vive en el
// widget y no en el renderer porque la medida se puede pedir antes de que el marco esté
// en un lienzo, y tiene que salir la misma.
func (m *MarcoAjustes) cabecera() *cabeceraAjustes {
	if m.cab == nil {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		m.cab = &cabeceraAjustes{
			titulo: NuevoParrafo("", EscalonTitulo(2), PesoTitulo(2), "text", v),
			desc:   NuevoParrafo("", baseRelajada, Normal, "text-muted", v),
			hueco:  HuecoCabeceraAjustes,
			margen: MargenEntradaAjustes,
		}
	}
	m.cab.titulo.Texto, m.cab.desc.Texto, m.cab.acciones = m.Titulo, m.Descripcion, m.Acciones
	return m.cab
}

// colocar mide (y, con `mover`, coloca) el marco a ese ancho; el corte `sm:` es el de
// `Ventana` si la hay. Devuelve el alto: la misma cuenta para medir y para colocar.
func (m *MarcoAjustes) colocar(ventana float32, mover bool) float32 {
	cab := m.cabecera()
	corte := ventana
	if m.Ventana > 0 {
		corte = m.Ventana
	}
	x, ancho := cajaAjustes(ventana, corte)
	y := MargenMarcoAjustesY
	if mover {
		y = cab.poner(x, y, ancho)
	} else {
		y += cab.alto(ancho)
	}
	if m.Aviso != nil {
		y += MargenAvisoAjustes
		h := AltoA(m.Aviso, ancho)
		if mover {
			m.Aviso.Move(fyne.NewPos(x, y))
			m.Aviso.Resize(fyne.NewSize(ancho, h))
		}
		y += h
	}
	for _, s := range m.Secciones {
		y += MargenMarcoAjustesY // el `mt-6` antes de la primera y el `gap-6` entre ellas
		h := AltoA(s, ancho)
		if mover {
			s.Move(fyne.NewPos(x, y))
			s.Resize(fyne.NewSize(ancho, h))
		}
		y += h
	}
	return y + MargenMarcoAjustesY
}

// CreateRenderer compone la cabecera, el aviso y las secciones. Termina en Refresh.
func (m *MarcoAjustes) CreateRenderer() fyne.WidgetRenderer {
	r := &marcoAjustesRenderer{m: m}
	r.Refresh()
	return r
}

type marcoAjustesRenderer struct {
	m       *MarcoAjustes
	objetos []fyne.CanvasObject
}

func (r *marcoAjustesRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.m.colocar(r.m.Size().Width, false))
}
func (r *marcoAjustesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *marcoAjustesRenderer) Destroy()                     {}

func (r *marcoAjustesRenderer) Layout(t fyne.Size) { r.m.colocar(t.Width, true) }

// Refresh repinta los textos con el tema en curso y rehace la lista de objetos: las
// acciones y las secciones se pueden cambiar entre un pintado y otro.
func (r *marcoAjustesRenderer) Refresh() {
	m := r.m
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cab := m.cabecera()
	cab.pintar(m.Titulo, m.Descripcion, v)
	r.objetos = cab.objetos()
	if m.Aviso != nil {
		r.objetos = append(r.objetos, m.Aviso)
	}
	r.objetos = append(r.objetos, m.Secciones...)
	r.Layout(m.Size())
}

// ---- La sección ----

// SeccionAjustes es `SettingsSection`: una tarjeta (`Card` con `shadow-sm` y `CardBody`
// `p-4 gap-4`) con el título de la sección (`sytue-h4`), una frase que dice qué hace
// (`text-sm leading-relaxed`, `max-w-2xl`), sus acciones a la derecha —guardar por
// sección, no un botón gigante al final— y lo de dentro debajo, a todo el ancho.
//
// `Plana` es el `plain` de la web: sin tarjeta, solo el encabezado sobre el contenido,
// con `gap-3`.
type SeccionAjustes struct {
	widget.BaseWidget

	Titulo      string
	Descripcion string
	// Acciones van a la derecha del título (`shrink-0 items-center gap-2`).
	Acciones []fyne.CanvasObject
	// Plana quita la tarjeta (`plain`).
	Plana bool
	// Cuerpo es lo de dentro; su alto se pide con `AltoA` al ancho interior. Nil, nada.
	Cuerpo fyne.CanvasObject

	cab *cabeceraAjustes
}

var (
	_ fyne.Widget     = (*SeccionAjustes)(nil)
	_ fyne.Accessible = (*SeccionAjustes)(nil)
)

// NuevaSeccionAjustes crea la sección con tarjeta.
func NuevaSeccionAjustes(titulo, descripcion string, cuerpo fyne.CanvasObject) *SeccionAjustes {
	s := &SeccionAjustes{Titulo: titulo, Descripcion: descripcion, Cuerpo: cuerpo}
	s.ExtendBaseWidget(s)
	return s
}

// AccessibilityRole: la `Card` de la web es un `<section>` con su `sectionLabel`.
func (s *SeccionAjustes) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AccessibilityLabel es el título, que es el `sectionLabel` de la tarjeta.
func (s *SeccionAjustes) AccessibilityLabel() string { return s.Titulo }

// marco es lo que la tarjeta añade a cada lado: el borde y el `p-4`. Cero, plana.
func (s *SeccionAjustes) marco() float32 {
	if s.Plana {
		return 0
	}
	return tema.Borde + RellenoSeccionAjustes
}

func (s *SeccionAjustes) hueco() float32 {
	if s.Plana {
		return HuecoSeccionPlana
	}
	return HuecoSeccionAjustes
}

// MedidaSeccionAjustes es lo que ocupa la sección a ese ancho. El ancho no baja de lo
// que piden las acciones o el cuerpo con el marco de la tarjeta. Es la única cuenta.
func MedidaSeccionAjustes(s *SeccionAjustes, ancho float32) fyne.Size {
	ancho = max(ancho, s.anchoMinimo())
	return fyne.NewSize(ancho, s.colocar(0, 0, ancho, false))
}

// AltoPara es el alto a ese ancho, para `AltoA` y para el marco.
func (s *SeccionAjustes) AltoPara(ancho float32) float32 {
	return MedidaSeccionAjustes(s, ancho).Height
}

// cabecera es la de la sección con los textos de ahora (ver MarcoAjustes.cabecera).
func (s *SeccionAjustes) cabecera() *cabeceraAjustes {
	if s.cab == nil {
		v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
		s.cab = &cabeceraAjustes{
			titulo: NuevoParrafo("", EscalonTitulo(4), PesoTitulo(4), "text", v),
			desc:   NuevoParrafo("", smRelajada, Normal, "text-muted", v),
			hueco:  HuecoSeccionPlana,
			margen: MargenDescSeccionAjustes,
		}
	}
	s.cab.titulo.Texto, s.cab.desc.Texto, s.cab.acciones = s.Titulo, s.Descripcion, s.Acciones
	return s.cab
}

func (s *SeccionAjustes) anchoMinimo() float32 {
	w, _ := s.cabecera().anchoAcciones()
	if s.Cuerpo != nil {
		w = max(w, s.Cuerpo.MinSize().Width)
	}
	return w + 2*s.marco()
}

// colocar mide (y, con `mover`, coloca) la sección a ese ancho y devuelve su alto.
func (s *SeccionAjustes) colocar(x, y, ancho float32, mover bool) float32 {
	cab := s.cabecera()
	marco := s.marco()
	interior := max(ancho-2*marco, 0)
	cy := y + marco
	if mover {
		cy = cab.poner(x+marco, cy, interior)
	} else {
		cy += cab.alto(interior)
	}
	if s.Cuerpo != nil {
		cy += s.hueco()
		h := AltoA(s.Cuerpo, interior)
		if mover {
			s.Cuerpo.Move(fyne.NewPos(x+marco, cy))
			s.Cuerpo.Resize(fyne.NewSize(interior, h))
		}
		cy += h
	}
	return cy + marco - y
}

// CreateRenderer compone la sombra, la tarjeta, la cabecera y el cuerpo. Termina en
// Refresh (regla 7).
func (s *SeccionAjustes) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &seccionAjustesRenderer{
		s: s,
		// `shadow-sm`, la misma que miden las tarjetas del lanzador.
		sombra: NuevaSombra(tema.SombraSM, radioTarjeta()),
		caja:   NuevaCaja(tema.Color("surface", v), tema.Color("border", v), radioTarjeta()),
	}
	r.Refresh()
	return r
}

type seccionAjustesRenderer struct {
	s       *SeccionAjustes
	sombra  *Sombra
	caja    *Caja
	objetos []fyne.CanvasObject
}

func (r *seccionAjustesRenderer) MinSize() fyne.Size {
	return MedidaSeccionAjustes(r.s, r.s.Size().Width)
}
func (r *seccionAjustesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *seccionAjustesRenderer) Destroy()                     {}

func (r *seccionAjustesRenderer) Layout(t fyne.Size) {
	alto := r.s.colocar(0, 0, t.Width, true)
	if r.s.Plana {
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.sombra.Ajustar(fyne.NewPos(0, 0), fyne.NewSize(t.Width, alto), v)
	r.caja.Redimensionar(fyne.NewSize(t.Width, alto))
}

// Refresh repinta la tarjeta y los textos con el tema en curso y rehace la lista de
// objetos.
func (r *seccionAjustesRenderer) Refresh() {
	s := r.s
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("surface", v), tema.Color("border", v))
	cab := s.cabecera()
	cab.pintar(s.Titulo, s.Descripcion, v)
	r.objetos = nil
	if !s.Plana {
		r.objetos = append([]fyne.CanvasObject{r.sombra.Objeto()}, r.caja.Objetos()...)
	}
	r.objetos = append(r.objetos, cab.objetos()...)
	if s.Cuerpo != nil {
		r.objetos = append(r.objetos, s.Cuerpo)
	}
	r.Layout(s.Size())
}
