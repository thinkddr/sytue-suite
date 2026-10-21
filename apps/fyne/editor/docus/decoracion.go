// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image"
	"math"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus/rico"
	"git.sytue.eu/sytue/one/apps/nucleo/fuentes"
)

// La MARCA DE AGUA y el BORDE DE PÁGINA en el folio (`pagedecor.ts`): la marca es una
// baldosa de 794 × 1123 —la del SVG de la web— repetida hacia abajo cada 1123 px, centrada
// en el ancho del papel, detrás del texto y con su opacidad; el borde, un marco a medio
// margen del papel. El HTML es de `rico` (decoracion.go).

// Medidas de la baldosa, las del `viewBox` de `watermarkSvg`.
const (
	baldosaAncho float32 = 794
	baldosaAlto  float32 = 1123
	// escalaBaldosa es a cuánto se rasteriza: a ×2, una pantalla HiDPI no la ve borrosa.
	escalaBaldosa = 2
)

// baldosas son las ya rasterizadas, por su ajuste: la marca no cambia al teclear.
var baldosas = struct {
	mu sync.Mutex
	m  map[rico.MarcaAgua]image.Image
}{m: map[rico.MarcaAgua]image.Image{}}

// caraMarca es la letra de la marca: el SVG pide `Inter, system-ui` en negrita; si el
// sistema no tiene Inter, la Figtree negrita de la suite.
// ponytail: el navegador caería en su `system-ui`, que depende de la máquina.
var caraMarca = sync.OnceValue(func() *opentype.Font {
	datos, ok := fuentes.DelSistema("Inter", true, false)
	if !ok {
		datos = fuentes.Leer("Figtree-Bold.ttf")
	}
	f, err := opentype.Parse(datos)
	if err != nil {
		f, _ = opentype.Parse(fuentes.Leer("Figtree-Bold.ttf"))
	}
	return f
})

// baldosaDe es la baldosa de la marca a ×escalaBaldosa, con el texto o la imagen en medio
// y girada −45° si va en diagonal. nil si no se puede dibujar.
func baldosaDe(m rico.MarcaAgua) image.Image {
	baldosas.mu.Lock()
	b, ok := baldosas.m[m]
	baldosas.mu.Unlock()
	if ok {
		return b
	}
	w, h := int(baldosaAncho*escalaBaldosa), int(baldosaAlto*escalaBaldosa)
	recto := image.NewNRGBA(image.Rect(0, 0, w, h))
	if m.Imagen {
		dibujarImagenMarca(recto, m)
	} else {
		dibujarTextoMarca(recto, m)
	}
	var out image.Image = recto
	if m.Diagonal {
		out = girar(recto, -45)
	}
	baldosas.mu.Lock()
	baldosas.m[m] = out
	baldosas.mu.Unlock()
	return out
}

// dibujarTextoMarca es el `<text>` del SVG: en su color, a su tamaño, negrita, con
// `letter-spacing: 0.06em`, centrado (`text-anchor: middle`) y con la línea base a media
// caja (`dominant-baseline: central`).
func dibujarTextoMarca(dst *image.NRGBA, m rico.MarcaAgua) {
	tam := float64(m.Tamano) * escalaBaldosa
	cara, err := opentype.NewFace(caraMarca(), &opentype.FaceOptions{Size: tam, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return
	}
	defer cara.Close()
	espacio := fixed.Int26_6(0.06 * tam * 64)
	var ancho fixed.Int26_6
	previa := rune(-1)
	n := 0
	for _, r := range m.Texto {
		if previa >= 0 {
			ancho += cara.Kern(previa, r)
		}
		av, _ := cara.GlyphAdvance(r)
		ancho += av
		previa, n = r, n+1
	}
	ancho += espacio * fixed.Int26_6(n) // el espaciado va detrás de cada letra, también la última
	col, _ := editor.Hex(m.Color)
	d := font.Drawer{Dst: dst, Src: image.NewUniform(col), Face: cara}
	met := cara.Metrics()
	cx, cy := float64(baldosaAncho)*escalaBaldosa/2, float64(baldosaAlto)*escalaBaldosa/2
	base := cy + float64(met.Ascent-met.Descent)/64/2
	d.Dot = fixed.Point26_6{X: fixed.Int26_6((cx - float64(ancho)/64/2) * 64), Y: fixed.Int26_6(base * 64)}
	previa = -1
	for _, r := range m.Texto {
		if previa >= 0 {
			d.Dot.X += cara.Kern(previa, r)
		}
		d.DrawString(string(r))
		d.Dot.X += espacio
		previa = r
	}
}

// dibujarImagenMarca es el `<image>` del SVG: al `size` % del ancho y alto de la baldosa,
// centrada y sin deformarse (`preserveAspectRatio: meet`).
func dibujarImagenMarca(dst *image.NRGBA, m rico.MarcaAgua) {
	img := imagenDe(m.Src)
	if img == nil || img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		return
	}
	esc := float64(m.Tamano) / 100
	bw, bh := float64(baldosaAncho)*esc*escalaBaldosa, float64(baldosaAlto)*esc*escalaBaldosa
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	k := math.Min(bw/iw, bh/ih)
	w, h := iw*k, ih*k
	x := (float64(dst.Bounds().Dx()) - w) / 2
	y := (float64(dst.Bounds().Dy()) - h) / 2
	draw.CatmullRom.Scale(dst, image.Rect(int(x), int(y), int(x+w), int(y+h)), img, img.Bounds(), draw.Over, nil)
}

