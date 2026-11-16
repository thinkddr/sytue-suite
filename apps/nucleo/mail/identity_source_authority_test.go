// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	historicalPolicyKey        struct{}
	historicalPrincipalKey     struct{}
	authorizedHistoricalCrypto struct {
		*transferMailCrypto
		before       func()
		publications int
	}
)

func (c *authorizedHistoricalCrypto) RequireAuthorizedStoragePublication(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, policy storage.PublicationPolicy) (context.Context, error) {
	if scope != c.binding.Scope || target != c.binding.Target || policy == nil {
		return nil, ErrRecord
	}
	return context.WithValue(ctx, historicalPolicyKey{}, policy), nil
}

func (c *authorizedHistoricalCrypto) checkPolicy(ctx context.Context) error {
	c.publications++
	if c.before != nil {
		c.before()
		c.before = nil
	}
	policy, ok := ctx.Value(historicalPolicyKey{}).(storage.PublicationPolicy)
	if !ok {
		return ErrRecord
	}
	return policy(ctx)
}

func (c *authorizedHistoricalCrypto) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	if err := c.checkPolicy(ctx); err != nil {
		return storage.Reference{}, err
	}
	return c.transferMailCrypto.PutStorageRecordAt(ctx, target, transaction, record)
}

func (c *authorizedHistoricalCrypto) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	if err := c.checkPolicy(ctx); err != nil {
		return storage.ObjectVersion{}, err
	}
	return c.transferMailCrypto.PutStorageObjectAt(ctx, target, transaction, object, revision, commit)
}

func (c *authorizedHistoricalCrypto) UploadStorageFileAt(ctx context.Context, gateway *api.Drive, journal *offline.Store, job string, target storage.PublicationTarget, request storage.FileRequest, input io.ReaderAt) (storage.Reference, error) {
	if err := c.checkPolicy(ctx); err != nil {
		return storage.Reference{}, err
	}
	return c.transferMailCrypto.UploadStorageFileAt(ctx, gateway, journal, job, target, request, input)
}

func TestHistoricalImportKeepsOriginalAuthoritySnapshotAndTargetAtPublication(t *testing.T) {
	for _, fault := range []string{"none", "authority", "source-epoch", "target"} {
		t.Run(fault, func(t *testing.T) {
			f := setupImport(t)
			f.repo.Close()
			f.crypto.identity = storage.LegacyIdentity
			sdk := &authorizedHistoricalCrypto{transferMailCrypto: &transferMailCrypto{cryptoFixture: f.crypto}}
			cfg := f.cfg
			cfg.Crypto = sdk
			cfg.Files = nil
			cfg.WorkLease = mailTransferLease
			previous, err := NewHistoricalSource(cfg, f.source)
			if err != nil {
				t.Fatal(err)
			}
			defer previous.Close()
			retired := false
			denied := errors.New("original source authority retired")
			checks := 0
			if fault != "none" {
				sdk.before = func() {
					switch fault {
					case "authority":
						retired = true
					case "source-epoch":
						f.source.state.Epoch = "018f3340-7b12-7000-8000-000000000009"
					case "target":
						f.crypto.binding.Target.DeviceID = "REPLACEMENT"
					}
				}
			}
			original := context.WithValue(t.Context(), historicalPrincipalKey{}, "original")
			policy := func(ctx context.Context) error {
				checks++
				if ctx.Value(historicalPrincipalKey{}) != "original" {
					return ErrRecord
				}
				if retired {
					return denied
				}
				return ctx.Err()
			}
			routing := storage.IdentityCutoverState{Version: 1, Scope: testScope, Phase: "legacy", SourceRooms: f.crypto.binding.Rooms}
			err = previous.PrepareAuthorized(original, routing, policy)
			if fault == "none" {
				if err != nil || sdk.publications < 3 || checks <= sdk.publications {
					t.Fatal("original authority did not reach every SDK boundary", err, checks, sdk.publications)
				}
				before := sdk.publications
				routing.Phase = "frozen"
				routing.Epoch = testEpoch
				if err = previous.PrepareAuthorized(original, routing, func(context.Context) error { return denied }); err != nil || sdk.publications != before {
					t.Fatal("frozen historical check acquired a writer", err)
				}
			} else {
				if err == nil || f.crypto.puts != 0 {
					t.Fatal("changed source authorization reached accepted storage", err, f.crypto.puts)
				}
				if fault == "authority" && !errors.Is(err, denied) {
					t.Fatal("original denial lost", err)
				}
			}
		})
	}
}

func TestHistoricalImportCannotDowngradeWithoutPublicationBinder(t *testing.T) {
	f := setupImport(t)
	f.repo.Close()
	f.crypto.identity = storage.LegacyIdentity
	cfg := f.cfg
	cfg.Crypto = &transferMailCrypto{cryptoFixture: f.crypto}
	cfg.Files = nil
	cfg.WorkLease = mailTransferLease
	previous, err := NewHistoricalSource(cfg, f.source)
	if err != nil {
		t.Fatal(err)
	}
	defer previous.Close()
	routing := storage.IdentityCutoverState{Version: 1, Scope: testScope, Phase: "legacy", SourceRooms: f.crypto.binding.Rooms}
	if err = previous.PrepareAuthorized(t.Context(), routing, func(context.Context) error { return nil }); !errors.Is(err, ErrRecord) || f.crypto.puts != 0 {
		t.Fatal("missing binder silently used ordinary SDK", err)
	}
}
