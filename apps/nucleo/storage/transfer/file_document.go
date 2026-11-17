// SPDX-License-Identifier: Apache-2.0
package transfer

import (
	"bytes"
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (t *engine) classifyFileDocument(ctx context.Context, version storage.ObjectVersion) error {
	rules, ok := t.cfg.Rules.(FileDocumentRules)
	if !ok {
		return nil
	}
	limit, err := rules.FileDocumentLimit(ctx, version)
	if err != nil {
		return err
	}
	if limit == 0 {
		return nil
	}
	if limit < 0 || limit > storage.MaxFileBytes || version.Commit.Content == nil || version.Commit.Content.Kind != "file" || t.cfg.ReadFileDocument == nil || t.cfg.PutFileDocument == nil {
		return ErrSource
	}
	t.documents[version.Reference] = limit
	return nil
}

// Recompute from authenticated original bytes and durable index mappings on
// every retry and readback. Only the rewritten commitment can authorize reuse
// of the SDK upload job; a destination reference alone never establishes it.
func (t *engine) rewrittenFileDocument(ctx context.Context, version storage.ObjectVersion) (storage.FileRequest, []byte, string, error) {
	rules, ok := t.cfg.Rules.(FileDocumentRules)
	limit := t.documents[version.Reference]
	if !ok || limit < 1 {
		return storage.FileRequest{}, nil, "", ErrSource
	}
	pointer := version.Commit.Content
	request, sum, err := t.cfg.SourceFileCommitment(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return storage.FileRequest{}, nil, "", err
	}
	if request.Validate(t.state.Scope) != nil || len(request.Previous) != 0 || request.ObjectID != version.ObjectID || request.RevisionID != pointer.RevisionID || !validHash(sum) || request.SizeBytes > limit {
		return storage.FileRequest{}, nil, "", ErrSource
	}
	original, err := t.cfg.ReadFileDocument(ctx, version, request.SizeBytes)
	if err != nil {
		clear(original)
		return storage.FileRequest{}, nil, "", err
	}
	defer clear(original)
	if int64(len(original)) != request.SizeBytes || digest(original) != sum {
		return storage.FileRequest{}, nil, "", ErrSource
	}
	rewritten, err := rules.RewriteFileDocument(ctx, version, original, t.dependency)
	if err != nil {
		clear(rewritten)
		return storage.FileRequest{}, nil, "", err
	}
	if len(rewritten) == 0 || int64(len(rewritten)) > limit {
		clear(rewritten)
		return storage.FileRequest{}, nil, "", ErrSource
	}
	payload := bytes.Clone(rewritten)
	clear(rewritten)
	expected := request
	expected.SizeBytes = int64(len(payload))
	input, err := json.Marshal(struct {
		Original       storage.FileRequest
		OriginalSHA256 string
		Expected       storage.FileRequest
		PayloadSHA256  string
	}{request, sum, expected, digest(payload)})
	if err != nil {
		clear(payload)
		return storage.FileRequest{}, nil, "", err
	}
	defer clear(input)
	return expected, payload, digest(input), nil
}

func (t *engine) copyFileDocument(ctx context.Context, version storage.ObjectVersion) (storage.Reference, error) {
	request, payload, input, err := t.rewrittenFileDocument(ctx, version)
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(payload)
	pointer := version.Commit.Content
	work, err := t.mapping(pointer.Reference, version.ObjectID, pointer.RevisionID, "file", input)
	if err != nil {
		return storage.Reference{}, err
	}
	var ref storage.Reference
	if work.Result == nil {
		ref, err = t.cfg.PutFileDocument(ctx, t.state.Destination.Target, t.transaction(pointer.Reference, "file"), request, bytes.NewReader(payload))
		if err != nil {
			return storage.Reference{}, err
		}
	} else {
		ref = *work.Result
	}
	if err := t.verifyFileDocument(ctx, ref, request, payload); err != nil {
		return storage.Reference{}, err
	}
	return ref, t.saveMapping(work, ref)
}

func (t *engine) verifyFileDocument(ctx context.Context, ref storage.Reference, request storage.FileRequest, payload []byte) error {
	if !ref.Valid() || ref.RoomID != t.state.Destination.Target.RoomID {
		return ErrSource
	}
	actual, sum, err := t.cfg.TargetFileCommitment(ctx, ref, request.ObjectID, request.RevisionID)
	if err != nil {
		return err
	}
	expectedJSON, _ := json.Marshal(request)
	actualJSON, _ := json.Marshal(actual)
	if sum != digest(payload) || !bytes.Equal(expectedJSON, actualJSON) {
		return ErrSource
	}
	return nil
}

func (t *engine) expectedFileDocumentReference(ctx context.Context, version storage.ObjectVersion) (storage.Reference, error) {
	request, payload, input, err := t.rewrittenFileDocument(ctx, version)
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(payload)
	pointer := version.Commit.Content
	work, err := t.mapping(pointer.Reference, version.ObjectID, pointer.RevisionID, "file", input)
	if err != nil {
		return storage.Reference{}, err
	}
	if work.Result == nil {
		return storage.Reference{}, ErrSource
	}
	return *work.Result, t.verifyFileDocument(ctx, *work.Result, request, payload)
}
