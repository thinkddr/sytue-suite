// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # La cabecera de vista de Sytue Drive, medida
//
// No es `ui.CabeceraApp` (SectionHeader): esa lleva título y subtítulo y `px-4`, y esta
// lleva la RUTA y `sm:px-6`. En la web tampoco es el mismo componente — está escrita a
// mano en FileBrowser.tsx.
//
// Medido a 1280×800 sobre `corporate.sytue.eu/drive`:
//
//	cabecera   48,5 de alto = `py-2.5` (8,75) × 2 + 30 de contenido + `border-b` (1)
//	canalón    21 desde `sm:` (`sm:px-6`), 14 por debajo (`px-4`)
//	ruta       el primer segmento en x=281 = 260 (carril) + 21
//	buscador   28 × 196, en x=833,25
//	«Nuevo»    28 × 76,25, en x=1036,25  ·  «Subir» 28 × 71, en x=1119,5
//	conmutador 62 × 32, el `SegmentedControl` de iconos solos (dos `w-7` y el `p-0.5`)
//
// Antes el conmutador llevaba `border` y medía 30 (28 + 2): era el hijo más alto y
// estiraba la línea. Ahora es un carril sin borde del alto de un control, así que la
// línea la ponen todos por igual y la cabecera mide 2 menos.
const (
	// AltoControlCromo es el `h-8` de los controles de la cabecera.
	AltoControlCromo float32 = 8 * 0.25 * tema.Rem
	// AltoLineaCabecera es lo que mide la línea de la cabecera: la de un control, que
	// es lo que miden el buscador, los botones y el conmutador de vista.
	AltoLineaCabecera float32 = AltoControlCromo
	// RellenoYCabecera es `py-2.5`.
	RellenoYCabecera float32 = 2.5 * 0.25 * tema.Rem
	// AltoCabeceraVista es el total, con el `border-b` por dentro de la caja.
	AltoCabeceraVista float32 = 2*RellenoYCabecera + AltoLineaCabecera + tema.Borde
	// HuecoCabecera es `gap-x-3` entre la ruta y el grupo de acciones.
	HuecoCabecera float32 = 3 * 0.25 * tema.Rem
	// HuecoAcciones es `gap-2` dentro del grupo.
	HuecoAcciones float32 = 2 * 0.25 * tema.Rem
	// AnchoBuscador es `sm:w-56`.
	AnchoBuscador float32 = 56 * 0.25 * tema.Rem
	// MargenCromo es `ml-1` entre «Subir» y el conmutador.
	MargenCromo float32 = 1 * 0.25 * tema.Rem
)

// CanalonCabecera es `px-4 sm:px-6`, resuelto contra el ancho de la VENTANA porque
// `sm:` es una media query y se mide con el rem inicial del navegador, no con el 14.
func CanalonCabecera(ventana float32) float32 {
	if ventana == 0 || ventana >= ui.CorteSM {
		return 6 * 0.25 * tema.Rem
	}
	return 4 * 0.25 * tema.Rem
}

// ---- El conmutador de vista (lista / cuadrícula) ----

// NuevoConmutadorVista es el `SegmentedControl` de iconos solos de FileBrowser.tsx: el
// `ui.Segmentado` de la suite en su talla mediana, dos cuadrados `w-7 h-7` con el menú y
// la rejilla dentro, sobre el carril `bg-subtle` con su `p-0.5` (62 × 32). La elegida
// va sobre `surface` en `primary-text`. Antes era un par de botones con borde y
// `divide-x` escrito aquí a mano, la tercera copia de la misma receta.
//
// Es un grupo de radios: una parada de Tab y las flechas; los rótulos, ya traducidos,
// quedan como nombre accesible de cada opción.
func NuevoConmutadorVista(grupo, lista, rejilla string, enRejilla bool, alCambiar func(bool)) *ui.Segmentado {
	s := ui.NuevoSegmentado(grupo, []string{lista, rejilla}, func(i int) {
		if alCambiar != nil {
			alCambiar(i == 1)
		}
	})
	s.Talla, s.SoloIcono = ui.SegmentoMD, true
	s.SubeTeclas = true // «v», «?», Supr o F2 justo después de cambiar de vista con un clic
	s.Iconos = []iconos.Icono{iconos.Menu, iconos.Rejilla}
	if enRejilla {
		s.Elegida = 1
	}
	return s
}

