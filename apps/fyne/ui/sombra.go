// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # La sombra se pinta en una IMAGEN, y las otras tres vías están medidas y descartadas
//
// Medido contra Fyne v2.8.1 con el pintor de software el 2026-09-21. Las tres
// alternativas existen de verdad; ninguna sirve, y por motivos distintos:
//
//  1. **`canvas.Shader` (GLSL) NO LO PINTA EL PINTOR DE SOFTWARE.** No entra en pánico:
//     no hace nada. El `switch` de `internal/painter/software/painter.go` tiene caso
//     para Image, Text, Circle, Line, Raster, Rectangle, Blur, Arc, BezierCurve y
//     Ellipse — y ninguno para `*canvas.Shader`. Comprobado pintando un shader que
//     devuelve rojo opaco en un lienzo de 50×50: **0 píxeles rojos de 10 000**, el
//     centro sigue blanco. O sea que un shader es invisible sin GPU, y con él la suite
//     no se vería en ningún sitio sin acelerador — ni los guardianes de píxel, que
//     corren a propósito sin CGO y sin servidor gráfico (`docs/CONVENTIONS.md` §13.6).
//
//  2. **`canvas.Rectangle.Shadow` —la sombra de serie, que SÍ existe desde 2.8 y SÍ la
//     pinta el pintor de software— cambia de tamaño con el DPI.** El pintor de software
//     la resuelve con `blur.Gaussian(raw, BlurRadius × escala)` de bild, cuyo «radio» es
//     en realidad σ²/2, así que sale σ = √(2·BlurRadius/escala): la sombra ENCOGE cuando
//     sube la densidad. Medido, con BlurRadius 24: σ de 7,00 px a ×1, 5,00 a ×2 y 4,00 a
//     ×3. Una sombra que mide distinto en un portátil Retina que en un monitor externo
//     no es paridad a píxel.
//
//     Y encima no llega: para el σ que pide la hoja haría falta BlurRadius = escala·r²/8
//     —288 para el `shadow-xl`—, y eso son **4 841 ms por pintado** en el mismo banco
//     donde esta implementación tarda 4,6.
//
//     Hay además un desacuerdo entre los dos pintores de Fyne que conviene saber: el de
//     GL no hace una gaussiana, hace `smoothstep` sobre la distancia con la forma metida
//     hacia dentro `BlurRadius/2` (`shaders/round_rectangle.frag`). El mismo número da
//     dos sombras distintas según haya GPU o no.
//
//  3. Un `canvas.Raster` se redibuja por callback en cada pintado. Aquí la sombra solo
//     cambia cuando cambia el tamaño del panel, así que se genera una vez y se reutiliza.
//
// Lo que queda es lo que hacen los navegadores: **tres desenfoques de caja sobre un
// plano de alfa**, que aproximan una gaussiana con error por debajo del 3 % y cuestan lo
// mismo sea cual sea σ. Medido contra la gaussiana teórica: error 0,0 % en σ = 4, 6, 8 y
// 12, y 4,2 % en σ = 24 (cuantización de las cajas a entero). El resultado es un
// `canvas.Image`, que los DOS pintores tratan igual — no hay camino de reserva que
// mantener ni dos aspectos que reconciliar, y no hace falta GPU, ni CGO, ni una versión
// mínima de OpenGL.

// Sombra es el `box-shadow` de un panel: una imagen que se pone DETRÁS de él.
//
// Va detrás y no dentro porque los paneles de la suite recortan (`IsClip`, que es el
// `overflow-hidden` de la web) y un recorte se lleva por delante justo lo que sobresale,
// que es toda la sombra. Quien la use la coloca de hermano, antes del panel.
type Sombra struct {
	nivel tema.NivelSombra
	// propias son unas capas que no salen de la hoja; nil = las del nivel.
	propias func(tema.Variante) []tema.CapaSombra
	capas   []tema.CapaSombra
	radio   float32
	img     *canvas.Image

	// lo último que se generó, para no rehacer la imagen en cada Layout
	ancho, alto float32
	variante    tema.Variante
	hecha       bool
	margen      float32
	// sinTamano: la escondió Ajustar porque no había tamaño, y la vuelve a enseñar
	// ella. Un Hide() de quien la lleva no lo toca.
	sinTamano   bool
	sinInterior bool
}

