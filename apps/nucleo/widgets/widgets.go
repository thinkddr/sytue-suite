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
	"sync/atomic"
	"time"
	"unicode/utf8"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

const (
	// Version is shared with the WidgetKit, Android and desktop providers. La 2 (del
	// 2026-11-06) lleva en `generation` la ÉPOCA de identidad, no la marca de sesión, y
	// vive hasta 24 h: los lectores solo aceptan la 2 (ADR 0078, enmienda).
	Version = 2
	// MaxAge es la caducidad máxima: pasado un día el widget se oculta. Los quince
	// widgets enseñan «lo de hoy», y pasado un día enseñarían el hoy de ayer como si fuera
	// el de hoy; es también el horizonte en que el sistema concede varias rondas en
	// segundo plano a una app usada, y acota lo que enseña un teléfono olvidado. Por
	// debajo, el lector enseña el último dato con «Actualizado hace X» (Rancio).
	MaxAge = 24 * time.Hour
	// Vigencia es la caducidad de lo que publica el Publicador: la máxima. Con la app
	// viva su latido (10 min) lo renueva; con la app parada, el widget enseña el último
	// dato con su antigüedad hasta MaxAge.
	Vigencia = MaxAge
	// Fresco: por debajo de esto el dato se considera vivo y el lector no dice de cuándo
	// es. A partir de aquí es Rancio y lo dice.
	Fresco = 15 * time.Minute
	// RenovarTras: un payload idéntico, con la misma época, no se reescribe si el
	// anterior tiene menos de esto. Pasado, se reescribe aunque no cambie nada, para
	// que `updated_at` diga cuándo se comprobó (y el «Actualizado hace X» no mienta).
	// Va con el latido del Publicador: una app abierta y quieta reescribe cada 10 min.
	RenovarTras = 10 * time.Minute
	// vidaParaReusar: tampoco se deja sin reescribir un snapshot que caducaría antes
	// de la siguiente ronda del Publicador —su latido, más lo que puede esperar un
	// aviso y lo que puede tardar la ronda—.
	vidaParaReusar   = LatidoWidget + EsperaMaxWidget + PlazoWidget
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
func Disponible() bool { return hayLector() }

// lectorSimulado es lo que fija SimularLector: 0, nada (manda `lector`); 1, sí; 2, no.
// Es atómico porque se restaura al acabar un test mientras aún puede correr un borrado
// lanzado en segundo plano (ClearIf desde la suite), y `lector` no se toca después del
// init de la plataforma.
var lectorSimulado atomic.Int32

func hayLector() bool {
	switch lectorSimulado.Load() {
	case 1:
		return true
	case 2:
		return false
	}
	return lector()
}

// SimularLector es SOLO para tests de otros paquetes: hace como si hubiera un widget
// nativo, para probar la publicación en Linux. Devuelve cómo dejarlo como estaba.
func SimularLector(activo bool) (restaurar func()) {
	v := int32(2)
	if activo {
		v = 1
	}
	anterior := lectorSimulado.Swap(v)
	return func() { lectorSimulado.Store(anterior) }
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
// an authenticated app. generation is the opaque identity epoch (ajustes.MarcarIdentidad;
// en Android con el broker, su marca de widgets), never a subject, email or token.
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
		if (b < '0' || b > '9') && (b < 'a' || b > 'f') {
			return false
		}
	}
	return s != ""
}

func isHexColor(s string) bool {
	return len(s) == 7 && s[0] == '#' && isHexDigits(s[1:])
}

func daysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// Scope is captured BEFORE fetching data. Publishing after a user switch, logout
// or lock change fails closed, even when an older request finishes later.
//
// Lo que guarda es la ÉPOCA de identidad (ajustes.MarcarIdentidad), no la marca de
// sesión: la marca cambia en cada renovación del token y dejaba sin publicar todo lo
// capturado antes de renovar (ADR 0078, enmienda del 2026-11-06).
type Scope struct{ generation string }

// Capture binds a pending fetch to the identity epoch adopted by the app.
// Pass Estado.EpocaSesion, never a fresh settings read: the disk epoch can
// change before another process has adopted the corresponding identity.
func Capture(adoptedEpoch string) Scope { return Scope{generation: adoptedEpoch} }

// vigente: la época capturada es la actual y el entorno deja enseñar (Entorno.permite).
func (s Scope) vigente(e Entorno) bool {
	return s.generation != "" && e.Epoca == s.generation && e.permite()
}

