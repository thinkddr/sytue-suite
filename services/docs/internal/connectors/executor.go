// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// Executor ejecuta consultas contra orígenes externos. Reutiliza un único
// *http.Client SSRF-safe (pool de conexiones) para REST; las conexiones
// PostgreSQL se abren por ejecución (ver postgres.go). Cachea los access tokens de
// OAuth2 client_credentials entre ejecuciones (ver oauth.go). Es concurrente-seguro.
type Executor struct {
	client *http.Client
	// internal es el cliente para el origen GATEWAY: un servicio INTERNO de la suite
	// (Sytue Integration Gateway), sin barrera SSRF (bloquearía su IP de clúster).
	internal *http.Client
	// gatewayURL es la raíz INTERNA del Sytue Integration Gateway (config de
	// SERVICIO, no de conexión): destino fijo del origen KindGateway, para no
	// reenviar el JWT del usuario a un host que elija el admin.
	gatewayURL string
	// basesURL es la raíz INTERNA de Sytue Bases (config de SERVICIO): destino fijo del
	// origen KindBases, para no mandar el token de una conexión a un host que elija nadie.
	basesURL string
	limits   Limits
	tokens   *tokenCache // access tokens OAuth2 client_credentials, por credencial (thread-safe)
}

// NewExecutor construye el ejecutor con los límites dados (con defaults) y la raíz
// del gateway (config de servicio; el llamante la aporta desde INTEGRATIONS_URL).
func NewExecutor(limits Limits, gatewayURL string) *Executor {
	limits = limits.withDefaults()
	return &Executor{
		client:     ssrfhttp.NewClient(limits.Timeout, 5),
		internal:   &http.Client{Timeout: limits.Timeout},
		gatewayURL: gatewayURL,
		limits:     limits,
		tokens:     newTokenCache(),
	}
}

// ConBases fija la raíz interna de Sytue Bases (config de servicio, BASES_URL). Sin
// ella, el origen KindBases responde ErrInvalid.
func (e *Executor) ConBases(url string) *Executor {
	e.basesURL = url
	return e
}

// ExecuteRaw decodifica specRaw (jsonb de una consulta guardada o inline) a
// QuerySpec y ejecuta. Es la puerta de entrada de la capa HTTP.
func (e *Executor) ExecuteRaw(ctx context.Context, kind Kind, connCfg json.RawMessage, creds Credentials, specRaw json.RawMessage) (*Result, error) {
	var spec QuerySpec
	if err := json.Unmarshal(specRaw, &spec); err != nil {
		return nil, invalid("la consulta no es JSON válido")
	}
	return e.Execute(ctx, kind, connCfg, creds, spec)
}

// Execute ejecuta spec contra la conexión (kind + connCfg público) usando creds
// YA descifradas, aplica el transform opcional y devuelve la tabla. Impone un
// timeout global. connCfg y creds NO se registran ni se devuelven al cliente.
//
// Contrato del resultado: { columns: [{name, type}], rows: [[...]] } — filas
// alineadas a columns; valores JSON-nativos (número/texto/bool/null) o JSON crudo
// para anidados. Es el contrato que consume el carril editor (POST execute).
func (e *Executor) Execute(ctx context.Context, kind Kind, connCfg json.RawMessage, creds Credentials, spec QuerySpec) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, e.limits.Timeout)
	defer cancel()

	var (
		res *Result
		err error
	)
	switch kind {
	case KindREST:
		var cfg RestConfig
		if err = json.Unmarshal(connCfg, &cfg); err != nil {
			return nil, invalid("config REST corrupta")
		}
		if err := cfg.validate(); err != nil {
			return nil, err
		}
		if spec.REST == nil {
			return nil, invalid("falta la consulta REST")
		}
		if err := spec.REST.validate(); err != nil {
			return nil, err
		}
		res, err = e.executeRESTAuthed(ctx, cfg, creds, *spec.REST)
	case KindPostgres:
		var cfg PostgresConfig
		if err = json.Unmarshal(connCfg, &cfg); err != nil {
			return nil, invalid("config PostgreSQL corrupta")
		}
		if err := cfg.validate(); err != nil {
			return nil, err
		}
		if spec.Postgres == nil {
			return nil, invalid("falta la consulta PostgreSQL")
		}
		if err := spec.Postgres.validate(); err != nil {
			return nil, err
		}
		res, err = executePostgres(ctx, cfg, creds.Secret, *spec.Postgres, e.limits)
	case KindGateway:
		var cfg GatewayConfig
		if err = json.Unmarshal(connCfg, &cfg); err != nil {
			return nil, invalid("config del gateway corrupta")
		}
		if err := cfg.validate(); err != nil {
			return nil, err
		}
		if spec.Gateway == nil {
			return nil, invalid("falta la consulta del gateway")
		}
		if err := spec.Gateway.validate(); err != nil {
			return nil, err
		}
		// Passthrough del token del usuario (efímero, por el contexto). La raíz del
		// gateway es config de servicio (e.gatewayURL); solo el InstanceID viene de la
		// conexión. La red va por el cliente INTERNO (destino de la suite, sin SSRF).
		res, err = executeGateway(ctx, e.internal, e.gatewayURL, cfg.InstanceID, *spec.Gateway, userToken(ctx), e.limits)
	case KindBases:
		// La raíz es config de servicio y el token, la credencial cifrada de la conexión.
		// Red por el cliente INTERNO (destino de la suite, sin SSRF), como el gateway.
		res, err = executeBases(ctx, e.internal, e.basesURL, creds.Secret, e.limits)
	default:
		return nil, invalid("tipo de conector no soportado: %q", kind)
	}
	if err != nil {
		return nil, err
	}

	if spec.Transform != nil {
		if err := spec.Transform.validate(); err != nil {
			return nil, err
		}
		procedencia := res.Provenance
		res = applyTransform(res, spec.Transform)
		res.Provenance = procedencia // los pasos aplicados no cambian de dónde vienen los datos
	}
	return res, nil
}
