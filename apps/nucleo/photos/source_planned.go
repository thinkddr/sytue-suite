// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type SourcePlanControl interface {
	SourcePlanAcceptor
	PrivateSourcePlanRouting(context.Context, storage.Scope) (model.SourcePlanRouting, error)
	PrivateSourcePreflight(context.Context, storage.Scope, string) (model.SourcePreflight, error)
}

// PlannedSource combines exact retained backend JSON/variants with authorized
// immutable Drive pins. It preserves the two inventories independently and
// never opens an original through its legacy HTTP backend.
type PlannedSource struct {
	scope   storage.Scope
	backend Source
	control SourcePlanControl
	plans   *SourcePlanStore
}

func NewPlannedSource(scope storage.Scope, backend Source, control SourcePlanControl, plans *SourcePlanStore) (*PlannedSource, error) {
	if !scope.Valid() || scope.Product != "photos" || backend == nil || control == nil || plans == nil || plans.cfg.Scope != scope {
		return nil, ErrSourcePlan
	}
	return &PlannedSource{scope: scope, backend: backend, control: control, plans: plans}, nil
}

func (s *PlannedSource) routing(ctx context.Context, scope storage.Scope) (model.SourcePlanRouting, error) {
	if scope != s.scope {
		return model.SourcePlanRouting{}, ErrSourcePlan
	}
	value, err := s.control.PrivateSourcePlanRouting(ctx, scope)
	if err != nil {
		return value, err
	}
	return value, value.Validate(scope)
}

// SnapshotEpoch is consulted by records only for a new legacy WAL. An accepted
// sidecar therefore survives a crash before Begin on a second authorized device.
func (s *PlannedSource) SnapshotEpoch(ctx context.Context, scope storage.Scope) (string, error) {
	routing, err := s.routing(ctx, scope)
	if err != nil {
		return "", err
	}
	if routing.Plan == nil {
		return "", nil
	}
	return routing.Plan.Epoch, nil
}

type loadedSourcePlan struct {
	legacy  *SourcePlan
	archive *sourceArchive
	scope   storage.Scope
	epoch   string
	counts  SourcePlanCounts
}

func (s *PlannedSource) load(ctx context.Context, scope storage.Scope, epoch string) (loadedSourcePlan, error) {
	return s.loadPlan(ctx, scope, epoch, true)
}

func (s *PlannedSource) loadPlan(ctx context.Context, scope storage.Scope, epoch string, resume bool) (loadedSourcePlan, error) {
	routing, err := s.routing(ctx, scope)
	if err != nil {
		return loadedSourcePlan{}, err
	}
	if routing.Plan == nil || routing.Plan.Epoch != epoch {
		return loadedSourcePlan{}, ErrSourcePlan
	}
	isArchive, err := s.plans.archiveFormat(ctx, *routing.Plan)
	if err != nil {
		return loadedSourcePlan{}, err
	}
	if isArchive {
		if resume {
			if err := s.plans.resumeAcceptedArchive(ctx, *routing.Plan, s.control); err != nil {
				return loadedSourcePlan{}, err
			}
		}
		archive, err := s.plans.openArchive(ctx, *routing.Plan, false)
		if err != nil {
			return loadedSourcePlan{}, err
		}
		header, err := archive.header(ctx)
		if err != nil {
			return loadedSourcePlan{}, err
		}
		return loadedSourcePlan{archive: archive, scope: header.Info.Scope, epoch: header.Info.Epoch, counts: header.Counts}, nil
	}
	plan, err := s.plans.ReadMetadata(ctx, *routing.Plan)
	if err != nil {
		return loadedSourcePlan{}, err
	}
	return loadedSourcePlan{legacy: &plan, scope: plan.Scope, epoch: plan.Epoch, counts: plan.Counts}, nil
}

func composePhotoState(base SnapshotState, plan SourcePlan) (SnapshotState, error) {
	return composePhotoCounts(base, plan.Scope, plan.Epoch, plan.Counts)
}

func (s *PlannedSource) State(ctx context.Context, scope storage.Scope) (SnapshotState, error) {
	return s.state(ctx, scope, true)
}

// RefreshState reads authenticated sidecar metadata without acknowledging or
// deleting a retained source-plan journal. Explicit import keeps recovery in State.
func (s *PlannedSource) RefreshState(ctx context.Context, scope storage.Scope) (SnapshotState, error) {
	return s.state(ctx, scope, false)
}

