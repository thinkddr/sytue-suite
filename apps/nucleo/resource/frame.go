// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// FrameEventType is the one Matrix application event shared by native and web.
// Its content is already hybrid encrypted; plaintext and file keys belong only
// inside Envelope. Large content uses resource-bound encrypted files, not a
// larger Matrix PDU or an owner's personal storage record.
const FrameEventType = "org.sytue.resource.frame.v1"

type Frame struct {
	Version  int          `json:"v"`
	Kind     string       `json:"kind"`
	Envelope pqc.Envelope `json:"envelope"`
}

func EncodeFrame(envelope pqc.Envelope) ([]byte, error) {
	if envelope.Version != pqc.EnvelopeVersion || envelope.Profile != pqc.HybridProfile || len(envelope.Ciphertext) == 0 || len(envelope.Ciphertext) > pqc.MaxEnvelopeCiphertext {
		return nil, ErrInvalid
	}
	return matrixproof.CanonicalBytes(Frame{Version: 1, Kind: FrameEventType, Envelope: envelope})
}

func DecodeFrame(raw []byte) (pqc.Envelope, error) {
	var frame Frame
	if err := decodeReservationValue(raw, &frame); err != nil || frame.Version != 1 || frame.Kind != FrameEventType {
		return pqc.Envelope{}, ErrInvalid
	}
	canonical, err := EncodeFrame(frame.Envelope)
	clear(canonical)
	if err != nil {
		return pqc.Envelope{}, err
	}
	return frame.Envelope, nil
}

// EncodeFrameEnvelope accepts only the canonical, strictly typed envelope wire
// produced by the shared Go core. Unknown fields and duplicate keys are denied.
func EncodeFrameEnvelope(raw []byte) ([]byte, error) {
	var envelope pqc.Envelope
	if err := decodeReservationValue(raw, &envelope); err != nil {
		return nil, err
	}
	defer clear(envelope.Ciphertext)
	return EncodeFrame(envelope)
}
