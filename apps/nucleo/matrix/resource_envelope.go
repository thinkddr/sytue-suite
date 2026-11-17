// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type preparedResourceEnvelope struct {
	certificate              json.RawMessage
	certificateID            string
	wire                     json.RawMessage
	innerSHA256, outerSHA256 string
}

// Every resource phase signs the same real SDK session certificate and commits
// the same Go frame/encrypted wire. Only the verified domain expectation differs.
func (p *PreparedEncryptedPublication) resourceEnvelope(ctx context.Context, session *PublicationSession, expected sessionproof.CertificateExpectation, frame []byte) (preparedResourceEnvelope, error) {
	var result preparedResourceEnvelope
	certificate, err := p.SignSessionCertificate(ctx, session, expected)
	if err != nil {
		return result, err
	}
	encoded, err := sessionproof.EncodeCertificate(expected)
	if err != nil {
		return result, err
	}
	clear(encoded.Payload)
	result.certificate, err = matrixproof.CanonicalBytes(certificate)
	if err != nil {
		return preparedResourceEnvelope{}, err
	}
	result.certificateID = encoded.ID
	envelope, err := resource.DecodeFrame(frame)
	if err != nil {
		return preparedResourceEnvelope{}, err
	}
	defer clear(envelope.Ciphertext)
	raw, err := json.Marshal(envelope)
	if err != nil {
		return preparedResourceEnvelope{}, err
	}
	defer clear(raw)
	result.innerSHA256, err = resource.CipherJSONDigest(raw)
	if err != nil {
		return preparedResourceEnvelope{}, err
	}
	result.wire = p.Wire()
	result.outerSHA256, err = resource.OuterCipherDigest(result.wire)
	if err != nil {
		return preparedResourceEnvelope{}, err
	}
	return result, nil
}
