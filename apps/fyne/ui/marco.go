// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// En la web: branding/tokens/appearance.css (una hoja, no una pantalla: sin marcador de paridad).

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// La forma «Marco»: la barra superior y el menú lateral son un solo marco de color
// continuo y el contenido, una HOJA redondeada que flota dentro. Es la forma por defecto
// de la suite; «Carril» es la que se elige (`tema.EsMarco`).
//
// En la web son tres clases de appearance.css: `.sytue-frame` pinta el marco,
// `.sytue-topbar-carril` hace de la barra parte de él y `.sytue-sheet` es la hoja, con
// `border-radius` y SIN recorte: desde el 2026-10-05 la hoja de la web no lleva
// `overflow` y sus esquinas las redondea lo de dentro (`.sytue-sheet-corners`), que es
// lo mismo que se hace aquí. Lo pinta esta pieza, UNA vez para las catorce
// apps: la usan `Armazon` (la hoja empieza donde acaba el menú) y `BarraSuite.Montar`
// (para lo que no trae armazón: el Hub, los editores, la página sin sesión).
//
// # Cómo se redondea una hoja sin recorte
//
// Fyne no recorta a una esquina redonda: un hijo opaco pinta su rectángulo entero y se
// come la esquina del papel. Así que la hoja se pinta en tres capas:
//
//  1. DETRÁS del contenido, el marco y el papel de la hoja, ya redondeado;
//  2. el contenido, rectangular, como siempre;
//  3. DELANTE, cuatro MÁSCARAS —la esquina al revés, del color del marco— que tapan lo
//     que el contenido haya pintado fuera del arco, y encima la sombra de la hoja, que
//     no lleva interior.
//
// Una máscara es un cuadrado del radio de la hoja con el arco vaciado. El contenido no
// se entera de nada, y lo que se abre POR ENCIMA de la app —diálogos, menús— va en las
// capas flotantes del lienzo, que quedan sobre las máscaras.
type marco struct {
	fondo    *fondoMarco
	hoja     *canvas.Rectangle
	esquinas [4]*canvas.Raster // arriba-izquierda, arriba-derecha, abajo-izquierda, abajo-derecha
	sombra   sombraHoja

	// Lo que las máscaras necesitan saber para pintar el color del marco en su sitio.
	ancho  float32    // el del marco: su degradado va de lado a lado de la ventana
	xs     [4]float32 // la x de cada esquina dentro del marco
	radio  float32
	cache  [4]*image.NRGBA
	hechas [4]mascaraHecha
}

// mascaraHecha es con qué se pintó la máscara que está en caché.
type mascaraHecha struct {
	w, h         int
	desde, hasta color.NRGBA
	x, ancho     float32
}

func nuevoMarco() *marco {
	m := &marco{fondo: nuevoFondoMarco(), hoja: canvas.NewRectangle(color.NRGBA{})}
	for i := range m.esquinas {
		m.esquinas[i] = canvas.NewRaster(m.mascara(i))
		m.esquinas[i].ScaleMode = canvas.ImageScalePixels
	}
	m.sombra = nuevaSombraHoja()
	return m
}

// detras son las capas que van ANTES del contenido: el marco y el papel de la hoja.
func (m *marco) detras() []fyne.CanvasObject {
	return append(m.fondo.objetos(), m.hoja)
}

// delante son las que van DESPUÉS: las máscaras de las esquinas y la sombra.
func (m *marco) delante() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, 0, len(m.esquinas)+len(m.sombra.teselas))
	for _, e := range m.esquinas {
		out = append(out, e)
	}
	for _, t := range m.sombra.teselas {
		out = append(out, t)
	}
	return out
}

// esconder quita el marco de la vista: es lo que toca en Carril, o cuando la hoja la
// pinta otro.
func (m *marco) esconder() {
	m.fondo.esconder()
	m.esconderHoja()
}

