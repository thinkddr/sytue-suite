// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

/*
DÓNDE CAE LA FIRMA VISIBLE (`firmaPosicion.ts` y `PosicionDeFirma` de `Paneles.tsx`).

El motor acepta cualquier recuadro en cualquier página; lo que faltaba era dejar
elegirlo. Las mismas cuatro posiciones que la web —abajo a la izquierda, la de siempre y
la de por defecto; abajo en el centro y a la derecha, como en los contratos; y arriba a
la derecha, donde se sella un registro de entrada— con el recuadro calculado sobre la
MediaBox REAL de la página: con uno fijo, en una apaisada o en un tique estrecho la firma
se salía del papel.

Todo en coordenadas del PDF (origen abajo-izquierda, sin /Rotate), que es lo que espera
el motor, y respecto a la MediaBox: si no empieza en 0,0, un recuadro fijo caía fuera.
*/

// PosicionFirma es una de las cuatro esquinas que se ofrecen.
type PosicionFirma string

// Las posiciones, con el mismo nombre que en la web: es también el sufijo de su clave
// `pdf.dialog.firmar.posicion.*`.
const (
	FirmaAbajoIzquierda PosicionFirma = "abajoIzquierda"
	FirmaAbajoCentro    PosicionFirma = "abajoCentro"
	FirmaAbajoDerecha   PosicionFirma = "abajoDerecha"
	FirmaArribaDerecha  PosicionFirma = "arribaDerecha"
	// FirmaDibujada es el recuadro DIBUJADO sobre la página (`'dibujada'` de la web),
	// para cuando la firma va en una línea concreta que ninguna esquina acierta. No es
	// una de [posicionesFirma]: se ofrece aparte, a lo ancho.
	FirmaDibujada PosicionFirma = "dibujada"
)

// posicionesFirma van en el orden en que se ofrecen. La primera es la de por defecto.
var posicionesFirma = []PosicionFirma{FirmaAbajoIzquierda, FirmaAbajoCentro, FirmaAbajoDerecha, FirmaArribaDerecha}

const (
	// margenFirma es la distancia al borde del papel, en puntos (unos 2 cm).
	margenFirma = 56.0
	// anchoFirma y altoFirma son el recuadro de siempre, 240 × 74 pt.
	anchoFirma, altoFirma = 240.0, 74.0
)

// cajaA4 es lo que se supone cuando aún no se sabe cómo es la página.
var cajaA4 = Rect{0, 0, 595, 842}

// posicionFirma es el recuadro de la firma en una página de MediaBox `caja`.
//
// Si no cabe con el margen entero —un tique, una etiqueta—, el margen se encoge y el
// recuadro se ESCALA sin deformarse: la rúbrica se dibuja estirada al recuadro, y
// cambiarle la proporción la aplastaría.
func posicionFirma(caja Rect, p PosicionFirma) Rect {
	cx0, cy0, cx1, cy1 := caja[0], caja[1], caja[2], caja[3]
	anchoPagina, altoPagina := math.Max(0, cx1-cx0), math.Max(0, cy1-cy0)
	// En cualquier hoja donde cabe (A4, carta, A5, apaisadas) el margen es el de siempre;
	// solo en una diminuta baja al 10 % del lado más corto, para dejar papel a la firma.
	margen := margenFirma
	if anchoPagina < anchoFirma+2*margenFirma || altoPagina < altoFirma+2*margenFirma {
		margen = math.Min(margenFirma, math.Min(anchoPagina, altoPagina)*0.1)
	}
	escala := math.Max(0, math.Min(1, math.Min((anchoPagina-2*margen)/anchoFirma, (altoPagina-2*margen)/altoFirma)))
	w, h := anchoFirma*escala, altoFirma*escala

	var x0 float64
	switch p {
	case FirmaAbajoCentro:
		x0 = cx0 + (anchoPagina-w)/2
	case FirmaAbajoDerecha, FirmaArribaDerecha:
		x0 = cx1 - margen - w
	default:
		x0 = cx0 + margen
	}
	y0 := cy0 + margen
	if p == FirmaArribaDerecha {
		y0 = cy1 - margen - h
	}
	return Rect{redondearFirma(x0), redondearFirma(y0), redondearFirma(x0 + w), redondearFirma(y0 + h)}
}

