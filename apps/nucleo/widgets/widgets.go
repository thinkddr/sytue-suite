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
	"maps"
	"math/bits"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

const (
	// Version is shared with the WidgetKit, Android and desktop providers.
	Version = 1
	// MaxAge bounds stale information even when the app cannot run in background.
	MaxAge           = 30 * time.Minute
	MaxItems         = 8
	MaxTitleBytes    = 120
	MaxSubtitleBytes = 160
	MaxBadgeBytes    = 24
	maxTimestamp     = 10_000_000_000
	maxFileBytes     = 16 << 10
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

// StoragePercent describes a complete quota already fetched by the app. It never
// exposes capacity or identifiers. Saturation also covers an exceeded quota;
// multiplication uses 128 bits so a valid large capacity cannot wrap around.
func StoragePercent(used, capacity int64) (Counter, bool) {
	if used < 0 || capacity <= 0 {
		return Counter{}, false
	}
	percent := uint64(100)
	if used < capacity {
		hi, lo := bits.Mul64(uint64(used), 100)
		percent, _ = bits.Div64(hi, lo, uint64(capacity))
	}
	return Counter{Kind: "storage_percent", Value: int64(percent)}, true
}

// Snapshot is a disposable view of counters and bounded visual items fetched by
// an authenticated app. generation is the opaque settings marker, never a
// subject, email or token.
type Snapshot struct {
	Version    int           `json:"version"`
	Product    string        `json:"product"`
	Generation string        `json:"generation"`
	UpdatedAt  int64         `json:"updated_at"`
	ExpiresAt  int64         `json:"expires_at"`
	Active     bool          `json:"active"`
	Counters   []Counter     `json:"counters"`
	Items      []Item        `json:"items,omitempty"`
	Calendar   *CalendarGrid `json:"calendar,omitempty"`
}

// Item is a compact, sanitized visual entry already loaded by an authenticated
// app (for example an upcoming calendar event, inbox conversation header, recent
// file or task). It never contains tokens, ciphertext ("enc:v1:"), message bodies
// or arbitrary URLs.
type Item struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Badge    string `json:"badge,omitempty"`
	Color    string `json:"color,omitempty"`
	StartAt  int64  `json:"start_at,omitempty"`
	EndAt    int64  `json:"end_at,omitempty"`
	AllDay   bool   `json:"all_day,omitempty"`
	Unread   bool   `json:"unread,omitempty"`
	Done     bool   `json:"done,omitempty"`
}

// CalendarGrid describes the displayed month's busy days and accent colors so
// native widgets can draw a graphical month grid with event indicators.
type CalendarGrid struct {
	Year      int               `json:"year"`
	Month     int               `json:"month"`
	BusyDays  []int             `json:"busy_days,omitempty"`
	DayColors map[string]string `json:"day_colors,omitempty"`
}

// Detail groups bounded counters with optional visual items and a calendar grid.
type Detail struct {
	Counters []Counter
	Items    []Item
	Calendar *CalendarGrid
}

// SanitizeText collapses whitespace, drops E2E ciphertext markers ("enc:v1:"),
// and truncates on a UTF-8 rune boundary to at most maxBytes.
func SanitizeText(raw string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	cleaned := strings.Join(strings.Fields(strings.ReplaceAll(raw, "\x00", " ")), " ")
	if cleaned == "" || containsCiphertext(cleaned) {
		return ""
	}
	if len(cleaned) <= maxBytes {
		return cleaned
	}
	const ellipsis = "…"
	if maxBytes <= len(ellipsis) {
		out := ""
		for _, r := range cleaned {
			if len(out)+utf8.RuneLen(r) > maxBytes {
				break
			}
			out += string(r)
		}
		return out
	}
	limit := maxBytes - len(ellipsis)
	for len(cleaned) > limit {
		_, size := utf8.DecodeLastRuneInString(cleaned)
		if size <= 0 {
			break
		}
		cleaned = cleaned[:len(cleaned)-size]
	}
	cleaned = strings.TrimRight(cleaned, " ")
	if cleaned == "" {
		return ""
	}
	return cleaned + ellipsis
}

// SanitizeColor normalizes a #RGB or #RRGGBB hex color to lowercase #rrggbb,
// returning "" for any non-hex input.
func SanitizeColor(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if len(s) == 4 && s[0] == '#' && isHexDigits(s[1:]) {
		return "#" + string([]byte{s[1], s[1], s[2], s[2], s[3], s[3]})
	}
	if isHexColor(s) {
		return s
	}
	return ""
}