// girar gira la imagen `grados` alrededor de su centro, sin cambiarla de tamaño: lo que se
// sale se pierde, como en la baldosa del SVG.
func girar(src *image.NRGBA, grados float64) image.Image {
	b := src.Bounds()
	dst := image.NewNRGBA(b)
	a := grados * math.Pi / 180
	cx, cy := float64(b.Dx())/2, float64(b.Dy())/2
	cos, sin := math.Cos(a), math.Sin(a)
	// El SVG gira en el sentido de las agujas con ángulo positivo (y hacia abajo): −45 sube
	// por la derecha. La matriz lleva del origen al destino (`s2d`).
	m := f64.Aff3{cos, -sin, cx - cos*cx + sin*cy, sin, cos, cy - sin*cx - cos*cy}
	draw.BiLinear.Transform(dst, m, src, b, draw.Over, nil)
	return dst
}

// pintarDecoracion añade la marca de agua (baldosa a baldosa, detrás del texto) y el borde
// de página, con el papel en `papel` y el zoom z.
func (f *Folio) pintarDecoracion(objs []fyne.CanvasObject, papel fyne.Position, z float32) []fyne.CanvasObject {
	anchoPag, _ := f.pagina.medidas()
	alto := f.altoPapel()
	if m := f.doc.MarcaAguaDe(); m != nil {
		if b := baldosaDe(*m); b != nil {
			x := papel.X + (anchoPag-baldosaAncho)/2*z
			for y := float32(0); y < alto; y += baldosaAlto {
				// La última baldosa se corta en el pie del papel, como un fondo.
				visible := min(baldosaAlto, alto-y)
				trozo := b
				if s, ok := b.(interface {
					SubImage(image.Rectangle) image.Image
				}); ok && visible < baldosaAlto {
					trozo = s.SubImage(image.Rect(0, 0, b.Bounds().Dx(), int(visible*escalaBaldosa)))
				}
				img := canvas.NewImageFromImage(trozo)
				img.FillMode, img.ScaleMode = canvas.ImageFillStretch, canvas.ImageScaleSmooth
				img.Translucency = 1 - m.Opacidad
				img.Move(fyne.NewPos(x, papel.Y+y*z))
				img.Resize(fyne.NewSize(baldosaAncho*z, visible*z))
				objs = append(objs, img)
			}
		}
	}
	if b := f.doc.BordeDe(); b != nil {
		objs = append(objs, pintarBorde(b, papel, anchoPag, alto, f.pagina.margenes()[3], z)...)
	}
	return objs
}

