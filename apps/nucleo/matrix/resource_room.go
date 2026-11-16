// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// ResourceRoom is an operation-scoped capability. It does not add a raw room to
// Bootstrap, broaden generic Read/Send/History, or expose the product leaf key.
// ActorScope is the enrolled producer; Resource identifies the owning resource.
// The host supplies its original ACL/CAS fence, never a server roster as a grant.
type ResourceRoom struct {
	owner   *Client
	current resource.VerifiedSession
	access  resource.VerifiedAccess
	handle  *resource.EpochHandle
	fence   resource.Fence
	session *PublicationSession
	journal *offline.Store
	retired atomic.Bool
}

type resourceRoomKey struct{}

func (c *Client) BindResourceRoom(ctx context.Context, current resource.VerifiedSession, access resource.VerifiedAccess, handle *resource.EpochHandle, fence resource.Fence, lease offline.WorkLease, journal *offline.Store) (*ResourceRoom, error) {
	if c == nil || c.cfg.ResourceProduct == "" || handle == nil || fence == nil || !journal.Encrypted() {
		return nil, ErrIdentity
	}
	binding, ok := access.CurrentSessionBinding()
	if !ok || binding != current.Binding() || access.Scope() != c.resourceScope() || access.Actor() != current.Actor() || access.Epoch().Value().Resource.Product != c.cfg.ResourceProduct {
		return nil, ErrIdentity
	}
	checked, err := resource.Reauthorize(ctx, access, c.cfg.Subject, access.Action())
	if err != nil {
		return nil, err
	}
	session, err := c.ResourcePublicationSession(ctx, current, lease)
	if err != nil {
		return nil, err
	}
	c.stateMu.Lock()
	policy, policyErr := c.currentOutboundRecipientPolicy()
	if policyErr == nil {
		policyErr = policy.ensureOutboundRecipientPolicy(ctx, map[id.RoomID]struct{}{id.RoomID(access.Epoch().Value().RoomID): {}})
	}
	c.stateMu.Unlock()
	if policyErr != nil {
		return nil, policyErr
	}
	room := &ResourceRoom{owner: c, current: current, access: checked, handle: handle, fence: fence, session: session, journal: journal}
	if err := room.Revalidate(ctx); err != nil {
		return nil, err
	}
	return room, nil
}

func (r *ResourceRoom) Close() {
	if r != nil {
		r.retired.Store(true)
	}
}

func (r *ResourceRoom) Resource() resource.Resource {
	if r == nil {
		return resource.Resource{}
	}
	return r.access.Epoch().Value().Resource
}

func (r *ResourceRoom) ActorScope() storage.Scope {
	if r == nil {
		return storage.Scope{}
	}
	return r.access.Scope()
}

func (r *ResourceRoom) RoomID() string {
	if r == nil {
		return ""
	}
	return r.access.Epoch().Value().RoomID
}

// Revalidate never runs under an SDK/state or work-lease lock: the Provider and
// original product fence can themselves use the SDK, persistence or HTTP.
func (r *ResourceRoom) Revalidate(ctx context.Context) error {
	if r == nil || r.owner == nil || r.retired.Load() {
		return ErrClosed
	}
	if err := r.current.Revalidate(ctx); err != nil {
		return err
	}
	if _, err := resource.Reauthorize(ctx, r.access, r.owner.cfg.Subject, r.access.Action()); err != nil {
		return err
	}
	if err := r.fence.Revalidate(ctx, r.access); err != nil {
		return err
	}
	return r.current.Revalidate(ctx)
}

func (r *ResourceRoom) operationContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, resourceRoomKey{}, r)
}

func (c *Client) allowedRoomContext(ctx context.Context, room string) bool {
	if c.cfg.ResourceProduct == "" {
		return c.allowedRoom(room)
	}
	r, ok := ctx.Value(resourceRoomKey{}).(resourceOperation)
	return ok && r.operationActive() && r.operationOwner() == c && !c.closed.Load() && r.RoomID() == room && r.ActorScope() == c.resourceScope() && r.operationBinding().DeviceID == c.session.DeviceID.String() && r.operationBinding().UserID == c.session.UserID.String()
}

