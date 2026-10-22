// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Origen GATEWAY: trae datos tabulares de un SaaS de terceros a Sytue Sheets a
// través del Sytue Integration Gateway (services/integrations), reutilizando TODO
// el pipeline de este paquete (transform «pasos aplicados» + refresh). El conector
// hace un ÚNICO POST a `/api/v1/instances/{id}/data/query` del gateway y recibe el
// mismo shape { columns, rows } (Result) que el resto de orígenes.
//
// A diferencia de REST/Postgres (destinos EXTERNOS que pasan por la barrera SSRF),
// el gateway es un servicio INTERNO de la suite: se llama con un http.Client normal
// (la barrera SSRF bloquearía su IP privada de clúster). La autorización es
// PASSTHROUGH del token del usuario (JWT de Sytue ID): el gateway comprueba que el
// llamante es miembro de la org y aísla por RLS. El token viaja por el contexto
// (WithUserToken), no como credencial persistida (es efímero por-petición).

// gatewayTokenKey es la clave de contexto del token de usuario para el passthrough.
type gatewayTokenKey struct{}

// WithUserToken adjunta al contexto el JWT del usuario para el passthrough al
// gateway. La capa HTTP de docs (executeConnectorQuery) la llama con el Bearer
// entrante antes de invocar al Executor —cubre el execute inline y el refresh de
// consulta guardada: ambos pasan por ese único handler—. Sin token en el
// contexto, un origen gateway responde ErrInvalid.
func WithUserToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, gatewayTokenKey{}, token)
}

// userToken recupera el token de usuario del contexto (vacío si no está).
func userToken(ctx context.Context) string {
	s, _ := ctx.Value(gatewayTokenKey{}).(string)
	return s
}

// GatewayConfig es la config pública de una conexión gateway: SOLO la instancia de
// conector a consultar. La raíz del gateway NO es config de conexión (sería entrada
// del admin y permitiría reenviar el JWT del usuario a un host arbitrario): viene
// de la config de SERVICIO de docs (INTEGRATIONS_URL) y la fija el Executor.
type GatewayConfig struct {
	// InstanceID es el id de la instancia de conector en el gateway.
	InstanceID string `json:"instance_id"`
}

func (c *GatewayConfig) validate() error {
	c.InstanceID = strings.TrimSpace(c.InstanceID)
	if c.InstanceID == "" {
		return invalid("falta el id de instancia del gateway")
	}
	if strings.ContainsAny(c.InstanceID, "/?#") {
		return invalid("id de instancia del gateway inválido")
	}
	return nil
}

// GatewayQuery es la consulta a un origen gateway: la spec OPACA que se reenvía al
// driver de la instancia (dataset/modelo + $select/filtros/paginación). Su forma
// concreta la define el driver del gateway (OData/Odoo…); este conector no la
// interpreta, solo la transporta.
type GatewayQuery struct {
	Spec json.RawMessage `json:"spec,omitempty"`
}

func (q *GatewayQuery) validate() error {
	if len(q.Spec) == 0 {
		return nil // spec vacía: el driver del gateway aplica sus defaults.
	}
	if !json.Valid(q.Spec) {
		return invalid("la spec del gateway no es JSON válido")
	}
	return nil
}

// ErrNotAllowed: quien consulta no está en la lista de la conexión (NEG-06). No es un
// fallo del origen, y decirlo como tal mandaba a buscar una avería que no existe.
var ErrNotAllowed = errors.New("connectors: no estás en la lista de quién consulta esta conexión")

// codigoNoEnLaLista es el código estable con el que la pasarela lo dice.
const codigoNoEnLaLista = "connection_not_allowed"

// codigoDelGateway lee el código del sobre de error de la pasarela ({"error":{"code"}}).
func codigoDelGateway(cuerpo io.Reader) string {
	var sobre struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(cuerpo, 64<<10)).Decode(&sobre)
	return sobre.Error.Code
}

// executeGateway hace el POST a `/api/v1/instances/{id}/data/query` del gateway con
// el token del usuario (passthrough) y decodifica la respuesta { columns, rows } al
// Result del pipeline. baseURL es la raíz INTERNA del gateway (config de servicio,
// destino de confianza); client es un http.Client normal (interno, sin SSRF).
func executeGateway(ctx context.Context, client *http.Client, baseURL, instanceID string, q GatewayQuery, token string, lim Limits) (*Result, error) {
	if token == "" {
		return nil, invalid("falta el token de usuario para consultar el gateway")
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/instances/" + url.PathEscape(instanceID) + "/data/query"

	payload := q.Spec
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, invalid("no se pudo construir la petición al gateway")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Sytue-Docs-Connector/1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: gateway: %w", ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusForbidden && codigoDelGateway(resp.Body) == codigoNoEnLaLista {
			return nil, ErrNotAllowed
		}
		return nil, fmt.Errorf("%w: el gateway respondió HTTP %d", ErrUpstream, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, lim.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: leer respuesta del gateway: %w", ErrUpstream, err)
	}
	if int64(len(raw)) > lim.MaxBytes {
		return nil, fmt.Errorf("%w: respuesta del gateway demasiado grande (> %d bytes)", ErrUpstream, lim.MaxBytes)
	}
	// El gateway devuelve exactamente el shape de Result (columns/rows).
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("%w: la respuesta del gateway no es una tabla", ErrUpstream)
	}
	if res.Columns == nil {
		res.Columns = []Column{}
	}
	if res.Rows == nil {
		res.Rows = [][]any{}
	}
	if len(res.Rows) > lim.MaxRows {
		res.Rows = res.Rows[:lim.MaxRows]
	}
	return &res, nil
}
