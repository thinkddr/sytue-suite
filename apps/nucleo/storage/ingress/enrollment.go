// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrEnrollmentEpoch = errors.New("ingress: retained enrollment belongs to another source epoch")

// EnrollmentState reports the latest public key generation, including its
// revocation. Absence is an explicit authenticated response, never an HTTP error.
// A revoked latest generation must not expose an older active plan as current.
type EnrollmentState struct {
	Version    int           `json:"v"`
	Scope      storage.Scope `json:"scope"`
	Epoch      string        `json:"epoch"`
	Status     string        `json:"status"`
	Generation int64         `json:"generation"`
	Plan       *KeyPlan      `json:"plan,omitempty"`
}

type EnrollmentRequest struct {
	Epoch string `json:"epoch"`
}

func (s EnrollmentState) Validate(scope storage.Scope, epoch string) error {
	if s.Version != 1 || s.Scope != scope || !scope.Valid() || s.Epoch != epoch || !storage.MigrationEpoch(epoch) {
		return ErrContract
	}
	switch s.Status {
	case "absent":
		if s.Generation == 0 && s.Plan == nil {
			return nil
		}
	case "active":
		if s.Plan != nil && s.Generation == s.Plan.Generation && s.Plan.Validate(scope, epoch) == nil {
			return nil
		}
	case "revoked":
		if s.Generation > 0 && s.Generation < 1<<53 && s.Plan == nil {
			return nil
		}
	}
	return ErrContract
}
