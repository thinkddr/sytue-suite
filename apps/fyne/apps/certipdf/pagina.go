// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	xdraw "golang.org/x/image/draw"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

/*
La HOJA: una página del documento con su capa de marcas encima.

# La página se dibuja RASTERIZADA, y por qué

El motor entrega cada página como SVG con `<text>` y las fuentes incrustadas. En el
navegador ese SVG entra en el DOM y se ve entero; en Fyne, todo SVG pasa por `oksvg`,
que dibuja formas y NO implementa `<text>` (medido en `TestElSVGDeUnaPaginaPierdeSuTexto`:
un `<rect>` pinta 10 000 píxeles y un `<text>`, 0). Una factura saldría con sus líneas y
SIN UNA CIFRA, con aspecto de estar bien.

Así que se le pide al motor la página YA rasterizada (`op: render` con `formato=png`,
#831) a la escala de la pantalla —el zoom por la escala del lienzo—, y se pinta con un
`canvas.Raster` que la entrega píxel a píxel. NUNCA con un `canvas.Image`: ese estira el
mapa de bits con el bilineal aproximado de Fyne, que es el que en SyPhotos midió 73,89
niveles de aliasing (`apps/photos/fototeca.go`). Cuando la escala pedida y la de la
pantalla no coinciden —justo después de cambiar el zoom, mientras llega la buena— se
remuestrea con Catmull-Rom, que es lo que allí salió bien.

Mientras no llega, la hoja dice `pdf.page.loading`; si el motor no supo dibujarla,
`pdf.page.failed`, repartido en líneas. Una página que no se sabe dibujar se DICE.

# Geometría, de la web

`PdfApp.tsx`, componente `Hoja`:

	<div class="flex flex-col items-center gap-1.5" data-pagina={n}>
	  <div class="border-border relative border bg-white shadow-sm" style={{width, height}}>
	  <span class="text-text-subtle text-xs tabular-nums">{n}</span>

`bg-white` es blanco literal y no `bg-surface`: el papel es blanco también en tema
oscuro, que es lo que hace cualquier visor. El borde es `border` (1 px) y va con
[ui.Caja]; el dibujo va DENTRO del borde (`absolute inset-0` de un hijo), como en la web.

# El `shadow-sm` del papel

Asoma seis píxeles fuera de la hoja ([sombraPapel]) y la hoja mide exactamente el papel:
cabe en el `gap-6` y el `py-6` de la rejilla, y Fyne no recorta lo que un widget pinta
fuera de su caja.
*/

// EstadoHoja es en cuál de sus casos está una página.
type EstadoHoja uint8

// Los tres de la web: el dibujo sin pedir, el que llegó vacío y el dibujado.
const (
	// HojaCargando es el lote todavía en vuelo (`pdf.page.loading`).
	HojaCargando EstadoHoja = iota
	// HojaSinDibujar es «no sabemos pintar esto» (`pdf.page.failed`).
	HojaSinDibujar
	// HojaDibujada es la página ya rasterizada.
	HojaDibujada
)

// Separación entre el papel y su número, `gap-1.5` de la web.
var sepNumero = tema.Espacio(1.5)

// ColorPapel es el blanco del papel, y es `bg-white` LITERAL y no un token a propósito:
// con `bg-surface`, en tema oscuro el papel saldría gris carbón y un documento —tinta
// negra sobre blanco— quedaría ilegible.
var ColorPapel = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

// MedidaHoja es lo que ocupa una hoja entera: el papel al zoom pedido, más el hueco y
// la línea del número debajo. Es la ÚNICA cuenta (regla 2 de `ui.go`).
func MedidaHoja(p Pagina, zoom float64) fyne.Size {
	ancho, alto := MedidaPapel(p, zoom)
	return fyne.NewSize(ancho, alto+sepNumero+tema.TextoXS.Linea)
}

