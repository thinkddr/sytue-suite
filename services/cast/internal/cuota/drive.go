// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cuota habla con Sytue Drive, que es el dueño del contador del plan
// (CAST-ENCARGO §8): pregunta cuánto sitio queda con el token de la persona y
// declara, con el secreto interno, cuánto ocupa Cast en cada organización
// (valor absoluto: idempotente). Y lee, también con el token de la persona, los
// vídeos de su Drive que se suben a Cast («Subir a Cast», ficheros.go): es el
// mismo servicio, la misma URL y la misma credencial, y un segundo cliente de
// Drive en Cast sería una copia de este.
package cuota

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Errores de la consulta.
var (
	// ErrSinSesion: no hay token con el que preguntar la cuota.
	ErrSinSesion = errors.New("cuota: sin sesión")
	// ErrNoDisponible: Drive no contesta (red, tiempo agotado o 5xx). Es el ÚNICO
	// caso en el que la subida sigue sin cuota (§8: «si Drive no contesta»).
	ErrNoDisponible = errors.New("cuota: Drive no contesta")
	// ErrRechazado: Drive contestó y dijo que no (4xx: sin permiso, sin Drive,
	// demasiadas consultas). La subida NO sigue: si no, bastaría con agotar el
	// límite de Drive para subir sin cuota.
	ErrRechazado = errors.New("cuota: Drive rechaza la consulta")
)

// Drive es el cliente de la cuota.
type Drive struct {
	baseURL string
	secreto string
	http    *http.Client
	// descargas es para los ficheros: sin plazo total (un original de 8 GiB tarda
	// lo que tarda) y sin seguir redirecciones solo (ver Abrir).
	descargas *http.Client
}

// New construye el cliente. Con baseURL vacía devuelve nil: sin Drive no hay
// cuota (solo en pruebas; el chart siempre la da).
func New(baseURL, secretoInterno string) *Drive {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil
	}
	return &Drive{
		baseURL: baseURL, secreto: secretoInterno,
		http:      &http.Client{Timeout: 5 * time.Second},
		descargas: clienteDeDescargas(),
	}
}

// Resumen es la cuota tal como la cuenta Drive.
type Resumen struct {
	LimitBytes     int64 `json:"limit_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
}

// Consultar pregunta a Drive, con el token de la persona, la cuota de su
// organización (`GET /api/v1/quota`).
func (d *Drive) Consultar(ctx context.Context, bearer string) (Resumen, error) {
	if bearer == "" {
		return Resumen{}, ErrSinSesion
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/api/v1/quota", http.NoBody)
	if err != nil {
		return Resumen{}, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return Resumen{}, fmt.Errorf("%w: %w", ErrNoDisponible, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode >= 500:
		return Resumen{}, fmt.Errorf("%w: respuesta %d", ErrNoDisponible, resp.StatusCode)
	default:
		return Resumen{}, fmt.Errorf("%w: respuesta %d", ErrRechazado, resp.StatusCode)
	}
	var out Resumen
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return Resumen{}, fmt.Errorf("%w: decodificar: %w", ErrNoDisponible, err)
	}
	return out, nil
}

// Disponible dice cuántos bytes le quedan a la organización de quien llama.
func (d *Drive) Disponible(ctx context.Context, bearer string) (int64, error) {
	r, err := d.Consultar(ctx, bearer)
	return r.AvailableBytes, err
}

// Declarar fija en Drive el total que Cast ocupa en la organización
// (`PUT /internal/quota/external/cast {tenant, bytes}`, con el secreto interno).
func (d *Drive) Declarar(ctx context.Context, tenant string, total int64) error {
	if d.secreto == "" {
		return errors.New("cuota: sin secreto interno para declarar el uso a Drive")
	}
	b, err := json.Marshal(map[string]any{"tenant": tenant, "bytes": total})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, d.baseURL+"/internal/quota/external/cast", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.secreto)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("drive: declarar uso: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("drive: declarar uso: respuesta %d", resp.StatusCode)
	}
	return nil
}