// ---- La cabecera de vista ----

// CabeceraVista es la barra de arriba del explorador: la ruta a la izquierda y el
// grupo de acciones a la derecha, sobre `surface` y con su `border-b`.
//
// Es un CONTENEDOR y no un widget: no se pulsa, no se enfoca y no se anuncia; lo
// interactivo son sus hijos, que ya son widgets. La regla del paquete `ui` es elegir por
// comportamiento, y este no tiene ninguno.
type CabeceraVista struct {
	// Ruta son las migas, de la raíz a la carpeta actual.
	Ruta *ui.Migas
	// Acciones van a la derecha, en el orden de la web: el buscador y, detrás, el
	// conmutador de vista. «Nuevo» y «Subir» estaban aquí hasta el 2026-11-25: ahora son
	// el «+» flotante de la pantalla.
	Acciones []fyne.CanvasObject
	// Conmutador va el último y se separa con `ml-1` ADEMÁS del `gap-2` del grupo:
	// 10,5 en total. Medido: «Subir» acababa en 1190,5 y el conmutador empezaba en 1201.
	Conmutador fyne.CanvasObject
	// Orden es el selector «Ordenar» (`SelectorOrden` de la web), delante del
	// conmutador: en la cuadrícula, que no tiene cabeceras de columna, y en la lista por
	// debajo de 768, donde «Tamaño» y «Modificado» no se pintan. Nil: no se ofrece.
	Orden fyne.CanvasObject
	// Ventana es el ancho de la VENTANA en px de CSS: decide el canalón (`sm:px-6`).
	Ventana float32
	// AlCambiarAlto lo llama Layout cuando el ancho cambia el número de líneas: quien la
	// apila vuelve a colocarla.
	AlCambiarAlto func()

	ancho float32 // el de la última vez que se colocó
}

var _ fyne.Layout = (*CabeceraVista)(nil)

// Objeto arma la cabecera. Los dos primeros hijos son SIEMPRE el fondo y el filete, y
// es lo único que `Layout` da por hecho de la lista: el resto lo coloca por los campos.
func (c *CabeceraVista) Objeto(v tema.Variante) fyne.CanvasObject {
	hijos := []fyne.CanvasObject{
		canvas.NewRectangle(tema.Color("surface", v)),
		canvas.NewRectangle(tema.Color("border", v)),
	}
	if c.Ruta != nil {
		hijos = append(hijos, c.Ruta)
	}
	hijos = append(hijos, c.Acciones...)
	if c.Orden != nil {
		hijos = append(hijos, c.Orden)
	}
	if c.Conmutador != nil {
		hijos = append(hijos, c.Conmutador)
	}
	return container.New(c, hijos...)
}

// enRejilla dice si la cabecera va en las DOS FILAS de la web por debajo de 768
// (`max-md:grid max-md:grid-cols-[minmax(0,1fr)_auto_auto]` en FileBrowser.tsx): las migas,
// el selector «Ordenar» y el conmutador de vista arriba; el buscador abajo, a todo el
// ancho (`max-md:col-span-full`). Era una fila que envolvía, y a 360 daba tres. Abajo iban
// también «Nuevo» y «Subir», que desde el 2026-11-25 son el «+» flotante.
func (c *CabeceraVista) enRejilla() bool {
	return c.Ventana > 0 && c.Ventana < ui.Estrecho && c.Ruta != nil && c.Conmutador != nil && len(c.Acciones) == 1
}

// AltoCabeceraRejilla es lo que mide en dos filas: la de arriba la estira el conmutador
// con su borde y la de abajo mide lo que el buscador. Medido en la web a 360 y a 390: 95.
const AltoCabeceraRejilla float32 = 2*RellenoYCabecera + AltoLineaCabecera + HuecoAcciones + AltoControlCromo + tema.Borde

