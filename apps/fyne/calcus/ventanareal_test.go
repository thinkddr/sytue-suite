//go:build ventanareal

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"fmt"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func init() {
	runtime.LockOSThread()
	enVentanaReal = ventanaRealCalcus
}

const libroVentanaReal = `{
  "active": 0,
  "sheets": [
    {
      "name": "Ventas",
      "tabColor": "#E11D48",
      "cells": {
        "B2": "Bordes sólidos",
        "D2": "Discontinuos"
      },
      "styles": {
        "B2": {
          "border": {
            "t": true, "ts": "thin",   "tc": "#E11D48",
            "b": true, "bs": "thick",  "bc": "#2563EB",
            "l": true, "ls": "medium", "lc": "#16A34A",
            "r": true, "rs": "thick",  "rc": "#D97706"
          }
        },
        "D2": {
          "border": {
            "t": true, "ts": "dashed", "tc": "#9333EA",
            "b": true, "bs": "dotted", "bc": "#0D9488",
            "l": true, "ls": "double", "lc": "#DC2626"
          }
        }
      }
    },
    {
      "name": "Costes",
      "tabColor": "#16A34A",
      "cells": {
        "B2": "Segunda hoja"
      }
    }
  ]
}`

// ventanaRealCalcus abre el lienzo de Calcus en una ventana DE VERDAD bajo Xvfb con dos
// hojas con `tabColor` y celdas con bordes de distinto color y grosor, comprueba sobre la
// captura real los píxeles de las bandas de pestaña y de los bordes, cambia de pestaña con
// xdotool y deja las capturas en `dir`.
func ventanaRealCalcus(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		return err
	}
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})
	lz := NuevoLienzo(NuevoLibro())
	lz.T = tr.T
	if !lz.Cargar(libroVentanaReal) {
		return fmt.Errorf("el libro de prueba no se abrió como editable")
	}
	lz.Rejilla.Seleccionar(0, 5, false)

	w := a.NewWindow("Sytue Calcus — Ventana real")
	w.SetPadded(false)
	w.SetContent(lz)
	w.Resize(fyne.NewSize(1280, 800))
	w.Show()

	captura := func(nombre string) (string, error) {
		ruta := filepath.Join(dir, nombre+".png")
		if out, err := exec.Command("import", "-window", "root", ruta).CombinedOutput(); err != nil {
			return "", fmt.Errorf("import %s: %w %s", nombre, err, out)
		}
		return ruta, nil
	}
	contieneColor := func(ruta string, buscado color.NRGBA) (int, error) {
		f, err := os.Open(ruta)
		if err != nil {
			return 0, err
		}
		defer func() { _ = f.Close() }()
		img, err := png.Decode(f)
		if err != nil {
			return 0, err
		}
		n := 0
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, _ := img.At(x, y).RGBA()
				got := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: 255}
				if cerca(got, buscado) {
					n++
				}
			}
		}
		return n, nil
	}

	fallo := make(chan error, 1)
	go func() {
		defer fyne.Do(a.Quit)
		pausa := func() { time.Sleep(1500 * time.Millisecond) }
		pausa()
		ruta1, err := captura("1-pestana-ventas-bordes")
		if err != nil {
			fallo <- err
			return
		}
		// Comprobar que en la captura de la ventana real se pintan la banda verde de la
		// pestaña inactiva (#16A34A) y los bordes azul (#2563EB) y ámbar (#D97706) de B2.
		for nombre, col := range map[string]color.NRGBA{
			"verde (#16A34A)": {R: 0x16, G: 0xA3, B: 0x4A, A: 255},
			"azul (#2563EB)":  {R: 0x25, G: 0x63, B: 0xEB, A: 255},
			"ámbar (#D97706)": {R: 0xD9, G: 0x77, B: 0x06, A: 255},
		} {
			n, err := contieneColor(ruta1, col)
			if err != nil {
				fallo <- err
				return
			}
			if n < 20 {
				fallo <- fmt.Errorf("en %s solo hay %d píxeles de %s", ruta1, n, nombre)
				return
			}
		}
		var x, y, ancho, alto float32
		fyne.DoAndWait(func() {
			if len(lz.Pestanas.tira.pestanas) < 2 {
				return
			}
			ph := lz.Pestanas.tira.pestanas[1]
			pos := a.Driver().AbsolutePositionForObject(ph)
			x, y, ancho, alto = pos.X, pos.Y, ph.Size().Width, ph.Size().Height
		})
		if ancho == 0 {
			fallo <- fmt.Errorf("no se encontró la segunda pestaña «Costes»")
			return
		}
		cx, cy := fmt.Sprint(int(x+ancho/2)), fmt.Sprint(int(y+alto/2))
		if out, err := exec.Command("xdotool", "mousemove", cx, cy, "click", "1").CombinedOutput(); err != nil {
			fallo <- fmt.Errorf("xdotool pestaña Costes: %w %s", err, out)
			return
		}
		pausa()
		if lz.Libro.Activa() != 1 {
			fallo <- fmt.Errorf("el clic real en «Costes» dejó activa la hoja %d", lz.Libro.Activa())
			return
		}
		ruta2, err := captura("2-pestana-costes")
		if err != nil {
			fallo <- err
			return
		}
		// Con «Costes» activa, la banda carmesí (#E11D48) de «Ventas» queda entera a la vista.
		n, err := contieneColor(ruta2, color.NRGBA{R: 0xE1, G: 0x1D, B: 0x48, A: 255})
		if err != nil {
			fallo <- err
			return
		}
		if n < 20 {
			fallo <- fmt.Errorf("en %s la banda carmesí de «Ventas» solo tiene %d píxeles", ruta2, n)
			return
		}
		fallo <- nil
	}()
	a.Run()
	select {
	case err := <-fallo:
		return err
	default:
		return fmt.Errorf("la ventana se cerró sin terminar el recorrido")
	}
}
