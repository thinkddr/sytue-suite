// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// webBuscador es lo que PINTA el buscador de la barra en producción (Sytue Mail,
// v3.6.23, 1440×900 a ×2, medido el 2026-09-25 con Playwright en el piloto): el píxel
// de la captura, no el valor declarado en la hoja. Con la paleta petróleo (2026-10-04)
// los ROLES son los mismos que se midieron y los valores, los de la hoja nueva, uno a
// uno: superficie, `chrome-border` sobre ella, `text-subtle`, `text`, `on-chrome-muted` y
// el aro de `focus-ring`.
//
// Y lo que pinta no es lo que dice su comentario: `GlobalSearch.tsx` pide `bg-chrome-hover
// text-on-chrome placeholder:text-on-chrome-muted`, pero `controlBase` de Form.tsx trae
// `bg-surface text-text placeholder:text-text-subtle` y, entre dos utilidades de la misma
// propiedad, manda el orden de la hoja generada. Resultado medido: campo de SUPERFICIE,
// letra y marca de agua de superficie, y solo el borde (`chrome-border`, que no choca con
// `border-border-strong` por el orden) y la lupa (`text-on-chrome-muted`, en el icono y
// no en el campo) son del cromo. El aro es el de `sytue-focus-ring`, no el del cromo.
type webBuscador struct {
	relleno, borde, bordeEncima, aro, marca, texto, lupa color.NRGBA
}

var buscadorWeb = map[string]webBuscador{
	"claro": {
		// El borde es rgba(15, 29, 32, .12) sobre el blanco.
		relleno: hex(0xffffff), borde: hex(0xe2e4e4), bordeEncima: hex(0x586a6f),
		aro: hex(0x0f7680), marca: hex(0x586a6f), texto: hex(0x0f1d20), lupa: hex(0x43555a),
	},
	"oscuro": {
		// El borde es rgba(255, 255, 255, .22) sobre la superficie #131c1e.
		relleno: hex(0x131c1e), borde: hex(0x474e4f), bordeEncima: hex(0x8a9ea1),
		aro: hex(0x7fd3d8), marca: hex(0x8a9ea1), texto: hex(0xe7efef), lupa: hex(0xa6b8ba),
	},
}

// hayTinta dice si en ese rectángulo de dispositivo hay algún píxel de ese color: el
// núcleo de un trazo sale macizo a ×2, el borde del trazo no, así que basta con uno.
func hayTinta(img image.Image, r image.Rectangle, c color.NRGBA, tol int) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if cercaDe(px(img, x, y), c, tol) {
				return true
			}
		}
	}
	return false
}

// TestElBuscadorDeLaBarraPintaLoQueLaWeb compara, píxel a píxel, el buscador de la barra
// con el de producción en los cuatro estados y los dos temas. Es la queja del dueño del
// 2026-09-25: relleno gris azulado (#b5c4ca), un borde blanco grueso y la marca de agua
// casi invisible. VISTO EN ROJO antes del arreglo, con las cifras de la tabla de la PR.
//
// Con `SYTUE_CAPTURA_BUSCADOR=/dir` deja `fyne-<tema>-<estado>.png`, recortadas igual
// que las de la web (12 px de CSS a los lados, 8 arriba y abajo).
func TestElBuscadorDeLaBarraPintaLoQueLaWeb(t *testing.T) {
	t.Cleanup(func() { tema.Elegir("") })
	for _, variante := range []string{"claro", "oscuro"} {
		web := buscadorWeb[variante]
		for _, estado := range []string{"reposo", "encima", "foco", "texto"} {
			t.Run(variante+"/"+estado, func(t *testing.T) {
				tema.Elegir(variante)
				m, b := cromoPrueba(t, anchoPrueba, 120)
				campo := b.buscador.campo
				pinta(t, anchoPrueba, 120, m) // monta y coloca antes de tocar el estado
				switch estado {
				case "encima":
					campo.MouseIn(nil)
				case "foco":
					campo.FocusGained()
				case "texto":
					campo.FocusGained()
					campo.PonerTexto("fx")
				}
				img := pinta(t, anchoPrueba, 120, m)

				p, s := campo.Position(), campo.Size()
				x0, y0 := int(p.X*escala), int(p.Y*escala)
				x1, y1 := int((p.X+s.Width)*escala), int((p.Y+s.Height)*escala)
				cx := (x0+x1)/2 + int(60*escala) // a la derecha de la marca de agua
				// El fondo de la barra, a la izquierda del campo y fuera de la esquina de marca.
				fondo := px(img, x0-int(8*escala), (y0+y1)/2)
				guardarBuscador(t, img, variante, estado, x0-int(12*escala), y0-int(8*escala), x1+int(12*escala), y1+int(8*escala))

				mira := func(que string, got, want color.NRGBA, tol int) {
					t.Helper()
					if !cercaDe(got, want, tol) {
						t.Errorf("%s: pinta %s y la web %s", que, aHex(got), aHex(want))
					}
				}
				mira("relleno", px(img, cx, (y0+y1)/2), web.relleno, 1)
				borde := web.borde
				if estado == "encima" {
					borde = web.bordeEncima
				}
				mira("borde", px(img, cx, y0), borde, 1)
				mira("borde (2.ª fila)", px(img, cx, y0+1), borde, 1)
				mira("dentro del borde", px(img, cx, y0+2), web.relleno, 1)
				if estado == "foco" || estado == "texto" {
					// `box-shadow: 0 0 0 2px`: cuatro filas de dispositivo por fuera del borde.
					for d := 1; d <= 4; d++ {
						mira(fmt.Sprintf("aro (fila −%d)", d), px(img, cx, y0-d), web.aro, 1)
					}
					mira("fuera del aro", px(img, cx, y0-5), fondo, 1)
				} else {
					mira("fuera del borde", px(img, cx, y0-1), fondo, 1)
				}

				// La lupa en `left-3` (12 px de CSS del borde de la caja), de 16.
				lupa := image.Rect(x0+int(12*escala), y0, x0+int(28*escala), y1)
				if !hayTinta(img, lupa, web.lupa, 3) {
					t.Errorf("la lupa no tiene ni un píxel de %s", aHex(web.lupa))
				}
				// El texto empieza en `pl-9` (31,5 con el rem de 14).
				letra := image.Rect(x0+int(30*escala), y0+2, x0+int(140*escala), y1-2)
				if estado == "texto" {
					if !hayTinta(img, letra, web.texto, 3) {
						t.Errorf("el texto escrito no tiene ni un píxel de %s", aHex(web.texto))
					}
				} else if !hayTinta(img, letra, web.marca, 3) {
					t.Errorf("la marca de agua no tiene ni un píxel de %s", aHex(web.marca))
				}
			})
		}
	}
}

func aHex(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func guardarBuscador(t *testing.T, img image.Image, variante, estado string, x0, y0, x1, y1 int) {
	t.Helper()
	dir := os.Getenv("SYTUE_CAPTURA_BUSCADOR")
	if dir == "" {
		return
	}
	sub := img.(interface {
		SubImage(image.Rectangle) image.Image
	}).SubImage(image.Rect(x0, y0, x1, y1))
	f, err := os.Create(filepath.Join(dir, "fyne-"+variante+"-"+estado+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, sub); err != nil {
		t.Fatal(err)
	}
}
