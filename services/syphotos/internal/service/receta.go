package service

import (
	"image"
	"image/draw"
	"math"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// aplicarReceta produce la imagen que describe la receta. ES EL MISMO CÁLCULO
// que hace el navegador (apps/frontend/apps/photos/src/editor/motor.ts): si
// difieren, lo que se descarga no es lo que se vio. Las fórmulas, de allí:
//
//	exposición   ×2^valor
//	contraste    (c−0,5)·(1+v)+0,5
//	saturación   mezcla con la luminancia Rec.709
//	temperatura  R·(1+0,15v), B·(1−0,15v)
//
// En el orden de la receta; la geometría se aplica al vuelo y el color se
// acumula en UNA pasada de píxeles al final, con el recorte a 0..1 solo al
// terminar — como en el navegador, que tampoco recorta entre pasos.
//
// ponytail: el color se aplica en float por píxel, sin tablas; una foto de
// 24 Mpx son ~100 ms. Si pesa, precomputar LUT por canal para los pasos que lo
// admitan (todos menos saturación, que mezcla canales).
func aplicarReceta(src image.Image, r domain.Receta) image.Image {
	if len(r.Pasos) == 0 {
		return src
	}
	img := src
	var color []domain.Paso
	for _, p := range r.Pasos {
		switch p.Tipo {
		case domain.PasoRecorte:
			img = recortar(img, p.Params["x"], p.Params["y"], p.Params["ancho"], p.Params["alto"])
		case domain.PasoGiro:
			img = girar(img, int(math.Round(p.Params["grados"])))
		case domain.PasoEspejo:
			img = espejar(img, p.Eje != "v")
		default:
			color = append(color, p)
		}
	}
	if len(color) == 0 {
		return img
	}
	return colorear(img, color)
}

// recortar toma la fracción [x,y,ancho,alto] de la imagen, redondeada a píxel
// igual que el navegador (Math.round), y nunca menos de 1×1.
func recortar(src image.Image, x, y, ancho, alto float64) image.Image {
	b := src.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	sx := int(math.Round(x * w))
	sy := int(math.Round(y * h))
	sw := max(1, int(math.Round(ancho*w)))
	sh := max(1, int(math.Round(alto*h)))
	dst := image.NewNRGBA(image.Rect(0, 0, sw, sh))
	draw.Draw(dst, dst.Bounds(), src, image.Pt(b.Min.X+sx, b.Min.Y+sy), draw.Src)
	return dst
}

// colorear aplica los pasos de color, en orden, a cada píxel, y recorta al final.
func colorear(src image.Image, pasos []domain.Paso) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
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
func ajustarColor(r, g, b float64, p domain.Paso) (float64, float64, float64) {
	v := p.Params["valor"]
	switch p.Tipo {
	case domain.PasoExposicion:
		k := math.Pow(2, v)
		return r * k, g * k, b * k
	case domain.PasoContraste:
		k := 1 + v
		return (r-0.5)*k + 0.5, (g-0.5)*k + 0.5, (b-0.5)*k + 0.5
	case domain.PasoSaturacion:
		l := luminancia(r, g, b)
		k := 1 + v
		return l + (r-l)*k, l + (g-l)*k, l + (b-l)*k
	case domain.PasoTemperatura:
		return r * (1 + 0.15*v), g, b * (1 - 0.15*v)
	// LOS GEOMÉTRICOS NO SON DE COLOR y por eso no están: recorte, giro y espejo
	// mueven píxeles de sitio y se aplican antes, sobre el lienzo entero. Es un
	// `default` y no un hueco: sin él, añadir un paso nuevo de color se olvida
	// aquí en silencio y la foto sale sin ese ajuste.
	case domain.PasoRecorte, domain.PasoGiro, domain.PasoEspejo:
		return r, g, b
	}
	return r, g, b
}

// luminancia Rec.709, la misma que el navegador.
func luminancia(r, g, b float64) float64 { return 0.2126*r + 0.7152*g + 0.0722*b }

// a8 recorta a 0..1 y redondea a 8 bits (Math.round en el navegador).
func a8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return uint8(math.Round(v * 255))
}
