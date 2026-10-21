// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package widgets supplies bounded, non-secret snapshots to native OS widget
// extensions. Providers never receive tokens, message text or arbitrary links.
package widgets

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

const (
	// Version is shared with the WidgetKit, Android and desktop providers.
	Version = 1
	// MaxAge bounds stale information even when the app cannot run in background.
	MaxAge       = 30 * time.Minute
	maxFileBytes = 16 << 10
)

var (
	// ErrPrivate means the session changed, is closed or has OS unlock enabled.
	ErrPrivate = errors.New("widgets: private session")
	// ErrInvalid means an unsupported snapshot or malformed widget identifier.
	ErrInvalid    = errors.New("widgets: invalid snapshot")
	bundlePattern = regexp.MustCompile(`^eu\.sytue\.[a-z][a-z0-9]*$`)
	// Installed during platform initialization, before any snapshot is written.
	notifyPlatform = func(string) {}
	// ErrSinLector: en esta plataforma o en este paquete no hay ningún widget nativo que
	// lea el snapshot, así que no se escribe. Quien publica lo ignora.
	ErrSinLector = errors.New("widgets: no native widget reader on this platform")
	// lector dice si hay un proveedor nativo que lea los snapshots. Lo fija la
	// plataforma en su init: Android siempre (AppWidgetProvider del propio APK); iOS y
	// macOS solo si el host cargó el puente de WidgetKit, que empaquetar.sh enlaza
	// únicamente con SYTUE_WIDGETS_APPLE encendido. Windows y Linux, nunca: no hay una
	// superficie de widgets que leer (ADR 0078 en docs/decisions.md).
	lector = func() bool { return false }
)

// Disponible dice si escribir un snapshot sirve de algo aquí. Sin lector no se escribe
// ni se borra nada: ni ficheros que nadie lee ni el cerrojo de la suite en cada cambio.
func Disponible() bool { return lector() }

// SimularLector es SOLO para tests de otros paquetes: hace como si hubiera un widget
// nativo, para probar la publicación en Linux. Devuelve cómo dejarlo como estaba.
func SimularLector(activo bool) (restaurar func()) {
	anterior := lector
	lector = func() bool { return activo }
	return func() { lector = anterior }
}

// Notify asks the OS to reload its widgets; an empty bundle reloads the suite.
// Rendering is scheduled by the OS. File-watching providers need no extra call.
func Notify(bundle string) {
	if bundle == "" || bundlePattern.MatchString(bundle) {
		notifyPlatform(bundle)
	}
}

// Counter deliberately cannot contain names, subjects, event titles or message
// snippets. Kind is one of the small, stable categories below.
type Counter struct {
	Kind  string `json:"kind"`
	Value int64  `json:"value"`
}

// Snapshot is a disposable view of counters fetched by an authenticated app.
// generation is the opaque settings marker, never a subject, email or token.
type Snapshot struct {
	Version    int       `json:"version"`
	Product    string    `json:"product"`
	Generation string    `json:"generation"`
	UpdatedAt  int64     `json:"updated_at"`
	ExpiresAt  int64     `json:"expires_at"`
	Active     bool      `json:"active"`
	Counters   []Counter `json:"counters"`
}

// Scope is captured BEFORE fetching data. Publishing after a user switch, logout
// or lock change fails closed, even when an older request finishes later.
type Scope struct{ generation string }

// Capture binds a pending fetch to the session revision adopted by the app.
// Pass Estado.GeneracionSesion, never a fresh settings read: the disk marker can
// change before another process has adopted the corresponding identity.
func Capture(adoptedGeneration string) Scope { return Scope{generation: adoptedGeneration} }

// View binds a pending data request to one app and its adopted session. It
// contains no token, identity, network client or configurable destination.
type View struct {
	scope           Scope
	bundle, product string
}

// NewView captures the app metadata alongside a previously adopted scope.
// Call it before requesting the data whose counters will be published.
func NewView(bundle, product string, scope Scope) View {
	return View{scope: scope, bundle: bundle, product: product}
}

// Source supplies the current view without coupling an app to the session UI.
// An absent source keeps standalone and signed-out screens private.
type Source func() View

// View captures the current session before fetching data. A nil source returns
// a private view, so consumers never need a fallback to a fresh disk marker.
func (s Source) View() View {
	if s == nil {
		return View{}
	}
	return s()
}

// Publicar publishes only bounded counters the app has already loaded. Providers
// never fetch private data; expiry limits background staleness.
func (v View) Publicar(ctx context.Context, counters ...Counter) error {
	if v.bundle == "" {
		return ErrPrivate
	}
	return Publish(ctx, v.bundle, v.product, v.scope, counters, 15*time.Minute)
}

