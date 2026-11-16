// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type httpHandoffControl interface {
	Inspect(context.Context, storage.Scope, string, string) (contract.TaskState, error)
	OpenHandoff(context.Context, contract.ControlProof, contract.Task) (storage.StreamSourceRow, io.ReadCloser, error)
}

func (s *HTTPSource) Inspect(ctx context.Context, id string) (contract.TaskState, error) {
	port, ok := s.cfg.Control.(httpHandoffControl)
	if !ok {
		return contract.TaskState{}, contract.ErrContract
	}
	result, err := port.Inspect(ctx, s.cfg.Scope, s.cfg.Epoch, id)
	if err != nil {
		return contract.TaskState{}, ingressHTTPError(err)
	}
	return result, result.Validate(s.cfg.Scope, s.cfg.Epoch, id)
}

func (s *HTTPSource) Handoff(ctx context.Context, request contract.ControlRequest, authority matrixproof.Authority) (contract.HandoffView, error) {
	if request.Kind != "task.handoff" {
		return contract.HandoffView{}, contract.ErrContract
	}
	proof, err := s.sign(ctx, request, authority)
	if err != nil {
		return contract.HandoffView{}, err
	}
	result, err := s.cfg.Control.Apply(ctx, proof)
	if err != nil {
		return contract.HandoffView{}, ingressHTTPError(err)
	}
	if validateHTTPControlResult(result, proof, authority) != nil || result.Handoff == nil {
		return contract.HandoffView{}, contract.ErrClaim
	}
	return *result.Handoff, nil
}

func (s *HTTPSource) OpenHandoff(ctx context.Context, request contract.ControlRequest, task contract.Task, authority matrixproof.Authority) (storage.StreamSourceRow, io.ReadCloser, error) {
	port, ok := s.cfg.Control.(httpHandoffControl)
	if !ok || request.Handoff == nil || request.Handoff.Phase != "begin" {
		return storage.StreamSourceRow{}, nil, contract.ErrContract
	}
	proof, err := s.sign(ctx, request, authority)
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	row, body, err := port.OpenHandoff(ctx, proof, task)
	if err != nil {
		return row, body, ingressHTTPError(err)
	}
	if row != task.Cipher {
		if body != nil {
			_ = body.Close()
		}
		return storage.StreamSourceRow{}, nil, contract.ErrContract
	}
	return row, body, nil
}
