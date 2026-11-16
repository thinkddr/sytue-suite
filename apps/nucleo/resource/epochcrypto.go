// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"
	"sync/atomic"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// Fence is the product-scoped host broker's current account/device/policy lease.
// Implementations also bind SytueID Scope + Matrix user + enrolled SDK device;
// another application's globally cross-signed Matrix device is not a substitute.
type Fence interface {
	Revalidate(context.Context, VerifiedAccess) error
}

func checkFence(ctx context.Context, fence Fence, access VerifiedAccess) error {
	if !access.valid() || fence == nil {
		return ErrRevoked
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := revalidateAccessSession(ctx, access); err != nil {
		return err
	}
	if err := fence.Revalidate(ctx, access); err != nil {
		return err
	}
	return revalidateAccessSession(ctx, access)
}

type EpochWindow struct {
	epoch   VerifiedEpoch
	through *Position
}

func CurrentEpoch(epoch VerifiedEpoch) (EpochWindow, error) {
	if !epoch.valid() {
		return EpochWindow{}, ErrInvalid
	}
	return EpochWindow{epoch: epoch}, nil
}

// ClosedEpoch binds historical keys to the immediate signed successor cutover.
// Old devices cannot create new accepted events outside this authenticated head.
func ClosedEpoch(epoch, next VerifiedEpoch) (EpochWindow, error) {
	if !epoch.valid() || !next.valid() || next.parent == nil || next.parent.hash != epoch.hash || next.value.PreviousEpoch != epoch.value.ID || next.value.PreviousPolicySHA256 != epoch.hash || next.value.Resource != epoch.value.Resource || next.value.RoomID != epoch.value.RoomID {
		return EpochWindow{}, ErrInvalid
	}
	through := next.value.Cutover
	return EpochWindow{epoch: epoch, through: &through}, nil
}

func (w EpochWindow) contains(publication Publication) bool {
	if !w.epoch.valid() || !publication.Valid() || publication.Resource != w.epoch.value.Resource || publication.Epoch != w.epoch.value.ID || publication.PolicySHA256 != w.epoch.hash {
		return false
	}
	base, err := publication.ExpectedBase.Seq.Int64()
	if err != nil {
		return false
	}
	first, _ := w.epoch.value.Cutover.Seq.Int64()
	if base < first || base == first && publication.ExpectedBase != w.epoch.value.Cutover {
		return false
	}
	if w.through != nil {
		end, err := publication.End()
		if err != nil {
			return false
		}
		last, _ := end.Int64()
		through, _ := w.through.Seq.Int64()
		if last > through {
			return false
		}
	}
	return true
}

// GrantContext distinguishes the key epoch from the current access epoch. This
// permits explicit/full historical grants without pretending a new member was
// part of an old roster, while joined-only never receives prior epoch keys.
type GrantContext struct {
	Domain             string        `json:"domain"`
	Version            int           `json:"v"`
	Resource           Resource      `json:"resource"`
	RoomID             string        `json:"room_id"`
	KeyEpoch           string        `json:"key_epoch"`
	KeyGeneration      Decimal       `json:"key_generation"`
	KeyPolicySHA256    string        `json:"key_policy_sha256"`
	KeyRecipientID     string        `json:"key_recipient_id"`
	AccessEpoch        string        `json:"access_epoch"`
	AccessGeneration   Decimal       `json:"access_generation"`
	AccessPolicySHA256 string        `json:"access_policy_sha256"`
	MembershipSHA256   string        `json:"membership_sha256"`
	History            HistoryPolicy `json:"history"`
	ReadFrom           Position      `json:"read_from"`
	ReadThrough        *Position     `json:"read_through,omitempty"`
	RecipientScope     storage.Scope `json:"recipient_scope"`
	RecipientUser      string        `json:"recipient_user"`
	RecipientKeyID     string        `json:"recipient_key_id"`
	CapabilitySHA256   string        `json:"capability_sha256"`
	Issuer             Actor         `json:"issuer"`
}
type EpochGrant struct {
	Version  int          `json:"v"`
	Domain   string       `json:"domain"`
	Context  GrantContext `json:"context"`
	Envelope pqc.Envelope `json:"envelope"`
}
type SignedEpochGrant struct {
	EpochGrant
	Signatures signatures.Signatures `json:"signatures"`
}
type EpochHandle struct {
	window EpochWindow
	key    atomic.Pointer[pqc.Identity]
}

// BindOwnedEpoch retains the already generated hybrid private key inside the
// broker after validating its public recipient against the signed epoch. It
// exposes operations only, never a vault or private-key getter.
func BindOwnedEpoch(window EpochWindow, identity *pqc.Identity) (*EpochHandle, error) {
	if !window.epoch.valid() || identity == nil || identity.Profile() != pqc.HybridProfile || identity.Recipient().String() != window.epoch.value.Recipient {
		return nil, ErrInvalid
	}
	handle := &EpochHandle{window: window}
	handle.key.Store(identity)
	return handle, nil
}

func (h *EpochHandle) Close() {
	if h != nil {
		h.key.Store(nil)
	}
}

func (h *EpochHandle) identity() *pqc.Identity {
	if h == nil {
		return nil
	}
	return h.key.Load()
}

func (h *EpochHandle) usable(identity *pqc.Identity) bool {
	return h != nil && identity != nil && h.key.Load() == identity && h.window.epoch.valid() && identity.Profile() == pqc.HybridProfile && identity.Recipient().String() == h.window.epoch.value.Recipient
}

func validGrantEpochHistory(window EpochWindow, manager VerifiedAccess, recipient VerifiedCapability) bool {
	keyGeneration, _ := window.epoch.value.Generation.Int64()
	accessGeneration, _ := manager.epoch.value.Generation.Int64()
	if keyGeneration > accessGeneration || !isAncestor(manager.epoch, window.epoch) {
		return false
	}
	if window.epoch.hash != manager.epoch.hash {
		if window.through == nil {
			return false
		}
		if manager.epoch.value.History == HistoryJoined && !historicalMember(window.epoch, recipient.member) {
			return false
		}
	} else if window.through != nil {
		return false
	}
	floor, _ := recipient.value.From.Seq.Int64()
	first, _ := window.epoch.value.Cutover.Seq.Int64()
	return floor <= first
}

func grantContext(window EpochWindow, manager VerifiedAccess, recipient VerifiedCapability) (GrantContext, error) {
	// A fresh policy restoration can read/prepare a current signed transition,
	// but cannot re-issue a private key grant as the historical producer.
	if manager.epoch.restored != nil {
		return GrantContext{}, ErrRevoked
	}
	if !window.epoch.valid() || !manager.valid() || manager.action != Manage || !recipient.valid() || recipient.epochHash != manager.epoch.hash || !recipient.value.Allows(Read) || window.epoch.value.Resource != manager.epoch.value.Resource || window.epoch.value.RoomID != manager.epoch.value.RoomID || !validGrantEpochHistory(window, manager, recipient) {
		return GrantContext{}, ErrInvalid
	}
	recipientKey, err := pqc.ParseRecipient(pqc.HybridProfile, window.epoch.value.Recipient)
	if err != nil {
		return GrantContext{}, err
	}
	recipientID, err := pqc.RecipientID(recipientKey)
	if err != nil {
		return GrantContext{}, err
	}
	var through *Position
	if window.through != nil {
		copied := *window.through
		through = &copied
	}
	return GrantContext{Domain: "org.sytue.resource.epoch.grant-context.v1", Version: 1, Resource: window.epoch.value.Resource, RoomID: window.epoch.value.RoomID, KeyEpoch: window.epoch.value.ID, KeyGeneration: window.epoch.value.Generation, KeyPolicySHA256: window.epoch.hash, KeyRecipientID: recipientID, AccessEpoch: manager.epoch.value.ID, AccessGeneration: manager.epoch.value.Generation, AccessPolicySHA256: manager.epoch.hash, MembershipSHA256: manager.epoch.roster.hash, History: manager.epoch.value.History, ReadFrom: recipient.value.From, ReadThrough: through, RecipientScope: recipient.member.Scope, RecipientUser: recipient.value.UserID, RecipientKeyID: recipient.value.ProductKeyID, CapabilitySHA256: recipient.hash, Issuer: manager.capability.enrollment.Issuer}, nil
}

func grantContextBytes(grantCtx GrantContext) ([]byte, error) {
	return matrixproof.CanonicalBytes(grantCtx)
}

// PrepareGrant seals the epoch key to ONE independently enrolled product key.
// Call the maintained SDK signJSON on the returned object and ACK its durable
// public signed grant before publishing content for that epoch.
func (h *EpochHandle) PrepareGrant(ctx context.Context, manager VerifiedAccess, recipient VerifiedCapability, fence Fence) (EpochGrant, error) {
	identity := h.identity()
	if !h.usable(identity) {
		return EpochGrant{}, ErrInvalid
	}
	if err := checkFence(ctx, fence, manager); err != nil {
		return EpochGrant{}, err
	}
	grantCtx, err := grantContext(h.window, manager, recipient)
	if err != nil {
		return EpochGrant{}, err
	}
	raw, err := grantContextBytes(grantCtx)
	if err != nil {
		return EpochGrant{}, err
	}
	defer clear(raw)
	payload, err := json.Marshal(struct {
		Domain   string `json:"domain"`
		Identity string `json:"identity"`
	}{Domain: "org.sytue.resource.epoch.private.v1", Identity: identity.String()})
	if err != nil {
		return EpochGrant{}, err
	}
	defer clear(payload)
	key, err := pqc.ParseRecipient(pqc.HybridProfile, recipient.member.Recipient)
	if err != nil {
		return EpochGrant{}, err
	}
	envelope, err := pqc.Seal(key, raw, payload)
	if err != nil {
		return EpochGrant{}, err
	}
	if err := checkFence(ctx, fence, manager); err != nil {
		clear(envelope.Ciphertext)
		return EpochGrant{}, err
	}
	if !h.usable(identity) {
		clear(envelope.Ciphertext)
		return EpochGrant{}, ErrRevoked
	}
	return EpochGrant{Version: 1, Domain: "org.sytue.resource.epoch.grant.v1", Context: grantCtx, Envelope: envelope}, nil
}

func decodeGrantedEpochIdentity(payload []byte) (*pqc.Identity, error) {
	var private struct {
		Domain   string `json:"domain"`
		Identity string `json:"identity"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&private) != nil || private.Domain != "org.sytue.resource.epoch.private.v1" {
		return nil, ErrInvalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, ErrInvalid
	}
	key, err := pqc.ParseIdentity(private.Identity)
	private.Identity = ""
	if err != nil {
		return nil, ErrInvalid
	}
	return key, nil
}

// OpenGrant requires the recipient's actual private product key and an exact
// manager-signed current policy. Public roster or Matrix membership alone cannot
// release the epoch identity. The private key never leaves this handle.
//
//nolint:gocyclo // sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func OpenGrant(ctx context.Context, grant SignedEpochGrant, window EpochWindow, manager, recipient VerifiedAccess, identity *pqc.Identity, issuer matrixproof.Device, fence Fence) (*EpochHandle, error) {
	if !recipient.valid() || recipient.action != Read || recipient.epoch.hash != manager.epoch.hash || identity == nil || identity.Profile() != pqc.HybridProfile {
		return nil, ErrInvalid
	}
	if err := checkFence(ctx, fence, recipient); err != nil {
		return nil, err
	}
	grantCtx, err := grantContext(window, manager, recipient.capability)
	if err != nil {
		return nil, err
	}
	expected, err := grantContextBytes(grantCtx)
	if err != nil {
		return nil, err
	}
	defer clear(expected)
	actual, err := grantContextBytes(grant.Context)
	if err != nil {
		return nil, err
	}
	defer clear(actual)
	if grant.Version != 1 || grant.Domain != "org.sytue.resource.epoch.grant.v1" || !bytes.Equal(actual, expected) || !actorMatches(issuer, grantCtx.Issuer, manager.capability.member.Scope) {
		return nil, ErrInvalid
	}
	if err := issuer.VerifyJSON(ctx, manager.capability.member.Scope, storage.PublicationTarget{RoomID: grantCtx.RoomID, DeviceID: grantCtx.Issuer.DeviceID}, grant); err != nil {
		return nil, err
	}
	ownID, err := pqc.RecipientID(identity.Recipient())
	if err != nil || ownID != recipient.capability.member.ProductKeyID {
		return nil, ErrInvalid
	}
	payload, err := pqc.Open(identity, expected, grant.Envelope)
	if err != nil {
		return nil, err
	}
	defer clear(payload)
	key, err := decodeGrantedEpochIdentity(payload)
	if err != nil {
		return nil, err
	}
	handle, err := BindOwnedEpoch(window, key)
	if err != nil {
		return nil, err
	}
	if err := checkFence(ctx, fence, recipient); err != nil {
		handle.Close()
		return nil, err
	}
	return handle, nil
}

func frameContext(publication Publication) ([]byte, error) {
	if !publication.Valid() {
		return nil, ErrInvalid
	}
	return matrixproof.CanonicalBytes(publication)
}

// SealFrame keeps bounded application frames inside the shared maintained age
// profile. Larger original app payloads use the existing encrypted file/page
// layer with descriptors inside these frames; this is not a new message limit.
func (h *EpochHandle) SealFrame(ctx context.Context, publication Publication, payload []byte, access VerifiedAccess, fence Fence) (pqc.Envelope, error) {
	identity := h.identity()
	if !h.usable(identity) || !access.valid() || h.window.through != nil || h.window.epoch.hash != access.epoch.hash || publication.Action != access.action || (access.action != Publish && access.action != Comment) || publication.Actor != access.capability.enrollment.Issuer || publication.CapabilitySHA256 != access.capability.hash || !h.window.contains(publication) {
		return pqc.Envelope{}, ErrInvalid
	}
	if err := checkFence(ctx, fence, access); err != nil {
		return pqc.Envelope{}, err
	}
	frameCtx, err := frameContext(publication)
	if err != nil {
		return pqc.Envelope{}, err
	}
	defer clear(frameCtx)
	envelope, err := pqc.Seal(identity.Recipient(), frameCtx, payload)
	if err != nil {
		return pqc.Envelope{}, err
	}
	if err := checkFence(ctx, fence, access); err != nil {
		clear(envelope.Ciphertext)
		return pqc.Envelope{}, err
	}
	if !h.usable(identity) {
		clear(envelope.Ciphertext)
		return pqc.Envelope{}, ErrRevoked
	}
	return envelope, nil
}

func (h *EpochHandle) OpenFrame(ctx context.Context, publication VerifiedPendingPublication, envelope pqc.Envelope, reader VerifiedAccess, fence Fence) ([]byte, error) {
	return h.openVerifiedFrame(ctx, publication.value, publication.epoch.hash, envelope, reader, fence)
}

func (h *EpochHandle) canOpenVerifiedFrame(identity *pqc.Identity, value CipherPublication, epochHash string, reader VerifiedAccess) error {
	if !h.usable(identity) || !reader.valid() || reader.action != Read || epochHash != h.window.epoch.hash || reader.epoch.value.Resource != h.window.epoch.value.Resource || !isAncestor(reader.epoch, h.window.epoch) || !h.window.contains(value.Publication) {
		return ErrInvalid
	}
	if h.window.epoch.hash != reader.epoch.hash && h.window.through == nil {
		return ErrRevoked
	}
	end, _ := value.End()
	if h.window.through != nil && end == h.window.through.Seq && value.OuterCipherSHA256 != h.window.through.HeadCipherSHA256 {
		return ErrFork
	}
	return nil
}

func (h *EpochHandle) openVerifiedFrame(ctx context.Context, value CipherPublication, epochHash string, envelope pqc.Envelope, reader VerifiedAccess, fence Fence) ([]byte, error) {
	identity := h.identity()
	if err := h.canOpenVerifiedFrame(identity, value, epochHash, reader); err != nil {
		return nil, err
	}
	if err := checkFence(ctx, fence, reader); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	hash, err := CipherJSONDigest(raw)
	clear(raw)
	if err != nil || hash != value.InnerCipherSHA256 {
		return nil, ErrInvalid
	}
	frameCtx, err := frameContext(value.Publication)
	if err != nil {
		return nil, err
	}
	defer clear(frameCtx)
	payload, err := pqc.Open(identity, frameCtx, envelope)
	if err != nil {
		return nil, err
	}
	if err := checkFence(ctx, fence, reader); err != nil {
		clear(payload)
		return nil, err
	}
	if !h.usable(identity) {
		clear(payload)
		return nil, ErrRevoked
	}
	return payload, nil
}

func historicalMember(epoch VerifiedEpoch, recipient Member) bool {
	for _, member := range epoch.roster.members {
		if member.Scope == recipient.Scope && member.UserID == recipient.UserID && member.RootKey == recipient.RootKey && slices.Contains(member.Actions, Read) {
			return true
		}
	}
	return false
}
