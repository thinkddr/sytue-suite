//go:build js && wasm

// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"syscall/js"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
)

func init() {
	domainOperations["matrix.session.certificate.encode"] = encodeMatrixSessionCertificate
	domainOperations["matrix.session.certificate.verify"] = verifyMatrixSessionCertificate
}

// The web session provider independently pins expected root/user and verifies
// live account/device authority. This worker has no network or trust-on-first-
// use: it never derives an expectation from a certificate or /keys/query result.
func certificateExpectation(req js.Value) (sessionproof.CertificateExpectation, error) {
	raw, err := bytesArg(req.Get("expectation"), sessionproof.MaxCertificateBytes)
	if err != nil {
		return sessionproof.CertificateExpectation{}, err
	}
	defer clear(raw)
	return sessionproof.DecodeCertificateExpectation(raw)
}

func encodeMatrixSessionCertificate(req js.Value) (any, error) {
	expected, err := certificateExpectation(req)
	if err != nil {
		return nil, err
	}
	encoded, err := sessionproof.EncodeCertificate(expected)
	if err != nil {
		return nil, err
	}
	defer clear(encoded.Payload)
	return map[string]any{"id": encoded.ID, "payload": bytesValue(encoded.Payload)}, nil
}

func verifyMatrixSessionCertificate(req js.Value) (any, error) {
	expected, err := certificateExpectation(req)
	if err != nil {
		return nil, err
	}
	raw, err := bytesArg(req.Get("certificate"), sessionproof.MaxCertificateBytes)
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	chain, err := bytesArg(req.Get("origin"), 1<<20)
	if err != nil {
		return nil, err
	}
	defer clear(chain)
	// Keep the origin's typed JSON contract at this I/O boundary. Signature and
	// root verification use the maintained Matrix SDK, shared with native.
	origin, err := sessionproof.DecodeCertificateOrigin(chain)
	if err != nil {
		return nil, err
	}
	verified, err := sessionproof.VerifyCertificateOrigin(context.Background(), raw, expected, origin)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": verified.ID()}, nil
}
