// SPDX-License-Identifier: Apache-2.0
package generated

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	webui "github.com/thinkddr/astro-fyne/native"
)

type observation struct {
	prefix   string
	previous float64
}

type generatedView interface {
	Object(string) fyne.CanvasObject
	Error() error
}

func conformanceView(t *testing.T) (*ConformanceWidget, *[]observation, map[string]int) {
	t.Helper()
	app := test.NewApp()
	t.Cleanup(app.Quit)
	observations := []observation{}
	calls := map[string]int{}
	observe := func(args ...any) any {
		if len(args) != 2 {
			t.Fatalf("observe expects prefix and previous count, got %v", args)
		}
		observations = append(observations, observation{webui.String(args[0]), webui.Number(args[1])})
		return nil
	}
	translate := func(args ...any) any {
		if len(args) != 1 {
			t.Fatalf("t expects one key, got %v", args)
		}
		key := webui.String(args[0])
		calls[key]++
		return "translated:" + key
	}
	view, err := NewConformance(webui.Scope{
		"observe": observe, "t": translate, "disabledBranch": false,
		"retainedText": "kept", "retainedNumber": float64(0), "nullValue": nil,
	}, webui.Actions{"observe": observe, "t": translate})
	if err != nil {
		t.Fatal(err)
	}
	window := app.NewWindow("Generated conformance")
	window.Resize(fyne.NewSize(800, 1200))
	window.SetContent(view)
	view.Refresh()
	if err := view.Error(); err != nil {
		t.Fatal(err)
	}
	return view, &observations, calls
}

func generatedText(t *testing.T, view *ConformanceWidget, id string) string {
	t.Helper()
	object := view.Object(id)
	if object == nil {
		t.Fatalf("generated node %q is missing", id)
	}
	accessible, ok := object.(interface{ AccessibilityLabel() string })
	if !ok {
		t.Fatalf("generated text %q has no observable text: %T", id, object)
	}
	return accessible.AccessibilityLabel()
}

func requireGeneratedText(t *testing.T, view *ConformanceWidget, id, want string) {
	t.Helper()
	if got := generatedText(t, view, id); got != want {
		t.Fatalf("generated %s: got %q, want %q", id, got, want)
	}
}

func tapGenerated(t *testing.T, view generatedView, id string) {
	t.Helper()
	object := view.Object(id)
	if object == nil {
		t.Fatalf("generated action %q is missing", id)
	}
	tappable, ok := object.(fyne.Tappable)
	if !ok {
		t.Fatalf("generated action %q is not native tappable: %T", id, object)
	}
	test.Tap(tappable)
	if err := view.Error(); err != nil {
		t.Fatalf("generated action %q: %v", id, err)
	}
}

func TestGeneratedSiblingStateInitializersBatchingAndClosures(t *testing.T) {
	view, observations, _ := conformanceView(t)
	// Both instances execute the same function and hook name. A const before the
	// hook must initialize each state with its own supplied props.
	requireGeneratedText(t, view, "left-value", "3")
	requireGeneratedText(t, view, "right-value", "11")
	right := view.Object("right-value")
	tapGenerated(t, view, "left-increment")
	requireGeneratedText(t, view, "left-value", "4")
	requireGeneratedText(t, view, "right-value", "11")
	if right != view.Object("right-value") {
		t.Fatal("updating one instance recreated its sibling")
	}
	// Direct setters retain the render closure; functional setters see pending
	// updates: 4 -> 5 -> 15 -> 6 -> 9. The callback still observes the old 4.
	tapGenerated(t, view, "left-batch")
	requireGeneratedText(t, view, "left-value", "9")
	requireGeneratedText(t, view, "right-value", "11")
	if len(*observations) != 1 || (*observations)[0] != (observation{"left", 4}) {
		t.Fatalf("batch changed the callback closure: %v", *observations)
	}
	tapGenerated(t, view, "left-batch")
	requireGeneratedText(t, view, "left-value", "14")
	if len(*observations) != 2 || (*observations)[1] != (observation{"left", 9}) {
		t.Fatalf("rerender did not install a new event closure: %v", *observations)
	}
	tapGenerated(t, view, "right-increment")
	requireGeneratedText(t, view, "right-value", "12")
	requireGeneratedText(t, view, "left-value", "14")
}

