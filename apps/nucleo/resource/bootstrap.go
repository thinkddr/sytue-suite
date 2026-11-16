// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// Baseline is the immutable original encrypted history anchor. Position retains
// the complete original sequence, including a snapshot of a zero-op document.
// It is never a reference to the creator's personal vault or whole source scope.
type Baseline struct {
	Position             Position          `json:"position"`
	Reference            storage.Reference `json:"reference"`
	ContextSHA256        string            `json:"context_sha256"`
	SessionCertificateID string            `json:"session_certificate_id"`
}

func (b Baseline) Valid() bool {
	return b.Position.Valid() && digest(b.Position.HeadCipherSHA256) && b.Reference.Valid() && digest(b.ContextSHA256) && storage.Identifier(b.SessionCertificateID)
}

// Checkpoint contains current application state at SourceCutover, encrypted for
// THIS epoch's recipient. It is distinct from historical Baseline: a joined-only
// member gets current state without old epoch keys. Rooms current state contains
// channel metadata/settings only, never pre-join conversation transcripts.
// SourceCutover is the logical chain head; CipherSHA256 hashes the new snapshot.
// Replacing a snapshot does not manufacture a new operation or rewrite that head.
type Checkpoint struct {
	SourceCutover        Position          `json:"source_cutover"`
	Reference            storage.Reference `json:"reference"`
	CipherSHA256         string            `json:"cipher_sha256"`
	ContextSHA256        string            `json:"context_sha256"`
	SessionCertificateID string            `json:"session_certificate_id"`
}

func (c Checkpoint) Valid() bool {
	return c.SourceCutover.Valid() && c.Reference.Valid() && digest(c.CipherSHA256) && digest(c.ContextSHA256) && storage.Identifier(c.SessionCertificateID)
}

type BootstrapKind string

const (
	BootstrapBaseline           BootstrapKind = "baseline"
	BootstrapCheckpoint         BootstrapKind = "checkpoint"
	SnapshotHistoryArchive                    = "history_archive"
	SnapshotCurrentState                      = "current_state"
	MaxBootstrapDescriptorBytes               = 64 << 10
)

// BootstrapContext is PUBLIC routing/commitments encoded by this Go codec. The
// payload and file keys remain inside the hybrid envelope. Store canonical bytes
// by ContextSHA256 in public evidence so a recovered device can open the age
// envelope before it has plaintext. Epoch policy hash is deliberately absent:
// the SDK certificate binds this pre-cipher context hash; the final signed epoch
// binds the accepted reference and cipher hash, avoiding a hash cycle.
type BootstrapContext struct {
	Version         int           `json:"v"`
	Domain          string        `json:"domain"`
	Kind            BootstrapKind `json:"kind"`
	Resource        Resource      `json:"resource"`
	RoomID          string        `json:"room_id"`
	Epoch           string        `json:"epoch"`
	RecipientID     string        `json:"recipient_id"`
	SourceCutover   Position      `json:"source_cutover"`
	CreatorScope    storage.Scope `json:"creator_scope"`
	Creator         Actor         `json:"creator"`
	SnapshotKind    string        `json:"snapshot_kind"`
	SnapshotVersion int           `json:"snapshot_version"`
}

func (c BootstrapContext) Valid() bool {
	if c.Version != 1 || c.Domain != "org.sytue.resource.bootstrap.v1" || !c.Resource.Valid() || !matrixID(c.RoomID, '!') || !storage.Identifier(c.Epoch) || !digest(c.RecipientID) || !c.CreatorScope.Valid() || c.CreatorScope.Tenant != c.Resource.Tenant || c.CreatorScope.Product != c.Resource.Product || !c.Creator.Valid() || c.SnapshotVersion != 1 {
		return false
	}
	if _, err := c.SourceCutover.Seq.Int64(); err != nil {
		return false
	}
	switch c.Kind {
	case BootstrapBaseline:
		return c.SnapshotKind == SnapshotHistoryArchive && c.SourceCutover.HeadCipherSHA256 == ""
	case BootstrapCheckpoint:
		return c.SnapshotKind == SnapshotCurrentState && c.SourceCutover.Valid()
	default:
		return false
	}
}

