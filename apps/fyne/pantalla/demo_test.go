// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pantalla

import (
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	os.Exit(m.Run())
}

// TestLaPantallaSePintaYSeMide es la prueba de punta a punta de los cimientos: tema +
// widgets propios + layout propio, capturado SIN servidor gráfico y con una medida
// contrastada contra el número de producción.
//
// Con `-captura` deja el PNG donde se le diga; es como se genera la imagen que
// acompaña al PR.
func TestLaPantallaSePintaYSeMide(t *testing.T) {
	w := test.NewWindow(Demo(tema.Claro))
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(Ancho, Alto))
	img := w.Canvas().Capture()

	if b := img.Bounds(); b.Dx() != Ancho || b.Dy() != Alto {
		t.Fatalf("la captura mide %dx%d y el lienzo %dx%d", b.Dx(), b.Dy(), Ancho, Alto)
	}

	px := func(x, y int) color.NRGBA {
		r, g, b, a := img.At(x, y).RGBA()
		return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
	}
	cerca := func(a, b color.NRGBA) bool {
		d := func(x, y uint8) bool { return int(x)-int(y) > 1 || int(y)-int(x) > 1 }
		return !d(a.R, b.R) && !d(a.G, b.G) && !d(a.B, b.B)
	}

	// 1. El papel de fondo es el token, no el gris de serie de Fyne.
	if got, quiero := px(Ancho-2, Alto-2), tema.Color("bg", tema.Claro); !cerca(got, quiero) {
		t.Errorf("el fondo pinta %v y el token `bg` es %v", got, quiero)
	}

	// Card.tsx: p-6=24 con raíz de16; la tarjeta es la superficie blanca.
	// border-subtle rgba(15,29,32,.07) sobre el blanco da (238,239,239) en CSS.
	const margen = 24
	if tema.Espacio(6) != margen {
		t.Fatalf("Espacio(6) = %v, quiero %v", tema.Espacio(6), margen)
	}
	produccion := color.NRGBA{R: 238, G: 239, B: 239, A: 255}
	// A media altura de la tarjeta, fuera de las esquinas redondeadas.
	medio := margen + int(ui.MedidaTarjeta("Cambios sin guardar", ui.NuevoBoton("Guardar cambios", nil)).Height)/2
	if got := px(margen, medio); !cerca(got, produccion) {
		t.Errorf("el borde izquierdo de la tarjeta (%d,%d) pinta %v y en producción es %v", margen, medio, got, produccion)
	}
	if got, quiero := px(margen+1, medio), tema.Color("surface", tema.Claro); !cerca(got, quiero) {
		t.Errorf("dentro del borde pinta %v y la superficie es %v: el borde no ocupa un píxel", got, quiero)
	}

	// 3. El primer botón es el principal, y su relleno es el token `primary`.
	//    Cae dentro de la tarjeta: borde + `px-4` + cabecera + `py-4`.
	xBoton := margen + int(tema.Borde+tema.Espacio(4)) + 4
	yBoton := margen + int(tema.Borde+ui.AltoCabecera("Cambios sin guardar")+tema.Espacio(4)) + 16
	if got, quiero := px(xBoton, yBoton), tema.Color("primary", tema.Claro); !cerca(got, quiero) {
		t.Errorf("el botón principal en (%d,%d) pinta %v y el token `primary` es %v", xBoton, yBoton, got, quiero)
	}

	if destino := os.Getenv("SYTUE_CAPTURA"); destino != "" {
		f, err := os.Create(destino)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		t.Logf("captura escrita en %s", destino)
	}
}

// TestLaPantallaOscuraCambiaDeColorYNoDeGeometria: el tema oscuro mueve los colores y
// NO las medidas. Si un día mueve las medidas, la paridad se rompe en un solo tema y
// nadie compara los dos.
func TestLaPantallaOscuraCambiaDeColorYNoDeGeometria(t *testing.T) {
	medidas := func(v tema.Variante) (fyne.Size, fyne.Size) {
		b := ui.NuevoBoton("Guardar cambios", nil)
		return ui.MedidaBoton(b.Texto, b.Talla, 0), ui.MedidaTarjeta("Cambios sin guardar", b)
	}
	bc, tc := medidas(tema.Claro)
	bo, to := medidas(tema.Oscuro)
	if bc != bo || tc != to {
		t.Errorf("las medidas cambian con el tema: botón %v→%v, tarjeta %v→%v", bc, bo, tc, to)
	}
	if tema.Color("bg", tema.Claro) == tema.Color("bg", tema.Oscuro) {
		t.Error("el token `bg` es el mismo en claro y en oscuro: el tema no está cambiando nada")
	}
}

// TestLosArosDeFocoSonLosDeLaWeb mide el aro de un control de cada familia sobre los
// píxeles del muestrario, a ×1 y a ×2 y en claro y en oscuro: 2 px del token, pegados a
// la caja (a 1 px, con hueco) y a 3:1 como poco. Es la MISMA medida que `cmd/demo` hace
// en una ventana de verdad (`scripts/ventana-real.sh --aros`), porque los dos pintores
// de Fyne no trazan igual y este test, él solo, no dice nada del aro que ve la gente.
//
// VISTO EN ROJO bajando un píxel la franja de arriba del aro abierto, que es donde el
// pintor de GL dejaba el trazo: las muestras de aro abierto salen centradas en 0 y no en −1.
func TestLosArosDeFocoSonLosDeLaWeb(t *testing.T) {
	hoja := webref.Read(t, "apps/frontend/packages/config/src/theme.css")
	if !strings.Contains(hoja, ":where(:focus-visible) {\n  outline: none;\n  box-shadow: 0 0 0 2px var(--sytue-focus-ring);\n}") {
		t.Error("theme.css ya no pinta el aro global de 2 px: el del muestrario ha dejado de ser el suyo")
	}
	t.Cleanup(func() { tema.Elegir("") })
	for _, modo := range []string{"claro", "oscuro"} {
		for _, escala := range []float32{1, 2} {
			tema.Elegir(modo)
			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			contenido, muestras := Aros(v)
			w := test.NewWindow(contenido)
			w.SetPadded(false)
			w.Canvas().(interface{ SetScale(float32) }).SetScale(escala)
			w.Resize(fyne.NewSize(AnchoAros, AltoAros))
			medidas := MedirAros(w.Canvas().Capture(), muestras, v)
			w.Close()
			if len(medidas) != 9 {
				t.Fatalf("el muestrario tiene %d muestras y son nueve", len(medidas))
			}
			t.Logf("%s a ×%v\n%s", modo, escala, TablaAros(medidas))
			for _, fallo := range ComprobarAros(medidas, muestras) {
				t.Errorf("%s a ×%v: %s", modo, escala, fallo)
			}
		}
	}
}
