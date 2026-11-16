// SPDX-License-Identifier: Apache-2.0
//go:build integration

package store_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func TestPrivateMailRoutingAliasesPreserveFrozenOriginalsAndAllContentFences(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("routing-private")
	account := seedAccount(ctx, t, st, "routing@sytue.eu")
	other := seedAccount(ctx, t, st, "other-routing@sytue.eu")
	previous, err := st.CreateAlias(ctx, account.ID, "original-routing@sytue.eu")
	if err != nil {
		t.Fatal(err)
	}
	epoch := uuid.New()
	state, err := st.BeginPrivateStorageMigration(ctx, account.Subject, epoch, func(context.Context, string) (io.ReadCloser, error) {
		return nil, errors.New("metadata-only capture opened a body")
	})
	if err != nil {
		t.Fatal(err)
	}
	original, err := st.PrivateStorageSourceBody(ctx, account.Subject, epoch, "alias", previous.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateAlias(ctx, account.ID, "new-routing@sytue.eu")
	if err != nil {
		t.Fatal("frozen mailbox could not add a public delivery alias", err)
	}
	if err = st.DeleteAlias(ctx, other.ID, created.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("routing mutation escaped original account ownership", err)
	}
	if err = st.DeleteAlias(ctx, account.ID, previous.ID); err != nil {
		t.Fatal("frozen mailbox could not remove its public delivery alias", err)
	}
	current, err := st.ListAliases(ctx, account.ID)
	if err != nil || len(current) != 1 || current[0].ID != created.ID {
		t.Fatal("live delivery aliases did not reflect the original CRUD", err)
	}
	retained, err := st.PrivateStorageSourceBody(ctx, account.Subject, epoch, "alias", previous.ID.String())
	if err != nil || retained.Descriptor != original.Descriptor || !bytes.Equal(retained.Payload, original.Payload) {
		t.Fatal("delivery mutation rewrote a retained original", err)
	}
	if _, err = st.PrivateStorageSourceBody(ctx, account.Subject, epoch, "alias", created.ID.String()); !errors.Is(err, store.ErrPrivateStorageEpoch) && !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("new public alias silently entered the old private snapshot", err)
	}
	after, err := st.PrivateStorageState(ctx, account.Subject)
	if err != nil || !reflect.DeepEqual(after, state) {
		t.Fatal("routing metadata replaced the exact retained source identity", err)
	}
	if err = store.PrivateStorageError(st.SetSignature(ctx, account.ID, "PRIVATE-CONTENT-MUST-STAY-CLIENT")); !errors.Is(err, store.ErrPrivateStorageFrozen) {
		t.Fatal("routing exception reopened clear private content", err)
	}
	err = h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE aliases SET account_id=$1 WHERE id=$2 AND tenant=$3", other.ID, created.ID, "routing-private")
		return err
	})
	if !errors.Is(store.PrivateStorageError(err), store.ErrPrivateStorageFrozen) {
		t.Fatal("routing exception moved a frozen identity to another account", err)
	}
}
