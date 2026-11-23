// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// MigrateStorageFileContentInto copia el contenido de un fichero de este cliente
// a otro almacén con la misma capa de ficheros: la bóveda directa (ADR 0086,
// paso de Drive de Matrix a la bóveda). Es MigrateStorageFileContentAt con otro
// destino: el mismo lector por bloques autenticados (sin fichero temporal en
// claro), el mismo subidor con su diario reanudable y la misma comprobación del
// resumen del original. No borra el original ni confirma la subida.
func (c *Client) MigrateStorageFileContentInto(ctx context.Context, destination StorageFiles, gateway *api.Drive, journal *offline.Store, job string, publication storage.PublicationTarget, ref EventRef, object, revision string) (EventRef, error) {
	if c == nil {
		return EventRef{}, ErrIdentity
	}
	c.stateMu.RLock()
	scope := c.storageScope()
	c.stateMu.RUnlock()
	if _, err := c.PrepareStorage(ctx, scope); err != nil {
		return EventRef{}, err
	}
	source := StorageFiles{scope: scope, records: c, closed: c.closed.Load}
	return CopyStorageFileContent(ctx, source, gateway, destination, gateway, journal, job, publication, ref, object, revision)
}

// CopyStorageFileContent recifra un fichero de `source` en `destination` (los dos
// del mismo ámbito), publicado en `publication`. Cada lado lee sus bloques por su
// pasarela. El subidor comprueba el resumen completo del original antes de
// publicar el manifiesto nuevo.
func CopyStorageFileContent(ctx context.Context, source StorageFiles, sourceGateway *api.Drive, destination StorageFiles, gateway *api.Drive, journal *offline.Store, job string, publication storage.PublicationTarget, ref EventRef, object, revision string) (EventRef, error) {
	if !source.scope.Valid() || source.scope != destination.scope || source.records == nil || destination.records == nil {
		return EventRef{}, ErrIdentity
	}
	if err := destination.records.CheckStoragePublicationTarget(publication); err != nil {
		return EventRef{}, err
	}
	closed := func() bool {
		return source.closed != nil && source.closed() || destination.closed != nil && destination.closed()
	}
	from := storageFileIO{scope: source.scope, gateway: sourceGateway, records: source.records, closed: source.closed}
	records := storageFileTarget{storageRecordIO: destination.records, storagePublicationIO: destination.records, target: publication}
	to := storageFileIO{scope: destination.scope, gateway: gateway, records: records, journal: journal, closed: closed}
	return to.migrateContent(ctx, from, job, ref, object, revision)
}