// NuevaSombraCon es NuevaSombra con unas capas PROPIAS en vez de un nivel de la hoja.
//
// Hace falta para las sombras que `theme.css` no declara como escalón: la del folio de
// Docus es `--editor-page-shadow` —un local de `editor.css`, dos capas y con otra tinta
// (15,23,42 en vez de 17,35,43)—, así que no es ninguno de los cinco. Redondearla al
// más parecido cambiaría el aspecto del papel, que es lo primero que se mira.
func NuevaSombraCon(capas func(tema.Variante) []tema.CapaSombra, radio float32) *Sombra {
	s := NuevaSombra(tema.SombraXS, radio)
	s.propias = capas
	return s
}

// NuevaSombra crea la sombra de un panel con ese nivel de elevación y ese radio de
// esquina. No genera nada todavía: la imagen se hace en el primer Ajustar, cuando ya se
// sabe cuánto mide el panel.
func NuevaSombra(n tema.NivelSombra, radio float32) *Sombra {
	s := &Sombra{radio: radio, img: canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 1, 1)))}
	s.img.FillMode = canvas.ImageFillStretch
	s.nivel = n
	return s
}

// SinInterior recorta el interior del panel, como un box-shadow exterior de CSS.
// Se necesita cuando el relleno deja ver el fondo: la sombra debe verse fuera del
// panel, no a través de su superficie. No cambia sus capas ni su alcance exterior.
func (s *Sombra) SinInterior() *Sombra {
	if s.sinInterior {
		return s
	}
	s.sinInterior = true
	s.hecha = false
	return s
}

// CambiarRadio invalida la imagen cuando cambia la forma del panel. Ajustar la
// regenerará con las mismas capas y el mismo alcance; el objeto se conserva.
func (s *Sombra) CambiarRadio(radio float32) {
	if s.radio == radio {
		return
	}
	s.radio = radio
	s.hecha = false
}

// Objeto es la imagen, para meterla en los `Objects()` de un renderer ANTES del panel.
func (s *Sombra) Objeto() fyne.CanvasObject { return s.img }

// Ajustar coloca la sombra detrás de un panel que ocupa `tam` en `pos`, regenerando la
// imagen solo si algo de lo que la define ha cambiado.
//
// Solo la ENSEÑA si la había escondido ella por no tener tamaño: la visibilidad es de
// quien la lleva. Antes la encendía siempre, y un `Hide()` de quien la usa duraba hasta
// el siguiente Layout: la barra de acciones de Rooms, apagada, dejaba su sombra pintada
// junto a cada mensaje, y la de una tarjeta de Drive salía sin pasar por encima (visto
// en una ventana real el 2026-09-27; el pintor de software no las pinta y los tests no
// lo veían).
func (s *Sombra) Ajustar(pos fyne.Position, tam fyne.Size, v tema.Variante) {
	if tam.Width <= 0 || tam.Height <= 0 {
		s.img.Hide()
		s.sinTamano = true
		return
	}
	if s.sinTamano {
		s.img.Show()
		s.sinTamano = false
	}
	if !s.hecha || tam.Width != s.ancho || tam.Height != s.alto || v != s.variante {
		s.generar(tam, v)
	}
	s.img.Move(fyne.NewPos(pos.X-s.margen, pos.Y-s.margen))
	s.img.Resize(fyne.NewSize(tam.Width+2*s.margen, tam.Height+2*s.margen))
}

