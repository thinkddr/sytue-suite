// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"math"
	"net/url"
	"strconv"

	"fyne.io/fyne/v2"
	temafyne "fyne.io/fyne/v2/theme"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

/*
El VISOR: la rejilla de hojas, su zoom y cómo se navega por ella (`PdfApp.tsx`).

	<div class="relative flex-1 overflow-auto px-6 py-6">
	  <div class="mx-auto grid w-fit justify-center gap-6 grid-cols-1 items-center">

Con dos páginas por fila, `grid-cols-2 items-start`: las dos columnas miden lo que la
hoja más ancha (`1fr` en una rejilla de `w-fit`) y cada fila, lo que la más alta del par.
*/

// Lo que la web escribe en píxeles LITERALES y no en la escala: el 48 del ajuste (el
// `px-6` de los dos lados, redondeado por la web a 24 de raíz 16), el 24 entre las dos
// columnas en el ajuste de dos páginas y el 12 que `irAPagina` deja por encima de la hoja.
const (
	margenAjuste  = 48
	huecoAjuste2  = 24
	margenIrA     = 12
	zoomMinimo    = 0.15
	zoomMaximo    = 8
	pasoZoom      = 0.15
	paginasLote   = 4 // LOTE
	paginasAntes  = 2 // ANTICIPO
	pixelesMaximo = 16 << 20
)

// rellenoVisor es el `px-6 py-6` del desplazador y huecoHojas el `gap-6` de la rejilla.
var (
	rellenoVisor = tema.Espacio(6)
	huecoHojas   = tema.Espacio(6)
)

// rejillaHojas coloca las hojas en una o dos columnas, centradas, con el relleno del
// desplazador alrededor.
type rejillaHojas struct{ p *Pantalla }

// filas reparte las hojas en filas de una o de dos y devuelve, por fila, su alto; y el
// ancho de una columna.
func (r *rejillaHojas) medidas(objs []fyne.CanvasObject) (columna float32, altos []float32) {
	por := 1
	if r.p.DosPaginas {
		por = 2
	}
	for i, o := range objs {
		m := o.MinSize()
		columna = max(columna, m.Width)
		if i%por == 0 {
			altos = append(altos, 0)
		}
		altos[len(altos)-1] = max(altos[len(altos)-1], m.Height)
	}
	return columna, altos
}

func (r *rejillaHojas) MinSize(objs []fyne.CanvasObject) fyne.Size {
	columna, altos := r.medidas(objs)
	cols := float32(1)
	if r.p.DosPaginas {
		cols = 2
	}
	alto := 2 * rellenoVisor
	for i, a := range altos {
		if i > 0 {
			alto += huecoHojas
		}
		alto += a
	}
	return fyne.NewSize(2*rellenoVisor+cols*columna+(cols-1)*huecoHojas, alto)
}

func (r *rejillaHojas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	columna, altos := r.medidas(objs)
	por := 1
	if r.p.DosPaginas {
		por = 2
	}
	ancho := float32(por)*columna + float32(por-1)*huecoHojas
	// `mx-auto w-fit`: centrada si cabe; si no, pegada al relleno y se desplaza.
	x0 := max((t.Width-ancho)/2, rellenoVisor)
	y := rellenoVisor
	for i, o := range objs {
		fila, col := i/por, i%por
		if col == 0 && fila > 0 {
			y += altos[fila-1] + huecoHojas
		}
		m := o.MinSize()
		// La hoja centra su papel en la celda (`items-center` de su columna flex).
		x := x0 + float32(col)*(columna+huecoHojas) + (columna-m.Width)/2
		dy := float32(0)
		if !r.p.DosPaginas {
			dy = (altos[fila] - m.Height) / 2
		}
		o.Move(fyne.NewPos(x, y+dy))
		o.Resize(m)
	}
}

