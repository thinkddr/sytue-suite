// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Origen BASES: trae a Sytue Calcus una tabla —o una vista— de Sytue Bases, de SOLO
// LECTURA, reutilizando el pipeline de este paquete (transform + refresh).
//
// La autorización NO es el token de la persona (como en el gateway) sino un TOKEN DE
// CONEXIÓN que concedió el propietario de la base: acotado a una tabla o una vista, con
// caducidad y revocable. Viaja cifrado como credencial de la conexión y se manda en
// `Authorization: Bearer` a una raíz FIJA (config de servicio): no hay forma de dirigirlo a
// otro host. Sytue Bases lo comprueba en CADA lectura, así que revocar corta en el acto:
// aquí no se cachea nada. La respuesta trae la PROCEDENCIA, que se devuelve al editor.

// prefijoTokenBases es el de los tokens de conexión de Sytue Bases. Se comprueba antes de
// mandar nada: cualquier otro secreto guardado por error no sale hacia Bases.
const prefijoTokenBases = "sbc_"

// executeBases hace el GET a `/_bases/v1/connector/read` y decodifica { columns, rows,
// provenance }. Un 401 es «conexión revocada o caducada», con mensaje apto para el usuario.
func executeBases(ctx context.Context, client *http.Client, baseURL, token string, lim Limits) (*Result, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, invalid("Sytue Bases no está configurado en este despliegue")
	}
	if !strings.HasPrefix(token, prefijoTokenBases) {
		return nil, invalid("falta el token de la conexión de Sytue Bases")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/_bases/v1/connector/read", http.NoBody)
	if err != nil {
		return nil, invalid("no se pudo construir la petición a Sytue Bases")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Docs-Connector/1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: Sytue Bases: %w", ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, invalid("la conexión con Sytue Bases se ha revocado o ha caducado: pide una nueva a quien administra la base")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: Sytue Bases respondió HTTP %d", ErrUpstream, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, lim.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: leer respuesta de Sytue Bases: %w", ErrUpstream, err)
	}
	if int64(len(raw)) > lim.MaxBytes {
		return nil, fmt.Errorf("%w: respuesta de Sytue Bases demasiado grande (> %d bytes)", ErrUpstream, lim.MaxBytes)
	}
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("%w: la respuesta de Sytue Bases no es una tabla", ErrUpstream)
	}
	if res.Columns == nil {
		res.Columns = []Column{}
	}
	if res.Rows == nil {
		res.Rows = [][]any{}
	}
	if len(res.Columns) > maxColumns {
		return nil, fmt.Errorf("%w: demasiadas columnas", ErrUpstream)
	}
	if len(res.Rows) > lim.MaxRows {
		res.Rows = res.Rows[:lim.MaxRows]
	}
	return &res, nil
}