func (c *CabeceraVista) colocarRejilla(ancho float32) {
	canalon := CanalonCabecera(c.Ventana)

	// Arriba, el conmutador en la tercera columna, pegado al canalón; el selector
	// «Ordenar» en la segunda, pegado a la derecha (`max-md:col-start-2 max-md:row-start-1
	// max-md:justify-self-end`), y las migas en lo que quede, recortadas si no caben.
	y := RellenoYCabecera
	conmutador := c.Conmutador.MinSize().Width
	x := ancho - canalon - conmutador
	centrarEnLinea(c.Conmutador, x, y, conmutador)
	if c.Orden != nil {
		w := c.Orden.MinSize().Width
		x -= HuecoAcciones + w
		centrarEnLinea(c.Orden, x, y, w)
	}
	centrarEnLinea(c.Ruta, canalon, y, min(c.Ruta.MinSize().Width, max(x-HuecoAcciones-canalon, 0)))

	// Abajo, el buscador de canalón a canalón.
	y += AltoLineaCabecera + HuecoAcciones
	centrarEnLinea(c.Acciones[0], canalon, y, max(ancho-2*canalon, 0))
}

// MinSize es el alto de las líneas que ocupa en el ancho de la última vez que se colocó
// (o en el de la ventana, antes de la primera); el ancho lo pone la pantalla.
func (c *CabeceraVista) MinSize([]fyne.CanvasObject) fyne.Size {
	ancho := c.ancho
	if ancho <= 0 {
		ancho = c.Ventana
	}
	return fyne.NewSize(0, c.AltoCon(nil, ancho))
}

// AltoCon permite que ui.Pila mida el salto del buscador antes de colocarlo,
// con el ancho útil de la hoja en vez de conservar el de la ventana anterior.
func (c *CabeceraVista) AltoCon(_ []fyne.CanvasObject, ancho float32) float32 {
	if c.enRejilla() {
		return AltoCabeceraRejilla
	}
	return AltoCabeceraEn(len(c.lineas(ancho)))
}

// AltoCabeceraEn es lo que mide la cabecera con `n` líneas: el `py-2.5`, las líneas con
// su `gap-y-2` y el `border-b`.
func AltoCabeceraEn(n int) float32 {
	n = max(n, 1)
	return 2*RellenoYCabecera + float32(n)*AltoLineaCabecera + float32(n-1)*HuecoAcciones + tema.Borde
}

// linea es una línea de la cabecera: sus hijos de izquierda a derecha, y si va pegada a
// la derecha (el `ml-auto` del grupo cuando cabe entero).
type linea struct {
	hijos   []fyne.CanvasObject
	derecha bool
}

// lineas reparte la cabecera como el `flex-wrap` de la web: todo en una línea si cabe;
// si no, la ruta arriba y el grupo de acciones debajo, partido en tantas líneas como haga
// falta. Antes no se partía y, a 360, el grupo se salía por la izquierda tapando la ruta:
// «Mi unidad» no se veía (visto en una ventana real el 2026-09-27).
func (c *CabeceraVista) lineas(ancho float32) []linea {
	grupo := append([]fyne.CanvasObject{}, c.Acciones...)
	if c.Orden != nil {
		grupo = append(grupo, c.Orden)
	}
	if c.Conmutador != nil {
		grupo = append(grupo, c.Conmutador)
	}
	hueco := func(i int) float32 { // lo que va delante del hijo i del grupo en su línea
		if i < len(grupo) && grupo[i] == c.Conmutador {
			return HuecoAcciones + MargenCromo
		}
		return HuecoAcciones
	}
	var anchoGrupo float32
	for i, o := range grupo {
		if i > 0 {
			anchoGrupo += hueco(i)
		}
		anchoGrupo += o.MinSize().Width
	}
	libre := ancho - 2*CanalonCabecera(c.Ventana)
	var ruta float32
	if c.Ruta != nil {
		ruta = c.Ruta.MinSize().Width + HuecoCabecera
	}
	if ancho <= 0 || ruta+anchoGrupo <= libre {
		return []linea{{hijos: append(soloRuta(c.Ruta), grupo...), derecha: true}}
	}
	out := []linea{}
	if c.Ruta != nil {
		out = append(out, linea{hijos: soloRuta(c.Ruta)})
	}
	if anchoGrupo <= libre {
		return append(out, linea{hijos: grupo, derecha: true})
	}
	var actual linea
	var usado float32
	for i, o := range grupo {
		w := o.MinSize().Width
		if len(actual.hijos) > 0 && usado+hueco(i)+w > libre {
			out, actual, usado = append(out, actual), linea{}, 0
		}
		if len(actual.hijos) > 0 {
			usado += hueco(i)
		}
		actual.hijos = append(actual.hijos, o)
		usado += w
	}
	return append(out, actual)
}

