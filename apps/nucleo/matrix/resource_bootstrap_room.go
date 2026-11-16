// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// ResourceBootstrapRoom prepares descriptors while final refs/epoch do not yet
// exist. It uses the same private SDK operation and public certificate/backup
// gates as ResourceRoom; it does not add raw rooms or expose direct Send.
type ResourceBootstrapRoom struct {
	owner   *Client
	vault   *Client
	current resource.VerifiedSession
	plan    resource.VerifiedBootstrapPlan
	session *PublicationSession
	journal *offline.Store
	key     atomic.Pointer[pqc.Identity]
	retired atomic.Bool
}

func (c *Client) BindResourceBootstrapRoom(ctx context.Context, current resource.VerifiedSession, plan resource.VerifiedBootstrapPlan, vault *Client, lease offline.WorkLease, journal *offline.Store) (*ResourceBootstrapRoom, error) {
	value := plan.Value()
	if c == nil || c.cfg.ResourceProduct == "" || !journal.Encrypted() || current.Binding() != plan.SessionBinding() || current.Scope() != value.CreatorScope || current.Actor() != value.Creator {
		return nil, ErrIdentity
	}
	if err := plan.RequireCurrentRosterCapture(ctx); err != nil {
		return nil, err
	}
	if err := c.BackupResourceBootstrapKey(ctx, current, plan, vault, lease, journal); err != nil {
		return nil, err
	}
	identity, err := c.bootstrapKey(ctx, current, value.Source.Resource, value.ID, lease, journal, false)
	if err != nil {
		return nil, err
	}
	if identity.Recipient().String() != value.Recipient {
		return nil, ErrRecoveryRequired
	}
	session, err := c.ResourcePublicationSession(ctx, current, lease)
	if err != nil {
		return nil, err
	}
	c.stateMu.Lock()
	policy, policyErr := c.currentOutboundRecipientPolicy()
	if policyErr == nil {
		policyErr = policy.ensureOutboundRecipientPolicy(ctx, map[id.RoomID]struct{}{id.RoomID(value.RoomID): {}})
	}
	c.stateMu.Unlock()
	if policyErr != nil {
		return nil, policyErr
	}
	room := &ResourceBootstrapRoom{owner: c, vault: vault, current: current, plan: plan, session: session, journal: journal}
	room.key.Store(identity)
	if err := room.Revalidate(ctx); err != nil {
		room.Close()
		return nil, err
	}
	return room, nil
}

func (r *ResourceBootstrapRoom) Close() {
	if r != nil {
		r.retired.Store(true)
		r.key.Store(nil)
	}
}
func (r *ResourceBootstrapRoom) operationOwner() *Client { return r.owner }
func (r *ResourceBootstrapRoom) operationBinding() resource.SessionBinding {
	return r.current.Binding()
}

func (r *ResourceBootstrapRoom) operationActive() bool {
	return r != nil && !r.retired.Load() && r.key.Load() != nil
}

func (r *ResourceBootstrapRoom) operationContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, resourceRoomKey{}, r)
}

func (r *ResourceBootstrapRoom) ActorScope() storage.Scope {
	if r == nil {
		return storage.Scope{}
	}
	return r.current.Scope()
}

func (r *ResourceBootstrapRoom) RoomID() string {
	if r == nil {
		return ""
	}
	return r.plan.Value().RoomID
}

func (r *ResourceBootstrapRoom) Revalidate(ctx context.Context) error {
	if !r.operationActive() || r.owner == nil || r.owner.closed.Load() || r.vault == nil || r.vault.closed.Load() {
		return ErrClosed
	}
	if err := r.plan.Revalidate(ctx); err != nil {
		return err
	}
	if err := r.current.Revalidate(ctx); err != nil {
		return err
	}
	if !r.operationActive() {
		return ErrClosed
	}
	return ctx.Err()
}

type bootstrapLocalPublication struct {
	Version       int
	Binding       resource.SessionBinding
	PlanSHA256    string
	Kind          resource.BootstrapKind
	TransactionID string
	Context       resource.BootstrapContext
	Payload       []byte
	Candidate     *resource.BootstrapCandidate
}
type PreparedBootstrapPublication struct {
	room      *ResourceBootstrapRoom
	prepared  *PreparedEncryptedPublication
	candidate resource.BootstrapCandidate
}

func (p *PreparedBootstrapPublication) Close() {
	if p != nil && p.prepared != nil {
		p.prepared.Close()
	}
}

