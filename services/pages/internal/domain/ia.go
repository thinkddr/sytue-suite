// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Constantes de IA para proveedores y tipos de trabajo (§9.2-9.3).
const (
	AIProviderOpenAI    = "openai"
	AIProviderAnthropic = "anthropic"
	AIProviderZAI       = "zai"
	AIProviderMistral   = "mistral"

	AIJobKindDesign    = "design"
	AIJobKindTranslate = "translate"
	AIJobKindExport    = "export"

	AIJobStatusRunning = "running"
	AIJobStatusDone    = "done"
	AIJobStatusFailed  = "failed"
)

// AIKey representa las credenciales BYOK selladas de una organización.
type AIKey struct {
	ID               uuid.UUID  `json:"id"`
	Tenant           string     `json:"tenant"`
	Provider         string     `json:"provider"`
	Model            string     `json:"model"`
	KeyEnc           string     `json:"-"`
	NoticeAcceptedAt *time.Time `json:"notice_accepted_at,omitempty"`
	AcceptedBy       string     `json:"accepted_by"`
	CheckedAt        time.Time  `json:"checked_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

// AIKeyVista es la vista pública que se devuelve por la API (nunca en claro ni cifrada).
type AIKeyVista struct {
	Provider         string     `json:"provider"`
	Model            string     `json:"model"`
	HasKey           bool       `json:"has_key"`
	CheckedAt        *time.Time `json:"checked_at,omitempty"`
	NoticeAcceptedAt *time.Time `json:"notice_accepted_at,omitempty"`
}

// AIJob es un trabajo asíncrono de IA ejecutado para un sitio (§9.3).
type AIJob struct {
	ID         uuid.UUID       `json:"id"`
	Tenant     string          `json:"tenant"`
	SiteID     uuid.UUID       `json:"site_id"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	Input      json.RawMessage `json:"input,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	ResultKey  string          `json:"result_key,omitempty"`
	Error      string          `json:"error,omitempty"`
	Provider   string          `json:"provider"`
	BYOK       bool            `json:"byok"`
	Model      string          `json:"model"`
	TokensIn   int             `json:"tokens_in"`
	TokensOut  int             `json:"tokens_out"`
	CreatedBy  string          `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
}

// AIQuota resume el consumo y límite mensual de trabajos estándar (no BYOK).
type AIQuota struct {
	Limit   int       `json:"limit"`
	Used    int       `json:"used"`
	Month   string    `json:"month"`
	ResetAt time.Time `json:"reset_at"`
}

// DesignInput es la entrada del trabajo de diseño.
type DesignInput struct {
	Prompt   string `json:"prompt"`
	PageID   string `json:"page_id,omitempty"`
	Mode     string `json:"mode,omitempty"` // "page" | "sections"
	Language string `json:"language"`
}

// TranslateInput es la entrada del trabajo de traducción.
type TranslateInput struct {
	From      string   `json:"from"`
	To        []string `json:"to"`
	Scope     string   `json:"scope"` // "site" | "page:<id>" | "store"
	Overwrite bool     `json:"overwrite,omitempty"`
}

// ExportInput es la entrada del trabajo de exportación.
type ExportInput struct {
	Destination string          `json:"destination"`
	IR          json.RawMessage `json:"ir,omitempty"`
	Catalogo    json.RawMessage `json:"catalogo,omitempty"`
}
