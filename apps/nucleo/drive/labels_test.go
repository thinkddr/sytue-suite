// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func labelGateway(t *testing.T, r *Repository, labels []api.EtiquetaDrive) *atomic.Int32 {
	t.Helper()
	calls := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		scope, err := storage.DecodeScopeHeader(req.Header.Get(storage.ScopeHeader))
		if err != nil || scope != r.cfg.Scope || req.Method != http.MethodGet || req.URL.Path != "/api/v1/sensitivity-labels" || req.Header.Get("Cache-Control") != "no-store" {
			t.Error("private label assignment used an unscoped or content endpoint", req.URL.Path, err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"labels": labels})
	}))
	t.Cleanup(server.Close)
	r.cfg.Gateway = api.NewDrive(server.URL, nil, server.Client())
	return calls
}

func TestPrivateLabelPatchPreservesContentTimeAndUnknownNumbers(t *testing.T) {
	r := queryFixture(t, 1)
	calls := labelGateway(t, r, []api.EtiquetaDrive{{ID: "restricted", Name: "Restricted", Rank: 30, BlockExternalShare: true}})
	ctx, done, err := r.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	raw := []byte(`{"id":"file_001","folder_id":"root","name":"before.txt","size_bytes":0,"version":7,"updated_at":"2026-10-28T10:00:00Z","extension":{"integer":900719925474099512345}}`)
	item, err := summarize(storage.LegacySourceRow{Kind: "file", ID: "file_001", Payload: raw}, "")
	if err != nil {
		t.Fatal(err)
	}
	request := metadataRequest{Operation: "label", Kind: api.DriveFile, ID: item.SourceID, Value: "restricted"}
	updated, err := r.changedMetadata(ctx, item, raw, request)
	if err != nil || !bytes.Contains(updated, []byte("900719925474099512345")) || checkLabelTransition(raw, updated, request.Value) != nil {
		t.Fatal("classification changed content metadata or rounded an extension", string(updated), err)
	}
	file, err := fileView(updated, item.SourceID)
	if err != nil || file.Version != 7 || !file.UpdatedAt.Equal(item.UpdatedAt) || file.Label == nil || !file.Label.BlockExternalShare {
		t.Fatal("classification changed time/version or lost its restriction", file, err)
	}
	if repeated, err := r.changedMetadata(ctx, item, updated, request); err != nil || repeated != nil {
		t.Fatal("unchanged classification generated another revision", err)
	}
	if calls.Load() != 2 {
		t.Fatal("classification did not resolve fresh policy", calls.Load())
	}
	request.Value = ""
	cleared, err := r.changedMetadata(ctx, item, updated, request)
	if err != nil || checkLabelTransition(updated, cleared, "") != nil || calls.Load() != 2 {
		t.Fatal("clear consulted unavailable policy or changed other metadata", err)
	}
	file, err = fileView(cleared, item.SourceID)
	if err != nil || file.Label != nil || file.Version != 7 || !file.UpdatedAt.Equal(item.UpdatedAt) {
		t.Fatal("clear did not remove the snapshot independently of content", file, err)
	}
	for key, value := range map[string]any{"updated_at": time.Now().UTC(), "version": 8, "label_id": "foreign"} {
		changed, err := patchRaw(updated, map[string]any{key: value})
		if err != nil || !errors.Is(checkLabelTransition(raw, changed, "restricted"), ErrSource) {
			t.Fatal("changed classification intent accepted", key, err)
		}
	}
}

func TestPrivateLabelsRejectAmbiguousPolicyAndHiddenTargets(t *testing.T) {
	for _, labels := range [][]api.EtiquetaDrive{
		{{ID: "a", Name: "A"}, {ID: "a", Name: "Other", BlockExternalShare: true}},
		{{ID: "a", Name: ""}},
		{{ID: "a", Name: "A", Rank: -1}},
	} {
		r := queryFixture(t, 1)
		labelGateway(t, r, labels)
		if _, err := r.SensitivityLabels(t.Context()); !errors.Is(err, ErrSource) {
			t.Fatal("invalid policy catalog accepted", labels, err)
		}
	}
	r := queryFixture(t, 1)
	calls := labelGateway(t, r, nil)
	ctx, done, err := r.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	raw := []byte(`{"id":"file_001","folder_id":"root","name":"a","version":1}`)
	item, err := summarize(storage.LegacySourceRow{Kind: "file", ID: "file_001", Payload: raw}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.changedLabel(ctx, item, raw, "foreign"); !errors.Is(err, ErrInvalidLabel) {
		t.Fatal("foreign label accepted", err)
	}
	now := time.Now().UTC()
	item.DeletedAt = &now
	if _, err := r.changedLabel(ctx, item, raw, "foreign"); !errors.Is(err, ErrNotFound) || calls.Load() != 1 {
		t.Fatal("hidden file consulted policy", calls.Load(), err)
	}
}