func (m *marco) esconderHoja() {
	for _, o := range append([]fyne.CanvasObject{m.hoja}, m.delante()...) {
		o.Hide()
	}
}

// colocar pinta el marco en el rectángulo (`origen`, `total`) y la hoja en (`pos`,
// `tam`), los dos en coordenadas de quien lo lleva. `total` es de lado a lado de la
// ventana: de ahí sale el degradado del traslúcido, que tiene que casar con el de la
// barra. Con `aSangre` la hoja solo redondea arriba y no lleva sombra: es como va en los
// editores y por debajo de 768 px.
func (m *marco) colocar(origen fyne.Position, total fyne.Size, pos fyne.Position, tam fyne.Size, aSangre bool, v tema.Variante) {
	m.fondo.pintar(v)
	m.fondo.colocar(origen, total)

	radio := min(tema.Forma("radius-sheet"), tam.Width/2, tam.Height/2)
	m.hoja.FillColor = tema.Color("sheet-bg", v)
	m.hoja.TopLeftCornerRadius, m.hoja.TopRightCornerRadius = radio, radio
	m.hoja.BottomLeftCornerRadius, m.hoja.BottomRightCornerRadius = radio, radio
	if aSangre {
		m.hoja.BottomLeftCornerRadius, m.hoja.BottomRightCornerRadius = 0, 0
	}
	m.hoja.Move(pos)
	m.hoja.Resize(tam)
	m.hoja.Show()
	m.hoja.Refresh()

	m.ancho, m.radio = total.Width, radio
	lado := fyne.NewSize(radio, radio)
	for i, e := range m.esquinas {
		p := pos
		if i%2 == 1 { // las de la derecha
			p.X += tam.Width - radio
		}
		if i >= 2 { // las de abajo
			p.Y += tam.Height - radio
		}
		m.xs[i] = p.X - origen.X
		e.Move(p)
		e.Resize(lado)
		esconder(e, radio <= 0 || (aSangre && i >= 2))
		e.Refresh()
	}
	m.sombra.colocar(pos, tam, radio, !aSangre, v)
}

// mascara es el generador de la esquina `i`: el cuadrado del radio con el arco vaciado,
// del color que el marco tiene justo ahí.
func (m *marco) mascara(i int) func(w, h int) image.Image {
	return func(w, h int) image.Image {
		con := mascaraHecha{w, h, m.fondo.desde, m.fondo.hasta, m.xs[i], m.ancho}
		if m.cache[i] != nil && m.hechas[i] == con {
			return m.cache[i]
		}
		img := image.NewNRGBA(image.Rect(0, 0, max(w, 0), max(h, 0)))
		m.cache[i], m.hechas[i] = img, con
		if w <= 0 || h <= 0 {
			return img
		}
		// El centro del arco es la esquina de la tesela que mira al interior de la hoja.
		cx, cy := float64(w), float64(h)
		if i%2 == 1 {
			cx = 0
		}
		if i >= 2 {
			cy = 0
		}
		r := float64(min(w, h))
		for y := range h {
			for x := range w {
				// Cuánto del píxel queda FUERA del arco, con el mismo medio píxel de
				// suavizado con el que el pintor redondea la hoja de debajo.
				fuera := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - r + 0.5
				if fuera <= 0 {
					continue
				}
				c := m.fondo.colorEn(m.xs[i]+float32(x)/float32(w)*m.radio, m.ancho)
				c.A = uint8(math.Round(min(fuera, 1) * 255))
				img.SetNRGBA(x, y, c)
			}
		}
		return img
	}
}

// ---- el fondo del marco ----

// fondoMarco es el color continuo del marco: el del menú lateral y, en el traslúcido,
// un degradado HORIZONTAL del primario (`--sytue-frame-from` → `--sytue-frame-to`).
// Horizontal porque la barra y el cuerpo lo pintan cada uno por su cuenta, a todo el
// ancho de la ventana, y así casan sin costura. Lo llevan los dos: `marco` y la barra.
type fondoMarco struct {
	liso         *canvas.Rectangle
	degradado    *canvas.LinearGradient
	desde, hasta color.NRGBA
}

