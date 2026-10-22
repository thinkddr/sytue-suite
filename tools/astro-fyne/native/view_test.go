// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package webui

import (
	"errors"
	"image"
	"image/color"
	"math"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestMain(m *testing.M) { test.NewApp(); os.Exit(m.Run()) }

func TestNativeTapReevaluatesGeneratedState(t *testing.T) {
	count := 0
	v := NewView(func() []Node {
		return []Node{{ID: "root", Kind: "container", Children: []Node{
			{ID: "count", Kind: "text", Text: String(count)},
			{ID: "increment", Kind: "button", Text: "Add", OnTap: func() { count++ }},
		}}}
	})
	button := v.Object("increment")
	tap(t, button)
	if count != 1 || v.elements["count"].node.Text != "1" {
		t.Fatalf("tap did not update generated state: %d %+v", count, v.elements["count"].node)
	}
	if button != v.Object("increment") {
		t.Fatal("a stable ID unnecessarily replaced its native object")
	}
}

func TestBuilderFailurePreservesLastValidMeasuredTree(t *testing.T) {
	cause := errors.New("missing native prop")
	fail := false
	v := NewView(func() []Node {
		if fail {
			panic(cause)
		}
		return []Node{{ID: "panel", Kind: "container"}}
	})
	v.SetViewport(40, 30)
	if err := v.ApplyMeasurements(map[string]Style{"panel": {X: 3.5, Y: 2.5, Width: 20, Height: 10, Background: "#ff0000", Opacity: 1, Measured: true}}); err != nil {
		t.Fatal(err)
	}
	panel := v.Object("panel")
	fail = true
	v.Refresh()
	if !errors.Is(v.Error(), cause) {
		t.Fatalf("builder error was lost: %v", v.Error())
	}
	if v.Object("panel") != panel || panel.Position() != fyne.NewPos(3.5, 2.5) || panel.Size() != fyne.NewSize(20, 10) {
		t.Fatal("builder failure replaced or repositioned the last valid measured tree")
	}
	fail = false
	v.Refresh()
	if v.Error() != nil {
		t.Fatalf("a successful render did not clear its earlier build error: %v", v.Error())
	}
	broken := NewView(func() []Node { panic("invalid initial expression") })
	if broken.Error() == nil || len(broken.elements) != 0 {
		t.Fatal("an invalid initial builder did not return an empty view with an error")
	}
}

func TestNavigationFailureSurvivesRefreshAndClearsOnSuccessfulRetry(t *testing.T) {
	href := "://invalid"
	v := NewView(func() []Node { return []Node{{ID: "link", Kind: "link", Text: "Open", Href: href}} })
	tap(t, v.Object("link"))
	if v.Error() == nil {
		t.Fatal("navigation failure disappeared in the action's automatic refresh")
	}
	message := v.Error().Error()
	v.Refresh()
	if v.Error() == nil || v.Error().Error() != message {
		t.Fatal("ordinary rendering erased the navigation error")
	}
	href = "https://example.com"
	v.Refresh()
	tap(t, v.Object("link"))
	if v.Error() != nil {
		t.Fatalf("successful navigation retry did not clear its error: %v", v.Error())
	}
}

func TestInputsHaveOneTabStopAndDisabledFieldsAreSkipped(t *testing.T) {
	v := NewView(func() []Node {
		return []Node{{ID: "root", Kind: "container", Children: []Node{
			{ID: "first", Kind: "input", Value: "one", AccessibleLabel: "First"},
			{ID: "disabled", Kind: "input", Value: "two", Disabled: true, AccessibleLabel: "Disabled"},
			{ID: "last", Kind: "input", Value: "three", AccessibleLabel: "Last"},
		}}}
	})
	w := test.NewWindow(v)
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(240, 140))
	first := v.Object("first").(fyne.Focusable)
	last := v.Object("last").(fyne.Focusable)
	w.Canvas().Focus(first)
	w.Canvas().FocusNext()
	if w.Canvas().Focused() != last {
		t.Fatalf("Tab entered an inner editor or a disabled field: %T", w.Canvas().Focused())
	}
	w.Canvas().FocusNext()
	if w.Canvas().Focused() != first {
		t.Fatal("tab chain contained an extra inner editor")
	}
	w.Canvas().FocusPrevious()
	if w.Canvas().Focused() != last {
		t.Fatal("reverse tab chain did not skip disabled fields and inner editors")
	}
	disabled, ok := v.Object("disabled").(fyne.Disableable)
	if !ok || !disabled.Disabled() {
		t.Fatal("disabled input does not satisfy Fyne's focus-manager contract")
	}
	w.Canvas().Focus(v.Object("disabled").(fyne.Focusable))
	if w.Canvas().Focused() != last {
		t.Fatal("programmatic focus entered a disabled input")
	}
	if _, ok := v.elements["first"].input.Object().(fyne.Focusable); ok {
		t.Fatal("input drawing exposes a second focusable control")
	}
}