// redondearFirma deja medio punto: más decimales solo ensucian el PDF. Es el
// `Math.round(n * 2) / 2` de la web, que redondea los medios hacia arriba también en
// negativo (una MediaBox puede empezar por debajo de cero); `math.Round` no.
func redondearFirma(n float64) float64 { return math.Floor(n*2+0.5) / 2 }

// cajaComun es la caja que tienen TODAS las páginas: «firmar en todas» pone el mismo
// recuadro en cada una, así que se calcula sobre la zona común para que no se salga de
// la más pequeña de un documento con hojas de varios tamaños.
func cajaComun(paginas []Pagina) (Rect, bool) {
	if len(paginas) == 0 {
		return Rect{}, false
	}
	r := paginas[0].Caja
	for _, p := range paginas[1:] {
		r = Rect{math.Max(r[0], p.Caja[0]), math.Max(r[1], p.Caja[1]), math.Min(r[2], p.Caja[2]), math.Min(r[3], p.Caja[3])}
	}
	return r, true
}

// paginaParaFirmar es la página elegida (1-based) o, si no existe, la primera: el motor
// la rechazaría con un error más claro que un recuadro raro.
func paginaParaFirmar(paginas []Pagina, pagina int) (Pagina, bool) {
	if pagina >= 1 && pagina <= len(paginas) {
		return paginas[pagina-1], true
	}
	if len(paginas) > 0 {
		return paginas[0], true
	}
	return Pagina{}, false
}

// cajaParaFirmar es la caja sobre la que se coloca la firma: la de la página elegida o,
// si se firma en todas, la común. Sin páginas, la de un A4, como la web.
func cajaParaFirmar(paginas []Pagina, pagina int, todas bool) Rect {
	if todas {
		if c, ok := cajaComun(paginas); ok {
			return c
		}
		return cajaA4
	}
	if p, ok := paginaParaFirmar(paginas, pagina); ok {
		return p.Caja
	}
	return cajaA4
}

// ---- El recuadro dibujado (`firmaPosicion.ts`) ----

// proporcionFirma es la del recuadro de siempre: la rúbrica se estira a él y otra la
// deformaría.
const proporcionFirma = anchoFirma / altoFirma

// anchoMinimoFirma es lo más pequeño que se deja dibujar, en puntos.
const anchoMinimoFirma = 48.0

// aFracciones pasa un recuadro a fracciones (0-1) de la caja. Así se guarda el
// dibujado: si cambia la página o se marca «firmar en todas», sigue en el mismo sitio
// de la hoja en vez de quedarse fuera.
func aFracciones(caja, r Rect) Rect {
	w, h := math.Max(1e-9, caja[2]-caja[0]), math.Max(1e-9, caja[3]-caja[1])
	return Rect{(r[0] - caja[0]) / w, (r[1] - caja[1]) / h, (r[2] - caja[0]) / w, (r[3] - caja[1]) / h}
}

// deFracciones es el recuadro en puntos sobre `caja`: la esquina de abajo a la izquierda
// y el ancho relativos, y el alto de la proporción de la firma.
func deFracciones(caja, f Rect) Rect {
	w := caja[2] - caja[0]
	ancho := (f[2] - f[0]) * w
	x0, y0 := caja[0]+f[0]*w, caja[1]+f[1]*(caja[3]-caja[1])
	return encajarFirma(caja, Rect{x0, y0, x0 + ancho, y0 + ancho/proporcionFirma})
}

// recuadroArrastrado es el que sale de arrastrar de `a` a `b` (puntos del PDF): manda el
// lado que más se ha estirado, con un mínimo para que un toque suelto deje uno usable.
func recuadroArrastrado(caja Rect, a, b [2]float64) Rect {
	dx, dy := b[0]-a[0], b[1]-a[1]
	w := math.Max(math.Max(math.Abs(dx), math.Abs(dy)*proporcionFirma), anchoMinimoFirma)
	h := w / proporcionFirma
	x0, y0 := a[0], a[1]
	if dx < 0 {
		x0 -= w
	}
	if dy < 0 {
		y0 -= h
	}
	return encajarFirma(caja, Rect{x0, y0, x0 + w, y0 + h})
}

