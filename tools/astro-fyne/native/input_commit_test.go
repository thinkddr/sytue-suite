// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package webui

import (
	"reflect"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func focusCommitField(t *testing.T, view *View) (fyne.Window, *inputWidget) {
	t.Helper()
	window := test.NewWindow(view)
	t.Cleanup(window.Close)
	window.SetPadded(false)
	window.Resize(fyne.NewSize(240, 100))
	field, ok := view.Object("field").(*inputWidget)
	if !ok {
		t.Fatalf("native field is not an input: %T", view.Object("field"))
	}
	window.Canvas().Focus(field)
	return window, field
}

func TestCommitOnlyInputRetainsMultipleCharactersUntilBlur(t *testing.T) {
	value := "A"
	commits := []string{}
	v := NewView(func() []Node {
		return []Node{{ID: "field", Kind: "input", Value: value, OnCommit: func(s string) {
			commits = append(commits, s)
			value = s
		}}}
	})
	window, field := focusCommitField(t, v)
	for _, r := range "bc" {
		field.TypedRune(r)
	}
	if value != "A" || len(commits) != 0 || field.element.input.Text() != "Abc" {
		t.Fatalf("input committed early or discarded pending text: state=%q editor=%q commits=%v", value, field.element.input.Text(), commits)
	}
	if window.Canvas().Focused() != field {
		t.Fatal("pending edits lost keyboard focus")
	}
	window.Canvas().Unfocus()
	if value != "Abc" || !reflect.DeepEqual(commits, []string{"Abc"}) {
		t.Fatalf("blur did not commit the complete value exactly once: state=%q commits=%v", value, commits)
	}
	window.Canvas().Focus(field)
	window.Canvas().Unfocus()
	if len(commits) != 1 {
		t.Fatal("focus without edits emitted another change")
	}
	window.Canvas().Focus(field)
	field.TypedRune('d')
	window.Canvas().Unfocus()
	if !reflect.DeepEqual(commits, []string{"Abc", "Abcd"}) {
		t.Fatalf("subsequent focus did not start a new commit baseline: %v", commits)
	}
}

func TestSingleLineReturnCommitsAndBlurDoesNotDuplicate(t *testing.T) {
	value := ""
	commits := []string{}
	v := NewView(func() []Node {
		return []Node{{ID: "field", Kind: "input", Value: value, OnCommit: func(s string) { value = s; commits = append(commits, s) }}}
	})
	window, field := focusCommitField(t, v)
	field.TypedRune('ñ')
	field.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !reflect.DeepEqual(commits, []string{"ñ"}) || window.Canvas().Focused() != field {
		t.Fatalf("Return commit/focus mismatch: commits=%v focus=%T", commits, window.Canvas().Focused())
	}
	field.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEnter})
	window.Canvas().Unfocus()
	if len(commits) != 1 {
		t.Fatal("Return followed by Enter/blur repeated the same change")
	}
	window.Canvas().Focus(field)
	field.TypedRune('!')
	window.Canvas().Unfocus()
	if !reflect.DeepEqual(commits, []string{"ñ", "ñ!"}) {
		t.Fatalf("typing after a commit failed to emit a new value: %v", commits)
	}
}

func TestTextareaReturnEditsWithoutCommitUntilBlur(t *testing.T) {
	value := "line"
	commits := []string{}
	v := NewView(func() []Node {
		return []Node{{ID: "field", Kind: "textarea", Value: value, OnCommit: func(s string) { value = s; commits = append(commits, s) }}}
	})
	window, field := focusCommitField(t, v)
	field.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	field.TypedRune('x')
	if len(commits) != 0 || field.element.input.Text() != "line\nx" {
		t.Fatalf("textarea Return committed or lost its newline: editor=%q commits=%v", field.element.input.Text(), commits)
	}
	window.Canvas().Unfocus()
	if !reflect.DeepEqual(commits, []string{"line\nx"}) {
		t.Fatalf("textarea blur did not commit the edited lines: %v", commits)
	}
}

func TestImmediateInputAndCommitCallbacksHaveSeparateLifecycles(t *testing.T) {
	value := ""
	events := []string{}
	v := NewView(func() []Node {
		return []Node{{ID: "field", Kind: "input", Value: value,
			OnChange: func(s string) { value = s; events = append(events, "input:"+s) },
			OnCommit: func(s string) { value = s; events = append(events, "change:"+s) },
		}}
	})
	window, field := focusCommitField(t, v)
	field.TypedRune('a')
	field.TypedRune('b')
	if !reflect.DeepEqual(events, []string{"input:a", "input:ab"}) || window.Canvas().Focused() != field {
		t.Fatalf("immediate callbacks/controlled focus mismatch: %v", events)
	}
	field.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if !reflect.DeepEqual(events, []string{"input:a", "input:ab", "change:ab"}) || window.Canvas().Focused() != field {
		t.Fatalf("commit did not preserve the immediate callback history and focus: %v", events)
	}
	window.Canvas().Unfocus()
	if len(events) != 3 || v.Object("field") != field {
		t.Fatal("blur duplicated change or replaced the reconciled input")
	}
}

