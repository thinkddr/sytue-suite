// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android || ios

package anfitrion

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// PonerSelector hace que `ficheros.Elegir` abra en el móvil el selector del toolkit. En
// el móvil `dialog` no pinta nada suyo: pide el del sistema (`ShowFileOpenPicker`).
// Si ya hay un selector nativo configurado (como en iOS con UIDocumentPickerModeImport),
// se conserva para no degradar al diálogo que exige iCloud. Lo llaman `sesion` (la
// edición conectada) y `local.Abrir` (Sytue Office, en Android).
func PonerSelector(a fyne.App) {
	if ficheros.EnElMovil == nil {
		ficheros.EnElMovil = func(extensiones []string) ([]string, error) { return elegirEnElMovil(a, extensiones) }
	}
}

func elegirEnElMovil(a fyne.App, extensiones []string) ([]string, error) {
	type resultado struct {
		ruta string
		err  error
	}
	listo := make(chan resultado, 1)
	fyne.Do(func() {
		ventanas := a.Driver().AllWindows()
		if len(ventanas) == 0 {
			listo <- resultado{err: ficheros.ErrNoDisponible}
			return
		}
		d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
			if err != nil || r == nil {
				listo <- resultado{err: err} // cancelar es (nil, nil)
				return
			}
			go func() {
				ruta, err := copiarElegido(r)
				listo <- resultado{ruta, err}
			}()
		}, ventanas[0])
		if tipos := tiposMovil(extensiones); tipos != nil {
			d.SetFilter(storage.NewMimeTypeFileFilter(tipos))
		} else if len(extensiones) > 0 {
			conPunto := make([]string, len(extensiones))
			for i, e := range extensiones {
				conPunto[i] = "." + e
			}
			d.SetFilter(storage.NewExtensionFileFilter(conPunto))
		}
		d.Show()
	})
	r := <-listo
	if r.err != nil || r.ruta == "" {
		return nil, r.err
	}
	return []string{r.ruta}, nil
}