func publicationOperationPolicy(ctx context.Context) error {
	if fixed, ok := ctx.Value(storageAdmissionKey{}).(*storageAdmission); ok {
		if fixed == nil {
			return ErrIdentity
		}
		if err := fixed.authorize(ctx); err != nil {
			return err
		}
	}
	if err := storagePublicationPolicy(ctx); err != nil {
		return err
	}
	if r, ok := ctx.Value(resourceRoomKey{}).(resourceOperation); ok {
		return r.Revalidate(ctx)
	}
	return ctx.Err()
}

type resourceLocalPublication = resource.PublicationWork

// PreparedResourcePublication contains an immutable durable Candidate. It has no
// direct Send method: only the common CAS/hold relay may publish the SDK wire.
// Admission signatures remain bound to this exact enrolled SDK/device/intent.
type PreparedResourcePublication struct {
	room      *ResourceRoom
	prepared  *PreparedEncryptedPublication
	candidate resource.Candidate
	receipt   *resource.Receipt
	closed    atomic.Bool
}

func (p *PreparedResourcePublication) Close() {
	if p != nil {
		p.closed.Store(true)
		if p.prepared != nil {
			p.prepared.Close()
		}
	}
}

func cloneResourceCandidate(value resource.Candidate) (resource.Candidate, error) {
	return cloneResourceJSON(value)
}

func (p *PreparedResourcePublication) Candidate() (resource.Candidate, error) {
	if p == nil || p.room == nil || p.closed.Load() || p.prepared == nil && p.receipt == nil || p.prepared != nil && p.prepared.retired.Load() {
		return resource.Candidate{}, ErrClosed
	}
	return cloneResourceCandidate(p.candidate)
}

func (p *PreparedResourcePublication) Revalidate(ctx context.Context) error {
	if p == nil || p.room == nil || p.closed.Load() || p.prepared == nil && p.receipt == nil {
		return ErrIdentity
	}
	if err := p.room.Revalidate(ctx); err != nil {
		return err
	}
	if p.receipt != nil {
		return ctx.Err()
	}
	return p.prepared.Revalidate(ctx)
}

func (p *PreparedResourcePublication) SignAdmission(ctx context.Context) (publication.SignedRequest, error) {
	if err := p.Revalidate(ctx); err != nil {
		return publication.SignedRequest{}, err
	}
	if p.receipt != nil {
		return publication.SignedRequest{}, ErrResourcePublicationAcknowledged
	}
	return p.prepared.SignAdmission(ctx, p.room.session)
}

