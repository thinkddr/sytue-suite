// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain modela el núcleo de Sytue Docs/Sheets/Slides: el modelo
// documental, los permisos (deny-by-default), el versionado y el motor de
// edición colaborativa CRDT.
//
// Clean-room: la edición colaborativa se implementa con un CRDT de secuencia
// propio (RGA — Replicated Growable Array — sobre identidades densas con reloj
// Lamport), obra original de Sytue sobre literatura pública de CRDTs. No se
// porta ni consulta código AGPL (OnlyOffice/Nextcloud quedan fuera).
package domain

import (
	"encoding/json"
	"errors"
	"time"
)

// Kind es la naturaleza del documento ofimático.
type Kind string

const (
	KindDocument     Kind = "document"     // procesador de textos (Docs)
	KindSpreadsheet  Kind = "spreadsheet"  // hoja de cálculo (Sheets)
	KindPresentation Kind = "presentation" // presentación (Slides)
)

// Valid indica si el tipo de documento es conocido.
func (k Kind) Valid() bool {
	switch k {
	case KindDocument, KindSpreadsheet, KindPresentation:
		return true
	default:
		return false
	}
}

// Role es el rol de un principal sobre un documento (orden creciente de poder).
type Role string

const (
	RoleViewer    Role = "viewer"
	RoleCommenter Role = "commenter"
	RoleEditor    Role = "editor"
	RoleOwner     Role = "owner"
)

// roleRank ordena los roles para comparaciones "al menos".
var roleRank = map[Role]int{
	RoleViewer:    1,
	RoleCommenter: 2,
	RoleEditor:    3,
	RoleOwner:     4,
}

// Valid indica si el rol es conocido.
func (r Role) Valid() bool { _, ok := roleRank[r]; return ok }

// AtLeast indica si r concede al menos el poder de minRole.
func (r Role) AtLeast(minRole Role) bool {
	actual, ok := roleRank[r]
	minimum, valid := roleRank[minRole]
	return ok && valid && actual >= minimum
}

// CanRead, CanComment, CanWrite, CanManage son comprobaciones de capacidad.
func (r Role) CanRead() bool    { return r.AtLeast(RoleViewer) }
func (r Role) CanComment() bool { return r.AtLeast(RoleCommenter) }
func (r Role) CanWrite() bool   { return r.AtLeast(RoleEditor) }
func (r Role) CanManage() bool  { return r.AtLeast(RoleOwner) }

// PrincipalKind es la naturaleza del sujeto de un permiso.
type PrincipalKind string

const (
	PrincipalUser   PrincipalKind = "user"   // un usuario concreto (sub)
	PrincipalGroup  PrincipalKind = "group"  // un grupo de Sytue ID
	PrincipalTenant PrincipalKind = "tenant" // toda la organización
	PrincipalPublic PrincipalKind = "public" // enlace público/compartido
)

// Valid indica si el tipo de principal es conocido.
func (p PrincipalKind) Valid() bool {
	switch p {
	case PrincipalUser, PrincipalGroup, PrincipalTenant, PrincipalPublic:
		return true
	default:
		return false
	}
}

// BlobFormat es el formato de un export/import materializado en S3.
type BlobFormat string

const (
	FormatCRDT     BlobFormat = "sytue-crdt"
	FormatOOXML    BlobFormat = "ooxml"
	FormatODF      BlobFormat = "odf"
	FormatPDF      BlobFormat = "pdf"
	FormatHTML     BlobFormat = "html"
	FormatMarkdown BlobFormat = "markdown"
)

// Valid indica si el formato es conocido.
func (f BlobFormat) Valid() bool {
	switch f {
	case FormatCRDT, FormatOOXML, FormatODF, FormatPDF, FormatHTML, FormatMarkdown:
		return true
	default:
		return false
	}
}

// Document es un documento ofimático y sus metadatos. El estado editable vive
// en las operaciones CRDT + snapshots, no en esta estructura.
type Document struct {
	ID         string     `json:"id"`
	Tenant     string     `json:"-"`
	Kind       Kind       `json:"kind"`
	Title      string     `json:"title"`
	OwnerSub   string     `json:"owner_sub"`
	Seq        int64      `json:"seq"`
	SnapshotID *string    `json:"snapshot_id,omitempty"`
	StarredBy  []string   `json:"-"`
	Starred    bool       `json:"starred"` // calculado para el solicitante
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
	// SourceRef es el fichero del que nació el documento cuando vino de fuera
	// («drive:<uuid>»); vacío en los creados aquí. Es lo que hace que abrir el
	// mismo fichero dos veces lleve al mismo documento en lugar de ir dejando
	// copias sueltas, y lo que le dice a los cambios a dónde tienen que volver.
	SourceRef string `json:"source_ref,omitempty"`
}