func (s *PlannedSource) state(ctx context.Context, scope storage.Scope, resume bool) (SnapshotState, error) {
	if scope != s.scope {
		return SnapshotState{}, ErrSourcePlan
	}
	state, err := s.backend.State(ctx, scope)
	if err != nil {
		return state, err
	}
	if state.Version != 1 || state.Validate(scope) != nil {
		return SnapshotState{}, ErrSourcePlan
	}
	if state.Mode == "legacy" {
		return state, nil
	}
	plan, err := s.loadPlan(ctx, scope, state.Epoch, resume)
	if err != nil {
		return SnapshotState{}, err
	}
	return composePhotoCounts(state, plan.scope, plan.epoch, plan.counts)
}

func (s *PlannedSource) Begin(ctx context.Context, scope storage.Scope, epoch string) (SnapshotState, error) {
	if !storage.MigrationEpoch(epoch) {
		return SnapshotState{}, ErrSourcePlan
	}
	routing, err := s.routing(ctx, scope)
	if err != nil {
		return SnapshotState{}, err
	}
	if routing.Plan == nil {
		if err := s.preparePlan(ctx, scope, epoch); err != nil {
			return SnapshotState{}, err
		}
	}
	// Reopen the accepted plan rather than recapturing current Drive heads.
	plan, err := s.load(ctx, scope, epoch)
	if err != nil {
		return SnapshotState{}, err
	}
	if plan.archive != nil {
		if err := plan.archive.validateOrigins(ctx); err != nil {
			return SnapshotState{}, err
		}
	} else if err := s.plans.ValidateOrigins(ctx, *plan.legacy); err != nil {
		return SnapshotState{}, err
	}
	state, err := s.backend.Begin(ctx, scope, epoch)
	if err != nil {
		return state, err
	}
	return composePhotoCounts(state, plan.scope, plan.epoch, plan.counts)
}

func (s *PlannedSource) preparePlan(ctx context.Context, scope storage.Scope, epoch string) error {
	var plan SourcePlan
	var err error
	if paged, ok := s.control.(PagedSourcePlanControl); ok {
		_, err := s.plans.publishArchive(ctx, epoch, s.control, paged)
		return err
	} else {
		preflight, err := s.control.PrivateSourcePreflight(ctx, scope, epoch)
		if err != nil {
			return err
		}
		if err := preflight.Validate(scope, epoch); err != nil {
			return err
		}
		snapshot, err := model.NewSnapshot(scope, preflight.Rows)
		if err != nil {
			return err
		}
		plan, err = PrepareSourcePlan(ctx, scope, epoch, preflight.CAS, snapshot, preflight.Backend, preflight.BackendBlobs, s.plans.cfg.Originals)
		if err != nil {
			return err
		}
	}
	_, err = s.plans.Publish(ctx, plan, s.control)
	return err
}

func sourceRowCompare(a, b storage.StreamSourceRow) int {
	if order := strings.Compare(a.Kind, b.Kind); order != 0 {
		return order
	}
	return strings.Compare(a.ID, b.ID)
}

func afterSourceRow(row storage.StreamSourceRow, after storage.LegacySourceCursor) bool {
	return row.Kind > after.Kind || row.Kind == after.Kind && row.ID > after.ID
}

func planBackendPage(plan SourcePlan, after storage.LegacySourceCursor) ([]storage.StreamSourceRow, storage.LegacySourceCursor) {
	rows := slices.Clone(plan.Backend)
	slices.SortFunc(rows, sourceRowCompare)
	previous := storage.LegacySourceCursor{}
	for i, row := range rows {
		if afterSourceRow(row, after) {
			return rows[i:], previous
		}
		previous = storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}
	}
	return nil, previous
}

func validatePlannedBackendPage(actual storage.StreamSourcePage, scope storage.Scope, epoch string, after storage.LegacySourceCursor, expected []storage.StreamSourceRow) error {
	if actual.Validate(scope, epoch, after) != nil || len(actual.Rows) > len(expected) || len(actual.Rows) == 0 && len(expected) != 0 {
		return ErrSourcePlan
	}
	for i, row := range actual.Rows {
		if row != expected[i] {
			return ErrSourcePlan
		}
	}
	if (actual.Next == nil) != (len(actual.Rows) == len(expected)) {
		return ErrSourcePlan
	}
	return nil
}

func knownPlannedCursor(after storage.LegacySourceCursor, backend, generated []storage.StreamSourceRow) bool {
	if after == (storage.LegacySourceCursor{}) {
		return true
	}
	for _, rows := range [][]storage.StreamSourceRow{backend, generated} {
		for _, row := range rows {
			if row.Kind == after.Kind && row.ID == after.ID {
				return true
			}
		}
	}
	return false
}

func (s *PlannedSource) Page(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor) (SnapshotPage, error) {
	if after != (storage.LegacySourceCursor{}) && !storage.SourceCursorValid(after) {
		return SnapshotPage{}, ErrSourcePlan
	}
	plan, err := s.load(ctx, scope, epoch)
	if err != nil {
		return SnapshotPage{}, err
	}
	if plan.archive != nil {
		return plan.archive.page(ctx, s.backend, after)
	}
	return s.pageLegacy(ctx, scope, epoch, after, *plan.legacy)
}