// moverRecuadro lo desplaza sin dejarlo salir del papel.
func moverRecuadro(caja, r Rect, dx, dy float64) Rect {
	return encajarFirma(caja, Rect{r[0] + dx, r[1] + dy, r[2] + dx, r[3] + dy})
}

// escalarRecuadro lo agranda o encoge por su centro, sin bajar del mínimo.
func escalarRecuadro(caja, r Rect, factor float64) Rect {
	w := math.Max(anchoMinimoFirma, (r[2]-r[0])*factor)
	h := w / proporcionFirma
	cx, cy := (r[0]+r[2])/2, (r[1]+r[3])/2
	return encajarFirma(caja, Rect{cx - w/2, cy - h/2, cx + w/2, cy + h/2})
}

// encajarFirma mete el recuadro en la caja: si es más grande que la hoja lo encoge sin
// deformarlo, y si asoma por un lado lo empuja hacia dentro.
func encajarFirma(caja, r Rect) Rect {
	anchoCaja, altoCaja := math.Max(0, caja[2]-caja[0]), math.Max(0, caja[3]-caja[1])
	w, h := math.Abs(r[2]-r[0]), math.Abs(r[3]-r[1])
	escala := 1.0
	if w > 0 {
		escala = math.Min(escala, anchoCaja/w)
	}
	if h > 0 {
		escala = math.Min(escala, altoCaja/h)
	}
	w, h = w*escala, h*escala
	x0 := math.Min(math.Max(math.Min(r[0], r[2]), caja[0]), caja[2]-w)
	y0 := math.Min(math.Max(math.Min(r[1], r[3]), caja[1]), caja[3]-h)
	return Rect{redondearFirma(x0), redondearFirma(y0), redondearFirma(x0 + w), redondearFirma(y0 + h)}
}

// rectElegido es el recuadro de lo elegido: el de la posición o el dibujado; uno
// dibujado que aún no existe sale donde siempre, que es de donde se empieza a moverlo.
func rectElegido(p PosicionFirma, caja Rect, dibujo *Rect) Rect {
	if p != FirmaDibujada {
		return posicionFirma(caja, p)
	}
	if dibujo != nil {
		return deFracciones(caja, *dibujo)
	}
	return posicionFirma(caja, posicionesFirma[0])
}

// ---- El selector ----

// Medidas de `PosicionDeFirma`: la miniatura es `w-20`, las opciones `min-h-8` (y
// `max(44px,2.75rem)` en el teléfono o con el dedo), la rejilla `gap-1` y la fila
// `gap-3`.
var (
	anchoMiniaturaFirma = tema.Espacio(20)
	altoOpcionFirma     = tema.Espacio(8)
	altoOpcionTactil    = max(float32(44), 2.75*tema.Rem)
)

