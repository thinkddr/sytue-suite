// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type plannedReaderCrypto struct {
	SourcePlanCrypto
	scope                          storage.Scope
	ref                            storage.Reference
	raw                            []byte
	reads, downloads, downloaded   int
	commitments                    int
	proofSHA                       string
	proofRequest                   *storage.FileRequest
	proofErr, readErr, downloadErr error
	root                           storage.Reference
}

func (c *plannedReaderCrypto) ReadStorageRecord(_ context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	c.reads++
	if c.readErr != nil {
		return storage.Record{}, c.readErr
	}
	if ref != c.ref || object != sourcePlanObject(plannedEpoch) || role != "content" {
		return storage.Record{}, ErrSourcePlan
	}
	root := c.root
	if !root.Valid() {
		root = storage.Reference{RoomID: c.ref.RoomID, EventID: "$plan_file"}
	}
	payload, _ := json.Marshal(storage.FileLink{Kind: storage.FileLinkKind, Content: storage.ContentPointer{Kind: "file", RevisionID: plannedEpoch, Reference: root}})
	return storage.Record{Version: 1, Scope: c.scope, ObjectID: object, RevisionID: plannedEpoch, Role: "content", Payload: payload}, nil
}

func (c *plannedReaderCrypto) StorageFileInfo(context.Context, storage.Reference, string, string) (storage.FileRequest, error) {
	return storage.FileRequest{ObjectID: sourcePlanObject(plannedEpoch), RevisionID: plannedEpoch, SizeBytes: int64(len(c.raw)), Metadata: []byte(`{"kind":"` + sourcePlanKind + `"}`)}, nil
}

func (c *plannedReaderCrypto) DownloadStorageFile(ctx context.Context, _ *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	c.downloads++
	c.downloaded += len(c.raw)
	if c.downloadErr != nil {
		return storage.FileRequest{}, c.downloadErr
	}
	request, err := c.StorageFileInfo(ctx, ref, object, revision)
	if err != nil {
		return request, err
	}
	_, err = out.Write(c.raw)
	return request, err
}

func (c *plannedReaderCrypto) StorageFileCommitment(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	c.commitments++
	request, err := c.StorageFileInfo(ctx, ref, object, revision)
	if c.proofRequest != nil {
		request = *c.proofRequest
	}
	sha := c.proofSHA
	if sha == "" {
		sha = planBytesHash(c.raw)
	}
	if c.proofErr != nil {
		err = c.proofErr
	}
	return request, sha, err
}

const plannedEpoch = "12345678-1234-1234-1234-123456789abc"

type plannedBackend struct {
	Source
	state              SnapshotState
	rows               []storage.StreamSourceRow
	payloads           map[string][]byte
	limit, begins      int
	begun              string
	tamper, earlyFinal bool
	opened             []string
}

func (b *plannedBackend) State(context.Context, storage.Scope) (SnapshotState, error) {
	return b.state, nil
}

func (b *plannedBackend) Begin(_ context.Context, _ storage.Scope, epoch string) (SnapshotState, error) {
	b.begins++
	b.begun = epoch
	b.state.Mode = "frozen"
	b.state.Epoch = epoch
	now := time.Now().UTC()
	b.state.StartedAt = &now
	b.state.Rows = int64(len(b.rows))
	b.state.Blobs = 1
	return b.state, nil
}

