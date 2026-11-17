// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type recoverablePlanStorage interface {
	RequireRecoverableStorage(context.Context, storage.Scope, storage.PublicationTarget) (context.Context, error)
}
type sourceArchiveBuildWork struct {
	Version      int                       `json:"v"`
	Scope        storage.Scope             `json:"scope"`
	Epoch        string                    `json:"epoch"`
	Target       storage.PublicationTarget `json:"target"`
	Info         model.SourcePreflightInfo `json:"source"`
	Graph        *offline.StagedFile       `json:"graph,omitempty"`
	File         *offline.StagedFile       `json:"file,omitempty"`
	Publication  *sourcePlanWork           `json:"publication,omitempty"`
	Accepted     *SourcePlanReference      `json:"accepted,omitempty"`
	CleanupRows  int64                     `json:"cleanup_rows"`
	CleanupPages int64                     `json:"cleanup_pages"`
	GraphKey     string                    `json:"graph_key,omitempty"`
}

func archiveBuildJob(epoch string) string { return sourcePlanJob(epoch) + "_archive_v2" }

// PublishPagedSourcePlan captures a bounded encrypted inventory and durable
// original pins before accepting the complete authenticated file in the source.
func (s *SourcePlanStore) PublishPagedSourcePlan(ctx context.Context, epoch string, control SourcePlanControl) (SourcePlanReference, error) {
	paged, ok := control.(PagedSourcePlanControl)
	if !ok {
		return SourcePlanReference{}, ErrNotReady
	}
	return s.publishArchive(ctx, epoch, control, paged)
}

func (s *SourcePlanStore) publishArchive(parent context.Context, epoch string, control SourcePlanControl, paged PagedSourcePlanControl) (SourcePlanReference, error) {
	ctx, done, err := s.gate.Start(parent)
	if err != nil {
		return SourcePlanReference{}, err
	}
	defer done()
	if control == nil || paged == nil || !storage.MigrationEpoch(epoch) {
		return SourcePlanReference{}, ErrSourcePlan
	}
	publication, ok := s.cfg.Crypto.(recoverablePlanStorage)
	if !ok {
		return SourcePlanReference{}, ErrNotReady
	}
	var accepted SourcePlanReference
	err = s.cfg.Journal.Exclusive(ctx, s.owner(), "photos-private-v1", sourcePlanJob(epoch), func() error {
		var err error
		accepted, err = s.publishArchiveLocked(ctx, epoch, control, paged, publication)
		return err
	})
	return accepted, err
}

func (s *SourcePlanStore) publishArchiveLocked(ctx context.Context, epoch string, control SourcePlanControl, paged PagedSourcePlanControl, publication recoverablePlanStorage) (SourcePlanReference, error) {
	if ref, found, err := s.resumeLegacyPlan(ctx, epoch, control, publication); err != nil || found {
		return ref, err
	}
	work, err := s.archiveWork(ctx, epoch, paged)
	if err != nil {
		return SourcePlanReference{}, err
	}
	if ref, found, err := s.adoptAcceptedArchive(ctx, &work, control); err != nil {
		return SourcePlanReference{}, err
	} else if found {
		return ref, s.finishAcceptedArchive(ctx, &work, ref)
	}
	protected, err := publication.RequireRecoverableStorage(ctx, s.cfg.Scope, work.Target)
	if err != nil {
		return SourcePlanReference{}, err
	}
	graph, err := s.prepareArchiveGraph(protected, &work, paged)
	if err != nil {
		return SourcePlanReference{}, err
	}
	if err := s.prepareArchivePublication(protected, &work, graph); err != nil {
		return SourcePlanReference{}, err
	}
	ref, err := s.publishPlanWork(protected, work.Publication)
	if err != nil {
		return SourcePlanReference{}, err
	}
	if err := s.saveArchiveWork(work); err != nil {
		return SourcePlanReference{}, err
	}
	if err := s.validateArchiveForPublication(protected, ref, work.Info, true); err != nil {
		return SourcePlanReference{}, err
	}
	// Complete file readback and exact session backup precede the server CAS.
	value, err := control.AcceptSourcePlan(protected, s.cfg.Scope, epoch, work.Info.CAS, ref.Link)
	if err != nil {
		return SourcePlanReference{}, err
	}
	if value != ref {
		return SourcePlanReference{}, ErrSourcePlan
	}
	work.Accepted = &ref
	if err := s.saveArchiveWork(work); err != nil {
		return SourcePlanReference{}, err
	}
	return ref, s.finishArchiveWork(protected, &work, graph, ref)
}