// MedidaPapel es solo el papel, sin el número. Los decimales se escriben tal cual y no
// se redondean: a ×2 salen exactos (regla 3 de `ui.go`).
func MedidaPapel(p Pagina, zoom float64) (ancho, alto float32) {
	return float32(p.Ancho * zoom), float32(p.Alto * zoom)
}

// Marca es algo dibujado encima del papel. Las tres clases tienen aspecto distinto
// porque significan cosas distintas: una zona de censura es lo único de la pantalla que
// DESTRUYE contenido.
type Marca struct {
	Rect  Rect
	Clase ClaseMarca
	// Titulo es lo que anuncia un lector de pantalla y lo que sale en la pista.
	Titulo string
}

// ClaseMarca es qué clase de marca es.
type ClaseMarca uint8

// Las tres clases que la web pinta sobre la hoja.
const (
	// MarcaAnotacion es una anotación que YA está en el documento.
	MarcaAnotacion ClaseMarca = iota
	// MarcaCensura es una zona marcada para censurar, tal y como quedará.
	MarcaCensura
	// MarcaCampo es un campo de formulario marcado y aún sin crear.
	MarcaCampo
)

// Hoja es una página dibujada con sus marcas encima.
type Hoja struct {
	widget.BaseWidget

	pagina Pagina
	zoom   float64
	estado EstadoHoja
	marcas []Marca
	// aviso es el texto ya traducido del estado (el cromo traduce, la hoja pinta).
	aviso string

	// enlaces son las zonas pulsables del documento, ya montadas.
	enlaces []fyne.CanvasObject

	// AlGesto recibe lo que el puntero hace con una herramienta: en cada movimiento
	// (`fin` falso) y al soltar. Ver `gestos.go`.
	AlGesto func(h *Hoja, g Gesto, fin bool)
	// seleccion es el texto que se está seleccionando, en cajas del SVG.
	seleccion []CajaSVG
	r         *hojaRenderer

	mu     sync.Mutex
	imagen image.Image
	// hecha es la última entrega del raster, para no remuestrear en cada fotograma.
	hecha image.Image
	// La herramienta de la cinta, con su tinta y su grosor, y el gesto en curso. Van
	// bajo el cerrojo porque los lee el generador de la previa al pintar.
	herramienta Herramienta
	tinta       color.NRGBA
	grosor      float64
	gesto       *Gesto
}

var _ fyne.Widget = (*Hoja)(nil)

// NuevaHoja monta una página. `aviso` es el rótulo YA TRADUCIDO que se pinta en el papel
// mientras no hay dibujo.
func NuevaHoja(p Pagina, zoom float64, estado EstadoHoja, aviso string, marcas []Marca) *Hoja {
	h := &Hoja{pagina: p, zoom: zoom, estado: estado, aviso: aviso, marcas: marcas}
	h.ExtendBaseWidget(h)
	return h
}

// Numero es el de la página (1-based).
func (h *Hoja) Numero() int { return h.pagina.Numero }

// PonerImagen entrega el dibujo de la página. Nil con `fallida` es que el motor no supo.
func (h *Hoja) PonerImagen(img image.Image, fallida, rotulo string) {
	h.mu.Lock()
	h.imagen, h.hecha = img, nil
	h.mu.Unlock()
	switch {
	case img != nil:
		h.estado = HojaDibujada
	case fallida != "":
		h.estado, h.aviso = HojaSinDibujar, fallida
	default:
		h.estado, h.aviso = HojaCargando, rotulo
	}
	h.Refresh()
}

// PonerZoom cambia el tamaño de la hoja sin rehacerla.
func (h *Hoja) PonerZoom(z float64) {
	if h.zoom == z {
		return
	}
	h.zoom = z
	h.mu.Lock()
	h.hecha = nil
	h.mu.Unlock()
	h.Refresh()
}

// PonerMarcas cambia lo dibujado encima del papel.
func (h *Hoja) PonerMarcas(m []Marca) {
	h.marcas = m
	h.Refresh()
}

