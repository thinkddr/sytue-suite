// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (r *Repository) load(ctx context.Context) (storage.AtomicCatalogSnapshot, map[string][]storage.ObjectVersion, error) {
	snapshot, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		if errors.Is(err, storage.ErrCatalogSource) {
			err = ErrSource
		}
		return storage.AtomicCatalogSnapshot{}, nil, err
	}
	objects := map[string][]storage.ObjectVersion{}
	for _, version := range snapshot.Heads {
		objects[version.ObjectID] = append(objects[version.ObjectID], version)
	}
	return snapshot, objects, nil
}

func (r *Repository) readContent(ctx context.Context, version storage.ObjectVersion) (storage.Record, error) {
	pointer := version.Commit.Content
	if pointer == nil || pointer.Kind != "record" {
		return storage.Record{}, ErrSource
	}
	record, err := r.cfg.Crypto.ReadStorageRecord(ctx, pointer.Reference, version.ObjectID, "content")
	if err != nil {
		return storage.Record{}, err
	}
	if record.Validate(r.cfg.Scope) != nil || record.ObjectID != version.ObjectID || record.RevisionID != pointer.RevisionID || record.Role != "content" {
		return storage.Record{}, ErrSource
	}
	return record, nil
}

func (r *Repository) readEntry(ctx context.Context, version storage.ObjectVersion) (summary, entry, []byte, error) {
	view, err := r.currentObservation(ctx)
	if err != nil {
		return summary{}, entry{}, nil, err
	}
	if err := r.checkObservation(ctx, view); err != nil {
		return summary{}, entry{}, nil, err
	}
	accepted, found, err := storage.AcceptedAtomicObject(view.snapshot, r.cfg.Scope, version.ObjectID, version.RevisionID, &version.Reference)
	if err != nil || !found || !accepted.Commit.MatchesExact(version.Commit) {
		return summary{}, entry{}, nil, ErrSource
	}
	description, err := decodeSummary(version)
	if err != nil {
		return summary{}, entry{}, nil, err
	}
	record, err := r.readContent(ctx, version)
	if err != nil {
		return summary{}, entry{}, nil, err
	}
	var data entry
	if json.Unmarshal(record.Payload, &data) != nil || !data.matches(description) {
		return summary{}, entry{}, nil, ErrSource
	}
	data.observation = view
	raw, err := r.entryBytes(ctx, data, description)
	if err != nil {
		return summary{}, entry{}, nil, err
	}
	if err := verifyProjection(description, version.Commit.State, raw); err != nil {
		clear(raw)
		return summary{}, entry{}, nil, err
	}
	if err := r.checkObservation(ctx, view); err != nil {
		clear(raw)
		return summary{}, entry{}, nil, err
	}
	return description, data, raw, nil
}

func (data entry) matches(item summary) bool {
	return data.Kind == entryKind && data.SourceKind == item.SourceKind && data.SourceID == item.SourceID && (data.Metadata == nil) != (data.Raw == nil) && binaryKind(item.SourceKind) == (data.Binary != nil)
}

func (r *Repository) entryBytes(ctx context.Context, data entry, item summary) ([]byte, error) {
	raw := bytes.Clone(data.Raw)
	if data.Metadata != nil {
		var output bytes.Buffer
		if err := r.readBody(ctx, data.observation, *data.Metadata, item, "metadata", &output); err != nil {
			clear(output.Bytes())
			return nil, err
		}
		raw = output.Bytes()
	}
	if len(raw) > storage.MaxLegacySourceBytes || !json.Valid(raw) || digest(raw) != item.RawSHA256 {
		clear(raw)
		return nil, ErrSource
	}
	return raw, nil
}

func verifyProjection(item summary, state string, raw []byte) error {
	projected, err := summarize(storage.LegacySourceRow{Kind: item.SourceKind, ID: item.SourceID, Payload: raw}, item.Epoch)
	if err != nil {
		return err
	}
	// Baseline and seed status describe lineage, not source JSON fields.
	projected.BaselineSHA256, projected.Seed = item.BaselineSHA256, item.Seed
	actual, _ := json.Marshal(item)
	expected, _ := json.Marshal(projected)
	if !bytes.Equal(actual, expected) {
		return ErrSource
	}
	if item.DeletedAt != nil && state != "trash" || item.DeletedAt == nil && state != "live" {
		return ErrSource
	}
	return nil
}

