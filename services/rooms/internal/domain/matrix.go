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
	// ErrRecoveryEscrowRetired: la custodia en claro con KEK (ADR 0081) ya no
	// admite claves nuevas; las claves van envueltas en el dispositivo (ADR 0082).
	ErrRecoveryEscrowRetired = errors.New("rooms: recovery key escrow retired")
)

// MatrixEventRef identifies ciphertext in Matrix. It contains no content or keys.
type MatrixEventRef struct {
	RoomID  string `json:"room_id"`
	EventID string `json:"event_id"`
}
