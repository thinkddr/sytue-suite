// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

// Paso de Matrix a la bóveda directa (ADR 0086, «Drive nativo sobre la bóveda»).
// Copia el catálogo de Drive entero —cada revisión, sus ficheros, sus enlaces y,
// lo último, el recibo de la migración del servidor— con el mismo motor que el
// corte de identidad de Matrix (storage/transfer): idempotente por transacción,
// con cada correspondencia en el diario de la bóveda, cada fichero comprobado
// contra su compromiso autenticado y, antes de dar nada por hecho, todo releído
// del destino y cada fichero descargado entero.
//
// Matrix no se congela (la valla de poder solo la pone el controlador del corte
// de identidad). En su lugar, la «valla» de cada sala es una huella de su
// catálogo autenticado: si alguien escribe en Matrix durante la copia, la huella
// cambia, la copia no se da por buena y hay que repetirla. Matrix nunca se toca:
// sus datos siguen ahí, se termine o no la copia.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	catalogtransfer "git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

const (
	// VaultMigrationNamespace guarda en el diario de la bóveda la copia de Matrix
	// en curso. Es trabajo pendiente (storage.PrivateJournalNamespaces).
	VaultMigrationNamespace        = "drive-vault-migration-v1"
	vaultMigrationNamespace        = VaultMigrationNamespace
	vaultMigrationHistoryNamespace = "drive-vault-migration-history-v1"
)

var (
	// ErrVaultNotEmpty: la bóveda guarda una copia a medias que este diario no
	// puede reanudar (Matrix cambió, o el plan se perdió). Sin recibo, nadie ha
	// escrito en ella más que la copia: vaciarla y empezar no pierde nada.
	ErrVaultNotEmpty = errors.New("drive: the vault holds a partial copy that cannot be resumed")
	// ErrVaultInUse: la bóveda ya tiene su Drive completo (la web ya migró). No se
	// copia nada encima.
	ErrVaultInUse = errors.New("drive: the vault already holds a complete private drive")
)

