// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type changedRecordReply struct {
	fileRecordFixture
	change func(*storage.Record)
}

func (s *changedRecordReply) ReadStorageRecord(ctx context.Context, ref EventRef, object, role string) (storage.Record, error) {
	rec, err := s.fileRecordFixture.ReadStorageRecord(ctx, ref, object, role)
	if s.change != nil {
		s.change(&rec)
	}
	return rec, err
}

func TestStorageRecordConfirmationRequiresExactAuthenticatedRevision(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	record := storage.Record{Version: 1, Scope: scope, ObjectID: "object", RevisionID: "revision", Role: "content", Payload: []byte(`{"integer":9007199254740993}`), Previous: []string{"$parent"}}
	for _, field := range []string{"scope", "object", "revision", "role", "payload", "parents"} {
		t.Run(field, func(t *testing.T) {
			fixture := &changedRecordReply{change: func(rec *storage.Record) {
				switch field {
				case "scope":
					rec.Scope.Subject = "mallory"
				case "object":
					rec.ObjectID = "foreign"
				case "revision":
					rec.RevisionID = "old"
				case "role":
					rec.Role = "manifest"
				case "payload":
					rec.Payload = []byte(`{"integer":9007199254740992}`)
				case "parents":
					rec.Previous = nil
				}
			}}
			if _, err := writeStorageRecordVerified(t.Context(), fixture, "stable", record); !errors.Is(err, storage.ErrRecord) {
				t.Fatal("changed readback was acknowledged", field, err)
			}
			fixture.change = nil
			if _, err := writeStorageRecordVerified(t.Context(), fixture, "stable", record); err != nil {
				t.Fatal("exact retry did not confirm the retained original", err)
			}
		})
	}
}