func (s *PlannedSource) pageLegacy(ctx context.Context, scope storage.Scope, epoch string, after storage.LegacySourceCursor, plan SourcePlan) (SnapshotPage, error) {
	base, err := s.backend.State(ctx, scope)
	if err != nil {
		return SnapshotPage{}, err
	}
	if _, err := composePhotoState(base, plan); err != nil {
		return SnapshotPage{}, err
	}
	generated, _, err := plan.GeneratedRows()
	if err != nil {
		return SnapshotPage{}, err
	}
	if !knownPlannedCursor(after, plan.Backend, generated) {
		return SnapshotPage{}, ErrSourcePlan
	}
	expected, backendCursor := planBackendPage(plan, after)
	backend, err := s.backend.Page(ctx, scope, epoch, backendCursor)
	if err != nil {
		return SnapshotPage{}, err
	}
	if err := validatePlannedBackendPage(backend, scope, epoch, backendCursor, expected); err != nil {
		return SnapshotPage{}, err
	}
	rows := slices.Clone(backend.Rows)
	for _, row := range generated {
		// Do not advance past a backend descriptor not returned by this page.
		// Otherwise an original row could skip remaining media/metadata rows.
		if afterSourceRow(row, after) && (backend.Next == nil || sourceRowCompare(row, expected[len(backend.Rows)]) < 0) {
			rows = append(rows, row)
		}
	}
	slices.SortFunc(rows, sourceRowCompare)
	more := backend.Next != nil || len(rows) > storage.MaxLegacySourceRows
	if len(rows) > storage.MaxLegacySourceRows {
		rows = rows[:storage.MaxLegacySourceRows]
	}
	result := SnapshotPage{Scope: scope, Epoch: epoch, Rows: rows}
	if more && len(rows) > 0 {
		last := rows[len(rows)-1]
		result.Next = &storage.LegacySourceCursor{Kind: last.Kind, ID: last.ID}
	}
	return result, result.Validate(scope, epoch, after)
}

type originalSourceReader struct {
	*io.PipeReader
	cancel context.CancelFunc
	stop   func() bool
}

func (r originalSourceReader) Close() error { r.cancel(); r.stop(); return r.PipeReader.Close() }

func (s *PlannedSource) Open(ctx context.Context, scope storage.Scope, epoch string, row SnapshotRow) (io.ReadCloser, error) {
	if row.Validate() != nil {
		return nil, ErrSourcePlan
	}
	plan, err := s.load(ctx, scope, epoch)
	if err != nil {
		return nil, err
	}
	if plan.archive != nil {
		return plan.archive.open(ctx, s.backend, row)
	}
	return s.openLegacy(ctx, scope, epoch, row, *plan.legacy)
}

func (s *PlannedSource) openLegacy(ctx context.Context, scope storage.Scope, epoch string, row SnapshotRow, plan SourcePlan) (io.ReadCloser, error) {
	base, err := s.backend.State(ctx, scope)
	if err != nil {
		return nil, err
	}
	if _, err := composePhotoState(base, plan); err != nil {
		return nil, err
	}
	generated, payloads, err := plan.GeneratedRows()
	if err != nil {
		return nil, err
	}
	for _, expected := range generated {
		if row.Kind != expected.Kind || row.ID != expected.ID {
			continue
		}
		if row != expected {
			return nil, ErrSourcePlan
		}
		if row.Kind == "media" {
			return io.NopCloser(bytes.NewReader(payloads["media\x00"+row.ID])), nil
		}
		for _, pin := range plan.Originals {
			if pin.Descriptor != row {
				continue
			}
			if s.plans.cfg.Originals == nil {
				return nil, ErrNotReady
			}
			work, cancel := context.WithCancel(ctx)
			reader, writer := io.Pipe()
			stop := context.AfterFunc(work, func() { _ = reader.CloseWithError(work.Err()) })
			go func() { err := s.plans.cfg.Originals.Write(work, scope, pin, writer); _ = writer.CloseWithError(err) }()
			return originalSourceReader{PipeReader: reader, cancel: cancel, stop: stop}, nil
		}
		return nil, ErrSourcePlan
	}
	for _, expected := range plan.Backend {
		if expected == row {
			reader, err := s.backend.Open(ctx, scope, epoch, row)
			if err != nil {
				return nil, err
			}
			if reader == nil {
				return nil, ErrSourcePlan
			}
			return reader, nil
		}
	}
	return nil, ErrSourcePlan
}
