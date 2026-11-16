// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

var ErrStoragePublicationPending = errors.New("matrix: encrypted publication is durably pending")

// StoragePublicationService is control metadata only. The original bound API
// session owns HTTP credentials; a JSON authority or URL is never a signer.
type StoragePublicationService interface {
	ReserveMatrixStoragePublication(context.Context, publication.SignedRequest) (publication.Receipt, error)
	ReadMatrixStoragePublication(context.Context, publication.Request) (publication.Receipt, error)
}

// Recovery can read a previous producer receipt without minting a marker.
type StoragePublicationRecoveryService interface {
	ReadMatrixStoragePublicationRecovery(context.Context, publication.RecoverySelection) (publication.RecoveryHint, error)
}

type (
	storageAdmissionKey       struct{}
	storageRecordAdmissionKey struct{}
	storageAdmission          struct {
		client    *Client
		scope     storage.Scope
		authority matrixproof.Authority
		lease     offline.WorkLease
		service   StoragePublicationService
		journal   *offline.Store
		authorize storage.PublicationPolicy
	}
)

// StoragePublicationBinder is installed on the ORIGINAL product writer context.
// Construction performs no IO or lock acquisition. It never changes the SDK's
// global configuration, wraps another identity, or gives SDK1 Authority2's root.
func (c *Client) StoragePublicationBinder(scope storage.Scope, authority matrixproof.Authority, lease offline.WorkLease, service StoragePublicationService, journal *offline.Store, authorize storage.PublicationPolicy) (func(context.Context) (context.Context, error), error) {
	if c == nil || c.cfg.ResourceProduct != "" || c.cfg.Bootstrap.StorageIdentityVersion.Normalized() != storage.IsolatedIdentity || scope != c.storageScope() || authority.RequireActive(scope, authority.Generation) != nil || lease == nil || service == nil || !journal.Encrypted() || authorize == nil {
		return nil, ErrIdentity
	}
	fixed := &storageAdmission{client: c, scope: scope, authority: authority, lease: lease, service: service, journal: journal, authorize: authorize}
	return func(ctx context.Context) (context.Context, error) {
		if previous, ok := ctx.Value(storageAdmissionKey{}).(*storageAdmission); ok && previous != fixed {
			return nil, ErrIdentity
		}
		if err := authorize(ctx); err != nil {
			return nil, err
		}
		if c.closed.Load() {
			return nil, ErrClosed
		}
		return context.WithValue(ctx, storageAdmissionKey{}, fixed), nil
	}, nil
}

//nolint:nilnil // absent admission means the unadmitted path; (nil, nil) is a valid absent state, not an error
func (c *Client) admissionFor(ctx context.Context) (*storageAdmission, error) {
	fixed, ok := ctx.Value(storageAdmissionKey{}).(*storageAdmission)
	if !ok {
		return nil, nil
	}
	if fixed == nil || fixed.client != c || fixed.scope != c.storageScope() || c.cfg.Bootstrap.StorageIdentityVersion.Normalized() != storage.IsolatedIdentity {
		return nil, ErrIdentity
	}
	if err := fixed.authorize(ctx); err != nil {
		return nil, err
	}
	return fixed, nil
}

func (a *storageAdmission) owner() string {
	return a.scope.Tenant + "\x00" + a.scope.Subject + "\x00" + a.scope.Product
}

type retainedStorageFrame struct {
	Kind       string                     `json:"kind"`
	Binding    PublicationBinding         `json:"binding"`
	Provenance sessionproof.Session       `json:"provenance"`
	Wire       json.RawMessage            `json:"wire"`
	Request    *publication.SignedRequest `json:"request,omitempty"`
	Reference  *storage.Reference         `json:"reference,omitempty"`
}
type retainedStoragePublication struct {
	Version      int                             `json:"v"`
	Scope        storage.Scope                   `json:"scope"`
	Target       storage.PublicationTarget       `json:"target"`
	Transaction  string                          `json:"transaction"`
	RecordSHA256 string                          `json:"record_sha256"`
	Frames       map[string]retainedStorageFrame `json:"frames"`
	Reference    *storage.Reference              `json:"reference,omitempty"`
}
type storageRecordAdmission struct {
	fixed  *storageAdmission
	id     string
	record *retainedStoragePublication
}

