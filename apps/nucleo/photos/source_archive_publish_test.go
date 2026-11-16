// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type archivePublisherFixture struct {
	*archiveCryptoFixture
	target                       storage.PublicationTarget
	request                      storage.FileRequest
	uploads, links, acks, grants int
	ackErr, targetErr            error
	afterDownload                func()
}

func (p *archivePublisherFixture) PrepareStorage(context.Context, storage.Scope) (storage.Binding, error) {
	return storage.Binding{Scope: p.scope, Target: p.target, Rooms: []string{p.target.RoomID}}, nil
}

func (p *archivePublisherFixture) CheckStoragePublicationTarget(storage.PublicationTarget) error {
	return p.targetErr
}

func (p *archivePublisherFixture) RequireRecoverableStorage(ctx context.Context, _ storage.Scope, target storage.PublicationTarget) (context.Context, error) {
	p.grants++
	if target != p.target {
		return nil, ErrSourcePlan
	}
	if p.targetErr != nil {
		return nil, p.targetErr
	}
	return ctx, nil
}

func (p *archivePublisherFixture) UploadStorageFileAt(_ context.Context, _ *api.Drive, _ *offline.Store, _ string, target storage.PublicationTarget, request storage.FileRequest, reader io.ReaderAt) (storage.Reference, error) {
	if target != p.target {
		return storage.Reference{}, ErrSourcePlan
	}
	p.uploads++
	raw, err := io.ReadAll(io.NewSectionReader(reader, 0, request.SizeBytes))
	if err != nil {
		return storage.Reference{}, err
	}
	p.raw = raw
	p.request = request
	return p.root, nil
}

func (p *archivePublisherFixture) PutStorageRecordAt(_ context.Context, target storage.PublicationTarget, _ string, record storage.Record) (storage.Reference, error) {
	if target != p.target || record.Scope != p.scope {
		return storage.Reference{}, ErrSourcePlan
	}
	p.links++
	return p.ref.Link, nil
}

func (p *archivePublisherFixture) StorageFileInfo(context.Context, storage.Reference, string, string) (storage.FileRequest, error) {
	return p.request, p.infoErr
}

func (p *archivePublisherFixture) StorageFileCommitment(ctx context.Context, root storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	request, err := p.StorageFileInfo(ctx, root, object, revision)
	return request, planBytesHash(p.raw), err
}

func (p *archivePublisherFixture) DownloadStorageFile(ctx context.Context, gateway *api.Drive, root storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	p.downloads++
	if p.downloadErr != nil {
		return storage.FileRequest{}, p.downloadErr
	}
	if err := ctx.Err(); err != nil {
		return storage.FileRequest{}, err
	}
	_, err := out.Write(p.raw)
	if p.afterDownload != nil {
		p.afterDownload()
	}
	return p.request, err
}

func (p *archivePublisherFixture) AcknowledgeStorageFileLink(context.Context, *offline.Store, string, storage.Reference) error {
	p.acks++
	return p.ackErr
}

type archiveControlFixture struct {
	SourcePlanControl
	*pagedPreflightFixture
	routing     model.SourcePlanRouting
	accepts     int
	lostAck     bool
	afterAccept func()
	reject      error
}

func (c *archiveControlFixture) PrivateSourcePlanRouting(context.Context, storage.Scope) (model.SourcePlanRouting, error) {
	return c.routing, nil
}

func (c *archiveControlFixture) AcceptSourcePlan(_ context.Context, scope storage.Scope, epoch, cas string, ref storage.Reference) (SourcePlanReference, error) {
	c.accepts++
	if c.reject != nil {
		return SourcePlanReference{}, c.reject
	}
	if scope != c.info.Scope || epoch != c.info.Epoch || cas != c.info.CAS {
		return SourcePlanReference{}, ErrSourcePlan
	}
	result := SourcePlanReference{Epoch: epoch, Link: ref}
	c.routing.Plan = &result
	if c.afterAccept != nil {
		c.afterAccept()
	}
	if c.lostAck {
		return SourcePlanReference{}, io.ErrUnexpectedEOF
	}
	return result, nil
}

func archivePublishFixture(t *testing.T) (*SourcePlanStore, *archivePublisherFixture, *archiveControlFixture, *archivePinAuthority) {
	t.Helper()
	store, crypto, _ := sourceArchiveCacheFixture(t)
	base, snapshot, _ := sourcePlanFixture(t)
	crypto.ref.Epoch = base.Epoch
	control := &archiveControlFixture{pagedPreflightFixture: pagedPreflightFixtureForRows(t, base.Scope, base.Epoch, snapshot.Rows()), routing: model.SourcePlanRouting{Scope: base.Scope}}
	publisher := &archivePublisherFixture{archiveCryptoFixture: crypto, target: storage.PublicationTarget{RoomID: crypto.ref.Link.RoomID, DeviceID: "DEVICE"}}
	authority := new(archivePinAuthority)
	store.cfg.Crypto = publisher
	store.cfg.Originals = authority
	return store, publisher, control, authority
}

