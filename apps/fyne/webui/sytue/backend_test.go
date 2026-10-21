// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package sytue

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	webui "github.com/thinkddr/astro-fyne/native"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func TestBackendPaintsThePrimaryFamilyItValidated(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	for _, choice := range []struct {
		family string
		weight float32
		face   fyne.Resource
	}{
		{"Figtree, 'JetBrains Mono'", 400, tema.Regular},
		{"Figtree, 'JetBrains Mono'", 600, tema.SemiNegra},
		{"'JetBrains Mono', Figtree", 400, tema.Mono},
	} {
		style := webui.Style{FontFamily: choice.family, FontWeight: choice.weight, FontSize: 14, LineHeight: 20, Measured: true}
		if err := (Backend{}).Validate(style, true); err != nil {
			t.Fatal(err)
		}
		text := (Backend{}).Text("same primary face", style)
		if text.FontSource != choice.face {
			t.Fatalf("family%q weight%v painted%s instead of%s", choice.family, choice.weight, text.FontSource.Name(), choice.face.Name())
		}
		(Backend{}).PlaceText(text, style, 0, 0)
		if text.FontSource != choice.face {
			t.Fatal("placing a validated text switched its font family")
		}
	}
}

func TestBackendRejectsUnrepresentableMeasuredTypography(t *testing.T) {
	base := webui.Style{FontFamily: "Figtree", FontWeight: 400, FontSize: 14, LineHeight: 20, Measured: true}
	for _, style := range []webui.Style{
		{FontFamily: "Figtree", FontWeight: 400, LineHeight: 20, Measured: true},
		{FontFamily: "Figtree", FontWeight: 400, FontSize: 14, Measured: true},
		{FontFamily: "unsupported face", FontWeight: 400, FontSize: 14, LineHeight: 20, Measured: true},
		{FontFamily: "Figtree", FontWeight: 300, FontSize: 14, LineHeight: 20, Measured: true},
		{FontFamily: "Figtree", FontWeight: 600, FontStyle: "italic", FontSize: 14, LineHeight: 20, Measured: true},
		{FontFamily: "JetBrains Mono", FontWeight: 700, FontSize: 14, LineHeight: 20, Measured: true},
	} {
		if err := (Backend{}).Validate(style, true); err == nil {
			t.Fatalf("unsupported typography silently accepted: %+v", style)
		}
	}
	if err := (Backend{}).Validate(base, true); err != nil {
		t.Fatalf("supported measured typography rejected: %v", err)
	}
}

func TestBackendSharedEditorUsesHostTypographyAndAlignment(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	for _, alignment := range []string{"center", "right"} {
		style := webui.Style{FontFamily: "Figtree, 'JetBrains Mono'", FontWeight: 400, FontSize: 14, LineHeight: 20, TextAlign: alignment, Color: "#11232b"}
		editor := (Backend{}).Editor(false, style, nil)
		if _, duplicate := editor.Object().(fyne.Focusable); duplicate {
			t.Fatal("editing canvas exposes a second keyboard focus owner")
		}
		editor.SetText("native")
		editor.Layout(fyne.NewPos(0, 0), fyne.NewSize(120, 30))
		renderer := test.WidgetRenderer(editor.Object().(fyne.Widget))
		var painted *canvas.Text
		for _, object := range renderer.Objects() {
			if text, ok := object.(*canvas.Text); ok {
				painted = text
			}
		}
		if painted == nil || painted.FontSource != tema.Regular {
			t.Fatal("shared editor did not use the host's primary Figtree font")
		}
		width := ui.Medir("native", tema.Escalon{Tam: 14, Linea: 20}, ui.Normal).Width
		x := float32(120) - width
		if alignment == "center" {
			x /= 2
		}
		if painted.Position().X != x {
			t.Fatalf("%s input was painted at%v, expected%v", alignment, painted.Position().X, x)
		}
	}
}

func TestBackendInputsHaveOneFocusOwnerAndCommitOnReturnAndBlur(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	value, commits := "A", []string{}
	v := webui.NewView(func() []webui.Node {
		return []webui.Node{{ID: "root", Kind: "container", Children: []webui.Node{
			{ID: "field", Kind: "input", Value: value, OnCommit: func(s string) { value = s; commits = append(commits, s) }},
			{ID: "disabled", Kind: "input", Disabled: true},
			{ID: "next", Kind: "input"},
		}}}
	}, Backend{})
	window := test.NewWindow(v)
	defer window.Close()
	window.SetPadded(false)
	window.Resize(fyne.NewSize(300, 150))
	field := v.Object("field").(fyne.Focusable)
	next := v.Object("next").(fyne.Focusable)
	window.Canvas().Focus(field)
	field.TypedRune('a')
	if len(commits) != 0 {
		t.Fatal("HTML change fired before the adapter input committed")
	}
	window.Canvas().FocusNext()
	if window.Canvas().Focused() != next || len(commits) != 1 || commits[0] != "Aa" {
		t.Fatalf("Tab entered an inner/disabled editor or skipped blur commit: focus=%T commits=%v", window.Canvas().Focused(), commits)
	}
	window.Canvas().Focus(field)
	field.TypedRune('b')
	field.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if window.Canvas().Focused() != field || len(commits) != 2 || commits[1] != "Aab" {
		t.Fatalf("Return lost the native focus owner or failed commit: commits=%v", commits)
	}
	window.Canvas().Unfocus()
	if len(commits) != 2 || v.Object("field").(fyne.Focusable) != field || v.Error() != nil {
		t.Fatalf("blur duplicated Return, replaced focus owner or failed render: commits=%v error=%v", commits, v.Error())
	}
}
