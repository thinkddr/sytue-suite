// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

func (c *PrivateIngress) Inspect(ctx context.Context, scope storage.Scope, epoch, id string) (ingress.TaskState, error) {
	if !storage.MigrationEpoch(epoch) || !storage.Identifier(id) {
		return ingress.TaskState{}, ingress.ErrContract
	}
	wire, err := c.request(scope, "inspect", ingress.InspectRequest{Epoch: epoch, ID: id})
	if err != nil {
		return ingress.TaskState{}, err
	}
	result, err := JSON[ingress.TaskState](ctx, c.c, wire)
	if err != nil {
		return ingress.TaskState{}, err
	}
	return result, result.Validate(scope, epoch, id)
}

func (c *PrivateIngress) OpenHandoff(ctx context.Context, request ingress.ControlProof, task ingress.Task) (storage.StreamSourceRow, io.ReadCloser, error) {
	body := ingress.HandoffBodyRequest{Control: request, Task: task}
	if body.Validate(task.Scope) != nil {
		return storage.StreamSourceRow{}, nil, ingress.ErrContract
	}
	wire, err := c.request(task.Scope, "handoff/body", body)
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	row := task.Cipher
	input, err := openPrivateSourceResponse(ctx, c.c, wire, row.Size, row.SHA256)
	return row, input, err
}
