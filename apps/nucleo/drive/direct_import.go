// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

// Importación de la bóveda directa (ADR 0086, «Drive nativo sobre la bóveda»).
// Es la misma que hace la web (`migrateDriveSource`, private-drive-migration.ts)
// y el mismo código que la de Matrix: congelar la fuente del servidor con una
// época propia, copiar cada fila con su intención en el diario y, al final,
// publicar el recibo `drive_migration_<sha>`. Lo que cambia es quién puede
// reanudar una época ya congelada:
//
//   - la fuente en `legacy`: este dispositivo la congela y la importa;
//   - la fuente congelada sin recibo en la bóveda: solo si la congeló este
//     dispositivo (su intención `begin_` con esa época sigue en el diario) o si
//     la persona eligió adoptarla (AdoptRetainedSource). Si no, ErrNotReady: la
//     importa quien la empezó (la web u otro dispositivo), o la época es la de
//     Matrix y sus datos están allí.
//
// Así una copia a medias de otra cosa (la de Matrix a la bóveda) nunca se mezcla
// con la fuente del servidor, y la importación es idempotente y reanudable: cada
// fila lleva su intención con transacciones fijas y una fila ya publicada se
// confirma leyéndola, no se vuelve a escribir.

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const adoptPrefix = "adopt_"

type adoptWork struct {
	Scope storage.Scope `json:"scope"`
	Epoch string        `json:"epoch"`
}

func (r *Repository) initializeDirect(ctx context.Context, binding storage.Binding, state storage.LegacyMigrationState) error {
	var err error
	if state.Mode == "legacy" {
		// beginMigration deja la intención en el diario ANTES de congelar: si se
		// corta aquí, la siguiente apertura reanuda la misma época.
		state, err = r.beginMigration(ctx, binding)
		if err != nil {
			return err
		}
	}
	found, err := r.completed(ctx, state)
	if err != nil {
		return err
	}
	if found {
		return r.finishInitialization(ctx, state)
	}
	owned, err := r.ownsRetainedSource(binding, state.Epoch)
	if err != nil {
		return err
	}
	if !owned {
		return ErrNotReady
	}
	if state.Mode != "frozen" {
		return ErrSource
	}
	receipt, err := r.importSnapshot(ctx, binding, state)
	if err != nil {
		return err
	}
	if err := r.publishCompletion(ctx, binding, receipt); err != nil {
		return err
	}
	return r.finishInitialization(ctx, state)
}

// ownsRetainedSource dice si este dispositivo puede seguir importando `epoch`.
func (r *Repository) ownsRetainedSource(binding storage.Binding, epoch string) (bool, error) {
	var begin beginWork
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, "begin_"+digest([]byte(binding.Target.DeviceID)), &begin)
	if err != nil {
		return false, err
	}
	if found && begin.Scope == r.cfg.Scope && begin.Epoch == epoch && begin.Target == binding.Target {
		return true, nil
	}
	var adopt adoptWork
	found, err = r.cfg.Journal.Read(r.owner(), migrationNamespace, adoptPrefix+digest([]byte(epoch)), &adopt)
	if err != nil {
		return false, err
	}
	return found && adopt.Scope == r.cfg.Scope && adopt.Epoch == epoch, nil
}

func journalOwner(scope storage.Scope) string {
	return scope.Tenant + "\x00" + scope.Subject + "\x00" + scope.Product
}

// AdoptRetainedSource deja en el diario de la bóveda que la persona eligió
// volver a la copia que el servidor guardó al congelar `epoch` (la salida de quien
// no puede abrir Matrix: «Vaciar y volver a los datos del servidor»). La
// siguiente apertura de la bóveda con DirectImport la importa. No borra nada: lo
// escrito en Matrix después de congelar sigue allí, aunque ya no se lea.
func AdoptRetainedSource(journal *offline.Store, scope storage.Scope, epoch string) error {
	if !journal.Encrypted() || !scope.Valid() || scope.Product != "drive" || !storage.MigrationEpoch(epoch) {
		return ErrSource
	}
	return journal.Write(journalOwner(scope), migrationNamespace, adoptPrefix+digest([]byte(epoch)), adoptWork{Scope: scope, Epoch: epoch})
}

// PendingDirectImport dice si el diario guarda una importación de la bóveda a
// medias (una intención `begin_` o una adopción). No lee nada del servidor.
func PendingDirectImport(journal *offline.Store, scope storage.Scope) (bool, error) {
	if !journal.Encrypted() || !scope.Valid() || scope.Product != "drive" {
		return false, ErrSource
	}
	ids, err := journal.RecordIDs(journalOwner(scope), migrationNamespace)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if strings.HasPrefix(id, "begin_") || strings.HasPrefix(id, adoptPrefix) {
			return true, nil
		}
	}
	return false, nil
}
