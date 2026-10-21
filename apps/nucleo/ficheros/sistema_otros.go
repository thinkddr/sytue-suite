// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || android || js || !(darwin || windows || linux)

package ficheros

import (
	"os"
	"path/filepath"
	"runtime"
)

// Este fichero es el destino que NO es un escritorio: el navegador, Android e iOS.
// En móvil, Revelar y Abrir abren la hoja de compartir o el visor asociado del sistema
// (abrirMovil / revelarMovil). En el navegador dicen ErrNoDisponible. Imprimir
// en Android va por PrintManager y en iOS por UIPrintInteractionController (AirPrint).
//
// Carpeta es la de descargas. En iOS apunta a ~/Documents, visible directamente en la
// aplicación «Archivos» gracias a UIFileSharingEnabled.
func Carpeta() (string, error) {
	casa, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "ios" {
		doc := filepath.Join(casa, "Documents")
		_ = os.MkdirAll(doc, 0o755)
		return doc, nil
	}
	return filepath.Join(casa, "Downloads"), nil
}

func Revelar(ruta string) error {
	abs, err := local(ruta)
	if err != nil {
		return err
	}
	return revelarMovil(abs)
}

func Abrir(ruta string) error {
	abs, err := local(ruta)
	if err != nil {
		return err
	}
	return abrirMovil(abs)
}

// Imprimir en Android es el diálogo de PrintManager para un PDF (movil_android.go).
func Imprimir(ruta string) error { return imprimirMovil(ruta) }

// Elegir, en el móvil, es el selector del toolkit que pone la sesión en [EnElMovil]; en
// el navegador no hay ninguno.
func Elegir() ([]string, error) {
	if EnElMovil != nil {
		return EnElMovil(nil)
	}
	return nil, ErrNoDisponible
}