// posicionDeFirma es el `<fieldset>`: la leyenda, las cuatro posiciones en dos columnas y
// la página en pequeño con el recuadro encima, para VER dónde va antes de firmar —una
// firma no se mueve después—.
func (d *dialogoFirmar) posicionDeFirma(ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	leyenda := container.New(ui.Margen{Abj: tema.Espacio(1.5)},
		ui.NuevoParrafo(a.t("pdf.dialog.firmar.posicion"), tema.TextoXS, ui.Normal, "text-muted", v).A(ancho))

	pagina, hayPagina := paginaParaFirmar(d.paginas(), d.numeroDePagina())
	rejilla := ancho
	if hayPagina {
		rejilla -= tema.Espacio(3) + anchoMiniaturaFirma
	}
	col := (rejilla - tema.Espacio(1)) / 2
	minimo := altoOpcionFirma
	if a.estrecha() || ui.EsTactil() {
		minimo = altoOpcionTactil
	}
	opciones := make([]*Accion, len(posicionesFirma))
	for i, p := range posicionesFirma {
		opciones[i] = d.opcionDePosicion(p, col)
	}
	filas := make([]fyne.CanvasObject, 0, 2)
	for i := 0; i < len(opciones); i += 2 {
		// Una fila de la rejilla mide lo que la más alta de sus dos celdas: si un rótulo
		// se parte en dos renglones, su vecina se estira con él.
		alto := minimo
		for _, o := range opciones[i : i+2] {
			alto = max(alto, MedidaAccion(o).Height)
		}
		for _, o := range opciones[i : i+2] {
			o.Alto = alto
		}
		filas = append(filas, container.New(ui.Fila{Hueco: tema.Espacio(1)}, opciones[i], opciones[i+1]))
	}
	fila := []fyne.CanvasObject{container.New(anchoFijo{ancho: rejilla}, container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))}
	if hayPagina {
		var imagen func() image.Image
		if a.p.imagenes != nil {
			n := pagina.Numero
			imagen = func() image.Image { return a.p.imagenDe(n) }
		}
		d.miniatura = nuevaMiniaturaFirma(pagina, d.rectVisible(), imagen)
		fila = append(fila, d.miniatura)
	}
	// La quinta, a lo ancho de la rejilla (`col-span-2`): no es una esquina más sino otra
	// manera de elegir.
	dibujada := d.opcionDePosicion(FirmaDibujada, rejilla)
	dibujada.Alto = max(minimo, MedidaAccion(dibujada).Height)
	filas = append(filas, dibujada)
	fila[0] = container.New(anchoFijo{ancho: rejilla}, container.New(ui.Columna{Hueco: tema.Espacio(1)}, filas...))
	partes := []fyne.CanvasObject{leyenda, container.New(ui.Fila{Hueco: tema.Espacio(3)}, fila...)}
	if d.posicion == FirmaDibujada && hayPagina {
		partes = append(partes, d.dibujarRecuadro(pagina, ancho))
	}
	return container.New(ui.Columna{}, partes...)
}

// anchoDibujoFirma es el `max-w-64` del área donde se dibuja.
var anchoDibujoFirma = tema.Espacio(64)

// dibujarRecuadro es `DibujarRecuadro`: la ayuda y la página en grande donde se arrastra
// el recuadro. Vive DENTRO del diálogo en vez de cerrarlo para señalar en el visor: así
// lo ya dibujado en el lienzo —o el certificado elegido— no se pierde.
func (d *dialogoFirmar) dibujarRecuadro(pagina Pagina, ancho float32) fyne.CanvasObject {
	a := d.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ayuda := ui.NuevoParrafo(a.t("pdf.dialog.firmar.dibujar.ayuda"), tema.Texto2XS, ui.Normal, "text-subtle", v).A(ancho)
	var imagen func() image.Image
	if a.p.imagenes != nil {
		n := pagina.Numero
		imagen = func() image.Image { return a.p.imagenDe(n) }
	}
	caja := cajaParaFirmar(d.paginas(), d.numeroDePagina(), d.todas)
	d.dibujo = nuevoDibujoFirma(pagina, caja, d.rectVisible(), min(anchoDibujoFirma, ancho), imagen, func(r Rect) {
		f := aFracciones(caja, r)
		d.recuadro = &f
		// La miniatura de al lado sigue al trazo, como en la web, que la repinta con el
		// mismo estado: sin esto enseñaría la posición de antes hasta rehacer el diálogo.
		if m := d.miniatura; m != nil {
			m.rect = d.rectVisible()
			m.Refresh()
		}
	})
	d.dibujo.etiqueta = a.t("pdf.dialog.firmar.posicion.dibujada")
	return container.New(ui.Margen{Arr: tema.Espacio(1.5)}, container.New(ui.Columna{Hueco: tema.Espacio(1.5)}, ayuda, container.NewCenter(d.dibujo)))
}