// SanitizeItem prepares a single visual item for widget publication. Items with
// empty or encrypted titles or out-of-range timestamps are rejected.
func SanitizeItem(in Item) (Item, bool) {
	title := SanitizeText(in.Title, MaxTitleBytes)
	if title == "" || in.StartAt < 0 || in.EndAt < 0 || in.StartAt > maxTimestamp || in.EndAt > maxTimestamp {
		return Item{}, false
	}
	endAt := in.EndAt
	if endAt > 0 && in.StartAt > 0 && endAt < in.StartAt {
		endAt = in.StartAt
	}
	return Item{
		Title:    title,
		Subtitle: SanitizeText(in.Subtitle, MaxSubtitleBytes),
		Badge:    SanitizeText(in.Badge, MaxBadgeBytes),
		Color:    SanitizeColor(in.Color),
		StartAt:  in.StartAt,
		EndAt:    endAt,
		AllDay:   in.AllDay,
		Unread:   in.Unread,
		Done:     in.Done,
	}, true
}

// SanitizeItems returns up to MaxItems sanitized visual entries.
func SanitizeItems(in []Item) []Item {
	if len(in) == 0 {
		return nil
	}
	out := make([]Item, 0, min(len(in), MaxItems))
	for _, raw := range in {
		item, ok := SanitizeItem(raw)
		if !ok {
			continue
		}
		out = append(out, item)
		if len(out) == MaxItems {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BuildCalendarGrid normalizes a month calendar summary with sorted unique busy
// days and valid #rrggbb per-day accent colors.
func BuildCalendarGrid(year int, month time.Month, busyDays []int, dayColors map[int]string) *CalendarGrid {
	m := int(month)
	if year < 1970 || year > 2200 || m < 1 || m > 12 {
		return nil
	}
	maxDay := daysInMonth(year, m)
	var days []int
	for _, d := range busyDays {
		if d >= 1 && d <= maxDay {
			days = append(days, d)
		}
	}
	slices.Sort(days)
	days = slices.Compact(days)
	var colors map[string]string
	for d, rawColor := range dayColors {
		if d < 1 || d > maxDay {
			continue
		}
		if c := SanitizeColor(rawColor); c != "" {
			if colors == nil {
				colors = make(map[string]string)
			}
			colors[strconv.Itoa(d)] = c
		}
	}
	return &CalendarGrid{
		Year:      year,
		Month:     m,
		BusyDays:  days,
		DayColors: colors,
	}
}

func containsCiphertext(s string) bool {
	return strings.Contains(strings.ToLower(s), "enc:v1:")
}

func isHexDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if !((b >= '0' && b <= '9') || (b >= 'a' && b <= 'f')) {
			return false
		}
	}
	return len(s) > 0
}

func isHexColor(s string) bool {
	return len(s) == 7 && s[0] == '#' && isHexDigits(s[1:])
}

func daysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
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
	return v.PublicarDetalle(ctx, Detail{Counters: counters})
}

// PublicarDetalle publishes bounded counters, sanitized visual items and/or a
// calendar month grid that the app has already loaded.
func (v View) PublicarDetalle(ctx context.Context, detail Detail) error {
	if v.bundle == "" {
		return ErrPrivate
	}
	return PublishDetail(ctx, v.bundle, v.product, v.scope, detail, 15*time.Minute)
}