func TestInputPointerPositionsCursorWithoutChangingFocusOwner(t *testing.T) {
	value := "ab"
	v := NewView(func() []Node {
		return []Node{{ID: "field", Kind: "input", Value: value, OnChange: func(s string) { value = s }}}
	})
	w := test.NewWindow(v)
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(240, 50))
	field := v.Object("field")
	style := v.elements["field"].style
	field.(fyne.Tappable).Tapped(&fyne.PointEvent{Position: fyne.NewPos(style.PaddingLeft+style.BorderWidth, style.PaddingTop+style.BorderWidth+10)})
	field.(fyne.Focusable).TypedRune('X')
	if value != "Xab" || w.Canvas().Focused() != field.(fyne.Focusable) {
		t.Fatalf("pointer/cursor/focus mismatch: value=%q focus=%T", value, w.Canvas().Focused())
	}
}

func TestCaptureBindingRejectsUnknownAndChangingDeviceScale(t *testing.T) {
	v := NewView(func() []Node { return []Node{{ID: "panel", Kind: "container"}} })
	v.SetViewport(100, 40)
	if err := v.ApplyMeasurements(map[string]Style{"panel": {Width: 100, Height: 40, Background: "#ffffff", Opacity: 1, Measured: true}}); err != nil {
		t.Fatal(err)
	}
	if err := v.SetCaptureScale(2); err != nil {
		t.Fatal(err)
	}
	if err := v.ValidateCanvas(); err == nil {
		t.Fatal("unbound device scale was assumed to be correct")
	}
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetScale(2)
	c.Resize(fyne.NewSize(100, 40))
	c.SetContent(v)
	if err := v.BindCanvas(c); err != nil {
		t.Fatal(err)
	}
	if err := v.ValidateCanvas(); err != nil {
		t.Fatal(err)
	}
	c.SetScale(1)
	if err := v.ValidateCanvas(); err == nil || !strings.Contains(err.Error(), "device scale") {
		t.Fatalf("changed driver scale was accepted: %v", err)
	}
	v.Refresh()
	if v.Error() == nil {
		t.Fatal("refresh did not record the bound canvas's scale mismatch")
	}
	c.SetScale(2)
	v.Refresh()
	if v.Error() != nil {
		t.Fatalf("restored matching scale remained invalid: %v", v.Error())
	}
	c.Resize(fyne.NewSize(200, 40))
	if err := v.ValidateCanvas(); err == nil {
		t.Fatal("bound canvas silently changed the captured viewport")
	}
}