// opcionDePosicion es una `<label>` con su radio escondido (`sr-only`): una caja con
// borde, el rótulo a la izquierda y, la marcada, `border-primary bg-selected
// font-medium`.
func (d *dialogoFirmar) opcionDePosicion(p PosicionFirma, ancho float32) *Accion {
	o := NuevaAccion(d.a.t("pdf.dialog.firmar.posicion."+string(p)), func() {
		d.posicion = p
		d.a.rehacer()
	})
	o.PX, o.PY, o.Ancho, o.Envuelto, o.AlInicio = tema.Espacio(2), tema.Espacio(1), ancho, true, true
	if d.posicion == p {
		o.Borde, o.Fondo, o.FondoEncima, o.Tinta, o.Peso = "primary", "selected", "", "text", ui.Medio
	} else {
		o.Borde, o.Fondo, o.FondoEncima, o.Tinta, o.TintaEncima, o.Peso = "border", "surface", "bg-subtle", "text-muted", "text", ui.Normal
	}
	return o
}

// miniaturaFirma es la página en pequeño (`w-20 border bg-white`, con la proporción de
// la hoja) y el recuadro donde va a caer (`border-primary bg-primary/15 rounded-sm`).
// Decorativa, como en la web (`aria-hidden`): lo que cuenta ya lo dice la opción marcada.
type miniaturaFirma struct {
	widget.BaseWidget
	pagina Pagina
	rect   Rect
	imagen func() image.Image
	// ancho es el de la caja; cero, el de la miniatura (`w-20`). El área de dibujar
	// reutiliza la misma página con el recuadro, en grande.
	ancho float32
	// grueso es el borde del recuadro (`border-2` al dibujar, `border` en la miniatura).
	grueso float32
}

func (m *miniaturaFirma) anchoCaja() float32 {
	if m.ancho > 0 {
		return m.ancho
	}
	return anchoMiniaturaFirma
}

func nuevaMiniaturaFirma(p Pagina, r Rect, imagen func() image.Image) *miniaturaFirma {
	m := &miniaturaFirma{pagina: p, rect: r, imagen: imagen}
	m.ExtendBaseWidget(m)
	return m
}

// altoMiniaturaFirma es el `aspect-ratio` de la página sobre los 80 de ancho, borde
// incluido (`box-sizing: border-box`).
func (m *miniaturaFirma) alto() float32 {
	if m.pagina.Ancho <= 0 || m.pagina.Alto <= 0 {
		return m.anchoCaja()
	}
	return m.anchoCaja() * float32(m.pagina.Alto/m.pagina.Ancho)
}

func (m *miniaturaFirma) MinSize() fyne.Size {
	m.ExtendBaseWidget(m)
	return fyne.NewSize(m.anchoCaja(), m.alto())
}

// cajaRecuadro es el recuadro en la miniatura: en porcentajes de la página, sobre la
// caja de RELLENO (dentro del borde), que es la que toma de referencia un hijo
// `absolute`.
func (m *miniaturaFirma) cajaRecuadro() (fyne.Position, fyne.Size) {
	c := RectEnSVG(m.pagina, m.rect)
	w, h := m.anchoCaja()-2*tema.Borde, m.alto()-2*tema.Borde
	if m.pagina.Ancho <= 0 || m.pagina.Alto <= 0 {
		return fyne.NewPos(tema.Borde, tema.Borde), fyne.Size{}
	}
	fx, fy := float64(w)/m.pagina.Ancho, float64(h)/m.pagina.Alto
	return fyne.NewPos(tema.Borde+float32(c.X*fx), tema.Borde+float32(c.Y*fy)),
		fyne.NewSize(float32(c.Ancho*fx), float32(c.Alto*fy))
}

func (m *miniaturaFirma) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &miniaturaFirmaRenderer{
		m:        m,
		papel:    ui.NuevaCaja(ColorPapel, tema.Color("border", v), 0),
		recuadro: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{A: 1}, tema.RadioSM),
	}
	if m.grueso > 0 {
		r.recuadro.ConBorde(m.grueso)
	}
	r.dibujo = canvas.NewRaster(r.generar)
	r.dibujo.ScaleMode = canvas.ImageScalePixels
	r.Refresh()
	return r
}