func NewBootstrapContext(kind BootstrapKind, value Resource, room, epoch, recipient string, source Position, creatorScope storage.Scope, creator Actor) (BootstrapContext, error) {
	public, err := pqc.ParseRecipient(pqc.HybridProfile, recipient)
	if err != nil {
		return BootstrapContext{}, err
	}
	keyID, err := pqc.RecipientID(public)
	if err != nil {
		return BootstrapContext{}, err
	}
	snapshot := SnapshotCurrentState
	if kind == BootstrapBaseline {
		snapshot = SnapshotHistoryArchive
	}
	out := BootstrapContext{Version: 1, Domain: "org.sytue.resource.bootstrap.v1", Kind: kind, Resource: value, RoomID: room, Epoch: epoch, RecipientID: keyID, SourceCutover: source, CreatorScope: creatorScope, Creator: creator, SnapshotKind: snapshot, SnapshotVersion: 1}
	if !out.Valid() {
		return BootstrapContext{}, ErrInvalid
	}
	return out, nil
}

// PrepareCheckpointContext requires a previously verified manager. It prepares
// routing only and never manufactures a VerifiedEpoch or an authorization from
// raw JSON. The managed workflow must fence/drain current authority before and
// after reading/validating state, sealing and accepting the SDK ciphertext.
func PrepareCheckpointContext(previous VerifiedAccess, nextEpoch, nextRecipient string, cutover Position) (BootstrapContext, error) {
	if !previous.valid() || previous.action != Manage || !cutover.Valid() || nextEpoch == previous.epoch.value.ID || nextRecipient == previous.epoch.value.Recipient {
		return BootstrapContext{}, ErrInvalid
	}
	seq, _ := cutover.Seq.Int64()
	start, _ := previous.epoch.value.Cutover.Seq.Int64()
	if seq < start {
		return BootstrapContext{}, ErrInvalid
	}
	return NewBootstrapContext(BootstrapCheckpoint, previous.epoch.value.Resource, previous.epoch.value.RoomID, nextEpoch, nextRecipient, cutover, previous.Scope(), previous.Actor())
}

// DecodeBootstrapContext is the single strict public routing codec for native
// and Wasm. Unknown fields, numeric sequences and trailing values are rejected.
func DecodeBootstrapContext(raw []byte) (BootstrapContext, error) {
	if len(raw) == 0 || len(raw) > 16<<10 {
		return BootstrapContext{}, ErrInvalid
	}
	var value BootstrapContext
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || !value.Valid() {
		return BootstrapContext{}, ErrInvalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return BootstrapContext{}, ErrInvalid
	}
	return value, nil
}

func BootstrapContextBytes(value BootstrapContext) ([]byte, error) {
	if !value.Valid() {
		return nil, ErrInvalid
	}
	return matrixproof.CanonicalBytes(value)
}