func (s *SourcePlanStore) prepareArchivePublication(ctx context.Context, work *sourceArchiveBuildWork, graph *sourceGraphFile) error {
	if work.File == nil {
		file, err := s.captureArchivePlan(ctx, work, graph)
		if err != nil {
			return err
		}
		work.File = &file
		if err := s.saveArchiveWork(*work); err != nil {
			return err
		}
	}
	if work.Publication == nil {
		work.Publication = &sourcePlanWork{Version: 1, Scope: work.Scope, Epoch: work.Epoch, Target: work.Target, File: *work.File, Format: sourceArchiveKind, Job: archiveBuildJob(work.Epoch) + "_publication"}
		return s.saveArchiveWork(*work)
	}
	return nil
}

func (s *SourcePlanStore) validateArchiveForPublication(ctx context.Context, ref SourcePlanReference, info model.SourcePreflightInfo, forceAvailability bool) error {
	archive, err := s.openArchive(ctx, ref, forceAvailability)
	if err != nil {
		return err
	}
	header, err := archive.header(ctx)
	if err != nil {
		return err
	}
	if header.Info != info {
		return ErrSourcePlan
	}
	return archive.validateOrigins(ctx)
}

func (s *SourcePlanStore) finishAcceptedArchive(ctx context.Context, work *sourceArchiveBuildWork, ref SourcePlanReference) error {
	var graph *sourceGraphFile
	if work.CleanupRows < work.Info.Rows {
		if work.Graph == nil {
			return ErrSourcePlan
		}
		var err error
		graph, err = s.openSourceGraph(ctx, work.Info, *work.Graph)
		if err != nil {
			return err
		}
	}
	return s.finishArchiveWork(ctx, work, graph, ref)
}

func (s *SourcePlanStore) saveArchiveWork(work sourceArchiveBuildWork) error {
	return s.cfg.Journal.Write(s.owner(), "photos-private-v1", archiveBuildJob(work.Epoch), work)
}

func (s *SourcePlanStore) archiveWork(ctx context.Context, epoch string, paged PagedSourcePlanControl) (sourceArchiveBuildWork, error) {
	var work sourceArchiveBuildWork
	found, err := s.cfg.Journal.Read(s.owner(), "photos-private-v1", archiveBuildJob(epoch), &work)
	if err != nil {
		return work, err
	}
	if found {
		if err := s.validateArchiveWork(work, epoch); err != nil {
			return work, err
		}
		return work, s.cfg.Crypto.CheckStoragePublicationTarget(work.Target)
	}
	info, err := paged.StartPrivateSourcePreflight(ctx, s.cfg.Scope, epoch)
	if err != nil {
		return work, err
	}
	if err := info.Validate(s.cfg.Scope, epoch); err != nil {
		return work, err
	}
	if info.Originals > 0 && s.cfg.Originals == nil {
		return work, ErrNotReady
	}
	binding, err := s.cfg.Crypto.PrepareStorage(ctx, s.cfg.Scope)
	if err != nil {
		return work, err
	}
	if !binding.Valid() || binding.Scope != s.cfg.Scope {
		return work, ErrSourcePlan
	}
	work = sourceArchiveBuildWork{Version: 2, Scope: s.cfg.Scope, Epoch: epoch, Target: binding.Target, Info: info}
	return work, s.saveArchiveWork(work)
}

func (s *SourcePlanStore) resumeAcceptedArchive(parent context.Context, ref SourcePlanReference, control SourcePlanControl) error {
	ctx, done, err := s.gate.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	var work sourceArchiveBuildWork
	found, err := s.cfg.Journal.Read(s.owner(), "photos-private-v1", archiveBuildJob(ref.Epoch), &work)
	if err != nil || !found {
		return err
	}
	if work.Scope != s.cfg.Scope || work.Epoch != ref.Epoch || work.Publication == nil || work.Publication.Link == nil || *work.Publication.Link != ref.Link {
		return ErrSourcePlan
	}
	paged, ok := control.(PagedSourcePlanControl)
	if !ok {
		return ErrNotReady
	}
	_, err = s.publishArchive(ctx, ref.Epoch, control, paged)
	return err
}

