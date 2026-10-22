// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pdfdoc

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // decodificadores registrados: PNG y GIF se pasan a ráster crudo
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"
)

// ErrImagen es una imagen que no se puede incrustar (formato desconocido, corrupta o
// demasiado grande). Quien llama decide qué poner en su lugar: nunca se pinta a medias.
var ErrImagen = errors.New("pdfdoc: imagen no admitida")

// maxPixeles acota la memoria de una imagen decodificada (16 Mpx ≈ 48 MiB en RGB).
const maxPixeles = 16_000_000

// Imagen es una imagen en línea. Alt es el texto alternativo que el documento ya guarda: con
// él la imagen es una Figure con /Alt; sin él es un artefacto (decorativa) y se cuenta en
// SinAlt, para que quien exporta pueda avisar.
type Imagen struct {
	Datos       []byte  // PNG, JPEG o GIF
	Ancho, Alto float64 // puntos; 0 = tamaño natural a 96 ppp
	Alt         string
	Alinear     Alineacion
}

type imgUsada struct {
	nombre    string
	w, h      int
	jpeg      []byte // JPEG pasado tal cual (DCTDecode)
	rgb, alfa []byte // ráster crudo 8 bit; alfa nil si es opaca
	gris      bool
	obj       int
}

// imagen decodifica (una sola vez por contenido) y registra una imagen del documento.
func (d *Doc) imagen(datos []byte) (*imgUsada, error) {
	clave := sha256.Sum256(datos)
	if im := d.imagenes[clave]; im != nil {
		return im, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(datos))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxPixeles/cfg.Height {
		return nil, fmt.Errorf("%w: cabecera o tamaño", ErrImagen)
	}
	img, formato, err := image.Decode(bytes.NewReader(datos)) // valida también el cuerpo
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrImagen, err)
	}
	im := &imgUsada{nombre: "Im" + strconv.Itoa(len(d.imagenes)), w: cfg.Width, h: cfg.Height}
	switch {
	case formato == "jpeg" && cfg.ColorModel == color.YCbCrModel:
		im.jpeg = datos
	case formato == "jpeg" && cfg.ColorModel == color.GrayModel:
		im.jpeg, im.gris = datos, true
	default:
		im.rgb, im.alfa = aRGB(img)
	}
	if d.imagenes == nil {
		d.imagenes = map[[32]byte]*imgUsada{}
	}
	d.imagenes[clave] = im
	d.ordenImg = append(d.ordenImg, im)
	return im, nil
}

// aRGB convierte a RGB de 8 bits y, si algún píxel no es opaco, a un canal alfa aparte.
func aRGB(img image.Image) (rgb, alfa []byte) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rgb = make([]byte, 0, 3*w*h)
	var a []byte
	translucida := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c, _ := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			rgb = append(rgb, c.R, c.G, c.B)
			a = append(a, c.A)
			translucida = translucida || c.A != 255
		}
	}
	if translucida {
		return rgb, a
	}
	return rgb, nil
}

// escribirImagenes emite los XObject y devuelve el diccionario /XObject de los recursos.
func (d *Doc) escribirImagenes(s *salida) string {
	if len(d.ordenImg) == 0 {
		return ""
	}
	var refs []string
	for _, im := range d.ordenImg {
		base := "/Type /XObject /Subtype /Image /Width " + strconv.Itoa(im.w) + " /Height " + strconv.Itoa(im.h) + " /BitsPerComponent 8"
		cs := " /ColorSpace /DeviceRGB"
		if im.gris {
			cs = " /ColorSpace /DeviceGray"
		}
		switch {
		case im.jpeg != nil:
			im.obj = s.crudo(base+cs+" /Filter /DCTDecode", im.jpeg)
		default:
			extra := ""
			if im.alfa != nil {
				extra = " /SMask " + strconv.Itoa(s.Flujo(base+" /ColorSpace /DeviceGray", im.alfa)) + " 0 R"
			}
			im.obj = s.Flujo(base+cs+extra, im.rgb)
		}
		refs = append(refs, "/"+im.nombre+" "+strconv.Itoa(im.obj)+" 0 R")
	}
	return " /XObject << " + strings.Join(refs, " ") + " >>"
}

// imagenLay es una imagen ya dimensionada, lista para colocar.
type imagenLay struct {
	im     *imgUsada
	w, h   float64
	alt    string
	figura *Elem
	alinea Alineacion
}

// maquetarImagen dimensiona la imagen para que quepa en ancho × alto máximos, sin deformarla,
// y crea su Figure (con /Alt) bajo padre si tiene texto alternativo.
func (d *Doc) maquetarImagen(padre *Elem, img Imagen, ancho, alto float64) (*imagenLay, error) {
	im, err := d.imagen(img.Datos)
	if err != nil {
		return nil, err
	}
	w, h := img.Ancho, img.Alto
	switch {
	case w <= 0 && h <= 0:
		w, h = float64(im.w)*0.75, float64(im.h)*0.75
	case w <= 0:
		w = h * float64(im.w) / float64(im.h)
	case h <= 0:
		h = w * float64(im.h) / float64(im.w)
	}
	if k := min(1, ancho/w, alto/h); k < 1 {
		w, h = w*k, h*k
	}
	l := &imagenLay{im: im, w: w, h: h, alt: strings.TrimSpace(img.Alt), alinea: img.Alinear}
	if l.alt != "" {
		l.figura = d.nuevo("Figure", padre, true)
		l.figura.alt = l.alt
	} else {
		d.sinAlt++
	}
	return l, nil
}

// SinAlt cuenta las imágenes escritas como artefacto por no tener texto alternativo.
func (d *Doc) SinAlt() int { return d.sinAlt }

// dibujarImagen coloca la imagen con su esquina superior izquierda en (x, ysup).
func (d *Doc) dibujarImagen(l *imagenLay, x, ysup float64) {
	op := fmt.Sprintf("q %s 0 0 %s %s %s cm /%s Do Q\n", ftoa(l.w), ftoa(l.h), ftoa(x), ftoa(ysup-l.h), l.im.nombre)
	if l.figura != nil {
		d.abrirMC(l.figura)
		d.cur.contenido.WriteString(op)
		return
	}
	d.artefacto(func(b *strings.Builder) { b.WriteString(op) })
}

// Imagen escribe una imagen en el flujo del documento, en su propio bloque.
func (d *Doc) Imagen(img Imagen) error {
	d.listas = nil
	l, err := d.maquetarImagen(d.contenedor(), img, PageW-2*Margin, PageH-2*Margin)
	if err != nil {
		return err
	}
	if d.y-l.h < Margin {
		d.NuevaPagina()
	}
	x := Margin
	switch img.Alinear {
	case Izquierda:
	case Centro:
		x += (PageW - 2*Margin - l.w) / 2
	case Derecha:
		x += PageW - 2*Margin - l.w
	}
	d.dibujarImagen(l, x, d.y)
	d.y -= l.h + 6
	return nil
}
