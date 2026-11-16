// SPDX-License-Identifier: Apache-2.0
package publication

import (
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// Receipt is public admission state, not proof that ciphertext has been read
// back and authenticated. Only accepted receipts carry an event reference.
type Receipt struct {
	Version       int                `json:"v"`
	Marker        Marker             `json:"marker"`
	Status        string             `json:"status"`
	RequestSHA256 string             `json:"request_sha256"`
	Reference     *storage.Reference `json:"reference,omitempty"`
}

func (r Receipt) Valid() bool {
	if r.Version != 1 || !r.Marker.Valid() || !validDigest(r.RequestSHA256) {
		return false
	}
	return validReceiptState(r.Status, r.Reference)
}

func validReceiptState(status string, reference *storage.Reference) bool {
	switch status {
	case "reserved", "sending", "cancelled":
		return reference == nil
	case "accepted":
		return reference != nil && reference.Valid()
	default:
		return false
	}
}

// DecodeReceipt binds a control-plane response to the exact client-retained
// request. It grants no authority by itself and does not replace SDK readback.
func DecodeReceipt(raw []byte, expected Request) (Receipt, error) {
	if !expected.Valid() {
		return Receipt{}, ErrInvalid
	}
	var value Receipt
	if decodeControlJSON(raw, 16384, &value) != nil || !value.Valid() {
		return Receipt{}, ErrInvalid
	}
	digest, err := matrixproof.RequestDigest(expected)
	if err != nil || value.RequestSHA256 != digest || (value.Reference != nil && value.Reference.RoomID != expected.Target.RoomID) {
		return Receipt{}, ErrInvalid
	}
	return value, nil
}

// ReservedFor returns only a matching, still-reserved marker. Sending, accepted
// and cancelled responses must be reconciled rather than used for another send.
// The homeserver origin still independently authorizes every actual event.
func (r Receipt) ReservedFor(expected Request) (Marker, error) {
	if !r.Valid() || r.Status != "reserved" || !expected.Valid() {
		return Marker{}, ErrInvalid
	}
	digest, err := matrixproof.RequestDigest(expected)
	if err != nil || digest != r.RequestSHA256 {
		return Marker{}, ErrInvalid
	}
	return r.Marker, nil
}
