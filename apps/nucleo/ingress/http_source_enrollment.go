// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

type enrollmentReader interface {
	Enrollment(context.Context, storage.Scope, string) (contract.EnrollmentState, error)
}

func (s *HTTPSource) Enrollment(ctx context.Context) (contract.EnrollmentState, error) {
	reader, ok := s.cfg.Control.(enrollmentReader)
	if !ok {
		return contract.EnrollmentState{}, contract.ErrContract
	}
	state, err := reader.Enrollment(ctx, s.cfg.Scope, s.cfg.Epoch)
	if err != nil {
		return contract.EnrollmentState{}, ingressHTTPError(err)
	}
	if err := state.Validate(s.cfg.Scope, s.cfg.Epoch); err != nil {
		return contract.EnrollmentState{}, err
	}
	return state, nil
}
