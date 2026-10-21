// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/SectionHeader.tsx

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// La cabecera de una vista (SectionHeader.tsx): fondo blanco, título en `sytue-h4` y
// un filete abajo. Es el patrón canónico del título de bandeja, lista o vista de cada
// app, y NO lleva el color del producto: ese vive en el menú y en la barra.
//
// Medida en producción, y con dos alturas que salen de la misma regla:
//
//	42     «Recibidos · 17 sin leer» (Mail) y «Hoy · Ver el calendario» (Hub):
//	       el contenido (22,75) no llega al `min-h-12`, y el mínimo manda
//	46,5   la cabecera de Calendar, que mete un botón de 28: ahí manda el contenido
//
// O sea `max(42, contenido + py-2.5 × 2 + 1)`. Los 42 son `min-h-12` con el rem de 14,
// y el 1 es el `border-b`, que en `border-box` va DENTRO del alto.
var (
	cabeceraPX     = tema.Espacio(4)   // px-4
	cabeceraPYv    = tema.Espacio(2.5) // py-2.5
	huecoCab       = tema.Espacio(3)   // gap-x-3
	huecoAcc       = tema.Espacio(2)   // gap-2 entre acciones
	huecoLineasCab = tema.Espacio(2)   // gap-y-2 entre las dos líneas
)

// MinCabeceraApp es el `min-h-12`: 42 con el rem de 14.
const MinCabeceraApp = 3 * tema.Rem

// CabeceraApp es SectionHeader: icono opcional, título, subtítulo opcional y acciones
// a la derecha.
//
// Con `flex-wrap`: si el icono, el título a su ancho natural y las acciones no caben en
// una línea, las acciones BAJAN a una segunda, a la izquierda y a `gap-y-2` (7). Así la
// cabecera del panel de Contacts mide 76,25 a 1280 —1 + 2 × 8,75 + 22,75 + 7 + 28—, que
// es lo medido en producción, en vez de recortar el título a «Contac…». Y ahí abajo el
// `<div class="flex flex-wrap gap-2">` de las acciones vuelve a envolver ENTRE ELLAS si
// no caben en el ancho, cada línea a `gap-2`.
type CabeceraApp struct {
	pulsable

	// Titulo va en `sytue-h4`: 15,75/22,75 semi-negrita.
	Titulo string
	// Subtitulo va debajo, en `text-sm` apagado. Vacío, no ocupa.
	Subtitulo string
	// Icono es la marca de la izquierda, en una caja de 17,5. Puede ser nil. Lo pinta
	// quien lo trae, con su color: una marca de producto. Para un glifo va Glifo.
	Icono fyne.CanvasObject
	// Glifo es el icono del catálogo que la cabecera TIÑE, como la web: el `<span
	// class="text-text-muted">` de SectionHeader le da `currentColor`. Gana a Icono.
	Glifo *iconos.Dibujo
	// Acciones son los botones de la derecha, separados por `gap-2`.
	Acciones []fyne.CanvasObject
	// Ancho es el que se usa para DECLARAR el alto, que con `flex-wrap` depende del
	// ancho. Con 0, el último que le dieron. Quien la pone en una columna de ancho
	// conocido —el panel de lista— lo dice aquí, como con `Parrafo.A`.
	Ancho float32
}

var _ fyne.Widget = (*CabeceraApp)(nil)

