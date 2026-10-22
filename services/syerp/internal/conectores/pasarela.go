// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package conectores

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Pasarela habla con el Sytue Integration Gateway (:8093), que es el ÚNICO sitio
// desde el que sale tráfico a un tercero.
//
// No usa ssrfhttp a propósito, y conviene que quede dicho: ese cliente bloquea
// direcciones internas, que es exactamente donde vive la pasarela. La protección
// anti-SSRF corresponde a URLs que elige un usuario; esta la fija el operador en
// INTEGRATIONS_BASE_URL, y el destino real de terceros ya lo valida la pasarela.
type Pasarela struct {
	base string
	cli  *http.Client
}

// NewPasarela construye el cliente hacia la pasarela.
func NewPasarela(base string) *Pasarela {
	return &Pasarela{
		base: strings.TrimRight(base, "/"),
		// Una consulta a un SaaS puede tardar; más de medio minuto ya es una
		// petición colgada que hay que soltar.
		cli: &http.Client{Timeout: 30 * time.Second},
	}
}

// Tabla es el resultado tabular de la pasarela: el mismo contrato que usa Sheets.
// Las celdas quedan SIN deserializar para no convertir un decimal en float64 por
// el camino: un importe que pasa por float64 ya no se puede llevar al libro.
type Tabla struct {
	Columns []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"columns"`
	Rows [][]json.RawMessage `json:"rows"`
}

// MaxRespuesta acota lo que se lee de la pasarela. Una importación grande se
// pagina con `limit`; un cuerpo de más de esto es un error de spec.
const MaxRespuesta = 8 << 20 // 8 MiB

// Consultar pide datos tabulares a una instancia de conector.
//
// `autorizacion` es la cabecera Authorization DE QUIEN PIDE, que se reenvía tal
// cual. Así la pasarela aplica el mismo tenant y los mismos permisos que tendría
// la persona si consultara directamente: SyERP no necesita una cuenta de
// servicio con acceso a todas las organizaciones, que es justo la credencial que
// nadie quiere tener que custodiar.
func (p *Pasarela) Consultar(ctx context.Context, instancia, autorizacion string, spec any) (Tabla, error) {
	var tabla Tabla
	cuerpo, err := json.Marshal(spec)
	if err != nil {
		return tabla, fmt.Errorf("spec de consulta: %w", err)
	}
	// La instancia la escribe la administración en la conexión y va en la RUTA,
	// con el token de quien pide: sin validarla, «x/../../operator/…» o un «?»
	// llevaban esa petición autenticada a otro sitio de la pasarela (NEG-26).
	id, err := uuid.Parse(instancia)
	if err != nil {
		return tabla, fmt.Errorf("instancia de conector no válida: %q", instancia)
	}
	url := fmt.Sprintf("%s/api/v1/instances/%s/data/query", p.base, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(cuerpo))
	if err != nil {
		return tabla, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", autorizacion)

	resp, err := p.cli.Do(req)
	if err != nil {
		return tabla, fmt.Errorf("pasarela de integraciones: %w", err)
	}
	defer resp.Body.Close()

	crudo, err := io.ReadAll(io.LimitReader(resp.Body, MaxRespuesta))
	if err != nil {
		return tabla, fmt.Errorf("pasarela de integraciones: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// El cuerpo de error de la pasarela es JSON con código estable; se pasa
		// recortado para que el motivo llegue a quien importó, sin arrastrar un
		// volcado entero a la interfaz.
		return tabla, fmt.Errorf("pasarela de integraciones (%d): %s",
			resp.StatusCode, recortar(string(crudo), 300))
	}
	if err := json.Unmarshal(crudo, &tabla); err != nil {
		return tabla, fmt.Errorf("respuesta de la pasarela ilegible: %w", err)
	}
	return tabla, nil
}

func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
