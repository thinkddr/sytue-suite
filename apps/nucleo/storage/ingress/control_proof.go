// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type ControlProof struct {
	Request ControlRequest           `json:"request"`
	Proof   matrixproof.SignedAction `json:"proof"`
	Claim   *Claim                   `json:"claim,omitempty"`
}
type PendingRequest struct {
	Epoch string `json:"epoch"`
	After string `json:"after,omitempty"`
	Limit int    `json:"limit"`
}

// Validate checks wire binding only. The domain must additionally verify the
// retained nonce, independent Rooms root and actual SDK signature transactionally.
func (p ControlProof) Validate(scope storage.Scope, requireClaim bool) error {
	if p.Request.Validate(scope, p.Request.Epoch) != nil || !p.Proof.Valid(time.Now()) {
		return ErrContract
	}
	intent, err := matrixproof.IntentFor(scope, p.Proof.Generation, p.Request.Epoch, p.Request.Kind, p.Request.Target, p.Request)
	if err != nil || p.Proof.Intent() != intent {
		return ErrContract
	}
	if !requireClaim {
		if p.Claim != nil {
			return ErrContract
		}
		return nil
	}
	if p.Claim == nil || p.Request.Kind != "task.claim" || p.Claim.Validate(scope, p.Request.Epoch) != nil || p.Claim.Target != p.Request.Target || p.Claim.Task.ID != p.Request.Claim.TaskID || p.Claim.Token != p.Request.Claim.Token {
		return ErrClaim
	}
	return nil
}
