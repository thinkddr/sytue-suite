// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrPrivateStorageReleaseStale: the source is not frozen or private at the
// epoch the person saw. Nothing changed; read the state again.
var ErrPrivateStorageReleaseStale = errors.New("pg: private storage source is not at the released epoch")

// PrivateStorageRelease is what «soltar la fuente» records about who asked:
// the epoch shown to the person and the reauthentication that authorized it.
// The service checks the reauthentication before calling; this only audits it.
type PrivateStorageRelease struct {
	Epoch    uuid.UUID
	AuthTime time.Time
	AMR      []string
}

// ReleasePrivateStorageState turns a frozen or private source back to legacy
// (sytue_private_storage_release, private_storage_release_v1.sql). The caller
// owns a tenant transaction with the owner's GUC set; maintenance and merge are
// refused in SQL. The domain rows are the ones frozen at Epoch: the writer
// guard rejected every change since, so nothing on the server is lost. What was
// written on devices after the freeze is not on the server and stays behind.
func ReleasePrivateStorageState(ctx context.Context, tx pgx.Tx, table, tenant, owner string, release PrivateStorageRelease) (PrivateStorageState, error) {
	name, err := privateStorageTable(ctx, table, tenant, owner)
	if err != nil || tx == nil || release.Epoch == uuid.Nil || release.AuthTime.IsZero() || len(release.AMR) > 16 {
		return PrivateStorageState{}, ErrPrivateStorageScope
	}
	amr := slices.Clone(release.AMR)
	for _, method := range amr {
		if method == "" || len(method) > 32 {
			return PrivateStorageState{}, ErrPrivateStorageScope
		}
	}
	if amr == nil {
		amr = []string{}
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM sytue_private_storage_release($1::regclass,$2,$3,$4,$5,$6)`, name, tenant, owner, release.Epoch, release.AuthTime.UTC(), amr); err != nil {
		var failure *pgconn.PgError
		if errors.As(err, &failure) && failure.Code == "PST03" {
			return PrivateStorageState{}, ErrPrivateStorageReleaseStale
		}
		return PrivateStorageState{}, err
	}
	return ReadPrivateStorageState(ctx, tx, table, tenant, owner)
}
