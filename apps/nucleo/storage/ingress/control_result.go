// SPDX-License-Identifier: Apache-2.0
package ingress

import "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"

type ControlResult struct {
	Version       int                       `json:"v"`
	Authorization matrixproof.Authorization `json:"authorization"`
	Claim         *Claim                    `json:"claim,omitempty"`
	Receipt       *Receipt                  `json:"receipt,omitempty"`
	Handoff       *HandoffView              `json:"handoff,omitempty"`
}

func (r ControlResult) Validate(request ControlRequest, expected matrixproof.Action) error {
	digest, err := matrixproof.RequestDigest(request)
	if err != nil || r.Version != 1 || request.Validate(expected.Scope, expected.Epoch) != nil || r.Authorization.Validate(expected) != nil || request.Kind != expected.Kind || request.Target != expected.Target || digest != expected.RequestSHA256 {
		return ErrContract
	}
	return r.validateOperationResult(request)
}

func (r ControlResult) validateOperationResult(request ControlRequest) error {
	switch request.Kind {
	case "key.enroll":
		if !r.validEnrollmentResult() {
			return ErrContract
		}
	case "task.claim":
		if !r.validClaimResult(request) {
			return ErrClaim
		}
	case "task.ack":
		if !r.validAcknowledgmentResult(request) {
			return ErrReceipt
		}
	case "task.handoff":
		if r.Claim != nil || r.Receipt != nil || r.Handoff == nil {
			return ErrContract
		}
		return r.Handoff.Validate(request, r.Authorization.Authority)
	default:
		return ErrContract
	}
	return nil
}

func (r ControlResult) validEnrollmentResult() bool {
	return r.Claim == nil && r.Receipt == nil && r.Handoff == nil
}

func (r ControlResult) validClaimResult(request ControlRequest) bool {
	return r.Claim != nil && r.Receipt == nil && r.Handoff == nil && r.Claim.Validate(request.Scope, request.Epoch) == nil && r.Claim.Task.ID == request.Claim.TaskID && r.Claim.Target == request.Target && r.Claim.Token == request.Claim.Token
}

func (r ControlResult) validAcknowledgmentResult(request ControlRequest) bool {
	return r.Claim == nil && r.Receipt != nil && r.Handoff == nil && *r.Receipt == request.Acknowledgment.Receipt
}
