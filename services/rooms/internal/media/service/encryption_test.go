// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

type encryptionFake struct {
	binding domain.CallEncryption
	device  string
	join    uuid.UUID
}

func (f *encryptionFake) PrepareCallEncryption(_ context.Context, _ uuid.UUID, _, device string, join uuid.UUID, _ domain.Role) (domain.CallEncryption, error) {
	f.device = device
	f.join = join
	return f.binding, nil
}

func (f *encryptionFake) CallEncryption(context.Context, uuid.UUID, string, string, uuid.UUID) (domain.CallEncryption, error) {
	return f.binding, nil
}

func (*encryptionFake) ReadyCallEncryption(context.Context, uuid.UUID, string, string, uuid.UUID, int64) error {
	return nil
}

func (*encryptionFake) CallPresence(context.Context, uuid.UUID, string, uuid.UUID, string, string, bool) (bool, error) {
	return true, nil
}

func (*encryptionFake) RevokeCallMember(context.Context, uuid.UUID, string, bool) error { return nil }

func (*encryptionFake) CloseCallEncryption(context.Context, uuid.UUID) error { return nil }

func (*encryptionFake) ReadmitCallMember(context.Context, uuid.UUID, string) error { return nil }

func (*encryptionFake) SetCallPublication(context.Context, uuid.UUID, string, bool) error { return nil }

func (f *encryptionFake) CallTransportName(context.Context, uuid.UUID) (string, error) {
	return f.binding.LiveKitRoom, nil
}

func (*encryptionFake) FinishCallTransport(context.Context, uuid.UUID, string) (bool, error) {
	return true, nil
}

func (*encryptionFake) CallRole(context.Context, uuid.UUID, string, string, uuid.UUID) (domain.Role, error) {
	return domain.RoleViewer, nil
}

func TestEncryptedJoinCannotPublishAdministerOrUseDataBeforeReady(t *testing.T) {
	room := domain.Room{ID: uuid.New(), Name: "canonical-call", CreatedBy: "alice", Status: domain.RoomActive}
	s := newSvc(&fakeStore{room: room}, &fakeLK{}, nil)
	gate := &encryptionFake{binding: domain.CallEncryption{CallID: room.ID, Generation: 9, LiveKitRoom: "epoch-nine", MatrixRoomID: "!nine:matrix.sytue.eu"}}
	s.SetEncryptionGateway(gate)
	out, err := s.JoinRoom(WithMatrixDevice(tenantCtx(t), "MY-DEVICE"), room.ID, "alice", "Alice", domain.RoleSpeaker, false)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.Split(out.Token, ".")[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Video struct {
			Room    string `json:"room"`
			Admin   bool   `json:"roomAdmin"`      //nolint:tagliatelle // LiveKit JWT claim format
			Publish bool   `json:"canPublish"`     //nolint:tagliatelle // LiveKit JWT claim format
			Data    bool   `json:"canPublishData"` //nolint:tagliatelle // LiveKit JWT claim format
		}
		Metadata string `json:"metadata"`
		Exp      int64  `json:"exp"`
		Nbf      int64  `json:"nbf"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Video.Room != "epoch-nine" || claims.Video.Admin || claims.Video.Publish || claims.Video.Data || claims.Exp-claims.Nbf > 151 {
		t.Fatalf("JWT bypassed encrypted admission: %s", payload)
	}
	if gate.device != "MY-DEVICE" || gate.join == uuid.Nil || out.JoinID != gate.join.String() {
		t.Fatalf("join not bound to device: %+v", out)
	}
	var metadata callParticipantMetadata
	if err := json.Unmarshal([]byte(claims.Metadata), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.JoinID != out.JoinID || metadata.MatrixDeviceID != "MY-DEVICE" {
		t.Fatalf("SFU metadata missing binding: %+v", metadata)
	}
	if _, err := s.StartRecording(tenantCtx(t), room.ID); !errors.Is(err, domain.ErrEncryptedOperation) {
		t.Fatalf("server recording enabled: %v", err)
	}
}

func TestReadyChecksCurrentSFUParticipantNonceAndDevice(t *testing.T) {
	join := uuid.NewString()
	metadata, _ := json.Marshal(callParticipantMetadata{JoinID: join, MatrixDeviceID: "DEVICE"})
	lk := &fakeLK{participante: livekit.ParticipantInfo{SID: "CURRENT", Identity: "alice", Metadata: string(metadata), State: "ACTIVE"}}
	s := newSvc(&fakeStore{}, lk, nil)
	if err := s.EnableCall(context.Background(), "epoch", "alice", "DEVICE", join, true); err != nil {
		t.Fatal(err)
	}
	if !lk.palabraOK || !lk.datosOK {
		t.Fatal("current ready did not enable encrypted media and data")
	}
	if err := s.EnableCall(context.Background(), "epoch", "alice", "DEVICE", join, false); err != nil {
		t.Fatal(err)
	}
	if lk.palabraOK || !lk.datosOK {
		t.Fatal("ready viewer must use encrypted reactions without publishing media")
	}
	if err := s.PauseCall(context.Background(), "epoch", []string{"alice"}, nil); err != nil {
		t.Fatal(err)
	}
	if lk.palabraOK || lk.datosOK {
		t.Fatal("rotation failed to pause both media and data")
	}
	for _, tc := range []struct{ device, join string }{{"OTHER", join}, {"DEVICE", uuid.NewString()}} {
		lk.palabraOK, lk.datosOK = false, false
		if err := s.EnableCall(context.Background(), "epoch", "alice", tc.device, tc.join, true); !errors.Is(err, domain.ErrPermission) {
			t.Fatalf("foreign ready accepted: %v", err)
		}
		if lk.palabraOK || lk.datosOK {
			t.Fatal("foreign ready enabled media or data")
		}
	}
}