func (r *storageRecordAdmission) persist() error {
	return r.fixed.journal.Write(r.fixed.owner(), storage.NativePublicationNamespace, r.id, r.record)
}

// withStorageAdmissionRecord is BEFORE owner proof, hybrid sealing and splitting.
// The semantic RecordDigest is stable across randomized age/SDK ciphertext. Each
// fragment retains its own immutable SDK wire/txn under this same original record.
func (c *Client) withStorageAdmissionRecord(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record, write func(context.Context) (EventRef, error)) (EventRef, error) {
	fixed, err := c.admissionFor(ctx)
	if err != nil {
		return EventRef{}, err
	}
	if fixed == nil {
		return write(ctx)
	}
	if err := storagePublicationPolicy(ctx); err != nil {
		return EventRef{}, err
	}
	if proof, ok := ctx.Value(storagePublicationContext{}).(storagePublication); !ok || proof.client != c || proof.scope != fixed.scope || proof.target != target {
		return EventRef{}, ErrIdentity
	}
	hash, err := storage.RecordDigest(target.RoomID, record)
	if err != nil {
		return EventRef{}, err
	}
	key := target.RoomID + "\x00" + target.DeviceID + "\x00" + transaction
	var result EventRef
	err = fixed.journal.Exclusive(ctx, fixed.owner(), storage.NativePublicationNamespace, key, func() error {
		var err error
		result, err = c.withStorageAdmissionRecordLocked(ctx, fixed, target, transaction, key, hash, record, write)
		return err
	})
	if err != nil {
		return EventRef{}, err
	}
	if err := storagePublicationPolicy(ctx); err != nil {
		return EventRef{}, err
	}
	if err := fixed.authorize(ctx); err != nil {
		return EventRef{}, err
	}
	return result, nil
}

func (c *Client) withStorageAdmissionRecordLocked(ctx context.Context, fixed *storageAdmission, target storage.PublicationTarget, transaction, key, hash string, record storage.Record, write func(context.Context) (EventRef, error)) (EventRef, error) {
	var retained retainedStoragePublication
	found, err := fixed.journal.Read(fixed.owner(), storage.NativePublicationHistoryNamespace, key, &retained)
	if err != nil {
		return EventRef{}, err
	}
	if !found {
		found, err = fixed.journal.Read(fixed.owner(), storage.NativePublicationNamespace, key, &retained)
		if err != nil {
			return EventRef{}, err
		}
	}
	if found {
		if retained.Version != 1 || retained.Scope != fixed.scope || retained.Target != target || retained.Transaction != transaction || retained.RecordSHA256 != hash || retained.Frames == nil {
			return EventRef{}, ErrIdentity
		}
	} else {
		retained = retainedStoragePublication{Version: 1, Scope: fixed.scope, Target: target, Transaction: transaction, RecordSHA256: hash, Frames: make(map[string]retainedStorageFrame)}
		if err := fixed.journal.Write(fixed.owner(), storage.NativePublicationNamespace, key, &retained); err != nil {
			return EventRef{}, err
		}
	}
	work := &storageRecordAdmission{fixed: fixed, id: key, record: &retained}
	if retained.Reference != nil {
		if err := c.verifyStoredRecordReceipt(ctx, *retained.Reference, record); err != nil {
			return EventRef{}, err
		}
		return *retained.Reference, fixed.journal.Delete(fixed.owner(), storage.NativePublicationNamespace, key)
	}
	result, err := write(context.WithValue(ctx, storageRecordAdmissionKey{}, work))
	if err != nil {
		return EventRef{}, err
	}
	// The existing PQ record writer has already authenticated SDK readback, owner
	// proof and exact original record. Persist history BEFORE removing pending.
	ref := result
	retained.Reference = &ref
	if err := fixed.journal.Write(fixed.owner(), storage.NativePublicationHistoryNamespace, key, &retained); err != nil {
		return EventRef{}, err
	}
	return result, fixed.journal.Delete(fixed.owner(), storage.NativePublicationNamespace, key)
}

