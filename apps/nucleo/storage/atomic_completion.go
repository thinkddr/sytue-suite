// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"context"
	"slices"
)

// FileCommitmentReader is the existing scoped file readback surface. BoundFiles
// satisfies it without granting this proof any upload or acknowledgement method.
type FileCommitmentReader interface {
	FileScope() Scope
	Commitment(context.Context, Reference, string, string) (FileRequest, string, error)
}

type atomicVerifiedMember struct {
	reference Reference
	object    string
	recordSHA string
	content   ContentPointer
	state     string
}
type atomicRecordWitness struct {
	reference         Reference
	object, role, sha string
}
type verifiedAtomicBatch struct {
	reader  AtomicCatalogReader
	binding Binding
	root    RecordEntry
	rootSHA string
	members map[Reference]atomicVerifiedMember
	records []atomicRecordWitness
}

// VerifiedAtomicBatch can only be minted from a complete original-SDK catalog.
// Its contents cannot be reconstructed from a JSON DTO, a group ID, or a caller's
// ObjectVersion. It grants historical readback only, never writer authority.
type (
	VerifiedAtomicBatch  struct{ data *verifiedAtomicBatch }
	atomicFileCompletion struct {
		batch     *verifiedAtomicBatch
		member    atomicVerifiedMember
		files     FileCommitmentReader
		request   FileRequest
		sourceSHA string
	}
)
type VerifiedAtomicFileCompletion struct{ data *atomicFileCompletion }

func VerifyAtomicBatch(ctx context.Context, reader AtomicCatalogReader, scope Scope, root Reference) (VerifiedAtomicBatch, error) {
	snapshot, err := LoadAtomicCatalog(ctx, reader, scope)
	if err != nil {
		return VerifiedAtomicBatch{}, err
	}
	entry, found := snapshot.Entries[root]
	if !found {
		return VerifiedAtomicBatch{}, ErrIncompleteCatalog
	}
	commit, err := DecodeAtomicBatch(entry, scope)
	if err != nil || snapshot.Batches[commit.BatchID] != root {
		return VerifiedAtomicBatch{}, ErrRecord
	}
	sum, err := AtomicRecordSHA256(entry.Record)
	if err != nil {
		return VerifiedAtomicBatch{}, err
	}
	data := &verifiedAtomicBatch{reader: reader, binding: snapshot.Binding, root: cloneCatalogEntry(entry), rootSHA: sum, members: map[Reference]atomicVerifiedMember{}}
	data.records = append(data.records, atomicRecordWitness{root, entry.Record.ObjectID, entry.Record.Role, sum})
	// Keep the exact manifest chain and every original member for fresh SDK
	// readback immediately before ACK. A root alone cannot attest availability.
	pointer := commit.Last
	for {
		page, ok := snapshot.Pages[pointer.Reference]
		if !ok {
			return VerifiedAtomicBatch{}, ErrIncompleteCatalog
		}
		sum, err := AtomicRecordSHA256(page.Record)
		if err != nil {
			return VerifiedAtomicBatch{}, err
		}
		data.records = append(data.records, atomicRecordWitness{page.Reference, page.Record.ObjectID, page.Record.Role, sum})
		var value AtomicBatchPage
		if decodeAtomicJSON(page.Record.Payload, &value) != nil {
			return VerifiedAtomicBatch{}, ErrRecord
		}
		if value.Previous == nil {
			break
		}
		pointer = *value.Previous
	}
	// History contains exactly the committed projection, including accepted
	// historical revisions superseded by later edits. Heads is never an ACK proof.
	for _, version := range snapshot.History {
		raw := snapshot.Entries[version.Reference]
		kind, err := atomicKind(raw, scope)
		if err != nil {
			return VerifiedAtomicBatch{}, err
		}
		if kind != ObjectPreparedKind {
			continue
		}
		prepared, actual, err := DecodeAtomicPrepared(raw, scope)
		if err != nil {
			return VerifiedAtomicBatch{}, err
		}
		if prepared.BatchID != commit.BatchID {
			continue
		}
		sum, err := AtomicRecordSHA256(raw.Record)
		if err != nil {
			return VerifiedAtomicBatch{}, err
		}
		data.records = append(data.records, atomicRecordWitness{raw.Reference, raw.Record.ObjectID, raw.Record.Role, sum})
		member := atomicVerifiedMember{reference: version.Reference, object: actual.ObjectID, recordSHA: sum, state: actual.Commit.State}
		if actual.Commit.Content != nil {
			member.content = *actual.Commit.Content
		}
		data.members[version.Reference] = member
	}
	if uint64(len(data.members)) != commit.Count {
		return VerifiedAtomicBatch{}, ErrIncompleteCatalog
	}
	if err := data.check(ctx); err != nil {
		return VerifiedAtomicBatch{}, err
	}
	return VerifiedAtomicBatch{data}, nil
}

