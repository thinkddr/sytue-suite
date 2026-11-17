// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type historicalNotesPort struct{ *trustedNotesPort }

func (p *historicalNotesPort) FrozenStorageBindings(context.Context) (storage.FrozenBinding, error) {
	return storage.FrozenBinding{}, ErrSource
}

func TestBothNotesIdentityPathsEnforceDomainBeforePublishing(t *testing.T) {
	for _, identity := range []storage.IdentityVersion{storage.LegacyIdentity, storage.IsolatedIdentity} {
		for _, row := range []struct {
			name, raw string
			want      error
		}{
			{"attachment", `{"id":"note","owner_sub":"alice","notebook_id":"book","kind":"image","content":"https://readable.invalid/content"}`, ErrAttachmentPending},
			{"foreign-owner", `{"id":"note","owner_sub":"bob","notebook_id":"book","kind":"text","content":"foreign content"}`, ErrShared},
		} {
			t.Run(fmt.Sprintf("%d/%s", identity, row.name), func(t *testing.T) {
				fixture, port, source := notesFixture(t)
				port.identity = identity
				source.rows["note/note"] = []byte(row.raw)
				cfg := fixture.cfg
				cfg.WorkLease = func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() }
				// An optional setup value must never weaken either domain path.
				cfg.ValidateRow = func(context.Context, string, string, []byte) error { return nil }
				cfg.ValidateCatalog = func(context.Context, records.Catalog) error { return nil }
				cfg.BinaryKind = func(string) bool { return true }
				cfg.ValidateBinary = func(context.Context, records.SnapshotRow) error { return nil }
				var err error
				if identity == storage.LegacyIdentity {
					cfg.Crypto = &historicalNotesPort{port}
					var historical *records.HistoricalSource
					historical, err = NewHistoricalSource(cfg)
					if err != nil {
						t.Fatal(err)
					}
					defer historical.Close()
					err = historical.Prepare(t.Context(), storage.IdentityCutoverState{Version: 1, Scope: testScope, Phase: "legacy", SourceRooms: port.binding.Rooms})
				} else {
					var target *Repository
					target, err = New(cfg)
					if err != nil {
						t.Fatal(err)
					}
					defer target.Close()
					err = target.ImportLegacySnapshot(t.Context())
				}
				if !errors.Is(err, row.want) || port.uploads != 0 || port.writes != 0 {
					t.Fatal("domain guard bypassed", err, port.uploads, port.writes)
				}
			})
		}
	}
}
