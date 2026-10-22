// SPDX-License-Identifier: Apache-2.0
package domain

import (
	"errors"

	"github.com/google/uuid"
)

var (
	ErrEncryptionRequired    = errors.New("rooms: an authenticated Matrix device is required")
	ErrEncryptionUnavailable = errors.New("rooms: encrypted call coordination is unavailable")
	ErrEncryptionGeneration  = errors.New("rooms: call encryption generation changed")
	ErrEncryptedOperation    = errors.New("rooms: this operation is unavailable in encrypted calls")
)

// CallEncryption contains authorization metadata only. Media keys stay on devices.
type CallEncryption struct {
	CallID         uuid.UUID    `json:"call_id"`
	LiveKitRoom    string       `json:"livekit_room"`
	MatrixRoomID   string       `json:"matrix_room_id"`
	Generation     int64        `json:"generation"`
	ExpiresAtMS    int64        `json:"expires_at_ms"`
	LeaderSubject  string       `json:"leader_subject"`
	LeaderDeviceID string       `json:"leader_device_id"`
	Members        []CallMember `json:"members"`
}
type CallMember struct {
	Subject  string `json:"subject"`
	DeviceID string `json:"device_id"`
}
