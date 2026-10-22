// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// El `Checkbox` y el `Radio` de Form.tsx son `<input>` NATIVOS con `accent-primary`: los
// pinta Chromium, no la hoja. Las cifras de aquí salen de capturar en producción a ×2 el
// marcado de Form.tsx (2026-09-25, pulido de Fyne), en claro y en oscuro, y leer el píxel.
// Las que dependen del acento se volvieron a medir en Chromium a ×2 con el petróleo
// (2026-10-04): #0f7680 en claro, con tic blanco, y #3cc4cc en oscuro, que por ser un
// acento CLARO lleva el tic y el hueco en #3b3b3b y se aclara con el ratón encima.
// Coordenadas en píxeles de DISPOSITIVO desde la esquina del cuadradito de 14 (28 a ×2).

// cuadradito pinta una casilla o un radio sin etiqueta en ese tema y devuelve la imagen y
// dónde empieza el cuadradito.
func cuadradito(t *testing.T, modo string, e *Eleccion, estado func(*Eleccion)) (image.Image, int) {
	t.Helper()
	tema.Elegir(modo)
	// Los acentos esperados son los de la casa: otra prueba del proceso pudo dejar
	// puesto el producto de su barra (barrasuite → ElegirApp) o un color, y entonces
	// el acento salía violeta según el orden en que corrieran.
	tema.ElegirColor("")
	tema.ElegirApp("")
	t.Cleanup(func() { tema.Elegir(""); tema.ElegirColor(""); tema.ElegirApp("") })
	e.SinEtiqueta = true
	e.ExtendBaseWidget(e)
	if estado != nil {
		estado(e)
	}
	// A la medida del cuadradito y no de la fila (19,25), para que caiga en píxel entero.
	const margen = 6
	return colocado(t, e, fyne.NewSize(LadoEleccion, LadoEleccion), margen), int(margen * escala)
}

func hex(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 255}
}

// Disabled: píxeles medidos en Chromium×2, paleta canónica final, 2026-10-04.
func TestLaCasillaEsLaDeChromium(t *testing.T) {
	encima := func(e *Eleccion) { e.MouseIn(nil) }
	inerte := func(e *Eleccion) { e.Disable() }
	for _, c := range []struct {
		que    string
		modo   string
		e      *Eleccion
		estado func(*Eleccion)
		x, y   int
		quiero uint32
		porque string
	}{
		{"sin marcar, borde", "claro", &Eleccion{}, nil, 0, 14, 0x767676, "el borde de Chromium, no `border-strong` (#839298)"},
		{"sin marcar, fondo", "claro", &Eleccion{}, nil, 14, 6, 0xffffff, "el fondo de Chromium"},
		{"sin marcar, esquina", "claro", &Eleccion{}, nil, 1, 3, 0x767676, "la esquina de 2: con la de 3,5 aquí aún no hay borde"},
		{"encima, borde", "claro", &Eleccion{}, encima, 0, 14, 0x4f4f4f, "el `:hover` del navegador oscurece el borde"},
		{"marcada, tic", "claro", &Eleccion{Marcada: true}, nil, 8, 16, 0xffffff, "el tic de Chromium, de 0,16·l de grueso"},
		{"marcada, relleno", "claro", &Eleccion{Marcada: true}, nil, 14, 6, 0x0f7680, "`accent-primary`"},
		{"marcada encima", "claro", &Eleccion{Marcada: true}, encima, 14, 6, 0x09484e, "el acento con el ratón encima"},
		{"marcada e inerte", "claro", &Eleccion{Marcada: true}, inerte, 14, 6, 0xe8e8e8, "gris, no el acento atenuado"},
		{"marcada e inerte, tic", "claro", &Eleccion{Marcada: true}, inerte, 8, 16, 0xf6f6f6, "el tic apagado"},
		{"a medias, raya", "claro", &Eleccion{Parcial: true}, nil, 14, 14, 0xffffff, "la raya de 0,6·l × 0,16·l"},
		{"a medias, raya oscura", "oscuro", &Eleccion{Parcial: true}, nil, 14, 14, 0x3b3b3b, "la raya va del color de la casilla sin estilo, no del tic"},
		{"radio inerte, hueco", "claro", &Eleccion{Redondo: true, Marcada: true}, inerte, 3, 13, 0xfbfbfb, "apagado, el hueco es el fondo"},
		{"a medias, sobre la raya", "claro", &Eleccion{Parcial: true}, nil, 14, 10, 0x0f7680, "la raya empieza en 0,42·l"},
		{"sin marcar, fondo oscuro", "oscuro", &Eleccion{}, nil, 14, 6, 0x3b3b3b, "Chromium con `color-scheme: dark`"},
		{"sin marcar, borde oscuro", "oscuro", &Eleccion{}, nil, 0, 14, 0x858585, "Chromium con `color-scheme: dark`"},
		{"encima, borde oscuro", "oscuro", &Eleccion{}, encima, 0, 14, 0xacacac, "el `:hover` oscuro aclara"},
		{"marcada, relleno oscuro", "oscuro", &Eleccion{Marcada: true}, nil, 14, 6, 0x3cc4cc, "`accent-primary` en oscuro"},
		{"marcada, tic oscuro", "oscuro", &Eleccion{Marcada: true}, nil, 8, 16, 0x3b3b3b, "acento claro: el tic no es blanco"},
		{"marcada encima, oscuro", "oscuro", &Eleccion{Marcada: true}, encima, 14, 6, 0x68d1d8, "un acento claro se ACLARA con el ratón encima"},
		{"marcada e inerte, oscuro", "oscuro", &Eleccion{Marcada: true}, inerte, 14, 6, 0x444849, "gris, al 50 % sobre la superficie"},
		{"radio, hueco", "claro", &Eleccion{Redondo: true, Marcada: true}, nil, 3, 13, 0xffffff, "el hueco entre anillo y punto"},
		{"radio, punto", "claro", &Eleccion{Redondo: true, Marcada: true}, nil, 14, 14, 0x0f7680, "el punto del acento"},
		{"radio, borde del punto", "claro", &Eleccion{Redondo: true, Marcada: true}, nil, 14, 7, 0x0f7680, "el punto mide 0,6·l: con 0,5·l aquí aún es hueco"},
		{"radio, hueco oscuro", "oscuro", &Eleccion{Redondo: true, Marcada: true}, nil, 3, 13, 0x3b3b3b, "el hueco va del color del tic también en oscuro, y con un acento claro el tic es #3b3b3b"},
		{"radio, fondo oscuro", "oscuro", &Eleccion{Redondo: true}, nil, 14, 14, 0x3b3b3b, "el radio sin marcar de Chromium"},
	} {
		img, o := cuadradito(t, c.modo, c.e, c.estado)
		if got := px(img, o+c.x, o+c.y); !cerca(got, hex(c.quiero)) {
			t.Errorf("%s (%s): el píxel (%d,%d) es %v y la web pinta #%06x: %s", c.que, c.modo, c.x, c.y, got, c.quiero, c.porque)
		}
	}
}

