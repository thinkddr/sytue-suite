// SPDX-License-Identifier: Apache-2.0
package ingress

import "git.sytue.eu/sytue/one/apps/nucleo/storage"

// ControlRequest is the complete public operation committed by a device
// signature. It contains no MIME, calendar fields, addresses or plaintext hash.
// Only one typed operation is allowed, so a challenge cannot hide extra work.
type ControlRequest struct {
	Version        int                       `json:"v"`
	Scope          storage.Scope             `json:"scope"`
	Epoch          string                    `json:"epoch"`
	Kind           string                    `json:"kind"`
	Target         storage.PublicationTarget `json:"target"`
	Enrollment     *KeyPlan                  `json:"enrollment,omitempty"`
	Claim          *ClaimRequest             `json:"claim,omitempty"`
	Acknowledgment *AcknowledgmentRequest    `json:"acknowledgment,omitempty"`
	Handoff        *HandoffRequest           `json:"handoff,omitempty"`
}
type (
	ClaimRequest struct {
		TaskID       string `json:"task_id"`
		Token        string `json:"token"`
		LeaseSeconds int64  `json:"lease_seconds"`
	}
	AcknowledgmentRequest struct {
		Claim   Claim   `json:"claim"`
		Receipt Receipt `json:"receipt"`
	}
)

func (r ControlRequest) Validate(scope storage.Scope, epoch string) error {
	if r.Version != 1 || r.Scope != scope || !scope.Valid() || (scope.Product != "mail" && scope.Product != "calendar") || r.Epoch != epoch || !storage.MigrationEpoch(epoch) || !r.Target.Valid() {
		return ErrContract
	}
	var valid bool
	switch r.Kind {
	case "key.enroll":
		valid = r.validEnrollment(scope, epoch)
	case "task.claim":
		valid = r.validClaim()
	case "task.ack":
		valid = r.validAcknowledgment(scope, epoch)
	case "task.handoff":
		valid = r.validHandoff(scope, epoch)
	}
	if !valid {
		return ErrContract
	}
	return nil
}

func (r ControlRequest) validEnrollment(scope storage.Scope, epoch string) bool {
	return r.Enrollment != nil && r.Claim == nil && r.Acknowledgment == nil && r.Handoff == nil && r.Enrollment.Validate(scope, epoch) == nil
}

func (r ControlRequest) validClaim() bool {
	return r.Enrollment == nil && r.Claim != nil && r.Acknowledgment == nil && r.Handoff == nil && storage.Identifier(r.Claim.TaskID) && storage.Identifier(r.Claim.Token) && r.Claim.LeaseSeconds >= 1 && r.Claim.LeaseSeconds <= 300
}

func (r ControlRequest) validAcknowledgment(scope storage.Scope, epoch string) bool {
	return r.Enrollment == nil && r.Claim == nil && r.Acknowledgment != nil && r.Handoff == nil && r.Acknowledgment.Claim.Validate(scope, epoch) == nil && r.Acknowledgment.Claim.Target == r.Target && r.Acknowledgment.Receipt.Validate(r.Acknowledgment.Claim) == nil
}

func (r ControlRequest) validHandoff(scope storage.Scope, epoch string) bool {
	return r.Enrollment == nil && r.Claim == nil && r.Acknowledgment == nil && r.Handoff != nil && r.Handoff.validate(scope, epoch, r.Target)
}
