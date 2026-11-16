// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type lifetimeJournalPort struct{ storagePort }

func (p lifetimeJournalPort) Call(ctx context.Context, scope storage.Scope, method Method, raw []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.storagePort.Call(ctx, scope, method, raw)
}

func TestJournalOwnerLifetimeSurvivesFinishedConstructionButNotRevokedSession(t *testing.T) {
	identity := fixtureIdentity()
	calls := 0
	port := storagePort{sourcePort: sourcePort{id: strings.Repeat("a", 64), identity: identity}, handle: func(method Method, _ []byte) ([]byte, error) {
		if method != JournalKeys {
			return nil, ErrRejected
		}
		calls++
		return []byte(`{"keys":[],"next":""}`), nil
	}}
	life, retire := context.WithCancel(t.Context())
	defer retire()
	construction, finish := context.WithCancel(life)
	store, got, err := OpenJournalForLifetime(construction, life, lifetimeJournalPort{port})
	if err != nil || got != identity {
		t.Fatal(err)
	}
	finish()
	owner := identity.Scope.Tenant + "\x00" + identity.Scope.Subject + "\x00mail"
	if count, err := store.CountOwner(owner, "synthetic-private-work"); err != nil || count != 0 || calls != 1 {
		t.Fatal("completed construction cancelled the retained authenticated journal", count, err, calls)
	}
	before := calls
	if err := store.Exclusive(construction, owner, "synthetic-private-work", "cancelled-operation", func() error { t.Fatal("cancelled operation entered journal work"); return nil }); !errors.Is(err, context.Canceled) || calls != before {
		t.Fatal("operation cancellation was revived by retained journal lifetime", err)
	}
	retire()
	if _, err := store.CountOwner(owner, "synthetic-private-work"); !errors.Is(err, context.Canceled) || calls != before {
		t.Fatal("revoked original session accessed retained journal", err)
	}
}