// generar pinta la imagen de la sombra: un plano de alfa por capa, desenfocado y teñido,
// compuesto de atrás hacia delante.
func (s *Sombra) generar(tam fyne.Size, v tema.Variante) {
	s.capas = tema.Sombra(s.nivel, v)
	if s.propias != nil {
		s.capas = s.propias(v)
	}
	s.ancho, s.alto, s.variante, s.hecha = tam.Width, tam.Height, v, true

	// El margen tiene que dar para el desenfoque MÁS el desplazamiento de la capa que
	// más se salga. 3σ deja por debajo del 0,3 % del alfa, que sobre un 0,18 es nada.
	s.margen = 0
	for _, c := range s.capas {
		alcance := float32(math.Ceil(float64(3*c.Desviacion()))) + max(abs(c.DesplX), abs(c.DesplY))
		s.margen = max(s.margen, alcance)
	}

	w := int(math.Ceil(float64(tam.Width + 2*s.margen)))
	h := int(math.Ceil(float64(tam.Height + 2*s.margen)))
	out := image.NewNRGBA(image.Rect(0, 0, w, h))

	// De atrás hacia delante: en CSS la PRIMERA sombra de la lista se pinta ENCIMA.
	for i := len(s.capas) - 1; i >= 0; i-- {
		c := s.capas[i]
		plano := planoRedondeado(w, h, s.margen+c.DesplX, s.margen+c.DesplY, tam.Width, tam.Height, s.radio)
		if desv := c.Desviacion(); desv > 0 {
			plano = desenfocar(plano, w, h, float64(desv))
		}
		componer(out, plano, c.Color)
	}

	if s.sinInterior {
		interior := planoRedondeado(w, h, s.margen, s.margen, tam.Width, tam.Height, s.radio)
		for i, cobertura := range interior {
			out.Pix[4*i+3] = uint8(float32(out.Pix[4*i+3])*(1-cobertura) + 0.5)
		}
	}

	s.img.Image = out
	s.img.Refresh()
}

// planoRedondeado rasteriza un rectángulo de esquinas redondeadas en un plano de
// cobertura de 0 a 1, con antialias por distancia con signo: la cobertura de un píxel es
// cuánto de su centro cae dentro, suavizado en la franja de un píxel del borde.
func planoRedondeado(w, h int, x0, y0, rw, rh, radio float32) []float32 {
	plano := make([]float32, w*h)
	radio = min(radio, min(rw, rh)/2)
	// centro y semiejes del rectángulo, para medir la distancia desde el centro
	cx, cy := float64(x0+rw/2), float64(y0+rh/2)
	bx, by := float64(rw/2-radio), float64(rh/2-radio)
	r := float64(radio)

	for y := range h {
		for x := range w {
			// distancia con signo al rectángulo redondeado (negativa dentro)
			dx := math.Abs(float64(x)+0.5-cx) - bx
			dy := math.Abs(float64(y)+0.5-cy) - by
			var d float64
			switch {
			case dx > 0 && dy > 0:
				d = math.Hypot(dx, dy) - r
			default:
				d = math.Max(dx, dy) - r
			}
			plano[y*w+x] = float32(math.Min(math.Max(0.5-d, 0), 1))
		}
	}
	return plano
}

// componer pinta un plano de alfa del color dado sobre la imagen, con la regla de
// siempre (source-over) y en sRGB, que es como compone Fyne y como compone el navegador.
func componer(dst *image.NRGBA, plano []float32, c color.NRGBA) {
	base := float32(c.A) / 255
	for i, cob := range plano {
		a := cob * base
		if a <= 0 {
			continue
		}
		p := i * 4
		da := float32(dst.Pix[p+3]) / 255
		na := a + da*(1-a)
		if na <= 0 {
			continue
		}
		mez := func(nueva, vieja uint8) uint8 {
			return uint8((float32(nueva)*a+float32(vieja)*da*(1-a))/na + 0.5)
		}
		dst.Pix[p] = mez(c.R, dst.Pix[p])
		dst.Pix[p+1] = mez(c.G, dst.Pix[p+1])
		dst.Pix[p+2] = mez(c.B, dst.Pix[p+2])
		dst.Pix[p+3] = uint8(na*255 + 0.5)
	}
}

// # El desenfoque: tres cajas, como el navegador
//
// Una caja móvil cuesta lo mismo con σ = 1 que con σ = 24 —dos sumas por píxel—, y tres
// seguidas convergen a una gaussiana por el teorema del límite central. Es lo que usan
// los motores de navegador para `box-shadow`, así que copiarlo es copiar la referencia.

