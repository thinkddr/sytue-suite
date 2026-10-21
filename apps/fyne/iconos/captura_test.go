// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLaminaDeIlustraciones pinta las doce en una lámina y, con SYTUE_CAPTURA, la deja
// en un PNG. Es para MIRARLA: los números dicen que hay tinta, no que el dibujo sea el
// de la web.
func TestLaminaDeIlustraciones(t *testing.T) {
	ruta := os.Getenv("SYTUE_CAPTURA")
	if ruta == "" {
		t.Skip("sin SYTUE_CAPTURA: esto es para sacar la lámina, no un guardián")
	}
	const cols, w, h = 4, 200, 160
	filas := (int(numIlustraciones) + cols - 1) / cols
	lamina := image.NewNRGBA(image.Rect(0, 0, cols*w, filas*h))
	draw.Draw(lamina, lamina.Bounds(), &image.Uniform{tema.Color("bg", tema.Claro)}, image.Point{}, draw.Src)
	for i := range int(numIlustraciones) {
		il := Ilustracion(i)
		m := il.MedidaNatural()
		img := rasteriza(conTokens(il.leer(), tema.Claro), int(m.Width), int(m.Height))
		x, y := (i%cols)*w, (i/cols)*h
		draw.Draw(lamina, image.Rect(x, y, x+int(m.Width), y+int(m.Height)), img, image.Point{}, draw.Over)
	}
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, lamina); err != nil {
		t.Fatal(err)
	}
}