func validCounters(counters []Counter) bool {
	if len(counters) > 12 {
		return false
	}
	seen := make(map[string]bool, len(counters))
	for _, c := range counters {
		switch c.Kind {
		case "unread", "files", "events", "documents", "contacts", "notes", "rows", "pending", "channels", "photos", "episodes", "apps", "calendars", "address_books":
		default:
			return false
		}
		if c.Value < 0 || c.Value > 1_000_000_000_000 || seen[c.Kind] {
			return false
		}
		seen[c.Kind] = true
	}
	return true
}

func path(bundle string) (string, error) {
	if !bundlePattern.MatchString(bundle) {
		return "", ErrInvalid
	}
	dir, err := ajustes.Carpeta()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "widgets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, bundle+".json"), nil
}

// Publish atomically replaces one app's snapshot under the existing suite lock.
// Writers share the same settings directory as native providers through the OS
// App Group on Apple; Android providers use their own app's private directory.
func Publish(ctx context.Context, bundle, product string, scope Scope, counters []Counter, age time.Duration) (result error) {
	if !lector() {
		return ErrSinLector
	}
	if !api.EsProducto(product) || age <= 0 || age > MaxAge || !validCounters(counters) {
		return ErrInvalid
	}
	destination, err := path(bundle)
	if err != nil {
		return err
	}
	release, err := ajustes.Cerrojo(ctx)
	if err != nil {
		return err
	}
	defer func() {
		release()
		if result == nil {
			Notify(bundle)
		}
	}()
	state := ajustes.Leer()
	if state.Bloqueo || scope.generation == "" || state.Sesion != scope.generation {
		return ErrPrivate
	}
	now := time.Now()
	s := Snapshot{Version: Version, Product: product, Generation: scope.generation, UpdatedAt: now.Unix(), ExpiresAt: now.Add(age).Unix(), Active: true, Counters: append([]Counter{}, counters...)}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return ajustes.EscribirEntero(destination, data)
}

// Visible is also the contract implemented by native providers. Counters are
// hidden for expired, future-dated, locked, mismatched or oversized snapshots.
func (s Snapshot) Visible(generation string, locked bool, now time.Time) bool {
	return !locked && s.Active && s.Version == Version && api.EsProducto(s.Product) && generation != "" && s.Generation == generation && s.UpdatedAt <= now.Unix()+5 && s.UpdatedAt > 0 && s.ExpiresAt > now.Unix() && s.ExpiresAt > s.UpdatedAt && s.ExpiresAt-s.UpdatedAt <= int64(MaxAge/time.Second) && validCounters(s.Counters)
}

// Read loads a widget snapshot only when its shared session marker is current.
// Native extensions implement the same checks without opening the keychain.
func Read(bundle string) (Snapshot, error) {
	filename, err := path(bundle)
	if err != nil {
		return Snapshot{}, err
	}
	f, err := os.Open(filename)
	if err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.Size() > maxFileBytes {
		return Snapshot{}, ErrInvalid
	}
	var s Snapshot
	decoder := json.NewDecoder(io.LimitReader(f, maxFileBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&s); err != nil {
		return Snapshot{}, ErrInvalid
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Snapshot{}, ErrInvalid
	}
	state := ajustes.Leer()
	if !s.Visible(state.Sesion, state.Bloqueo, time.Now()) {
		return Snapshot{}, ErrPrivate
	}
	return s, nil
}

// Clear removes every cached counter at logout or when OS unlock is enabled.
// Providers still check the marker and lock flag, so a failed removal cannot
// make a stale snapshot visible to the next account.
func Clear(ctx context.Context) error {
	return clearSnapshots(ctx, nil)
}

// ClearIf removes snapshots only while the adopted revision is still current.
// A late logout/lock callback must not erase a freshly connected account's data.
func ClearIf(ctx context.Context, adoptedGeneration string) error {
	return clearSnapshots(ctx, &adoptedGeneration)
}

func clearSnapshots(ctx context.Context, expected *string) error {
	if !lector() {
		return nil
	}
	dir, err := ajustes.Carpeta()
	if err != nil {
		return err
	}
	release, err := ajustes.Cerrojo(ctx)
	if err != nil {
		return err
	}
	defer func() { release(); Notify("") }()
	if expected != nil && ajustes.Leer().Sesion != *expected {
		return ErrPrivate
	}
	entries, err := os.ReadDir(filepath.Join(dir, "widgets"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var failures []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || !bundlePattern.MatchString(strings.TrimSuffix(entry.Name(), ".json")) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, "widgets", entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