func (b *plannedBackend) Page(_ context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (SnapshotPage, error) {
	page := SnapshotPage{Scope: scope, Epoch: epoch, Rows: []storage.StreamSourceRow{}}
	for _, row := range b.rows {
		if !afterSourceRow(row, after) {
			continue
		}
		if len(page.Rows) == b.limit {
			last := page.Rows[len(page.Rows)-1]
			page.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
			break
		}
		page.Rows = append(page.Rows, row)
	}
	if b.tamper && len(page.Rows) > 0 {
		page.Rows[0].SHA256 = planBytesHash([]byte("changed backend inventory"))
	}
	if b.earlyFinal {
		page.Next = nil
	}
	return page, nil
}

func (b *plannedBackend) Open(_ context.Context, _ storage.Scope, _ string, row SnapshotRow) (io.ReadCloser, error) {
	b.opened = append(b.opened, row.Kind)
	body, ok := b.payloads[row.Kind+"\x00"+row.ID]
	if !ok {
		return nil, ErrSourcePlan
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

type plannedControl struct {
	SourcePlanControl
	routing    model.SourcePlanRouting
	preflights int
}

func (c *plannedControl) PrivateSourcePlanRouting(context.Context, storage.Scope) (model.SourcePlanRouting, error) {
	return c.routing, nil
}

func (c *plannedControl) PrivateSourcePreflight(context.Context, storage.Scope, string) (model.SourcePreflight, error) {
	c.preflights++
	return model.SourcePreflight{}, ErrSourcePlan
}

func plannedSourceFixture(t *testing.T) (*PlannedSource, *plannedBackend, *plannedControl, *pinAuthorityFixture, SourcePlan) {
	t.Helper()
	plan, snapshot, authority := sourcePlanFixture(t)
	plan.Epoch = plannedEpoch
	backend := &plannedBackend{limit: 1, payloads: map[string][]byte{}}
	for _, row := range snapshot.Rows() {
		backend.payloads[row.Kind+"\x00"+row.ID] = row.JSON
	}
	body := []byte("exact retained thumbnail")
	d := model.Derivada{ActivoID: uuid.MustParse(plan.Originals[0].Original.AssetID), Huella: "historical-recipe", Variante: model.VarianteMiniatura, Formato: "png"}
	key, _ := d.Clave()
	media := model.MediaRow{ID: planBytesHash([]byte(key)), Tenant: plan.Scope.Tenant, Owner: plan.Scope.Subject, AssetID: d.ActivoID.String(), Role: "variant", Size: int64(len(body)), SHA256: planBytesHash(body), ContentType: "image/png", Fingerprint: d.Huella, Variant: d.Variante, Format: d.Formato}
	raw, _ := json.Marshal(media)
	plan.Backend = append(plan.Backend, storage.StreamSourceRow{Kind: "media", ID: media.ID, Size: int64(len(raw)), SHA256: planBytesHash(raw)}, media.Descriptor())
	plan.Counts.BackendRows = int64(len(plan.Backend))
	plan.Counts.BackendBlobs = 1
	backend.payloads["media\x00"+media.ID] = raw
	backend.payloads["variant\x00"+media.ID] = body
	backend.rows = slices.Clone(plan.Backend)
	slices.SortFunc(backend.rows, sourceRowCompare)
	now := time.Now().UTC()
	backend.state = SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: plan.Scope, Mode: "frozen", Epoch: plan.Epoch, StartedAt: &now, Rows: plan.Counts.BackendRows, Blobs: plan.Counts.BackendBlobs}}
	ref := model.SourcePlanReference{Epoch: plan.Epoch, Link: storage.Reference{RoomID: "!photos:test", EventID: "$saved_plan"}}
	control := &plannedControl{routing: model.SourcePlanRouting{Scope: plan.Scope, Plan: &ref}}
	encoded, _ := json.Marshal(plan)
	crypto := &plannedReaderCrypto{scope: plan.Scope, ref: ref.Link, raw: encoded}
	plans := &SourcePlanStore{cfg: SourcePlanConfig{Scope: plan.Scope, Crypto: crypto, Originals: authority}, gate: offline.NewWorkGate(t.Context())}
	t.Cleanup(plans.Close)
	source, err := NewPlannedSource(plan.Scope, backend, control, plans)
	if err != nil {
		t.Fatal(err)
	}
	return source, backend, control, authority, plan
}

func TestPlannedSourceMergesEveryBackendPageAndPrivateOriginalWithoutSkippingMetadata(t *testing.T) {
	source, backend, _, authority, plan := plannedSourceFixture(t)
	state, err := source.State(t.Context(), plan.Scope)
	if err != nil || state.Rows != 5 || state.Blobs != 2 || backend.state.Rows != 3 || backend.state.Blobs != 1 {
		t.Fatal("composite counts hid backend/pin work", state, err)
	}
	seen := map[string]bool{}
	after := storage.LegacySourceCursor{}
	for {
		page, err := source.Page(t.Context(), plan.Scope, plan.Epoch, after)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Rows {
			key := row.Kind + "\x00" + row.ID
			if seen[key] {
				t.Fatal("duplicate source row", key)
			}
			seen[key] = true
			reader, err := source.Open(t.Context(), plan.Scope, plan.Epoch, row)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil || int64(len(body)) != row.Size || planBytesHash(body) != row.SHA256 {
				t.Fatal("source row changed", row.Kind, err)
			}
			if row.Kind == "original" && !bytes.Equal(body, authority.body) {
				t.Fatal("original changed")
			}
			after = storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}
		}
		if page.Next == nil {
			break
		}
	}
	if len(seen) != 5 {
		t.Fatal("partial backend page skipped metadata before original", len(seen))
	}
	for _, kind := range backend.opened {
		if kind == "original" {
			t.Fatal("original traversed backend HTTP")
		}
	}
	probe := source.plans.cfg.Crypto.(*plannedReaderCrypto)
	if probe.downloads != 1 || probe.downloaded != len(probe.raw) || probe.commitments == 0 {
		t.Fatal("source reopened its complete plan for every row", probe.downloads, probe.downloaded)
	}
	t.Logf("source metadata cost: 1 original, %d imported rows, plan=%d bytes, SDK plan downloads=%d (%d bytes), record reads=%d", len(seen), len(probe.raw), probe.downloads, probe.downloaded, probe.reads)
}