// MigrateToVault copia en este repositorio (la bóveda, sin abrir) el Drive de
// `source` (el de Matrix, ya abierto). copyFile recifra el contenido de un
// fichero de Matrix en la bóveda (matrix.Client.MigrateStorageFileContentInto).
// Al terminar, la bóveda tiene el recibo de la época del servidor y se abre con
// Initialize como si la hubiera migrado la web. Nunca escribe en Matrix.
func (r *Repository) MigrateToVault(parent context.Context, source *Repository, copyFile CopyPrivateFile) error {
	if source == nil || source == r || source.cfg.Scope != r.cfg.Scope || copyFile == nil {
		return ErrSource
	}
	identity, ok := r.cfg.Crypto.(interface {
		StorageIdentityVersion() storage.IdentityVersion
	})
	if !ok || identity.StorageIdentityVersion() != storage.IsolatedIdentity {
		return ErrSource
	}
	r.mu.Lock()
	ready := r.ready
	r.mu.Unlock()
	if ready {
		return ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	ctx, sourceDone, err := source.start(ctx)
	if err != nil {
		return err
	}
	defer sourceDone()
	return r.cfg.Journal.Exclusive(ctx, r.owner(), migrationNamespace, "initialize", func() error {
		return r.migrateToVault(ctx, source, copyFile)
	})
}

//nolint:gocyclo,gocognit // cada rama es un estado distinto de la copia, en el orden en que se comprueban.
func (r *Repository) migrateToVault(ctx context.Context, source *Repository, copyFile CopyPrivateFile) error {
	if err := source.requireReady(); err != nil {
		return err
	}
	destination, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope)
	if err != nil {
		return err
	}
	if !destination.Valid() || destination.Scope != r.cfg.Scope {
		return ErrSource
	}
	state, err := r.cfg.Source.PrivateStorageState(ctx, r.cfg.Scope)
	if err != nil {
		return err
	}
	if state.Validate(r.cfg.Scope) != nil || state.Mode == "legacy" {
		return ErrSource
	}
	current, err := source.catalogFingerprint(ctx)
	if err != nil {
		return err
	}
	// La marca va antes de leer la bóveda: desde aquí nadie más publica ni vacía,
	// y lo que se lea es lo que hay (ADR 0086, «pasa Matrix a ella»).
	mark, err := r.claimVaultCopy(ctx, state.Epoch)
	if err != nil {
		return err
	}
	ctx = api.WithPQVaultCopy(ctx, mark.id)
	defer r.keepVaultCopy(ctx, mark, state.Epoch)()
	if err := r.refresh(ctx); err != nil {
		return err
	}
	var plan catalogtransfer.Plan
	found, err := r.cfg.Journal.Read(r.owner(), vaultMigrationNamespace, "plan", &plan)
	if err != nil {
		return err
	}
	complete, err := r.completed(ctx, state)
	if err != nil {
		return err
	}
	same := found && plan.Source.Same(current)
	switch {
	case complete && found && plan.Complete:
		// Se cortó entre terminar y archivar: solo queda archivar.
		if err := r.archiveVaultMigration(ctx, false); err != nil {
			return err
		}
		return r.releaseVaultCopy(ctx, mark)
	case complete && !same:
		// La bóveda ya tiene su recibo (la web migró, o la copia llegó al recibo y
		// Matrix cambió después): es la que vale y nunca se vacía. Lo escrito en
		// Matrix después sigue en Matrix.
		if found {
			if err := r.archiveVaultMigration(ctx, true); err != nil {
				return err
			}
		}
		if err := r.releaseVaultCopy(ctx, mark); err != nil {
			return err
		}
		return ErrVaultInUse
	case found && !same:
		// Matrix cambió desde que empezó la copia: el plan ya no vale. Lo que hay
		// sin recibo es esta copia: se puede vaciar (con lo visto) y repetir.
		if !r.catalogEmpty() {
			return r.vaultNotEmpty(mark, true)
		}
		if err := DiscardVaultMigration(r.cfg.Journal, r.cfg.Scope); err != nil {
			return err
		}
	case !found && !r.catalogEmpty():
		// Sin plan: es una copia de Matrix si la bóveda ya tenía la marca de una
		// (el diario de la que la empezó se perdió). Si no, es otra cosa a medias.
		return r.vaultNotEmpty(mark, mark.prior != nil && mark.prior.Kind == api.PQVaultCopyKind)
	}
	policy := identityTransfer{target: r, source: source}
	err = catalogtransfer.Run(ctx, catalogtransfer.Config{
		Scope: r.cfg.Scope, Owner: r.owner(), Namespace: vaultMigrationNamespace, ReceiptKind: migrationKind, Journal: r.cfg.Journal,
		Source: source.cfg.Crypto, Target: r.cfg.Crypto, FrozenBindings: source.catalogFingerprint, Rules: &policy,
		PutObject: r.putObject, PutRecord: r.putRecord, CopyFile: copyFile,
		SourceFileCommitment: source.cfg.Crypto.StorageFileCommitment, TargetFileCommitment: r.cfg.Crypto.StorageFileCommitment,
		VerifyFile: func(ctx context.Context, ref storage.Reference, object, revision string) error {
			_, err := r.cfg.Crypto.DownloadStorageFile(ctx, r.cfg.Gateway, ref, object, revision, discardWriter{})
			return err
		},
		AcknowledgeFile: func(ctx context.Context, job string, ref storage.Reference) error {
			return r.cfg.Crypto.AcknowledgeStorageFile(ctx, r.cfg.Journal, job, ref)
		},
		// La bóveda se abre después, con Initialize y su recibo, como si la hubiera
		// migrado la web: aquí no se marca lista.
		PrepareTarget: r.refresh, MarkReady: func(ctx context.Context) error { return ctx.Err() },
	}, current, destination)
	if errors.Is(err, catalogtransfer.ErrSource) {
		return ErrSource
	}
	if api.IsPQVaultCode(err, api.CodePQVaultCopyInProgress) {
		// La marca se canceló (o la tomó otro tras caducar): la copia se para.
		return errors.Join(ErrVaultCopyInProgress, err)
	}
	if err != nil {
		return err
	}
	if err := r.archiveVaultMigration(ctx, false); err != nil {
		return err
	}
	return r.releaseVaultCopy(ctx, mark)
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func (r *Repository) catalogEmpty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.heads) == 0
}

