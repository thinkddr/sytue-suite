// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type pagedPreflightFixture struct {
	info                       model.SourcePreflightInfo
	rows                       []storage.StreamSourceRow
	payloads                   map[string][]byte
	change                     string
	active, maxActive, maxBody int
}

func (f *pagedPreflightFixture) StartPrivateSourcePreflight(context.Context, storage.Scope, string) (model.SourcePreflightInfo, error) {
	return f.info, nil
}

func (f *pagedPreflightFixture) PrivateSourcePreflightPage(_ context.Context, info model.SourcePreflightInfo, after storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	if info != f.info {
		return storage.StreamSourcePage{}, ErrSourcePlan
	}
	page := storage.StreamSourcePage{Scope: info.Scope, Epoch: info.Epoch, Rows: []storage.StreamSourceRow{}}
	for _, row := range f.rows {
		if !afterSourceRow(row, after) {
			continue
		}
		if len(page.Rows) == 2 {
			last := page.Rows[len(page.Rows)-1]
			page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
			break
		}
		page.Rows = append(page.Rows, row)
	}
	switch f.change {
	case "missing page":
		page.Next = nil
	case "foreign scope":
		page.Scope.Subject = "other"
	case "changed descriptor":
		if len(page.Rows) > 0 {
			page.Rows[0].SHA256 = planBytesHash([]byte("other"))
		}
	}
	return page, nil
}

func (f *pagedPreflightFixture) OpenPrivateSourcePreflightJSON(_ context.Context, info model.SourcePreflightInfo, row storage.StreamSourceRow) (io.ReadCloser, error) {
	if info != f.info {
		return nil, ErrSourcePlan
	}
	body := f.payloads[row.Kind+"\x00"+row.ID]
	if f.change == "changed body" {
		body = bytes.ReplaceAll(body, []byte("312345"), []byte("312346"))
	}
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	if len(body) > f.maxBody {
		f.maxBody = len(body)
	}
	return &preflightCountingReader{Reader: bytes.NewReader(body), close: func() { f.active-- }}, nil
}

type preflightCountingReader struct {
	*bytes.Reader
	close func()
}

func (r *preflightCountingReader) Close() error {
	if r.close != nil {
		r.close()
		r.close = nil
	}
	return nil
}

func pagedPreflightFixtureForRows(t *testing.T, scope storage.Scope, epoch string, rows []model.Row) *pagedPreflightFixture {
	t.Helper()
	f := &pagedPreflightFixture{info: model.SourcePreflightInfo{Scope: scope, Epoch: epoch, CAS: uuid.NewString()}, payloads: map[string][]byte{}}
	for _, row := range rows {
		f.rows = append(f.rows, storage.StreamSourceRow{Kind: row.Kind, ID: row.ID, Size: int64(len(row.JSON)), SHA256: planBytesHash(row.JSON)})
		f.payloads[row.Kind+"\x00"+row.ID] = row.JSON
		if row.Kind == "asset" {
			f.info.Originals++
		}
	}
	slices.SortFunc(f.rows, sourceRowCompare)
	sum, err := model.SourceInventoryHash(scope, epoch)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range f.rows {
		if err := model.AppendSourceInventoryHash(sum, row); err != nil {
			t.Fatal(err)
		}
	}
	f.info.SHA256 = hex.EncodeToString(sum.Sum(nil))
	f.info.Rows = int64(len(f.rows))
	return f
}