func (description bodySummary) matches(item summary, purpose string) bool {
	if description.Kind != bodyKind || description.SourceKind != item.SourceKind || description.SourceID != item.SourceID || description.Purpose != purpose || description.Epoch != item.Epoch || !validHash(description.SHA256) || description.SizeBytes < 0 || description.SizeBytes > storage.MaxFileBytes {
		return false
	}
	return purpose != "binary" || description.SizeBytes == item.SizeBytes && (item.Checksum == "" || description.SHA256 == item.Checksum)
}

func (r *Repository) bodyVersion(ctx context.Context, view *observation, pointer bodyPointer, item summary, purpose string) (bodySummary, storage.ObjectVersion, error) {
	if err := r.checkObservation(ctx, view); err != nil {
		return bodySummary{}, storage.ObjectVersion{}, err
	}
	source, err := pointer.source(item, purpose)
	if err != nil {
		return bodySummary{}, storage.ObjectVersion{}, err
	}
	var version storage.ObjectVersion
	if view == nil {
		// Durable publication intents have no observed parent. They retain the
		// existing SDK readback, which rejects prepared indexes fail-closed.
		version, err = r.cfg.Crypto.StorageObject(ctx, pointer.Reference, pointer.ObjectID)
		if err != nil {
			return bodySummary{}, storage.ObjectVersion{}, err
		}
	} else {
		var found bool
		version, found, err = storage.AcceptedAtomicObject(view.snapshot, r.cfg.Scope, pointer.ObjectID, "", &pointer.Reference)
		if err != nil || !found {
			return bodySummary{}, storage.ObjectVersion{}, ErrSource
		}
	}
	if version.ObjectID != pointer.ObjectID || version.Reference != pointer.Reference {
		return bodySummary{}, storage.ObjectVersion{}, ErrSource
	}
	var description bodySummary
	if json.Unmarshal(version.Commit.Metadata, &description) != nil || !description.matches(source, purpose) {
		return bodySummary{}, storage.ObjectVersion{}, ErrSource
	}
	if version.Commit.State != "live" || version.Commit.Content == nil || version.Commit.Content.Kind != "file" {
		return bodySummary{}, storage.ObjectVersion{}, ErrSource
	}
	if err := r.checkObservation(ctx, view); err != nil {
		return bodySummary{}, storage.ObjectVersion{}, err
	}
	return description, version, nil
}

func (r *Repository) readBody(ctx context.Context, view *observation, pointer bodyPointer, item summary, purpose string, out io.Writer) error {
	return r.readBodyWithPin(ctx, view, pointer, item, purpose, out, nil)
}

func (r *Repository) readBodyWithPin(ctx context.Context, view *observation, pointer bodyPointer, item summary, purpose string, out io.Writer, pin *FilePin) error {
	description, version, err := r.bodyVersion(ctx, view, pointer, item, purpose)
	if err != nil {
		return err
	}
	content := version.Commit.Content
	if pin != nil && (purpose != "binary" || pointer.ObjectID != pin.ObjectID || content.RevisionID != pin.ContentRevision || description.SizeBytes != pin.SizeBytes || description.SHA256 != pin.SHA256) {
		return ErrSource
	}
	details, err := r.cfg.Crypto.StorageFileInfo(ctx, content.Reference, pointer.ObjectID, content.RevisionID)
	if err != nil {
		return err
	}
	if details.ObjectID != pointer.ObjectID || details.RevisionID != content.RevisionID || details.SizeBytes != description.SizeBytes {
		return ErrSource
	}
	if purpose == "metadata" && details.SizeBytes > storage.MaxLegacySourceBytes {
		return ErrSource
	}
	sink := newBodySink(out, description.SizeBytes)
	copied, err := r.cfg.Crypto.DownloadStorageFile(ctx, r.cfg.Gateway, content.Reference, pointer.ObjectID, content.RevisionID, sink)
	if err != nil {
		return err
	}
	if copied.ObjectID != details.ObjectID || copied.RevisionID != details.RevisionID || copied.SizeBytes != details.SizeBytes || !bytes.Equal(copied.Metadata, details.Metadata) {
		return ErrSource
	}
	if err := r.checkObservation(ctx, view); err != nil {
		return err
	}
	return sink.finish(description.SHA256)
}
