// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"slices"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type pinAuthorityFixture struct {
	captures int
	body     []byte
	scope    storage.Scope
}

func (f *pinAuthorityFixture) Capture(_ context.Context, scope storage.Scope, original model.Original) (PlannedOriginal, error) {
	f.captures++
	return PlannedOriginal{Original: original, Descriptor: storage.StreamSourceRow{Kind: "original", ID: original.AssetID, Size: int64(len(f.body)), SHA256: planBytesHash(f.body)}, Pin: json.RawMessage(`{"kind":"synthetic-authorized-immutable-pin"}`)}, nil
}

func (f *pinAuthorityFixture) Validate(_ context.Context, scope storage.Scope, value PlannedOriginal) error {
	if scope != f.scope || value.Descriptor.SHA256 != planBytesHash(f.body) || string(value.Pin) != `{"kind":"synthetic-authorized-immutable-pin"}` {
		return ErrSourcePlan
	}
	return nil
}

func (f *pinAuthorityFixture) Write(_ context.Context, scope storage.Scope, value PlannedOriginal, out io.Writer) error {
	if err := f.Validate(context.Background(), scope, value); err != nil {
		return err
	}
	_, err := out.Write(f.body)
	return err
}

func sourcePlanFixture(t *testing.T) (SourcePlan, *model.Snapshot, *pinAuthorityFixture) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	id := uuid.NewString()
	raw := []byte(`{"id":"` + id + `","tenant":"acme","owner_sub":"alice","drive_file_id":"retained-drive-id","name":"original.png","media_type":"image/png","bytes":4,"width":16,"height":12,"orientation":1,"uploaded_at":"2026-10-30T10:00:00Z","future_integer":900719925474099312345}`)
	snapshot, err := model.NewSnapshot(scope, []model.Row{{Kind: "asset", ID: id, JSON: raw}})
	if err != nil {
		t.Fatal(err)
	}
	backend := []storage.StreamSourceRow{{Kind: "asset", ID: id, Size: int64(len(raw)), SHA256: planBytesHash(raw)}}
	authority := &pinAuthorityFixture{body: []byte{0, 1, 2, 3}, scope: scope}
	plan, err := PrepareSourcePlan(t.Context(), scope, uuid.NewString(), "opaque-preflight-ticket", snapshot, backend, 0, authority)
	if err != nil {
		t.Fatal(err)
	}
	return plan, snapshot, authority
}

func TestSourcePlanPreservesExactBackendAndOriginalCountsAndPins(t *testing.T) {
	plan, snapshot, authority := sourcePlanFixture(t)
	rows, blobs, err := plan.Counts.Totals()
	if err != nil || rows != 3 || blobs != 1 || authority.captures != 1 {
		t.Fatal(rows, blobs, err)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var recovered SourcePlan
	if json.Unmarshal(encoded, &recovered) != nil || recovered.Validate(t.Context(), authority) != nil {
		t.Fatal("saved plan did not reopen")
	}
	if authority.captures != 1 {
		t.Fatal("recovery recaptured current Drive head")
	}
	if !slices.Equal(recovered.Backend, plan.Backend) {
		t.Fatal("backend source hashes or sizes normalized")
	}
	generated, payloads, err := recovered.GeneratedRows()
	if err != nil || len(generated) != 2 || len(payloads) != 1 {
		t.Fatal(err)
	}
	if recovered.validateMedia(snapshot) != nil {
		t.Fatal("generated original changed graph")
	}
	for _, row := range generated {
		if row.Kind == "media" {
			raw := payloads["media\x00"+row.ID]
			if int64(len(raw)) != row.Size || planBytesHash(raw) != row.SHA256 {
				t.Fatal("generated descriptor changed bytes")
			}
		}
	}
}

func TestSourcePlanRejectsCountsScopePinAndBackendSubstitution(t *testing.T) {
	for _, change := range []string{"backend count", "blob count", "original count", "overflow", "scope", "pin", "digest", "asset", "duplicate"} {
		t.Run(change, func(t *testing.T) {
			plan, _, authority := sourcePlanFixture(t)
			switch change {
			case "backend count":
				plan.Counts.BackendRows++
			case "blob count":
				plan.Counts.BackendBlobs++
			case "original count":
				plan.Counts.OriginalRows++
			case "overflow":
				plan.Counts.OriginalBlobs = math.MaxInt64
				plan.Counts.OriginalRows = -2
			case "scope":
				plan.Scope.Subject = "other"
			case "pin":
				plan.Originals[0].Pin = json.RawMessage(`{"pin":"other"}`)
			case "digest":
				plan.Originals[0].Descriptor.SHA256 = planBytesHash([]byte("changed"))
			case "asset":
				plan.Backend[0].ID = uuid.NewString()
			case "duplicate":
				plan.Backend = append(plan.Backend, plan.Backend[0])
				plan.Counts.BackendRows++
			}
			if plan.Validate(t.Context(), authority) == nil {
				t.Fatal("changed plan accepted")
			}
		})
	}
	plan, snapshot, authority := sourcePlanFixture(t)
	for _, change := range []string{"hash", "size", "missing"} {
		backend := slices.Clone(plan.Backend)
		switch change {
		case "hash":
			backend[0].SHA256 = planBytesHash([]byte("normalized"))
		case "size":
			backend[0].Size--
		case "missing":
			backend = nil
		}
		before := authority.captures
		if _, err := PrepareSourcePlan(t.Context(), plan.Scope, plan.Epoch, plan.CAS, snapshot, backend, 0, authority); err == nil || authority.captures != before {
			t.Fatal("changed metadata captured Drive pins", change, err)
		}
	}
}

func TestSourcePlanRejectsSharedSnapshotBeforePinCapture(t *testing.T) {
	plan, snapshot, authority := sourcePlanFixture(t)
	rows := snapshot.Rows()
	var asset map[string]json.RawMessage
	if json.Unmarshal(rows[0].JSON, &asset) != nil {
		t.Fatal("fixture")
	}
	asset["owner_sub"] = json.RawMessage(`"other"`)
	rows[0].JSON, _ = json.Marshal(asset)
	shared, err := model.NewSnapshot(plan.Scope, rows)
	if err != nil {
		t.Fatal(err)
	}
	before := authority.captures
	_, err = PrepareSourcePlan(t.Context(), plan.Scope, plan.Epoch, plan.CAS, shared, nil, 0, authority)
	if !errors.Is(err, model.ErrShared) || authority.captures != before {
		t.Fatal("shared snapshot captured or froze source", err)
	}
}
