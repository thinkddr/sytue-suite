// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// StorageFileRecords es el almacén de registros sobre el que va la capa de
// ficheros. El cliente de Matrix es uno; la bóveda post-cuántica directa (ADR
// 0086, storage/pqvault) es otro. La capa de ficheros —bloques cifrados en
// /cipher/blobs, páginas y manifiesto como registros, diario de subida
// reanudable, lectura de vuelta y confirmación— no depende de Matrix: solo de
// poder publicar un registro en un destino fijo y leerlo autenticado.
type StorageFileRecords interface {
	StoragePublicationTarget() (storage.PublicationTarget, error)
	CheckStoragePublicationTarget(storage.PublicationTarget) error
	PutStorageRecord(context.Context, string, storage.Record) (storage.Reference, error)
	PutStorageRecordAt(context.Context, storage.PublicationTarget, string, storage.Record) (storage.Reference, error)
	ReadStorageRecord(context.Context, storage.Reference, string, string) (storage.Record, error)
}

// StorageFiles es la capa de ficheros del cliente de Matrix sobre otro almacén de
// registros, con el mismo formato de manifiesto que la web (filesdk). Es el mismo
// código que usan los métodos de Client: no hay un segundo cifrado de ficheros.
type StorageFiles struct {
	scope   storage.Scope
	records StorageFileRecords
	closed  func() bool
}

// NewStorageFiles ata la capa de ficheros a un ámbito y un almacén. `closed` dice
// si el almacén ya se cerró (puede ser nil).
func NewStorageFiles(scope storage.Scope, records StorageFileRecords, closed func() bool) (StorageFiles, error) {
	if !scope.Valid() || records == nil {
		return StorageFiles{}, storage.ErrRecord
	}
	return StorageFiles{scope: scope, records: records, closed: closed}, nil
}

func (s StorageFiles) io(gateway *api.Drive, journal *offline.Store, records storageRecordIO) storageFileIO {
	if records == nil {
		records = s.records
	}
	return storageFileIO{scope: s.scope, gateway: gateway, records: records, journal: journal, closed: s.closed}
}

// UploadStorageFileAt es Client.UploadStorageFileAt: la primera publicación ya va
// fijada al destino de la intención de la app.
func (s StorageFiles) UploadStorageFileAt(ctx context.Context, gateway *api.Drive, journal *offline.Store, job string, target storage.PublicationTarget, request StorageFileRequest, source io.ReaderAt) (EventRef, error) {
	if err := s.records.CheckStoragePublicationTarget(target); err != nil {
		return EventRef{}, err
	}
	records := storageFileTarget{storageRecordIO: s.records, storagePublicationIO: s.records, target: target}
	return s.io(gateway, journal, records).upload(ctx, job, request, source)
}

// DownloadStorageFile es Client.DownloadStorageFile.
func (s StorageFiles) DownloadStorageFile(ctx context.Context, gateway *api.Drive, ref EventRef, object, revision string, out io.Writer) (StorageFileRequest, error) {
	return s.io(gateway, nil, nil).download(ctx, ref, object, revision, out)
}

// StorageFileInfo es Client.StorageFileInfo.
func (s StorageFiles) StorageFileInfo(ctx context.Context, ref EventRef, object, revision string) (StorageFileRequest, error) {
	request, _, err := s.StorageFileCommitment(ctx, ref, object, revision)
	return request, err
}

// StorageFileCommitment es Client.StorageFileCommitment.
func (s StorageFiles) StorageFileCommitment(ctx context.Context, ref EventRef, object, revision string) (StorageFileRequest, string, error) {
	file, err := s.io(nil, nil, nil).root(ctx, ref, object, revision)
	if err != nil {
		return StorageFileRequest{}, "", err
	}
	return file.Request, file.SHA256, nil
}

// AcknowledgeStorageFile es Client.AcknowledgeStorageFile.
func (s StorageFiles) AcknowledgeStorageFile(ctx context.Context, journal *offline.Store, job string, catalog EventRef) error {
	return s.io(nil, journal, nil).acknowledge(ctx, job, catalog)
}

// ReadStorageFileRange es Client.ReadStorageFileRange.
func (s StorageFiles) ReadStorageFileRange(ctx context.Context, gateway *api.Drive, ref EventRef, object, revision string, offset, length int64) ([]byte, error) {
	return s.io(gateway, nil, nil).readRange(ctx, ref, object, revision, offset, length)
}
