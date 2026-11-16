// SPDX-License-Identifier: Apache-2.0
package ingress

import "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"

// HandoffRejection permanently closes one begin intent before any fence or
// claim exists. A network error is never interchangeable with this result.
type HandoffRejection struct {
	Version       int                   `json:"v"`
	ID            string                `json:"id"`
	RequestSHA256 string                `json:"request_sha256"`
	Authority     matrixproof.Authority `json:"authority"`
	Reason        string                `json:"reason"`
}

func (r HandoffRejection) Validate(request ControlRequest, authority matrixproof.Authority) error {
	if request.Kind != "task.handoff" || request.Handoff == nil || request.Handoff.Phase != "begin" || request.Validate(request.Scope, request.Epoch) != nil || authority.RequireActive(request.Scope, authority.Generation) != nil {
		return ErrContract
	}
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		return err
	}
	if r.Version != 1 || r.ID != request.Handoff.ID || r.RequestSHA256 != digest || r.Authority != authority || r.Reason != "assignment_changed" {
		return ErrRevoked
	}
	return nil
}
