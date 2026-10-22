// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package asistente llama a Sytue AI (POST /api/v1/assist) desde un servicio Go.
//
// Va con el token de la PERSONA, no con uno de servicio: el gasto, el presupuesto
// del modo calidad y los límites se le cuentan a quien pregunta, igual que cuando
// el asistente se abre desde el navegador.
package asistente

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

// maxRespuesta acota el cuerpo que se deserializa del servicio de IA.
const maxRespuesta = 256 << 10

// ErrSinServicio es un despliegue sin SYTUE_AI_URL.
var ErrSinServicio = errors.New("asistente: sin servicio de IA configurado")

// ErrEstado es una respuesta no 2xx del servicio de IA, con su código: 503 es que
// no tiene proveedor y 429 que la persona agotó su límite.
type ErrEstado int

func (e ErrEstado) Error() string {
	return fmt.Sprintf("asistente: el servicio de IA devolvió %d", int(e))
}

// Peticion es el contrato de /api/v1/assist. La orden va en Instruction y el
// material de referencia en Context, que el servicio trata como DATOS.
type Peticion struct {
	Action      string `json:"action"`
	Instruction string `json:"instruction"`
	Context     string `json:"context"`
	Kind        string `json:"kind"`
	// Modo: "interactivo" (rápido) o "calidad" (acierta más y tarda más).
	Modo string `json:"modo,omitempty"`
	// Esfuerzo es el reasoning_effort: none | low | medium | high.
	Esfuerzo string `json:"esfuerzo,omitempty"`
}

// Cliente habla con el servicio de IA.
type Cliente struct {
	base string
	hc   *http.Client
}

// Nuevo construye el cliente. `base` es la URL interna del servicio (http://sytue-ai);
// vacía = sin IA, y quien llama lo consulta con Activo().
func Nuevo(base string, plazo time.Duration) *Cliente {
	return &Cliente{
		base: strings.TrimRight(base, "/"),
		hc: &http.Client{
			Timeout: plazo,
			Transport: &http.Transport{
				MaxIdleConns:      10,
				IdleConnTimeout:   90 * time.Second,
				ForceAttemptHTTP2: true,
			},
		},
	}
}

// Activo indica si hay servicio de IA configurado.
func (c *Cliente) Activo() bool { return c != nil && c.base != "" }

// Pedir envía la petición y devuelve el texto de la respuesta.
func (c *Cliente) Pedir(ctx context.Context, token string, p Peticion) (string, error) {
	if !c.Activo() {
		return "", ErrSinServicio
	}
	cuerpo, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("asistente: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/assist", bytes.NewReader(cuerpo))
	if err != nil {
		return "", fmt.Errorf("asistente: construir petición: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("asistente: %w", err)
	}
	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, maxRespuesta)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, body)
		return "", ErrEstado(resp.StatusCode)
	}
	var r struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(body).Decode(&r); err != nil {
		return "", fmt.Errorf("asistente: respuesta ilegible: %w", err)
	}
	return r.Text, nil
}

// JSON recorta el objeto JSON de una respuesta del modelo.
//
// Existe porque el modelo pone el vallado ```json … ``` aunque se le diga que no,
// y porque a veces antepone «Aquí tienes:». Se toma del primer `{` al último `}`:
// es tosco y es exactamente lo que hace falta. Vacío si no hay objeto.
func JSON(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i < 0 || j <= i {
		return ""
	}
	return s[i : j+1]
}

// Identidad identifica al usuario y organización para los que se ejecuta el trabajo (§9.1).
// La identidad va afirmada en cabeceras X-Sytue-Tenant y X-Sytue-Sub.
type Identidad struct {
	Tenant string
	Sub    string
}

// BYOK lleva credenciales propias para invocar al proveedor (§9.1).
type BYOK struct {
	Proveedor string `json:"proveedor"`
	Modelo    string `json:"modelo"`
	Clave     string `json:"clave"`
}

// PeticionPages es el contrato de POST /internal/v1/assist (§9.1).
type PeticionPages struct {
	Action      string          `json:"action"`
	Instruction string          `json:"instruction"`
	Context     string          `json:"context"`
	Kind        string          `json:"kind"`
	Modo        string          `json:"modo,omitempty"`
	Esfuerzo    string          `json:"esfuerzo,omitempty"`
	Sistema     string          `json:"sistema,omitempty"`
	Esquema     json.RawMessage `json:"esquema,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	BYOK        *BYOK           `json:"byok,omitempty"`
}

// RespuestaPages es la respuesta detallada de POST /internal/v1/assist (§9.1).
type RespuestaPages struct {
	Text          string `json:"text"`
	Modelo        string `json:"modelo"`
	Degradado     bool   `json:"degradado"`
	TokensEntrada int    `json:"tokens_entrada"`
	TokensSalida  int    `json:"tokens_salida"`
}

// PedirPages envía una petición a la ruta interna de IA para Sytue Web (§9.1).
func (c *Cliente) PedirPages(ctx context.Context, secreto string, id Identidad, p PeticionPages) (*RespuestaPages, error) {
	if !c.Activo() {
		return nil, ErrSinServicio
	}
	cuerpo, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("asistente: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/internal/v1/assist", bytes.NewReader(cuerpo))
	if err != nil {
		return nil, fmt.Errorf("asistente: construir petición: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if secreto != "" {
		req.Header.Set("Authorization", "Bearer "+secreto)
	}
	if id.Tenant != "" {
		req.Header.Set("X-Sytue-Tenant", id.Tenant)
	}
	if id.Sub != "" {
		req.Header.Set("X-Sytue-Sub", id.Sub)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asistente: %w", err)
	}
	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, maxRespuesta)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, body)
		return nil, ErrEstado(resp.StatusCode)
	}
	var r RespuestaPages
	if err := json.NewDecoder(body).Decode(&r); err != nil {
		return nil, fmt.Errorf("asistente: respuesta ilegible: %w", err)
	}
	return &r, nil
}

// ListarModelos consulta los modelos disponibles con credenciales BYOK a través de POST /internal/v1/models (§9.1).
func (c *Cliente) ListarModelos(ctx context.Context, secreto string, id Identidad, byok BYOK) ([]string, error) {
	if !c.Activo() {
		return nil, ErrSinServicio
	}
	payload := map[string]any{"byok": byok}
	cuerpo, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("asistente: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/internal/v1/models", bytes.NewReader(cuerpo))
	if err != nil {
		return nil, fmt.Errorf("asistente: construir petición: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if secreto != "" {
		req.Header.Set("Authorization", "Bearer "+secreto)
	}
	if id.Tenant != "" {
		req.Header.Set("X-Sytue-Tenant", id.Tenant)
	}
	if id.Sub != "" {
		req.Header.Set("X-Sytue-Sub", id.Sub)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asistente: %w", err)
	}
	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, maxRespuesta)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, body)
		return nil, ErrEstado(resp.StatusCode)
	}
	var r struct {
		Modelos []string `json:"modelos"`
	}
	if err := json.NewDecoder(body).Decode(&r); err != nil {
		return nil, fmt.Errorf("asistente: respuesta ilegible: %w", err)
	}
	return r.Modelos, nil
}
