// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package iconos

import (
	"bytes"
	"fmt"

	"fyne.io/fyne/v2"
	"github.com/fyne-io/oksvg"
)

// DeSVGAjeno es un dibujo de un SVG que NO es de la suite —el logotipo de una
// aplicación conectada, que llega en el manifiesto de la pasarela—, pintado con las
// mismas correcciones de `oksvg` que las marcas (el `ry`, el `scale` de un número, los
// arcos y el par-impar) y a píxeles de dispositivo.
//
// Dos diferencias con DeSVG, las dos porque el fichero viene de fuera:
//
//   - **No entra en pánico.** Uno ilegible, sin tamaño o que rompa el rasterizador
//     devuelve `false`, y quien llama deja el cuadro vacío, que es lo que hace la web
//     con un `<img>` roto.
//   - **Encaja sin deformar**: un `<img width=28 height=28>` con un SVG que tiene
//     `viewBox` lo pinta con el `preserveAspectRatio` por defecto (`xMidYMid meet`).
//     El dibujo sale ya con la medida encajada en `lado × lado`; centrarlo es de quien
//     lo coloca (ver `Medida`).
func DeSVGAjeno(svg []byte, lado float32) (d *Dibujo, ok bool) {
	limpio := normaliza(svg)
	icono, err := oksvg.ReadIconStream(bytes.NewReader(limpio))
	if err != nil || icono.ViewBox.W <= 0 || icono.ViewBox.H <= 0 {
		return nil, false
	}
	// Una pasada de prueba antes de entregarlo: el pintor de Fyne llama al generador
	// en su propio bucle, y un pánico ahí se lleva la aplicación entera.
	if !rasterizable(svg) {
		return nil, false
	}
	w, h := lado, lado
	if r := float32(icono.ViewBox.W / icono.ViewBox.H); r > 1 {
		h = lado / r
	} else {
		w = lado * r
	}
	return DeSVG(svg, fyne.NewSize(w, h)), true
}

func rasterizable(svg []byte) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
			fyne.LogError("iconos: un SVG de fuera no se deja pintar", fmt.Errorf("%v", r))
		}
	}()
	rasteriza(svg, 16, 16)
	return true
}
