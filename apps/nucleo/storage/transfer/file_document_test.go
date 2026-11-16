// SPDX-License-Identifier: Apache-2.0
package transfer

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type boundedDocumentRules struct{}

func (boundedDocumentRules) SourceComplete(context.Context) error { return nil }
func (boundedDocumentRules) TargetComplete(context.Context) error { return nil }
func (boundedDocumentRules) Classify(context.Context, storage.ObjectVersion, Lookup) (Class, error) {
	return Class{}, nil
}

type rewrittenDocumentRules struct {
	boundedDocumentRules
	payload []byte
}

func (r rewrittenDocumentRules) RewriteFileDocument(context.Context, storage.ObjectVersion, []byte, Resolve) ([]byte, error) {
	return bytes.Clone(r.payload), nil
}

func TestFileDocumentMappingPrecedesUploadAndExistingReferenceMustStillMatchPayload(t *testing.T) {
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}
	version := storage.ObjectVersion{Reference: storage.Reference{RoomID: "!old:fixture", EventID: "$document"}, ObjectID: "document", RevisionID: "index", Commit: storage.ObjectCommit{Content: &storage.ContentPointer{Kind: "file", RevisionID: "body", Reference: storage.Reference{RoomID: "!old:fixture", EventID: "$body"}}}}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	before := []byte(`{"ref":"old","future":9007199254740993}`)
	after := []byte(`{"ref":"new","future":9007199254740993}`)
	request := storage.FileRequest{ObjectID: "document", RevisionID: "body", SizeBytes: int64(len(before)), Metadata: []byte(`{"kind":"document"}`)}
	work := engine{state: Plan{Scope: scope, Salt: "retained_salt", Destination: storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: "!new:fixture", DeviceID: "DEVICE_B"}, Rooms: []string{"!new:fixture"}}}, documents: map[storage.Reference]int64{version.Reference: 1 << 20}}
	work.cfg = Config{Scope: scope, Owner: "fixture\x00owner\x00mail", Namespace: "document-test", Journal: journal, Rules: rewrittenDocumentRules{payload: after}, SourceFileCommitment: func(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error) {
		return request, digest(before), nil
	}, ReadFileDocument: func(context.Context, storage.ObjectVersion, int64) ([]byte, error) { return bytes.Clone(before), nil }}
	var uploaded storage.FileRequest
	uploads := 0
	work.cfg.PutFileDocument = func(_ context.Context, _ storage.PublicationTarget, _ string, request storage.FileRequest, input io.ReaderAt) (storage.Reference, error) {
		var retained Mapping
		found, err := journal.Read(work.cfg.Owner, work.cfg.Namespace, work.transaction(version.Commit.Content.Reference, "file"), &retained)
		if err != nil || !found || retained.Result != nil || !validHash(retained.Input) {
			t.Fatal("upload preceded its durable exact mapping", err)
		}
		raw, err := io.ReadAll(io.NewSectionReader(input, 0, request.SizeBytes))
		if err != nil || !bytes.Equal(raw, after) {
			t.Fatal("upload did not receive rewritten bytes", err)
		}
		clear(raw)
		uploads++
		uploaded = request
		return storage.Reference{RoomID: "!new:fixture", EventID: "$uploaded"}, nil
	}
	tamper := false
	work.cfg.TargetFileCommitment = func(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error) {
		if tamper {
			return uploaded, digest(before), nil
		}
		return uploaded, digest(after), nil
	}
	ref, err := work.copyFileDocument(t.Context(), version)
	if err != nil || !ref.Valid() || uploads != 1 {
		t.Fatal(err)
	}
	tamper = true
	if _, err := work.expectedFileDocumentReference(t.Context(), version); err == nil || uploads != 1 {
		t.Fatal("reference existence substituted for rewritten payload readback", err)
	}
}

func (boundedDocumentRules) Rewrite(context.Context, storage.ObjectVersion, storage.Record, Resolve) ([]byte, error) {
	return nil, ErrSource
}

func (boundedDocumentRules) CheckPending(context.Context, storage.FrozenBinding, storage.Binding) error {
	return nil
}

func (boundedDocumentRules) FileDocumentLimit(context.Context, storage.ObjectVersion) (int64, error) {
	return 1 << 20, nil
}

func (boundedDocumentRules) RewriteFileDocument(context.Context, storage.ObjectVersion, []byte, Resolve) ([]byte, error) {
	return nil, ErrSource
}

func TestFileDocumentRejectsDeclaredOversizeBeforeReadingOrUpload(t *testing.T) {
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}
	version := storage.ObjectVersion{Reference: storage.Reference{RoomID: "!old:fixture", EventID: "$document"}, ObjectID: "document", RevisionID: "index", Commit: storage.ObjectCommit{Content: &storage.ContentPointer{Kind: "file", RevisionID: "body", Reference: storage.Reference{RoomID: "!old:fixture", EventID: "$body"}}}}
	reads := 0
	work := engine{state: Plan{Scope: scope}, documents: map[storage.Reference]int64{version.Reference: 1 << 20}, cfg: Config{Rules: boundedDocumentRules{}, SourceFileCommitment: func(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error) {
		return storage.FileRequest{ObjectID: "document", RevisionID: "body", SizeBytes: (1 << 20) + 1, Metadata: []byte(`{"kind":"document"}`)}, strings.Repeat("a", 64), nil
	}, ReadFileDocument: func(context.Context, storage.ObjectVersion, int64) ([]byte, error) { reads++; return nil, ErrSource }}}
	if _, _, _, err := work.rewrittenFileDocument(t.Context(), version); err == nil || reads != 0 {
		t.Fatal("oversize document reached its plaintext reader", reads, err)
	}
}