// Entorno es lo que un lector comprueba FUERA del snapshot antes de enseñarlo. Es el
// mismo contrato en Go, en Swift (Snapshot.swift) y en Java (WidgetSnapshot.java), y
// los tres lo prueban contra testdata/contrato.tsv.
type Entorno struct {
	// Epoca es la época de identidad actual (primera línea de `sesion.epoca`; en
	// Android con el broker, su `marca_widgets`).
	Epoca string
	// Confirmada es la marca de sesión con la que se escribió esa época (cuarta línea
	// de `sesion.epoca`). Marca es la `sesion.marca` actual. Si no coinciden, la última
	// sesión la cambió una compilación que no conoce la época: no se enseña nada.
	Confirmada, Marca string
	// Cerrada: hay algún `*.cerrada` en la carpeta (AlmacenSesion: un cierre de sesión).
	Cerrada bool
	// Bloqueo: la persona pidió el bloqueo de la suite.
	Bloqueo bool
}

func (e Entorno) permite() bool {
	return !e.Bloqueo && !e.Cerrada && e.Epoca != "" && e.Marca != "" && e.Confirmada == e.Marca
}

// entornoActual lee el entorno del disco (y, en Android con el broker, del broker).
func entornoActual() (Entorno, error) {
	dir, err := ajustes.Carpeta()
	if err != nil {
		return Entorno{}, err
	}
	state := ajustes.Leer()
	return Entorno{
		Epoca: state.Epoca, Confirmada: state.EpocaConfirmada, Marca: state.Sesion,
		Cerrada: hayCierre(dir), Bloqueo: state.Bloqueo,
	}, nil
}

// hayCierre: algún `*.cerrada` en la carpeta de la suite. Si no se puede listar, como
// si lo hubiera: falla cerrado.
func hayCierre(dir string) bool {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return true
	}
	for _, e := range entradas {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".cerrada") {
			return true
		}
	}
	return false
}

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