// cajasPara reparte una gaussiana de desviación σ en n cajas enteras (aproximación de
// Wells): las primeras `m` de un ancho y el resto uno más, para clavar el σ pedido.
func cajasPara(sigma float64, n int) []int {
	ideal := math.Sqrt(12*sigma*sigma/float64(n) + 1)
	bajo := int(math.Floor(ideal))
	if bajo%2 == 0 {
		bajo--
	}
	if bajo < 1 {
		bajo = 1
	}
	alto := bajo + 2
	mIdeal := (12*sigma*sigma - float64(n*bajo*bajo) - float64(4*n*bajo) - float64(3*n)) /
		(-4*float64(bajo) - 4)
	m := int(math.Round(mIdeal))

	cajas := make([]int, n)
	for i := range cajas {
		if i < m {
			cajas[i] = bajo
		} else {
			cajas[i] = alto
		}
	}
	return cajas
}

// cajaH desenfoca cada fila con una caja de radio r por suma móvil: el borde se extiende
// (se repite el píxel de los extremos), que es lo que evita que la sombra se aclare al
// llegar al borde de la imagen.
func cajaH(src, dst []float32, w, h, r int) {
	if r <= 0 {
		copy(dst, src)
		return
	}
	inv := float32(1) / float32(2*r+1)
	for y := range h {
		fila := y * w
		var suma float32
		for i := -r; i <= r; i++ {
			suma += src[fila+min(max(i, 0), w-1)]
		}
		for x := range w {
			dst[fila+x] = suma * inv
			suma += src[fila+min(x+r+1, w-1)] - src[fila+max(x-r, 0)]
		}
	}
}

func transponer(src, dst []float32, w, h int) {
	for y := range h {
		for x := range w {
			dst[x*h+y] = src[y*w+x]
		}
	}
}

// desenfocar aplica las tres cajas en horizontal, transpone, las aplica otra vez y
// destranspone. Transponer y reutilizar `cajaH` sale más rápido que escribir la versión
// vertical: recorre memoria seguida en los dos ejes.
func desenfocar(plano []float32, w, h int, sigma float64) []float32 {
	cajas := cajasPara(sigma, 3)

	a, b := plano, make([]float32, len(plano))
	for _, c := range cajas {
		cajaH(a, b, w, h, (c-1)/2)
		a, b = b, a
	}

	t := make([]float32, len(plano))
	transponer(a, t, w, h)
	a, b = t, make([]float32, len(plano))
	for _, c := range cajas {
		cajaH(a, b, h, w, (c-1)/2)
		a, b = b, a
	}

	out := make([]float32, len(plano))
	transponer(a, out, h, w)
	return out
}

// DesenfocarImagen es el `blur(σ)` de CSS sobre una imagen OPACA, con σ en píxeles de la
// imagen: las mismas tres cajas de la sombra, canal a canal, con el borde extendido. Es
// lo que hace falta para un `backdrop-blur-*` de la web, que se pinta desenfocando lo que
// queda DEBAJO de la caja.
//
// No es `canvas.Blur`, que existe desde Fyne 2.8 y lo pintan los dos pintores, porque no
// llega: su gaussiana tiene σ = √(2·Radius·escala) px de dispositivo solo hasta
// Radius·escala = 50 —σ 10, por debajo de los 12 de `backdrop-blur-md` a ×1—, y por
// encima el de GL estira sus 101 muestras (σ ≈ Radius·escala/5) mientras el de software
// sigue con la raíz: el mismo número da dos desenfoques. Medido leyendo
// `internal/painter/{gl,software}/draw.go` del fork v2.8.1-sytue.3.
func DesenfocarImagen(src *image.NRGBA, sigma float64) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w == 0 || h == 0 {
		return out
	}
	plano := make([]float32, w*h)
	for c := range 3 {
		for y := range h {
			for x := range w {
				plano[y*w+x] = float32(src.Pix[src.PixOffset(src.Rect.Min.X+x, src.Rect.Min.Y+y)+c])
			}
		}
		for i, v := range desenfocar(plano, w, h, sigma) {
			out.Pix[i*4+c] = uint8(min(max(v+0.5, 0), 255))
		}
	}
	for i := 3; i < len(out.Pix); i += 4 {
		out.Pix[i] = 255
	}
	return out
}
