// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package correo

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// En el alta de una cuenta, el servidor de IMAP y de SMTP tiene todo el ancho de su fila
// y el puerto y la seguridad van debajo: los tres en una fila dejaban el servidor en
// unos 60 píxeles en cualquier idioma (capturas en es y de, antes y después).
func TestElServidorTieneTodoElAnchoDeSuFila(t *testing.T) {
	test.NewTempApp(t)
	host, puerto := widget.NewEntry(), widget.NewEntry()
	seguridad := widget.NewSelect(seguridades, nil)
	seguridad.SetSelected(seguridades[1]) // «starttls», el más largo
	fila := filaServidor(host, puerto, seguridad)
	const ancho = 260 // lo que deja el formulario del diálogo a la derecha de «IMAP»
	fila.Resize(fyne.NewSize(ancho, fila.MinSize().Height))
	if w := host.Size().Width; w != ancho {
		t.Errorf("el servidor mide %.0f de ancho y la fila %d", w, ancho)
	}
	if c, ok := fila.(*fyne.Container); !ok || len(c.Objects) != 2 || c.Objects[0] != host ||
		c.Objects[1].Position().Y < host.Size().Height {
		t.Error("el puerto y la seguridad no van debajo del servidor")
	}
	if w := puerto.Size().Width; w < AnchoPuerto-1 {
		t.Errorf("el puerto mide %.0f; quiero %.0f", w, AnchoPuerto)
	}
	if w := seguridad.Size().Width; w < ancho-AnchoPuerto-2*fyne.CurrentApp().Settings().Theme().Size("padding") {
		t.Errorf("la seguridad mide %.0f: no se lleva el resto de su fila", w)
	}
}
