// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Las FILAS JUSTIFICADAS de la galería: el `.filas` de
// apps/frontend/apps/photos/src/styles/app.css y el `proporcion()` de lib/filas.ts.
// Cada foto conserva su proporción, las filas llenan el ancho y la última no se estira.
//
// En la web es flexbox sin una línea de JavaScript, así que aquí no hay nada que
// inventar: se hace LA MISMA CUENTA que el navegador. Cada tesela entra con una base
// de `--r × --fila`; una fila se cierra cuando la siguiente base ya no cabe; el
// sobrante se reparte en proporción a la base, que es lo mismo que subir el alto de
// la fila; y la última no se estira porque un `::after` se queda con su sobrante.

// Los topes de la proporción, de lib/filas.ts: 9:20 es la captura de un móvil de hoy
// y 5:2 deja entrar el 21:9. Solo lo que se sale de ahí —una panorámica, una tira
// vertical— se recorta.
const (
	ProporcionMin float32 = 9.0 / 20
	ProporcionMax float32 = 5.0 / 2
)

// Los números de `.filas`.
const (
	// HuecoFilas es su `gap: 0.25rem`, entre teselas y entre filas.
	HuecoFilas = 1 * 0.25 * tema.Rem
	// CabenEnFila es el 3,6 de `(100cqw − hueco) / 3.6`: en un teléfono, el alto con el
	// que dos fotos 16:9 caben a la par (2 × 1,78 = 3,56).
	CabenEnFila float32 = 3.6
)

// Proporcion es `proporcion()`: la de la foto, acotada, y un cuadrado si el índice no
// supo medirla.
func Proporcion(f api.Photo) float32 {
	if f.Width <= 0 || f.Height <= 0 {
		return 1
	}
	return min(max(float32(f.Width)/float32(f.Height), ProporcionMin), ProporcionMax)
}

// AltoFila es `--fila`, el alto MÍNIMO de una fila: el tope del corte de VENTANA
// (`--fila-max`: 9, 11 y 13 rem, los mismos escalones que tenía la tesela cuadrada) o,
// si la rejilla es estrecha, lo que deja caber dos 16:9. Lo segundo depende del ancho
// de la REJILLA (`cqw`) y no del de la ventana.
func AltoFila(ventana, ancho float32) float32 {
	tope := float32(9 * tema.Rem)
	switch {
	case ventana >= ui.CorteXL:
		tope = 13 * tema.Rem
	case ventana >= ui.CorteSM:
		tope = 11 * tema.Rem
	}
	return min(tope, (ancho-HuecoFilas)/CabenEnFila)
}

// Filas es el layout de `.filas`. Razones lleva el `--r` de cada hijo, en su orden.
type Filas struct {
	Razones []float32
	// Ventana es el ancho de la ventana: los topes son media queries.
	Ventana float32
}

var _ fyne.Layout = Filas{}

// repartir hace la cuenta de flexbox a ese ancho: la caja de cada tesela y el alto
// total.
func (f Filas) repartir(ancho float32) (cajas []fyne.Position, medidas []fyne.Size, alto float32) {
	n := len(f.Razones)
	cajas, medidas = make([]fyne.Position, n), make([]fyne.Size, n)
	if n == 0 || ancho <= 0 {
		return cajas, medidas, 0
	}
	base := AltoFila(f.Ventana, ancho)
	// Un sesentaicuatroavo de holgura: el navegador cuenta en esa rejilla, y sin ella
	// una fila que cabe justa saltaría por el redondeo de la coma flotante.
	const holgura = ui.LayoutUnit
	y := float32(0)
	for i := 0; i < n; {
		j, suma := i, float32(0)
		for j < n {
			b := f.Razones[j] * base
			if j > i && suma+b+HuecoFilas*float32(j-i) > ancho+holgura {
				break
			}
			suma += b
			j++
		}
		huecos := HuecoFilas * float32(j-i-1)
		// La última fila no se estira: el `::after` se lleva el sobrante. Pero el
		// `::after` también es un hijo, con su hueco delante: si ni ese hueco cabe,
		// salta él solo a otra fila —de alto cero— y esta se estira como las demás.
		ultima := j == n && suma+huecos+HuecoFilas <= ancho+holgura
		k := float32(1)
		if !ultima {
			// Estirar hasta llenar o, si es una sola más ancha que la rejilla, encoger.
			k = (ancho - huecos) / suma
		}
		x := float32(0)
		for c := i; c < j; c++ {
			w := f.Razones[c] * base * k
			cajas[c], medidas[c] = fyne.NewPos(x, y), fyne.NewSize(w, base*k)
			x += w + HuecoFilas
		}
		y += base * k
		if j < n || !ultima {
			y += HuecoFilas
		}
		i = j
	}
	return cajas, medidas, y
}

// AltoCon es lo que ocupa a ese ancho, que es la cuenta de verdad.
func (f Filas) AltoCon(_ []fyne.CanvasObject, ancho float32) float32 {
	_, _, alto := f.repartir(ancho)
	return alto
}

// MinSize es el de fyne.Layout: sin ancho no hay filas que contar, así que da el de
// una tesela cuadrada en su fila más baja. Quien quiera el de verdad llama a AltoCon.
func (f Filas) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(9*tema.Rem*ProporcionMin, 9*tema.Rem)
}

// Layout coloca las teselas.
func (f Filas) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	cajas, medidas, _ := f.repartir(t.Width)
	for i, o := range objs {
		if i >= len(cajas) {
			break
		}
		o.Move(cajas[i])
		o.Resize(medidas[i])
	}
}

// filasDe arma el layout para esas fotos.
func filasDe(v Vista, fotos []api.Photo) Filas {
	r := make([]float32, len(fotos))
	for i, f := range fotos {
		r[i] = Proporcion(f)
	}
	return Filas{Razones: r, Ventana: v.Ventana}
}