func TestPlannedSourceMetadataIsIndependentOfDriveButBeginAndOriginalReadsAreNot(t *testing.T) {
	source, backend, _, _, plan := plannedSourceFixture(t)
	source.plans.cfg.Originals = nil
	if _, err := source.State(t.Context(), plan.Scope); err != nil {
		t.Fatal("metadata required the Drive identity", err)
	}
	page, err := source.Page(t.Context(), plan.Scope, plan.Epoch, storage.LegacySourceCursor{})
	if err != nil || len(page.Rows) == 0 {
		t.Fatal("metadata page required Drive", err)
	}
	if _, err := source.Begin(t.Context(), plan.Scope, plan.Epoch); err == nil || backend.begins != 0 {
		t.Fatal("Begin skipped origin authorization")
	}
	for _, pin := range plan.Originals {
		if _, err := source.Open(t.Context(), plan.Scope, plan.Epoch, pin.Descriptor); err == nil {
			t.Fatal("original opened without Drive authorization")
		}
	}
}

func TestSourcePlanMetadataCacheRevalidatesCommitmentsAndNeverMasksErrors(t *testing.T) {
	for _, change := range []string{"digest", "request", "root", "read", "proof", "download", "scope", "lease"} {
		t.Run(change, func(t *testing.T) {
			source, _, control, _, plan := plannedSourceFixture(t)
			ref := *control.routing.Plan
			if _, err := source.plans.ReadMetadata(t.Context(), ref); err != nil {
				t.Fatal(err)
			}
			crypto := source.plans.cfg.Crypto.(*plannedReaderCrypto)
			switch change {
			case "digest":
				crypto.proofSHA = planBytesHash([]byte("different commitment"))
			case "request":
				r, _ := crypto.StorageFileInfo(t.Context(), storage.Reference{}, "", "")
				r.Metadata = []byte(`{"kind":"changed"}`)
				crypto.proofRequest = &r
			case "root":
				crypto.root = storage.Reference{RoomID: ref.Link.RoomID, EventID: "$different_root"}
			case "read":
				crypto.readErr = ErrSourcePlan
			case "proof":
				crypto.proofErr = ErrSourcePlan
			case "download":
				crypto.downloadErr = ErrSourcePlan
			case "scope":
				source.plans.cfg.Scope.Subject = "other"
			case "lease":
				source.plans.gate.Close()
			}
			var err error
			if change == "download" {
				_, err = source.plans.AuditMetadata(t.Context(), ref)
			} else {
				_, err = source.plans.ReadMetadata(t.Context(), ref)
			}
			if err == nil {
				t.Fatal("changed or unavailable plan reused cached success", change, plan.Epoch)
			}
			if source.plans.cache != nil {
				t.Fatal("failure retained stale plan cache")
			}
		})
	}
}