func TestSourceArchivePublicationResumesLostAcceptAndAckWithoutRecapture(t *testing.T) {
	for _, lost := range []string{"accept response", "upload ack"} {
		t.Run(lost, func(t *testing.T) {
			store, publisher, control, pins := archivePublishFixture(t)
			if lost == "accept response" {
				control.lostAck = true
			} else {
				publisher.ackErr = io.ErrUnexpectedEOF
			}
			if _, err := store.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control); err == nil {
				t.Fatal("fixture interruption not observed")
			}
			if control.routing.Plan == nil || control.accepts != 1 || pins.captures != 1 {
				t.Fatal("CAS was not durably accepted", control.accepts, pins.captures)
			}
			first := *control.routing.Plan
			// Restart uses the same encrypted journal but a new plan store/work gate.
			restarted, err := NewSourcePlanStore(store.cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(restarted.Close)
			publisher.ackErr = nil
			control.lostAck = false
			got, err := restarted.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control)
			if err != nil || got != first || control.accepts != 1 || pins.captures != 1 || publisher.uploads != 1 || publisher.links != 1 {
				t.Fatal("resume recaptured or republished accepted intent", err, control.accepts, pins.captures, publisher.uploads, publisher.links)
			}
			var pending sourceArchiveBuildWork
			if found, err := store.cfg.Journal.Read(store.owner(), "photos-private-v1", archiveBuildJob(control.info.Epoch), &pending); err != nil || found {
				t.Fatal("acknowledged build retained pending intent", found, err)
			}
		})
	}
}

func TestSourceArchivePublicationNeverAcceptsUnavailableIncompleteOrRevokedPlan(t *testing.T) {
	for _, failure := range []string{"missing page", "SDK download", "revoked pin", "stale target", "CAS changed"} {
		t.Run(failure, func(t *testing.T) {
			store, publisher, control, pins := archivePublishFixture(t)
			switch failure {
			case "missing page":
				// More than one descriptor page is necessary to observe truncation.
				rows := make([]model.Row, 0, 3)
				for range 3 {
					id := uuid.NewString()
					original := control.rows[0]
					raw := control.payloads[original.Kind+"\x00"+original.ID]
					raw = bytes.ReplaceAll(raw, []byte(original.ID), []byte(id))
					raw = bytes.ReplaceAll(raw, []byte("retained-drive-id"), []byte("retained-"+id))
					rows = append(rows, model.Row{Kind: "asset", ID: id, JSON: raw})
				}
				control.pagedPreflightFixture = pagedPreflightFixtureForRows(t, control.info.Scope, control.info.Epoch, rows)
				control.change = "missing page"
			case "SDK download":
				publisher.downloadErr = io.ErrUnexpectedEOF
			case "revoked pin":
				publisher.afterDownload = func() { pins.denied = true }
			case "stale target":
				publisher.targetErr = ErrNotReady
			case "CAS changed":
				control.reject = ErrSourcePlan
			}
			_, err := store.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control)
			if err == nil || control.routing.Plan != nil || publisher.acks != 0 {
				t.Fatal("failure accepted or retired source", failure, err, publisher.acks)
			}
			if failure != "CAS changed" && control.accepts != 0 {
				t.Fatal("invalid plan reached CAS", failure, control.accepts)
			}
			if failure == "missing page" && pins.captures != 0 {
				t.Fatal("partial graph reached pin capture")
			}
			if failure == "stale target" && !errors.Is(err, ErrNotReady) {
				t.Fatal("target error hidden", err)
			}
		})
	}
}

func TestSourceArchiveUpgradeFinishesExistingV1CaptureWithoutRepinning(t *testing.T) {
	store, publisher, control, pins := archivePublishFixture(t)
	plan, _, _ := sourcePlanFixture(t)
	plan.Epoch = control.info.Epoch
	plan.CAS = control.info.CAS
	original := plan.Originals[0].Original
	pin, err := pins.Capture(t.Context(), plan.Scope, original)
	if err != nil {
		t.Fatal(err)
	}
	plan.Originals[0] = pin
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.planWork(t.Context(), plan, publisher.target, raw); err != nil {
		t.Fatal(err)
	}
	ref, err := store.PublishPagedSourcePlan(t.Context(), plan.Epoch, control)
	if err != nil || !ref.Valid() || pins.captures != 1 || publisher.uploads != 1 || publisher.links != 1 || !bytes.Equal(publisher.request.Metadata, sourcePlanMetadata(sourcePlanKind)) {
		t.Fatal("upgrade replaced existing capture with a v2 intent", err, pins.captures, publisher.uploads, publisher.links)
	}
	if !bytes.Equal(raw, publisher.raw) {
		t.Fatal("upgrade normalized retained original plan bytes")
	}
}

func TestSourceArchiveRetryUsesCapturedPinsAfterReadbackFailure(t *testing.T) {
	store, publisher, control, pins := archivePublishFixture(t)
	publisher.downloadErr = io.ErrUnexpectedEOF
	if _, err := store.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control); err == nil || control.accepts != 0 || pins.captures != 1 {
		t.Fatal("readback failed without retaining exact intent", err, control.accepts, pins.captures)
	}
	publisher.downloadErr = nil
	if _, err := store.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control); err != nil || control.accepts != 1 || pins.captures != 1 || publisher.uploads != 1 || publisher.links != 1 {
		t.Fatal("retry selected a later original or republished a file", err, control.accepts, pins.captures, publisher.uploads, publisher.links)
	}
}

