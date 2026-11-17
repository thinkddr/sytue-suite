// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Public signed-record/file-commitment metadata witnesses only. Actual SDK
// provenance remains in the integration fixture, never in this test reader.
type atomicCommitmentPort struct {
	scope    Scope
	requests map[Reference]FileRequest
	digests  map[Reference]string
	calls    int
	hook     func()
}

func (p *atomicCommitmentPort) FileScope() Scope { return p.scope }
func (p *atomicCommitmentPort) Commitment(ctx context.Context, ref Reference, object, revision string) (FileRequest, string, error) {
	p.calls++
	if p.hook != nil {
		p.hook()
	}
	if err := ctx.Err(); err != nil {
		return FileRequest{}, "", err
	}
	request, found := p.requests[ref]
	if !found || request.ObjectID != object || request.RevisionID != revision {
		return FileRequest{}, "", ErrRecord
	}
	return request, p.digests[ref], nil
}

func atomicFileProofFixture(t *testing.T) (*atomicCatalogPort, *atomicCommitmentPort, Reference, Reference, FileRequest) {
	t.Helper()
	p := atomicTestPort(t, 2, true)
	files := &atomicCommitmentPort{scope: catalogScope, requests: map[Reference]FileRequest{}, digests: map[Reference]string{}}
	for i, suffix := range []string{"0", "1"} {
		entry := atomicEntry(p, "$prepared"+suffix)
		var value AtomicPrepared
		_ = json.Unmarshal(entry.Record.Payload, &value)
		ref := Reference{RoomID: "!private:test", EventID: "$file" + suffix}
		revision := "file_revision" + suffix
		value.Commit.State = "live"
		value.Commit.Content = &ContentPointer{Kind: "file", RevisionID: revision, Reference: ref}
		entry.Record.Payload = atomicPayload(t, value)
		files.requests[ref] = FileRequest{ObjectID: entry.Record.ObjectID, RevisionID: revision, SizeBytes: int64(17 + i), Metadata: []byte(`{"private":"file metadata","unknown":9007199254740993}`)}
		files.digests[ref] = strings.Repeat(suffix, 64)
	}
	rootEntry := atomicEntry(p, "$commit")
	var root AtomicBatchCommit
	_ = json.Unmarshal(rootEntry.Record.Payload, &root)
	record := p.records[root.Last.Reference]
	var page AtomicBatchPage
	_ = json.Unmarshal(record.Payload, &page)
	for i := range page.Members {
		entry := atomicEntry(p, page.Members[i].Reference.EventID)
		page.Members[i].SHA256, _ = AtomicRecordSHA256(entry.Record)
	}
	record.Payload = atomicPayload(t, page)
	p.records[root.Last.Reference] = record
	root.Last.SHA256, _ = AtomicRecordSHA256(record)
	rootEntry.Record.Payload = atomicPayload(t, root)
	for _, history := range p.pages["!private:test"] {
		for _, entry := range history.Entries {
			p.records[entry.Reference] = cloneCatalogEntry(entry).Record
		}
	}
	member := Reference{RoomID: "!private:test", EventID: "$prepared0"}
	file := files.requests[Reference{RoomID: "!private:test", EventID: "$file0"}]
	return p, files, rootEntry.Reference, member, file
}

func TestAtomicFileCompletionBindsOriginalRequestAndDigestWithoutObjectVersion(t *testing.T) {
	p, files, root, member, request := atomicFileProofFixture(t)
	digest := strings.Repeat("0", 64)
	batch, err := VerifyAtomicBatch(t.Context(), p, catalogScope, root)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := batch.FileCompletion(t.Context(), files, member)
	if err != nil {
		t.Fatal(err)
	}
	published := Reference{RoomID: "!private:test", EventID: "$file0"}
	pointer, err := proof.ReadPointer(t.Context(), catalogScope, request, published, digest)
	if err != nil || pointer.Kind != "file" || pointer.Reference != published || pointer.RevisionID != request.RevisionID || proof.MemberReference() != member || !proof.MatchesScope(catalogScope) {
		t.Fatal("exact accepted file completion unavailable", err)
	}
	for _, fault := range []string{"scope", "object", "revision", "metadata", "source-digest", "published"} {
		s, req, ref, sha := catalogScope, cloneFileRequest(request), published, digest
		switch fault {
		case "scope":
			s.Subject = "other"
		case "object":
			req.ObjectID = "other"
		case "revision":
			req.RevisionID = "other"
		case "metadata":
			req.Metadata = []byte(`{"unknown":9007199254740995}`)
		case "source-digest":
			sha = strings.Repeat("f", 64)
		case "published":
			ref.EventID = "$other"
		}
		before := p.reads
		if _, err := proof.ReadPointer(t.Context(), s, req, ref, sha); err == nil || p.reads != before {
			t.Fatal("foreign completion reached SDK readback", fault, err)
		}
	}
	if _, err := batch.FileCompletion(t.Context(), files, root); err == nil {
		t.Fatal("batch root became a single-object file index")
	}
}