// NuevaCabeceraApp crea la cabecera con su título.
func NuevaCabeceraApp(titulo string, acciones ...fyne.CanvasObject) *CabeceraApp {
	c := &CabeceraApp{Titulo: titulo, Acciones: acciones}
	c.Nombre = titulo
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel es el título, que en la web es el encabezado de la vista.
func (c *CabeceraApp) AccessibilityLabel() string { return c.Titulo }

// AccessibilityRole: es el encabezado de la vista, no un control.
func (c *CabeceraApp) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// repartoCabecera es la geometría de `flex flex-wrap` a un ancho: si las acciones bajan,
// el alto de la línea del título y el de TODAS las de las acciones, que con su propio
// `flex-wrap` pueden ser varias. `filas` son cuántas acciones van en cada una.
type repartoCabecera struct {
	envuelve       bool
	desliza        bool
	linea1, linea2 float32
	filas          []filaAcciones
}

// filaAcciones es una línea de acciones de la segunda fila: cuántas y cuánto mide.
type filaAcciones struct {
	n    int
	alto float32
}

// marca es lo que va a la izquierda del título: el glifo que tiñe la cabecera o el
// objeto que trae su color.
func (c *CabeceraApp) marca() fyne.CanvasObject {
	if c.Glifo != nil {
		return c.Glifo.Objeto()
	}
	return c.Icono
}

// envolverAcciones reparte las acciones en líneas de `ancho` como su `flex flex-wrap
// gap-2`: cada una entra en la línea si cabe con su hueco, y si no abre otra.
func envolverAcciones(acciones []fyne.CanvasObject, ancho float32) []filaAcciones {
	var filas []filaAcciones
	usado := float32(0)
	for _, a := range acciones {
		m := a.MinSize()
		if len(filas) == 0 || usado+huecoAcc+m.Width > ancho {
			filas = append(filas, filaAcciones{})
			usado = -huecoAcc
		}
		f := &filas[len(filas)-1]
		f.n++
		f.alto = max(f.alto, m.Height)
		usado += huecoAcc + m.Width
	}
	return filas
}

func repartir(c *CabeceraApp, ancho float32) repartoCabecera {
	var r repartoCabecera
	r.linea1 = EscalonTitulo(4).Linea
	if c.Subtitulo != "" {
		r.linea1 += tema.TextoSM.Linea
	}
	natural := Medir(c.Titulo, EscalonTitulo(4), PesoTitulo(4)).Width
	if c.Subtitulo != "" {
		natural = max(natural, Medir(c.Subtitulo, tema.TextoSM, Normal).Width)
	}
	if c.marca() != nil {
		r.linea1 = max(r.linea1, CajaIcono)
		natural += CajaIcono + huecoCab
	}
	acciones := float32(0)
	for i, a := range c.Acciones {
		m := a.MinSize()
		if i > 0 {
			acciones += huecoAcc
		}
		acciones += m.Width
		r.linea2 = max(r.linea2, m.Height)
	}
	r.envuelve = len(c.Acciones) > 0 && ancho > 0 && natural+huecoCab+acciones > ancho-2*cabeceraPX
	// EN TÁCTIL las acciones que no caben NO se reparten en líneas: van en una
	// tira deslizable (la misma pieza de BarraDeLote), que es como la web las
	// pone en móvil. Apilar dos o tres filas de botones sobre la lista robaba
	// la pantalla que la lista venía a usar. `linea2` ya quedó arriba con el
	// alto del botón más alto, que es el de la tira.
	if r.envuelve && EsTactil() {
		r.desliza = true
		r.filas = nil
		return r
	}
	if !r.envuelve {
		r.linea1, r.linea2 = max(r.linea1, r.linea2), 0
		return r
	}
	r.filas = envolverAcciones(c.Acciones, ancho-2*cabeceraPX)
	r.linea2 = 0
	for i, f := range r.filas {
		if i > 0 {
			r.linea2 += huecoAcc // `gap-2`, que también es el de filas
		}
		r.linea2 += f.alto
	}
	return r
}

// MedidaCabeceraApp es lo que ocupa: el `min-h-12` o, si el contenido pasa de ahí, el
// contenido con su relleno y su filete. Con las acciones en su segunda fila, las dos
// líneas y su `gap-y-2`.
func MedidaCabeceraApp(c *CabeceraApp) fyne.Size {
	ancho := c.Ancho
	if ancho == 0 {
		ancho = c.Size().Width
	}
	r := repartir(c, ancho)
	contenido := r.linea1
	if r.envuelve {
		contenido += huecoLineasCab + r.linea2
	}
	alto := max(MinCabeceraApp, contenido+2*cabeceraPYv+tema.Borde)
	return fyne.NewSize(0, alto)
}

// CreateRenderer construye fondo, filete, título y subtítulo.
func (c *CabeceraApp) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &cabeceraRenderer{
		c:      c,
		fondo:  canvas.NewRectangle(tema.Color("surface", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		titulo: Texto(c.Titulo, EscalonTitulo(4), PesoTitulo(4), tema.Color("text", v)),
	}
	// El subtítulo existe SIEMPRE, escondido si va vacío: con él creado solo cuando ya
	// venía, un subtítulo que llegaba después —el recuento de una lista al cargar— no
	// se pintaba nunca.
	r.subtitulo = Texto(c.Subtitulo, tema.TextoSM, Normal, tema.Color("text-muted", v))
	r.objetos = []fyne.CanvasObject{r.fondo, r.filete, r.titulo, r.subtitulo}
	if o := c.marca(); o != nil {
		r.objetos = append(r.objetos, o)
	}
	if c.Glifo != nil {
		c.Glifo.Tenir("text-muted", v)
	}
	r.objetosFijos = len(r.objetos)
	// El reparto manda aquí OTRA VEZ y con el ancho de la medida: dice si las
	// acciones van sueltas a la derecha o en la tira deslizable de táctil.
	rep := repartir(c, func() float32 {
		if c.Ancho > 0 {
			return c.Ancho
		}
		return c.Size().Width
	}())
	if rep.desliza {
		r.prepararTira(rep)
	}
	r.objetos = append(r.objetos, c.Acciones...)
	return r
}

type cabeceraRenderer struct {
	c                 *CabeceraApp
	fondo, filete     *canvas.Rectangle
	titulo, subtitulo *canvas.Text
	// Se conserva la tira al cambiar de orientación, pero solo se pinta cuando las
	// acciones no caben. En la vista ancha vuelven a colgar directo de la cabecera.
	tira                   *TiraDeslizable
	desliza                bool
	objetosFijos           int
	objetos, objetosEnTira []fyne.CanvasObject
}

func (r *cabeceraRenderer) MinSize() fyne.Size { return MedidaCabeceraApp(r.c) }
func (r *cabeceraRenderer) Objects() []fyne.CanvasObject {
	if r.desliza {
		return r.objetosEnTira
	}
	return r.objetos
}
func (r *cabeceraRenderer) Destroy() {}

func (r *cabeceraRenderer) prepararTira(rep repartoCabecera) {
	r.desliza = rep.desliza
	if r.tira != nil && r.tira.Visible() != rep.desliza {
		// Los callbacks de foco siguen unidos a la tira cuando las acciones vuelven
		// a la cabecera ancha. Ocultarla evita que les cambie las coordenadas desde allí.
		if rep.desliza {
			r.tira.Show()
		} else {
			r.tira.Hide()
		}
	}
	if !rep.desliza || r.tira != nil {
		return
	}
	r.tira = NuevaTiraDeslizable(rep.linea2, huecoAcc, r.c.Acciones...)
	r.tira.AnchoPropio = true
	r.objetosEnTira = append(append([]fyne.CanvasObject{}, r.objetos[:r.objetosFijos]...), r.tira)
}

// Layout coloca icono, bloque de texto y acciones, centrados en su línea
// (`items-center`); con `flex-wrap`, las acciones en la segunda, a la izquierda.
func (r *cabeceraRenderer) Layout(t fyne.Size) {
	c := r.c
	r.fondo.Resize(t)
	r.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))

	rep := repartir(c, t.Width)
	r.prepararTira(rep)
	contenido := rep.linea1
	if rep.envuelve {
		contenido += huecoLineasCab + rep.linea2
	}
	y1 := (t.Height - tema.Borde - contenido) / 2 // arriba de la primera línea
	x := cabeceraPX
	if o := c.marca(); o != nil {
		// `grid h-5 w-5 place-items-center`: el glifo, a su tamaño, centrado en la caja.
		m := fyne.NewSize(CajaIcono, CajaIcono)
		if c.Glifo != nil {
			m = c.Glifo.Medida()
		}
		o.Move(fyne.NewPos(x+(CajaIcono-m.Width)/2, y1+(rep.linea1-m.Height)/2))
		o.Resize(m)
		x += CajaIcono + huecoCab
	}
	der := t.Width - cabeceraPX
	switch {
	case rep.desliza:
		// La tira ocupa la segunda línea entera; el título conserva su ancho,
		// como con la envoltura (las acciones ya no están a su derecha).
		r.tira.Move(fyne.NewPos(cabeceraPX, y1+rep.linea1+huecoLineasCab))
		r.tira.Resize(fyne.NewSize(t.Width-2*cabeceraPX, rep.linea2))
	case rep.envuelve:
		y2, i := y1+rep.linea1+huecoLineasCab, 0
		for _, f := range rep.filas {
			xa := cabeceraPX
			for _, a := range c.Acciones[i : i+f.n] {
				m := a.MinSize()
				a.Move(fyne.NewPos(xa, y2+(f.alto-m.Height)/2))
				a.Resize(m)
				xa += m.Width + huecoAcc
			}
			i += f.n
			y2 += f.alto + huecoAcc
		}
	default:
		for i := len(c.Acciones) - 1; i >= 0; i-- {
			m := c.Acciones[i].MinSize()
			der -= m.Width
			c.Acciones[i].Move(fyne.NewPos(der, y1+(rep.linea1-m.Height)/2))
			c.Acciones[i].Resize(m)
			der -= huecoAcc
		}
		if len(c.Acciones) > 0 {
			der -= huecoCab - huecoAcc
		}
	}
	bloque := EscalonTitulo(4).Linea
	if c.Subtitulo != "" {
		bloque += tema.TextoSM.Linea
	}
	y := y1 + (rep.linea1-bloque)/2
	r.titulo.Text = recorta(c.Titulo, EscalonTitulo(4), PesoTitulo(4), der-x)
	ColocarTexto(r.titulo, EscalonTitulo(4), x, y)
	r.subtitulo.Hidden = c.Subtitulo == ""
	r.subtitulo.Text = recorta(c.Subtitulo, tema.TextoSM, Normal, der-x)
	ColocarTexto(r.subtitulo, tema.TextoSM, x, y+EscalonTitulo(4).Linea)
}

// Refresh repinta con los tokens del tema actual y RECOLOCA.
//
// Lo segundo no es una precaución: el título se escribe en Layout —se recorta al hueco
// que dejan las acciones— y Fyne solo llama a Layout cuando cambia el tamaño. Sin esto,
// la cabecera de una bandeja que cambia de carpeta seguía diciendo la de antes, y su
// conteo «1 sin leer» después de leerlo (medido en la captura de Mail con datos).
func (r *cabeceraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("surface", v)
	r.fondo.Refresh()
	r.filete.FillColor = tema.Color("border", v)
	r.filete.Refresh()
	r.titulo.Color = tema.Color("text", v)
	r.subtitulo.Color = tema.Color("text-muted", v)
	if r.c.Glifo != nil {
		r.c.Glifo.Tenir("text-muted", v)
	}
	for _, a := range r.c.Acciones {
		a.Refresh()
	}
	r.Layout(r.c.Size())
	r.titulo.Refresh()
	r.subtitulo.Refresh()
}
