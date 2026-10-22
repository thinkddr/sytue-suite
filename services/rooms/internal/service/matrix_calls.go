// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	mediadomain "git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
)

func (s *Service) SetCallMediaTransport(g store.CallMediaTransport) {
	if s.pstore != nil {
		s.pstore.SetCallMediaTransport(g)
	}
}

func callGateError(err error) error {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		return mediadomain.ErrPermission
	case errors.Is(err, domain.ErrNotFound):
		return mediadomain.ErrNotFound
	case errors.Is(err, domain.ErrMatrixUnavailable):
		return mediadomain.ErrEncryptionUnavailable
	default:
		return err
	}
}

func (s *Service) callActor(ctx context.Context, actor, device string) error {
	if s.matrix == nil || s.pstore == nil {
		return mediadomain.ErrEncryptionUnavailable
	}
	if _, _, err := s.matrixIdentity(ctx, actor); err != nil {
		return callGateError(err)
	}
	if err := s.pstore.MatrixActorAllowed(ctx, actor); err != nil {
		return callGateError(err)
	}
	return callGateError(s.matrix.ValidateDevice(ctx, actor, device))
}

func (s *Service) PrepareCallEncryption(ctx context.Context, id uuid.UUID, actor, device string, join uuid.UUID, role mediadomain.Role) (mediadomain.CallEncryption, error) {
	if device == "" {
		return mediadomain.CallEncryption{}, mediadomain.ErrEncryptionRequired
	}
	if err := s.callActor(ctx, actor, device); err != nil {
		return mediadomain.CallEncryption{}, err
	}
	out, err := s.pstore.PrepareCallEncryption(ctx, id, actor, device, join, role)
	return out, callGateError(err)
}

func (s *Service) CallEncryption(ctx context.Context, id uuid.UUID, actor, device string, join uuid.UUID) (mediadomain.CallEncryption, error) {
	if err := s.callActor(ctx, actor, device); err != nil {
		return mediadomain.CallEncryption{}, err
	}
	out, err := s.pstore.CallEncryption(ctx, id, actor, device, join)
	return out, callGateError(err)
}

func (s *Service) ReadyCallEncryption(ctx context.Context, id uuid.UUID, actor, device string, join uuid.UUID, generation int64) error {
	if err := s.callActor(ctx, actor, device); err != nil {
		return err
	}
	return callGateError(s.pstore.ReadyCallEncryption(ctx, id, actor, device, join, generation))
}

func (s *Service) CallPresence(ctx context.Context, id uuid.UUID, actor string, join uuid.UUID, sid, transport string, joined bool) (bool, error) {
	return s.pstore.CallPresence(ctx, id, actor, join, sid, transport, joined)
}

func (s *Service) RevokeCallMember(ctx context.Context, id uuid.UUID, actor string, blocked bool) error {
	return callGateError(s.pstore.RevokeCallMember(ctx, id, actor, blocked))
}

func (s *Service) CloseCallEncryption(ctx context.Context, id uuid.UUID) error {
	return callGateError(s.pstore.CloseCallEncryption(ctx, id))
}

func (s *Service) ReadmitCallMember(ctx context.Context, id uuid.UUID, actor string) error {
	return callGateError(s.pstore.ReadmitCallMember(ctx, id, actor))
}

func (s *Service) SetCallPublication(ctx context.Context, id uuid.UUID, actor string, allowed bool) error {
	return callGateError(s.pstore.SetCallPublication(ctx, id, actor, allowed))
}

// Expiring calls are also rotated when no client polls. The server stores and
// schedules ACL metadata only; clients discard media keys at expires_at_ms.
func (s *Service) RunCallEncryptionExpiry(ctx context.Context) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if s.pstore != nil {
				if err := s.pstore.ExpireCallEncryptions(ctx); err != nil {
					log.From(ctx).Warn("encrypted call epoch refresh failed", "error", err)
				}
			}
		}
	}
}

func (s *Service) CallTransportName(ctx context.Context, id uuid.UUID) (string, error) {
	return s.pstore.CallTransportName(ctx, id)
}

func (s *Service) FinishCallTransport(ctx context.Context, id uuid.UUID, name string) (bool, error) {
	return s.pstore.FinishCallTransport(ctx, id, name)
}

func (s *Service) CallRole(ctx context.Context, id uuid.UUID, actor, device string, join uuid.UUID) (mediadomain.Role, error) {
	return s.pstore.CallRole(ctx, id, actor, device, join)
}