func TestRestoredAndProgrammaticallyChangedInputValuesDoNotCommit(t *testing.T) {
	value := "a"
	commits := 0
	v := NewView(func() []Node {
		return []Node{{ID: "field", Kind: "input", Value: value, OnCommit: func(string) { commits++ }}}
	})
	window, field := focusCommitField(t, v)
	field.TypedRune('b')
	field.TypedKey(&fyne.KeyEvent{Name: fyne.KeyBackspace})
	window.Canvas().Unfocus()
	if commits != 0 {
		t.Fatal("returning to the focus baseline emitted change")
	}
	window.Canvas().Focus(field)
	value = "from parent"
	v.Refresh()
	window.Canvas().Unfocus()
	if commits != 0 || field.element.input.Text() != value {
		t.Fatal("a programmatic value replacement emitted a user change")
	}
}

func TestUncommittedInputEditsInvalidateMeasuredVisualState(t *testing.T) {
	value := "a"
	font := theme.DefaultTheme().Font(fyne.TextStyle{})
	backend := FyneBackend{Fonts: map[string]map[Font]fyne.Resource{"Test": {{Weight: 400}: font}}}
	v := NewView(func() []Node {
		return []Node{{ID: "field", Kind: "input", Value: value, OnCommit: func(s string) { value = s }}}
	}, backend)
	v.SetViewport(240, 100)
	profile := map[string]Style{"field": {Width: 200, Height: 36, FontFamily: "Test", FontSize: 14, LineHeight: 20, FontWeight: 400, Opacity: 1, Measured: true}}
	if err := v.ApplyMeasurements(profile); err != nil {
		t.Fatal(err)
	}
	window, field := focusCommitField(t, v)
	field.TypedRune('b')
	if v.Error() == nil || !strings.Contains(v.Error().Error(), "uncommitted visual value") {
		t.Fatalf("a locally edited measured value retained its certificate: %v", v.Error())
	}
	if err := v.ApplyMeasurements(profile); err == nil {
		t.Fatal("a source-state profile was accepted while the editor displayed pending text")
	}
	window.Canvas().Unfocus()
	if value != "ab" {
		t.Fatal("profile validation interfered with the pending edit's eventual commit")
	}
}

func TestAutoRefreshEventsCanDelegateActionRenderingToGeneratedHandlers(t *testing.T) {
	builds, calls := 0, 0
	v := NewView(func() []Node {
		builds++
		return []Node{{ID: "action", Kind: "button", Text: "Call adapter", OnTap: func() { calls++ }}}
	})
	v.SetAutoRefreshEvents(false)
	tap(t, v.Object("action"))
	if calls != 1 || builds != 1 {
		t.Fatalf("action-only callback caused an implicit render: calls=%d builds=%d", calls, builds)
	}
	v.Refresh()
	if builds != 2 {
		t.Fatal("disabling automatic event refresh disabled explicit refresh")
	}
	v.SetAutoRefreshEvents(true)
	tap(t, v.Object("action"))
	if calls != 2 || builds != 3 {
		t.Fatalf("hand-authored nodes lost automatic event refresh: calls=%d builds=%d", calls, builds)
	}
}

func TestGeneratedStyleCallbacksRefreshInputAndCommitExactlyOnce(t *testing.T) {
	value := ""
	builds, inputs, commits := 0, 0, 0
	var v *View
	v = NewView(func() []Node {
		builds++
		return []Node{{ID: "field", Kind: "input", Value: value,
			OnChange: func(s string) { value = s; inputs++; v.Refresh() },
			OnCommit: func(s string) { value = s; commits++; v.Refresh() },
		}}
	})
	v.SetAutoRefreshEvents(false)
	window, field := focusCommitField(t, v)
	before := builds
	field.TypedRune('a')
	if inputs != 1 || commits != 0 || builds != before+1 || window.Canvas().Focused() != field {
		t.Fatalf("generated input refresh was duplicated or lost focus: input=%d commit=%d builds=%d", inputs, commits, builds-before)
	}
	field.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if inputs != 1 || commits != 1 || builds != before+2 || window.Canvas().Focused() != field {
		t.Fatalf("generated commit refresh was duplicated or lost focus: input=%d commit=%d builds=%d", inputs, commits, builds-before)
	}
	window.Canvas().Unfocus()
	if commits != 1 || builds != before+2 {
		t.Fatal("blur repeated an already confirmed generated callback")
	}
}