func (s *SourcePlanStore) prepareArchiveGraph(ctx context.Context, work *sourceArchiveBuildWork, paged PagedSourcePlanControl) (*sourceGraphFile, error) {
	if work.Graph == nil {
		file, err := s.captureSourceGraph(ctx, work.Info, paged)
		if err != nil {
			return nil, err
		}
		work.Graph = &file
		work.GraphKey = sourceGraphFileKey(work.Info, file)
		if err := s.saveArchiveWork(*work); err != nil {
			return nil, err
		}
	}
	graph, err := s.openSourceGraph(ctx, work.Info, *work.Graph)
	if err != nil {
		return nil, err
	}
	if err := graph.validate(ctx); err != nil {
		return nil, err
	}
	return graph, nil
}

func sourceGraphPinKey(graph *sourceGraphFile, id string) string {
	return graph.source.key + "_pin_" + planBytesHash([]byte(id))
}

func (s *SourcePlanStore) archiveOriginalPin(ctx context.Context, graph *sourceGraphFile, row model.Row) (PlannedOriginal, error) {
	var asset model.AssetRow
	if row.Kind != "asset" || json.Unmarshal(row.JSON, &asset) != nil || asset.Owner != s.cfg.Scope.Subject {
		return PlannedOriginal{}, ErrSourcePlan
	}
	original := model.Original{AssetID: asset.ID, DriveID: asset.DriveID, Size: asset.Bytes, ContentType: asset.MediaType}
	if s.cfg.Originals == nil {
		return PlannedOriginal{}, ErrNotReady
	}
	key := sourceGraphPinKey(graph, asset.ID)
	var pin PlannedOriginal
	found, err := s.cfg.Journal.Read(s.owner(), sourceArchiveNamespace, key, &pin)
	if err != nil {
		return pin, err
	}
	if !found {
		pin, err = s.cfg.Originals.Capture(ctx, s.cfg.Scope, original)
		if err != nil {
			return pin, err
		}
	}
	if pin.Original != original {
		clear(pin.Pin)
		return PlannedOriginal{}, ErrSourcePlan
	}
	if _, err := originalDrivePin(s.cfg.Scope, pin); err != nil {
		clear(pin.Pin)
		return PlannedOriginal{}, err
	}
	if err := s.cfg.Originals.Validate(ctx, s.cfg.Scope, pin); err != nil {
		clear(pin.Pin)
		return PlannedOriginal{}, err
	}
	if !found {
		if err := s.cfg.Journal.Write(s.owner(), sourceArchiveNamespace, key, pin); err != nil {
			clear(pin.Pin)
			return PlannedOriginal{}, err
		}
	}
	return pin, nil
}

func (s *SourcePlanStore) captureArchivePlan(ctx context.Context, work *sourceArchiveBuildWork, graph *sourceGraphFile) (offline.StagedFile, error) {
	reader, writer := io.Pipe()
	operation, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(operation, func() { _ = reader.CloseWithError(operation.Err()) })
	defer stop()
	finished := make(chan error, 1)
	go func() {
		var ordinal int64
		var previous []byte
		defer func() { clear(previous) }()
		next := func() (sourceArchiveEntry, bool, error) {
			clear(previous)
			previous = nil
			if ordinal == work.Info.Rows {
				return sourceArchiveEntry{}, false, nil
			}
			entry, err := graph.entry(operation, ordinal)
			if err != nil {
				return sourceArchiveEntry{}, false, err
			}
			ordinal++
			result := sourceArchiveEntry{Backend: entry.Backend}
			if entry.Graph != nil {
				defer clear(entry.Graph.JSON)
			}
			if entry.Backend.Kind == "asset" {
				pin, err := s.archiveOriginalPin(operation, graph, *entry.Graph)
				if err != nil {
					return sourceArchiveEntry{}, false, err
				}
				result.Original = &pin
				previous = pin.Pin
			}
			return result, true, nil
		}
		header := sourceArchiveHeader{Kind: sourceArchiveKind, Info: work.Info, Counts: SourcePlanCounts{BackendRows: work.Info.Rows, BackendBlobs: work.Info.Blobs, OriginalRows: 2 * work.Info.Originals, OriginalBlobs: work.Info.Originals}}
		err := encodeSourceArchive(operation, writer, header, next)
		_ = writer.CloseWithError(err)
		finished <- err
	}()
	file, err := s.cfg.Journal.StageStream(operation, s.owner(), sourceArchiveNamespace, reader, storage.MaxFileBytes)
	_ = reader.Close()
	cancel()
	producerErr := <-finished
	if err != nil {
		return offline.StagedFile{}, err
	}
	if producerErr != nil {
		return offline.StagedFile{}, producerErr
	}
	return file, nil
}

