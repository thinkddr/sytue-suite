// SPDX-License-Identifier: Apache-2.0
// Package domain preserves the Notes service API while shared models and
// normalization run from the same source on authorized devices.
package domain

import (
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/notes/model"
)

type (
	Visibility            = model.Visibility
	Layout                = model.Layout
	CryptoParams          = model.CryptoParams
	Notebook              = model.Notebook
	NotebookInput         = model.NotebookInput
	Kind                  = model.Kind
	Note                  = model.Note
	NoteState             = model.NoteState
	NoteLink              = model.NoteLink
	NoteInput             = model.NoteInput
	Tag                   = model.Tag
	TagState              = model.TagState
	TagInput              = model.TagInput
	TagStateInput         = model.TagStateInput
	LegalHold             = model.LegalHold
	AuditEntry            = model.AuditEntry
	Persona               = model.Persona
	NotaCompartida        = model.NotaCompartida
	EncryptedContentInput = model.EncryptedContentInput
)

const (
	VisPrivate                = model.VisPrivate
	VisPersonal               = model.VisPersonal
	VisOrganization           = model.VisOrganization
	VisPublic                 = model.VisPublic
	LayoutTree                = model.LayoutTree
	LayoutFree                = model.LayoutFree
	LayoutColumns             = model.LayoutColumns
	LayoutList                = model.LayoutList
	MinKDFIterations          = model.MinKDFIterations
	KindText                  = model.KindText
	KindTodo                  = model.KindTodo
	KindImage                 = model.KindImage
	KindFile                  = model.KindFile
	KindLink                  = model.KindLink
	KindColor                 = model.KindColor
	KindLauncher              = model.KindLauncher
	KindCrossref              = model.KindCrossref
	KindDoc                   = model.KindDoc
	KindDrive                 = model.KindDrive
	KindEvent                 = model.KindEvent
	KindRoom                  = model.KindRoom
	TargetCalendarEvent       = model.TargetCalendarEvent
	TargetRoomChannel         = model.TargetRoomChannel
	TargetRoomMessage         = model.TargetRoomMessage
	TargetDocsDocument        = model.TargetDocsDocument
	TargetDriveFile           = model.TargetDriveFile
	TargetNotebook            = model.TargetNotebook
	TargetNote                = model.TargetNote
	MaxClientID               = model.MaxClientID
	AuditNotebookPublish      = model.AuditNotebookPublish
	AuditNotebookUnpublish    = model.AuditNotebookUnpublish
	AuditNotebookDelete       = model.AuditNotebookDelete
	AuditNotePurge            = model.AuditNotePurge
	AuditNotebookExport       = model.AuditNotebookExport
	AuditNotebookImport       = model.AuditNotebookImport
	AuditLegalHoldCreate      = model.AuditLegalHoldCreate
	AuditLegalHoldRelease     = model.AuditLegalHoldRelease
	PermisoLeer               = model.PermisoLeer
	PermisoEditar             = model.PermisoEditar
	EncryptedNoteWriteVersion = model.EncryptedNoteWriteVersion
	MaxEncryptedNoteCipher    = model.MaxEncryptedNoteCipher
	MaxNoteRequestBody        = model.MaxNoteRequestBody
)

var (
	ErrNotFound                   = model.ErrNotFound
	ErrInvalidInput               = model.ErrInvalidInput
	ErrForbidden                  = model.ErrForbidden
	ErrSoloDueno                  = model.ErrSoloDueno
	ErrEncrypted                  = model.ErrEncrypted
	ErrFormatoNoReconocido        = model.ErrFormatoNoReconocido
	ErrVersionNoSoportada         = model.ErrVersionNoSoportada
	ErrBackendWriteUpgrade        = model.ErrBackendWriteUpgrade
	ErrEncryptedClientUpgrade     = model.ErrEncryptedClientUpgrade
	ErrEncryptedMigrationRequired = model.ErrEncryptedMigrationRequired
	ErrNoteChanged                = model.ErrNoteChanged
	ErrNoteEditConflict           = model.ErrNoteEditConflict
	ErrNotebookCryptoChanged      = model.ErrNotebookCryptoChanged
)

func ValidVisibility(v Visibility) bool  { return model.ValidVisibility(v) }
func DefaultIndexable(v Visibility) bool { return model.DefaultIndexable(v) }
func ValidLayout(v Layout) bool          { return model.ValidLayout(v) }
func ValidKind(v Kind) bool              { return model.ValidKind(v) }
func LinkKindFor(v Kind) (string, bool)  { return model.LinkKindFor(v) }
func ValidTargetKind(v string) bool      { return model.ValidTargetKind(v) }
func ValidHexColor(v string) bool        { return model.ValidHexColor(v) }
func ValidWebURL(v string) bool          { return model.ValidWebURL(v) }
func EncryptedPublicData(data json.RawMessage, serverControls bool) bool {
	return model.EncryptedPublicData(data, serverControls)
}
func SanitizeHTML(v string) string { return model.SanitizeHTML(v) }

// TrashedNote es una nota tal como sale en la papelera: con lo que hace falta para
// decidir si restaurarla —de qué cuaderno viene— y cuándo se irá sola.
//
// VA APARTE de `Note` y no como campos con `omitempty` dentro: `Note` es el JSON de
// todas las rutas, y un `deleted_at` o un `purge_at` colados en ellas serían
// contrato nuevo que nadie pidió. Embebida, se serializa en línea: la fila de la
// papelera sigue siendo una nota para quien solo lea los campos de siempre.
type TrashedNote struct {
	Note
	DeletedAt time.Time `json:"deleted_at"`
	// NotebookTitle es el título del cuaderno donde estaba, aunque el cuaderno
	// también esté en la papelera: es justo cuando más falta hace saberlo.
	NotebookTitle string `json:"notebook_title"`
	// PurgeAt es cuándo la destruirá el recolector. Nil = no caduca: la purga está
	// apagada (`TRASH_RETENTION=0`) o hay una retención legal que la cubre. Las dos
	// se contestan IGUAL a propósito: un hold no se anuncia (migración 0006).
	PurgeAt *time.Time `json:"purge_at,omitempty"`
	// Held lo rellena el store y el handler lo convierte en `PurgeAt` nil; no viaja.
	Held bool `json:"-"`
}