// TestElAroSigueLaFormaDeLaCasilla: el aro de 2 rodea la pieza con su forma, como el
// `sytue-focus-ring-choice` de Form.tsx —esquina de 2 + 2 en la casilla, círculo de 18 en el
// radio— y no el cuadrado de 18 que salía cuando lo llevaba el `<input>` (radio 0). Por
// dentro no pinta nada: donde la pieza no llega se ve el fondo, sea cual sea, y no una tapa
// de `surface`. Coordenadas en píxeles de dispositivo (×2) desde la esquina de la pieza.
func TestElAroSigueLaFormaDeLaCasilla(t *testing.T) {
	for _, c := range []struct {
		redondo bool
		x, y    int
		aro     bool
		que     string
	}{
		{false, -4, -4, false, "la esquina de fuera: el aro de la casilla va redondeado a 4"},
		{false, -2, 14, true, "el lado de la casilla"},
		{true, -4, -4, false, "la esquina de fuera del radio: el aro es un círculo"},
		{true, -2, 14, true, "el lado del radio"},
		{true, 3, 3, true, "en diagonal, entre el círculo de 14 y el de 18"},
		{true, 1, 1, false, "la esquina de dentro del cuadrado, fuera del círculo de 18"},
	} {
		img, o := cuadradito(t, "claro", &Eleccion{Redondo: c.redondo}, func(e *Eleccion) { e.FocusGained() })
		// Después de cuadradito, que fija el color de la casa: leído antes, el anillo
		// esperado salía del color que otra prueba hubiera dejado puesto.
		anillo := tema.Color("focus-ring", tema.Claro)
		if got := px(img, o+c.x, o+c.y); cerca(got, anillo) != c.aro {
			t.Errorf("redondo=%v, píxel (%d,%d) = %v; aro=%v esperado: %s", c.redondo, c.x, c.y, got, c.aro, c.que)
		}
	}
}

// TestElAcentoEncimaEsElDeChromium: la cuenta de AcentoEncima contra los seis acentos
// medidos, cuatro oscuros que oscurecen y dos claros que aclaran. El primario del tema
// oscuro (#3cc4cc, luminancia 0,44) es el que fija el corte: con el de 0,5 de antes
// oscurecía, y Chromium lo aclara.
func TestElAcentoEncimaEsElDeChromium(t *testing.T) {
	for _, c := range [][2]uint32{
		{0x1a3541, 0x0a1419}, {0x1a6fbb, 0x13528a}, // los `accent-primary` de las paletas anteriores
		{0x0f7680, 0x09484e}, {0x3cc4cc, 0x68d1d8}, // `accent-primary` petróleo, claro y oscuro
		{0x0075ff, 0x005cc8}, {0x99c8ff, 0xd1e6ff}, // el azul de serie de Chromium
	} {
		if got := AcentoEncima(hex(c[0])); !cerca(got, hex(c[1])) {
			t.Errorf("AcentoEncima(#%06x) = %v y Chromium pinta #%06x", c[0], got, c[1])
		}
	}
}

// TestLaCasillaDeChromiumNoSeCopia: la tabla del ERP, los comentarios de CertiPDF, las
// tareas de Notepads y los medios de Diapos se escribían cada uno su casilla de Chromium
// —cuatro tics distintos, dos con el `StrokeWidth = 1` que la regla 4 prohíbe— a partir de
// los colores de CasillaNativa. Ahora la pintan con PiezaCasilla, y fuera de `ui` nadie
// tiene por qué volver a leer esos colores: quien los lee, está dibujando otra copia.
func TestLaCasillaDeChromiumNoSeCopia(t *testing.T) {
	var copias []string
	_ = filepath.WalkDir("..", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") ||
			strings.HasPrefix(filepath.ToSlash(ruta), "../ui/") {
			return nil
		}
		datos, _ := os.ReadFile(ruta)
		for i, l := range strings.Split(string(datos), "\n") {
			if l = strings.TrimSpace(l); !strings.HasPrefix(l, "//") && strings.Contains(l, "ui.CasillaNativa(") {
				copias = append(copias, fmt.Sprintf("%s:%d", ruta, i+1))
			}
		}
		return nil
	})
	if len(copias) > 0 {
		t.Errorf("fuera de `ui` se leen los colores de la casilla de Chromium para pintar otra: %v; usa ui.NuevaPiezaCasilla", copias)
	}
}