type miniaturaFirmaRenderer struct {
	m        *miniaturaFirma
	papel    *ui.Caja
	dibujo   *canvas.Raster
	recuadro *ui.Caja
	hecha    image.Image
}

// generar remuestrea el dibujo de la hoja a la miniatura, por dentro del borde.
func (r *miniaturaFirmaRenderer) generar(w, h int) image.Image {
	var img image.Image
	if r.m.imagen != nil {
		img = r.m.imagen()
	}
	if img == nil || w <= 0 || h <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	}
	if r.hecha != nil && r.hecha.Bounds().Dx() == w && r.hecha.Bounds().Dy() == h {
		return r.hecha
	}
	r.hecha = RecortarPagina(img, float64(r.m.anchoCaja()), float64(r.m.alto()), w, h)
	return r.hecha
}

func (r *miniaturaFirmaRenderer) MinSize() fyne.Size { return r.m.MinSize() }
func (r *miniaturaFirmaRenderer) Destroy()           {}

func (r *miniaturaFirmaRenderer) Objects() []fyne.CanvasObject {
	objs := append([]fyne.CanvasObject{}, r.papel.Objetos()...)
	return append(append(objs, r.dibujo), r.recuadro.Objetos()...)
}

func (r *miniaturaFirmaRenderer) Layout(t fyne.Size) {
	r.papel.Redimensionar(t)
	r.dibujo.Move(fyne.NewPos(tema.Borde, tema.Borde))
	r.dibujo.Resize(fyne.NewSize(max(t.Width-2*tema.Borde, 0), max(t.Height-2*tema.Borde, 0)))
	pos, tam := r.m.cajaRecuadro()
	ui.MoverCaja(r.recuadro, pos, tam)
}

func (r *miniaturaFirmaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	primario := tema.Color("primary", v)
	// `bg-primary/15` sobre el papel, que es blanco en los dos temas: aplanado, para que
	// el borde del recuadro no se vea a través del relleno.
	relleno := primario
	relleno.A = uint8(math.Round(0.15 * 255))
	r.recuadro.Pintar(tokens.SobreSRGB(relleno, ColorPapel), primario)
	r.papel.Pintar(ColorPapel, tema.Color("border", v))
	r.Layout(r.m.Size())
	r.dibujo.Refresh()
}

// ---- El área donde se dibuja ----

// dibujoFirma es `DibujarRecuadro`: la página en grande con el recuadro encima. Se
// arrastra de una esquina a la otra (ratón, dedo o lápiz) y, con el foco, las flechas lo
// mueven (Mayúsculas, un paso diez veces mayor) y + y − lo agrandan o encogen. Todo en
// coordenadas del dibujo de la página, pasadas al PDF con [APDF], que sabe de /Rotate.
type dibujoFirma struct {
	widget.BaseWidget
	m         *miniaturaFirma
	caja      Rect
	alCambiar func(Rect)
	etiqueta  string
	inicio    *[2]float64
	enfocado  bool
}

var (
	_ fyne.Draggable     = (*dibujoFirma)(nil)
	_ fyne.Tappable      = (*dibujoFirma)(nil)
	_ fyne.Focusable     = (*dibujoFirma)(nil)
	_ desktop.Cursorable = (*dibujoFirma)(nil)
)

func nuevoDibujoFirma(p Pagina, caja, rect Rect, ancho float32, imagen func() image.Image, alCambiar func(Rect)) *dibujoFirma {
	d := &dibujoFirma{caja: caja, alCambiar: alCambiar}
	d.m = nuevaMiniaturaFirma(p, rect, imagen)
	d.m.ancho, d.m.grueso = ancho, 2*tema.Borde
	d.ExtendBaseWidget(d)
	return d
}

// puntoPDF lleva un punto del área al PDF, sin salir de la hoja.
func (d *dibujoFirma) puntoPDF(pos fyne.Position) [2]float64 {
	s := d.Size()
	fx := math.Min(1, math.Max(0, float64(pos.X/max(s.Width, 1))))
	fy := math.Min(1, math.Max(0, float64(pos.Y/max(s.Height, 1))))
	x, y := APDF(d.m.pagina, fx*d.m.pagina.Ancho, fy*d.m.pagina.Alto)
	return [2]float64{x, y}
}