func TestNativeInputKeepsFocusAndCursorDuringReconciliation(t *testing.T) {
	value := ""
	v := NewView(func() []Node {
		return []Node{{ID: "name", Kind: "input", Value: value, Placeholder: "Name", OnChange: func(s string) { value = s }}}
	})
	w := test.NewWindow(v)
	defer w.Close()
	w.SetPadded(false)
	w.Resize(fyne.NewSize(240, 50))
	field := v.Object("name").(fyne.Focusable)
	w.Canvas().Focus(field)
	field.TypedRune('ñ')
	field.TypedRune('b')
	if value != "ñb" {
		t.Fatalf("controlled input/cursor lost text: %q", value)
	}
	if v.Object("name").(fyne.Focusable) != field || w.Canvas().Focused() != field {
		t.Fatal("state refresh replaced the focused field")
	}
}

func TestMeasuredGeometryKeepsBrowserFractionsAndFirstPaint(t *testing.T) {
	v := NewView(func() []Node {
		return []Node{{ID: "panel", Kind: "container", Children: []Node{{ID: "red", Kind: "container"}}}}
	})
	v.SetViewport(40, 30)
	profile := map[string]Style{
		"panel": {X: 1.5, Y: 2.5, Width: 22.5, Height: 18.5, Background: "#ffffff", Opacity: 1, Measured: true},
		"red":   {X: 2.5, Y: 3.5, Width: 7.5, Height: 5.5, Background: "#ff0000", Opacity: 1, Measured: true},
	}
	if err := v.ApplyMeasurements(profile); err != nil {
		t.Fatal(err)
	}
	first := capture(v, 2, 40, 30)
	red := v.Object("red")
	if red.Position() != fyne.NewPos(2.5, 3.5) || red.Size() != fyne.NewSize(7.5, 5.5) {
		t.Fatalf("fractional browser geometry changed: %v %v", red.Position(), red.Size())
	}
	// Absolute red box from the external browser bounds: (1.5+2.5,2.5+3.5),
	// 7.5 by 5.5 CSS pixels, so exactly 15 by 11 device pixels at scale two.
	count := 0
	for y := range first.Bounds().Dy() {
		for x := range first.Bounds().Dx() {
			if rgba(first.At(x, y)) == (color.NRGBA{255, 0, 0, 255}) {
				count++
			}
		}
	}
	if count != 15*11 {
		t.Fatalf("painted red pixels = %d, browser geometry demands 165", count)
	}
	v.Refresh()
	second := capture(v, 2, 40, 30)
	for y := range first.Bounds().Dy() {
		for x := range first.Bounds().Dx() {
			if rgba(first.At(x, y)) != rgba(second.At(x, y)) {
				t.Fatalf("first frame differs after refresh at %d,%d", x, y)
			}
		}
	}
}

func TestMeasurementsRejectPartialExtraStaleAndRescaledProfiles(t *testing.T) {
	text := "one"
	font := theme.DefaultTheme().Font(fyne.TextStyle{})
	b := FyneBackend{Fonts: map[string]map[Font]fyne.Resource{"Test": {{Weight: 400}: font}}}
	v := NewView(func() []Node { return []Node{{ID: "label", Kind: "text", Text: text}} }, b)
	v.SetViewport(100, 40)
	style := Style{Width: 100, Height: 20, FontSize: 14, LineHeight: 20, FontWeight: 400, FontFamily: "Test", Color: "#000000", Opacity: 1, Measured: true}
	if err := v.ApplyMeasurements(map[string]Style{"extra": style}); err == nil {
		t.Fatal("partial/extra measurement accepted")
	}
	if err := v.ApplyMeasurements(map[string]Style{"label": style, "extra": style}); err == nil {
		t.Fatal("unknown measurement accepted")
	}
	if err := v.ApplyMeasurements(map[string]Style{"label": style}); err != nil {
		t.Fatal(err)
	}
	if err := v.ValidateViewport(fyne.NewSize(200, 40)); err == nil {
		t.Fatal("captured geometry silently stretched")
	}
	text = "two"
	v.Refresh()
	if v.Error() == nil || v.elements["label"].style.Measured {
		t.Fatal("changed visual text reused the stale capture")
	}
}

