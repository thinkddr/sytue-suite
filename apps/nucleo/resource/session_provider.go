// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// SessionRecord is public enrollment evidence read from the product registry.
// Implementations must resolve the independent root pin and consumed consent
// from their own durable rows, with current lifecycle/generation checks. This
// record must never be populated from an API caller's JSON.
type SessionRecord struct {
	Binding        SessionBinding          `json:"binding"`
	PinnedRoot     string                  `json:"pinned_root"`
	RootGeneration Decimal                 `json:"root_generation"`
	Recipient      SignedRecipient         `json:"recipient"`
	Challenge      SessionChallenge        `json:"challenge"`
	Proof          SignedSessionEnrollment `json:"proof"`
	ProofSHA256    string                  `json:"proof_sha256"`
	ConfirmedAt    time.Time               `json:"confirmed_at"`
}
type SessionRegistry interface {
	CurrentResourceSession(context.Context, storage.Scope, string) (SessionRecord, error)
	CheckResourceSession(context.Context, SessionBinding) error
}

// SessionDirectory resolves the canonical human user at a FIXED configured
// Matrix origin and verifies current account/device state plus SDK signatures.
// A request-provided homeserver, display name or public key is not a directory.
type SessionDirectory interface {
	ResourceSessionDevice(context.Context, storage.Scope, string, string) (matrixproof.Device, error)
}
type SessionProvider struct {
	registry  SessionRegistry
	directory SessionDirectory
	capture   SessionCaptureSource
}

func NewSessionProvider(registry SessionRegistry, directory SessionDirectory) (*SessionProvider, error) {
	if registry == nil || directory == nil {
		return nil, ErrInvalid
	}
	return &SessionProvider{registry: registry, directory: directory}, nil
}

// VerifiedSession has no JSON constructor and no private key getter. Only the
// provider's current registry + independently verified SDK chain creates one.
// Historical signed RoleGrant A remains distinct from this current Actor B.
type VerifiedSession struct {
	provider   *SessionProvider
	binding    SessionBinding
	enrollment VerifiedSessionEnrollment
	record     []byte
}

func (s VerifiedSession) PublicRecordBytes() []byte {
	if !s.valid() {
		return nil
	}
	return bytes.Clone(s.record)
}
func (s VerifiedSession) Binding() SessionBinding      { return s.binding }
func (s VerifiedSession) Actor() Actor                 { return s.binding.Actor() }
func (s VerifiedSession) Scope() storage.Scope         { return s.binding.Scope }
func (s VerifiedSession) Device() matrixproof.Device   { return s.enrollment.device }
func (s VerifiedSession) Recipient() VerifiedRecipient { return s.enrollment.recipient }
func (s VerifiedSession) valid() bool {
	return s.provider != nil && s.binding.Valid() && digest(s.enrollment.hash) && s.enrollment.binding == s.binding
}

// SessionCaptureSource is an optional atomic CURRENT authority read. Its
// implementation must verify the fresh response/nonce and independent SDK root
// before returning the same record/device pair. It is a trusted composition
// port, never a constructor from caller JSON or a cache of permission decisions.
// Only Resolve consumes this pair; future Revalidate always uses live ports.
type SessionCaptureSource interface {
	CaptureCurrentResourceSession(context.Context, storage.Scope, string) (SessionRecord, matrixproof.Device, error)
}

// NewCurrentSessionProvider explicitly opts ONE fixed authority into an atomic
// current read. NewSessionProvider keeps separate registry/directory semantics;
// a registry cannot silently bypass its independently configured directory.
func NewCurrentSessionProvider(authority interface {
	SessionRegistry
	SessionDirectory
	SessionCaptureSource
},
) (*SessionProvider, error) {
	if authority == nil {
		return nil, ErrInvalid
	}
	return &SessionProvider{registry: authority, directory: authority, capture: authority}, nil
}

func validSessionRecord(scope storage.Scope, device string, record SessionRecord) bool {
	b := record.Binding
	return b.Valid() && b.Scope == scope && b.DeviceID == device && b.RootKey == record.PinnedRoot && b.RootGeneration == record.RootGeneration && !record.ConfirmedAt.IsZero() && !record.ConfirmedAt.After(time.Unix(record.Challenge.ExpiresAt, 0))
}

// resolveCaptured is private: the SAME enrollment verifier consumes a single
// atomic public snapshot, and the returned session retains this LIVE provider.
// No captured record, request witness or nonce becomes a future revalidation.
func (p *SessionProvider) resolveCaptured(ctx context.Context, scope storage.Scope, device string, record SessionRecord, current matrixproof.Device) (VerifiedSession, error) {
	if !validSessionRecord(scope, device, record) || !identityMatches(current.Identity(), record.Binding.Actor(), scope) {
		return VerifiedSession{}, ErrInvalid
	}
	enrollment, err := VerifySessionEnrollment(ctx, record.Proof, record.Recipient, record.Challenge, current, record.ConfirmedAt)
	if err != nil || enrollment.binding != record.Binding || enrollment.hash != record.ProofSHA256 {
		return VerifiedSession{}, ErrInvalid
	}
	public, err := json.Marshal(record)
	if err != nil {
		return VerifiedSession{}, err
	}
	if err := ctx.Err(); err != nil {
		return VerifiedSession{}, err
	}
	return VerifiedSession{provider: p, binding: record.Binding, enrollment: enrollment, record: public}, nil
}

func (p *SessionProvider) Resolve(ctx context.Context, scope storage.Scope, device string) (VerifiedSession, error) {
	if p == nil || !scope.Valid() || device == "" {
		return VerifiedSession{}, ErrInvalid
	}
	if p.capture != nil {
		record, current, err := p.capture.CaptureCurrentResourceSession(ctx, scope, device)
		if err != nil {
			return VerifiedSession{}, err
		}
		return p.resolveCaptured(ctx, scope, device, record, current)
	}
	record, err := p.registry.CurrentResourceSession(ctx, scope, device)
	if err != nil {
		return VerifiedSession{}, err
	}
	if !validSessionRecord(scope, device, record) {
		return VerifiedSession{}, ErrInvalid
	}
	current, err := p.directory.ResourceSessionDevice(ctx, scope, device, record.PinnedRoot)
	if err != nil {
		return VerifiedSession{}, err
	}
	verified, err := p.resolveCaptured(ctx, scope, device, record, current)
	if err != nil {
		return VerifiedSession{}, err
	}
	if err := p.registry.CheckResourceSession(ctx, record.Binding); err != nil {
		return VerifiedSession{}, err
	}
	return verified, nil
}

func (s VerifiedSession) Revalidate(ctx context.Context) error {
	if !s.valid() {
		return ErrRevoked
	}
	if err := s.provider.registry.CheckResourceSession(ctx, s.binding); err != nil {
		return err
	}
	current, err := s.provider.directory.ResourceSessionDevice(ctx, s.binding.Scope, s.binding.DeviceID, s.binding.RootKey)
	if err != nil {
		return err
	}
	if current.Identity() != s.enrollment.device.Identity() {
		return ErrRevoked
	}
	if err := s.provider.registry.CheckResourceSession(ctx, s.binding); err != nil {
		return err
	}
	return ctx.Err()
}