func TestGeneratedUnmountRemountResetsOnlyTheUnmountedState(t *testing.T) {
	view, _, _ := conformanceView(t)
	requireGeneratedText(t, view, "ephemeral-value", "21")
	old := view.Object("ephemeral-value")
	tapGenerated(t, view, "ephemeral-increment")
	requireGeneratedText(t, view, "ephemeral-value", "22")
	tapGenerated(t, view, "left-increment")
	tapGenerated(t, view, "toggle-ephemeral")
	if view.Object("ephemeral-value") != nil || view.Object("ephemeral-increment") != nil {
		t.Fatal("the hidden component retained native objects")
	}
	requireGeneratedText(t, view, "left-value", "4")
	requireGeneratedText(t, view, "right-value", "11")
	tapGenerated(t, view, "toggle-ephemeral")
	requireGeneratedText(t, view, "ephemeral-value", "21")
	if old == view.Object("ephemeral-value") {
		t.Fatal("the remounted component reused its destroyed native object")
	}
	requireGeneratedText(t, view, "left-value", "4")
	requireGeneratedText(t, view, "right-value", "11")
}

func TestGeneratedMissingPropertiesAreUndefinedRatherThanNull(t *testing.T) {
	view, _, _ := conformanceView(t)
	for _, form := range []string{"destructured", "object"} {
		for suffix, want := range map[string]string{
			"missing-undefined": "true", "missing-null": "false",
			"null-undefined": "false", "null-null": "true",
		} {
			requireGeneratedText(t, view, fmt.Sprintf("%s-%s", form, suffix), want)
		}
	}
}

func TestGeneratedLogicalExpressionsSkipUnneededHostCalls(t *testing.T) {
	view, _, calls := conformanceView(t)
	requireGeneratedText(t, view, "short-and", "false")
	requireGeneratedText(t, view, "short-or", "kept")
	requireGeneratedText(t, view, "short-nullish", "0")
	requireGeneratedText(t, view, "selected-null", "translated:selected-null")
	requireGeneratedText(t, view, "selected-missing", "translated:selected-missing")
	tapGenerated(t, view, "left-increment")
	for _, key := range []string{"unexpected-and", "unexpected-or", "unexpected-nullish"} {
		if calls[key] != 0 {
			t.Fatalf("short circuit evaluated unused host callback %q %d times", key, calls[key])
		}
	}
	for _, key := range []string{"selected-null", "selected-missing"} {
		if calls[key] == 0 {
			t.Fatalf("nullish expression failed to evaluate its selected callback %q", key)
		}
	}
}

func TestGeneratedStateAndInput(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	view, err := NewCounter(webui.Scope{}, webui.Actions{})
	if err != nil {
		t.Fatal(err)
	}
	window := app.NewWindow("Counter")
	window.Resize(fyne.NewSize(480, 480))
	window.SetContent(view)
	view.Refresh()
	before := view.Object("name")
	tapGenerated(t, view, "increment")
	if view.Object("changed") == nil {
		t.Fatal("useState did not reveal conditional content")
	}
	if before != view.Object("name") {
		t.Fatal("state update recreated the text input")
	}
	input, ok := before.(fyne.Focusable)
	if !ok {
		t.Fatal("generated input is not focusable")
	}
	window.Canvas().Focus(input)
	input.TypedRune('!')
	if window.Canvas().Focused() != input {
		t.Fatal("controlled update lost keyboard focus")
	}
	if err := view.Error(); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureMeasuredGeometry(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	view, err := NewGeometry(webui.Scope{}, webui.Actions{})
	if err != nil {
		t.Fatal(err)
	}
	target := software.NewCanvas()
	target.SetPadded(false)
	target.SetScale(1)
	target.Resize(fyne.NewSize(320, 240))
	target.SetContent(view)
	view.Resize(fyne.NewSize(320, 240))
	if err := view.BindCanvas(target); err != nil {
		t.Fatal(err)
	}
	if err := view.ValidateCanvas(); err != nil {
		t.Fatal(err)
	}
	image := target.Capture()
	out := os.Getenv("ASTRO_FYNE_ARTIFACTS")
	if out == "" {
		t.Skip("set ASTRO_FYNE_ARTIFACTS to export a native comparison capture")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(out, "native.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, image); err != nil {
		t.Fatal(err)
	}
	if err := view.Error(); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureMeasuredImageGeometry(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	view, err := NewImageGeometry(webui.Scope{}, webui.Actions{})
	if err != nil {
		t.Fatal(err)
	}
	target := software.NewCanvas()
	target.SetPadded(false)
	target.SetScale(1)
	target.Resize(fyne.NewSize(320, 240))
	target.SetContent(view)
	view.Resize(fyne.NewSize(320, 240))
	view.BindCanvas(target)
	if err := view.ValidateCanvas(); err != nil {
		t.Fatal(err)
	}
	image := target.Capture()
	object := view.Object("local-image")
	if object == nil || object.Size() != fyne.NewSize(16, 16) {
		t.Fatal("the compiled local image must produce its measured native object")
	}
	if err := view.Error(); err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("ASTRO_FYNE_IMAGE_ARTIFACTS")
	if out == "" {
		t.Skip("set ASTRO_FYNE_IMAGE_ARTIFACTS to export the native image comparison capture")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(out, "native.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, image); err != nil {
		t.Fatal(err)
	}
}