func nuevoFondoMarco() *fondoMarco {
	return &fondoMarco{
		liso:      canvas.NewRectangle(color.NRGBA{}),
		degradado: canvas.NewHorizontalGradient(color.NRGBA{}, color.NRGBA{}),
	}
}

func (f *fondoMarco) objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{f.liso, f.degradado}
}

// pintar toma los colores del marco en ese tema. De un solo color —claro y oscuro— va
// en un rectángulo; el degradado solo se genera cuando lo es.
func (f *fondoMarco) pintar(v tema.Variante) {
	f.desde, f.hasta = tema.Color("frame-from", v), tema.Color("frame-to", v)
	f.liso.FillColor = f.desde
	f.degradado.StartColor, f.degradado.EndColor = f.desde, f.hasta
	esconder(f.liso, f.desde != f.hasta)
	esconder(f.degradado, f.desde == f.hasta)
	f.liso.Refresh()
	f.degradado.Refresh()
}

func (f *fondoMarco) colocar(pos fyne.Position, tam fyne.Size) {
	for _, o := range f.objetos() {
		o.Move(pos)
		o.Resize(tam)
	}
}

func (f *fondoMarco) esconder() {
	f.liso.Hide()
	f.degradado.Hide()
}

// colorEn es el color del marco a `x` de su borde izquierdo, con `ancho` de lado a lado:
// la misma cuenta que el degradado, canal a canal en sRGB.
func (f *fondoMarco) colorEn(x, ancho float32) color.NRGBA {
	if f.desde == f.hasta || ancho <= 0 {
		return f.desde
	}
	t := float64(min(max(x/ancho, 0), 1))
	canal := func(a, b uint8) uint8 { return uint8(math.Round(float64(a) + (float64(b)-float64(a))*t)) }
	return color.NRGBA{R: canal(f.desde.R, f.hasta.R), G: canal(f.desde.G, f.hasta.G), B: canal(f.desde.B, f.hasta.B), A: 0xff}
}

// ---- la sombra de la hoja ----

// sombraHoja es el `shadow-md` de la hoja, en OCHO teselas: las cuatro esquinas a su
// tamaño y los cuatro lados estirados.
//
// No es una `Sombra` a secas porque la hoja mide lo que la ventana: `Sombra` desenfoca
// un plano del tamaño del panel cada vez que cambia, y eso, con una hoja de 1000×700 y a
// cada fotograma mientras se arrastra el borde de la ventana, se nota. El perfil de una
// sombra no depende del tamaño del panel —solo de su radio y de su desenfoque—, así que
// se genera UNA, pequeña, con la misma `Sombra` de siempre, y se trocea.
type sombraHoja struct {
	teselas [8]*canvas.Image
	hecha   bool
	de      tema.Variante
	radio   float32
	margen  float32 // lo que la sombra se sale de la hoja
	esquina float32 // el lado de una tesela de esquina
}

func nuevaSombraHoja() sombraHoja {
	var s sombraHoja
	for i := range s.teselas {
		s.teselas[i] = canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 1, 1)))
		s.teselas[i].FillMode = canvas.ImageFillStretch
	}
	return s
}

