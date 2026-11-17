// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const sourcePlanKind = "org.sytue.photos.source-plan.v1"

const maxSourcePlanBytes = 32 << 20

func planBytesHash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

var ErrSourcePlan = errors.New("photos: retained source plan is invalid or changed")

// OriginalPinAuthority adapts the existing authorized Drive repository. Pin is
// the exact serialized SDK FilePin, not a second pin format or a content key.
// Implementations validate that pin's scope/id/digest through Drive before use;
// Write must call WritePinnedOriginal and has no HTTP alternative.
type OriginalPinAuthority interface {
	Capture(context.Context, storage.Scope, model.Original) (PlannedOriginal, error)
	Validate(context.Context, storage.Scope, PlannedOriginal) error
	Write(context.Context, storage.Scope, PlannedOriginal, io.Writer) error
}

type PlannedOriginal struct {
	Original   model.Original          `json:"original"`
	Descriptor storage.StreamSourceRow `json:"descriptor"`
	Pin        json.RawMessage         `json:"pin"`
}

// SourcePlanCounts keeps the retained server inventory and device-only pins
// explicit. The composite source never rewrites either count to hide work.
type SourcePlanCounts struct {
	BackendRows   int64 `json:"backend_rows"`
	BackendBlobs  int64 `json:"backend_blobs"`
	OriginalRows  int64 `json:"original_rows"`
	OriginalBlobs int64 `json:"original_blobs"`
}

func (c SourcePlanCounts) Totals() (int64, int64, error) {
	if c.BackendRows < 0 || c.BackendBlobs < 0 || c.BackendBlobs > c.BackendRows || c.OriginalBlobs < 0 || c.OriginalBlobs >= (1<<52) || c.OriginalRows != 2*c.OriginalBlobs || c.BackendRows >= 1<<53 || c.BackendRows > (1<<53-1)-c.OriginalRows {
		return 0, 0, ErrSourcePlan
	}
	return c.BackendRows + c.OriginalRows, c.BackendBlobs + c.OriginalBlobs, nil
}

type SourcePlan struct {
	Kind  string        `json:"kind"`
	Scope storage.Scope `json:"scope"`
	Epoch string        `json:"epoch"`
	// CAS is the opaque server preflight ticket. It covers raw metadata and all
	// existing derived object hashes; it contains no device pin or private key.
	CAS       string                    `json:"cas"`
	Counts    SourcePlanCounts          `json:"counts"`
	Backend   []storage.StreamSourceRow `json:"backend"`
	Originals []PlannedOriginal         `json:"originals"`
}

func backendPlanKind(kind string) bool { return kind == "variant" || model.SourceKind(kind) }