func (s *SourcePlanStore) adoptAcceptedArchive(ctx context.Context, work *sourceArchiveBuildWork, control SourcePlanControl) (SourcePlanReference, bool, error) {
	routing, err := control.PrivateSourcePlanRouting(ctx, s.cfg.Scope)
	if err != nil {
		return SourcePlanReference{}, false, err
	}
	if routing.Validate(s.cfg.Scope) != nil {
		return SourcePlanReference{}, false, ErrSourcePlan
	}
	if routing.Plan == nil {
		if work.Accepted != nil {
			return SourcePlanReference{}, false, ErrSourcePlan
		}
		return SourcePlanReference{}, false, nil
	}
	if routing.Plan.Epoch != work.Epoch || work.Publication == nil || work.Publication.Link == nil || *work.Publication.Link != routing.Plan.Link {
		return SourcePlanReference{}, false, ErrSourcePlan
	}
	if work.Accepted != nil && *work.Accepted != *routing.Plan {
		return SourcePlanReference{}, false, ErrSourcePlan
	}
	archive, err := s.openArchive(ctx, *routing.Plan, true)
	if err != nil {
		return SourcePlanReference{}, false, err
	}
	header, err := archive.header(ctx)
	if err != nil {
		return SourcePlanReference{}, false, err
	}
	if header.Info != work.Info {
		return SourcePlanReference{}, false, ErrSourcePlan
	}
	work.Accepted = routing.Plan
	if err := s.saveArchiveWork(*work); err != nil {
		return SourcePlanReference{}, false, err
	}
	return *routing.Plan, true, nil
}

func (s *SourcePlanStore) finishArchiveWork(ctx context.Context, work *sourceArchiveBuildWork, graph *sourceGraphFile, ref SourcePlanReference) error {
	if work.Accepted == nil || *work.Accepted != ref || work.GraphKey == "" {
		return ErrSourcePlan
	}
	if err := s.validateArchiveForPublication(ctx, ref, work.Info, false); err != nil {
		return err
	}
	if err := s.cfg.Crypto.AcknowledgeStorageFileLink(ctx, s.cfg.Journal, sourcePlanJob(work.Epoch)+"_file", ref.Link); err != nil && !errors.Is(err, storage.ErrUploadMissing) {
		return err
	}
	if err := s.cleanupArchiveRows(ctx, work, graph); err != nil {
		return err
	}
	for page := work.CleanupPages; page < (work.Info.Rows+sourceArchiveIndexRows-1)/sourceArchiveIndexRows; page++ {
		if err := s.cfg.Journal.Delete(s.owner(), sourceArchiveNamespace, archiveIndexKey(work.GraphKey, page)); err != nil {
			return err
		}
		work.CleanupPages = page + 1
		if err := s.saveArchiveWork(*work); err != nil {
			return err
		}
	}
	if work.Graph != nil {
		if err := s.cfg.Journal.DeleteStagedFile(s.owner(), sourceArchiveNamespace, *work.Graph); err != nil {
			return err
		}
		work.Graph = nil
		if err := s.saveArchiveWork(*work); err != nil {
			return err
		}
	}
	if work.File != nil {
		if err := s.cfg.Journal.DeleteStagedFile(s.owner(), sourceArchiveNamespace, *work.File); err != nil {
			return err
		}
		work.File = nil
		if err := s.saveArchiveWork(*work); err != nil {
			return err
		}
	}
	if work.Publication != nil {
		if err := s.cfg.Journal.Delete(s.owner(), "photos-private-v1", work.Publication.journalKey()); err != nil {
			return err
		}
	}
	return s.cfg.Journal.Delete(s.owner(), "photos-private-v1", archiveBuildJob(work.Epoch))
}

