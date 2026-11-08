// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package widgets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
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
	return entrar(t, "ana")
}

// entrar guarda la sesión de `sujeto` como lo hace auth: la marca y, después, la
// época confirmada. Devuelve la captura de quien la adopta.
func entrar(t *testing.T, sujeto string) Scope {
	t.Helper()
	ajustes.MarcarSesion()
	if err := ajustes.MarcarIdentidad(ajustes.Identidad{Sujeto: sujeto}); err != nil {
		t.Fatal(err)
	}
	return Capture(ajustes.Leer().Epoca)
}

// publicarContadores es lo que hacían Publish y View.Publicar antes de que solo
// publicara el Publicador: solo contadores, con el ámbito ya capturado.
func publicarContadores(ctx context.Context, bundle, product string, scope Scope, counters []Counter, age time.Duration) error {
	return PublishDetail(ctx, bundle, product, scope, Detail{Counters: counters}, age)
}

func (v View) publicarPrueba(ctx context.Context, counters ...Counter) error {
	if v.bundle == "" {
		return ErrPrivate
	}
	return publicarContadores(ctx, v.bundle, v.product, v.scope, counters, Vigencia)
}

func TestSourcePreservesCapturedSessionAndFullCollectionCounts(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	var absent Source
	if err := absent.View().publicarPrueba(ctx); !errors.Is(err, ErrPrivate) {
		t.Fatalf("absent source published: %v", err)
	}
	source := Source(func() View { return NewView("eu.sytue.calendar", "calendar", scope) })
	beforeFetch := source.View()
	if err := beforeFetch.publicarPrueba(ctx, Counter{Kind: "calendars", Value: 3}, Counter{Kind: "address_books", Value: 2}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Read("eu.sytue.calendar")
	if err != nil || len(snapshot.Counters) != 2 {
		t.Fatalf("complete collection counts: %+v %v", snapshot, err)
	}
	// Renovar el token (otra marca, la misma persona) no invalida la captura: era el
	// widget de Calendar que dejaba de publicar a los ~15 min con la app abierta.
	entrar(t, "ana")
	if err := beforeFetch.publicarPrueba(ctx, Counter{Kind: "calendars", Value: 4}); err != nil {
		t.Fatalf("a renewal invalidated the captured identity: %v", err)
	}
	if s, err := Read("eu.sytue.calendar"); err != nil || s.Counters[0].Value != 4 {
		t.Fatalf("the renewed snapshot is not visible to the reader: %+v %v", s, err)
	}
	// Another person did.
	entrar(t, "bea")
	if err := beforeFetch.publicarPrueba(ctx, Counter{Kind: "calendars", Value: 99}); !errors.Is(err, ErrPrivate) {
		t.Fatalf("pending view adopted a newer identity: %v", err)
	}
}

func TestSnapshotRejectsLateOldAccountResponses(t *testing.T) {
	old := fixture(t)
	ctx := context.Background()
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", old, []Counter{{Kind: "unread", Value: 7}}, time.Minute); err != nil {
		t.Fatal(err)
	}
	s, err := Read("eu.sytue.mail")
	if err != nil || len(s.Counters) != 1 || s.Counters[0].Value != 7 {
		t.Fatalf("snapshot: %+v %v", s, err)
	}
	// A logout/new login invalidates both existing snapshots AND any older
	// request still completing. The old writer cannot stamp the new marker.
	entrar(t, "bea")
	if _, err := Read("eu.sytue.mail"); !errors.Is(err, ErrPrivate) {
		t.Fatalf("old account still visible: %v", err)
	}
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", old, []Counter{{Kind: "unread", Value: 99}}, time.Minute); !errors.Is(err, ErrPrivate) {
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
	fresh := entrar(t, "bea")
	ctx := context.Background()
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", fresh, []Counter{{Kind: "unread", Value: 3}}, time.Minute); err != nil {
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
	if err := publicarContadores(ctx, "eu.sytue.calendar", "calendar", scope, []Counter{{Kind: "events", Value: 3}}, time.Minute); err != nil {
		t.Fatal(err)
	}
	s, err := Read("eu.sytue.calendar")
	if err != nil {
		t.Fatal(err)
	}
	e, err := entornoActual()
	if err != nil {
		t.Fatal(err)
	}
	bloqueado, ajeno := e, e
	bloqueado.Bloqueo, ajeno.Epoca = true, "other"
	if !s.Visible(e, time.Now()) || s.Visible(e, time.Now().Add(2*time.Minute)) || s.Visible(bloqueado, time.Now()) || s.Visible(ajeno, time.Now()) {
		t.Fatal("expired/locked/foreign snapshot visible")
	}
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.Bloqueo = true }); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.calendar"); !errors.Is(err, ErrPrivate) {
		t.Fatalf("lock did not hide snapshot: %v", err)
	}
	if err := publicarContadores(ctx, "eu.sytue.calendar", "calendar", scope, nil, time.Minute); !errors.Is(err, ErrPrivate) {
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
		if err := publicarContadores(ctx, bundle, "mail", scope, nil, time.Minute); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad bundle accepted: %s %v", bundle, err)
		}
	}
	for _, counters := range [][]Counter{{{Kind: "subject", Value: 1}}, {{Kind: "unread", Value: -1}}, {{Kind: "unread", Value: 1}, {Kind: "unread", Value: 2}}, {{Kind: "storage_percent", Value: 101}}} {
		if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, counters, time.Minute); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad counters: %+v %v", counters, err)
		}
	}
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, nil, MaxAge+time.Second); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded TTL: %v", err)
	}
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, nil, time.Minute); err != nil {
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

func TestStoragePercentUsesCompleteValidQuotaWithoutOverflow(t *testing.T) {
	for _, tc := range []struct {
		used, capacity int64
		percent        int64
		valid          bool
	}{
		{0, 100, 0, true},
		{13, 40, 32, true},
		{99, 100, 99, true},
		{100, 100, 100, true},
		{101, 100, 100, true},
		{math.MaxInt64 - 1, math.MaxInt64, 99, true},
		{math.MaxInt64 / 2, math.MaxInt64, 49, true},
		{-1, 100, 0, false},
		{1, 0, 0, false},
		{1, -1, 0, false},
	} {
		counter, valid := StoragePercent(tc.used, tc.capacity)
		if valid != tc.valid || valid && counter != (Counter{Kind: "storage_percent", Value: tc.percent}) {
			t.Fatalf("quota %d/%d: %+v, %t", tc.used, tc.capacity, counter, valid)
		}
	}
}

func TestUnchangedRefreshKeepsObservationAndCoalescesReloads(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	previousNotify := notifyPlatform
	t.Cleanup(func() { notifyPlatform = previousNotify })
	reloads := 0
	notifyPlatform = func(string) { reloads++ }
	publish := func(value int64) {
		t.Helper()
		if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, []Counter{{Kind: "unread", Value: value}}, 15*time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	publish(7)
	filename, _ := path("eu.sytue.mail")
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	publish(7)
	after, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(before, after) || reloads != 1 {
		t.Fatalf("identical refresh renewed or reloaded: %q %q %d %v", before, after, reloads, err)
	}
	publish(8)
	current, err := Read("eu.sytue.mail")
	if err != nil || current.Counters[0].Value != 8 || reloads != 2 {
		t.Fatalf("changed data delayed: %+v %d %v", current, reloads, err)
	}
	// An identical authenticated response near expiry must still renew. This also
	// proves coalescing cannot turn a valid fresh fetch into an expired widget.
	current.UpdatedAt = time.Now().Add(-12 * time.Minute).Unix()
	current.ExpiresAt = current.UpdatedAt + int64(15*time.Minute/time.Second)
	data, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	publish(8)
	current, err = Read("eu.sytue.mail")
	if err != nil || current.ExpiresAt < time.Now().Add(14*time.Minute).Unix() || reloads != 3 {
		t.Fatalf("fresh observation did not renew: %+v %d %v", current, reloads, err)
	}
}

func TestShorterLifetimeCannotPreserveLongerCachedExpiry(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	counters := []Counter{{Kind: "unread", Value: 7}}
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, counters, MaxAge); err != nil {
		t.Fatal(err)
	}
	before, err := Read("eu.sytue.mail")
	if err != nil {
		t.Fatal(err)
	}
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, counters, time.Minute); err != nil {
		t.Fatal(err)
	}
	after, err := Read("eu.sytue.mail")
	if err != nil || after.ExpiresAt >= before.ExpiresAt || after.ExpiresAt > time.Now().Add(time.Minute).Unix() {
		t.Fatalf("shorter lifetime retained a longer expiry: before=%+v after=%+v %v", before, after, err)
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
	err := publicarContadores(context.Background(), "eu.sytue.mail", "mail", scope, []Counter{{Kind: "unread", Value: 1}}, time.Minute)
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

func TestDetailSnapshotItemsCalendarSanitizationAndReloads(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	previousNotify := notifyPlatform
	t.Cleanup(func() { notifyPlatform = previousNotify })
	reloads := 0
	notifyPlatform = func(string) { reloads++ }

	items := SanitizeItems([]Item{
		{Title: "  Revisión\ntrimestral  ", Subtitle: "Sala\tNorte", Badge: " 10:00 ", Color: "#93E", StartAt: 1700000000, EndAt: 1700003600},
		{Title: "enc:v1:secreto-cifrado", Subtitle: "no debe salir"},
		{Title: "Nota abierta", Subtitle: "ENC:V1:cuerpo-cifrado", Badge: "MD"},
	})
	if len(items) != 2 || items[0].Title != "Revisión trimestral" || items[0].Subtitle != "Sala Norte" || items[0].Color != "#9933ee" {
		t.Fatalf("unexpected sanitized items: %+v", items)
	}
	if items[1].Subtitle != "" {
		t.Fatalf("encrypted subtitle leaked: %+v", items[1])
	}

	cal := BuildCalendarGrid(2026, time.February, []int{18, 3, 18, 30, 0}, map[int]string{
		3:  "#953BE7",
		18: "invalid",
		30: "#16734d",
	})
	if cal == nil || len(cal.BusyDays) != 2 || cal.BusyDays[0] != 3 || cal.BusyDays[1] != 18 || cal.DayColors["3"] != "#953be7" || len(cal.DayColors) != 1 {
		t.Fatalf("unexpected calendar grid: %+v", cal)
	}

	detail := Detail{
		Counters: []Counter{{Kind: "calendars", Value: 2}, {Kind: "events", Value: 1}},
		Items:    items,
		Calendar: cal,
	}
	if err := PublishDetail(ctx, "eu.sytue.calendar", "calendar", scope, detail, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	if reloads != 1 {
		t.Fatalf("expected 1 reload, got %d", reloads)
	}
	// Identical detail coalesces without rewriting or reloading.
	if err := PublishDetail(ctx, "eu.sytue.calendar", "calendar", scope, detail, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	if reloads != 1 {
		t.Fatalf("identical detail reloaded: %d", reloads)
	}
	// Changing an item title with identical counters triggers an immediate reload.
	detail.Items = []Item{{Title: "Demo de producto", Color: "#953be7", AllDay: true}}
	if err := PublishDetail(ctx, "eu.sytue.calendar", "calendar", scope, detail, 15*time.Minute); err != nil {
		t.Fatal(err)
	}
	readBack, err := Read("eu.sytue.calendar")
	if err != nil || reloads != 2 || len(readBack.Items) != 1 || readBack.Items[0].Title != "Demo de producto" || readBack.Calendar == nil || readBack.Calendar.Month != 2 {
		t.Fatalf("updated detail not persisted: %+v reloads=%d err=%v", readBack, reloads, err)
	}

	// Direct publication of ciphertext, control chars, >MaxItems or impossible calendar dates fails closed.
	badDetails := []Detail{
		{Items: []Item{{Title: "enc:v1:ciphertext"}}},
		{Items: []Item{{Title: "line\nbreak"}}},
		{Items: []Item{{Title: "bad color", Color: "red"}}},
		{Items: make([]Item, MaxItems+1)},
		{Calendar: &CalendarGrid{Year: 2026, Month: 2, BusyDays: []int{29}}},
		{Calendar: &CalendarGrid{Year: 2026, Month: 3, BusyDays: []int{10, 5}}},
		{Calendar: &CalendarGrid{Year: 2026, Month: 3, DayColors: map[string]string{"01": "#953be7"}}},
	}
	for i := range badDetails[3].Items {
		badDetails[3].Items[i] = Item{Title: "ok"}
	}
	for _, bad := range badDetails {
		if err := PublishDetail(ctx, "eu.sytue.calendar", "calendar", scope, bad, time.Minute); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid detail accepted: %+v -> %v", bad, err)
		}
	}
}

// TestUnaMarcaSinConfirmarFallaCerrado: si la sesión la cambia una compilación que no
// conoce la época (escribe la marca y no la época), no se publica con la época vieja.
func TestUnaMarcaSinConfirmarFallaCerrado(t *testing.T) {
	scope := fixture(t)
	ajustes.MarcarSesion()
	err := publicarContadores(context.Background(), "eu.sytue.mail", "mail", scope, []Counter{{Kind: "unread", Value: 1}}, Vigencia)
	if !errors.Is(err, ErrPrivate) {
		t.Fatalf("published under an unconfirmed session marker: %v", err)
	}
}

// TestUnPayloadIgualSeReescribeTrasRenovarOPasadoRenovarTras: el mismo contenido no se
// reescribe durante RenovarTras con la misma época, tampoco tras una renovación (v2
// lleva la época, que no cambia al renovar). Pasado RenovarTras sí, para que
// `updated_at` diga cuándo se comprobó.
func TestUnPayloadIgualSeReescribeTrasRenovarOPasadoRenovarTras(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	counters := []Counter{{Kind: "unread", Value: 2}}
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, counters, Vigencia); err != nil {
		t.Fatal(err)
	}
	filename, _ := path("eu.sytue.mail")
	antes, _ := os.ReadFile(filename)
	entrar(t, "ana") // renovación
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, counters, Vigencia); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.mail"); err != nil {
		t.Fatalf("a renewal hid the snapshot: %v", err)
	}
	if despues, _ := os.ReadFile(filename); !bytes.Equal(antes, despues) {
		t.Fatal("a renewal rewrote an identical payload: v2 carries the epoch, not the session marker")
	}
	s, _ := readFile(filename)
	s.UpdatedAt = time.Now().Add(-RenovarTras - time.Minute).Unix()
	data, _ := json.Marshal(s)
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, counters, Vigencia); err != nil {
		t.Fatal(err)
	}
	if nuevo, _ := readFile(filename); nuevo.UpdatedAt <= s.UpdatedAt {
		t.Fatalf("an identical payload older than RenovarTras was not rewritten: %d", nuevo.UpdatedAt)
	}
}