func validCounters(counters []Counter) bool {
	if len(counters) > 12 {
		return false
	}
	seen := make(map[string]bool, len(counters))
	for _, c := range counters {
		switch c.Kind {
		case "unread", "files", "events", "documents", "contacts", "notes", "rows", "pending", "channels", "photos", "episodes", "apps", "calendars", "address_books":
		case "storage_percent":
			if c.Value > 100 {
				return false
			}
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

func validTextField(s string, maxBytes int) bool {
	if len(s) > maxBytes || !utf8.ValidString(s) || containsCiphertext(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return false
		}
	}
	return true
}

func validItems(items []Item) bool {
	if len(items) > MaxItems {
		return false
	}
	for _, it := range items {
		if strings.TrimSpace(it.Title) == "" ||
			!validTextField(it.Title, MaxTitleBytes) ||
			!validTextField(it.Subtitle, MaxSubtitleBytes) ||
			!validTextField(it.Badge, MaxBadgeBytes) {
			return false
		}
		if it.Color != "" && !isHexColor(it.Color) {
			return false
		}
		if it.StartAt < 0 || it.StartAt > maxTimestamp || it.EndAt < 0 || it.EndAt > maxTimestamp {
			return false
		}
		if it.EndAt > 0 && it.StartAt > 0 && it.EndAt < it.StartAt {
			return false
		}
	}
	return true
}

func validCalendar(cal *CalendarGrid) bool {
	if cal == nil {
		return true
	}
	if cal.Year < 1970 || cal.Year > 2200 || cal.Month < 1 || cal.Month > 12 {
		return false
	}
	maxDay := daysInMonth(cal.Year, cal.Month)
	if len(cal.BusyDays) > 31 || len(cal.DayColors) > 31 {
		return false
	}
	prev := 0
	for _, d := range cal.BusyDays {
		if d < 1 || d > maxDay || d <= prev {
			return false
		}
		prev = d
	}
	for k, v := range cal.DayColors {
		d, err := strconv.Atoi(k)
		if err != nil || strconv.Itoa(d) != k || d < 1 || d > maxDay || !isHexColor(v) {
			return false
		}
	}
	return true
}

func cloneCalendar(cal *CalendarGrid) *CalendarGrid {
	if cal == nil {
		return nil
	}
	out := &CalendarGrid{
		Year:  cal.Year,
		Month: cal.Month,
	}
	if len(cal.BusyDays) > 0 {
		out.BusyDays = slices.Clone(cal.BusyDays)
	}
	if len(cal.DayColors) > 0 {
		out.DayColors = maps.Clone(cal.DayColors)
	}
	return out
}

func equalCalendar(a, b *CalendarGrid) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Year == b.Year &&
		a.Month == b.Month &&
		slices.Equal(a.BusyDays, b.BusyDays) &&
		maps.Equal(a.DayColors, b.DayColors)
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
func Publish(ctx context.Context, bundle, product string, scope Scope, counters []Counter, age time.Duration) error {
	return PublishDetail(ctx, bundle, product, scope, Detail{Counters: counters}, age)
}

// PublishDetail atomically replaces one app's snapshot (counters, visual items
// and optional calendar grid) under the existing suite lock.
func PublishDetail(ctx context.Context, bundle, product string, scope Scope, detail Detail, age time.Duration) (result error) {
	if !lector() {
		return ErrSinLector
	}
	if !api.EsProducto(product) || age <= 0 || age > MaxAge ||
		!validCounters(detail.Counters) || !validItems(detail.Items) || !validCalendar(detail.Calendar) {
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
	changed := false
	defer func() {
		release()
		if result == nil && changed {
			Notify(bundle)
		}
	}()
	state := ajustes.Leer()
	if state.Bloqueo || scope.generation == "" || state.Sesion != scope.generation {
		return ErrPrivate
	}
	now := time.Now()
	// Real-time deltas and repeated list refreshes can return the same payload.
	// Keep the original observation/expiry and avoid exhausting WidgetKit's reload
	// budget. A fresh authenticated fetch still renews the snapshot before the last
	// third of its lifetime, and changed data is published immediately.
	if previous, err := readFile(destination); err == nil && previous.reusable(product, state.Sesion, detail, now, age) {
		return nil
	}
	s := Snapshot{
		Version:    Version,
		Product:    product,
		Generation: scope.generation,
		UpdatedAt:  now.Unix(),
		ExpiresAt:  now.Add(age).Unix(),
		Active:     true,
		Counters:   append([]Counter{}, detail.Counters...),
		Items:      slices.Clone(detail.Items),
		Calendar:   cloneCalendar(detail.Calendar),
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := ajustes.EscribirEntero(destination, data); err != nil {
		return err
	}
	changed = true
	return nil
}

// reusable is called after validating the current session lock under the suite
// lock. It cannot preserve another identity, stale data or an excessive lifetime.
func (s Snapshot) reusable(product, generation string, detail Detail, now time.Time, age time.Duration) bool {
	return s.Product == product && s.Visible(generation, false, now) &&
		slices.Equal(s.Counters, detail.Counters) &&
		slices.Equal(s.Items, detail.Items) &&
		equalCalendar(s.Calendar, detail.Calendar) &&
		s.ExpiresAt <= now.Add(age).Unix() &&
		s.UpdatedAt > now.Add(-age*2/3).Unix() && s.ExpiresAt >= now.Add(age/3).Unix()
}

// Visible is also the contract implemented by native providers. Counters and
// items are hidden for expired, future-dated, locked, mismatched or oversized snapshots.
func (s Snapshot) Visible(generation string, locked bool, now time.Time) bool {
	return !locked && s.Active && s.Version == Version && api.EsProducto(s.Product) &&
		generation != "" && s.Generation == generation &&
		s.UpdatedAt <= now.Unix()+5 && s.UpdatedAt > 0 &&
		s.ExpiresAt > now.Unix() && s.ExpiresAt > s.UpdatedAt &&
		s.ExpiresAt-s.UpdatedAt <= int64(MaxAge/time.Second) &&
		validCounters(s.Counters) && validItems(s.Items) && validCalendar(s.Calendar)
}

// Read loads a widget snapshot only when its shared session marker is current.
// Native extensions implement the same checks without opening the keychain.
func Read(bundle string) (Snapshot, error) {
	filename, err := path(bundle)
	if err != nil {
		return Snapshot{}, err
	}
	s, err := readFile(filename)
	if err != nil {
		return Snapshot{}, err
	}
	state := ajustes.Leer()
	if !s.Visible(state.Sesion, state.Bloqueo, time.Now()) {
		return Snapshot{}, ErrPrivate
	}
	return s, nil
}

func readFile(filename string) (Snapshot, error) {
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