func (batch *verifiedAtomicBatch) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if batch == nil || batch.reader == nil || !batch.binding.Valid() {
		return ErrRecord
	}
	current, err := batch.reader.StorageBindings(ctx)
	if err != nil {
		return err
	}
	if !batch.binding.Same(current) {
		return ErrCatalogSource
	}
	return ctx.Err()
}

func (batch *verifiedAtomicBatch) readExact(ctx context.Context, witness atomicRecordWitness) error {
	record, err := batch.reader.ReadStorageRecord(ctx, witness.reference, witness.object, witness.role)
	if err != nil {
		return err
	}
	actual, err := AtomicRecordSHA256(record)
	if err != nil || record.Validate(batch.binding.Scope) != nil || record.ObjectID != witness.object || record.Role != witness.role || actual != witness.sha {
		return ErrRecord
	}
	return ctx.Err()
}

func cloneFileRequest(request FileRequest) FileRequest {
	request.Metadata = slices.Clone(request.Metadata)
	request.Previous = slices.Clone(request.Previous)
	return request
}
func sameFileRequest(a, b FileRequest) bool { return a.MatchesExact(b) }

// FileCompletion binds an actual signed file manifest's request/digest to the
// accepted prepared member. The file reader supplies original Go readback, not
// a hash hint supplied by the application or recovery service.
func (proof VerifiedAtomicBatch) FileCompletion(ctx context.Context, files FileCommitmentReader, memberRef Reference) (VerifiedAtomicFileCompletion, error) {
	batch := proof.data
	if err := batch.check(ctx); err != nil {
		return VerifiedAtomicFileCompletion{}, err
	}
	member, found := batch.members[memberRef]
	if !found || member.content.Kind != "file" || (member.state != "live" && member.state != "trash") || files == nil || files.FileScope() != batch.binding.Scope {
		return VerifiedAtomicFileCompletion{}, ErrRecord
	}
	request, sourceSHA, err := files.Commitment(ctx, member.content.Reference, member.object, member.content.RevisionID)
	if err != nil {
		return VerifiedAtomicFileCompletion{}, err
	}
	if request.Validate(batch.binding.Scope) != nil || request.ObjectID != member.object || request.RevisionID != member.content.RevisionID || !validAtomicHash(sourceSHA) {
		return VerifiedAtomicFileCompletion{}, ErrRecord
	}
	if err := batch.check(ctx); err != nil {
		return VerifiedAtomicFileCompletion{}, err
	}
	return VerifiedAtomicFileCompletion{&atomicFileCompletion{batch, member, files, cloneFileRequest(request), sourceSHA}}, nil
}

func (proof VerifiedAtomicFileCompletion) MemberReference() Reference {
	if proof.data == nil {
		return Reference{}
	}
	return proof.data.member.reference
}

// ReadPointer revalidates current source binding, exact SDK root/page/all-member bytes,
// and the original file commitment immediately before the existing upload WAL
// can be retired. The returned pointer is private data, not an ObjectVersion or
// publication capability. A zero/JSON-constructed/retired proof always rejects.
func (proof VerifiedAtomicFileCompletion) ReadPointer(ctx context.Context, scope Scope, request FileRequest, published Reference, sourceSHA string) (ContentPointer, error) {
	data := proof.data
	if data == nil || data.batch == nil || scope != data.batch.binding.Scope || request.Validate(scope) != nil || !sameFileRequest(request, data.request) || sourceSHA != data.sourceSHA || published != data.member.content.Reference {
		return ContentPointer{}, ErrRecord
	}
	if err := data.batch.check(ctx); err != nil {
		return ContentPointer{}, err
	}
	for _, witness := range data.batch.records {
		if err := data.batch.readExact(ctx, witness); err != nil {
			return ContentPointer{}, err
		}
	}
	if data.files == nil || data.files.FileScope() != scope {
		return ContentPointer{}, ErrCatalogSource
	}
	current, digest, err := data.files.Commitment(ctx, published, data.member.object, data.member.content.RevisionID)
	if err != nil {
		return ContentPointer{}, err
	}
	if current.Validate(scope) != nil || !sameFileRequest(current, data.request) || digest != data.sourceSHA {
		return ContentPointer{}, ErrRecord
	}
	if err := data.batch.check(ctx); err != nil {
		return ContentPointer{}, err
	}
	return data.member.content, nil
}

// MatchesScope checks the declared owner of an existing bound file adapter.
// It cannot mint or deserialize a completion proof.
func (proof VerifiedAtomicFileCompletion) MatchesScope(scope Scope) bool {
	return proof.data != nil && proof.data.batch != nil && proof.data.batch.binding.Scope == scope
}