// catalogFingerprint es la «valla» de una fuente que no se congela: por cada
// sala, una huella de todo su catálogo autenticado (referencias y contenido de
// cada registro). Cualquier escritura nueva la cambia.
func (r *Repository) catalogFingerprint(ctx context.Context) (storage.FrozenBinding, error) {
	snapshot, err := storage.LoadCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return storage.FrozenBinding{}, err
	}
	rooms := map[string][]string{}
	for ref, entry := range snapshot.Entries {
		rooms[ref.RoomID] = append(rooms[ref.RoomID], ref.EventID+"\x00"+entry.Record.ObjectID+"\x00"+entry.Record.RevisionID+"\x00"+entry.Record.Role+"\x00"+digest(entry.Record.Payload))
		clear(entry.Record.Payload)
	}
	for _, version := range snapshot.Heads {
		clear(version.Commit.Metadata)
	}
	frozen := storage.FrozenBinding{Binding: snapshot.Binding}
	for _, room := range snapshot.Binding.Rooms {
		lines := rooms[room]
		slices.Sort(lines)
		frozen.Fences = append(frozen.Fences, storage.Reference{RoomID: room, EventID: "$snapshot_" + digest([]byte(room+"\x00"+strings.Join(lines, "\n")))})
	}
	if !frozen.Valid() {
		return storage.FrozenBinding{}, ErrSource
	}
	return frozen, nil
}

// archiveVaultMigration guarda el mapa de la copia aparte (cifrado, sin contar
// como trabajo pendiente) y retira el activo. Solo con el recibo ya autenticado
// en la bóveda; `superseded` archiva también un plan sin terminar cuyo recibo ya
// está publicado.
func (r *Repository) archiveVaultMigration(ctx context.Context, superseded bool) error {
	var plan catalogtransfer.Plan
	found, err := r.cfg.Journal.Read(r.owner(), vaultMigrationNamespace, "plan", &plan)
	if err != nil || !found {
		return err
	}
	if !plan.Complete && !superseded {
		return ErrSource
	}
	ids, err := r.cfg.Journal.RecordIDs(r.owner(), vaultMigrationNamespace)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id == "plan" || id == vaultCopyRecord {
			continue
		}
		if err := r.archiveRecord(ctx, id); err != nil {
			return err
		}
	}
	return r.archiveRecord(ctx, "plan")
}

func (r *Repository) archiveRecord(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var raw json.RawMessage
	found, err := r.cfg.Journal.Read(r.owner(), vaultMigrationNamespace, id, &raw)
	defer clear(raw)
	if err != nil || !found {
		return err
	}
	var prior json.RawMessage
	archived, err := r.cfg.Journal.Read(r.owner(), vaultMigrationHistoryNamespace, id, &prior)
	defer clear(prior)
	if err != nil {
		return err
	}
	if archived && !bytes.Equal(raw, prior) {
		return ErrSource
	}
	if !archived {
		if err := r.cfg.Journal.Write(r.owner(), vaultMigrationHistoryNamespace, id, raw); err != nil {
			return err
		}
	}
	return r.cfg.Journal.Delete(r.owner(), vaultMigrationNamespace, id)
}

// VaultMigrationPending dice si el diario de la bóveda guarda una copia de Matrix
// sin terminar. Mientras la haya, la cuenta sigue en Matrix (que conserva todos
// sus datos) y la copia se reanuda o se repite desde allí.
func VaultMigrationPending(journal *offline.Store, scope storage.Scope) (bool, error) {
	if !journal.Encrypted() || !scope.Valid() || scope.Product != "drive" {
		return false, ErrSource
	}
	ids, err := journal.RecordIDs(journalOwner(scope), vaultMigrationNamespace)
	return len(ids) > 0, err
}

// DiscardVaultMigration borra del diario de la bóveda una copia de Matrix que ya
// no se puede reanudar: su plan y sus correspondencias, y las subidas sin
// confirmar que dejó. Solo cuando la bóveda no tiene recibo (nadie la ha usado)
// y se va a vaciar o ya está vacía: Matrix conserva todos los datos. La marca de
// copia se queda: la copia sigue, ahora desde el principio.
func DiscardVaultMigration(journal *offline.Store, scope storage.Scope) error {
	if !journal.Encrypted() || !scope.Valid() || scope.Product != "drive" {
		return ErrSource
	}
	owner := journalOwner(scope)
	for _, namespace := range []string{vaultMigrationNamespace, storage.PendingUploadsNamespace} {
		ids, err := journal.RecordIDs(owner, namespace)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if namespace == vaultMigrationNamespace && id == vaultCopyRecord {
				continue
			}
			if err := journal.Delete(owner, namespace, id); err != nil {
				return err
			}
		}
	}
	return nil
}
