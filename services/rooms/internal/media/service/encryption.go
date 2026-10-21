// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

type EncryptionGateway interface {
	PrepareCallEncryption(context.Context, uuid.UUID, string, string, uuid.UUID, domain.Role) (domain.CallEncryption, error)
	CallEncryption(context.Context, uuid.UUID, string, string, uuid.UUID) (domain.CallEncryption, error)
	ReadyCallEncryption(context.Context, uuid.UUID, string, string, uuid.UUID, int64) error
	CallPresence(context.Context, uuid.UUID, string, uuid.UUID, string, string, bool) (bool, error)
	RevokeCallMember(context.Context, uuid.UUID, string, bool) error
	CloseCallEncryption(context.Context, uuid.UUID) error
	ReadmitCallMember(context.Context, uuid.UUID, string) error
	SetCallPublication(context.Context, uuid.UUID, string, bool) error
	CallTransportName(context.Context, uuid.UUID) (string, error)
	FinishCallTransport(context.Context, uuid.UUID, string) (bool, error)
	CallRole(context.Context, uuid.UUID, string, string, uuid.UUID) (domain.Role, error)
}

func (s *Service) SetEncryptionGateway(g EncryptionGateway) { s.encryption = g }

type matrixDeviceKey struct{}

func WithMatrixDevice(ctx context.Context, device string) context.Context {
	return context.WithValue(ctx, matrixDeviceKey{}, device)
}

func matrixDevice(ctx context.Context) string {
	device, _ := ctx.Value(matrixDeviceKey{}).(string)
	return device
}

type callParticipantMetadata struct {
	Role           string `json:"role"`
	JoinID         string `json:"join_id"`
	MatrixDeviceID string `json:"matrix_device_id"`
}

func participantJoinID(metadata string) uuid.UUID {
	var m callParticipantMetadata
	if json.Unmarshal([]byte(metadata), &m) != nil {
		return uuid.Nil
	}
	id, _ := uuid.Parse(m.JoinID)
	return id
}

func (s *Service) RoomEncryption(ctx context.Context, id uuid.UUID, device, joinID string) (domain.CallEncryption, error) {
	if s.encryption == nil {
		return domain.CallEncryption{}, domain.ErrEncryptionUnavailable
	}
	claims, _ := authn.FromContext(ctx)
	room, err := s.store.GetRoom(ctx, id)
	if err != nil {
		return domain.CallEncryption{}, err
	}
	ctx, err = s.encryptedRoomAccess(ctx, room, claims.Subject, claims.HasAnyRole("admin", "owner"))
	if err != nil {
		return domain.CallEncryption{}, err
	}
	join, err := uuid.Parse(joinID)
	if err != nil || join == uuid.Nil {
		return domain.CallEncryption{}, domain.ErrPermission
	}
	return s.encryption.CallEncryption(ctx, id, claims.Subject, device, join)
}

func (s *Service) ReadyRoomEncryption(ctx context.Context, id uuid.UUID, device, joinID string, generation int64) error {
	if s.encryption == nil {
		return domain.ErrEncryptionUnavailable
	}
	claims, _ := authn.FromContext(ctx)
	room, err := s.store.GetRoom(ctx, id)
	if err != nil {
		return err
	}
	ctx, err = s.encryptedRoomAccess(ctx, room, claims.Subject, claims.HasAnyRole("admin", "owner"))
	if err != nil {
		return err
	}
	join, err := uuid.Parse(joinID)
	if err != nil || join == uuid.Nil {
		return domain.ErrPermission
	}
	return s.encryption.ReadyCallEncryption(ctx, id, claims.Subject, device, join, generation)
}