// TestClearBundleIfSoloBorraEsaApp: un rechazo de la sesión en una app no vacía el
// widget de las demás.
func TestClearBundleIfSoloBorraEsaApp(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	for _, b := range []struct{ bundle, product string }{{"eu.sytue.mail", "mail"}, {"eu.sytue.calendar", "calendar"}} {
		if err := publicarContadores(ctx, b.bundle, b.product, scope, nil, Vigencia); err != nil {
			t.Fatal(err)
		}
	}
	if err := ClearBundleIf(ctx, "eu.sytue.mail", scope.generation); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.mail"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mail snapshot kept: %v", err)
	}
	if _, err := Read("eu.sytue.calendar"); err != nil {
		t.Fatalf("another app's snapshot was cleared: %v", err)
	}
	if err := ClearBundleIf(ctx, "../fuera", scope.generation); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a malformed bundle reached the filesystem: %v", err)
	}
}

// TestSinPoderLeerLaEpocaNoSeBorraNada es el lanzamiento en frío de iOS con el teléfono
// bloqueado: `sesion.epoca` está ahí pero no se deja leer (NSFileProtectionComplete da
// EPERM), ajustes.Leer da la época vacía y la app, sin sesión legible, pedía
// ClearIf(""). Lo vacío coincidía y se borraban los widgets de las quince. Ahora, sin
// poder leer la época, ni ClearIf ni ClearBundleIf borran; Clear (salir, bloquear) sí.
//
// VISTO EN ROJO: sin la comprobación de ajustes.EpocaLegible en clearSnapshots,
// ClearIf("") devuelve nil y el snapshot desaparece.
func TestSinPoderLeerLaEpocaNoSeBorraNada(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, []Counter{{Kind: "unread", Value: 4}}, Vigencia); err != nil {
		t.Fatal(err)
	}
	dir, err := ajustes.Carpeta()
	if err != nil {
		t.Fatal(err)
	}
	epoca := filepath.Join(dir, "sesion.epoca")
	crudo, err := os.ReadFile(epoca)
	if err != nil {
		t.Fatal(err)
	}
	// Ilegible también para root, que es quien corre las pruebas en el CI: un chmod 000
	// no le impide leer, un directorio en su sitio sí.
	if err := os.Remove(epoca); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(epoca, 0o700); err != nil {
		t.Fatal(err)
	}
	if ajustes.EpocaLegible() == nil || ajustes.Leer().Epoca != "" {
		t.Fatal("la prueba no deja la época ilegible")
	}
	snapshot := filepath.Join(dir, "widgets", "eu.sytue.mail.json")
	for _, borrar := range []func() error{
		func() error { return ClearIf(ctx, "") },
		func() error { return ClearIf(ctx, scope.generation) },
		func() error { return ClearBundleIf(ctx, "eu.sytue.mail", "") },
	} {
		if err := borrar(); !errors.Is(err, ErrPrivate) {
			t.Fatalf("sin poder leer la época: %v", err)
		}
		if _, err := os.Stat(snapshot); err != nil {
			t.Fatalf("sin poder leer la época se ha borrado el widget: %v", err)
		}
	}

	// Desbloqueado, la época vuelve y ClearIf con la suya borra como siempre.
	if err := os.Remove(epoca); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(epoca, crudo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ClearIf(ctx, scope.generation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snapshot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("con la época legible no se ha borrado: %v", err)
	}
	// Y una época que no existe es «nadie ha entrado»: ClearIf("") borra.
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, nil, Vigencia); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(epoca); err != nil {
		t.Fatal(err)
	}
	if err := ClearIf(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(snapshot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sin época, ClearIf(\"\") no ha borrado: %v", err)
	}
}