// arriba es la `offsetTop` de la hoja `n` (1-based) dentro del desplazador, o -1 si no
// hay hoja para ella (el motor manda las 500 primeras páginas y el total aparte).
func (p *Pantalla) arriba(n int) float32 {
	por := 1
	if p.DosPaginas {
		por = 2
	}
	y := rellenoVisor
	for i := 0; i < len(p.hojas); i += por {
		var alto float32
		for _, h := range p.hojas[i:min(i+por, len(p.hojas))] {
			if h.Numero() == n {
				return y
			}
			alto = max(alto, h.MinSize().Height)
		}
		y += alto + huecoHojas
	}
	return -1
}

// IrA lleva a la página `n` (1-based). Es la ÚNICA puerta —el número de la cinta, las
// flechas, las miniaturas, el índice y los enlaces internos pasan por aquí—, así que se
// acota AQUÍ: vaciar el campo o escribir «999» en un documento de doce no deja el visor
// quieto y callado.
func (p *Pantalla) IrA(n int) {
	if p.Contenido == nil {
		return
	}
	destino := min(max(n, 1), max(p.Contenido.Total, 1))
	if y := p.arriba(destino); p.desplazar != nil && y >= 0 {
		p.desplazarA(max(y-margenIrA, 0))
	}
	p.PaginaActual = destino
	p.pedirLote(destino)
	p.refrescarPagina()
}

// desplazarA lleva el documento a `y` con el desplazamiento suave de Chromium
// (`ui.DesplazarSuave`).
func (p *Pantalla) desplazarA(y float32) {
	p.animDesplazar = ui.DesplazarSuave(p.desplazar, y, p.animDesplazar)
}

// alDesplazar es `alDesplazar` de la web: qué página cae en el centro de la vista y qué
// lotes hay que ir pidiendo alrededor.
func (p *Pantalla) alDesplazar() {
	if p.desplazar == nil || p.Contenido == nil {
		return
	}
	centro := p.desplazar.Offset.Y + p.desplazar.Size().Height/2
	for _, h := range p.hojas {
		arriba := h.Position().Y
		if arriba <= centro && arriba+h.Size().Height >= centro {
			n := h.Numero()
			p.pedirLote(max(1, n-1))
			p.pedirLote(min(p.Contenido.Total, n+paginasAntes))
			if n != p.PaginaActual {
				p.PaginaActual = n
				p.refrescarPagina()
			}
			if marcaTexto(p.barra.Herramienta) {
				p.pedirTramos(n)
			}
			return
		}
	}
}

// refrescarPagina pone la página actual en la cinta y en las miniaturas sin rehacer el
// documento.
func (p *Pantalla) refrescarPagina() {
	if p.panel != nil {
		p.panel.PaginaActual = p.PaginaActual
		p.panel.RefrescarActual()
	}
	if p.barra.Pagina != p.PaginaActual {
		p.Recomponer()
	}
}

// CambiarZoom es `cambiarZoom`: ±0,15, acotado entre 0,15 y 8, y el ajuste pasa a libre.
func (p *Pantalla) CambiarZoom(delta float64) {
	p.Ajuste = AjusteLibre
	p.ponerZoom(math.Max(zoomMinimo, math.Min(zoomMaximo, p.Zoom+delta)))
	p.Recomponer()
}

// PonerAjuste elige «al ancho» o «a la página».
func (p *Pantalla) PonerAjuste(a Ajuste) {
	p.Ajuste = a
	p.relayout()
}

// relayout recalcula el ajuste y recoloca, sin rehacer las hojas.
func (p *Pantalla) relayout() {
	if p.desplazar != nil {
		p.alMedir(p.desplazar.Size())
		p.rejilla.Refresh()
	}
	p.Recomponer()
}