// PauseCall is called under the canonical call lock, before roster changes commit.
func (s *Service) PauseCall(ctx context.Context, name string, subjects, removed []string) error {
	for _, subject := range subjects {
		_, err := s.lk.GetParticipant(ctx, name, subject)
		if errors.Is(err, livekit.ErrParticipantNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if err := s.lk.SetMediaPermissions(ctx, name, subject, false, false); err != nil && !errors.Is(err, livekit.ErrParticipantNotFound) {
			return err
		}
	}
	for _, subject := range removed {
		if err := s.lk.RemoveParticipant(ctx, name, subject); err != nil && !errors.Is(err, livekit.ErrParticipantNotFound) {
			return err
		}
	}
	return nil
}

func (s *Service) EnableCall(ctx context.Context, name, actor, device, join string, publish bool) error {
	part, err := s.lk.GetParticipant(ctx, name, actor)
	if err != nil {
		return err
	}
	var metadata callParticipantMetadata
	if json.Unmarshal([]byte(part.Metadata), &metadata) != nil || metadata.JoinID != join || metadata.MatrixDeviceID != device || part.Identity != actor || part.SID == "" || part.State == "DISCONNECTED" {
		return domain.ErrPermission
	}
	// Both clients encrypt data packets through the maintained LiveKit SDK and
	// reject NONE/legacy packets. Keys are still exchanged only through Olm.
	return s.lk.SetMediaPermissions(ctx, name, actor, publish, true)
}

func (s *Service) encryptedPresence(ctx context.Context, ev livekit.WebhookEvent, joined bool) (bool, error) {
	if s.encryption == nil {
		return true, nil
	}
	room, err := s.store.GetRoomByName(ctx, ev.Room.Name)
	if err != nil {
		return false, err
	}
	join := participantJoinID(ev.Participant.Metadata)
	if join == uuid.Nil {
		return false, nil
	}
	return s.encryption.CallPresence(ctx, room.ID, ev.Participant.Identity, join, ev.Participant.SID, ev.Room.Name, joined)
}

// Every epoch has a new SFU room. Historical refresh tokens remain bound to a
// retired room and cannot subscribe to or publish in the current generation.
func (s *Service) RotateCallTransport(ctx context.Context, oldName, newName, tenant string, id uuid.UUID, generation int64, maxParticipants int) error {
	if oldName != "" {
		if err := s.lk.DeleteRoom(ctx, oldName); err != nil && !errors.Is(err, livekit.ErrParticipantNotFound) {
			return err
		}
	}
	if newName == "" {
		return nil
	}
	metadata, _ := json.Marshal(map[string]any{"sytue_tenant": tenant, "sytue_call_id": id.String(), "sytue_generation": generation})
	_, err := s.lk.CreateRoom(ctx, livekit.CreateRoomRequest{Name: newName, MaxParticipants: u32(maxParticipants), EmptyTimeout: 120, Metadata: string(metadata)})
	return err
}

func (s *Service) ReconnectRoomEncryption(ctx context.Context, id uuid.UUID, device, joinID string, generation int64) (JoinResult, error) {
	binding, err := s.RoomEncryption(ctx, id, device, joinID)
	if err != nil {
		return JoinResult{}, err
	}
	if binding.Generation != generation {
		return JoinResult{}, domain.ErrEncryptionGeneration
	}
	claims, _ := authn.FromContext(ctx)
	room, err := s.store.GetRoom(ctx, id)
	if err != nil {
		return JoinResult{}, err
	}
	join, _ := uuid.Parse(joinID)
	role, err := s.encryption.CallRole(ctx, id, claims.Subject, device, join)
	if err != nil {
		return JoinResult{}, err
	}
	no, yes := false, true
	metadata, _ := json.Marshal(callParticipantMetadata{Role: string(role), JoinID: joinID, MatrixDeviceID: device})
	token, err := s.signer.AccessToken(livekit.AccessTokenOptions{Identity: claims.Subject, Name: claims.Name, Room: binding.LiveKitRoom, Grant: livekit.VideoGrant{Room: binding.LiveKitRoom, RoomJoin: true, CanPublish: &no, CanSubscribe: &yes, CanPublishData: &no}, Metadata: string(metadata), TTL: 2 * time.Minute})
	if err != nil {
		return JoinResult{}, err
	}
	return JoinResult{Room: room, Token: token, LiveKitURL: s.cfg.LiveKitURL, Identity: claims.Subject, Role: role, JoinID: joinID, Encryption: &binding, ExpiresAtMS: time.Now().Add(2 * time.Minute).UnixMilli()}, nil
}

func (s *Service) prepareEncryptedJoin(ctx context.Context, room domain.Room, actor string, role domain.Role) (*domain.CallEncryption, string, time.Duration, error) {
	if s.encryption == nil {
		return nil, "", s.cfg.AccessTokenTTL, nil
	}
	var err error
	ctx, err = s.encryptedDocumentContext(ctx, room, actor)
	if err != nil {
		return nil, "", 0, err
	}
	join := uuid.New()
	binding, err := s.encryption.PrepareCallEncryption(ctx, room.ID, actor, matrixDevice(ctx), join, role)
	if err != nil {
		return nil, "", 0, err
	}
	return &binding, join.String(), 2 * time.Minute, nil
}
