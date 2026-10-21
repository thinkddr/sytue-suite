// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command demo abre la pantalla mínima de los cimientos en una ventana de verdad.
//
// Para VERLA hace falta una máquina con pantalla; el piloto no la tiene. Lo que sí
// corre en cualquier sitio, sin CGO y sin servidor gráfico, es
// `go test ./pantalla/...`, que pinta lo mismo con el pintor de software y comprueba
// los píxeles. Con `SYTUE_CAPTURA=/ruta/demo.png` deja además la imagen.
package main

import (
	"fmt"
	"image/png"
	"os"
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
	w := a.NewWindow("Sytue — cimientos en Fyne")
	w.SetPadded(false)
	w.SetContent(pantalla.Demo(tema.De(a.Settings().ThemeVariant())))
	w.Resize(fyne.NewSize(pantalla.Ancho, pantalla.Alto))
	w.SetFixedSize(true)
	w.ShowAndRun()
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
