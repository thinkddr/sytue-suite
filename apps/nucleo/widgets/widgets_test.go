// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package widgets

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

func fixture(t *testing.T) Scope {
	t.Helper()
	t.Cleanup(SimularLector(true))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// En macOS os.UserConfigDir sale de $HOME, no de XDG: sin esto el test escribiría
	// en la carpeta real de quien lo corre.
	t.Setenv("HOME", t.TempDir())
	ajustes.MarcarSesion()
	return Capture(ajustes.Leer().Sesion)
}

func TestSourcePreservesCapturedSessionAndFullCollectionCounts(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	var absent Source
	if err := absent.View().Publicar(ctx); !errors.Is(err, ErrPrivate) {
		t.Fatalf("absent source published: %v", err)
	}
	source := Source(func() View { return NewView("eu.sytue.calendar", "calendar", scope) })
	beforeFetch := source.View()
	if err := beforeFetch.Publicar(ctx, Counter{Kind: "calendars", Value: 3}, Counter{Kind: "address_books", Value: 2}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Read("eu.sytue.calendar")
	if err != nil || len(snapshot.Counters) != 2 {
		t.Fatalf("complete collection counts: %+v %v", snapshot, err)
	}
	ajustes.MarcarSesion()
	if err := beforeFetch.Publicar(ctx, Counter{Kind: "calendars", Value: 99}); !errors.Is(err, ErrPrivate) {
		t.Fatalf("pending view adopted a newer identity: %v", err)
	}
}

func TestSnapshotRejectsLateOldAccountResponses(t *testing.T) {
	old := fixture(t)
	ctx := context.Background()
	if err := Publish(ctx, "eu.sytue.mail", "mail", old, []Counter{{Kind: "unread", Value: 7}}, time.Minute); err != nil {
		t.Fatal(err)
	}
	s, err := Read("eu.sytue.mail")
	if err != nil || len(s.Counters) != 1 || s.Counters[0].Value != 7 {
		t.Fatalf("snapshot: %+v %v", s, err)
	}
	// A logout/new login invalidates both existing snapshots AND any older
	// request still completing. The old writer cannot stamp the new marker.
	ajustes.MarcarSesion()
	if _, err := Read("eu.sytue.mail"); !errors.Is(err, ErrPrivate) {
		t.Fatalf("old account still visible: %v", err)
	}
	if err := Publish(ctx, "eu.sytue.mail", "mail", old, []Counter{{Kind: "unread", Value: 99}}, time.Minute); !errors.Is(err, ErrPrivate) {
		t.Fatalf("late old response accepted: %v", err)
	}
	if err := Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.mail"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("logout retained data: %v", err)
	}
}

func TestLateLogoutCannotClearNewAccountSnapshot(t *testing.T) {
	old := fixture(t)
	oldGeneration := old.generation
	ajustes.MarcarSesion()
	fresh := Capture(ajustes.Leer().Sesion)
	ctx := context.Background()
	if err := Publish(ctx, "eu.sytue.mail", "mail", fresh, []Counter{{Kind: "unread", Value: 3}}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := ClearIf(ctx, oldGeneration); !errors.Is(err, ErrPrivate) {
		t.Fatalf("late logout: %v", err)
	}
	current, err := Read("eu.sytue.mail")
	if err != nil || len(current.Counters) != 1 || current.Counters[0].Value != 3 {
		t.Fatalf("new account cleared: %+v %v", current, err)
	}
	if err := ClearIf(ctx, fresh.generation); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.mail"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("logout retained counters: %v", err)
	}
}

func TestSnapshotLockExpiryAndStrictSchema(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	if err := Publish(ctx, "eu.sytue.calendar", "calendar", scope, []Counter{{Kind: "events", Value: 3}}, time.Minute); err != nil {
		t.Fatal(err)
	}
	s, err := Read("eu.sytue.calendar")
	if err != nil {
		t.Fatal(err)
	}
	if s.Visible(s.Generation, false, time.Now().Add(2*time.Minute)) || s.Visible(s.Generation, true, time.Now()) || s.Visible("other", false, time.Now()) {
		t.Fatal("expired/locked/foreign snapshot visible")
	}
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.Bloqueo = true }); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.calendar"); !errors.Is(err, ErrPrivate) {
		t.Fatalf("lock did not hide snapshot: %v", err)
	}
	if err := Publish(ctx, "eu.sytue.calendar", "calendar", scope, nil, time.Minute); !errors.Is(err, ErrPrivate) {
		t.Fatalf("writer bypassed lock: %v", err)
	}
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.Bloqueo = false }); err != nil {
		t.Fatal(err)
	}
	filename, err := path("eu.sytue.calendar")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"access_token":"should-never-exist"}`)...)
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.calendar"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown secret field accepted: %v", err)
	}
}

func TestSnapshotBoundsAndPrivateFilePermissions(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	for _, bundle := range []string{"../private", "eu.sytue.mail/../../other", "foreign.app", "eu.sytue.mail.widgets"} {
		if err := Publish(ctx, bundle, "mail", scope, nil, time.Minute); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad bundle accepted: %s %v", bundle, err)
		}
	}
	for _, counters := range [][]Counter{{{Kind: "subject", Value: 1}}, {{Kind: "unread", Value: -1}}, {{Kind: "unread", Value: 1}, {Kind: "unread", Value: 2}}} {
		if err := Publish(ctx, "eu.sytue.mail", "mail", scope, counters, time.Minute); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad counters: %+v %v", counters, err)
		}
	}
	if err := Publish(ctx, "eu.sytue.mail", "mail", scope, nil, MaxAge+time.Second); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded TTL: %v", err)
	}
	if err := Publish(ctx, "eu.sytue.mail", "mail", scope, nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	filename, _ := path("eu.sytue.mail")
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot permissions: %o", info.Mode().Perm())
	}
	info, err = os.Stat(filepath.Dir(filename))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("directory permissions: %v %v", info, err)
	}
	if err := os.WriteFile(filename, []byte(strings.Repeat("x", maxFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.mail"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded input: %v", err)
	}
}

// Sin un widget nativo que lo lea (Windows, Linux, un Mac o un iPhone sin el puente de
// WidgetKit), no se escribe ni se borra nada. Visto en rojo quitando la guarda de Publish.
func TestSinLectorNoSeEscribeNada(t *testing.T) {
	scope := fixture(t)
	restaurar := SimularLector(false)
	defer restaurar()
	if Disponible() {
		t.Fatal("Disponible no refleja que no hay lector")
	}
	err := Publish(context.Background(), "eu.sytue.mail", "mail", scope, []Counter{{Kind: "unread", Value: 1}}, time.Minute)
	if !errors.Is(err, ErrSinLector) {
		t.Fatalf("sin lector se publica igual: %v", err)
	}
	dir, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "widgets")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sin lector se crea la carpeta de snapshots: %v", err)
	}
	if err := Clear(context.Background()); err != nil {
		t.Fatalf("sin lector, borrar no es un no-op: %v", err)
	}
}

// Sin init de plataforma (Linux y Windows), el lector por defecto es «no hay».
func TestWindowsYLinuxNoTienenLector(t *testing.T) {
	if runtime.GOOS == "android" || runtime.GOOS == "ios" || runtime.GOOS == "darwin" {
		t.Skip("plataforma con lector posible")
	}
	if Disponible() {
		t.Fatal("una plataforma sin widgets nativos dice tener lector")
	}
}
