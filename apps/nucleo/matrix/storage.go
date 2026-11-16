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
	if !b.StorageIdentityVersion.Normalized().Valid() || !storage.Product(b.StorageProduct) || b.StorageRoomID == "" || len(b.Channels) != 0 || b.DraftRoomID != "" || len(b.DraftRoomIDs) != 0 || len(b.StorageRoomIDs) == 0 || len(b.StorageRoomIDs) > 100 || !slices.Contains(b.StorageRoomIDs, b.StorageRoomID) {
		return false
	}
	if b.StorageIdentityVersion == storage.IsolatedIdentity {
		prefix := storage.IsolatedMatrixUserPrefix(storage.Scope{Subject: b.Actor, Tenant: b.Tenant, Product: b.StorageProduct})
		if prefix == "" || !strings.HasPrefix(b.MatrixUserID, prefix) || len(b.MatrixUserID) <= len(prefix) {
			return false
		}
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

// StoragePublicationTarget captures the public Matrix endpoint/device of a durable intent.
// Refreshing a token preserves idempotency; changing the room or device does not.
func (c *Client) StoragePublicationTarget() (storage.PublicationTarget, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	target := storage.PublicationTarget{RoomID: c.cfg.Bootstrap.StorageRoomID, DeviceID: c.DeviceID()}
	return target, c.checkStoragePublicationTarget(target)
}

// StorageBindings exposes only authenticated routing. Catalog loaders compare
// this capture again before accepting a scan as complete.
func (c *Client) StorageBindings(ctx context.Context) (storage.Binding, error) {
	scope, err := c.storageBlockScope(ctx)
	if err != nil {
		return storage.Binding{}, err
	}
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	target := storage.PublicationTarget{RoomID: c.cfg.Bootstrap.StorageRoomID, DeviceID: c.DeviceID()}
	if scope != c.storageScope() {
		return storage.Binding{}, ErrIdentity
	}
	if err := c.checkStoragePublicationTarget(target); err != nil {
		return storage.Binding{}, err
	}
	binding := storage.Binding{Scope: scope, Target: target, Rooms: slices.Clone(c.cfg.Bootstrap.StorageRoomIDs)}
	if !binding.Valid() {
		return storage.Binding{}, ErrIdentity
	}
	return binding, nil
}

// PrepareStorage requires a verified device AND its authenticated owner key
// before an application freezes a legacy source. No key material escapes here.
func (c *Client) PrepareStorage(ctx context.Context, expected storage.Scope) (storage.Binding, error) {
	c.stateMu.RLock()
	valid := expected.Valid() && expected == c.storageScope()
	c.stateMu.RUnlock()
	if !valid {
		return storage.Binding{}, ErrIdentity
	}
	binding, err := c.StorageBindings(ctx)
	if err != nil {
		return storage.Binding{}, err
	}
	if binding.Scope != expected {
		return storage.Binding{}, ErrIdentity
	}
	key, err := c.storageAuthKey(ctx, "")
	if err != nil {
		return storage.Binding{}, err
	}
	clear(key.Key)
	if _, err := c.storagePQIdentity(ctx, "", true); err != nil {
		return storage.Binding{}, err
	}
	current, err := c.StorageBindings(ctx)
	if err != nil {
		return storage.Binding{}, err
	}
	if !binding.Same(current) {
		return storage.Binding{}, ErrIdentity
	}
	return binding, nil
}

func (c *Client) checkStoragePublicationTarget(target storage.PublicationTarget) error {
	if c.closed.Load() {
		return ErrClosed
	}
	if !target.Valid() || !c.storageScope().Valid() || target.DeviceID != c.DeviceID() || !c.allowedRoom(target.RoomID) {
		return ErrIdentity
	}
	return nil
}

// CheckStoragePublicationTarget has no network side effect. Resume checks it before uploading bytes.
func (c *Client) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.checkStoragePublicationTarget(target)
}

// PutStorageRecord uses the current endpoint for a new private record. Durable
// intents must retain its target and use PutStorageRecordAt for all retries.
func (c *Client) PutStorageRecord(ctx context.Context, transaction string, record storage.Record) (EventRef, error) {
	target, err := c.StoragePublicationTarget()
	if err != nil {
		return EventRef{}, err
	}
	return c.PutStorageRecordAt(ctx, target, transaction, record)
}

// PutStorageRecordAt never redirects a previously journaled transaction to a new room or device.
func (c *Client) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (EventRef, error) {
	record.Payload = bytes.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	c.stateMu.RLock()
	scope := c.storageScope()
	targetErr := c.checkStoragePublicationTarget(target)
	c.stateMu.RUnlock()
	if targetErr != nil {
		return c.recoverAdmittedStorageRecord(ctx, target, transaction, record, targetErr)
	}
	if !storage.Identifier(transaction) || record.Validate(scope) != nil {
		return EventRef{}, storage.ErrRecord
	}
	ctx, err := c.bindStoragePublication(ctx, scope, target)
	if err != nil {
		return EventRef{}, err
	}
	return c.withStorageAdmissionRecord(ctx, target, transaction, record, func(ctx context.Context) (EventRef, error) {
		key, err := c.storageAuthKey(ctx, "")
		if err != nil {
			return EventRef{}, err
		}
		defer clear(key.Key)
		proof, err := sealStorageOwnerRecord(target.RoomID, record, key)
		if err != nil {
			return EventRef{}, err
		}
		return c.putStoragePQRecord(ctx, target, transaction, record, proof)
	})
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
	// readPrivateStorageRecord (storage auth keys) ya rechaza los eventos
	// recuperados sin prueba de propiedad: el guard de sytue-native quedó dentro.
	record, err := c.readPrivateStorageRecord(ctx, value, scope, c.session.UserID.String())
	if err == nil && (record.ObjectID != object || record.Role != role) {
		err = storage.ErrRecord
	}
	if err != nil {
		return storage.Record{}, err
	}
	return record, nil
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
	if len(value.Content) > storage.MaxRecordBytes+4096 || json.Unmarshal(value.Content, &content) != nil || content.Record.Validate(scope) != nil {
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
	if err == nil && !rec.MatchesExact(got) {
		err = storage.ErrRecord
	}
	return ref, err
}

// Publication is separate from the read port: older adapters cannot silently
// claim safe resumability without pinning Matrix's idempotency scope.
type storagePublicationIO interface {
	StoragePublicationTarget() (storage.PublicationTarget, error)
	CheckStoragePublicationTarget(storage.PublicationTarget) error
	PutStorageRecordAt(context.Context, storage.PublicationTarget, string, storage.Record) (EventRef, error)
}

type pinnedStorageRecords struct {
	storageRecordIO
	publication storagePublicationIO
	target      storage.PublicationTarget
}

func (p pinnedStorageRecords) PutStorageRecord(ctx context.Context, txn string, record storage.Record) (EventRef, error) {
	return p.publication.PutStorageRecordAt(ctx, p.target, txn, record)
}
