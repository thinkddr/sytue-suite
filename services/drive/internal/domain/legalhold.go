// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/auditlog"
)

// ErrLegalHold indica que la operación borraría FÍSICAMENTE un fichero (o su
// subárbol) de un sujeto bajo un legal hold activo, y por eso se rechaza: el hold
// preserva el fichero y todas sus versiones. La capa HTTP lo traduce a 409. El
// soft-delete (papelera) y el resto de operaciones NO se ven afectados; solo la
// purga/borrado-físico. Al liberar el hold, el fichero vuelve a ser purgable.
var ErrLegalHold = errors.New("drive: fichero bajo retención legal (legal hold)")

// LegalHold es una orden de preservación de ficheros para litigios (paridad M365
// Purview / Google Vault). Mientras está activa (ReleasedAt == nil), los ficheros
// del sujeto —y TODAS sus versiones— NUNCA se purgan de verdad: el borrado normal
// los sigue enviando a la papelera (soft-delete), pero la purga física (manual y
// auto-purga) y la cascada de borrado de carpeta/unidad la respetan. Subject vacío
// ("") = hold de TODA la org.
type LegalHold struct {
	ID         uuid.UUID  `json:"id"`
	Tenant     string     `json:"-"`
	Subject    string     `json:"subject,omitempty"` // sub de Sytue ID (owner_sub); "" = toda la org
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

// EDiscoveryHit es un fichero encontrado por una búsqueda de eDiscovery: el
// metadato del fichero más si está PRESERVADO (soft-deleted: oculto de la unidad
// del usuario pero conservado para el caso). La lista NO baja el contenido; el
// export baja los blobs de S3 y los empaqueta en un ZIP.
type EDiscoveryHit struct {
	File      File
	Preserved bool // deleted_at IS NOT NULL: borrado por el usuario pero retenido
}

// AuditEntry es una entrada de la traza de acceso privilegiado (compliance):
// quién (Actor), qué (Action), sobre quién (Target) y el detalle (caso/consulta).
// AuditEntry es la fila de la traza. Alias del tipo compartido: la forma es
// idéntica en los servicios que exponen auditoría —mismos campos y mismos tags—
// y quien la lee es el mismo código. Al ser un ALIAS y no un tipo nuevo, el
// store, los handlers y el JSON siguen exactamente igual.
type AuditEntry = auditlog.Entrada