// PonerEnlaces cambia las zonas pulsables. Solo existen con la mano: con una
// herramienta de dibujo, un enlace bajo el trazo se lo comería.
func (h *Hoja) PonerEnlaces(e []fyne.CanvasObject) {
	h.enlaces = e
	h.Refresh()
}

// AccessibilityLabel anuncia de qué página se trata.
func (h *Hoja) AccessibilityLabel() string {
	if h.estado == HojaDibujada {
		return numeroDePagina(h.pagina.Numero)
	}
	return h.aviso
}

// AccessibilityRole la anuncia como contenedor: es contenido, no un control.
func (h *Hoja) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MinSize es [MedidaHoja].
func (h *Hoja) MinSize() fyne.Size {
	h.ExtendBaseWidget(h)
	return MedidaHoja(h.pagina, h.zoom)
}

// CreateRenderer compone papel, dibujo, aviso, marcas, enlaces y número.
func (h *Hoja) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &hojaRenderer{
		h:      h,
		papel:  ui.NuevaCaja(ColorPapel, tema.Color("border", v), 0),
		numero: nuevoTabular(numeroDePagina(h.pagina.Numero), tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)),
		tapa:   nuevaTapa(h),
		sombra: nuevaSombraPapel(),
	}
	r.dibujo = canvas.NewRaster(h.generar)
	r.dibujo.ScaleMode = canvas.ImageScalePixels
	r.previa = canvas.NewRaster(h.generarPrevia)
	r.previa.ScaleMode = canvas.ImageScalePixels
	h.r = r
	r.rehacerAviso(v)
	r.rehacerMarcas(v)
	r.Refresh()
	return r
}

// generar entrega el dibujo a los píxeles de dispositivo que pide el lienzo.
func (h *Hoja) generar(w, alto int) image.Image {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.imagen == nil || w <= 0 || alto <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, max(w, 1), max(alto, 1)))
	}
	if h.hecha != nil && h.hecha.Bounds().Dx() == w && h.hecha.Bounds().Dy() == alto {
		return h.hecha
	}
	ancho, altoP := MedidaPapel(h.pagina, h.zoom)
	h.hecha = RecortarPagina(h.imagen, float64(ancho), float64(altoP), w, alto)
	return h.hecha
}

// RecortarPagina saca, del dibujo de la página ENTERA, lo que va dentro del borde: una
// caja de `w`×`h` píxeles de dispositivo en un papel de `anchoPapel`×`altoPapel` px.
//
// Si el dibujo está a la escala de la pantalla —lo normal: se pide así—, es un RECORTE
// exacto de un píxel de borde por lado, sin remuestrear: nítido. La web encoge el dibujo
// entero para que quepa dentro del borde (`inset-0`); aquí se tapa ese píxel, que en un
// PDF es margen. Medido: la diferencia de posición es de 1 px en los cantos y 0 en el
// centro, a cambio de no emborronar el texto con un remuestreo de 1127 a 1126.
//
// Si no está a la escala —justo después de un zoom—, se remuestrea con Catmull-Rom hasta
// que llegue el dibujo bueno.
func RecortarPagina(src image.Image, anchoPapel, altoPapel float64, w, h int) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	interior := anchoPapel - 2*float64(tema.Borde)
	if interior <= 0 || altoPapel <= 0 {
		return dst
	}
	escala := float64(w) / interior              // píxeles de dispositivo por px
	k := float64(b.Dx()) / (anchoPapel * escala) // píxeles del dibujo por píxel de dispositivo
	borde := float64(tema.Borde) * escala * k    // el borde, en píxeles del dibujo
	origen := image.Rect(
		b.Min.X+int(math.Round(borde)), b.Min.Y+int(math.Round(borde)),
		b.Min.X+int(math.Round(borde+float64(w)*k)), b.Min.Y+int(math.Round(borde+float64(h)*k)),
	).Intersect(b)
	if origen.Dx() == w && origen.Dy() == h {
		xdraw.Copy(dst, image.Point{}, src, origen, xdraw.Src, nil)
		return dst
	}
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, origen, xdraw.Src, nil)
	return dst
}

