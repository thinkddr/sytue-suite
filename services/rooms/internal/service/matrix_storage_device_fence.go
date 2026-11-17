// SPDX-License-Identifier: Apache-2.0
package service

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

func (s *Service) FenceStorageDevice(ctx context.Context, proof ingress.ControlProof) (ingress.DeviceFence, error) {
	scope := proof.Request.Scope
	if err := s.matrixAuthorityScope(ctx, scope.Subject, scope.Product); err != nil {
		return ingress.DeviceFence{}, err
	}
	backend := s.matrix
	coverage, err := backend.RequirePublicationCoverage(ctx)
	if err != nil {
		return ingress.DeviceFence{}, err
	}
	// Do not reuse the matrixContext from this committed planning transaction.
	if _, err := s.pstore.PlanMatrixStorageDeviceFence(ctx, proof); err != nil {
		return ingress.DeviceFence{}, err
	}
	return s.pstore.ConfirmMatrixStorageDeviceFence(ctx, proof, func(ctx context.Context, scope storage.Scope, previous ingress.Assignment, revoke bool) error {
		if err := requireSamePublicationOrigin(ctx, backend, coverage); err != nil {
			return err
		}
		if revoke {
			if err := backend.RevokeStorageDevice(ctx, scope, previous.Target, previous.SigningKey); err != nil {
				return err
			}
		}
		if err := backend.RequireStorageDeviceAbsent(ctx, scope, previous.Target); err != nil {
			return err
		}
		return requireSamePublicationOrigin(ctx, backend, coverage)
	})
}

func (s *Service) ReadStorageDeviceFence(ctx context.Context, proof ingress.ControlProof) (ingress.DeviceFence, error) {
	scope := proof.Request.Scope
	if err := s.matrixAuthorityScope(ctx, scope.Subject, scope.Product); err != nil {
		return ingress.DeviceFence{}, err
	}
	backend := s.matrix
	coverage, err := backend.RequirePublicationCoverage(ctx)
	if err != nil {
		return ingress.DeviceFence{}, err
	}
	return s.pstore.ReadMatrixStorageDeviceFence(ctx, proof, func(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) error {
		if err := backend.RequireStorageDeviceAbsent(ctx, scope, target); err != nil {
			return err
		}
		return requireSamePublicationOrigin(ctx, backend, coverage)
	})
}
