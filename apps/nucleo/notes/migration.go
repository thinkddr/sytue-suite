// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// ErrAttachmentPending blocks activation until attachment bytes are private too.
var ErrAttachmentPending = errors.New("notes: attachment content must be privately imported before activation")

type (
	SnapshotState = records.SnapshotState
	SnapshotRow   = records.SnapshotRow
	SnapshotPage  = records.SnapshotPage
	Source        = records.Source
)

func (r *Repository) validatePersonal(kind, id string, raw []byte) error {
	var value struct {
		ID          json.RawMessage `json:"id"`
		ContentKind string          `json:"kind"`
		Owner       string          `json:"owner_sub"`
		Visibility  string          `json:"visibility"`
		PublicToken string          `json:"public_token"`
		RoomID      string          `json:"room_id"`
		TargetKind  string          `json:"target_kind"`
		Links       []enlaceFuente  `json:"links"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return ErrSource
	}
	if r.adjuntoPendiente(kind, value.ContentKind, value.TargetKind, value.Links, raw) {
		return ErrAttachmentPending
	}
	if !sourceIdentityMatches(kind, id, value.ID) || ((kind == "notebook" || kind == "note") && value.Owner == "") {
		return ErrSource
	}
	if value.Owner != "" && value.Owner != r.cfg.Scope.Subject {
		return ErrShared
	}
	if (kind == "notebook" || kind == "note") && (value.Visibility == "organization" || value.Visibility == "public" || value.PublicToken != "" || value.RoomID != "") {
		return ErrShared
	}
	return nil
}

// enlaceFuente es un enlace de la fila de origen: solo importa a qué apunta.
type enlaceFuente struct {
	TargetKind string `json:"target_kind"`
}

// adjuntoPendiente dice si la fila cita un adjunto que aún no se ha importado en privado
// (ErrAttachmentPending): su destino, el de alguno de sus enlaces o, en una nota, su
// contenido. En Sytue Office, una nota de imagen guardada tal cual no lo está.
func (r *Repository) adjuntoPendiente(kind, contentKind, targetKind string, links []enlaceFuente, raw []byte) bool {
	if unresolvedAttachmentLink(targetKind) {
		return true
	}
	for _, link := range links {
		if unresolvedAttachmentLink(link.TargetKind) {
			return true
		}
	}
	return kind == "note" && unresolvedAttachment(contentKind) && (!r.local || !imagenDeEsteEquipo(contentKind, raw))
}

// imagenDeEsteEquipo dice si la fila es una nota de imagen que la edición local guarda tal
// cual (Sytue Office): no tiene nada que importar aparte. La que lleva la imagen dentro
// (un adjunto en `data.attachments`, como URL de datos, sin `data.url`: N-06); la de una
// dirección web (`data.url`, http o https, que ya exige el modelo), cuya imagen Sytue
// Office no baja sola: la carga a petición y la guarda dentro también, con esa dirección
// por origen (la nota de imagen por URL); y la purgada, cuya lápida tiene `data` a null.
// En la conectada (New), toda nota de imagen espera a que sus bytes se importen en
// privado (ErrAttachmentPending): eso no cambia.
func imagenDeEsteEquipo(kind string, raw []byte) bool {
	if kind != "image" {
		return false
	}
	var fila struct {
		Data *struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &fila) != nil {
		return false
	}
	return fila.Data == nil || fila.Data.URL == "" || strings.HasPrefix(fila.Data.URL, "https://") || strings.HasPrefix(fila.Data.URL, "http://")
}

func unresolvedAttachment(kind string) bool {
	switch kind {
	case "image", "file", "drive", "doc":
		return true
	default:
		return false
	}
}

func sourceIdentityMatches(kind, id string, raw json.RawMessage) bool {
	if len(raw) == 0 {
		return kind != "notebook" && kind != "note"
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text == id
	}
	if kind != "audit" {
		return false
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil || number.String() != id {
		return false
	}
	_, err := strconv.ParseInt(id, 10, 64)
	return err == nil
}

func unresolvedAttachmentLink(kind string) bool {
	return kind == "drive_file" || kind == "docs_document"
}