// alMedir es el `ResizeObserver` del zoom: con ajuste al ancho o a la página, el zoom
// sale del hueco que hay. Lo llama el layout del visor con el tamaño del desplazador.
func (p *Pantalla) alMedir(t fyne.Size) {
	if p.Contenido == nil || len(p.Contenido.Paginas) == 0 || p.Ajuste == AjusteLibre || t.Width <= 0 {
		return
	}
	pg := p.Contenido.Paginas[0]
	disponible := float64(t.Width) - margenAjuste
	util := disponible
	if p.DosPaginas {
		util = (disponible - huecoAjuste2) / 2
	}
	porAncho := util / pg.Ancho
	z := porAncho
	if p.Ajuste == AjustePagina {
		z = math.Min(porAncho, (float64(t.Height)-margenAjuste)/pg.Alto)
	}
	if z > 0 && z != p.Zoom {
		p.ponerZoom(z)
	}
}

// ponerZoom cambia el zoom de las hojas y, si la escala de dibujo se queda corta o
// sobra mucho, vuelve a pedir la vista.
func (p *Pantalla) ponerZoom(z float64) {
	p.Zoom = z
	for _, h := range p.hojas {
		h.PonerZoom(z)
	}
	if p.barra.porciento != nil {
		p.barra.porciento.Texto = strconv.Itoa(int(math.Round(z*100))) + "%"
		p.barra.porciento.Refresh()
	}
	// ponytail: se vuelve a pedir solo si la escala cambia más de un 10 %: al arrastrar
	// el borde de la ventana el ajuste cambia el zoom en cada píxel, y pedir una tanda
	// por píxel es tirar trabajo. Mientras tanto, Catmull-Rom sobre lo que hay.
	if e := p.escalaDeDibujo(); p.escala > 0 && math.Abs(e-p.escala)/p.escala > 0.1 {
		p.imagenes, p.fallidas, p.pidiendo = map[int]image.Image{}, map[int]bool{}, map[int]bool{}
		p.escala = 0
		p.pedirLote(max(1, p.PaginaActual-1))
		p.pedirLote(min(p.Contenido.Total, p.PaginaActual+paginasAntes))
	}
}

// escalaDeDibujo es a cuántos píxeles por punto se pide la página: el zoom por la escala
// del lienzo, para que el dibujo llegue ya a píxel de dispositivo. Se acota para que un A4
// quepa en el tope del motor (16 Mpx).
func (p *Pantalla) escalaDeDibujo() float64 {
	escala := float64(1)
	if c := fyne.CurrentApp().Driver().CanvasForObject(p.raiz); c != nil {
		escala = float64(c.Scale())
	}
	e := p.Zoom * escala
	if p.Contenido != nil && len(p.Contenido.Paginas) > 0 {
		pg := p.Contenido.Paginas[0]
		if tope := math.Sqrt(pixelesMaximo * 0.95 / (pg.Ancho * pg.Alto)); e > tope {
			e = tope
		}
	}
	return math.Max(math.Min(e, 16), 0.1)
}

// temaVisor es el de la suite sin la sombra con que `container.Scroll` avisa de que hay
// más contenido: la web no la pinta. Los widgets propios no leen el tema de Fyne, así que
// esto solo apaga esa sombra.
type temaVisor struct{ tema.Sytue }

// Color devuelve transparente para la sombra y lo de la suite para todo lo demás.
func (t temaVisor) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if n == temafyne.ColorNameShadow {
		return color.Transparent
	}
	return t.Sytue.Color(n, v)
}

// abrirFuera abre una dirección externa en el navegador del sistema.
//
// Un `mailto:` de un PDF abre el correo con su destinatario y nada más: el asunto, el
// cuerpo y lo que traiga detrás lo ha escrito quien hizo el documento.
func abrirFuera(dir string) {
	if u, err := url.Parse(dir); err == nil && u.Scheme == "mailto" {
		para, _ := url.PathUnescape(u.Opaque)
		_ = api.AbrirCorreo(fyne.CurrentApp(), para, "", "")
		return
	}
	_ = api.AbrirFuera(fyne.CurrentApp(), dir)
}
