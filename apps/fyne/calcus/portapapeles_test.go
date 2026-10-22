// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"
)

// Las pruebas no dejan la copia compartida en la carpeta de la persona: van a una
// temporal, la misma para todo el paquete (como las ventanas de una misma máquina).
func init() {
	if dir, err := os.MkdirTemp("", "calcus-copia-*"); err == nil {
		carpetaCopia = func() (string, error) { return dir, nil }
	}
}

// TestSePeganFormulasDeOtraVentana: dos ventanas de Calcus son dos procesos, y la web
// entre dos pestañas pega la fórmula con sus referencias movidas. Aquí la otra ventana
// es otro lienzo que no ha copiado nada: el texto del portapapeles es el que dejó la
// primera, y la copia de alta fidelidad llega por la carpeta de la suite.
func TestSePeganFormulasDeOtraVentana(t *testing.T) {
	portapapeles := fyne.CurrentApp().Clipboard()
	una := lienzoDePrueba()
	una.Cargar(`{"active":0,"sheets":[{"name":"H","cells":{"A1":"1","A2":"<b>=A1*2</b>"}}]}`)
	una.Rejilla.Seleccionar(0, 1, false)
	una.Rejilla.Copiar(portapapeles, true) // un corte: en la otra ventana se pega como copia

	otra := lienzoDePrueba()
	otra.Rejilla.Seleccionar(1, 4, false)
	otra.Rejilla.Pegar(portapapeles.Content(), false)
	if got := otra.Libro.HTML(1, 4); got != "<b>=B4*2</b>" {
		t.Errorf("en la otra ventana se pega %q y tiene que ser la fórmula movida con su negrita", got)
	}
	if una.Libro.Crudo(0, 1) != "=A1*2" {
		t.Error("pegar en otra ventana ha vaciado el origen de la primera")
	}

	// Si otra aplicación pisa el portapapeles, se pega su texto.
	portapapeles.SetContent("hola")
	otra.Rejilla.Seleccionar(0, 0, false)
	otra.Rejilla.Pegar(portapapeles.Content(), false)
	if got := otra.Libro.Crudo(0, 0); got != "hola" {
		t.Errorf("con otro texto en el portapapeles se pega %q", got)
	}
}