func (c *Client) verifyStoredRecordReceipt(ctx context.Context, ref storage.Reference, wanted storage.Record) error {
	accepted, err := c.ReadStorageRecord(ctx, ref, wanted.ObjectID, wanted.Role)
	if err != nil {
		return err
	}
	defer clear(accepted.Payload)
	if !wanted.MatchesExact(accepted) {
		return ErrIdentity
	}
	return ctx.Err()
}

// sendAdmittedStoragePQWire returns handled=false only for an unconfigured
// legacy/compatibility caller. Strict SDK2 has no fallback when admission fails.
//
//nolint:gocyclo // retained-frame admission protocol: each step persists before the next and has no fallback; sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func (c *Client) sendAdmittedStoragePQWire(ctx context.Context, target storage.PublicationTarget, transaction, kind string, envelope pqc.Envelope) (EventRef, bool, error) {
	fixed, err := c.admissionFor(ctx)
	if err != nil {
		return EventRef{}, true, err
	}
	if fixed == nil {
		return EventRef{}, false, nil
	}
	work, ok := ctx.Value(storageRecordAdmissionKey{}).(*storageRecordAdmission)
	if !ok || work == nil || work.fixed != fixed || work.record.Target != target {
		return EventRef{}, true, ErrIdentity
	}
	txn := "sytue-storage-" + transaction
	snapshot, exists := work.record.Frames[txn]
	if exists {
		ref, done, err := c.checkExistingAdmittedStorageFrame(ctx, fixed, work, snapshot, target, txn, kind)
		if err != nil || done {
			return ref, true, err
		}
	}
	var prepared *PreparedEncryptedPublication
	if exists {
		prepared = c.resumePreparedPublication(ctx, snapshot.Binding, snapshot.Wire, snapshot.Provenance)
	} else {
		content := storagePQContentFor(kind, envelope)
		prepared, err = c.prepareEncryptedPublication(ctx, target.RoomID, event.EventMessage, content, txn)
		if err != nil {
			return EventRef{}, true, err
		}
		snapshot = retainedStorageFrame{Kind: kind, Binding: prepared.Binding(), Provenance: prepared.Provenance(), Wire: prepared.Wire()}
		work.record.Frames[txn] = snapshot
		if err := work.persist(); err != nil {
			prepared.Close()
			return EventRef{}, true, err
		}
	}
	defer prepared.Close()
	session, err := c.StoragePublicationSession(ctx, fixed.scope, fixed.authority, fixed.lease)
	if err != nil {
		return EventRef{}, true, fmt.Errorf("native publication session: %w", err)
	}
	signed, err := prepared.SignAdmission(ctx, session)
	if err != nil {
		return EventRef{}, true, fmt.Errorf("retained publication signature: %w", err)
	}
	if snapshot.Request != nil && snapshot.Request.Request != signed.Request {
		return EventRef{}, true, ErrIdentity
	}
	if snapshot.Request == nil {
		snapshot.Request = &signed
		work.record.Frames[txn] = snapshot
		if err := work.persist(); err != nil {
			return EventRef{}, true, err
		}
	}
	ref, err := prepared.Send(ctx, c.buildStorageAdmissionHook(fixed, prepared, snapshot))
	var accepted *storedStorageAcceptance
	if errors.As(err, &accepted) {
		ref = accepted.reference
		err = nil
	}
	if err != nil {
		return ref, true, err
	}
	storageRef := ref
	if err := c.verifyPreparedStorageReceipt(ctx, snapshot, storageRef); err != nil {
		return EventRef{}, true, err
	}
	snapshot.Reference = &storageRef
	work.record.Frames[txn] = snapshot
	if err := work.persist(); err != nil {
		return EventRef{}, true, err
	}
	if err := fixed.authorize(ctx); err != nil {
		return EventRef{}, true, err
	}
	return ref, true, nil
}