func TestPagedPreflightPreparesExactPinsWithoutAccumulatingLargeMetadata(t *testing.T) {
	base, snapshot, authority := sourcePlanFixture(t)
	rows := make([]model.Row, 0, 8)
	var total int64
	for range 8 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(snapshot.Rows()[0].JSON, &fields); err != nil {
			t.Fatal(err)
		}
		id := uuid.NewString()
		fields["id"], _ = json.Marshal(id)
		fields["drive_file_id"], _ = json.Marshal("retained-drive-" + id)
		fields["camera"], _ = json.Marshal(strings.Repeat("retained EXIF / ", 350000))
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		total += int64(len(raw))
		rows = append(rows, model.Row{Kind: "asset", ID: id, JSON: raw})
	}
	if total <= 32<<20 {
		t.Fatal("large aggregate fixture is too small")
	}
	control := pagedPreflightFixtureForRows(t, base.Scope, base.Epoch, rows)
	plan, err := PrepareSourcePlanPaged(t.Context(), base.Scope, base.Epoch, control, authority)
	if err != nil || plan.Counts.BackendRows != 8 || plan.Counts.OriginalBlobs != 8 || !slices.Equal(plan.Backend, control.rows) {
		t.Fatal("paged preflight lost exact metadata commitments or original pins", err)
	}
	encoded, err := json.Marshal(plan)
	if err != nil || len(encoded) > 32<<10 || control.maxActive != 1 || control.active != 0 || control.maxBody >= 8<<20 {
		t.Fatal("large metadata accumulated in plan or concurrent entity readers", len(encoded), control.maxActive, control.active, control.maxBody, err)
	}
	t.Log("PHOTOS_LARGE_CLIENT_PREFLIGHT_PASS", total, "raw bytes; one entity reader; largest body", control.maxBody, "encrypted plan bytes", len(encoded))
}

func TestPagedPreflightRejectsIncompleteChangedOrForeignSourceBeforeCapturingPins(t *testing.T) {
	base, snapshot, authority := sourcePlanFixture(t)
	rows := snapshot.Rows()
	// Three descriptors require a second page, making an early final page a
	// genuinely incomplete inventory even when each returned row is valid.
	for range 2 {
		raw := bytes.Clone(rows[0].JSON)
		id := uuid.NewString()
		raw = bytes.ReplaceAll(raw, []byte(rows[0].ID), []byte(id))
		raw = bytes.ReplaceAll(raw, []byte("retained-drive-id"), []byte("retained-drive-"+id))
		rows = append(rows, model.Row{Kind: "asset", ID: id, JSON: raw})
	}
	for _, change := range []string{"missing page", "foreign scope", "changed descriptor", "changed body", "wrong epoch", "wrong inventory"} {
		t.Run(change, func(t *testing.T) {
			control := pagedPreflightFixtureForRows(t, base.Scope, base.Epoch, rows)
			control.change = change
			if change == "wrong epoch" {
				control.info.Epoch = uuid.NewString()
			}
			if change == "wrong inventory" {
				control.info.SHA256 = strings.Repeat("1", 64)
			}
			before := authority.captures
			if _, err := PrepareSourcePlanPaged(t.Context(), base.Scope, base.Epoch, control, authority); err == nil || authority.captures != before || control.active != 0 {
				t.Fatal("unverified paged source reached original capture", err, authority.captures-before, control.active)
			}
		})
	}
}

type rejectedPreflightPins struct{ OriginalPinAuthority }

func (rejectedPreflightPins) Validate(context.Context, storage.Scope, PlannedOriginal) error {
	return ErrSourcePlan
}

type pagedPlannedControl struct {
	SourcePlanControl
	*pagedPreflightFixture
}

func TestPagedPreflightMissingRecoveryCapabilityLeavesBackendUnfrozen(t *testing.T) {
	source, backend, _, authority, plan := plannedSourceFixture(t)
	backend.state = SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: plan.Scope, Mode: "legacy"}}
	rows := []model.Row{}
	for _, row := range backend.rows {
		if row.Kind != "variant" {
			rows = append(rows, model.Row{Kind: row.Kind, ID: row.ID, JSON: backend.payloads[row.Kind+"\x00"+row.ID]})
		}
	}
	control := pagedPreflightFixtureForRows(t, plan.Scope, plan.Epoch, rows)
	control.rows = backend.rows
	control.info.Blobs = 1
	control.info.Rows = int64(len(backend.rows))
	sum, _ := model.SourceInventoryHash(plan.Scope, plan.Epoch)
	for _, row := range control.rows {
		_ = model.AppendSourceInventoryHash(sum, row)
	}
	control.info.SHA256 = hex.EncodeToString(sum.Sum(nil))
	source.control = &pagedPlannedControl{SourcePlanControl: &plannedControl{routing: model.SourcePlanRouting{Scope: plan.Scope}}, pagedPreflightFixture: control}
	source.plans.cfg.Originals = rejectedPreflightPins{OriginalPinAuthority: authority}
	if _, err := source.Begin(t.Context(), plan.Scope, plan.Epoch); err == nil || backend.begins != 0 {
		t.Fatal("missing publication recovery capability froze source", err, backend.begins)
	}
}