func BootstrapContextSHA256(value BootstrapContext) (string, error) {
	raw, err := BootstrapContextBytes(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ValidateInitialBaseline checks the public pre-cipher binding. Independently
// pinned owner/roster signatures and actual SDK acceptance/certificates are still
// required by the managed workflow; structure alone is not an authority grant.
func ValidateInitialBaseline(epoch Epoch, anchor Anchor) error {
	if !epoch.Valid() || epoch.PreviousEpoch != "" || epoch.Generation != "1" || epoch.Resource != anchor.Resource || epoch.Issuer != anchor.Owner || epoch.Cutover != anchor.Baseline || epoch.Baseline.Position != anchor.Baseline || epoch.Checkpoint.SourceCutover != epoch.Baseline.Position {
		return ErrInvalid
	}
	pinned, err := EpochDigest(epoch)
	if err != nil || pinned != anchor.PolicySHA256 {
		return ErrInvalid
	}
	source := Position{Seq: epoch.Baseline.Position.Seq, HeadCipherSHA256: ""}
	baseline, err := NewBootstrapContext(BootstrapBaseline, epoch.Resource, epoch.RoomID, epoch.ID, epoch.Recipient, source, anchor.OwnerScope, anchor.Owner)
	if err != nil {
		return err
	}
	hash, err := BootstrapContextSHA256(baseline)
	if err != nil || hash != epoch.Baseline.ContextSHA256 {
		return ErrInvalid
	}
	checkpoint, err := NewBootstrapContext(BootstrapCheckpoint, epoch.Resource, epoch.RoomID, epoch.ID, epoch.Recipient, epoch.Cutover, anchor.OwnerScope, anchor.Owner)
	if err != nil {
		return err
	}
	hash, err = BootstrapContextSHA256(checkpoint)
	if err != nil || hash != epoch.Checkpoint.ContextSHA256 {
		return ErrInvalid
	}
	return nil
}

// ValidateCheckpointContext binds current-state snapshot routing to the actual
// verified manager scope/SDK actor. The canonical context evidence must hash to
// this signed commitment before a recovered recipient opens the descriptor.
func ValidateCheckpointContext(epoch Epoch, scope storage.Scope, creator Actor) error {
	if !epoch.Valid() || creator != epoch.Issuer {
		return ErrInvalid
	}
	value, err := NewBootstrapContext(BootstrapCheckpoint, epoch.Resource, epoch.RoomID, epoch.ID, epoch.Recipient, epoch.Cutover, scope, creator)
	if err != nil {
		return err
	}
	hash, err := BootstrapContextSHA256(value)
	if err != nil || hash != epoch.Checkpoint.ContextSHA256 {
		return ErrInvalid
	}
	return nil
}

// ValidateBaselineTransition preserves original history exactly and binds the
// new current-state snapshot to the new epoch and old committed logical head.
// Issuer/manager scope and SDK provenance are verified by the authority layer.
func ValidateBaselineTransition(previous, next Epoch) error {
	if !previous.Valid() || !next.Valid() || next.Resource != previous.Resource || next.RoomID != previous.RoomID || next.Baseline != previous.Baseline || next.Recipient == previous.Recipient || next.Checkpoint.Reference == previous.Checkpoint.Reference || next.Checkpoint.SourceCutover != next.Cutover {
		return ErrInvalid
	}
	return nil
}

// SealBootstrapDescriptor/OpenBootstrapDescriptor are CRYPTO ONLY, not grants or
// SDK provenance verification. The same maintained age ML-KEM768+X25519 profile
// protects a small descriptor; large state/history/originals use scoped BoundFiles
// and file keys inside this descriptor. This crypto bound is not a Matrix PDU
// fit guarantee: transport must measure the complete encrypted event and spill
// oversized descriptors/state into scoped BoundFiles.
func SealBootstrapDescriptor(recipient pqc.Recipient, context BootstrapContext, payload []byte) (pqc.Envelope, error) {
	if recipient == nil || len(payload) > MaxBootstrapDescriptorBytes {
		return pqc.Envelope{}, ErrInvalid
	}
	public, err := pqc.ParseRecipient(pqc.HybridProfile, recipient.String())
	if err != nil {
		return pqc.Envelope{}, err
	}
	keyID, err := pqc.RecipientID(public)
	if err != nil || keyID != context.RecipientID {
		return pqc.Envelope{}, ErrInvalid
	}
	raw, err := BootstrapContextBytes(context)
	if err != nil {
		return pqc.Envelope{}, err
	}
	return pqc.Seal(public, raw, payload)
}

func OpenBootstrapDescriptor(identity *pqc.Identity, context BootstrapContext, envelope pqc.Envelope) ([]byte, error) {
	if identity == nil || identity.Profile() != pqc.HybridProfile || envelope.Profile != pqc.HybridProfile {
		return nil, ErrInvalid
	}
	keyID, err := pqc.RecipientID(identity.Recipient())
	if err != nil || keyID != context.RecipientID {
		return nil, ErrInvalid
	}
	raw, err := BootstrapContextBytes(context)
	if err != nil {
		return nil, err
	}
	payload, err := pqc.Open(identity, raw, envelope)
	if err != nil {
		return nil, err
	}
	if len(payload) > MaxBootstrapDescriptorBytes {
		clear(payload)
		return nil, ErrInvalid
	}
	return payload, nil
}