func TestSourceArchiveCleanupResumesAtDurableCrashBoundaries(t *testing.T) {
	for _, phase := range []string{"row deletion before checkpoint", "index retired", "graph retired"} {
		t.Run(phase, func(t *testing.T) {
			store, publisher, control, pins := archivePublishFixture(t)
			publisher.ackErr = io.ErrUnexpectedEOF
			if _, err := store.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control); err == nil {
				t.Fatal("expected pending ACK")
			}
			var work sourceArchiveBuildWork
			if found, err := store.cfg.Journal.Read(store.owner(), "photos-private-v1", archiveBuildJob(control.info.Epoch), &work); err != nil || !found || work.Accepted == nil || work.Graph == nil {
				t.Fatal("missing durable accepted work", err)
			}
			graph, err := store.openSourceGraph(t.Context(), work.Info, *work.Graph)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "row deletion before checkpoint" {
				entry, err := graph.entry(t.Context(), 0)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.cleanupArchiveRow(graph, entry); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := store.cleanupArchiveRows(t.Context(), &work, graph); err != nil {
					t.Fatal(err)
				}
				if err := store.cfg.Journal.Delete(store.owner(), sourceArchiveNamespace, archiveIndexKey(work.GraphKey, 0)); err != nil {
					t.Fatal(err)
				}
				work.CleanupPages = 1
				if phase == "graph retired" {
					if err := store.cfg.Journal.DeleteStagedFile(store.owner(), sourceArchiveNamespace, *work.Graph); err != nil {
						t.Fatal(err)
					}
					work.Graph = nil
				}
				if err := store.saveArchiveWork(work); err != nil {
					t.Fatal(err)
				}
			}
			publisher.ackErr = nil
			if ref, err := store.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control); err != nil || ref != *work.Accepted || control.accepts != 1 || pins.captures != 1 || publisher.uploads != 1 {
				t.Fatal("cleanup crash lost accepted intent", phase, err, control.accepts, pins.captures, publisher.uploads)
			}
		})
	}
}

func TestSourceArchiveDoesNotAcknowledgeAfterOriginalAuthorizationChanges(t *testing.T) {
	store, publisher, control, pins := archivePublishFixture(t)
	control.afterAccept = func() { pins.denied = true }
	if _, err := store.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control); err == nil || control.routing.Plan == nil || publisher.acks != 0 {
		t.Fatal("changed original authorization retired accepted work", err, publisher.acks)
	}
	pins.denied = false
	control.afterAccept = nil
	if _, err := store.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control); err != nil || control.accepts != 1 || pins.captures != 1 || publisher.acks != 1 {
		t.Fatal("restored authorization could not finish exact accepted work", err, control.accepts, pins.captures, publisher.acks)
	}
}

func TestSourceRefreshStateDoesNotAcknowledgeAcceptedSidecarWork(t *testing.T) {
	store, publisher, control, _ := archivePublishFixture(t)
	control.lostAck = true
	if _, err := store.PublishPagedSourcePlan(t.Context(), control.info.Epoch, control); err == nil || control.routing.Plan == nil {
		t.Fatal("accepted sidecar ACK loss was not retained", err)
	}
	control.lostAck = false
	var before sourceArchiveBuildWork
	if found, err := store.cfg.Journal.Read(store.owner(), "photos-private-v1", archiveBuildJob(control.info.Epoch), &before); err != nil || !found {
		t.Fatal("accepted sidecar has no retry work", found, err)
	}
	archive, err := store.openArchive(t.Context(), *control.routing.Plan, false)
	if err != nil {
		t.Fatal(err)
	}
	header, err := archive.header(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewPlannedSource(store.cfg.Scope, archiveBackendFor(t, header), control, store)
	if err != nil {
		t.Fatal(err)
	}
	state, err := source.RefreshState(t.Context(), store.cfg.Scope)
	if err != nil || state.Epoch != control.info.Epoch {
		t.Fatal("metadata refresh failed", err)
	}
	var after sourceArchiveBuildWork
	if found, err := store.cfg.Journal.Read(store.owner(), "photos-private-v1", archiveBuildJob(control.info.Epoch), &after); err != nil || !found {
		t.Fatal("refresh deleted accepted sidecar retry work", found, err)
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if !bytes.Equal(beforeJSON, afterJSON) || publisher.acks != 0 || control.accepts != 1 {
		t.Fatal("refresh acknowledged or rewrote sidecar work", publisher.acks, control.accepts)
	}
	if _, err := source.State(t.Context(), store.cfg.Scope); err != nil {
		t.Fatal("explicit recovery metadata could not resume sidecar", err)
	}
	if found, err := store.cfg.Journal.Read(store.owner(), "photos-private-v1", archiveBuildJob(control.info.Epoch), &after); err != nil || found || publisher.acks != 1 {
		t.Fatal("ordinary recovery did not acknowledge same retained sidecar", found, err, publisher.acks)
	}
}
