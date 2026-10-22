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
)

// SytueRooms recrea en Sytue Rooms la estructura de equipos del origen: un equipo
// por equipo, un canal por canal y sus miembros dentro.
//
// Escribe por la API PÚBLICA de Rooms con el token de quien lanza la migración —no
// contra su base de datos—, así que hereda sus permisos, sus validaciones y su
// auditoría. Quien lanza queda como creador de los equipos; el resto de personas
// entra como miembro.
//
// ponytail: la idempotencia es por NOMBRE (se lista y se reutiliza el que ya
// exista) además de por el mapa de migración. Dos pasadas no duplican canales, y
// un canal que la organización ya tenía se reutiliza en vez de crear «General (2)».
type SytueRooms struct {
	base  string
	token TokenSource
	hc    *http.Client
}

// NewSytueRooms construye el cliente de Rooms. hc es opcional.
func NewSytueRooms(baseURL string, token TokenSource, hc *http.Client) *SytueRooms {
	if hc == nil {
		hc = defaultSuiteClient()
	}
	return &SytueRooms{base: strings.TrimRight(baseURL, "/"), token: token, hc: hc}
}

// roomsTeam y roomsChannel son lo justo de la respuesta de Rooms.
type roomsTeam struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type roomsChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// EnsureTeam devuelve el id del equipo con ese nombre, creándolo si no existe.
func (s *SytueRooms) EnsureTeam(ctx context.Context, name, description string) (string, error) {
	name = trimTo(strings.TrimSpace(name), 120)
	if name == "" {
		return "", fmt.Errorf("migrate: el equipo de origen no tiene nombre")
	}
	var list struct {
		Teams []roomsTeam `json:"teams"`
	}
	if err := s.call(ctx, http.MethodGet, "/api/v1/teams", nil, &list); err != nil {
		return "", err
	}
	for _, t := range list.Teams {
		if strings.EqualFold(strings.TrimSpace(t.Name), name) {
			return t.ID, nil
		}
	}
	var created roomsTeam
	body := map[string]string{"name": name, "description": trimTo(description, 500)}
	if err := s.call(ctx, http.MethodPost, "/api/v1/teams", body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// EnsureChannel devuelve el id del canal con ese nombre dentro del equipo,
// creándolo si no existe.
func (s *SytueRooms) EnsureChannel(ctx context.Context, teamID, name, topic string) (string, error) {
	name = trimTo(strings.TrimSpace(name), 120)
	if name == "" {
		return "", fmt.Errorf("migrate: el canal de origen no tiene nombre")
	}
	path := "/api/v1/teams/" + url.PathEscape(teamID) + "/channels"
	var list struct {
		Channels []roomsChannel `json:"channels"`
	}
	if err := s.call(ctx, http.MethodGet, path, nil, &list); err != nil {
		return "", err
	}
	for _, c := range list.Channels {
		if strings.EqualFold(strings.TrimSpace(c.Name), name) {
			return c.ID, nil
		}
	}
	var created roomsChannel
	body := map[string]string{"name": name, "topic": trimTo(topic, 500)}
	if err := s.call(ctx, http.MethodPost, path, body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// AddMember mete a una persona en el equipo y en el canal. Rooms exige pertenecer
// al equipo para estar en uno de sus canales, así que se hacen las dos cosas; que
// ya estuviera en cualquiera de ellos no es un error.
func (s *SytueRooms) AddMember(ctx context.Context, teamID, channelID, subject, name string) error {
	body := map[string]string{"subject": subject, "name": name}
	if err := s.call(ctx, http.MethodPost, "/api/v1/teams/"+url.PathEscape(teamID)+"/members", body, nil); err != nil &&
		!isAlreadyThere(err) {
		return err
	}
	if err := s.call(ctx, http.MethodPost, "/api/v1/channels/"+url.PathEscape(channelID)+"/members", body, nil); err != nil &&
		!isAlreadyThere(err) {
		return err
	}
	return nil
}

// isAlreadyThere reconoce el «ya estaba» de Rooms (409). Volver a añadir a alguien
// que ya es miembro es el resultado esperado de una segunda pasada, no un fallo.
func isAlreadyThere(err error) bool {
	var se *suiteError
	return asSuiteError(err, &se) && se.status == http.StatusConflict
}

// call emite una petición autenticada a Rooms y decodifica la respuesta.
func (s *SytueRooms) call(ctx context.Context, method, path string, body, dst any) error {
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("migrate: Sytue Rooms rechazó la escritura (HTTP %d): el permiso para escribir en nombre de esa persona no es válido; comprueba que sigue activa en la organización y reanuda el trabajo", resp.StatusCode)
		}
		return &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Sytue Rooms"}
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst); err != nil {
		return fmt.Errorf("migrate: respuesta de Sytue Rooms ilegible: %w", err)
	}
	return nil
}

var _ ChatDestination = (*SytueRooms)(nil)
