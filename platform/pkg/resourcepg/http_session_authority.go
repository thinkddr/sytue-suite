// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"net/http"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// SessionAuthorityClient reads only the fixed Rooms authority's ORIGINAL public
// enrollment and SDK directory. The existing SessionProvider verifies the
// consumed consent, recipient possession and every SDK chain/signature; this
// transport never constructs a VerifiedSession from response JSON.
type SessionAuthorityClient struct{ endpoint *OriginSourceEndpoint }

var (
	_ resource.SessionRegistry  = (*SessionAuthorityClient)(nil)
	_ resource.SessionDirectory = (*SessionAuthorityClient)(nil)
)

func NewSessionAuthorityClient(serviceID, base, secret string, transport http.RoundTripper) (*SessionAuthorityClient, error) {
	if !ValidSessionServiceSecret(secret) {
		return nil, resource.ErrInvalid
	}
	endpoint, err := NewOriginSourceEndpoint(serviceID, base, secret, transport)
	if err != nil {
		return nil, err
	}
	return &SessionAuthorityClient{endpoint: endpoint}, nil
}

func (c *SessionAuthorityClient) Provider() (*resource.SessionProvider, error) {
	if c == nil || c.endpoint == nil {
		return nil, resource.ErrInvalid
	}
	return resource.NewCurrentSessionProvider(c)
}

func (c *SessionAuthorityClient) current(ctx context.Context, scope storage.Scope, device string) (SessionOriginResponse, error) {
	var out SessionOriginResponse
	if c == nil || c.endpoint == nil {
		return out, resource.ErrInvalid
	}
	q, err := NewSessionOriginRequest(scope, device)
	if err != nil || !q.ValidForService(c.endpoint.serviceID) {
		return out, resource.ErrInvalid
	}
	if err := c.endpoint.post(ctx, "/internal/resources/sessions/"+c.endpoint.serviceID+"/current", q, &out, 256<<10); err != nil {
		return SessionOriginResponse{}, err
	}
	if VerifySessionOriginResponse(c.endpoint.secret, q, out) != nil {
		return SessionOriginResponse{}, resource.ErrRevoked
	}
	return out, nil
}

func (c *SessionAuthorityClient) CurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, error) {
	value, err := c.current(ctx, scope, device)
	if err != nil {
		return resource.SessionRecord{}, err
	}
	return value.Record, nil
}

func (c *SessionAuthorityClient) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, root string) (matrixproof.Device, error) {
	value, err := c.current(ctx, scope, device)
	if err != nil {
		return matrixproof.Device{}, err
	}
	if root == "" || value.Record.PinnedRoot != root {
		return matrixproof.Device{}, resource.ErrRevoked
	}
	// expectedUser and root are independently retained by the fixed authority,
	// never fields from the SDK-signed source query or its request-provided actor.
	return matrixproof.VerifyUser(ctx, value.Origin, id.UserID(value.Record.Binding.UserID), id.Ed25519(root))
}

func (c *SessionAuthorityClient) CheckResourceSession(ctx context.Context, binding resource.SessionBinding) error {
	if !binding.Valid() {
		return resource.ErrInvalid
	}
	value, err := c.current(ctx, binding.Scope, binding.DeviceID)
	if err != nil {
		return err
	}
	if value.Record.Binding != binding {
		return resource.ErrRevoked
	}
	_, err = matrixproof.VerifyUser(ctx, value.Origin, id.UserID(binding.UserID), id.Ed25519(binding.RootKey))
	return err
}

// CaptureCurrentResourceSession verifies the fresh complete nonce/MAC BEFORE
// returning the exact record and maintained-SDK device to the shared provider.
func (c *SessionAuthorityClient) captureCurrent(ctx context.Context, scope storage.Scope, device string) (SessionOriginResponse, matrixproof.Device, error) {
	value, err := c.current(ctx, scope, device)
	if err != nil {
		return SessionOriginResponse{}, matrixproof.Device{}, err
	}
	verified, err := matrixproof.VerifyUser(ctx, value.Origin, id.UserID(value.Record.Binding.UserID), id.Ed25519(value.Record.PinnedRoot))
	if err != nil {
		return SessionOriginResponse{}, matrixproof.Device{}, err
	}
	return value, verified, nil
}

func (c *SessionAuthorityClient) CaptureCurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, matrixproof.Device, error) {
	value, verified, err := c.captureCurrent(ctx, scope, device)
	return value.Record, verified, err
}

// A witness belongs to exactly ONE Resolve call. take clears all captured public
// input before returning; the retained provider delegates future checks LIVE.
type currentEvidenceRequest struct {
	authority *SessionAuthorityClient
	scope     storage.Scope
	device    string
	used      bool
	value     *resource.SessionEvidence
}

func (s *currentEvidenceRequest) CaptureCurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, matrixproof.Device, error) {
	if s.used || scope != s.scope || device != s.device {
		return resource.SessionRecord{}, matrixproof.Device{}, resource.ErrRevoked
	}
	s.used = true
	value, verified, err := s.authority.captureCurrent(ctx, scope, device)
	if err != nil {
		return resource.SessionRecord{}, matrixproof.Device{}, err
	}
	s.value = &resource.SessionEvidence{Record: value.Record, Origin: value.Origin}
	return value.Record, verified, nil
}

func (s *currentEvidenceRequest) CurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, error) {
	if scope != s.scope || device != s.device {
		return resource.SessionRecord{}, resource.ErrRevoked
	}
	return s.authority.CurrentResourceSession(ctx, scope, device)
}

func (s *currentEvidenceRequest) ResourceSessionDevice(ctx context.Context, scope storage.Scope, device, root string) (matrixproof.Device, error) {
	if scope != s.scope || device != s.device {
		return matrixproof.Device{}, resource.ErrRevoked
	}
	return s.authority.ResourceSessionDevice(ctx, scope, device, root)
}

func (s *currentEvidenceRequest) CheckResourceSession(ctx context.Context, b resource.SessionBinding) error {
	if b.Scope != s.scope || b.DeviceID != s.device {
		return resource.ErrRevoked
	}
	return s.authority.CheckResourceSession(ctx, b)
}

func (s *currentEvidenceRequest) take() (resource.SessionEvidence, error) {
	value := s.value
	s.value = nil
	if value == nil || !s.used {
		return resource.SessionEvidence{}, resource.ErrRevoked
	}
	return *value, nil
}

func (c *SessionAuthorityClient) resolveCurrentEvidence(ctx context.Context, scope storage.Scope, device string) (resource.VerifiedSession, resource.SessionEvidence, error) {
	var zero resource.SessionEvidence
	request := &currentEvidenceRequest{authority: c, scope: scope, device: device}
	provider, err := resource.NewCurrentSessionProvider(request)
	if err != nil {
		return resource.VerifiedSession{}, zero, err
	}
	current, err := provider.Resolve(ctx, scope, device)
	value, takeErr := request.take()
	if err != nil {
		return resource.VerifiedSession{}, zero, err
	}
	if takeErr != nil || value.Record.Binding != current.Binding() {
		return resource.VerifiedSession{}, zero, resource.ErrRevoked
	}
	return current, value, nil
}

// CurrentResourceSessionEvidence returns only the exact public input consumed by
// a one-use current Resolve. No witness/nonce or permission cache survives it.
func (c *SessionAuthorityClient) CurrentResourceSessionEvidence(ctx context.Context, scope storage.Scope, device string) (resource.SessionEvidence, error) {
	_, evidence, err := c.resolveCurrentEvidence(ctx, scope, device)
	return evidence, err
}
