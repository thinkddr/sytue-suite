// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type unopenedPrivateCalendarTx struct{ pgx.Tx }

func TestCalendarPrivateStagingRejectsUnverifiedRoutingBeforeIO(t *testing.T) {
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}), authn.Claims{Tenant: "acme", Subject: "alice", Email: "alice@example.invalid"})
	identity := privateCalendarIdentity{scope: storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}, addresses: []string{"alice@example.invalid"}}
	foreign := identity
	foreign.scope.Tenant = "foreign"
	aliases := identity
	aliases.addresses = []string{"another@example.invalid"}
	for _, test := range []struct {
		ctx      context.Context
		identity privateCalendarIdentity
	}{
		{context.Background(), identity},
		{ctx, foreign},
		{ctx, aliases},
	} {
		state, err := stagePrivateStorageSnapshot(test.ctx, unopenedPrivateCalendarTx{}, test.identity, uuid.New())
		if state != (pg.PrivateStorageState{}) || !errors.Is(err, pg.ErrPrivateStorageScope) {
			t.Fatal("unverified route reached source capture", state, err)
		}
	}
	state, err := stagePrivateStorageSnapshot(ctx, unopenedPrivateCalendarTx{}, identity, uuid.Nil)
	if state != (pg.PrivateStorageState{}) || !errors.Is(err, ErrPrivateStorageEpoch) {
		t.Fatal("invalid epoch reached source capture", state, err)
	}
}