func (c *Client) checkExistingAdmittedStorageFrame(ctx context.Context, fixed *storageAdmission, work *storageRecordAdmission, snapshot retainedStorageFrame, target storage.PublicationTarget, txn, kind string) (EventRef, bool, error) {
	if snapshot.Kind != kind || snapshot.Binding.TransactionID != txn || snapshot.Binding.RoomID != target.RoomID || snapshot.Binding.Tenant != fixed.scope.Tenant || snapshot.Binding.Subject != fixed.scope.Subject || snapshot.Binding.MatrixUserID != c.cfg.Bootstrap.MatrixUserID || snapshot.Binding.DeviceID != target.DeviceID || !snapshot.Provenance.Valid() {
		return EventRef{}, true, ErrIdentity
	}
	if err := validateRetainedStorageFrame(snapshot, fixed.scope, target, txn); err != nil {
		return EventRef{}, true, fmt.Errorf("retained publication binding: %w", err)
	}
	if snapshot.Reference != nil {
		if err := c.verifyPreparedStorageReceipt(ctx, snapshot, *snapshot.Reference); err != nil {
			return EventRef{}, true, err
		}
		return *snapshot.Reference, true, nil
	}
	if snapshot.Request == nil {
		return EventRef{}, false, nil
	}
	receipt, err := fixed.service.ReadMatrixStoragePublication(ctx, snapshot.Request.Request)
	if err != nil {
		// Unknown status is not a cancellation. An idempotent reserve of the SAME
		// retained signature below may recover its own reserved ACK; never a new wire.
		return EventRef{}, false, nil //nolint:nilerr // deliberate: an unknown status is not a failure; the idempotent reserve of the SAME retained signature recovers it
	}
	receipt, err = validatedStorageReceipt(receipt, snapshot.Request.Request)
	if err != nil {
		return EventRef{}, true, err
	}
	if receipt.Status == "accepted" {
		ref := *receipt.Reference
		if err := c.verifyPreparedStorageReceipt(ctx, snapshot, ref); err != nil {
			return EventRef{}, true, err
		}
		snapshot.Reference = &ref
		work.record.Frames[txn] = snapshot
		if err := work.persist(); err != nil {
			return EventRef{}, true, err
		}
		return ref, true, nil
	}
	if receipt.Status != "reserved" {
		return EventRef{}, true, ErrStoragePublicationPending
	}
	return EventRef{}, false, nil
}

func (c *Client) buildStorageAdmissionHook(fixed *storageAdmission, prepared *PreparedEncryptedPublication, snapshot retainedStorageFrame) PublicationAdmissionHook {
	return func(ctx context.Context, p *PreparedEncryptedPublication) (publication.Marker, error) {
		if p != prepared {
			return publication.Marker{}, ErrIdentity
		}
		receipt, err := fixed.service.ReserveMatrixStoragePublication(ctx, *snapshot.Request)
		if err != nil {
			receipt, err = fixed.service.ReadMatrixStoragePublication(ctx, snapshot.Request.Request)
		}
		if err != nil {
			return publication.Marker{}, err
		}
		receipt, err = validatedStorageReceipt(receipt, snapshot.Request.Request)
		if err != nil {
			return publication.Marker{}, err
		}
		if receipt.Status == "accepted" {
			return publication.Marker{}, &storedStorageAcceptance{reference: *receipt.Reference}
		}
		if receipt.Status != "reserved" {
			return publication.Marker{}, ErrStoragePublicationPending
		}
		return receipt.ReservedFor(snapshot.Request.Request)
	}
}

type storedStorageAcceptance struct{ reference storage.Reference }

func (e *storedStorageAcceptance) Error() string {
	return "matrix: retained publication already accepted"
}

