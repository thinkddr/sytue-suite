// SPDX-License-Identifier: Apache-2.0
package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// RetireAcknowledged preserves the encrypted transfer evidence before removing
// its active work markers. The caller first authenticates receipt from the SDK;
// public routing, a successful HTTP response or a cached timestamp is not enough.
func RetireAcknowledged(ctx context.Context, journal *offline.Store, owner, active, archive string, scope storage.Scope, state storage.IdentityCutoverState, receipt storage.Reference) error {
	if !journal.Encrypted() || owner != scope.Tenant+"\x00"+scope.Subject+"\x00"+scope.Product || active == "" || archive == "" || active == archive || state.Validate(scope) != nil || state.Phase != "isolated" || state.Receipt == nil || *state.Receipt != receipt {
		return ErrSource
	}
	var plan Plan
	found, err := journal.Read(owner, active, "plan", &plan)
	if err != nil {
		return err
	}
	ids, err := journal.RecordIDs(owner, active)
	if err != nil {
		return err
	}
	if !found {
		if len(ids) != 0 {
			return ErrSource
		}
		return nil
	}
	if !plan.MatchesAcknowledgment(scope, state, receipt) {
		return ErrSource
	}
	for _, id := range ids {
		if id == "plan" {
			continue
		}
		if !strings.HasPrefix(id, "transfer_") {
			return ErrSource
		}
		if err := archiveRecord(ctx, journal, owner, active, archive, id); err != nil {
			return err
		}
	}
	return archiveRecord(ctx, journal, owner, active, archive, "plan")
}

func archiveRecord(ctx context.Context, journal *offline.Store, owner, active, archive, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var raw, prior json.RawMessage
	found, err := journal.Read(owner, active, id, &raw)
	defer clear(raw)
	if err != nil || !found {
		return err
	}
	archived, err := journal.Read(owner, archive, id, &prior)
	defer clear(prior)
	if err != nil {
		return err
	}
	if archived && !bytes.Equal(raw, prior) {
		return ErrSource
	}
	if !archived {
		if err := journal.Write(owner, archive, id, raw); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return journal.Delete(owner, active, id)
}
