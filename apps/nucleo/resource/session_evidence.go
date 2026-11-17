// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// SessionEvidence contains only public authority/SDK metadata. It cannot be
// decoded into a Verified value. Pins come from the authenticated fixed backend
// registry, independently of the untrusted resource roster being verified.
type SessionEvidence struct {
	Record SessionRecord      `json:"record"`
	Origin matrixproof.Origin `json:"origin"`
}

// SessionEvidenceSource must perform a fresh authenticated lookup at a FIXED
// application backend for every invocation. Implementations may not accept an
// origin URL, root/pin, registry record or lifecycle assertions from their caller.
type SessionEvidenceSource interface {
	CurrentResourceSessionEvidence(context.Context, storage.Scope, string) (SessionEvidence, error)
}

type sessionEvidenceAuthority struct {
	source SessionEvidenceSource
	server string
}

// NewSessionEvidenceProvider uses the SAME enrollment and maintained SDK
// verifiers as own-session. The configured server and canonical human prefix
// are independent of response JSON and roster values; no historical fallback.
func NewSessionEvidenceProvider(source SessionEvidenceSource, server string) (*SessionProvider, error) {
	user := id.NewUserID("sytue_probe", server)
	local, parsed, err := user.Parse()
	if source == nil || err != nil || local != "sytue_probe" || parsed != server || server == "" || strings.ContainsAny(server, "/?#@ \t\r\n") {
		return nil, ErrInvalid
	}
	authority := &sessionEvidenceAuthority{source: source, server: server}
	return NewCurrentSessionProvider(authority)
}

func (s *sessionEvidenceAuthority) evidence(ctx context.Context, scope storage.Scope, device string) (SessionEvidence, error) {
	var zero SessionEvidence
	if s == nil || s.source == nil || !scope.Valid() || !ValidDeviceID(device) {
		return zero, ErrInvalid
	}
	value, err := s.source.CurrentResourceSessionEvidence(ctx, scope, device)
	if err != nil {
		return zero, err
	}
	b := value.Record.Binding
	expected := id.NewUserID("sytue_"+scope.Subject, s.server)
	if !b.Valid() || b.Scope != scope || b.DeviceID != device || b.UserID != expected.String() || value.Record.PinnedRoot != b.RootKey || value.Record.RootGeneration != b.RootGeneration || value.Origin.Scope != scope || value.Origin.ServerName != s.server || value.Origin.UserID != expected || value.Origin.DeviceID.String() != device {
		return zero, ErrRevoked
	}
	return value, ctx.Err()
}

func (s *sessionEvidenceAuthority) CurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (SessionRecord, error) {
	v, err := s.evidence(ctx, scope, device)
	return v.Record, err
}

func (s *sessionEvidenceAuthority) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, root string) (matrixproof.Device, error) {
	v, err := s.evidence(ctx, scope, device)
	if err != nil {
		return matrixproof.Device{}, err
	}
	if root == "" || root != v.Record.PinnedRoot {
		return matrixproof.Device{}, ErrRevoked
	}
	return matrixproof.VerifyUser(ctx, v.Origin, id.NewUserID("sytue_"+scope.Subject, s.server), id.Ed25519(root))
}

func (s *sessionEvidenceAuthority) CheckResourceSession(ctx context.Context, binding SessionBinding) error {
	v, err := s.evidence(ctx, binding.Scope, binding.DeviceID)
	if err != nil {
		return err
	}
	if v.Record.Binding != binding {
		return ErrRevoked
	}
	_, err = matrixproof.VerifyUser(ctx, v.Origin, id.NewUserID("sytue_"+binding.Scope.Subject, s.server), id.Ed25519(v.Record.PinnedRoot))
	return err
}

// CaptureCurrentResourceSession consumes one fresh public response only inside
// Resolve. Canonical server/user and registry pin remain independent of roster
// JSON, and the maintained SDK checks the exact captured origin/root pair.
func (s *sessionEvidenceAuthority) CaptureCurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (SessionRecord, matrixproof.Device, error) {
	value, err := s.evidence(ctx, scope, device)
	if err != nil {
		return SessionRecord{}, matrixproof.Device{}, err
	}
	verified, err := matrixproof.VerifyUser(ctx, value.Origin, id.NewUserID("sytue_"+scope.Subject, s.server), id.Ed25519(value.Record.PinnedRoot))
	if err != nil {
		return SessionRecord{}, matrixproof.Device{}, err
	}
	return value.Record, verified, nil
}
