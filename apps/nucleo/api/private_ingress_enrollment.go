// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

// Enrollment returns only an explicit, validated server state. Errors retain
// their meaning, especially 404/409 and unavailable or revoked authority.
func (c *PrivateIngress) Enrollment(ctx context.Context, scope storage.Scope, epoch string) (ingress.EnrollmentState, error) {
	if !storage.MigrationEpoch(epoch) {
		return ingress.EnrollmentState{}, ingress.ErrContract
	}
	wire, err := c.request(scope, "enrollment", ingress.EnrollmentRequest{Epoch: epoch})
	if err != nil {
		return ingress.EnrollmentState{}, err
	}
	result, err := JSON[ingress.EnrollmentState](ctx, c.c, wire)
	if err != nil {
		return ingress.EnrollmentState{}, err
	}
	if err := result.Validate(scope, epoch); err != nil {
		return ingress.EnrollmentState{}, err
	}
	return result, nil
}