func TestMeasuredTextRequiresMatchingExplicitFont(t *testing.T) {
	v := NewView(func() []Node { return []Node{{ID: "text", Kind: "text", Text: "hello"}} })
	style := Style{Width: 80, Height: 20, FontSize: 14, LineHeight: 20, FontFamily: "Unloaded", FontWeight: 400, Opacity: 1, Measured: true}
	if err := v.ApplyMeasurements(map[string]Style{"text": style}); err == nil {
		t.Fatal("an unprovided font was silently substituted")
	}
}

func TestTransparentBorderKeepsItsInteriorTransparent(t *testing.T) {
	style := Style{Width: 20, Height: 20, BorderWidth: 2.5, BorderColor: "rgba(255,0,0,1)", Background: "transparent", Opacity: 1, Measured: true}
	box := (FyneBackend{}).Box(style)
	box.Resize(fyne.NewSize(20, 20))
	// Inspect the independently generated border raster. Its center must be a hole,
	// not a full red rectangle exposed through a transparent background rectangle.
	primitive := box.(*primitiveBox)
	img := primitive.paintBorder(40, 40)
	if rgba(img.At(20, 20)).A != 0 {
		t.Fatal("border leaked into the transparent center")
	}
	if got := rgba(img.At(1, 20)); got != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatalf("measured 2.5px border missing: %v", got)
	}
}

func TestExpressionSemanticsPreserveUndefinedAndJSXProjection(t *testing.T) {
	if String(false) != "false" || ChildText(false) != "" || String(nil) != "null" || ChildText(nil) != "" {
		t.Fatal("JavaScript conversion was confused with JSX omission")
	}
	if String(Undefined) != "undefined" || Truth(Undefined) || !math.IsNaN(Number(Undefined)) {
		t.Fatal("undefined lost its JavaScript scalar semantics")
	}
	if Truth(Binary("===", Get(Scope{}, "missing"), nil)) {
		t.Fatal("missing property became null instead of undefined")
	}
	if Get("a😀b", "length") != 4 || Get("mañana", 2) != "ñ" {
		t.Fatal("string indexing or length is not UTF-16 safe")
	}
	if ChildText([]any{"a", false, nil, 2}) != "a2" {
		t.Fatal("JSX array children did not flatten")
	}
	if !Truth([]any{}) || !Truth(map[string]any{}) {
		t.Fatal("empty JavaScript collections must be truthy")
	}
	if Binary("+", false, " value") != "false value" {
		t.Fatal("concatenation silently omitted false")
	}
	if String(1e21) != "1e+21" || String(1e-7) != "1e-7" || String(1e-6) != "0.000001" {
		t.Fatal("number strings differ from JavaScript exponent thresholds")
	}
	if Number("0x10") != 16 || Number("0b11") != 3 || Number("0o10") != 8 || !math.IsNaN(Number("Inf")) {
		t.Fatal("JavaScript numeric string conversion differs")
	}
	if !isUndefined(Get([]string{"zero", "one"}, "01")) || !isUndefined(Get([]string{"zero", "one"}, true)) {
		t.Fatal("noncanonical array property became an index")
	}
	type flag bool
	if Truth(flag(false)) || String(flag(false)) != "false" || ChildText(flag(true)) != "" {
		t.Fatal("native named boolean lost its scalar value")
	}

}

func capture(view *View, scale, width, height float32) image.Image {
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetContent(view)
	c.SetScale(scale)
	c.Resize(fyne.NewSize(width, height))
	return c.Capture()
}
func rgba(c color.Color) color.NRGBA { return color.NRGBAModel.Convert(c).(color.NRGBA) }
func tap(t *testing.T, object fyne.CanvasObject) {
	t.Helper()
	button, ok := object.(fyne.Tappable)
	if !ok {
		t.Fatalf("native object %T is not tappable", object)
	}
	test.Tap(button)
}
