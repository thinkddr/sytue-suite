// SPDX-License-Identifier: Apache-2.0
package callaccess

import (
	"context"
	"net/http"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

// StorageDeviceFencer extends ongoing ingress without changing existing callers
// of PrivateIngressAuthority. Only public fence evidence crosses this channel.
type StorageDeviceFencer interface {
	FenceStorageDevice(context.Context, ingress.ControlProof) (ingress.DeviceFence, error)
	ReadStorageDeviceFence(context.Context, ingress.ControlProof) (ingress.DeviceFence, error)
}

func (c *Client) storageDeviceFence(ctx context.Context, path string, proof ingress.ControlProof, beginOnly bool) (ingress.DeviceFence, error) {
	request := proof.Request
	if proof.Validate(request.Scope, false) != nil || request.Kind != "task.handoff" || request.Handoff == nil || beginOnly && request.Handoff.Phase != "begin" {
		return ingress.DeviceFence{}, ErrUnavailable
	}
	var out ingress.DeviceFence
	if err := c.request(ctx, http.MethodPost, path, proof, &out); err != nil {
		return ingress.DeviceFence{}, err
	}
	if out.ValidateProof(proof) != nil {
		return ingress.DeviceFence{}, ErrUnavailable
	}
	return out, nil
}

func (c *Client) FenceStorageDevice(ctx context.Context, proof ingress.ControlProof) (ingress.DeviceFence, error) {
	return c.storageDeviceFence(ctx, "/internal/storage-authority/device-fence", proof, true)
}

func (c *Client) ReadStorageDeviceFence(ctx context.Context, proof ingress.ControlProof) (ingress.DeviceFence, error) {
	return c.storageDeviceFence(ctx, "/internal/storage-authority/device-fence/state", proof, false)
}

var _ StorageDeviceFencer = (*Client)(nil)
