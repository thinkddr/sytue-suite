// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package personas encuentra en Sytue ID a una persona de CUALQUIER organización
// por su @usuario, su correo o su identificador (`POST /api/v1/people:resolve`,
// ADR 0040).
//
// Pregunta con el token de quien llama, no con una credencial del servicio: así
// Sytue ID aplica su límite por persona y nadie puede usar un producto para
// recorrer el directorio más deprisa de lo que le dejaría Sytue ID directamente.
// El token viaja en el contexto (ConBearer / BearerDe). Lo usan Rooms
// (conversaciones con gente de fuera) y Cast (vídeos para personas concretas).
package personas

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

// Errores de la consulta. Cada producto los traduce a los suyos.
var (
	// ErrNoExiste: nadie activo usa ese @usuario, correo o identificador.
	ErrNoExiste = errors.New("personas: no existe")
	// ErrEntradaInvalida: lo tecleado no es ni un @usuario ni un correo.
	ErrEntradaInvalida = errors.New("personas: entrada inválida")
	// ErrDemasiadas: Sytue ID ha frenado a esta persona (429).
	ErrDemasiadas = errors.New("personas: demasiadas búsquedas seguidas")
	// ErrSinSesion: no hay token con el que preguntar.
	ErrSinSesion = errors.New("personas: sin sesión")
)

// Persona es alguien de Sytue One, con lo mínimo para nombrarle: nunca su correo
// de acceso ni su organización.
type Persona struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
}

// Visible es el nombre con el que se le pinta: el suyo, o su @usuario si no
// tiene, o el identificador.
func (p Persona) Visible() string {
	switch {
	case strings.TrimSpace(p.Name) != "":
		return p.Name
	case p.Username != "":
		return "@" + p.Username
	default:
		return p.UserID
	}
}

// Directorio encuentra a alguien de cualquier organización. Lo implementa
// DirectorioID; se inyecta para poder probar sin Sytue ID.
type Directorio interface {
	// Resolver busca por lo que se teclea: «@ana», «ana» o «ana@acme.com».
	Resolver(ctx context.Context, consulta string) (Persona, error)
	// PorID confirma que el sub existe y trae su nombre.
	PorID(ctx context.Context, userID string) (Persona, error)
}

type bearerCtxKey struct{}

// ConBearer deja en el contexto el token de quien llama, con el que se pregunta a
// Sytue ID. Vacío no cambia nada.
func ConBearer(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, bearerCtxKey{}, token)
}

// BearerDe devuelve el token que dejó ConBearer, o "".
func BearerDe(ctx context.Context) string {
	if v, ok := ctx.Value(bearerCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// DirectorioID es el Directorio real: habla con Sytue ID.
type DirectorioID struct {
	baseURL string
	http    *http.Client
}

// NewDirectorioID construye el cliente. Con baseURL vacía devuelve nil: sin
// directorio no se resuelve a nadie (fail-closed), y quien lo usa lo comprueba.
func NewDirectorioID(baseURL string) *DirectorioID {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil
	}
	return &DirectorioID{baseURL: baseURL, http: &http.Client{Timeout: 5 * time.Second}}
}

// Resolver implementa Directorio.
func (d *DirectorioID) Resolver(ctx context.Context, consulta string) (Persona, error) {
	return d.resolver(ctx, map[string]string{"query": consulta})
}

// PorID implementa Directorio.
func (d *DirectorioID) PorID(ctx context.Context, userID string) (Persona, error) {
	return d.resolver(ctx, map[string]string{"user_id": userID})
}

func (d *DirectorioID) resolver(ctx context.Context, cuerpo map[string]string) (Persona, error) {
	token := BearerDe(ctx)
	if token == "" {
		return Persona{}, ErrSinSesion
	}
	b, err := json.Marshal(cuerpo)
	if err != nil {
		return Persona{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/api/v1/people:resolve", bytes.NewReader(b))
	if err != nil {
		return Persona{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return Persona{}, fmt.Errorf("sytue id: solicitud: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Persona{}, ErrNoExiste
	case http.StatusBadRequest:
		return Persona{}, ErrEntradaInvalida
	case http.StatusTooManyRequests:
		return Persona{}, ErrDemasiadas
	default:
		return Persona{}, fmt.Errorf("sytue id: respuesta %d", resp.StatusCode)
	}
	var p Persona
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&p); err != nil {
		return Persona{}, fmt.Errorf("sytue id: decodificar: %w", err)
	}
	if p.UserID == "" {
		return Persona{}, ErrNoExiste
	}
	return p, nil
}
