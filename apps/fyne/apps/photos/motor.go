// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"image"
	"image/draw"
	"math"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El motor del editor: aplica una receta a una imagen. ES EL MISMO CÁLCULO que hacen el
// navegador (`editor/motor.ts`) y el servicio (`services/syphotos/internal/service/
// receta.go`): si difieren, lo que se descarga no es lo que se vio. Las fórmulas:
//
//	exposición   ×2^valor
//	contraste    (c−0,5)·(1+v)+0,5
//	saturación   mezcla con la luminancia Rec.709
//	temperatura  R·(1+0,15v), B·(1−0,15v)
//
// La geometría se aplica en el orden de la receta y el color se acumula en UNA pasada al
// final, recortando a 0..1 solo al terminar, como hacen los otros dos.
//
// Se reparte como en la web: la geometría solo cuando cambia un paso de geometría (la
// «fuente»), y el color en cada movimiento de un deslizador, sobre la fuente ya
// reducida al tamaño del lienzo en pantalla.

// nrgba devuelve la imagen como NRGBA con origen en 0,0, copiándola si hace falta.
func nrgba(src image.Image) *image.NRGBA {
	if m, ok := src.(*image.NRGBA); ok && m.Rect.Min == (image.Point{}) {
		return m
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// aplicarGeometria aplica, en orden, los recortes, giros y espejos de la receta.
func aplicarGeometria(src image.Image, r api.Receta) *image.NRGBA {
	img := nrgba(src)
	for _, p := range r.Pasos {
		switch p.Tipo {
		case api.PasoRecorte:
			img = recortar(img, p.Params["x"], p.Params["y"], p.Params["ancho"], p.Params["alto"])
		case api.PasoGiro:
			img = girar(img, int(math.Round(p.Params["grados"])))
		case api.PasoEspejo:
			img = espejar(img, p.Eje != "v")
		}
	}
	return img
}

// recortar toma la fracción [x,y,ancho,alto], redondeada a píxel como el navegador
// (`Math.round`) y nunca menos de 1×1.
func recortar(src *image.NRGBA, x, y, ancho, alto float64) *image.NRGBA {
	w, h := float64(src.Rect.Dx()), float64(src.Rect.Dy())
	sx, sy := int(math.Round(x*w)), int(math.Round(y*h))
	sw, sh := max(1, int(math.Round(ancho*w))), max(1, int(math.Round(alto*h)))
	dst := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	draw.Draw(dst, dst.Bounds(), src, image.Pt(sx, sy), draw.Src)
	return dst
}

// girar rota en sentido horario 90, 180 o 270 grados.
func girar(src *image.NRGBA, grados int) *image.NRGBA {
	grados = ((grados % 360) + 360) % 360
	if grados == 0 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if grados != 180 {
		dst = image.NewNRGBA(image.Rect(0, 0, h, w))
	}
	for y := range h {
		for x := range w {
			var dx, dy int
			switch grados {
			case 90:
				dx, dy = h-1-y, x
			case 180:
				dx, dy = w-1-x, h-1-y
			default:
				dx, dy = y, w-1-x
			}
			copy(dst.Pix[dst.PixOffset(dx, dy):][:4], src.Pix[src.PixOffset(x, y):][:4])
		}
	}
	return dst
}

// espejar da la vuelta de izquierda a derecha (horizontal) o de arriba abajo.
func espejar(src *image.NRGBA, horizontal bool) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			sx, sy := w-1-x, y
			if !horizontal {
				sx, sy = x, h-1-y
			}
			copy(dst.Pix[dst.PixOffset(x, y):][:4], src.Pix[src.PixOffset(sx, sy):][:4])
		}
	}
	return dst
}

// colorear aplica los pasos de color de la receta a una copia de `src`. Sin pasos de
// color devuelve la misma imagen.
func colorear(src *image.NRGBA, r api.Receta) *image.NRGBA {
	var pasos []api.PasoReceta
	for _, p := range r.Pasos {
		if !p.EsDeGeometria() {
			pasos = append(pasos, p)
		}
	}
	if len(pasos) == 0 {
		return src
	}
	dst := image.NewNRGBA(src.Rect)
	copy(dst.Pix, src.Pix)
	pix := dst.Pix
	for i := 0; i+3 < len(pix); i += 4 {
		rr, gg, bb := float64(pix[i])/255, float64(pix[i+1])/255, float64(pix[i+2])/255
		for _, p := range pasos {
			rr, gg, bb = ajustarColor(rr, gg, bb, p)
		}
		pix[i], pix[i+1], pix[i+2] = a8(rr), a8(gg), a8(bb)
	}
	return dst
}