func validatedStorageReceipt(receipt publication.Receipt, expected publication.Request) (publication.Receipt, error) {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return publication.Receipt{}, err
	}
	defer clear(raw)
	return publication.DecodeReceipt(raw, expected)
}

// A control receipt is only a hint. Verify the exact original SDK wire and
// sender/session before any semantic Go record/fragment readback acknowledges it.
// This is READ ONLY: it does not revive an old outbound session or producer.
func (c *Client) verifyPreparedStorageReceipt(ctx context.Context, snapshot retainedStorageFrame, ref storage.Reference) error {
	if !ref.Valid() || ref.RoomID != snapshot.Binding.RoomID {
		return ErrIdentity
	}
	if err := storagePublicationPolicy(ctx); err != nil {
		return err
	}
	encrypted, err := c.cli.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
	if err != nil {
		return err
	}
	if encrypted.Type != event.EventEncrypted || encrypted.ID.String() != ref.EventID || encrypted.Sender.String() != snapshot.Binding.MatrixUserID || encrypted.RoomID != "" && encrypted.RoomID.String() != ref.RoomID {
		return ErrIdentity
	}
	raw, err := eventContentBytes(encrypted)
	if err != nil {
		return err
	}
	defer clear(raw)
	expected, _, e1 := publication.WireDigest(snapshot.Wire, false)
	actual, _, e2 := publication.WireDigest(raw, false)
	if e1 != nil || e2 != nil || expected != actual {
		return ErrIdentity
	}
	var wire event.EncryptedEventContent
	if json.Unmarshal(raw, &wire) != nil || wire.SessionID.String() != snapshot.Provenance.SessionID || wire.SenderKey.String() != snapshot.Provenance.SenderKey {
		return ErrIdentity
	}
	if err := storagePublicationPolicy(ctx); err != nil {
		return err
	}
	fixed, err := c.admissionFor(ctx)
	if err != nil {
		return err
	}
	if fixed == nil {
		return ErrIdentity
	}
	return fixed.authorize(ctx)
}

func validateRetainedStorageFrame(snapshot retainedStorageFrame, scope storage.Scope, target storage.PublicationTarget, txn string) error {
	if snapshot.Binding.SessionID != snapshot.Provenance.SessionID || snapshot.Binding.MatrixUserID != snapshot.Provenance.SenderUser || snapshot.Binding.RoomID != snapshot.Provenance.RoomID {
		return ErrIdentity
	}
	var wire event.EncryptedEventContent
	if json.Unmarshal(snapshot.Wire, &wire) != nil || wire.SessionID.String() != snapshot.Provenance.SessionID || wire.SenderKey.String() != snapshot.Provenance.SenderKey {
		return ErrIdentity
	}
	if snapshot.Request != nil {
		expected, err := publication.BuildRequest(scope, "storage:"+scope.Product, target, txn, snapshot.Request.AuthorityGeneration, snapshot.Wire)
		if err != nil {
			return err
		}
		defer clear(expected.Payload)
		if snapshot.Request.Request != expected.Request || len(snapshot.Request.Signatures) == 0 {
			return ErrIdentity
		}
	} else {
		if _, _, err := publication.WireDigest(snapshot.Wire, false); err != nil {
			return err
		}
	}
	return nil
}

