// SPDX-License-Identifier: Apache-2.0
package recorrido

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestBuscarDoesNotSelectHiddenOrUnrelatedControls(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	target := widget.NewButton("Save draft", nil)
	hidden := widget.NewButton("Save draft", nil)
	hidden.Hide()
	other := widget.NewButton("Send", nil)
	root := container.NewVBox(other, container.NewHBox(hidden, target))
	found := Buscar(root, func(object fyne.CanvasObject) bool {
		button, ok := object.(*widget.Button)
		return ok && button.Text == "Save draft"
	})
	if len(found) != 1 || found[0] != target {
		t.Fatal("semantic lookup selected a hidden or unrelated control", len(found))
	}
}
