// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// LegalHold es una orden de preservación de correo para litigios (paridad M365
// Purview / Google Vault). Mientras está activa (ReleasedAt == nil), el correo
// del sujeto NUNCA se purga de verdad: el borrado normal lo sigue ocultando de
// la bandeja (soft-delete), pero la purga física y el cascade de borrado de
// carpeta lo respetan. Subject vacío ("") = hold de TODA la org.
type LegalHold struct {
	ID         uuid.UUID  `json:"id"`
	Tenant     string     `json:"-"`
	Subject    string     `json:"subject,omitempty"` // sub de Sytue ID; "" = toda la org
	Reason     string     `json:"reason,omitempty"`
	Case       string     `json:"case,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	ReleasedBy string     `json:"released_by,omitempty"`
}

// OrgWide indica si el hold cubre a toda la organización (sin sujeto concreto).
func (h LegalHold) OrgWide() bool { return h.Subject == "" }

// Active indica si el hold sigue vigente (no liberado).
func (h LegalHold) Active() bool { return h.ReleasedAt == nil }

// EDiscoveryHit es un mensaje encontrado por una búsqueda de eDiscovery: el
// sobre/metadato del mensaje más si está PRESERVADO (soft-deleted: oculto de la
// bandeja del usuario pero conservado para el caso). La lista NO incluye el
// cuerpo completo; el export baja el .eml crudo de S3.
type EDiscoveryHit struct {
	Message   Message
	Preserved bool // deleted_at IS NOT NULL: borrado por el usuario pero retenido
}

// AuditEntry es una entrada de la traza de acceso privilegiado (compliance):
// quién (Actor), qué (Action), sobre quién (Target) y el detalle (caso/consulta).
// AuditEntry es la fila de la traza. Alias del tipo compartido: la forma es
// idéntica en los servicios que exponen auditoría —mismos campos y mismos tags—
// y quien la lee es el mismo código. Al ser un ALIAS y no un tipo nuevo, el
// store, los handlers y el JSON siguen exactamente igual.
type AuditEntry = auditlog.Entrada
