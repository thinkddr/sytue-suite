// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// BinaryAudit counts retained body revisions read from the provider. Metadata
// bodies are included. Repeated content is counted for each retained revision.
type BinaryAudit struct {
	Scope   storage.Scope
	Objects int64
	Bytes   int64
}

var ErrAuditBudget = errors.New("drive: retained audit budget exhausted or invalid")

// AuditBudget must be explicitly supplied by the caller. MaxRevisions applies
// to all observed index revisions; MaxBytes counts logical body revisions,
// including external metadata and repeated content, not network overhead or
// metadata rereads. MaxDuration cancels cooperative SDK and provider operations.
// These limits do not bound the memory needed by the initial catalog load.
type AuditBudget struct {
	MaxDuration  time.Duration
	MaxRevisions int
	MaxBytes     int64
}

// AuditBinaries validates the observed private Drive catalog and reads every
// body revision in that history, including older revisions and metadata bodies.
// Reads happen over an interval: they do not prove simultaneous availability.
// Matching catalogs detect observed changes, not an intervening change that is
// later undone; the journal lock does not fence writers on other devices.
// This does not reconcile the legacy source, backups, or pending work, and never
// authorizes removing an original, backup, index, or blob.
func (r *Repository) AuditBinaries(parent context.Context, budget AuditBudget) (BinaryAudit, error) {
	if budget.MaxDuration <= 0 || budget.MaxRevisions <= 0 || budget.MaxBytes <= 0 {
		return BinaryAudit{}, ErrAuditBudget
	}
	parent, cancel := context.WithTimeout(parent, budget.MaxDuration)
	defer cancel()
	ctx, done, err := r.start(parent)
	if err != nil {
		return BinaryAudit{}, err
	}
	defer done()
	var result BinaryAudit
	err = r.cfg.Journal.Exclusive(ctx, r.owner(), migrationNamespace, "initialize", func() error {
		if err := r.requireReady(); err != nil {
			return err
		}
		before, err := storage.LoadCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
		if err != nil {
			return err
		}
		versions, err := r.planBinaryAudit(ctx, before, budget)
		if err != nil {
			return err
		}
		result, err = r.executeBinaryAudit(ctx, versions)
		if err != nil {
			return err
		}
		after, err := storage.LoadCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
		if err != nil {
			return err
		}
		if !before.Binding.Same(after.Binding) || !reflect.DeepEqual(before.Entries, after.Entries) {
			return ErrConflict
		}
		return ctx.Err()
	})
	if err != nil {
		return BinaryAudit{}, err
	}
	return result, nil
}

func (r *Repository) planBinaryAudit(ctx context.Context, before storage.CatalogSnapshot, budget AuditBudget) (map[storage.Reference]storage.ObjectVersion, error) {
	if len(before.Entries) > budget.MaxRevisions {
		return nil, ErrAuditBudget
	}
	seen := make(map[string]bool, len(before.Heads))
	for _, head := range before.Heads {
		if seen[head.ObjectID] {
			return nil, ErrConflict
		}
		seen[head.ObjectID] = true
	}
	versions := make(map[storage.Reference]storage.ObjectVersion, len(before.Entries))
	var plannedBytes int64
	for ref, entry := range before.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		version, err := storage.DecodeObject(entry, r.cfg.Scope)
		if err != nil {
			return nil, err
		}
		var header bodySummary
		if json.Unmarshal(version.Commit.Metadata, &header) != nil {
			return nil, ErrSource
		}
		if header.Kind == bodyKind && version.Commit.State != "deleted" {
			if !validTransferBody(header, version.ObjectID) {
				return nil, ErrSource
			}
			if header.SizeBytes > budget.MaxBytes-plannedBytes {
				return nil, ErrAuditBudget
			}
			plannedBytes += header.SizeBytes
		}
		versions[ref] = version
	}
	return versions, nil
}

func (r *Repository) executeBinaryAudit(ctx context.Context, versions map[storage.Reference]storage.ObjectVersion) (BinaryAudit, error) {
	lookup := func(ref storage.Reference) (storage.ObjectVersion, bool) {
		version, found := versions[ref]
		return version, found
	}
	policy := identityTransfer{source: r}
	result := BinaryAudit{Scope: r.cfg.Scope}
	for _, version := range versions {
		if err := ctx.Err(); err != nil {
			return BinaryAudit{}, err
		}
		if _, err := policy.Classify(ctx, version, lookup); err != nil {
			return BinaryAudit{}, err
		}
		var header struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(version.Commit.Metadata, &header); err != nil {
			return BinaryAudit{}, ErrSource
		}
		if header.Kind != bodyKind || version.Commit.State == "deleted" {
			continue
		}
		size, err := r.auditBody(ctx, version)
		if err != nil {
			return BinaryAudit{}, err
		}
		if size > (1<<63-1)-result.Bytes || result.Objects == 1<<63-1 {
			return BinaryAudit{}, ErrSource
		}
		result.Bytes += size
		result.Objects++
	}
	return result, nil
}

func (r *Repository) auditBody(ctx context.Context, version storage.ObjectVersion) (int64, error) {
	var body bodySummary
	if json.Unmarshal(version.Commit.Metadata, &body) != nil || !validTransferBody(body, version.ObjectID) || version.Commit.Content == nil || version.Commit.Content.Kind != "file" {
		return 0, ErrSource
	}
	pointer := version.Commit.Content
	request, sum, err := r.cfg.Crypto.StorageFileCommitment(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return 0, err
	}
	if request.Validate(r.cfg.Scope) != nil || request.ObjectID != version.ObjectID || request.RevisionID != pointer.RevisionID || request.SizeBytes != body.SizeBytes || len(request.Previous) != 0 || sum != body.SHA256 {
		return 0, ErrSource
	}
	sink := newBodySink(io.Discard, body.SizeBytes)
	actual, err := r.cfg.Crypto.DownloadStorageFile(ctx, r.cfg.Gateway, pointer.Reference, version.ObjectID, pointer.RevisionID, sink)
	if err != nil {
		return 0, err
	}
	if !request.MatchesExact(actual) {
		return 0, ErrSource
	}
	if err := sink.finish(body.SHA256); err != nil {
		return 0, err
	}
	return body.SizeBytes, ctx.Err()
}