// Prepare first commits the original txn/header/payload to encrypted local WAL.
// It then commits the exact SDK Candidate before returning it to Stage. A crash
// after either write resumes without a new txn/ciphertext. Changed intent, key,
// epoch or producer stays pending and must be explicitly rebased by the app.
func (r *ResourceRoom) Prepare(ctx context.Context, header resource.Publication, payload []byte) (*PreparedResourcePublication, error) {
	if err := r.Revalidate(ctx); err != nil {
		return nil, err
	}
	epoch := r.access.Epoch().Value()
	if !header.Valid() || header.Resource != r.Resource() || header.RoomID != r.RoomID() || header.Epoch != epoch.ID || header.PolicySHA256 != r.access.Epoch().SHA256() || header.Generation != epoch.Generation || header.MembershipSHA256 != epoch.MembershipSHA256 || header.CapabilitySHA256 != r.access.Capability().SHA256() || header.Action != r.access.Action() || header.Actor != r.current.Actor() {
		return nil, ErrIdentity
	}
	namespace := storage.ResourcePublicationNamespace
	owner, err := r.ActorScope().JournalOwner()
	if err != nil {
		return nil, err
	}
	workKey, err := resource.PublicationWorkKey(r.Resource(), header.OpID)
	if err != nil {
		return nil, err
	}
	var result *PreparedResourcePublication
	err = r.journal.Exclusive(ctx, owner, namespace, workKey, func() error {
		var err error
		result, err = r.prepareLocked(ctx, owner, namespace, workKey, header, payload)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := result.Revalidate(ctx); err != nil {
		result.Close()
		return nil, err
	}
	return result, nil
}

func (r *ResourceRoom) prepareLocked(ctx context.Context, owner, namespace, workKey string, header resource.Publication, payload []byte) (*PreparedResourcePublication, error) {
	var completed resource.AcknowledgedPublicationWork
	historical, err := r.journal.Read(owner, storage.ResourcePublicationHistoryNamespace, workKey, &completed)
	if err != nil {
		return nil, err
	}
	if historical {
		defer clear(completed.Work.Payload)
		if !completed.Valid() || !completed.Work.Matches(r.current.Binding(), header, payload) {
			return nil, resource.ErrConflict
		}
		candidate, err := cloneResourceCandidate(*completed.Work.Candidate)
		if err != nil {
			return nil, err
		}
		receipt := completed.Receipt
		return &PreparedResourcePublication{room: r, candidate: candidate, receipt: &receipt}, nil
	}
	var retained resourceLocalPublication
	found, err := r.journal.Read(owner, namespace, workKey, &retained)
	if err != nil {
		return nil, err
	}
	if found {
		if !retained.Matches(r.current.Binding(), header, payload) {
			return nil, resource.ErrConflict
		}
	} else {
		retained = resourceLocalPublication{Version: 1, Binding: r.current.Binding(), Publication: header, Payload: bytes.Clone(payload)}
		if err := r.journal.Write(owner, namespace, workKey, retained); err != nil {
			return nil, err
		}
	}
	defer clear(retained.Payload)
	prepared, err := r.buildOrResumeResourceCandidate(ctx, owner, namespace, workKey, &retained, header, payload)
	if err != nil {
		return nil, err
	}
	candidate, err := cloneResourceCandidate(*retained.Candidate)
	if err != nil {
		prepared.Close()
		return nil, err
	}
	return &PreparedResourcePublication{room: r, prepared: prepared, candidate: candidate}, nil
}

func (r *ResourceRoom) buildOrResumeResourceCandidate(ctx context.Context, owner, namespace, workKey string, retained *resourceLocalPublication, header resource.Publication, payload []byte) (*PreparedEncryptedPublication, error) {
	if retained.Candidate != nil {
		if _, err := resource.VerifyProducerCandidate(ctx, *retained.Candidate, r.access, r.current.Device()); err != nil {
			return nil, err
		}
		certificate, err := sessionproof.DecodeCertificate(retained.Candidate.SessionCertificate)
		if err != nil {
			return nil, err
		}
		prepared := r.resumePrepared(ctx, retained.Candidate.EncryptedEvent, header.TransactionID, certificate.Session)
		if err := prepared.Revalidate(ctx); err != nil {
			prepared.Close()
			return nil, err
		}
		return prepared, nil
	}
	envelope, err := r.handle.SealFrame(ctx, header, payload, r.access, r.fence)
	if err != nil {
		return nil, err
	}
	frame, err := resource.EncodeFrame(envelope)
	clear(envelope.Ciphertext)
	if err != nil {
		return nil, err
	}
	defer clear(frame)
	prepared, err := r.owner.prepareEncryptedPublication(r.operationContext(ctx), r.RoomID(), event.Type{Type: resource.FrameEventType, Class: event.MessageEventType}, json.RawMessage(frame), header.TransactionID)
	if err != nil {
		return nil, err
	}
	candidate, err := r.signCandidate(ctx, prepared, header, frame)
	if err != nil {
		prepared.Close()
		return nil, err
	}
	retained.Candidate = &candidate
	if err := r.journal.Write(owner, namespace, workKey, *retained); err != nil {
		prepared.Close()
		return nil, err
	}
	return prepared, nil
}

func (r *ResourceRoom) resumePrepared(ctx context.Context, wire []byte, txn string, provenance sessionproof.Session) *PreparedEncryptedPublication {
	c := r.owner
	binding := PublicationBinding{Tenant: r.ActorScope().Tenant, Subject: r.ActorScope().Subject, MatrixUserID: r.current.Binding().UserID, DeviceID: r.current.Binding().DeviceID, RoomID: r.RoomID(), SessionID: provenance.SessionID, TransactionID: txn, NativeConsumer: c.cfg.NativeConsumer}
	return c.resumePreparedPublication(r.operationContext(ctx), binding, wire, provenance)
}

func (r *ResourceRoom) signCandidate(ctx context.Context, p *PreparedEncryptedPublication, header resource.Publication, frame []byte) (resource.Candidate, error) {
	var candidate resource.Candidate
	expected := sessionproof.CertificateExpectation{Scope: r.ActorScope(), Resource: r.Resource(), Epoch: header.Epoch, Generation: header.Generation, PolicySHA256: header.PolicySHA256, Session: p.Provenance(), DeviceID: header.Actor.DeviceID, RootKey: header.Actor.RootKey}
	prepared, err := p.resourceEnvelope(ctx, r.session, expected, frame)
	if err != nil {
		return candidate, err
	}
	candidate.SessionCertificate = prepared.certificate
	candidate.EncryptedEvent = prepared.wire
	candidate.Proof.CipherPublication = resource.CipherPublication{Publication: header, SessionCertificateID: prepared.certificateID, InnerCipherSHA256: prepared.innerSHA256, OuterCipherSHA256: prepared.outerSHA256}
	err = p.withPublicationSession(ctx, r.session, func(ctx context.Context) error {
		var err error
		candidate.Proof.Signatures, err = r.owner.signResourceJSON(ctx, candidate.Proof)
		return err
	})
	if err != nil {
		return resource.Candidate{}, err
	}
	if _, err := resource.VerifyProducerCandidate(ctx, candidate, r.access, r.current.Device()); err != nil {
		return resource.Candidate{}, err
	}
	return candidate, nil
}

// Read opens only the exact producer-authenticated candidate and receipt. The
// caller supplies an independently verified publication, never an HTTP body as
// authority. Historical windows keep their signed cutover through EpochHandle.
// The public metadata reader/source must verify the committed receipt chain.
func (r *ResourceRoom) Read(ctx context.Context, reference storage.Reference, verified resource.VerifiedPendingPublication, candidate resource.Candidate) ([]byte, error) {
	hash, err := resource.CandidateDigest(candidate)
	if err != nil || hash != verified.CandidateSHA256() {
		return nil, ErrIdentity
	}
	certificate, err := sessionproof.DecodeCertificate(candidate.SessionCertificate)
	if err != nil {
		return nil, err
	}
	return r.readResourceFrame(ctx, reference, verified.Value(), certificate.Session, pendingResourceFrame{proof: verified})
}

// RestoreBackup imports only this already authorized room. A current resource
// leaf/session does not itself grant historical epoch keys or other app rooms.
func (r *ResourceRoom) RestoreBackup(ctx context.Context) error {
	if err := r.Revalidate(ctx); err != nil {
		return err
	}
	bound := r.operationContext(ctx)
	c := r.owner
	release, err := c.lockBackup(bound)
	if err != nil {
		return err
	}
	err = func() error {
		defer release()
		c.stateMu.RLock()
		defer c.stateMu.RUnlock()
		if err := c.requireEncrypted(bound, r.RoomID()); err != nil {
			return err
		}
		return c.restoreBackup(bound)
	}()
	if err != nil {
		return err
	}
	return r.Revalidate(ctx)
}

type (
	resourceReadKey   struct{}
	resourceReadProof struct {
		room        *ResourceRoom
		reference   storage.Reference
		expected    sessionproof.Session
		outerSHA256 string
	}
)

// Only Read constructs this context after checking a typed verified publication
// and its exact candidate digest. A public backup cannot create that authority.
func (c *Client) validateResourceReadCipher(ctx context.Context, reference EventRef, sender id.UserID, outer string, wire *event.EncryptedEventContent) error {
	proof, ok := ctx.Value(resourceReadKey{}).(resourceReadProof)
	if !ok || proof.room == nil || proof.room.owner != c || !c.allowedRoomContext(ctx, reference.RoomID) || proof.reference.RoomID != reference.RoomID || proof.reference.EventID != reference.EventID || proof.outerSHA256 != outer || wire == nil || wire.SessionID.String() != proof.expected.SessionID || wire.SenderKey.String() != proof.expected.SenderKey || sender.String() != proof.expected.SenderUser {
		return ErrIdentity
	}
	return nil
}

func (c *Client) verifyResourceEventProvenance(ctx context.Context, clearEvent *event.Event) error {
	proof, ok := ctx.Value(resourceReadKey{}).(resourceReadProof)
	if !ok || proof.room == nil || proof.room.owner != c || clearEvent.RoomID.String() != proof.reference.RoomID || clearEvent.ID.String() != proof.reference.EventID || clearEvent.Sender.String() != proof.expected.SenderUser || clearEvent.Type.Type != resource.FrameEventType {
		return ErrIdentity
	}
	session, err := c.crypto.Machine().CryptoStore.GetGroupSession(ctx, clearEvent.RoomID, id.SessionID(proof.expected.SessionID))
	if err != nil {
		return err
	}
	if session == nil || session.RoomID != clearEvent.RoomID || session.ID().String() != proof.expected.SessionID || session.SenderKey.String() != proof.expected.SenderKey || session.SigningKey.String() != proof.expected.SigningKey {
		return ErrUnverified
	}
	return ctx.Err()
}