// TestLoQueNoSeReescribeLlegaVivoAlSiguienteLatido: un payload idéntico se salta la
// escritura, pero solo si el snapshot sigue Visible cuando llegue la siguiente ronda
// del Publicador (latido + espera de un aviso + plazo de la ronda). El caso del
// revisor: escrito en t=0, una ronda idéntica a los 4:59 lo dejaba sin tocar y el
// latido llegaba con el widget ya caducado.
func TestLoQueNoSeReescribeLlegaVivoAlSiguienteLatido(t *testing.T) {
	t0 := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	detalle := Detail{Counters: []Counter{{Kind: "unread", Value: 1}}}
	s := Snapshot{
		Version: Version, Product: "mail", Generation: "g", Active: true,
		UpdatedAt: t0.Unix(), ExpiresAt: t0.Add(Vigencia).Unix(), Counters: detalle.Counters,
	}
	e := Entorno{Epoca: "g", Confirmada: "m", Marca: "m"}
	reusadas := 0
	for d := time.Duration(0); d <= Vigencia; d += time.Second {
		now := t0.Add(d)
		if !s.reusable("mail", e, detalle, now, Vigencia) {
			continue
		}
		reusadas++
		siguiente := now.Add(LatidoWidget + EsperaMaxWidget + PlazoWidget)
		if !s.Visible(e, siguiente) {
			t.Fatalf("se reusa a los %v y el widget ya está en blanco cuando llega el latido", d)
		}
	}
	if reusadas == 0 {
		t.Fatal("nunca se reusa: cada ronda idéntica gastaría el presupuesto de recargas")
	}
}