// ajustarColor es UN paso de color sobre un píxel en 0..1, sin recortar.
func ajustarColor(r, g, b float64, p api.PasoReceta) (float64, float64, float64) {
	v := p.Params["valor"]
	switch p.Tipo {
	case api.PasoExposicion:
		k := math.Pow(2, v)
		return r * k, g * k, b * k
	case api.PasoContraste:
		k := 1 + v
		return (r-0.5)*k + 0.5, (g-0.5)*k + 0.5, (b-0.5)*k + 0.5
	case api.PasoSaturacion:
		l := 0.2126*r + 0.7152*g + 0.0722*b // Rec.709, la misma que el navegador
		k := 1 + v
		return l + (r-l)*k, l + (g-l)*k, l + (b-l)*k
	case api.PasoTemperatura:
		return r * (1 + 0.15*v), g, b * (1 - 0.15*v)
	}
	return r, g, b
}

// a8 recorta a 0..1 y redondea a 8 bits (`Math.round` en el navegador).
func a8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(math.Round(v * 255))
}

// ---- La receta que tocan los mandos ----

// Deslizadores son los cuatro del editor, en el orden de `DESLIZADORES` de Editor.tsx.
var Deslizadores = [...]string{api.PasoExposicion, api.PasoContraste, api.PasoSaturacion, api.PasoTemperatura}

// valorDe es el valor vigente de un deslizador: el ÚLTIMO paso de su tipo, o 0.
func valorDe(r api.Receta, tipo string) float64 {
	for i := len(r.Pasos) - 1; i >= 0; i-- {
		if r.Pasos[i].Tipo == tipo {
			return r.Pasos[i].Params["valor"]
		}
	}
	return 0
}

// conValor sustituye (o añade) el paso de ese deslizador. A 0 se quita: no ensucia la
// receta.
func conValor(r api.Receta, tipo string, valor float64) api.Receta {
	c := api.Receta{Version: r.Version, Pasos: []api.PasoReceta{}}
	for _, p := range r.Clonar().Pasos {
		if p.Tipo != tipo {
			c.Pasos = append(c.Pasos, p)
		}
	}
	if valor != 0 {
		c.Pasos = append(c.Pasos, api.PasoReceta{Tipo: tipo, Params: map[string]float64{"valor": valor}})
	}
	return c
}

// conPaso añade un paso al final (girar, espejar, recortar).
func conPaso(r api.Receta, p api.PasoReceta) api.Receta {
	c := r.Clonar()
	c.Pasos = append(c.Pasos, p)
	return c
}

// claveGeometria resume los pasos de geometría: la fuente solo se rehace si cambia.
func claveGeometria(r api.Receta) string {
	g := api.Receta{Version: r.Version}
	for _, p := range r.Pasos {
		if p.EsDeGeometria() {
			g.Pasos = append(g.Pasos, p)
		}
	}
	return g.Canonica()
}

// ---- El recorte ----

// Marco es el del recorte, en FRACCIONES del lienzo: es lo que la receta espera y lo que
// sobrevive a cualquier tamaño de pantalla.
type Marco struct{ X, Y, W, H float64 }

// Proporciones son las que ofrece el editor; 0 es libre.
var Proporciones = [...]float64{0, 1, 4.0 / 3, 3.0 / 2, 16.0 / 9}

// conProporcion es la de `editor/proporcion.ts`: ajusta el marco a la proporción EN
// PÍXELES (el lienzo no es cuadrado) creciendo desde el ancla —donde empezó el
// arrastre— y sin salirse; se conserva el lado más pequeño, así nunca crece solo.
func conProporcion(m Marco, proporcion, anchoPx, altoPx, anclaX, anclaY float64) Marco {
	if anchoPx <= 0 || altoPx <= 0 || proporcion <= 0 {
		return m
	}
	wpx, hpx := m.W*anchoPx, m.H*altoPx
	if wpx/max(hpx, 1e-9) > proporcion {
		wpx = hpx * proporcion
	} else {
		hpx = wpx / proporcion
	}
	izquierda := m.X+m.W <= anclaX+1e-9
	arriba := m.Y+m.H <= anclaY+1e-9
	maxW, maxH := (1-anclaX)*anchoPx, (1-anclaY)*altoPx
	if izquierda {
		maxW = anclaX * anchoPx
	}
	if arriba {
		maxH = anclaY * altoPx
	}
	escala := min(1, maxW/max(wpx, 1e-9), maxH/max(hpx, 1e-9))
	w, h := wpx*escala/anchoPx, hpx*escala/altoPx
	x, y := anclaX, anclaY
	if izquierda {
		x = anclaX - w
	}
	if arriba {
		y = anclaY - h
	}
	return Marco{X: x, Y: y, W: w, H: h}
}