// Permission es una entrada de la ACL de un documento.
type Permission struct {
	DocumentID    string        `json:"document_id"`
	PrincipalKind PrincipalKind `json:"principal_kind"`
	PrincipalID   string        `json:"principal_id"`
	Role          Role          `json:"role"`
	GrantedBy     string        `json:"granted_by"`
	CreatedAt     time.Time     `json:"created_at"`
}

// ShareLink es un enlace de compartición (token opaco, guardado hasheado).
type ShareLink struct {
	ID         string     `json:"id"`
	DocumentID string     `json:"document_id"`
	Role       Role       `json:"role"`
	CreatedBy  string     `json:"created_by"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	// AdjustedAt: cuándo el techo de la organización acortó la caducidad de este enlace.
	AdjustedAt *time.Time `json:"policy_adjusted_at,omitempty"`
	// HasPassword: el enlace pide contraseña. El hash no sale nunca del almacén.
	HasPassword bool `json:"has_password"`
	// Token solo se rellena al CREAR el enlace (única vez que se ve en claro).
	Token string `json:"token,omitempty"`
}

// Active indica si el enlace es válido en el instante now.
func (s ShareLink) Active(now time.Time) bool {
	if s.RevokedAt != nil {
		return false
	}
	if s.ExpiresAt != nil && !now.Before(*s.ExpiresAt) {
		return false
	}
	return true
}

// Version es una entrada del historial de versiones.
type Version struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"document_id"`
	SnapshotID string    `json:"snapshot_id"`
	Label      *string   `json:"label,omitempty"`
	AuthorSub  string    `json:"author_sub"`
	Seq        int64     `json:"seq"`
	Auto       bool      `json:"auto"`
	CreatedAt  time.Time `json:"created_at"`
}

// Snapshot es un estado CRDT consolidado (inline o en S3).
type Snapshot struct {
	ID         string          `json:"id"`
	DocumentID string          `json:"document_id"`
	Seq        int64           `json:"seq"`
	State      json.RawMessage `json:"state,omitempty"`
	BlobKey    string          `json:"blob_key,omitempty"`
	SizeBytes  int64           `json:"size_bytes"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Comment es un comentario anclado a una referencia CRDT estable.
type Comment struct {
	ID         string  `json:"id"`
	DocumentID string  `json:"document_id"`
	ParentID   *string `json:"parent_id,omitempty"`
	AuthorSub  string  `json:"author_sub"`
	// AuthorName es el nombre del autor en el directorio de Sytue ID, resuelto al
	// servirlo con la sesión de quien lee; no se guarda. Vacío si no se pudo.
	AuthorName string          `json:"author_name,omitempty"`
	Anchor     json.RawMessage `json:"anchor"`
	Body       string          `json:"body"`
	Resolved   bool            `json:"resolved"`
	ResolvedBy *string         `json:"resolved_by,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// Blob describe un export/import materializado en S3.
type Blob struct {
	ID          string     `json:"id"`
	DocumentID  string     `json:"document_id"`
	Format      BlobFormat `json:"format"`
	BlobKey     string     `json:"-"`
	Seq         int64      `json:"seq"`
	SizeBytes   int64      `json:"size_bytes"`
	ContentType string     `json:"content_type"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Errores de dominio (la capa http los mapea a *httpx.Error).
var (
	ErrInvalidKind   = errors.New("docs: tipo de documento inválido")
	ErrInvalidRole   = errors.New("docs: rol inválido")
	ErrInvalidFormat = errors.New("docs: formato de export inválido")
	ErrTitleRequired = errors.New("docs: el título es obligatorio")
	ErrNotFound      = errors.New("docs: documento no encontrado")
	ErrForbidden     = errors.New("docs: permiso insuficiente")
	ErrStaleSeq      = errors.New("docs: secuencia base obsoleta (conflicto de versión)")
	ErrShareExpired  = errors.New("docs: enlace de compartición caducado o revocado")
	// ErrClaveDeEnlace: el enlace pide contraseña y falta la prueba de haberla dado.
	ErrClaveDeEnlace = errors.New("docs: el enlace pide contraseña")
	// ErrClaveDeEnlaceInvalida: la contraseña que se quiere poner no vale (corta o larga).
	ErrClaveDeEnlaceInvalida = errors.New("docs: contraseña de enlace no válida")
	ErrInvalidOp             = errors.New("docs: operación CRDT inválida")
	ErrSnapshotMissing       = errors.New("docs: snapshot no encontrado")
	// ErrConflict: restaurar dejaría dos documentos vivos del mismo fichero.
	ErrConflict = errors.New("docs: ya hay otro documento vivo de ese fichero")
)
