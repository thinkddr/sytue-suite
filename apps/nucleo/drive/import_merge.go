// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

// Copias repetidas de una importación (ADR 0086).
//
// Dos importaciones de la misma fuente congelada a la vez —la web en dos
// pestañas, la web y este dispositivo, dos dispositivos con su diario cada uno—
// dejan dos copias de las mismas filas, o dos recibos. Cada copia está bien (es
// la fila congelada, autenticada por la persona), pero un objeto con dos cabezas
// era un conflicto y Drive no volvía a abrir.
//
// Aquí se juntan sin borrar nada: si TODAS las cabezas de un objeto son copias
// idénticas de la importación de la época (resumen semilla de esa época, la misma
// fila y el mismo fichero) o recibos idénticos de ella, se publica una revisión
// que las tiene a todas por padres. Lo que no sea exactamente eso —una edición,
// otra época, otro fichero— no se toca: sigue siendo un conflicto de verdad. Es lo
// mismo que hace la web (`mergeImportCopies` en private-drive-migration.ts).

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// mergeImportCopies junta las copias idénticas de la importación de `epoch`.
// Necesita una observación en ctx; cada publicación recarga el catálogo.
func (r *Repository) mergeImportCopies(ctx context.Context, binding storage.Binding, epoch string) error {
	if !storage.MigrationEpoch(epoch) {
		return nil
	}
	receipt := "drive_migration_" + digest([]byte(epoch))
	for _, object := range r.duplicatedObjects() {
		if err := r.mergeObject(ctx, binding.Target, epoch, object, object == receipt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) duplicatedObjects() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for object, versions := range r.heads {
		if len(versions) > 1 && len(versions) <= 16 {
			out = append(out, object)
		}
	}
	slices.Sort(out)
	return out
}

func (r *Repository) mergeObject(parent context.Context, target storage.PublicationTarget, epoch, object string, receipt bool) error {
	ctx, release, err := r.temporaryObservation(parent)
	if err != nil {
		return err
	}
	defer release()
	versions := r.versions(object)
	if len(versions) < 2 || len(versions) > 16 {
		return nil
	}
	// En un orden fijo: dos clientes que juntan las mismas cabezas publican el
	// mismo registro con la misma transacción, y el servidor se queda con uno.
	slices.SortFunc(versions, func(a, b storage.ObjectVersion) int {
		return strings.Compare(a.Reference.EventID, b.Reference.EventID)
	})
	var same bool
	if receipt {
		same = sameReceipts(versions)
	} else {
		same, err = r.sameImportCopies(ctx, epoch, versions)
		if err != nil {
			return err
		}
	}
	if !same {
		return nil
	}
	return r.publishMerge(ctx, target, object, versions)
}

func sameReceipts(versions []storage.ObjectVersion) bool {
	var first completion
	for i, version := range versions {
		var receipt completion
		if json.Unmarshal(version.Commit.Metadata, &receipt) != nil || receipt.Kind != migrationKind || version.Commit.State != "live" {
			return false
		}
		if i == 0 {
			first = receipt
		} else if receipt != first {
			return false
		}
	}
	return true
}

// sameImportCopies dice si todas las cabezas son la misma fila importada en
// `epoch`, con el mismo fichero. El de la primera se lee entero: es lo que se va
// a enseñar.
func (r *Repository) sameImportCopies(ctx context.Context, epoch string, versions []storage.ObjectVersion) (bool, error) {
	var firstRaw []byte
	defer func() { clear(firstRaw) }()
	firstBody := ""
	for i, version := range versions {
		var item summary
		if json.Unmarshal(version.Commit.Metadata, &item) != nil || item.Kind != itemKind {
			return false, nil
		}
		// Solo una importación deja `seed` con la línea base en la propia fila:
		// cualquier cambio posterior (renombrar, mover, otra versión) lo quita.
		if !item.Seed || item.Epoch != epoch || item.BaselineSHA256 != item.RawSHA256 || version.Commit.State != versions[0].Commit.State {
			return false, nil
		}
		prior, data, raw, err := r.readEntry(ctx, version)
		if err != nil {
			return false, err
		}
		body := ""
		if binaryKind(prior.SourceKind) {
			if data.Binary == nil || data.Binary.Origin != (bodyOrigin{}) {
				clear(raw)
				return false, nil
			}
			description, _, err := r.bodyVersion(ctx, data.observation, *data.Binary, prior, "binary")
			if err != nil {
				clear(raw)
				return false, err
			}
			body = description.SHA256 + "\x00" + strconv.FormatInt(description.SizeBytes, 10)
			if i == 0 {
				if err := r.readBody(ctx, data.observation, *data.Binary, prior, "binary", io.Discard); err != nil {
					clear(raw)
					return false, err
				}
			}
		}
		if i == 0 {
			firstRaw, firstBody = raw, body
			continue
		}
		equal := bytes.Equal(raw, firstRaw) && body == firstBody
		clear(raw)
		if !equal {
			return false, nil
		}
	}
	return true, nil
}

// publishMerge publica una revisión con el contenido de la primera cabeza y todas
// por padres. Sin diario: las transacciones salen de lo que se junta, y repetirla
// tras un corte devuelve lo ya aceptado.
func (r *Repository) publishMerge(ctx context.Context, target storage.PublicationTarget, object string, versions []storage.ObjectVersion) error {
	chosen := versions[0]
	record, err := r.readContent(ctx, chosen)
	if err != nil {
		return err
	}
	defer clear(record.Payload)
	parents := make([]storage.Reference, len(versions))
	key := object
	for i, version := range versions {
		parents[i] = version.Reference
		key += "\x00" + version.Reference.RoomID + "\x00" + version.Reference.EventID
	}
	revision := "merge_native_" + digest([]byte(key))
	ref, err := r.putRecord(ctx, target, revision+"_content", storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: object, RevisionID: revision, Role: "content", Payload: record.Payload})
	if err != nil {
		return err
	}
	_, err = r.putObject(ctx, target, revision+"_index", object, revision+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: chosen.Commit.State, Metadata: chosen.Commit.Metadata, Content: &storage.ContentPointer{Kind: "record", RevisionID: revision, Reference: ref}, Parents: parents})
	return err
}