// recoverAdmittedStorageRecord is ACK-only for an original target that is no
// longer this device. A current B never signs, sends or claims A's SDK identity.
func (c *Client) recoverAdmittedStorageRecord(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record, originalError error) (EventRef, error) {
	fixed, err := c.admissionFor(ctx)
	if err != nil {
		return EventRef{}, err
	}
	if fixed == nil || !target.Valid() || !storage.Identifier(transaction) || record.Validate(fixed.scope) != nil {
		return EventRef{}, originalError
	}
	service, ok := fixed.service.(StoragePublicationRecoveryService)
	if !ok {
		return EventRef{}, originalError
	}
	c.stateMu.RLock()
	allowed := c.allowedRoom(target.RoomID) && c.storageScope() == fixed.scope
	c.stateMu.RUnlock()
	if !allowed {
		return EventRef{}, originalError
	}
	selection := publication.RecoverySelection{Scope: fixed.scope, RoomID: target.RoomID, TransactionID: "sytue-storage-" + transaction}
	hint, err := service.ReadMatrixStoragePublicationRecovery(ctx, selection)
	if err != nil {
		return EventRef{}, err
	}
	original := hint.Original()
	if original.Scope != fixed.scope || original.Target != target || original.Namespace != "storage:"+fixed.scope.Product || original.TransactionID != selection.TransactionID {
		return EventRef{}, ErrIdentity
	}
	ref, accepted := hint.AcceptedReference()
	if !accepted {
		return EventRef{}, ErrStoragePublicationPending
	}
	if err := c.verifyRecoveredPublicationEvent(ctx, ref, original.CipherSHA256, record); err != nil {
		return EventRef{}, err
	}
	if err := fixed.authorize(ctx); err != nil {
		return EventRef{}, err
	}
	key := target.RoomID + "\x00" + target.DeviceID + "\x00" + transaction
	hash, err := storage.RecordDigest(target.RoomID, record)
	if err != nil {
		return EventRef{}, err
	}
	if err := persistRecoveredStorageRecord(ctx, fixed, target, transaction, key, hash, ref); err != nil {
		return EventRef{}, err
	}
	if err := fixed.authorize(ctx); err != nil {
		return EventRef{}, err
	}
	return ref, nil
}

func (c *Client) verifyRecoveredPublicationEvent(ctx context.Context, ref storage.Reference, expectedCipherSHA256 string, record storage.Record) error {
	encrypted, err := c.cli.GetEvent(ctx, id.RoomID(ref.RoomID), id.EventID(ref.EventID))
	if err != nil {
		return err
	}
	if encrypted.Type != event.EventEncrypted || encrypted.ID.String() != ref.EventID || encrypted.Sender.String() != c.cfg.Bootstrap.MatrixUserID || encrypted.RoomID != "" && encrypted.RoomID.String() != ref.RoomID {
		return ErrIdentity
	}
	raw, err := eventContentBytes(encrypted)
	if err != nil {
		return err
	}
	defer clear(raw)
	actual, _, err := publication.WireDigest(raw, false)
	if err != nil || actual != expectedCipherSHA256 {
		return ErrIdentity
	}
	return c.verifyStoredRecordReceipt(ctx, ref, record)
}

func persistRecoveredStorageRecord(ctx context.Context, fixed *storageAdmission, target storage.PublicationTarget, transaction, key, hash string, ref storage.Reference) error {
	return fixed.journal.Exclusive(ctx, fixed.owner(), storage.NativePublicationNamespace, key, func() error {
		var retained retainedStoragePublication
		found, err := fixed.journal.Read(fixed.owner(), storage.NativePublicationHistoryNamespace, key, &retained)
		if err != nil {
			return err
		}
		if !found {
			found, err = fixed.journal.Read(fixed.owner(), storage.NativePublicationNamespace, key, &retained)
			if err != nil {
				return err
			}
		}
		if found {
			if retained.Version != 1 || retained.Scope != fixed.scope || retained.Target != target || retained.Transaction != transaction || retained.RecordSHA256 != hash || retained.Reference != nil && *retained.Reference != ref {
				return ErrIdentity
			}
		} else {
			retained = retainedStoragePublication{Version: 1, Scope: fixed.scope, Target: target, Transaction: transaction, RecordSHA256: hash, Frames: make(map[string]retainedStorageFrame)}
		}
		retained.Reference = &ref
		if err := fixed.journal.Write(fixed.owner(), storage.NativePublicationHistoryNamespace, key, &retained); err != nil {
			return err
		}
		return fixed.journal.Delete(fixed.owner(), storage.NativePublicationNamespace, key)
	})
}
