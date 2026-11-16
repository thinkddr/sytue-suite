// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/zalando/go-keyring"

	"git.sytue.eu/sytue/one/apps/fyne/apps/photos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
)

type castPresenterProbe struct{ background, foreground, repaint, pipEvents int }

func (p *castPresenterProbe) AlFondo()       { p.background++ }
func (p *castPresenterProbe) AlVolver()      { p.foreground++ }
func (p *castPresenterProbe) Repintar()      { p.repaint++ }
func (p *castPresenterProbe) SucesoPiP(bool) { p.pipEvents++ }

// This covers the real Suite/Fyne lifecycle registration, but the mounts below
// are probes: it does not exercise PrivateApplication.Mount or its Detach.
func TestCastLifecycleRegistrationUsesCurrentMount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	keyring.MockInit()
	app := test.NewApp()
	defer app.Quit()
	suite := sesion.Abrir(app, "sytue.eu")
	var views castViews
	views.registerLifecycle(suite)

	lifecycle, ok := app.Lifecycle().(interface {
		OnEnteredForeground() func()
		OnExitedForeground() func()
	})
	if !ok {
		t.Fatal("Fyne lifecycle callbacks unavailable")
	}
	a, b := &castPresenterProbe{}, &castPresenterProbe{}
	first := views.mount(a)
	var queued []func()
	first.poster(func(f func()) { queued = append(queued, f) })("poster")
	lifecycle.OnExitedForeground()()
	first.detach(photos.NuevaFototeca())
	second := views.mount(b)
	if len(queued) != 1 {
		t.Fatal("A's poster callback was not queued")
	}
	queued[0]()
	lifecycle.OnEnteredForeground()()
	lifecycle.OnExitedForeground()()
	second.detach(photos.NuevaFototeca())
	lifecycle.OnEnteredForeground()()
	if a.background != 1 || a.foreground != 0 || b.background != 1 || b.foreground != 1 {
		t.Fatalf("wrong lifecycle delivery: A=%+v B=%+v", a, b)
	}
	if a.repaint != 0 || b.repaint != 0 {
		t.Fatal("A's queued callback repainted a retired or new mount")
	}
}

func TestCastMountRejectsQueuedPosterAfterDetach(t *testing.T) {
	var views castViews
	a, b := &castPresenterProbe{}, &castPresenterProbe{}
	cacheA, cacheB := photos.NuevaFototeca(), photos.NuevaFototeca()
	cacheA.Poner("poster", []byte("private-poster"))
	oldA := views.mount(a)
	var queued []func()
	notifyA := oldA.poster(func(f func()) { queued = append(queued, f) })
	notifyA("poster") // queued for A, before A is detached
	if len(queued) != 1 || !oldA.detach(cacheA) {
		t.Fatal("A did not detach with its poster callback queued")
	}
	if raw, painted := cacheA.Memoria(); raw != 0 || painted != 0 {
		t.Fatalf("detached cache retained entries: %d, %d", raw, painted)
	}
	viewB := views.mount(b)
	queued[0]()
	oldA.pip(true)
	if a.repaint != 0 || b.repaint != 0 {
		t.Fatal("A's queued poster callback repainted a retired or new presenter")
	}
	if a.pipEvents != 0 || b.pipEvents != 0 {
		t.Fatal("retired PiP callback reached a presenter")
	}
	views.background()
	views.foreground()
	if a.background != 0 || a.foreground != 0 || b.background != 1 || b.foreground != 1 {
		t.Fatal("lifecycle callback reached the wrong mount")
	}
	// Even an A→B→A adoption cannot make the original A callback live again.
	newA := views.mount(a)
	if viewB.detach(cacheB) {
		t.Fatal("stale B detach cleared the current mount")
	}
	queued[0]()
	if a.repaint != 0 {
		t.Fatal("old A callback revived after A was mounted again")
	}
	views.foreground()
	if a.foreground != 1 || b.foreground != 1 {
		t.Fatal("stale detach changed the current lifecycle recipient")
	}
	newA.poster(func(f func()) { f() })("poster")
	if a.repaint != 1 {
		t.Fatal("current mount did not receive its poster callback")
	}
	newA.detach(photos.NuevaFototeca())
}

func TestCastDetachCancelsPosterDownload(t *testing.T) {
	var views castViews
	cache := photos.NuevaFototeca()
	t.Cleanup(cache.Vaciar)
	started, canceled := make(chan struct{}), make(chan struct{})
	cache.Pedir = func(ctx context.Context, _, _ string) ([]byte, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	}
	view := views.mount(&castPresenterProbe{})
	cache.Precargar("poster")
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("poster download did not start")
	}
	view.detach(cache)
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("detaching did not cancel the poster request")
	}
	done := make(chan struct{})
	go func() { cache.Esperar(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("poster worker did not drain")
	}
	if raw, painted := cache.Memoria(); raw != 0 || painted != 0 {
		t.Fatalf("detached cache retained entries: %d, %d", raw, painted)
	}
}
