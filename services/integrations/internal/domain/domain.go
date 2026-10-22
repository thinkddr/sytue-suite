// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define las entidades del Sytue Integration Gateway que persisten
// y cruzan la API: el catálogo de plantillas (app_templates), las instancias
// configuradas por organización (app_instances) y el registro de egress auditado
// (egress_audit). Los VALORES de secretos NUNCA viven aquí: HasSecrets es un
// booleano derivado; el material sensible lo sella la capa store con keycrypto.
package domain

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound indica que la entidad pedida no existe en el tenant activo.
var ErrNotFound = errors.New("integrations: recurso no encontrado")

var ErrForbidden = errors.New("integrations: personal connection belongs to another actor")

// ErrInvalidInput indica una entrada de usuario inválida (la capa HTTP → 400).
var ErrInvalidInput = errors.New("integrations: entrada inválida")

// ErrConflict indica que la operación choca con el estado actual (p. ej. un
// trabajo que ya está corriendo).
var ErrConflict = errors.New("integrations: conflicto con el estado actual")

// Lector es una entrada de la lista de quién puede consultar una conexión (NEG-06): una
// persona por su `sub` (Kind "user") o un grupo de Sytue ID por su nombre (Kind "group"),
// que es lo que trae el claim `groups` del token. Name solo se enseña en la consola.
type Lector struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Clases de Lector.
const (
	LectorPersona = "user"
	LectorGrupo   = "group"
)

// AppTemplate es una plantilla del catálogo: un tipo de conector instalable
// (p. ej. "salesforce", "slack"). Manifest describe la config/secretos/
// capacidades que el driver espera (JSON opaco para el núcleo). Tier segmenta por
// plan comercial; Enabled permite retirar una plantilla sin borrarla.
type AppTemplate struct {
	Key      string          `json:"key"`
	Manifest json.RawMessage `json:"manifest"`
	Tier     string          `json:"tier"`
	Enabled  bool            `json:"enabled"`
}

// Status es el estado del ciclo de vida de una instancia.
type Status string

const (
	StatusDraft    Status = "draft"    // creada, aún sin verificar
	StatusActive   Status = "active"   // verificada y operativa
	StatusError    Status = "error"    // el último Test/uso falló
	StatusDisabled Status = "disabled" // desactivada por el admin
)

// Valid indica si el estado es uno de los permitidos.
func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusActive, StatusError, StatusDisabled:
		return true
	default:
		return false
	}
}

// AppInstance es una instancia de plantilla configurada por una organización.
// Config es la configuración PÚBLICA (jsonb, sin secretos). HasSecrets indica si
// hay secretos sellados asociados (el valor NUNCA se expone). LevelEnabled son las
// capacidades/niveles habilitados para esta instancia.
type AppInstance struct {
	ID           string          `json:"id"`
	OrgID        string          `json:"org_id"`
	TemplateKey  string          `json:"template_key"`
	Name         string          `json:"name"`
	Config       json.RawMessage `json:"config"`
	Status       Status          `json:"status"`
	HasSecrets   bool            `json:"has_secrets"`
	LevelEnabled []string        `json:"level_enabled"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	// Consent readiness is computed server-side without opening or exposing tokens.
	ConsentManifest        json.RawMessage `json:"-"`
	HasOAuthClientSecret   bool            `json:"-"`
	HasOAuthToken          bool            `json:"-"`
	ConsentTemplateEnabled bool            `json:"-"`
}

// ConfigMap aplana la config pública a un mapa de strings. Los valores no textuales se
// omiten: la config de una instancia es un objeto plano de cadenas por contrato (lo
// valida el alta), y lo que no encaje no debe llegar a una plantilla de URL saliente.
func (a *AppInstance) ConfigMap() map[string]string {
	if len(a.Config) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(a.Config, &raw); err != nil {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

// ScimKind distingue el tipo de entidad mapeada en scim_map (usuario o grupo).
type ScimKind string

const (
	ScimUser  ScimKind = "user"
	ScimGroup ScimKind = "group"
)

// ScimMapEntry es un renglón de scim_map: el mapeo entre una entidad de Sytue
// (SytueID) y su recurso en el SaaS (RemoteID) para una instancia SCIM, junto al
// hash del último estado empujado (Hash) que hace idempotente el reconcile-por-diff.
type ScimMapEntry struct {
	Kind     ScimKind
	SytueID  string
	RemoteID string
	Hash     string
}

// SyncCounts resume el resultado de una reconciliación: cuántos recursos se
// crearon, actualizaron, desactivaron y cuántos fallaron.
type SyncCounts struct {
	Created     int `json:"created"`
	Updated     int `json:"updated"`
	Deactivated int `json:"deactivated"`
	Errors      int `json:"errors"`
}

// SyncRun es una ejecución de sincronización (historial de GET /sync-runs).
// FinishedAt es cero mientras el run está en curso (Status "running").
type SyncRun struct {
	ID         string     `json:"id"`
	InstanceID string     `json:"instance_id"`
	Status     string     `json:"status"` // running | ok | error
	Counts     SyncCounts `json:"counts"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// EgressAudit es una fila del registro de tráfico saliente a terceros. Solo
// METADATOS: nunca payloads ni el valor de Authorization (Auth guarda "REDACTED"
// si la petición llevaba cabecera de autorización, jamás el token).
type EgressAudit struct {
	OrgID      string
	InstanceID string
	Host       string
	Method     string
	Status     int
	ReqBytes   int64
	RespBytes  int64
	LatencyMS  int64
	Auth       string // "REDACTED" si hubo Authorization; "" si no.
	Err        string // mensaje de error de transporte, si lo hubo.
	OccurredAt time.Time
}

// PolicyAction es el veredicto de una regla de gobernanza sobre un egress:
//   - allow: dejar salir (exención explícita; corta la evaluación).
//   - warn:  dejar salir pero marcar (queda en policy_decisions).
//   - block: NO dejar salir (el request nunca llega al transporte) y registrar.
type PolicyAction string

const (
	PolicyAllow PolicyAction = "allow"
	PolicyWarn  PolicyAction = "warn"
	PolicyBlock PolicyAction = "block"
)

// Valid indica si la acción es una de las permitidas.
func (a PolicyAction) Valid() bool {
	switch a {
	case PolicyAllow, PolicyWarn, PolicyBlock:
		return true
	default:
		return false
	}
}

// Policy es una regla de gobernanza de egress por organización. Condition es un
// objeto JSON con campos opcionales AND-combinados (api_family, host, method,
// path_prefix, body_contains); vacío casa TODO. El motor (internal/policy) la
// compila a su forma evaluable; el SQL la trata como jsonb opaco.
type Policy struct {
	ID        string          `json:"id"`
	OrgID     string          `json:"org_id"`
	Name      string          `json:"name"`
	Condition json.RawMessage `json:"condition"`
	Action    PolicyAction    `json:"action"`
	Enabled   bool            `json:"enabled"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// PolicyDecision es una decisión NO trivial (warn/block) tomada en el egress. Es
// la evidencia de auditoría de "qué regla marcó/bloqueó qué"; un block no deja
// fila en egress_audit (nunca salió), así que esta es su único registro. Solo
// metadatos: RequestID correlaciona con el X-Request-Id de la petición del usuario.
type PolicyDecision struct {
	OrgID      string
	InstanceID string
	PolicyID   string
	PolicyName string
	Action     PolicyAction
	RequestID  string
	Host       string
	Method     string
	OccurredAt time.Time
}
