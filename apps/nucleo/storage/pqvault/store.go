// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pqvault

// La bóveda como almacén de registros y catálogo (storage.RecordStore), el mismo
// contrato que cumple el cliente de Matrix: así los repositorios de dominio
// (`storage/records` y los de cada app) pueden ir sobre la bóveda sin cambiar.
// Lo que la web hace en `StorageRepository` sobre `PqVault.storage()`.
//
// Los ficheros (bloques cifrados en /cipher/blobs) los añade storage/vaultfiles;
// sin esa capa, un objeto cuyo contenido es un fichero se rechaza.

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var _ storage.RecordStore = (*Vault)(nil)

// StorageIdentityVersion: la bóveda directa es la identidad aislada, por producto.
func (v *Vault) StorageIdentityVersion() storage.IdentityVersion { return storage.IsolatedIdentity }

// StorageBindings es el enrutado de la bóveda: una sola «sala», la bóveda.
func (v *Vault) StorageBindings(context.Context) (storage.Binding, error) {
	if _, _, err := v.keys(); err != nil {
		return storage.Binding{}, err
	}
	binding := storage.Binding{Scope: v.scope, Target: v.PublicationTarget(), Rooms: []string{v.vaultID}}
	if !binding.Valid() {
		return storage.Binding{}, ErrIdentity
	}
	return binding, nil
}

// PrepareStorage comprueba que la bóveda es la de `expected` y está abierta.
func (v *Vault) PrepareStorage(ctx context.Context, expected storage.Scope) (storage.Binding, error) {
	if !expected.Valid() || expected != v.scope {
		return storage.Binding{}, ErrIdentity
	}
	return v.StorageBindings(ctx)
}

// CheckStoragePublicationTarget no toca la red: el destino tiene que ser esta bóveda.
func (v *Vault) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if _, _, err := v.keys(); err != nil {
		return err
	}
	if !target.Valid() || target != v.PublicationTarget() {
		return ErrIdentity
	}
	return nil
}

// StoragePublicationTarget es el destino actual de las publicaciones nuevas.
func (v *Vault) StoragePublicationTarget() (storage.PublicationTarget, error) {
	target := v.PublicationTarget()
	return target, v.CheckStoragePublicationTarget(target)
}

// PutStorageRecord publica un registro nuevo en el destino actual. Una intención
// ya guardada usa PutStorageRecordAt con su destino.
func (v *Vault) PutStorageRecord(ctx context.Context, transaction string, record storage.Record) (storage.Reference, error) {
	target, err := v.StoragePublicationTarget()
	if err != nil {
		return storage.Reference{}, err
	}
	return v.PutStorageRecordAt(ctx, target, transaction, record)
}

// Closed dice si la bóveda ya se cerró.
func (v *Vault) Closed() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.closed
}

// PutStorageRecordAt publica un registro en el destino fijado por una intención.
func (v *Vault) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	if err := v.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	return v.Put(ctx, transaction, record)
}

// ReadStorageRecord lee un registro concreto y exige su objeto y su papel.
func (v *Vault) ReadStorageRecord(ctx context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	record, err := v.Read(ctx, ref)
	if err != nil {
		return storage.Record{}, err
	}
	if record.ObjectID != object || record.Role != role {
		clear(record.Payload)
		return storage.Record{}, storage.ErrRecord
	}
	return record, nil
}

// StorageObjectHistory devuelve las entradas de catálogo (papel `index`) de una
// página del historial. Una página parcial no prueba que algo se borrara.
func (v *Vault) StorageObjectHistory(ctx context.Context, room, cursor string, limit int) (storage.ObjectPage, error) {
	if room != v.vaultID {
		return storage.ObjectPage{}, ErrIdentity
	}
	page, err := v.History(ctx, cursor, min(limit, 100))
	if err != nil {
		return storage.ObjectPage{}, err
	}
	out := storage.ObjectPage{Next: page.Next}
	for _, entry := range page.Entries {
		if entry.Record.Role != "index" {
			continue
		}
		if err := storage.ValidateCatalogEntry(entry, v.scope); err != nil {
			return storage.ObjectPage{}, err
		}
		out.Entries = append(out.Entries, entry)
	}
	return out, nil
}

// StorageObject autentica una revisión exacta del catálogo.
func (v *Vault) StorageObject(ctx context.Context, ref storage.Reference, object string) (storage.ObjectVersion, error) {
	record, err := v.ReadStorageRecord(ctx, ref, object, "index")
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: record}, v.scope)
}

// PutStorageObjectAt publica una revisión del catálogo tras comprobar sus padres y
// su contenido exacto. Nunca borra una revisión anterior.
func (v *Vault) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	return v.PutStorageObjectWithFiles(ctx, target, transaction, object, revision, commit, nil)
}

// FileContentCheck autentica el manifiesto al que apunta un contenido de fichero.
type FileContentCheck func(ctx context.Context, object string, pointer storage.ContentPointer) error

// PutStorageObjectWithFiles es PutStorageObjectAt para quien tiene capa de
// ficheros (storage/vaultfiles): un contenido de fichero se acepta solo si
// `files` autentica su manifiesto. Con `files` nil, se rechaza.
func (v *Vault) PutStorageObjectWithFiles(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit, files FileContentCheck) (storage.ObjectVersion, error) {
	if !commit.Valid() || !storage.Identifier(object) || !storage.Identifier(revision) || !storage.Identifier(transaction) {
		return storage.ObjectVersion{}, storage.ErrRecord
	}
	// Congelar la entrada antes de ir a la red.
	payload, err := json.Marshal(commit)
	var copied storage.ObjectCommit
	if err != nil || json.Unmarshal(payload, &copied) != nil {
		return storage.ObjectVersion{}, storage.ErrRecord
	}
	commit = copied
	if err := v.CheckStoragePublicationTarget(target); err != nil {
		return storage.ObjectVersion{}, err
	}
	previous := make([]string, len(commit.Parents))
	for i, ref := range commit.Parents {
		parent, err := v.StorageObject(ctx, ref, object)
		if err != nil {
			return storage.ObjectVersion{}, err
		}
		if parent.RevisionID == revision {
			return storage.ObjectVersion{}, storage.ErrRecord
		}
		previous[i] = ref.EventID
	}
	if pointer := commit.Content; pointer != nil {
		if err := v.checkContent(ctx, object, *pointer, files); err != nil {
			return storage.ObjectVersion{}, err
		}
	}
	record := storage.Record{Version: 1, Scope: v.scope, ObjectID: object, RevisionID: revision, Role: "index", Previous: previous, Payload: payload}
	ref, err := v.Put(ctx, transaction, record)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: record}, v.scope)
}

func (v *Vault) checkContent(ctx context.Context, object string, pointer storage.ContentPointer, files FileContentCheck) error {
	switch pointer.Kind {
	case "record":
		content, err := v.ReadStorageRecord(ctx, pointer.Reference, object, "content")
		if err != nil {
			return err
		}
		clear(content.Payload)
		if content.RevisionID != pointer.RevisionID {
			return storage.ErrRecord
		}
		return nil
	case "file":
		if files == nil {
			return storage.ErrRecord
		}
		return files(ctx, object, pointer)
	default:
		return storage.ErrRecord
	}
}