// pintarBorde es `border: <grosor>px <estilo> <color>` a `margen / 2` de cada lado del
// papel. Discontinuo y punteado como los pinta Chromium: trazos de tres grosores con el
// mismo hueco, y puntos de un grosor con el hueco de otro; el doble, dos rayas de un tercio
// con un tercio entre ellas.
func pintarBorde(b *rico.BordePagina, papel fyne.Position, ancho, alto, margen, z float32) []fyne.CanvasObject {
	col, _ := editor.Hex(b.Color)
	g := float32(b.Grosor)
	x0, y0 := margen/2, margen/2
	w, h := ancho-margen, alto-margen
	var objs []fyne.CanvasObject
	rect := func(x, y, rw, rh float32) {
		r := canvas.NewRectangle(col)
		r.Move(fyne.NewPos(papel.X+x*z, papel.Y+y*z))
		r.Resize(fyne.NewSize(max(rw*z, 1), max(rh*z, 1)))
		objs = append(objs, r)
	}
	lado := func(x, y, largo, grueso float32, horizontal bool, trazo, hueco float32) {
		if trazo <= 0 {
			if horizontal {
				rect(x, y, largo, grueso)
			} else {
				rect(x, y, grueso, largo)
			}
			return
		}
		for p := float32(0); p < largo; p += trazo + hueco {
			l := min(trazo, largo-p)
			if horizontal {
				rect(x+p, y, l, grueso)
			} else {
				rect(x, y+p, grueso, l)
			}
		}
	}
	marco := func(x, y, mw, mh, grueso, trazo, hueco float32) {
		lado(x, y, mw, grueso, true, trazo, hueco)
		lado(x, y+mh-grueso, mw, grueso, true, trazo, hueco)
		lado(x, y, mh, grueso, false, trazo, hueco)
		lado(x+mw-grueso, y, mh, grueso, false, trazo, hueco)
	}
	switch b.Estilo {
	case "dashed":
		marco(x0, y0, w, h, g, 3*g, 3*g)
	case "dotted":
		marco(x0, y0, w, h, g, g, g)
	case "double":
		t := float32(math.Round(float64(g) / 3))
		if t < 1 {
			marco(x0, y0, w, h, g, 0, 0)
			break
		}
		marco(x0, y0, w, h, t, 0, 0)
		marco(x0+g-t, y0+g-t, w-2*(g-t), h-2*(g-t), t, 0, 0)
	default:
		marco(x0, y0, w, h, g, 0, 0)
	}
	return objs
}

// Las órdenes de Disposición que escriben en el folio; en la web no pasan por `exec`.
const (
	cmdMarcaAgua editor.Comando = "watermark"  // valor: la marca, «kind|texto|src|color|opacidad|tamaño|diagonal», o "" para quitarla
	cmdBorde     editor.Comando = "pageBorder" // valor: «estilo|grosor|color», o "" para quitarlo
	cmdColumnas  editor.Comando = "columns"    // valor: «1», «2» o «3»
)

// ordenDecoracion ejecuta las órdenes de Disposición. Devuelve false si no es una.
func (f *Folio) ordenDecoracion(c editor.Comando, valor string) bool {
	switch c {
	case cmdMarcaAgua:
		f.antesDeCambiar(false)
		f.doc.PonerMarcaAgua(leerMarca(valor))
		f.reestructurado()
	case cmdBorde:
		f.antesDeCambiar(false)
		f.doc.PonerBorde(leerBorde(valor))
		f.reestructurado()
	case cmdColumnas:
		n, _ := strconv.Atoi(valor)
		a, b := f.orden()
		f.antesDeCambiar(false)
		l, ok := f.doc.Columnas(f.bloques, a.b, b.b, n)
		if !ok {
			f.hist.atras = f.hist.atras[:len(f.hist.atras)-1]
			return true
		}
		if l != (rico.Lugar{}) {
			f.irA(l, 0)
		}
		f.reestructurado()
	default:
		return false
	}
	return true
}

// leerMarca y leerBorde deshacen el valor de sus órdenes, saneado como en la web.
func leerMarca(v string) *rico.MarcaAgua {
	p := partes(v, 7)
	if p == nil {
		return nil
	}
	return rico.SanearMarcaAgua(p[0] == "image", p[1], p[2], p[3], p[4], p[5], p[6])
}

func leerBorde(v string) *rico.BordePagina {
	p := partes(v, 3)
	if p == nil {
		return nil
	}
	return rico.SanearBorde(p[0], p[1], p[2])
}

// partes parte un valor por «\x1f» (el separador de unidades: no sale en un texto
// tecleado) en exactamente n trozos; nil si no.
func partes(v string, n int) []string {
	if v == "" {
		return nil
	}
	var out []string
	ini := 0
	for i := 0; i < len(v); i++ {
		if v[i] == 0x1f {
			out = append(out, v[ini:i])
			ini = i + 1
		}
	}
	out = append(out, v[ini:])
	if len(out) != n {
		return nil
	}
	return out
}

// valorMarca y valorBorde escriben el valor de sus órdenes.
func valorMarca(m *rico.MarcaAgua) string {
	if m == nil {
		return ""
	}
	kind := "text"
	if m.Imagen {
		kind = "image"
	}
	diag := "0"
	if m.Diagonal {
		diag = "1"
	}
	return kind + "\x1f" + m.Texto + "\x1f" + m.Src + "\x1f" + m.Color + "\x1f" +
		strconv.FormatFloat(m.Opacidad, 'f', -1, 64) + "\x1f" + strconv.Itoa(m.Tamano) + "\x1f" + diag
}

func valorBorde(b *rico.BordePagina) string {
	if b == nil {
		return ""
	}
	return b.Estilo + "\x1f" + strconv.Itoa(b.Grosor) + "\x1f" + b.Color
}
