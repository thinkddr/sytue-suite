// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type usageFixture struct {
	value storage.CipherUsage
	err   error
	calls int
}

func (f *usageFixture) CipherUsage(ctx context.Context, scope storage.Scope) (storage.CipherUsage, error) {
	f.calls++
	if ctx.Err() != nil {
		return storage.CipherUsage{}, ctx.Err()
	}
	return f.value, f.err
}

func TestPrivateQuotaUsesCiphertextCounterAndOriginalPlanWithoutCatalogScan(t *testing.T) {
	f := setupDrafts(t)
	metadata := newRoutingFixture()
	usage := &usageFixture{value: storage.CipherUsage{Version: 1, Scope: testScope, Bytes: 100, SytueBytes: 60, ExternalBytes: 40, PendingDeleteBytes: 60, Blobs: 2}}
	f.repo.cfg.Metadata = metadata
	f.repo.cfg.Usage = usage
	facade, err := api.NewMail("https://closed.invalid", nil, nil).WithPrivateData(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	value, err := facade.Quota(t.Context())
	if err != nil || value.UsedBytes != 60 || value.QuotaBytes != 10000 || value.ExternalUsedBytes != 40 || value.PendingDeleteBytes != 60 || value.CiphertextBytes != 100 || usage.calls != 1 {
		t.Fatal("quota used plaintext Mail bytes or lost external usage", value, err)
	}
	usage.value.Scope.Subject = "foreign"
	if _, err = facade.Quota(t.Context()); !errors.Is(err, ErrRecord) {
		t.Fatal("foreign ciphertext quota accepted", err)
	}
	usage.value.Scope = testScope
	usage.err = errors.New("synthetic gateway unavailable")
	if _, err = facade.Quota(t.Context()); err == nil {
		t.Fatal("unavailable ciphertext usage became zero")
	}
	f.repo.cfg.Usage = nil
	if _, err = facade.Quota(t.Context()); !errors.Is(err, api.ErrPrivateMailOperation) {
		t.Fatal("missing counter fell back to clear Mail REST", err)
	}
}

func TestPrivateFacadeAliasChangesUseScopedEncryptedProjection(t *testing.T) {
	f := setupDrafts(t)
	metadata := newRoutingFixture()
	f.repo.cfg.Metadata = metadata
	facade, err := api.NewMail("https://closed.invalid", nil, nil).WithPrivateData(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	value, err := facade.CreateAlias(t.Context(), "new@example.test")
	if err != nil || metadata.creates != 1 {
		t.Fatal("private facade did not use routing authority", err)
	}
	_, aliases, err := facade.Aliases(t.Context())
	if err != nil || len(aliases) != 2 {
		t.Fatal("new alias missing from encrypted senders", err)
	}
	if err = facade.DeleteAlias(t.Context(), value.ID); err != nil || metadata.deletes != 1 {
		t.Fatal("private alias deletion lost projection", err)
	}
}
