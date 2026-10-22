// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package connectors es el conector de datos externos de Sytue Sheets (estilo
// Power Query): trae datos de un origen externo, los materializa como una tabla
// (columnas + filas) y permite refrescar. Dos orígenes de primer nivel:
//
//	REST  — petición HTTP autenticada a una API externa; el JSON se aplana a
//	        filas/columnas. TODA la superficie de red pasa por la barrera SSRF
//	        (platform/pkg/ssrfhttp): jamás alcanza un destino interno.
//	Postgres — consulta SOLO-LECTURA a un PostgreSQL externo: un ÚNICO SELECT
//	        parametrizado, en transacción read-only con statement_timeout y tope
//	        de filas; el host también pasa por la barrera SSRF.
//
// Modelo de seguridad (crítico): las credenciales viajan descifradas SOLO en
// memoria durante la ejecución (el cifrado at-rest lo hace la capa store con el
// Sealer de este paquete); NUNCA se persisten en claro, NUNCA se registran ni se
// devuelven al cliente. La inyección SQL es imposible por construcción (params
// posicionales + guard de SELECT único). El aislamiento por tenant lo refuerza
// RLS en la capa store. Este paquete es puro cómputo + E/S de red saliente: no
// conoce PostgreSQL interno, tenants ni HTTP entrante.
package connectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Kind es el tipo de origen externo.
type Kind string

const (
	KindREST     Kind = "rest"
	KindPostgres Kind = "postgres"
	KindGateway  Kind = "gateway"
	// KindBases lee una tabla o una vista de Sytue Bases con un token de conexión de
	// SOLO LECTURA que concede (y revoca) el propietario de la base. Ver bases.go.
	KindBases Kind = "bases"
)

// Valid indica si el tipo de conector es conocido.
func (k Kind) Valid() bool {
	return k == KindREST || k == KindPostgres || k == KindGateway || k == KindBases
}

// ErrInvalid es el error centinela de validación de entrada (la capa HTTP lo
// mapea a 400). Su mensaje es apto para el usuario (español, sin filtrar
// internals).
var ErrInvalid = errors.New("connectors: entrada inválida")

// invalid construye un ErrInvalid con mensaje formateado.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// -----------------------------------------------------------------------------
// Entidades persistidas (la capa store las mapea a filas; el tenant y las
// credenciales cifradas se gestionan aparte y nunca se serializan aquí).
// -----------------------------------------------------------------------------

// DataConnection es una conexión a un origen externo. Config es la configuración
// PÚBLICA (sin secretos). HasCredentials indica si hay credenciales cifradas
// guardadas — el secreto en sí NUNCA se expone.
type DataConnection struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Kind           Kind            `json:"kind"`
	Config         json.RawMessage `json:"config"`
	HasCredentials bool            `json:"has_credentials"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// DataQuery es una consulta guardada sobre una conexión (Spec = QuerySpec).
type DataQuery struct {
	ID           string          `json:"id"`
	ConnectionID string          `json:"connection_id"`
	Name         string          `json:"name"`
	Spec         json.RawMessage `json:"spec"`
	CreatedBy    string          `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Credentials es el material secreto de una conexión, descifrado en memoria.
// Un único campo Secret, interpretado según el tipo de auth/conexión:
//   - REST api_key => la clave; REST bearer => el token; REST basic => la
//     contraseña (el usuario va en la config pública); postgres => la contraseña.
type Credentials struct {
	Secret string `json:"secret,omitempty"`
}

// -----------------------------------------------------------------------------
// Configuración pública por tipo (se guarda como jsonb en data_connections).
// -----------------------------------------------------------------------------

// RestAuthType es el esquema de autenticación de una API REST.
type RestAuthType string

const (
	RestAuthNone   RestAuthType = "none"
	RestAuthAPIKey RestAuthType = "api_key"
	RestAuthBearer RestAuthType = "bearer"
	RestAuthBasic  RestAuthType = "basic"
	//nolint:gosec // G101: nombre del modo de auth, no una credencial.
	RestAuthOAuth2ClientCreds RestAuthType = "oauth2_client_credentials"
)

