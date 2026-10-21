// SPDX-License-Identifier: Apache-2.0
package domain

import "errors"

const MatrixControllerLocalpart = "rooms_controller"

const (
	MatrixEncryption = "m.megolm.v1.aes-sha2"
	LegacyEncryption = "legacy"
)

var (
	ErrMatrixUnavailable = errors.New("rooms: encrypted messaging unavailable")
	ErrEncryptedRequired = errors.New("rooms: client-side encrypted event required")
)

// MatrixEventRef identifies ciphertext in Matrix. It contains no content or keys.
type MatrixEventRef struct {
	RoomID  string `json:"room_id"`
	EventID string `json:"event_id"`
}
