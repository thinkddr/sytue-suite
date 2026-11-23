// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package vaultfiles da a la bóveda post-cuántica directa (storage/pqvault) la
// capa de ficheros del cliente nativo: bloques cifrados en /cipher/blobs y su
// manifiesto como registros de la bóveda, con el mismo diario de subida
// reanudable, la lectura de vuelta y la confirmación que ya usa Matrix
// (matrix.StorageFiles). Con ella, una bóveda es un records.Crypto y un
// drive.Crypto completos.
package vaultfiles

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/filesdk"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqvault"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// Store es una bóveda abierta con su capa de ficheros. Los registros, el catálogo
// y la identidad son los de la bóveda; los ficheros, los de matrix.StorageFiles
// sobre esa misma bóveda.
type Store struct {
	*pqvault.Vault
	matrix.StorageFiles
}

var (
	_ records.Crypto = (*Store)(nil)
	_ drive.Crypto   = (*Store)(nil)
	_ filesdk.SDK    = (*Store)(nil)
)

// New ata la capa de ficheros a la bóveda. No toma posesión de ella: quien la
// abrió la cierra.
func New(vault *pqvault.Vault) (*Store, error) {
	if vault == nil {
		return nil, pqvault.ErrIdentity
	}
	files, err := matrix.NewStorageFiles(vault.Scope(), vault, vault.Closed)
	if err != nil {
		return nil, err
	}
	return &Store{Vault: vault, StorageFiles: files}, nil
}

// PutStorageObjectAt publica una revisión del catálogo; un contenido de fichero
// tiene que tener un manifiesto autenticado de ese objeto y esa revisión.
func (s *Store) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	return s.PutStorageObjectWithFiles(ctx, target, transaction, object, revision, commit, func(ctx context.Context, object string, pointer storage.ContentPointer) error {
		_, _, err := s.StorageFileCommitment(ctx, pointer.Reference, object, pointer.RevisionID)
		return err
	})
}