type hojaRenderer struct {
	h      *Hoja
	papel  *ui.Caja
	dibujo *canvas.Raster
	aviso  []*canvas.Text
	numero *tabular
	marcas []fyne.CanvasObject
	sombra *sombraPapel
	// tapa recoge el puntero con una herramienta; previa enseña lo que se arrastra y
	// sel, lo que se está seleccionando.
	tapa   *tapaHoja
	previa *canvas.Raster
	sel    []fyne.CanvasObject
}

// MargenAviso es el `px-6` del aviso: lo que se le quita al papel por cada lado antes
// de repartir el texto en líneas.
var MargenAviso = tema.Espacio(6)

// AnchoDelAviso es lo que le queda al aviso dentro del papel: el papel menos su borde
// y menos el `px-6` de los dos lados. Nunca negativo.
func AnchoDelAviso(anchoPapel float32) float32 {
	return max(anchoPapel-2*tema.Borde-2*MargenAviso, 0)
}

// rehacerAviso reparte el aviso en las líneas que caben en el papel. Con la página
// dibujada no hay aviso.
func (r *hojaRenderer) rehacerAviso(v tema.Variante) {
	r.aviso = r.aviso[:0]
	if r.h.estado == HojaDibujada {
		return
	}
	ancho, _ := MedidaPapel(r.h.pagina, r.h.zoom)
	cabe := AnchoDelAviso(ancho)
	if r.h.estado == HojaCargando {
		// `pdf.page.loading` va sin `px-6` en la web: solo lo lleva el de «no se pudo».
		cabe = max(ancho-2*tema.Borde, 0)
	}
	for _, l := range ui.Repartir(r.h.aviso, tema.TextoXS, ui.Normal, cabe) {
		r.aviso = append(r.aviso, ui.Texto(l, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v)))
	}
}

func (r *hojaRenderer) MinSize() fyne.Size { return r.h.MinSize() }

func (r *hojaRenderer) Objects() []fyne.CanvasObject {
	objs := append(r.sombra.Objetos(), r.papel.Objetos()...)
	if r.h.estado == HojaDibujada {
		objs = append(objs, r.dibujo)
	}
	for _, l := range r.aviso {
		objs = append(objs, l)
	}
	hm := r.h.Herramienta()
	// Con las de texto, la tapa va DEBAJO de los enlaces: en la web el enlace es un
	// `<a>` encima del texto y se sigue pudiendo pulsar. Con las de dibujo no hay
	// enlaces: uno bajo el trazo se lo comería.
	if seleccionaTexto(hm) {
		objs = append(objs, r.tapa)
	}
	if !capaInteractiva(hm) {
		objs = append(objs, r.h.enlaces...)
	}
	objs = append(objs, r.marcas...)
	objs = append(objs, r.sel...)
	objs = append(objs, r.previa)
	if capaInteractiva(hm) {
		objs = append(objs, r.tapa)
	}
	return append(objs, r.numero.Objetos()...)
}

func (r *hojaRenderer) Destroy() {}

