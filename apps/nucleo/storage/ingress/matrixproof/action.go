// SPDX-License-Identifier: Apache-2.0
package matrixproof

import (
	"context"
	"time"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/accion"
)

// El formato de las acciones vive en accion, que no importa el SDK de Matrix: el
// núcleo de bóveda del navegador lo firma sin verificar dispositivos. Son alias, no
// copias: matrixproof.Action ES accion.Action, con sus mismos métodos.
type (
	Action         = accion.Action
	Intent         = accion.Intent
	SignedAction   = accion.SignedAction
	Authority      = accion.Authority
	RootEnrollment = accion.RootEnrollment
	RootChallenge  = accion.RootChallenge
	Authorization  = accion.Authorization
)

var (
	ErrUnverified     = accion.ErrUnverified
	ErrRootEnrollment = accion.ErrRootEnrollment
)

func IntentFor(scope storage.Scope, generation int64, epoch, kind string, target storage.PublicationTarget, request any) (Intent, error) {
	return accion.IntentFor(scope, generation, epoch, kind, target, request)
}

// CanonicalBytes preserves encoding/json wire semantics before the maintained
// SDK canonical JSON serializer. Native, Wasm and server sign the same bytes.
func CanonicalBytes(value any) ([]byte, error) { return accion.CanonicalBytes(value) }
func RequestDigest(value any) (string, error)  { return accion.RequestDigest(value) }
func validKey(value string) bool               { return accion.ValidKey(value) }

// VerifyAction requires equality with the server-retained challenge, not just a
// signature over a caller-selected payload. Success alone does not consume the
// nonce: its one-use CAS must commit with the authorized enrollment/claim/ACK.
func (d Device) VerifyAction(ctx context.Context, proof SignedAction, expected Action, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if proof.Action != expected || !expected.Valid(now) || expected.RootKey != d.root.String() {
		return ErrUnverified
	}
	if _, err := d.SigningKey(expected.Scope, expected.Target); err != nil {
		return err
	}
	if ok, err := signatures.VerifySignatureJSON(proof, d.user, d.device.String(), d.signing); err != nil || !ok {
		return ErrUnverified
	}
	return ctx.Err()
}