// PublishDetail atomically replaces one app's snapshot (counters, visual items
// and optional calendar grid) under the existing suite lock. Writers share the
// same settings directory as native providers through the OS App Group on Apple;
// Android providers use their own app's private directory.
//
// Solo lo llama el Publicador (publicador.go): ninguna pantalla publica
// (TestNadiePublicaWidgetsDesdeLaVista, en apps/fyne/sesion).
//
// La captura es de la ÉPOCA, y la época es lo que se escribe en `generation` (v2). Exige
// lo mismo que el lector (Entorno): sin bloqueo, sin cierre y la época confirmada con la
// marca actual. Una sesión cambiada por una compilación que no conoce la época falla
// cerrado hasta que otra la adopte. Renovar el token no cambia la época: no invalida ni
// la captura ni lo ya publicado.
func PublishDetail(ctx context.Context, bundle, product string, scope Scope, detail Detail, age time.Duration) (result error) {
	if !hayLector() {
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
	entorno, err := entornoActual()
	if err != nil {
		return err
	}
	if !scope.vigente(entorno) {
		return ErrPrivate
	}
	now := time.Now()
	// Real-time deltas and repeated list refreshes can return the same payload.
	// Skipping the write saves WidgetKit's reload budget, but only for RenovarTras
	// and only under the same epoch: after that the snapshot is rewritten so
	// updated_at («Actualizado hace X») and the expiry move forward.
	if previous, err := readFile(destination); err == nil && previous.reusable(product, entorno, detail, now, age) {
		return nil
	}
	s := Snapshot{
		Version:    Version,
		Product:    product,
		Generation: entorno.Epoca,
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
func (s Snapshot) reusable(product string, e Entorno, detail Detail, now time.Time, age time.Duration) bool {
	return s.Product == product && s.Visible(e, now) &&
		slices.Equal(s.Counters, detail.Counters) &&
		slices.Equal(s.Items, detail.Items) &&
		equalCalendar(s.Calendar, detail.Calendar) &&
		s.ExpiresAt <= now.Add(age).Unix() &&
		s.UpdatedAt > now.Add(-RenovarTras).Unix() &&
		s.ExpiresAt > now.Add(vidaParaReusar).Unix()
}

// Visible is also the contract implemented by native providers (testdata/contrato.tsv).
// Counters and items are hidden for expired, future-dated, locked, closed, unconfirmed,
// mismatched or oversized snapshots.
func (s Snapshot) Visible(e Entorno, now time.Time) bool {
	return e.permite() && s.Active && s.Version == Version && api.EsProducto(s.Product) &&
		s.Generation == e.Epoca &&
		s.UpdatedAt <= now.Unix()+5 && s.UpdatedAt > 0 &&
		s.ExpiresAt > now.Unix() && s.ExpiresAt > s.UpdatedAt &&
		s.ExpiresAt-s.UpdatedAt <= int64(MaxAge/time.Second) &&
		validCounters(s.Counters) && validItems(s.Items) && validCalendar(s.Calendar)
}

// Rancio dice si el lector tiene que decir de cuándo es el dato («Actualizado hace X»):
// lleva Fresco o más sin comprobarse.
func (s Snapshot) Rancio(now time.Time) bool {
	return now.Unix()-s.UpdatedAt >= int64(Fresco/time.Second)
}

// Vigentes son los elementos que siguen valiendo a `now`, para un snapshot ya Visible.
// Solo Calendar los recorta: quita los eventos que ya terminaron. Es la misma regla que
// la Fuente de Calendar al publicar (fuentes/calendar.go), y la misma en Swift
// (SytueWidgetTimeline.vigentes) y en Java (WidgetSnapshot.vigentes), que la prueban
// contra testdata/contrato.tsv. En las demás apps los elementos no caducan solos.
func Vigentes(items []Item, product string, now time.Time) []Item {
	if product != "calendar" {
		return items
	}
	var out []Item
	for _, it := range items {
		if vigente(it, now) {
			out = append(out, it)
		}
	}
	return out
}

// vigente: un evento con hora vale mientras no haya terminado (o no haya empezado). Uno
// de día completo es FLOTANTE —el servicio lo guarda a medianoche UTC, y cae el mismo
// día en cualquier zona (api.Instance.Day)—: vale hasta la medianoche LOCAL que sigue a
// su último día, con el final exclusivo. Sin fechas, vale siempre.
func vigente(it Item, now time.Time) bool {
	if it.StartAt == 0 && it.EndAt == 0 {
		return true
	}
	if !it.AllDay {
		return it.EndAt > now.Unix() || it.StartAt >= now.Unix()
	}
	ultimo := it.StartAt
	if it.EndAt > it.StartAt {
		ultimo = it.EndAt - 1
	}
	d := time.Unix(ultimo, 0).UTC()
	return now.Before(time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, now.Location()))
}

// Read loads a widget snapshot only when the reader would show it (Visible).
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
	entorno, err := entornoActual()
	if err != nil {
		return Snapshot{}, err
	}
	if !s.Visible(entorno, time.Now()) {
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
	return clearSnapshots(ctx, nil, "")
}

// ClearIf removes snapshots only while the adopted identity epoch is still current.
// A late logout/lock callback must not erase a freshly connected account's data.
func ClearIf(ctx context.Context, adoptedEpoch string) error {
	return clearSnapshots(ctx, &adoptedEpoch, "")
}

// ClearBundleIf borra solo el snapshot de UNA app, con la misma condición que ClearIf.
// Es lo que hace el Publicador cuando el servidor rechaza la sesión de esa app: no se
// rota la época de las quince por un rechazo puntual de una.
func ClearBundleIf(ctx context.Context, bundle, adoptedEpoch string) error {
	if !bundlePattern.MatchString(bundle) {
		return ErrInvalid
	}
	return clearSnapshots(ctx, &adoptedEpoch, bundle)
}

func clearSnapshots(ctx context.Context, expected *string, onlyBundle string) error {
	if !hayLector() {
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
	defer func() { release(); Notify(onlyBundle) }()
	// Una época que no se deja leer sale vacía en ajustes.Leer, igual que sin sesión, y
	// coincidiría con la de quien no tiene sesión: lanzada por iOS con el teléfono
	// bloqueado, la app borraba así los widgets de las quince. Sin poder leerla no se
	// sabe quién ha entrado, y no se borra nada.
	if expected != nil && (ajustes.EpocaLegible() != nil || ajustes.Leer().Epoca != *expected) {
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
		bundle := strings.TrimSuffix(entry.Name(), ".json")
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || !bundlePattern.MatchString(bundle) ||
			onlyBundle != "" && bundle != onlyBundle {
			continue
		}
		if err := os.Remove(filepath.Join(dir, "widgets", entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