func TestSourcePlanMetadataAuditReadsBytesAndReturnedPlansDoNotAliasCache(t *testing.T) {
	source, _, control, _, _ := plannedSourceFixture(t)
	ref := *control.routing.Plan
	plan, err := source.plans.ReadMetadata(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	plan.Originals[0].Pin[0] = 'x'
	plan.Backend[0].SHA256 = planBytesHash([]byte("caller mutation"))
	again, err := source.plans.ReadMetadata(t.Context(), ref)
	if err != nil || !json.Valid(again.Originals[0].Pin) || again.Backend[0].SHA256 == plan.Backend[0].SHA256 {
		t.Fatal("returned plan aliased retained cache", err)
	}
	crypto := source.plans.cfg.Crypto.(*plannedReaderCrypto)
	before := crypto.downloads
	if _, err := source.plans.AuditMetadata(t.Context(), ref); err != nil || crypto.downloads != before+1 {
		t.Fatal("explicit audit inferred byte availability from metadata", err)
	}
}

func TestSourcePlanMetadataWithoutCommitmentPortAlwaysReadsCompleteBytes(t *testing.T) {
	source, _, control, _, _ := plannedSourceFixture(t)
	crypto := source.plans.cfg.Crypto.(*plannedReaderCrypto)
	source.plans.cfg.Crypto = struct{ SourcePlanCrypto }{SourcePlanCrypto: crypto}
	for range 2 {
		if _, err := source.plans.ReadMetadata(t.Context(), *control.routing.Plan); err != nil {
			t.Fatal(err)
		}
	}
	if crypto.downloads != 2 {
		t.Fatal("adapter without authenticated commitment inferred cached success")
	}
}

func TestPlannedSourceRestoresAcceptedEpochBeforeBeginWithoutRecapturingDriveHead(t *testing.T) {
	source, backend, control, authority, plan := plannedSourceFixture(t)
	backend.state.Mode = "legacy"
	backend.state.Epoch = ""
	backend.state.StartedAt = nil
	backend.state.Rows = 0
	backend.state.Blobs = 0
	before := authority.captures
	epoch, err := source.SnapshotEpoch(t.Context(), plan.Scope)
	if err != nil || epoch != plan.Epoch {
		t.Fatal("fresh accepted plan epoch lost", err)
	}
	state, err := source.Begin(t.Context(), plan.Scope, epoch)
	if err != nil || state.Rows != 5 || state.Blobs != 2 || backend.begun != plan.Epoch || authority.captures != before || control.preflights != 0 {
		t.Fatal("fresh Begin recaptured or changed accepted source", err)
	}
}

func TestPlannedSourceRejectsChangedRoutingCountsAndBackendInventory(t *testing.T) {
	for _, change := range []string{"scope", "link", "epoch", "rows", "blobs", "body descriptor", "early final"} {
		t.Run(change, func(t *testing.T) {
			source, backend, control, _, plan := plannedSourceFixture(t)
			switch change {
			case "scope":
				control.routing.Scope.Subject = "other"
			case "link":
				control.routing.Plan.Link.EventID = "$other"
			case "epoch":
				control.routing.Plan.Epoch = uuid.NewString()
			case "rows":
				backend.state.Rows++
			case "blobs":
				backend.state.Blobs++
			case "body descriptor":
				backend.tamper = true
			case "early final":
				backend.earlyFinal = true
			}
			if _, err := source.Page(t.Context(), plan.Scope, plan.Epoch, storage.LegacySourceCursor{}); err == nil {
				t.Fatal("changed source accepted")
			}
		})
	}
}
