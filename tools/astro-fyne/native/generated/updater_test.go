// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package generated

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	webui "github.com/thinkddr/astro-fyne/native"
)

func updaterView(t *testing.T) *UpdaterConformanceWidget {
	t.Helper()
	app := test.NewApp()
	t.Cleanup(app.Quit)
	view, err := NewUpdaterConformance(webui.Scope{}, webui.Actions{})
	if err != nil {
		t.Fatal(err)
	}
	window := app.NewWindow("Updater conformance")
	window.Resize(fyne.NewSize(800, 1200))
	window.SetContent(view)
	view.Refresh()
	if err := view.Error(); err != nil {
		t.Fatal(err)
	}
	return view
}

func updaterText(t *testing.T, view *UpdaterConformanceWidget, id string) string {
	t.Helper()
	object := view.Object(id)
	if object == nil {
		t.Fatalf("updater node %q is missing", id)
	}
	text, ok := object.(interface{ AccessibilityLabel() string })
	if !ok {
		t.Fatalf("updater node %q has no observable text: %T", id, object)
	}
	return text.AccessibilityLabel()
}

func requireUpdaterText(t *testing.T, view *UpdaterConformanceWidget, expected map[string]string) {
	t.Helper()
	for id, want := range expected {
		if got := updaterText(t, view, id); got != want {
			t.Fatalf("%s = %q, expected %q", id, got, want)
		}
	}
}

func tapUpdater(t *testing.T, view *UpdaterConformanceWidget, id string) {
	t.Helper()
	object := view.Object(id)
	tappable, ok := object.(fyne.Tappable)
	if !ok {
		t.Fatalf("updater action %q is not tappable: %T", id, object)
	}
	test.Tap(tappable)
	if err := view.Error(); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedUpdaterReadsCapturedOtherHookAndIsolatesSiblings(t *testing.T) {
	view := updaterView(t)
	requireUpdaterText(t, view, map[string]string{"left-a": "0", "left-b": "1", "right-a": "0", "right-b": "1"})
	right := view.Object("right-a")
	// b=2 is pending. The updater argument sees pending a=0, while its ordinary
	// reference to b still belongs to the click's render closure, where b=1.
	tapUpdater(t, view, "left-update")
	requireUpdaterText(t, view, map[string]string{"left-a": "1", "left-b": "2", "right-a": "0", "right-b": "1"})
	if right != view.Object("right-a") {
		t.Fatal("updating the left unit recreated its sibling")
	}
	// The second click installs the next render closure, now containing b=2.
	tapUpdater(t, view, "left-update")
	requireUpdaterText(t, view, map[string]string{"left-a": "3", "left-b": "2", "right-a": "0", "right-b": "1"})
	tapUpdater(t, view, "right-update")
	requireUpdaterText(t, view, map[string]string{"left-a": "3", "left-b": "2", "right-a": "1", "right-b": "2"})
}

func TestGeneratedUpdaterReadsPendingArgumentAndCapturedSameHook(t *testing.T) {
	view := updaterView(t)
	requireUpdaterText(t, view, map[string]string{"self-a": "0"})
	// The direct setter queues a=1. Only prev reads this queued value: a itself
	// remains the captured zero, so the functional result is 1+0, not 1+1.
	tapUpdater(t, view, "self-update")
	requireUpdaterText(t, view, map[string]string{"self-a": "1"})
	tapUpdater(t, view, "self-update")
	requireUpdaterText(t, view, map[string]string{"self-a": "2"})
	tapUpdater(t, view, "self-update")
	requireUpdaterText(t, view, map[string]string{"self-a": "3"})
}

func TestGeneratedFunctionalUpdaterParametersComposePendingValues(t *testing.T) {
	view := updaterView(t)
	tapUpdater(t, view, "sequential-update")
	requireUpdaterText(t, view, map[string]string{"sequential-count": "3"})
	tapUpdater(t, view, "sequential-update")
	requireUpdaterText(t, view, map[string]string{"sequential-count": "6"})
}

func TestGeneratedUpdaterParameterShadowingDoesNotRewriteCapturedNames(t *testing.T) {
	view := updaterView(t)
	requireUpdaterText(t, view, map[string]string{"shadow-prev": "2", "shadow-count": "0"})
	// The first prev is a local updater parameter (count=0). The later prev
	// belongs to a separate hook's old render closure (prev=2), despite its setter.
	tapUpdater(t, view, "shadow-update")
	requireUpdaterText(t, view, map[string]string{"shadow-prev": "9", "shadow-count": "3"})
	tapUpdater(t, view, "shadow-update")
	requireUpdaterText(t, view, map[string]string{"shadow-prev": "9", "shadow-count": "13"})
}

func TestGeneratedEventArgumentCannotReplacePendingHookState(t *testing.T) {
	view := updaterView(t)
	object := view.Object("event-input")
	input, ok := object.(fyne.Focusable)
	if !ok {
		t.Fatalf("generated input has no native keyboard contract: %T", object)
	}
	// The HTML event argument is called count, but a functional updater still
	// starts from numeric hook state, never from the event object/string.
	for _, want := range []string{"1", "2"} {
		input.TypedRune('x')
		if err := view.Error(); err != nil {
			t.Fatal(err)
		}
		requireUpdaterText(t, view, map[string]string{"event-count": want})
		if object != view.Object("event-input") {
			t.Fatal("controlled input rerender replaced the native field")
		}
	}
}

func TestGeneratedMappedUpdaterUsesComponentStateAndCapturedRowNames(t *testing.T) {
	view := updaterView(t)
	requireUpdaterText(t, view, map[string]string{"mapped-count": "0", "normal-mapped-total": "0"})
	first, second := view.Object("mapped-0"), view.Object("mapped-1")
	// The row parameter is named count and shadows the component hook. It remains
	// captured row data (10), while prev must read the component's numeric zero.
	tapUpdater(t, view, "mapped-0")
	requireUpdaterText(t, view, map[string]string{"mapped-count": "10", "normal-mapped-total": "0"})
	// A different row must update the same component state, rather than a hidden
	// hook at that row's generated ID prefix. Its captured count now contributes 20.
	tapUpdater(t, view, "mapped-1")
	requireUpdaterText(t, view, map[string]string{"mapped-count": "30", "normal-mapped-total": "0"})
	tapUpdater(t, view, "mapped-0")
	requireUpdaterText(t, view, map[string]string{"mapped-count": "40", "normal-mapped-total": "0"})
	if first != view.Object("mapped-0") || second != view.Object("mapped-1") {
		t.Fatal("mapped state update replaced stable native row controls")
	}
}

func TestGeneratedMappedUpdaterWithoutShadowingStillTargetsOwningComponent(t *testing.T) {
	view := updaterView(t)
	tapUpdater(t, view, "normal-mapped-0")
	requireUpdaterText(t, view, map[string]string{"normal-mapped-total": "10", "mapped-count": "0"})
	tapUpdater(t, view, "normal-mapped-1")
	requireUpdaterText(t, view, map[string]string{"normal-mapped-total": "30", "mapped-count": "0"})
}