func (r *hojaRenderer) Layout(_ fyne.Size) {
	h := r.h
	ancho, alto := MedidaPapel(h.pagina, h.zoom)
	r.sombra.Colocar(ancho, alto, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.papel.Redimensionar(fyne.NewSize(ancho, alto))
	r.dibujo.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.dibujo.Resize(fyne.NewSize(max(ancho-2*tema.Borde, 0), max(alto-2*tema.Borde, 0)))

	// El aviso: el BLOQUE centrado en el papel y cada línea centrada dentro de él.
	y := (alto - float32(len(r.aviso))*tema.TextoXS.Linea) / 2
	for i, l := range r.aviso {
		m := ui.Medir(l.Text, tema.TextoXS, ui.Normal)
		ui.ColocarTexto(l, tema.TextoXS, (ancho-m.Width)/2, y+float32(i)*tema.TextoXS.Linea)
	}

	// Enlaces y marcas, en coordenadas de SVG multiplicadas por el zoom. Se construyen
	// en coordenadas LÓGICAS y se multiplican AL COLOCAR (medido por Diapos).
	colocar := func(o fyne.CanvasObject, rect Rect) {
		c := RectEnSVG(h.pagina, rect)
		z := h.zoom
		o.Move(fyne.NewPos(float32(c.X*z), float32(c.Y*z)))
		o.Resize(fyne.NewSize(float32(c.Ancho*z), float32(c.Alto*z)))
	}
	for _, e := range h.enlaces {
		if en, ok := e.(*enlaceHoja); ok {
			colocar(en, en.e.Rect)
		}
	}
	for i, mk := range h.marcas {
		if i < len(r.marcas) {
			colocar(r.marcas[i], mk.Rect)
			if mk.Clase == MarcaAnotacion {
				// El trazo va centrado en el borde: el raster lleva medio grosor más por
				// cada lado.
				m := float32(grosorAnotacion / 2 * h.zoom)
				o := r.marcas[i]
				o.Move(o.Position().SubtractXY(m, m))
				o.Resize(o.Size().AddWidthHeight(2*m, 2*m))
			}
		}
	}
	for i, c := range h.seleccion {
		if i < len(r.sel) {
			z := h.zoom
			r.sel[i].Move(fyne.NewPos(float32(c.X*z), float32(c.Y*z)))
			r.sel[i].Resize(fyne.NewSize(float32(c.Ancho*z), float32(c.Alto*z)))
		}
	}
	r.tapa.Move(fyne.NewPos(0, 0))
	r.tapa.Resize(fyne.NewSize(ancho, alto))
	r.colocarPrevia()

	r.numero.Colocar((ancho-r.numero.Ancho())/2, alto+sepNumero)
}

func (r *hojaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.papel.Pintar(ColorPapel, tema.Color("border", v))
	r.rehacerAviso(v)
	r.numero.Tenir(tema.Color("text-subtle", v))
	r.rehacerMarcas(v)
	r.rehacerSeleccion()
	r.Layout(r.h.Size())
	r.dibujo.Refresh()
	r.previa.Refresh()
	canvas.Refresh(r.h)
}

// rehacerMarcas construye los rectángulos de las marcas con el aspecto de su clase.
func (r *hojaRenderer) rehacerMarcas(v tema.Variante) {
	r.marcas = r.marcas[:0]
	for _, m := range r.h.marcas {
		r.marcas = append(r.marcas, rectanguloDeMarca(m, r.h.pagina, r.h.zoom, v))
	}
}

// rehacerSeleccion pone un recuadro por trozo seleccionado, del color del `::selection`
// de la suite: `color-mix(in srgb, var(--sytue-focus-ring) 26%, transparent)`.
func (r *hojaRenderer) rehacerSeleccion() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a := tema.Color("focus-ring", v)
	r.sel = r.sel[:0]
	for range r.h.seleccion {
		r.sel = append(r.sel, canvas.NewRectangle(color.NRGBA{R: a.R, G: a.G, B: a.B, A: 66}))
	}
}

// colocarPrevia pone el raster de la previa sobre lo que ocupa el gesto.
func (r *hojaRenderer) colocarPrevia() {
	h := r.h
	h.mu.Lock()
	g, hm, grosor := h.gesto, h.herramienta, h.grosor
	var c Gesto
	if g != nil {
		c = *g
	}
	h.mu.Unlock()
	if g == nil || seleccionaTexto(hm) {
		r.previa.Hide()
		return
	}
	x, y, w, alto := cajaPrevia(c, hm, grosor)
	z := h.zoom
	r.previa.Move(fyne.NewPos(float32(x*z), float32(y*z)))
	r.previa.Resize(fyne.NewSize(float32(w*z), float32(alto*z)))
	r.previa.Show()
}