// generar hace la sombra de un panel pequeño y la corta en las ocho teselas.
func (s *sombraHoja) generar(radio float32, v tema.Variante) {
	base := NuevaSombra(tema.SombraMD, radio).SinInterior()
	// Un panel lo bastante grande para que las cuatro esquinas no se pisen: el lado de
	// una tesela de esquina, dos veces, y un tramo recto en medio del que sale el lado.
	const recto = 8
	base.generar(fyne.NewSize(1, 1), v) // solo para saber el margen de estas capas
	s.margen = base.margen
	s.esquina = float32(math.Ceil(float64(s.margen + radio + 2)))
	lado := 2*(s.esquina-s.margen) + recto
	base.hecha = false
	base.generar(fyne.NewSize(lado, lado), v)
	img, ok := base.img.Image.(*image.NRGBA)
	if !ok {
		return
	}
	e, total := int(s.esquina), img.Bounds().Dx()
	cortes := [3][2]int{{0, e}, {e, total - e}, {total - e, total}} // inicio, medio y final
	i := 0
	for fila := range 3 {
		for col := range 3 {
			if fila == 1 && col == 1 {
				continue // el interior no lleva sombra
			}
			r := image.Rect(cortes[col][0], cortes[fila][0], cortes[col][1], cortes[fila][1])
			trozo := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
			for y := range r.Dy() {
				copy(trozo.Pix[y*trozo.Stride:y*trozo.Stride+4*r.Dx()], img.Pix[(r.Min.Y+y)*img.Stride+4*r.Min.X:])
			}
			s.teselas[i].Image = trozo
			i++
		}
	}
	s.hecha, s.de, s.radio = true, v, radio
}

// colocar reparte las teselas alrededor de la hoja, o las esconde.
func (s *sombraHoja) colocar(pos fyne.Position, tam fyne.Size, radio float32, ver bool, v tema.Variante) {
	if !ver || tam.Width <= 2*radio || tam.Height <= 2*radio {
		for _, t := range s.teselas {
			t.Hide()
		}
		return
	}
	if !s.hecha || s.de != v || s.radio != radio {
		s.generar(radio, v)
	}
	// Las tres columnas y las tres filas: el margen y la esquina, el tramo que se estira
	// y la otra esquina.
	xs := [4]float32{pos.X - s.margen, pos.X - s.margen + s.esquina, pos.X + tam.Width + s.margen - s.esquina, pos.X + tam.Width + s.margen}
	ys := [4]float32{pos.Y - s.margen, pos.Y - s.margen + s.esquina, pos.Y + tam.Height + s.margen - s.esquina, pos.Y + tam.Height + s.margen}
	i := 0
	for fila := range 3 {
		for col := range 3 {
			if fila == 1 && col == 1 {
				continue
			}
			t := s.teselas[i]
			t.Move(fyne.NewPos(xs[col], ys[fila]))
			t.Resize(fyne.NewSize(max(xs[col+1]-xs[col], 0), max(ys[fila+1]-ys[fila], 0)))
			t.Show()
			t.Refresh()
			i++
		}
	}
}

// ---- quién pinta la hoja ----

// armazonEn busca un Armazon dentro de `o`, bajando por los contenedores. Lo usa
// `BarraSuite.Montar` para saber si la hoja la pinta el armazón de la app —que es quien
// sabe dónde acaba su menú— o tiene que pintarla él.
//
// ponytail: baja por contenedores y desplazamientos, no por los hijos de un widget
// propio. Las nueve apps con armazón lo cuelgan de un contenedor de la raíz; si una lo
// escondiera dentro de un widget, saldrían dos hojas y se vería a la primera captura.
func armazonEn(o fyne.CanvasObject) *Armazon {
	switch v := o.(type) {
	case *Armazon:
		return v
	case *fyne.Container:
		for _, h := range v.Objects {
			if a := armazonEn(h); a != nil {
				return a
			}
		}
	case *container.Scroll:
		return armazonEn(v.Content)
	}
	return nil
}

// lateralEn busca el menú lateral de la suite dentro de lo que una app cuelga como
// menú del armazón: a veces es el `Lateral` y a veces va dentro de un desplazamiento.
func lateralEn(o fyne.CanvasObject) *Lateral {
	switch v := o.(type) {
	case *Lateral:
		return v
	case *fyne.Container:
		for _, h := range v.Objects {
			if l := lateralEn(h); l != nil {
				return l
			}
		}
	case *container.Scroll:
		return lateralEn(v.Content)
	}
	return nil
}