// TestElFormatoDosLlevaLaEpocaYDuraUnDia: lo que se escribe es la versión 2 con la
// época (no la marca de sesión) y vive 24 h; un v1 que siga en disco no se enseña, y
// un `*.cerrada` en la carpeta (un cierre de sesión) lo oculta todo.
func TestElFormatoDosLlevaLaEpocaYDuraUnDia(t *testing.T) {
	scope := fixture(t)
	ctx := context.Background()
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, []Counter{{Kind: "unread", Value: 2}}, Vigencia); err != nil {
		t.Fatal(err)
	}
	filename, _ := path("eu.sytue.mail")
	s, err := readFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	estado := ajustes.Leer()
	if s.Version != 2 || s.Generation != estado.Epoca || s.Generation == estado.Sesion ||
		s.ExpiresAt-s.UpdatedAt != int64(24*time.Hour/time.Second) {
		t.Fatalf("snapshot escrito: %+v (época %q, marca %q)", s, estado.Epoca, estado.Sesion)
	}
	if _, err := Read("eu.sytue.mail"); err != nil {
		t.Fatal(err)
	}
	dir, _ := ajustes.Carpeta()
	cierre := filepath.Join(dir, "sesion-0123456789abcdef.cerrada")
	if err := os.WriteFile(cierre, []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.mail"); !errors.Is(err, ErrPrivate) {
		t.Fatalf("con una sesión cerrada el snapshot sigue visible: %v", err)
	}
	if err := publicarContadores(ctx, "eu.sytue.mail", "mail", scope, []Counter{{Kind: "unread", Value: 3}}, Vigencia); !errors.Is(err, ErrPrivate) {
		t.Fatalf("con una sesión cerrada se publica: %v", err)
	}
	if err := os.Remove(cierre); err != nil {
		t.Fatal(err)
	}
	s.Version = 1
	data, _ := json.Marshal(s)
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("eu.sytue.mail"); !errors.Is(err, ErrPrivate) {
		t.Fatalf("un snapshot v1 sigue visible: %v", err)
	}
}