func (p *PreparedBootstrapPublication) Candidate() (resource.BootstrapCandidate, error) {
	if p == nil || p.prepared == nil || p.prepared.retired.Load() {
		return resource.BootstrapCandidate{}, ErrClosed
	}
	return cloneResourceJSON(p.candidate)
}

func (p *PreparedBootstrapPublication) Revalidate(ctx context.Context) error {
	if p == nil || p.room == nil || p.prepared == nil {
		return ErrIdentity
	}
	if err := p.room.Revalidate(ctx); err != nil {
		return err
	}
	return p.prepared.Revalidate(ctx)
}

func (p *PreparedBootstrapPublication) SignAdmission(ctx context.Context) (publication.SignedRequest, error) {
	if err := p.Revalidate(ctx); err != nil {
		return publication.SignedRequest{}, err
	}
	return p.prepared.SignAdmission(ctx, p.room.session)
}

// PrepareDescriptor accepts encrypted-application descriptors, not large source
// state/history blobs. Those require the capability-scoped shared file gateway.
// Original payload/txn are journaled BEFORE age/SDK preparation; retries preserve
// exact bytes even across process/SDK restart or randomized hybrid encryption.
func (r *ResourceBootstrapRoom) PrepareDescriptor(ctx context.Context, kind resource.BootstrapKind, transaction string, payload []byte, baseline *resource.VerifiedBootstrapAcceptance) (*PreparedBootstrapPublication, error) {
	if err := r.Revalidate(ctx); err != nil {
		return nil, err
	}
	parsed, err := uuid.Parse(transaction)
	if err != nil || parsed == uuid.Nil || parsed.String() != transaction {
		return nil, ErrIdentity
	}
	bootCtx, err := r.resolveBootstrapContext(kind, baseline)
	if err != nil {
		return nil, err
	}
	namespace := storage.ResourceBootstrapPublicationNamespace
	owner, err := r.ActorScope().JournalOwner()
	if err != nil {
		return nil, err
	}
	recordID := r.plan.Value().ID + "/" + string(kind)
	var result *PreparedBootstrapPublication
	err = r.journal.Exclusive(ctx, owner, namespace, recordID, func() error {
		var err error
		result, err = r.prepareDescriptorLocked(ctx, owner, namespace, recordID, kind, transaction, bootCtx, payload, baseline)
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

func (r *ResourceBootstrapRoom) resolveBootstrapContext(kind resource.BootstrapKind, baseline *resource.VerifiedBootstrapAcceptance) (resource.BootstrapContext, error) {
	switch kind {
	case resource.BootstrapBaseline:
		if baseline != nil {
			return resource.BootstrapContext{}, ErrIdentity
		}
		return r.plan.BaselineContext()
	case resource.BootstrapCheckpoint:
		if baseline == nil {
			return resource.BootstrapContext{}, ErrIdentity
		}
		return r.plan.CheckpointContext(*baseline)
	default:
		return resource.BootstrapContext{}, ErrIdentity
	}
}

func (r *ResourceBootstrapRoom) prepareDescriptorLocked(ctx context.Context, owner, namespace, recordID string, kind resource.BootstrapKind, transaction string, bootCtx resource.BootstrapContext, payload []byte, baseline *resource.VerifiedBootstrapAcceptance) (*PreparedBootstrapPublication, error) {
	var retained bootstrapLocalPublication
	found, err := r.journal.Read(owner, namespace, recordID, &retained)
	if err != nil {
		return nil, err
	}
	if found {
		if retained.Version != 1 || retained.Binding != r.current.Binding() || retained.PlanSHA256 != r.plan.SHA256() || retained.Kind != kind || retained.TransactionID != transaction || retained.Context != bootCtx || !bytes.Equal(retained.Payload, payload) {
			return nil, resource.ErrConflict
		}
	} else {
		retained = bootstrapLocalPublication{Version: 1, Binding: r.current.Binding(), PlanSHA256: r.plan.SHA256(), Kind: kind, TransactionID: transaction, Context: bootCtx, Payload: bytes.Clone(payload)}
		if err := r.journal.Write(owner, namespace, recordID, retained); err != nil {
			return nil, err
		}
	}
	defer clear(retained.Payload)
	prepared, err := r.buildOrResumeBootstrapCandidate(ctx, owner, namespace, recordID, &retained, kind, transaction, bootCtx, payload, baseline)
	if err != nil {
		return nil, err
	}
	candidate, err := cloneResourceJSON(*retained.Candidate)
	if err != nil {
		prepared.Close()
		return nil, err
	}
	return &PreparedBootstrapPublication{room: r, prepared: prepared, candidate: candidate}, nil
}

func (r *ResourceBootstrapRoom) buildOrResumeBootstrapCandidate(ctx context.Context, owner, namespace, recordID string, retained *bootstrapLocalPublication, kind resource.BootstrapKind, transaction string, bootCtx resource.BootstrapContext, payload []byte, baseline *resource.VerifiedBootstrapAcceptance) (*PreparedEncryptedPublication, error) {
	if retained.Candidate != nil {
		if _, err := resource.VerifyBootstrapCandidate(ctx, *retained.Candidate, r.plan, baseline); err != nil {
			return nil, err
		}
		certificate, err := sessionproof.DecodeCertificate(retained.Candidate.SessionCertificate)
		if err != nil {
			return nil, err
		}
		binding := PublicationBinding{Tenant: r.ActorScope().Tenant, Subject: r.ActorScope().Subject, MatrixUserID: r.current.Binding().UserID, DeviceID: r.current.Binding().DeviceID, RoomID: r.RoomID(), SessionID: certificate.Session.SessionID, TransactionID: transaction, NativeConsumer: r.owner.cfg.NativeConsumer}
		prepared := r.owner.resumePreparedPublication(r.operationContext(ctx), binding, retained.Candidate.EncryptedEvent, certificate.Session)
		if err := prepared.Revalidate(ctx); err != nil {
			prepared.Close()
			return nil, err
		}
		return prepared, nil
	}
	identity := r.key.Load()
	if identity == nil {
		return nil, ErrClosed
	}
	envelope, err := resource.SealBootstrapDescriptor(identity.Recipient(), bootCtx, payload)
	if err != nil {
		return nil, err
	}
	frame, err := resource.EncodeFrame(envelope)
	clear(envelope.Ciphertext)
	if err != nil {
		return nil, err
	}
	defer clear(frame)
	prepared, err := r.owner.prepareEncryptedPublication(r.operationContext(ctx), r.RoomID(), event.Type{Type: resource.FrameEventType, Class: event.MessageEventType}, json.RawMessage(frame), transaction)
	if err != nil {
		return nil, err
	}
	candidate, err := r.signBootstrapCandidate(ctx, prepared, kind, transaction, bootCtx, frame)
	if err != nil {
		prepared.Close()
		return nil, err
	}
	if _, err := resource.VerifyBootstrapCandidate(ctx, candidate, r.plan, baseline); err != nil {
		prepared.Close()
		return nil, err
	}
	retained.Candidate = &candidate
	if err := r.journal.Write(owner, namespace, recordID, *retained); err != nil {
		prepared.Close()
		return nil, err
	}
	return prepared, nil
}

func (r *ResourceBootstrapRoom) signBootstrapCandidate(ctx context.Context, p *PreparedEncryptedPublication, kind resource.BootstrapKind, transaction string, contextValue resource.BootstrapContext, frame []byte) (resource.BootstrapCandidate, error) {
	var candidate resource.BootstrapCandidate
	contextHash, err := resource.BootstrapContextSHA256(contextValue)
	if err != nil {
		return candidate, err
	}
	value := r.plan.Value()
	actor := r.current.Actor()
	expected := sessionproof.CertificateExpectation{Scope: r.ActorScope(), Resource: value.Source.Resource, Epoch: value.Epoch, Generation: "1", PolicySHA256: contextHash, Session: p.Provenance(), DeviceID: actor.DeviceID, RootKey: actor.RootKey}
	prepared, err := p.resourceEnvelope(ctx, r.session, expected, frame)
	if err != nil {
		return candidate, err
	}
	candidate.SessionCertificate = prepared.certificate
	candidate.EncryptedEvent = prepared.wire
	candidate.Proof.BootstrapPublication = resource.BootstrapPublication{Version: 1, Domain: "org.sytue.resource.bootstrap.publication.v1", PlanSHA256: r.plan.SHA256(), Kind: kind, TransactionID: transaction, ContextSHA256: contextHash, InnerCipherSHA256: prepared.innerSHA256, OuterCipherSHA256: prepared.outerSHA256, SessionCertificateID: prepared.certificateID, Actor: actor}
	err = p.withPublicationSession(ctx, r.session, func(ctx context.Context) error {
		var err error
		candidate.Proof.Signatures, err = r.owner.signResourceJSON(ctx, candidate.Proof)
		return err
	})
	if err != nil {
		return resource.BootstrapCandidate{}, err
	}
	return candidate, nil
}