// poner deja el recuadro nuevo, lo pinta y lo cuenta.
func (d *dibujoFirma) poner(r Rect) {
	d.m.rect = r
	d.m.Refresh()
	if d.alCambiar != nil {
		d.alCambiar(r)
	}
}

// Tapped es un toque suelto: el recuadro mínimo desde ahí.
func (d *dibujoFirma) Tapped(e *fyne.PointEvent) {
	p := d.puntoPDF(e.Position)
	d.poner(recuadroArrastrado(d.caja, p, p))
}

// Dragged dibuja desde donde empezó el arrastre.
func (d *dibujoFirma) Dragged(e *fyne.DragEvent) {
	if d.inicio == nil {
		p := d.puntoPDF(e.Position.Subtract(e.Dragged))
		d.inicio = &p
	}
	d.poner(recuadroArrastrado(d.caja, *d.inicio, d.puntoPDF(e.Position)))
}

func (d *dibujoFirma) DragEnd() { d.inicio = nil }

// Cursor es el `cursor-crosshair`.
func (d *dibujoFirma) Cursor() desktop.Cursor { return desktop.CrosshairCursor }

func (d *dibujoFirma) FocusGained() { d.enfocado = true; d.Refresh() }
func (d *dibujoFirma) FocusLost()   { d.enfocado = false; d.Refresh() }
func (d *dibujoFirma) TypedRune(r rune) {
	switch r {
	case '+', '=':
		d.poner(escalarRecuadro(d.caja, d.m.rect, 1.1))
	case '-', '−':
		d.poner(escalarRecuadro(d.caja, d.m.rect, 1/1.1))
	}
}

// TypedKey mueve con las flechas un 1 % de la hoja (un 10 % con Mayúsculas) en la
// dirección de la PANTALLA: en una página girada no coincide con la del PDF.
func (d *dibujoFirma) TypedKey(e *fyne.KeyEvent) {
	// Escape y «atrás» no son de este control: suben al diálogo, que se cierra.
	if ui.SubirEscape(d, e) {
		return
	}
	paso := 0.01
	if ui.MayusculasPulsadas() {
		paso = 0.1
	}
	paso *= math.Max(d.m.pagina.Ancho, d.m.pagina.Alto)
	var sx, sy float64
	switch e.Name {
	case fyne.KeyLeft:
		sx = -paso
	case fyne.KeyRight:
		sx = paso
	case fyne.KeyUp:
		sy = -paso
	case fyne.KeyDown:
		sy = paso
	default:
		return
	}
	ax, ay := APDF(d.m.pagina, 0, 0)
	bx, by := APDF(d.m.pagina, sx, sy)
	d.poner(moverRecuadro(d.caja, d.m.rect, bx-ax, by-ay))
}

func (d *dibujoFirma) MinSize() fyne.Size { return d.m.MinSize() }

func (d *dibujoFirma) CreateRenderer() fyne.WidgetRenderer {
	r := &dibujoFirmaRenderer{d: d, aro: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, 0).ConBorde(2 * tema.Borde)}
	return r
}

type dibujoFirmaRenderer struct {
	d   *dibujoFirma
	aro *ui.Caja
}

func (r *dibujoFirmaRenderer) MinSize() fyne.Size { return r.d.MinSize() }
func (r *dibujoFirmaRenderer) Destroy()           {}
func (r *dibujoFirmaRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.d.m}, r.aro.Objetos()...)
}

func (r *dibujoFirmaRenderer) Layout(t fyne.Size) {
	r.d.m.Move(fyne.NewPos(0, 0))
	r.d.m.Resize(t)
	r.aro.Redimensionar(t)
}

// Refresh pinta el aro de foco (`sytue-focus-ring`) solo con el foco.
func (r *dibujoFirmaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	borde := color.NRGBA{}
	if r.d.enfocado {
		borde = tema.Color("focus-ring", v)
	}
	r.aro.Pintar(color.NRGBA{}, borde)
	r.Layout(r.d.Size())
	r.d.m.Refresh()
}
