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
	historicalDestinationIntentKey struct{}
	projectingHistoricalCrypto     struct {
		*authorizedHistoricalCrypto
		projections int
		deny        error
		empty       bool
		cancel      context.CancelFunc
	}
)

//nolint:nilnil // test double: empty deliberately returns (nil, nil) to prove callers reject a nil context
func (c *projectingHistoricalCrypto) HistoricalStoragePublicationContext(ctx context.Context) (context.Context, error) {
	c.projections++
	if ctx.Value(historicalPrincipalKey{}) != "original" || ctx.Value(historicalDestinationIntentKey{}) != true {
		return nil, ErrRecord
	}
	policy, ok := ctx.Value(historicalPolicyKey{}).(storage.PublicationPolicy)
	if !ok {
		return nil, ErrRecord
	}
	if err := policy(ctx); err != nil {
		return nil, err
	}
	if c.deny != nil {
		return nil, c.deny
	}
	if c.empty {
		return nil, nil
	}
	if c.cancel != nil {
		c.cancel()
	}
	return context.WithValue(ctx, historicalDestinationIntentKey{}, false), nil
}

func (c *projectingHistoricalCrypto) projected(ctx context.Context) error {
	if ctx.Value(historicalDestinationIntentKey{}) != false {
		return ErrRecord
	}
	return ctx.Err()
}

func (c *projectingHistoricalCrypto) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	if err := c.projected(ctx); err != nil {
		return storage.Reference{}, err
	}
	return c.authorizedHistoricalCrypto.PutStorageRecordAt(ctx, target, transaction, record)
}

func (c *projectingHistoricalCrypto) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	if err := c.projected(ctx); err != nil {
		return storage.ObjectVersion{}, err
	}
	return c.authorizedHistoricalCrypto.PutStorageObjectAt(ctx, target, transaction, object, revision, commit)
}

func (c *projectingHistoricalCrypto) UploadStorageFileAt(ctx context.Context, gateway *api.Drive, journal *offline.Store, job string, target storage.PublicationTarget, request storage.FileRequest, input io.ReaderAt) (storage.Reference, error) {
	if err := c.projected(ctx); err != nil {
		return storage.Reference{}, err
	}
	return c.authorizedHistoricalCrypto.UploadStorageFileAt(ctx, gateway, journal, job, target, request, input)
}

func TestHistoricalContextProjectionRequiresItsOwnWriterPolicyAndKeepsOriginalLifetime(t *testing.T) {
	for _, fault := range []string{"none", "projection-denied", "nil-context", "cancelled", "authority-denied"} {
		t.Run(fault, func(t *testing.T) {
			f := setupImport(t)
			f.repo.Close()
			f.crypto.identity = storage.LegacyIdentity
			sdk := &projectingHistoricalCrypto{authorizedHistoricalCrypto: &authorizedHistoricalCrypto{transferMailCrypto: &transferMailCrypto{cryptoFixture: f.crypto}}}
			cfg := f.cfg
			cfg.Crypto = sdk
			cfg.Files = nil
			cfg.WorkLease = mailTransferLease
			previous, err := NewHistoricalSource(cfg, f.source)
			if err != nil {
				t.Fatal(err)
			}
			defer previous.Close()
			ctx, cancel := context.WithCancel(context.WithValue(context.WithValue(t.Context(), historicalPrincipalKey{}, "original"), historicalDestinationIntentKey{}, true))
			defer cancel()
			denied := errors.New("explicit original authority denial")
			switch fault {
			case "projection-denied":
				sdk.deny = denied
			case "nil-context":
				sdk.empty = true
			case "cancelled":
				sdk.cancel = cancel
			}
			authorize := func(current context.Context) error {
				if current.Value(historicalPrincipalKey{}) != "original" {
					return ErrRecord
				}
				if fault == "authority-denied" {
					return denied
				}
				return current.Err()
			}
			state := storage.IdentityCutoverState{Version: 1, Scope: testScope, Phase: "legacy", SourceRooms: f.crypto.binding.Rooms}
			err = previous.PrepareAuthorized(ctx, state, authorize)
			if fault == "none" {
				if err != nil || sdk.projections != 1 || sdk.publications < 3 {
					t.Fatal("authorized projected source did not retain every original publication guard", err, sdk.projections, sdk.publications)
				}
				if ctx.Value(historicalDestinationIntentKey{}) != true {
					t.Fatal("source projection replaced its destination parent")
				}
			} else {
				if err == nil || f.crypto.puts != 0 {
					t.Fatal("failed projection reached accepted storage", err, f.crypto.puts)
				}
				if fault == "authority-denied" && sdk.projections != 0 {
					t.Fatal("projection ran before the original source policy")
				}
				if fault == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatal("source projection detached the original cancellation", err)
				}
			}
		})
	}
}