func (p SourcePlan) Validate(ctx context.Context, authority OriginalPinAuthority) error {
	if err := p.validateStructure(ctx); err != nil {
		return err
	}
	if authority == nil && len(p.Originals) > 0 {
		return ErrNotReady
	}
	for _, original := range p.Originals {
		if err := authority.Validate(ctx, p.Scope, original); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (p SourcePlan) validateStructure(ctx context.Context) error {
	if !p.validHeader() {
		return ErrSourcePlan
	}
	if _, _, err := p.Counts.Totals(); err != nil {
		return err
	}
	if int64(len(p.Backend)) != p.Counts.BackendRows || int64(len(p.Originals)) != p.Counts.OriginalBlobs {
		return ErrSourcePlan
	}
	seen, err := p.validateBackend()
	if err != nil {
		return err
	}
	for _, original := range p.Originals {
		if err := validatePlannedOriginal(original); err != nil {
			return err
		}
		if !seen["asset\x00"+original.Original.AssetID] || seen["original\x00"+original.Original.AssetID] || seen["media\x00"+original.Original.AssetID] {
			return ErrSourcePlan
		}
		seen["original\x00"+original.Original.AssetID] = true
		seen["media\x00"+original.Original.AssetID] = true
	}
	for _, row := range p.Backend {
		if row.Kind == "asset" && !seen["original\x00"+row.ID] {
			return ErrSourcePlan
		}
	}
	return ctx.Err()
}

func (p SourcePlan) validHeader() bool {
	return p.Kind == sourcePlanKind && p.Scope.Valid() && p.Scope.Product == "photos" && storage.MigrationEpoch(p.Epoch) && p.CAS != "" && len(p.CAS) <= 4096
}

func (p SourcePlan) validateBackend() (map[string]bool, error) {
	seen := map[string]bool{}
	var blobs int64
	budget := len(p.CAS) + 256
	for _, row := range p.Backend {
		budget += len(row.ID) + len(row.Kind) + 256
		if budget > maxSourcePlanBytes/2 {
			return nil, ErrSourcePlan
		}
		key := row.Kind + "\x00" + row.ID
		if row.Validate() != nil || !backendPlanKind(row.Kind) || seen[key] {
			return nil, ErrSourcePlan
		}
		seen[key] = true
		if row.Kind == "variant" {
			blobs++
		}
	}
	if blobs != p.Counts.BackendBlobs {
		return nil, ErrSourcePlan
	}
	for _, original := range p.Originals {
		budget += len(original.Pin) + len(original.Original.DriveID) + 1024
		if budget > maxSourcePlanBytes/2 {
			return nil, ErrSourcePlan
		}
	}
	return seen, nil
}

func validatePlannedOriginal(value PlannedOriginal) error {
	if value.Descriptor.Kind != "original" || value.Descriptor.ID != value.Original.AssetID || value.Descriptor.Size != value.Original.Size || value.Original.DriveID == "" || len(value.Original.DriveID) > 1024 || value.Original.ContentType == "" || value.Descriptor.Validate() != nil || len(value.Pin) == 0 || len(value.Pin) > 16<<10 || !json.Valid(value.Pin) {
		return ErrSourcePlan
	}
	media := model.MediaRow{ID: value.Original.AssetID, AssetID: value.Original.AssetID, Role: "original", Size: value.Descriptor.Size, SHA256: value.Descriptor.SHA256, ContentType: value.Original.ContentType}
	return media.Validate()
}

func PrepareSourcePlan(ctx context.Context, scope storage.Scope, epoch, cas string, snapshot *model.Snapshot, backend []storage.StreamSourceRow, backendBlobs int64, authority OriginalPinAuthority) (SourcePlan, error) {
	if snapshot == nil {
		return SourcePlan{}, ErrSourcePlan
	}
	if err := snapshot.PersonalCompatible(); err != nil {
		return SourcePlan{}, err
	}
	if err := validateBackendPlan(snapshot, backend, backendBlobs); err != nil {
		return SourcePlan{}, err
	}
	return prepareSourcePlanPins(ctx, scope, epoch, cas, snapshot, backend, backendBlobs, authority)
}

func prepareSourcePlanPins(ctx context.Context, scope storage.Scope, epoch, cas string, snapshot *model.Snapshot, backend []storage.StreamSourceRow, backendBlobs int64, authority OriginalPinAuthority) (SourcePlan, error) {
	if err := snapshot.PersonalCompatible(); err != nil {
		return SourcePlan{}, err
	}
	if authority == nil && len(snapshot.Originals()) > 0 {
		return SourcePlan{}, ErrNotReady
	}
	plan := SourcePlan{Kind: sourcePlanKind, Scope: scope, Epoch: epoch, CAS: cas, Backend: slices.Clone(backend), Counts: SourcePlanCounts{BackendRows: int64(len(backend)), BackendBlobs: backendBlobs}}
	for _, original := range snapshot.Originals() {
		pin, err := authority.Capture(ctx, scope, original)
		if err != nil {
			return SourcePlan{}, err
		}
		if pin.Original != original {
			return SourcePlan{}, ErrSourcePlan
		}
		pin.Pin = slices.Clone(pin.Pin)
		plan.Originals = append(plan.Originals, pin)
	}
	plan.Counts.OriginalBlobs = int64(len(plan.Originals))
	plan.Counts.OriginalRows = 2 * plan.Counts.OriginalBlobs
	if err := plan.Validate(ctx, authority); err != nil {
		return SourcePlan{}, err
	}
	if err := plan.validateMedia(snapshot); err != nil {
		return SourcePlan{}, err
	}
	return plan, nil
}

func (p SourcePlan) validateMedia(snapshot *model.Snapshot) error {
	generated, payloads, err := p.GeneratedRows()
	if err != nil {
		return err
	}
	rows := snapshot.Rows()
	defer func() {
		for _, row := range rows {
			clear(row.JSON)
		}
	}()
	blobs := []storage.StreamSourceRow{}
	for _, row := range p.Backend {
		if row.Kind == "variant" {
			blobs = append(blobs, row)
		}
	}
	for _, row := range generated {
		if row.Kind == "original" {
			blobs = append(blobs, row)
			continue
		}
		rows = append(rows, model.Row{Kind: row.Kind, ID: row.ID, JSON: payloads[row.Kind+"\x00"+row.ID]})
	}
	complete, err := model.NewSnapshot(p.Scope, rows)
	if err != nil {
		return err
	}
	return complete.ValidateMedia(blobs)
}

func validateBackendPlan(snapshot *model.Snapshot, descriptors []storage.StreamSourceRow, declaredBlobs int64) error {
	metadata := map[string]storage.StreamSourceRow{}
	for _, row := range snapshot.Rows() {
		metadata[row.Kind+"\x00"+row.ID] = storage.StreamSourceRow{Kind: row.Kind, ID: row.ID, Size: int64(len(row.JSON)), SHA256: planBytesHash(row.JSON)}
		clear(row.JSON)
	}
	var blobs int64
	seen := map[string]bool{}
	for _, row := range descriptors {
		key := row.Kind + "\x00" + row.ID
		if row.Validate() != nil || seen[key] {
			return ErrSourcePlan
		}
		seen[key] = true
		if row.Kind == "variant" {
			blobs++
			continue
		}
		if expected, ok := metadata[key]; !ok || expected != row {
			return ErrSourcePlan
		}
		delete(metadata, key)
	}
	if len(metadata) != 0 || blobs != declaredBlobs {
		return ErrSourcePlan
	}
	return nil
}

// GeneratedRows are private original descriptors and their bytes. Server rows
// remain byte-for-byte retained and are merged by cursor identity, not replaced.
func (p SourcePlan) GeneratedRows() ([]storage.StreamSourceRow, map[string][]byte, error) {
	rows := make([]storage.StreamSourceRow, 0, 2*len(p.Originals))
	payloads := map[string][]byte{}
	for _, original := range p.Originals {
		if err := validatePlannedOriginal(original); err != nil {
			return nil, nil, err
		}
		metadata, raw, err := generatedOriginalMetadata(p.Scope, original)
		if err != nil {
			return nil, nil, err
		}
		key := "media\x00" + metadata.ID
		payloads[key] = raw
		rows = append(rows, metadata, original.Descriptor)
	}
	slices.SortFunc(rows, func(a, b storage.StreamSourceRow) int {
		if order := strings.Compare(a.Kind, b.Kind); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	return rows, payloads, nil
}

func generatedOriginalMetadata(scope storage.Scope, original PlannedOriginal) (storage.StreamSourceRow, []byte, error) {
	if !scope.Valid() || scope.Product != "photos" || validatePlannedOriginal(original) != nil {
		return storage.StreamSourceRow{}, nil, ErrSourcePlan
	}
	media := model.MediaRow{ID: original.Original.AssetID, Tenant: scope.Tenant, Owner: scope.Subject, AssetID: original.Original.AssetID, Role: "original", Size: original.Descriptor.Size, SHA256: original.Descriptor.SHA256, ContentType: original.Original.ContentType}
	raw, err := json.Marshal(media)
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	return storage.StreamSourceRow{Kind: "media", ID: media.ID, Size: int64(len(raw)), SHA256: planBytesHash(raw)}, raw, nil
}
