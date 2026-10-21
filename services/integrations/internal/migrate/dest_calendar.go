// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SytueCalendar es el destino de la agenda. Escribe por la API pública de Sytue
// Calendar, que habla iCalendar de verdad (RRULE/RDATE/EXDATE, TZID, asistentes
// con su rol y su respuesta), así que una serie que se repite llega COMO SERIE y
// se puede seguir editando entera.
type SytueCalendar struct {
	base  string
	token TokenSource
	hc    *http.Client

	mu      sync.Mutex
	byName  map[string]string // nombre → id (una carga por trabajo)
	loaded  bool
	primary string
}

// NewSytueCalendar construye el cliente. hc es opcional.
func NewSytueCalendar(baseURL string, token TokenSource, hc *http.Client) *SytueCalendar {
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}
	return &SytueCalendar{base: strings.TrimRight(baseURL, "/"), token: token, hc: hc, byName: map[string]string{}}
}

// EnsureCalendar devuelve el id del calendario destino, creándolo si no existe.
//
// El calendario PRINCIPAL del origen se funde con el principal de destino: crear
// un segundo «Mi calendario» al lado del que ya usa la persona es la forma más
// rápida de que nadie encuentre sus citas.
func (s *SytueCalendar) EnsureCalendar(ctx context.Context, c Calendar) (string, error) {
	if err := s.load(ctx); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Primary && s.primary != "" {
		return s.primary, nil
	}
	if id, ok := s.byName[strings.ToLower(c.Name)]; ok {
		return id, nil
	}

	body := map[string]any{"name": c.Name, "description": c.Description, "color": c.Color}
	if c.TimeZone != "" {
		body["tzid"] = c.TimeZone
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := s.call(ctx, http.MethodPost, "/api/v1/calendars", body, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", fmt.Errorf("migrate: Sytue Calendar no devolvió el id del calendario creado")
	}
	s.byName[strings.ToLower(c.Name)] = created.ID
	return created.ID, nil
}

// load trae los calendarios que la persona ya tiene (una vez por trabajo).
func (s *SytueCalendar) load(ctx context.Context) error {
	s.mu.Lock()
	done := s.loaded
	s.mu.Unlock()
	if done {
		return nil
	}
	var list struct {
		Calendars []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Primary bool   `json:"primary"`
		} `json:"calendars"`
	}
	if err := s.call(ctx, http.MethodGet, "/api/v1/calendars", nil, &list); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range list.Calendars {
		s.byName[strings.ToLower(c.Name)] = c.ID
		if c.Primary && s.primary == "" {
			s.primary = c.ID
		}
	}
	if s.primary == "" && len(list.Calendars) > 0 {
		s.primary = list.Calendars[0].ID
	}
	s.loaded = true
	return nil
}

// CreateEvent crea el evento con su recurrencia y sus asistentes.
func (s *SytueCalendar) CreateEvent(ctx context.Context, calendarID string, e Event) (string, error) {
	body := map[string]any{
		"uid":         e.UID,
		"summary":     e.Summary,
		"description": e.Description,
		"location":    e.Location,
		"organizer":   e.Organizer,
		"start":       e.Start.UTC().Format(time.RFC3339),
		"end":         e.End.UTC().Format(time.RFC3339),
		"tzid":        e.TZID,
		"all_day":     e.AllDay,
		"status":      e.Status,
		"transp":      e.Transparency,
		// Nunca se avisa a nadie de un evento MIGRADO: son citas que ya existían y
		// mandar invitaciones otra vez llenaría de correo a media organización.
		"send": false,
	}
	if e.Classification != "" {
		body["classification"] = e.Classification
	}
	if e.ConferenceURL != "" {
		body["conference_url"] = e.ConferenceURL
	}
	if e.RRule != "" {
		body["rrule"] = e.RRule
	}
	if len(e.RDate) > 0 {
		body["rdate"] = rfc3339List(e.RDate)
	}
	if len(e.ExDate) > 0 {
		body["exdate"] = rfc3339List(e.ExDate)
	}
	if len(e.Attendees) > 0 {
		people := make([]map[string]any, 0, len(e.Attendees))
		for _, a := range e.Attendees {
			people = append(people, map[string]any{
				"email": a.Email, "common_name": a.Name,
				"role": a.Role, "partstat": a.PartStat, "cutype": a.CUType,
			})
		}
		body["attendees"] = people
	}

	var created struct {
		ID string `json:"id"`
	}
	path := "/api/v1/calendars/" + url.PathEscape(calendarID) + "/events"
	if err := s.call(ctx, http.MethodPost, path, body, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", fmt.Errorf("migrate: Sytue Calendar no devolvió el id del evento creado")
	}
	return created.ID, nil
}

// rfc3339List formatea una lista de instantes.
func rfc3339List(ts []time.Time) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.UTC().Format(time.RFC3339))
	}
	return out
}

// call emite una petición JSON autenticada a Sytue Calendar.
func (s *SytueCalendar) call(ctx context.Context, method, path string, body, dst any) error {
	var payload io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}
	token, err := s.token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate: %s %s: %w", method, redactPath(s.base+path), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("migrate: Sytue Calendar rechazó la escritura (HTTP %d): el permiso para escribir en la agenda de destino no es válido; comprueba que esa persona sigue activa y reanuda el trabajo", resp.StatusCode)
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity:
		// Un evento que el destino no acepta (una regla de repetición que no
		// entiende, una fecha imposible) no se arregla reintentando.
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: Sytue Calendar no acepta este evento: %s", ErrUnimportable, strings.TrimSpace(string(msg)))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Sytue Calendar"}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst); err != nil {
		return fmt.Errorf("migrate: respuesta de Sytue Calendar ilegible: %w", err)
	}
	return nil
}

var _ CalendarDestination = (*SytueCalendar)(nil)
