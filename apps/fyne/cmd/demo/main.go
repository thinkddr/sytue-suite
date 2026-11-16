// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command demo abre la pantalla mínima de los cimientos en una ventana de verdad.
//
// Para VERLA hace falta una máquina con pantalla; el piloto no la tiene. Lo que sí
// corre en cualquier sitio, sin CGO y sin servidor gráfico, es
// `go test ./pantalla/...`, que pinta lo mismo con el pintor de software y comprueba
// los píxeles. Con `SYTUE_CAPTURA=/ruta/demo.png` deja además la imagen.
//
// Con `SYTUE_MUESTRARIO=1` abre el muestrario de `apps/fyne/ui` desde su primera sección,
// y ← y → pasan de una a otra; `SYTUE_SECCION=<sección>` lo abre en esa (`botones`,
// `campos`, `dialogo`…; una que no existe lista las que hay). `SYTUE_TEMA` (`claro`,
// `oscuro`, `traslucido`) elige el modo, como con `SYTUE_AROS`, y `SYTUE_COLOR`
// (`petroleo`, `cobalto`) y `SYTUE_FORMA` (`marco`, `carril`) los otros dos ejes: los
// mismos que el muestrario de la web (`packages/ui/test/muestrario.tsx`), para ponerlos
// lado a lado.
package main

import (
	"fmt"
	"image/png"
	"os"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/pantalla"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})
	if os.Getenv("SYTUE_AROS") != "" {
		os.Exit(aros(a))
	}
	if seccion := os.Getenv("SYTUE_SECCION"); seccion != "" || os.Getenv("SYTUE_MUESTRARIO") != "" {
		if seccion == "" {
			seccion = pantalla.Secciones()[0]
		}
		os.Exit(muestrario(a, seccion))
	}
	w := a.NewWindow("Sytue — cimientos en Fyne")
	w.SetPadded(false)
	w.SetContent(pantalla.Demo(tema.De(a.Settings().ThemeVariant())))
	w.Resize(fyne.NewSize(pantalla.Ancho, pantalla.Alto))
	w.SetFixedSize(true)
	w.ShowAndRun()
}

// muestrario abre el muestrario de `ui` en la sección `seccion`, con el modo, el color y
// la forma de `SYTUE_TEMA`, `SYTUE_COLOR` y `SYTUE_FORMA`; ← y → cambian de sección. El
// diálogo y el menú se abren cuando la ventana ya está puesta, porque cuelgan del lienzo.
func muestrario(a fyne.App, seccion string) int {
	claves := pantalla.Secciones()
	i := slices.Index(claves, seccion)
	if i < 0 {
		fmt.Fprintf(os.Stderr, "no hay sección %q; las secciones son: %s\n", seccion, strings.Join(claves, ", "))
		return 2
	}
	comb := pantalla.Combinacion{Color: os.Getenv("SYTUE_COLOR"), Modo: os.Getenv("SYTUE_TEMA"), Forma: os.Getenv("SYTUE_FORMA")}
	v := comb.Aplicar()
	w := a.NewWindow("")
	w.SetPadded(false)
	w.Resize(fyne.NewSize(pantalla.AnchoMuestrario, pantalla.AltoMuestrario))
	w.SetFixedSize(true)
	// vez cuenta las secciones puestas: la capa de una sección que se ha dejado atrás con
	// ← o → antes de su medio segundo no se abre encima de la siguiente.
	vez := 0
	mostrar := func() {
		vez++
		esta := vez
		// Remove quita la capa y todas las de encima: basta con la de abajo.
		if capas := w.Canvas().Overlays().List(); len(capas) > 0 {
			w.Canvas().Overlays().Remove(capas[0])
		}
		contenido, abrir, _ := pantalla.Muestrario(claves[i], v)
		w.SetTitle(fmt.Sprintf("Sytue — muestrario: %s (%d/%d) · ← →", claves[i], i+1, len(claves)))
		w.SetContent(contenido)
		if abrir != nil {
			go func() {
				time.Sleep(500 * time.Millisecond) // la ventana, ya medida
				fyne.Do(func() {
					if esta == vez {
						abrir(w.Canvas())
					}
				})
			}()
		}
	}
	w.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		switch k.Name {
		case fyne.KeyRight:
			i = (i + 1) % len(claves)
		case fyne.KeyLeft:
			i = (i + len(claves) - 1) % len(claves)
		default:
			return
		}
		mostrar()
	})
	mostrar()
	w.ShowAndRun()
	return 0
}

// aros abre el muestrario de aros de foco, lo mide sobre los píxeles de la ventana —los
// del pintor de GL, que es el que ve la gente— y devuelve 1 si alguno no es el de la web.
//
// `SYTUE_AROS` es dónde dejar la captura (un `.png`; con cualquier otra cosa, solo mide),
// `SYTUE_TEMA` el modo (`claro`, `oscuro`, `traslucido`) y `FYNE_SCALE` la escala. Es lo
// que lanza `scripts/ventana-real.sh --aros`; en un Mac se puede lanzar a mano.
func aros(a fyne.App) int {
	tema.Elegir(os.Getenv("SYTUE_TEMA"))
	v := tema.De(a.Settings().ThemeVariant())
	contenido, muestras := pantalla.Aros(v)
	w := a.NewWindow("Sytue — aros de foco")
	w.SetPadded(false)
	w.SetContent(contenido)
	w.Resize(fyne.NewSize(pantalla.AnchoAros, pantalla.AltoAros))
	w.SetFixedSize(true)
	estado := 0
	go func() {
		time.Sleep(2 * time.Second) // la ventana, ya pintada
		fyne.Do(func() {
			img := w.Canvas().Capture()
			if destino := os.Getenv("SYTUE_AROS"); strings.HasSuffix(destino, ".png") {
				if f, err := os.Create(destino); err == nil { //nolint:gosec // G703: la ruta la pone quien lanza la demo
					_ = png.Encode(f, img)
					_ = f.Close()
				}
			}
			medidas := pantalla.MedirAros(img, muestras, v)
			fmt.Print(pantalla.TablaAros(medidas))
			for _, fallo := range pantalla.ComprobarAros(medidas, muestras) {
				fmt.Println("MAL", fallo)
				estado = 1
			}
			a.Quit()
		})
	}()
	w.ShowAndRun()
	return estado
}
