// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package webui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

type editorHost struct {
	FyneBackend
	measurements, texts, placements int
}

func (h *editorHost) Editor(bool, Style, func(string)) Editor {
	panic("factory recursively called Backend.Editor")
}
func (h *editorHost) Measure(string, Style) fyne.Size {
	h.measurements++
	return fyne.NewSize(9, 20)
}
func (h *editorHost) Text(value string, style Style) *canvas.Text {
	h.texts++
	return h.FyneBackend.Text(value, style)
}
func (h *editorHost) PlaceText(text *canvas.Text, _ Style, x, y float32) {
	h.placements++
	text.Move(fyne.NewPos(x+5, y+7))
	text.Resize(text.MinSize())
}

func TestSharedEditorFactoryUsesHostDrawingWithoutCreatingAnotherFocusOwner(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	host := &editorHost{}
	style := Style{FontSize: 14, LineHeight: 20, FontWeight: 400, TextAlign: "center"}
	changed := ""
	editor := NewEditor(host, false, style, func(value string) { changed = value })
	if _, focusable := editor.Object().(fyne.Focusable); focusable {
		t.Fatal("shared editing canvas exposes an extra keyboard focus owner")
	}
	editor.SetText("x")
	editor.Layout(fyne.NewPos(0, 0), fyne.NewSize(120, 30))
	renderer := test.WidgetRenderer(editor.Object().(fyne.Widget))
	var text *canvas.Text
	for _, object := range renderer.Objects() {
		if drawn, ok := object.(*canvas.Text); ok {
			text = drawn
		}
	}
	if text == nil || text.Text != "x" || text.Position() != fyne.NewPos(60.5, 12) {
		t.Fatalf("editor ignored host measurements/placement: %+v", text)
	}
	if host.measurements == 0 || host.texts == 0 || host.placements == 0 {
		t.Fatal("editor did not use the host's typography methods")
	}
	editor.TypedRune('a')
	if editor.Text() != "xa" || changed != "xa" {
		t.Fatal("sharing host drawing lost the reusable editing engine's callback")
	}
}