func TestAtomicFileCompletionRejectsForgedOrRetiredProofAndChangedReadback(t *testing.T) {
	for _, fault := range []string{"zero", "json", "root-bytes", "member-bytes", "other-member-bytes", "page-bytes", "missing-other-member", "member-scope", "file-request", "file-digest", "source-binding", "source-error", "file-scope", "cancel-after-read"} {
		t.Run(fault, func(t *testing.T) {
			p, files, root, member, request := atomicFileProofFixture(t)
			digest := strings.Repeat("0", 64)
			batch, err := VerifyAtomicBatch(t.Context(), p, catalogScope, root)
			if err != nil {
				t.Fatal(err)
			}
			proof, err := batch.FileCompletion(t.Context(), files, member)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch fault {
			case "zero":
				proof = VerifiedAtomicFileCompletion{}
			case "json":
				raw, _ := json.Marshal(proof) //nolint:staticcheck // SA9005: intentional test that unexported proof fields do not serialize.
				proof = VerifiedAtomicFileCompletion{}
				if err := json.Unmarshal(raw, &proof); err != nil { //nolint:staticcheck // SA9005: intentional test that unexported proof fields do not serialize.
					t.Fatal(err)
				}
			case "root-bytes":
				r := p.records[root]
				r.Payload = append(r.Payload, ' ')
				p.records[root] = r
			case "member-bytes":
				r := p.records[member]
				r.Payload = append(r.Payload, ' ')
				p.records[member] = r
			case "other-member-bytes":
				ref := Reference{RoomID: "!private:test", EventID: "$prepared1"}
				r := p.records[ref]
				r.Payload = append(r.Payload, ' ')
				p.records[ref] = r
			case "page-bytes":
				ref := Reference{RoomID: "!private:test", EventID: "$page0"}
				r := p.records[ref]
				r.Payload = append(r.Payload, ' ')
				p.records[ref] = r
			case "missing-other-member":
				delete(p.records, Reference{RoomID: "!private:test", EventID: "$prepared1"})
			case "member-scope":
				r := p.records[member]
				r.Scope.Subject = "other"
				p.records[member] = r
			case "file-request":
				r := files.requests[Reference{RoomID: "!private:test", EventID: "$file0"}]
				r.SizeBytes++
				files.requests[Reference{RoomID: "!private:test", EventID: "$file0"}] = r
			case "file-digest":
				files.digests[Reference{RoomID: "!private:test", EventID: "$file0"}] = strings.Repeat("f", 64)
			case "source-binding":
				p.binding.Target.DeviceID = "CURRENT_OTHER"
			case "source-error":
				p.bind = func(int) (Binding, error) { return Binding{}, errors.New("retired original reader") }
			case "file-scope":
				files.scope.Subject = "other"
			case "cancel-after-read":
				p.onRead = cancel
			}
			if _, err := proof.ReadPointer(ctx, catalogScope, request, Reference{RoomID: "!private:test", EventID: "$file0"}, digest); err == nil {
				t.Fatal("forged/changed/retired completion admitted", fault)
			}
		})
	}
}

func TestAtomicFileCompletionAcceptsExactHistoricalMemberAfterNewerHead(t *testing.T) {
	p, files, root, member, request := atomicFileProofFixture(t)
	digest := strings.Repeat("0", 64)
	later := catalogEntry("later", "r2", "deleted", member)
	later.Record.ObjectID = request.ObjectID
	history := p.pages["!private:test"][""]
	history.Entries = append(history.Entries, later)
	p.pages["!private:test"][""] = history
	p.records[later.Reference] = later.Record
	batch, err := VerifyAtomicBatch(t.Context(), p, catalogScope, root)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := batch.FileCompletion(t.Context(), files, member)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proof.ReadPointer(t.Context(), catalogScope, request, Reference{RoomID: "!private:test", EventID: "$file0"}, digest); err != nil {
		t.Fatal("newer head prevented exact accepted cleanup", err)
	}
}