// An app update must finish the already captured v1 intent, rather than capture
// today's Drive heads under the same publication transaction IDs.
func (s *SourcePlanStore) resumeLegacyPlan(ctx context.Context, epoch string, control SourcePlanControl, publication recoverablePlanStorage) (SourcePlanReference, bool, error) {
	var work sourcePlanWork
	found, err := s.cfg.Journal.Read(s.owner(), "photos-private-v1", sourcePlanJob(epoch), &work)
	if err != nil || !found {
		return SourcePlanReference{}, false, err
	}
	if work.Version != 1 || work.Scope != s.cfg.Scope || work.Epoch != epoch || work.Format != "" || work.Job != "" || !work.Target.Valid() || work.File.SizeBytes > maxSourcePlanBytes {
		return SourcePlanReference{}, true, ErrSourcePlan
	}
	if err := s.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return SourcePlanReference{}, true, err
	}
	protected, err := publication.RequireRecoverableStorage(ctx, s.cfg.Scope, work.Target)
	if err != nil {
		return SourcePlanReference{}, true, err
	}
	reader, err := s.cfg.Journal.OpenStagedFile(protected, s.owner(), sourceArchiveNamespace, work.File)
	if err != nil {
		return SourcePlanReference{}, true, err
	}
	defer reader.Close()
	raw, err := io.ReadAll(io.NewSectionReader(reader, 0, work.File.SizeBytes))
	if err != nil {
		return SourcePlanReference{}, true, err
	}
	defer clear(raw)
	var plan SourcePlan
	if json.Unmarshal(raw, &plan) != nil || plan.Scope != s.cfg.Scope || plan.Epoch != epoch || plan.Validate(protected, s.cfg.Originals) != nil {
		return SourcePlanReference{}, true, ErrSourcePlan
	}
	ref, err := s.publish(protected, plan, work.Target, raw, control)
	return ref, true, err
}

func (s *SourcePlanStore) validateArchiveWork(work sourceArchiveBuildWork, epoch string) error {
	if work.Version != 2 || work.Scope != s.cfg.Scope || work.Epoch != epoch || !work.Target.Valid() || work.Info.Validate(s.cfg.Scope, epoch) != nil || work.CleanupRows < 0 || work.CleanupRows > work.Info.Rows || work.CleanupPages < 0 || work.CleanupPages > (work.Info.Rows+sourceArchiveIndexRows-1)/sourceArchiveIndexRows {
		return ErrSourcePlan
	}
	if work.Graph != nil && work.GraphKey != sourceGraphFileKey(work.Info, *work.Graph) {
		return ErrSourcePlan
	}
	if work.Publication != nil {
		return validateArchivePublication(work)
	}
	return nil
}

func validateArchivePublication(work sourceArchiveBuildWork) error {
	p := work.Publication
	if p.Version != 1 || p.Scope != work.Scope || p.Epoch != work.Epoch || p.Target != work.Target || p.Format != sourceArchiveKind || p.Job != archiveBuildJob(work.Epoch)+"_publication" || work.File != nil && p.File != *work.File {
		return ErrSourcePlan
	}
	return nil
}

func (s *SourcePlanStore) cleanupArchiveRows(ctx context.Context, work *sourceArchiveBuildWork, graph *sourceGraphFile) error {
	// Each temporary graph row is reachable by ordinal. Cleanup stays bounded
	// and idempotent after a crash; private cache/index pages remain available.
	for ordinal := work.CleanupRows; ordinal < work.Info.Rows; ordinal++ {
		if graph == nil {
			return ErrSourcePlan
		}
		entry, err := graph.entry(ctx, ordinal)
		if err != nil {
			return err
		}
		if err := s.cleanupArchiveRow(graph, entry); err != nil {
			return err
		}
		work.CleanupRows = ordinal + 1
		if work.CleanupRows%sourceArchiveIndexRows == 0 || work.CleanupRows == work.Info.Rows {
			if err := s.saveArchiveWork(*work); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SourcePlanStore) cleanupArchiveRow(graph *sourceGraphFile, entry sourceGraphEntry) error {
	if entry.Graph != nil {
		var fields struct {
			Drive string `json:"drive_file_id"`
			Asset string `json:"asset_id"`
			Owner string `json:"owner_sub"`
		}
		if json.Unmarshal(entry.Graph.JSON, &fields) != nil {
			clear(entry.Graph.JSON)
			return ErrSourcePlan
		}
		clear(entry.Graph.JSON)
		if entry.Backend.Kind == "asset" {
			for _, name := range []string{sourceGraphPinKey(graph, entry.Backend.ID), graph.source.key + "_seen_" + planBytesHash([]byte("drive\x00"+fields.Drive))} {
				if err := s.cfg.Journal.Delete(s.owner(), sourceArchiveNamespace, name); err != nil {
					return err
				}
			}
		}
		if entry.Backend.Kind == "edit" {
			if err := s.cfg.Journal.Delete(s.owner(), sourceArchiveNamespace, graph.source.key+"_seen_"+planBytesHash([]byte("edit\x00"+fields.Asset+"\x00"+fields.Owner))); err != nil {
				return err
			}
		}
	}
	return nil
}