// El grosor, el discontinuo y la opacidad del contorno de una anotación ya puesta, en
// puntos: `stroke-width={0.6} stroke-dasharray="3 2" opacity={0.45}`.
const grosorAnotacion = 0.6

var discontinuoAnotacion = []float64{3, 2}

// rectanguloDeMarca es el aspecto de cada clase, copiado de la capa SVG de `PdfApp.tsx`.
// Los grosores son de la `<svg>` con `viewBox` en puntos: escalan con el zoom.
func rectanguloDeMarca(m Marca, pag Pagina, zoom float64, v tema.Variante) fyne.CanvasObject {
	switch m.Clase {
	case MarcaCensura:
		// `fill="#111111"` en la web: negro literal y opaco, el color que tendrá la barra.
		q := canvas.NewRectangle(colorCensura)
		q.StrokeColor = tema.Color("danger-text", v)
		q.StrokeWidth = float32(zoom)
		return q
	case MarcaCampo:
		// `fill-opacity={0.14}` sobre el acento, con su contorno a plena fuerza.
		a := tema.Color("accent", v)
		q := canvas.NewRectangle(color.NRGBA{R: a.R, G: a.G, B: a.B, A: 36})
		q.StrokeColor = a
		q.StrokeWidth = float32(zoom)
		return q
	case MarcaAnotacion:
	}
	// Anotación: sin relleno, contorno de acento al 45 % y DISCONTINUO, que `canvas.
	// Rectangle` no sabe hacer: va con el trazador de `trazos.go`.
	a := tema.Color("accent", v)
	tinta := color.NRGBA{R: a.R, G: a.G, B: a.B, A: 115}
	c := RectEnSVG(pag, m.Rect)
	return canvas.NewRaster(func(w, h int) image.Image {
		m := grosorAnotacion / 2
		p := nuevoPincel(w, h, float64(w)/(c.Ancho+2*m), -m, -m)
		p.caja(0, 0, c.Ancho, c.Alto, nil, tinta, grosorAnotacion, discontinuoAnotacion)
		return p.img
	})
}

// numeroDePagina es el rótulo de debajo del papel y de la miniatura; va con
// `tabular-nums` ([tabular]): las cifras de serie de Figtree son proporcionales.
func numeroDePagina(n int) string { return strconv.Itoa(n) }

// enlaceHoja es una zona pulsable del documento: un salto interno o una dirección.
// `absolute rounded-[2px] hover:bg-accent/10 focus-visible:bg-accent/10`, y enfocable:
// en la web son un `<a>` y un `<button>` de verdad para que el tabulador los alcance.
type enlaceHoja struct {
	ui.Pulsable
	e Enlace
}

func nuevoEnlaceHoja(e Enlace, nombre string, alPulsar func()) *enlaceHoja {
	x := &enlaceHoja{e: e}
	x.Nombre, x.AlPulsar = nombre, alPulsar
	x.ExtendBaseWidget(x)
	return x
}

func (x *enlaceHoja) CreateRenderer() fyne.WidgetRenderer {
	f := canvas.NewRectangle(color.Transparent)
	f.CornerRadius = 2
	r := &enlaceHojaRenderer{x: x, fondo: f}
	r.Refresh()
	return r
}

type enlaceHojaRenderer struct {
	x     *enlaceHoja
	fondo *canvas.Rectangle
}

func (r *enlaceHojaRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, 0) }
func (r *enlaceHojaRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.fondo} }
func (r *enlaceHojaRenderer) Destroy()                     {}
func (r *enlaceHojaRenderer) Layout(t fyne.Size)           { r.fondo.Resize(t) }

func (r *enlaceHojaRenderer) Refresh() {
	r.fondo.FillColor = color.Transparent
	if r.x.Encima() || r.x.Enfocado() {
		a := tema.Color("accent", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
		r.fondo.FillColor = color.NRGBA{R: a.R, G: a.G, B: a.B, A: 26}
	}
	r.fondo.Refresh()
}
