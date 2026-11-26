// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package local

import (
	"errors"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

// ErrSinEtiqueta: el binario no se compiló con la etiqueta `edicionlocal` (D-02). Sin
// ella `ui` enlaza la sesión de la suite y su login, y la app no sería de Sytue Office.
var ErrSinEtiqueta = errors.New("local: este binario no se compiló con la etiqueta edicionlocal (go build -tags goolm,edicionlocal)")

var (
	appMu       sync.Mutex
	appDePrueba func(id string) fyne.App
)

// NuevaApp crea la app de Fyne de una app de Sytue Office: la de `app.NewWithID(id)`, o
// con id vacío la de `app.New()` (el identificador del FyneApp.toml). Cada `main` de
// `cmd-local` la pide aquí y no a Fyne, para que la prueba de arranque
// (`local/prueba.Arrancar`) monte ESE `main` en el driver de pruebas de Fyne.
func NuevaApp(id string) fyne.App {
	appMu.Lock()
	f := appDePrueba
	appMu.Unlock()
	if f != nil {
		return f(id)
	}
	if id == "" {
		return app.New()
	}
	return app.NewWithID(id)
}

// AppDePrueba hace que [NuevaApp] devuelva lo que dé f (la app del driver de pruebas) y
// devuelve cómo deshacerlo. Es SOLO para `local/prueba`: ningún otro fichero que no sea
// de pruebas lo llama (TestLasPuertasDePruebaSoloEnPruebas, en `nucleo/local/red`).
func AppDePrueba(f func(id string) fyne.App) (deshacer func()) {
	appMu.Lock()
	antes := appDePrueba
	appDePrueba = f
	appMu.Unlock()
	return func() {
		appMu.Lock()
		appDePrueba = antes
		appMu.Unlock()
	}
}