func soloRuta(r *ui.Migas) []fyne.CanvasObject {
	if r == nil {
		return nil
	}
	return []fyne.CanvasObject{r}
}

// Layout coloca cada línea centrada en su alto de 30: la de una sola pieza, con la ruta a
// la izquierda y las acciones pegadas a la derecha; partida, la ruta arriba y el grupo
// debajo.
func (c *CabeceraVista) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) >= 2 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
		// El `border-b` va DENTRO de la caja: `box-sizing: border-box`.
		objs[1].Move(fyne.NewPos(0, t.Height-tema.Borde))
		objs[1].Resize(fyne.NewSize(t.Width, tema.Borde))
	}
	if c.enRejilla() {
		c.ancho = t.Width
		c.colocarRejilla(t.Width)
		return
	}
	lineas := c.lineas(t.Width)
	if c.ancho != t.Width {
		antes := len(c.lineas(c.ancho))
		c.ancho = t.Width
		// El alto depende del ancho y `MinSize` no lo recibe: si cambia el número de
		// líneas, el padre tiene que volver a preguntarlo.
		if antes != len(lineas) && c.AlCambiarAlto != nil {
			go fyne.Do(c.AlCambiarAlto)
		}
	}
	for n, l := range lineas {
		arriba := RellenoYCabecera + float32(n)*(AltoLineaCabecera+HuecoAcciones)
		if l.derecha {
			c.colocarADerecha(l, arriba, t.Width)
		} else {
			c.colocarAIzquierda(l, arriba, t.Width)
		}
	}
}

// centrarEnLinea coloca un hijo con ese ancho centrado en el alto de la línea que
// empieza en y (`items-center`). Todas las filas de la cabecera miden lo mismo: la de un
// control.
func centrarEnLinea(o fyne.CanvasObject, x, y, ancho float32) {
	alto := o.MinSize().Height
	o.Move(fyne.NewPos(x, y+(AltoLineaCabecera-alto)/2))
	o.Resize(fyne.NewSize(ancho, alto))
}

// colocarAIzquierda es una línea partida: de izquierda a derecha desde el canalón.
func (c *CabeceraVista) colocarAIzquierda(l linea, y, ancho float32) {
	canalon := CanalonCabecera(c.Ventana)
	x := canalon
	for i, o := range l.hijos {
		if i > 0 {
			x += HuecoAcciones
			if o == c.Conmutador {
				x += MargenCromo
			}
		}
		w := o.MinSize().Width
		if o == fyne.CanvasObject(c.Ruta) {
			w = min(w, max(ancho-2*canalon, 0))
		}
		centrarEnLinea(o, x, y, w)
		x += w
	}
}

// colocarADerecha es la línea entera: las acciones pegadas a la derecha y la ruta con
// lo que quede.
func (c *CabeceraVista) colocarADerecha(l linea, y, ancho float32) {
	canalon := CanalonCabecera(c.Ventana)
	derecha := ancho - canalon
	for i := len(l.hijos) - 1; i >= 0; i-- {
		o := l.hijos[i]
		if o == fyne.CanvasObject(c.Ruta) {
			// `grow`: la ruta se lleva lo que quede, y se recorta si no cabe.
			resto := max(derecha-canalon+HuecoAcciones-HuecoCabecera, 0)
			centrarEnLinea(o, canalon, y, min(resto, c.Ruta.MinSize().Width))
			continue
		}
		m := o.MinSize()
		centrarEnLinea(o, derecha-m.Width, y, m.Width)
		derecha -= m.Width + HuecoAcciones
		if o == c.Conmutador {
			derecha -= MargenCromo
		}
	}
}
