// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"

	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func validStorageBindings(b Bootstrap) bool {
	if !storage.Product(b.StorageProduct) || b.StorageRoomID == "" || len(b.Channels) != 0 || b.DraftRoomID != "" || len(b.DraftRoomIDs) != 0 || len(b.StorageRoomIDs) == 0 || len(b.StorageRoomIDs) > 100 || !slices.Contains(b.StorageRoomIDs, b.StorageRoomID) {
		return false
	}
	seen := map[string]bool{}
	for _, room := range b.StorageRoomIDs {
		if !strings.HasPrefix(room, "!") || len(room) < 2 || len(room) > 255 || strings.ContainsAny(room, "\x00\r\n\t ") || seen[room] {
			return false
		}
		seen[room] = true
	}
	return true
}

func (c *Client) privateRoom(room string) bool {
	b := c.cfg.Bootstrap
	return room != "" && (b.StorageProduct != "" || room == b.DraftRoomID || slices.Contains(b.DraftRoomIDs, room))
}

func (c *Client) storageScope() storage.Scope {
	return storage.Scope{Tenant: c.cfg.Tenant, Subject: c.cfg.Subject, Product: c.cfg.Bootstrap.StorageProduct}
}

// PutStorageRecord uses the same verified-device/recovery policy as Rooms. The
// caller-generated transaction makes retries idempotent in the maintained SDK.
// No application payload is sent to the metadata API or Matrix plaintext state.
func (c *Client) PutStorageRecord(ctx context.Context, transaction string, record storage.Record) (EventRef, error) {
	c.stateMu.RLock()
	scope, room := c.storageScope(), c.cfg.Bootstrap.StorageRoomID
	c.stateMu.RUnlock()
	if !storage.Identifier(transaction) || record.Validate(scope) != nil {
		return EventRef{}, storage.ErrRecord
	}
	return c.sendEventWithTxn(ctx, room, event.EventMessage, map[string]any{
		"msgtype": "m.text", "body": "", "org.sytue.storage": record,
	}, "sytue-storage-"+transaction)
}

// ReadStorageRecord accepts an explicitly scoped receipt. It never searches for
// a same-named object in another app, tenant, room or sender's history.
func (c *Client) ReadStorageRecord(ctx context.Context, ref EventRef, object, role string) (storage.Record, error) {
	c.stateMu.RLock()
	scope := c.storageScope()
	allowed := c.cfg.Bootstrap.StorageProduct != "" && c.allowedRoom(ref.RoomID)
	c.stateMu.RUnlock()
	if !allowed {
		return storage.Record{}, ErrIdentity
	}
	value, err := c.Read(ctx, ref)
	if err != nil {
		return storage.Record{}, err
	}
	return decodeStorageRecord(value, scope, c.session.UserID.String(), object, role)
}

func decodeStorageRecord(value DecryptedEvent, scope storage.Scope, own, object, role string) (storage.Record, error) {
	rec, err := decodePrivateRecord(value, scope, own)
	if err != nil {
		return rec, err
	}
	if rec.ObjectID != object || rec.Role != role {
		return storage.Record{}, storage.ErrRecord
	}
	return rec, nil
}

func decodePrivateRecord(value DecryptedEvent, scope storage.Scope, own string) (storage.Record, error) {
	if value.Sender != own || value.Type != event.EventMessage.Type {
		return storage.Record{}, ErrIdentity
	}
	var content struct {
		Record storage.Record `json:"org.sytue.storage"` //nolint:tagliatelle // Matrix namespaced extension.
	}
	if len(value.Content) > storage.MaxRecordBytes+1024 || json.Unmarshal(value.Content, &content) != nil || content.Record.Validate(scope) != nil {
		return storage.Record{}, storage.ErrRecord
	}
	return content.Record, nil
}

type storageRecordIO interface {
	PutStorageRecord(context.Context, string, storage.Record) (EventRef, error)
	ReadStorageRecord(context.Context, EventRef, string, string) (storage.Record, error)
}

// Read back the SDK receipt before acknowledging a private write. Reusing a
// transaction with changed content cannot silently return a different revision.
func writeStorageRecordVerified(ctx context.Context, records storageRecordIO, txn string, rec storage.Record) (EventRef, error) {
	if records == nil || !storage.Identifier(txn) || rec.Validate(rec.Scope) != nil {
		return EventRef{}, storage.ErrRecord
	}
	ref, err := records.PutStorageRecord(ctx, txn, rec)
	if err != nil {
		return ref, err
	}
	if !ref.Valid() {
		return ref, storage.ErrRecord
	}
	got, err := records.ReadStorageRecord(ctx, ref, rec.ObjectID, rec.Role)
	if err == nil && (got.Validate(rec.Scope) != nil || got.ObjectID != rec.ObjectID || got.RevisionID != rec.RevisionID || got.Role != rec.Role || !bytes.Equal(got.Payload, rec.Payload) || !slices.Equal(got.Previous, rec.Previous)) {
		err = storage.ErrRecord
	}
	return ref, err
}