// RestAuth describe cómo autenticar (sin el secreto en sí).
type RestAuth struct {
	Type RestAuthType `json:"type"`
	// In: dónde va la api_key: "header" | "query" (solo para api_key).
	In string `json:"in,omitempty"`
	// Name: nombre de la cabecera o del parámetro de query de la api_key.
	Name string `json:"name,omitempty"`
	// Username: usuario de basic auth (público; la contraseña va cifrada).
	Username string `json:"username,omitempty"`

	// --- OAuth2 client_credentials (RFC 6749 §4.4) ---
	// El client_secret NO va aquí: es una credencial y viaja CIFRADO en
	// Credentials.Secret, como cualquier otro secreto REST. Estos campos son
	// PÚBLICOS. Ver oauth.go para el flujo (obtener token + caché + Bearer).
	//
	// TokenURL: el token endpoint (URL externa; pasa por la barrera SSRF al pedir
	// el token). ClientID: identificador público del cliente. Scope/Audience:
	// opcionales. ClientAuth: cómo enviar client_id/secret al token endpoint —
	// "body" (client_secret_post, por defecto) o "basic" (client_secret_basic).
	TokenURL   string `json:"token_url,omitempty"`
	ClientID   string `json:"client_id,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Audience   string `json:"audience,omitempty"`
	ClientAuth string `json:"client_auth,omitempty"`
}

// RestConfig es la config pública de una conexión REST.
type RestConfig struct {
	// BaseURL es el origen/base http(s) de la API (validado contra SSRF al ejecutar).
	BaseURL string   `json:"base_url"`
	Auth    RestAuth `json:"auth"`
	// Headers estáticas NO secretas añadidas a cada petición (opcional).
	Headers map[string]string `json:"headers,omitempty"`
}

// PostgresConfig es la config pública de una conexión PostgreSQL (la contraseña
// va cifrada como credencial, jamás aquí).
type PostgresConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
	Database string `json:"database"`
	User     string `json:"user"`
	// SSLMode: allowlist libpq. Por defecto "prefer".
	SSLMode string `json:"sslmode,omitempty"`
}

// -----------------------------------------------------------------------------
// Especificación de consulta (se guarda como jsonb en data_queries y también se
// acepta inline en el endpoint execute para refrescar sin persistir).
// -----------------------------------------------------------------------------

// RestQuery es la consulta a una API REST.
type RestQuery struct {
	// Method: GET (por defecto) | POST.
	Method string `json:"method,omitempty"`
	// Path: ruta RELATIVA a BaseURL (nunca absoluta: no puede cambiar de host).
	Path string `json:"path,omitempty"`
	// Query: parámetros de query añadidos a la URL.
	Query map[string]string `json:"query,omitempty"`
	// Headers: cabeceras NO secretas de esta consulta.
	Headers map[string]string `json:"headers,omitempty"`
	// Body: cuerpo JSON para POST (opcional).
	Body json.RawMessage `json:"body,omitempty"`
	// RootPath: json-path SIMPLE (dot / [idx]) al array dentro del JSON de
	// respuesta. Vacío => se usa el cuerpo entero (array u objeto único).
	RootPath string `json:"root_path,omitempty"`
}

// PostgresQuery es la consulta a un PostgreSQL externo.
type PostgresQuery struct {
	// SQL: un ÚNICO SELECT (o WITH ... SELECT). Se valida con guardSelect.
	SQL string `json:"sql"`
	// Params: parámetros posicionales para $1..$n (NUNCA se concatenan al SQL).
	Params []any `json:"params,omitempty"`
}

// QuerySpec es la consulta completa: la parte específica del tipo + un transform
// opcional. Solo una de REST/Postgres/Gateway se rellena, según el tipo de la
// conexión.
type QuerySpec struct {
	REST      *RestQuery     `json:"rest,omitempty"`
	Postgres  *PostgresQuery `json:"postgres,omitempty"`
	Gateway   *GatewayQuery  `json:"gateway,omitempty"`
	Transform *Transform     `json:"transform,omitempty"`
}

// -----------------------------------------------------------------------------
// Resultado tabular (contrato del endpoint execute).
// -----------------------------------------------------------------------------

// Column es una columna del resultado: nombre + tipo lógico inferido.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Result es la tabla materializada: columnas + filas (fila = valores alineados a
// Columns). Los valores son JSON-nativos (número, texto, bool, null) o
// json.RawMessage para anidados.
type Result struct {
	Columns []Column `json:"columns"`
	Rows    [][]any  `json:"rows"`
	// Provenance es la PROCEDENCIA que declara el origen, cuando la da (hoy, Sytue Bases):
	// qué base, tabla y vista, quién lo concedió, cuándo y qué versión. Se devuelve tal
	// cual al editor, que la enseña junto a la tabla; nunca lleva credenciales.
	Provenance json.RawMessage `json:"provenance,omitempty"`
}

// Limits acota una ejecución: filas, bytes de respuesta y tiempo total.
type Limits struct {
	MaxRows  int
	MaxBytes int64
	Timeout  time.Duration
}

const (
	defaultMaxRows  = 10000
	defaultMaxBytes = 8 << 20 // 8 MiB
	defaultTimeout  = 20 * time.Second
	maxColumns      = 512 // guarda contra resultados patológicamente anchos
	maxNameLen      = 200
)

func (l Limits) withDefaults() Limits {
	if l.MaxRows <= 0 {
		l.MaxRows = defaultMaxRows
	}
	if l.MaxBytes <= 0 {
		l.MaxBytes = defaultMaxBytes
	}
	if l.Timeout <= 0 {
		l.Timeout = defaultTimeout
	}
	return l
}

// -----------------------------------------------------------------------------
// Validación / saneado.
// -----------------------------------------------------------------------------

// ValidateName valida un nombre de conexión/consulta.
func ValidateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalid("el nombre es obligatorio")
	}
	if len(name) > maxNameLen {
		return "", invalid("el nombre es demasiado largo (máx. %d)", maxNameLen)
	}
	return name, nil
}

// Destino es A DÓNDE VIAJA la credencial guardada de una conexión, sacado de su config
// ya canónica (SanitizeConfig): la URL base, el tipo de autenticación y la URL del token
// en REST; el host, el puerto y el modo TLS en PostgreSQL. Los orígenes de la casa
// (gateway, Bases) van a una raíz fija del servicio y no tienen destino que cambiar.
//
// Quien edita una conexión sin volver a escribir su secreto puede cambiarlo todo menos
// esto: con el destino distinto, la credencial guardada se invalida (ver
// http.updateConnection). Si no, bastaba apuntarla a un servidor propio y ejecutarla
// para recibir el secreto que guardó otra persona.
func Destino(kind Kind, config json.RawMessage) string {
	switch kind {
	case KindREST:
		var c RestConfig
		_ = json.Unmarshal(config, &c)
		return c.BaseURL + "\n" + string(c.Auth.Type) + "\n" + c.Auth.TokenURL
	case KindPostgres:
		var c PostgresConfig
		_ = json.Unmarshal(config, &c)
		return fmt.Sprintf("%s\n%d\n%s", c.Host, c.Port, c.SSLMode)
	case KindGateway, KindBases:
	}
	return ""
}

// SanitizeConfig valida la config pública de una conexión y devuelve su forma
// canónica (re-serializada: descarta campos desconocidos y normaliza). No hace
// red (la resolución/SSRF ocurre al ejecutar).
func SanitizeConfig(kind Kind, raw json.RawMessage) (json.RawMessage, error) {
	switch kind {
	case KindREST:
		var c RestConfig
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, invalid("config REST no es JSON válido")
		}
		if err := c.validate(); err != nil {
			return nil, err
		}
		return mustMarshal(c), nil
	case KindPostgres:
		var c PostgresConfig
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, invalid("config PostgreSQL no es JSON válido")
		}
		if err := c.validate(); err != nil {
			return nil, err
		}
		return mustMarshal(c), nil
	case KindGateway:
		var c GatewayConfig
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, invalid("config del gateway no es JSON válido")
		}
		if err := c.validate(); err != nil {
			return nil, err
		}
		return mustMarshal(c), nil
	case KindBases:
		// Sin config de conexión: la raíz de Sytue Bases es config de SERVICIO y lo que se
		// lee lo fija el token (una tabla o una vista), que va cifrado como credencial.
		return json.RawMessage(`{}`), nil
	default:
		return nil, invalid("tipo de conector no soportado: %q", kind)
	}
}

func (c *RestConfig) validate() error {
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	if err := checkHTTPURL(c.BaseURL, "la URL base"); err != nil {
		return err
	}
	if c.Auth.Type == "" {
		c.Auth.Type = RestAuthNone
	}
	switch c.Auth.Type {
	case RestAuthNone, RestAuthBearer, RestAuthBasic:
	case RestAuthAPIKey:
		c.Auth.In = strings.TrimSpace(c.Auth.In)
		c.Auth.Name = strings.TrimSpace(c.Auth.Name)
		if c.Auth.In != "header" && c.Auth.In != "query" {
			return invalid("api_key: 'in' debe ser 'header' o 'query'")
		}
		if c.Auth.Name == "" {
			return invalid("api_key: falta el nombre de la cabecera/parámetro")
		}
	case RestAuthOAuth2ClientCreds:
		if err := c.Auth.validateOAuth2(); err != nil {
			return err
		}
	default:
		return invalid("tipo de auth REST inválido: %q", c.Auth.Type)
	}
	for k := range c.Headers {
		if strings.TrimSpace(k) == "" {
			return invalid("cabecera con nombre vacío")
		}
	}
	return nil
}

// validateOAuth2 valida y normaliza los campos públicos de client_credentials. El
// client_secret NO se valida aquí (es una credencial cifrada, no config): si falta,
// el token endpoint rechazará la petición con un error claro al ejecutar.
func (a *RestAuth) validateOAuth2() error {
	a.TokenURL = strings.TrimSpace(a.TokenURL)
	a.ClientID = strings.TrimSpace(a.ClientID)
	a.Scope = strings.TrimSpace(a.Scope)
	a.Audience = strings.TrimSpace(a.Audience)
	a.ClientAuth = strings.TrimSpace(a.ClientAuth)
	if err := checkHTTPURL(a.TokenURL, "token_url"); err != nil {
		return err
	}
	if a.ClientID == "" {
		return invalid("oauth2: falta client_id")
	}
	if a.ClientAuth == "" {
		a.ClientAuth = "body" // client_secret_post por defecto
	}
	if a.ClientAuth != "body" && a.ClientAuth != "basic" {
		return invalid("oauth2: client_auth debe ser 'body' o 'basic'")
	}
	return nil
}

// sslModes es el allowlist de sslmode de libpq.
var sslModes = map[string]bool{
	"disable": true, "allow": true, "prefer": true,
	"require": true, "verify-ca": true, "verify-full": true,
}

func (c *PostgresConfig) validate() error {
	c.Host = strings.TrimSpace(c.Host)
	c.Database = strings.TrimSpace(c.Database)
	c.User = strings.TrimSpace(c.User)
	c.SSLMode = strings.TrimSpace(c.SSLMode)
	if c.Host == "" {
		return invalid("falta el host de PostgreSQL")
	}
	if c.Port == 0 {
		c.Port = 5432
	}
	if c.Port < 1 || c.Port > 65535 {
		return invalid("puerto fuera de rango")
	}
	if c.Database == "" {
		return invalid("falta la base de datos")
	}
	if c.User == "" {
		return invalid("falta el usuario")
	}
	if c.SSLMode == "" {
		c.SSLMode = "prefer"
	}
	if !sslModes[c.SSLMode] {
		return invalid("sslmode inválido: %q", c.SSLMode)
	}
	return nil
}

// SanitizeSpec valida una QuerySpec contra el tipo de la conexión y devuelve su
// forma canónica.
func SanitizeSpec(kind Kind, raw json.RawMessage) (json.RawMessage, error) {
	spec, err := parseSpec(kind, raw)
	if err != nil {
		return nil, err
	}
	return mustMarshal(spec), nil
}

// parseSpec decodifica y valida una QuerySpec para el tipo dado.
func parseSpec(kind Kind, raw json.RawMessage) (QuerySpec, error) {
	var spec QuerySpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return QuerySpec{}, invalid("la consulta no es JSON válido")
	}
	switch kind {
	case KindREST:
		if spec.REST == nil {
			return QuerySpec{}, invalid("falta la consulta REST")
		}
		if err := spec.REST.validate(); err != nil {
			return QuerySpec{}, err
		}
		spec.Postgres = nil
		spec.Gateway = nil
	case KindPostgres:
		if spec.Postgres == nil {
			return QuerySpec{}, invalid("falta la consulta PostgreSQL")
		}
		if err := spec.Postgres.validate(); err != nil {
			return QuerySpec{}, err
		}
		spec.REST = nil
		spec.Gateway = nil
	case KindGateway:
		if spec.Gateway == nil {
			return QuerySpec{}, invalid("falta la consulta del gateway")
		}
		if err := spec.Gateway.validate(); err != nil {
			return QuerySpec{}, err
		}
		spec.REST = nil
		spec.Postgres = nil
	case KindBases:
		// Nada que consultar: el token ya dice qué tabla o vista. Solo admite transform.
		spec.REST, spec.Postgres, spec.Gateway = nil, nil, nil
	default:
		return QuerySpec{}, invalid("tipo de conector no soportado: %q", kind)
	}
	if spec.Transform != nil {
		if err := spec.Transform.validate(); err != nil {
			return QuerySpec{}, err
		}
	}
	return spec, nil
}

func (q *RestQuery) validate() error {
	q.Method = strings.ToUpper(strings.TrimSpace(q.Method))
	if q.Method == "" {
		q.Method = "GET"
	}
	if q.Method != http.MethodGet && q.Method != http.MethodPost {
		return invalid("método REST no permitido: %q (GET|POST)", q.Method)
	}
	// La ruta debe ser RELATIVA: no puede aportar esquema ni host (evita que la
	// consulta redirija la petición a otro destino, saltándose la BaseURL).
	if q.Path != "" {
		ref, err := url.Parse(q.Path)
		if err != nil {
			return invalid("ruta inválida")
		}
		if ref.IsAbs() || ref.Host != "" || strings.HasPrefix(q.Path, "//") {
			return invalid("la ruta debe ser relativa a la URL base")
		}
	}
	return nil
}

func (q *PostgresQuery) validate() error {
	q.SQL = strings.TrimSpace(q.SQL)
	if q.SQL == "" {
		return invalid("falta el SQL")
	}
	if err := guardSelect(q.SQL); err != nil {
		return err
	}
	if len(q.Params) > 512 {
		return invalid("demasiados parámetros")
	}
	return nil
}

// checkHTTPURL valida (sin resolver DNS) que rawURL sea http(s) absoluta con
// host. label nombra el campo en los mensajes de error. La barrera SSRF real
// (resolución + IP) actúa al ejecutar.
func checkHTTPURL(rawURL, label string) error {
	if rawURL == "" {
		return invalid("falta %s", label)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return invalid("%s no es válida", label)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return invalid("%s debe ser http(s)", label)
	}
	if u.Host == "" {
		return invalid("%s no tiene host", label)
	}
	return nil
}

// mustMarshal serializa un valor ya validado; el error es imposible (structs
// simples), así que se degrada a un JSON vacío defensivo.
func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}
