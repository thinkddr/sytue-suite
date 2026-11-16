// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// CommittedReader must perform the maintained SDK GET/decrypt and open the
// exact context-bound hybrid frame. A response body or public receipt alone
// cannot implement this contract. The returned buffer belongs to the caller.
type CommittedReader interface {
	ReadCommitted(context.Context, VerifiedCommittedRead) ([]byte, error)
}

// EncodeRecord uses the same private record wire as personal application
// storage. The caller must still seal and publish through its resource broker;
// encoding never grants any recipient, vault, signing or send authority.
func EncodeRecord(record storage.Record) ([]byte, error) {
	if record.Validate(record.Scope) != nil {
		return nil, ErrInvalid
	}
	return matrixproof.CanonicalBytes(record)
}

// ReadCommittedRecord binds the shared private record to its verified ORIGINAL
// author, object and revision while retaining CURRENT reader authorization.
// It never mounts the author's personal storage or restores its live SDK.
func ReadCommittedRecord(ctx context.Context, sdk CommittedReader, proof VerifiedCommittedRead, reader VerifiedAccess, fence Fence, object, revision, role string) (storage.Record, error) {
	if sdk == nil {
		return storage.Record{}, ErrInvalid
	}
	if err := proof.RevalidateReader(ctx, reader, fence); err != nil {
		return storage.Record{}, err
	}
	scope := proof.ProducerScope()
	expected := storage.Record{Version: 1, Scope: scope, ObjectID: object, RevisionID: revision, Role: role, Payload: []byte("{}")}
	if expected.Validate(scope) != nil {
		return storage.Record{}, ErrInvalid
	}
	raw, err := sdk.ReadCommitted(ctx, proof)
	if err != nil {
		clear(raw)
		return storage.Record{}, err
	}
	defer clear(raw)
	var record storage.Record
	if len(raw) > storage.MaxRecordBytes || decodeReservationValue(raw, &record) != nil || record.Validate(scope) != nil || record.ObjectID != object || record.RevisionID != revision || record.Role != role {
		clear(record.Payload)
		return storage.Record{}, ErrInvalid
	}
	if err := proof.RevalidateReader(ctx, reader, fence); err != nil {
		clear(record.Payload)
		return storage.Record{}, err
	}
	return record, nil
}
