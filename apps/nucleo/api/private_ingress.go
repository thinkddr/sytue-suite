// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type PrivateIngress struct {
	c       *Client
	product string
}

func NewPrivateIngress(serviceURL, product string, session Session, httpc *http.Client) (*PrivateIngress, error) {
	if !ingress.Product(product) {
		return nil, ingress.ErrContract
	}
	return &PrivateIngress{c: New(serviceURL+"/api/v1", session, httpc), product: product}, nil
}
func (m *Mail) PrivateIngress() *PrivateIngress { return &PrivateIngress{c: m.c, product: "mail"} }
func (c *Calendars) PrivateIngress() *PrivateIngress {
	return &PrivateIngress{c: c.c, product: "calendar"}
}

// PrivateIngress es la entrada privada de avisos de One (ADR 0086).
func (h *Hub) PrivateIngress() *PrivateIngress { return &PrivateIngress{c: h.c, product: "hub"} }

func (c *PrivateIngress) request(scope storage.Scope, path string, body any) (Request, error) {
	if c == nil || c.c == nil {
		return Request{}, ingress.ErrContract
	}
	request, err := privateAppRequest(scope, c.product, "/private-storage/ingress/"+path)
	if err != nil {
		return Request{}, err
	}
	request.Method = http.MethodPost
	request.Header.Set("Content-Type", "application/json")
	request.Body, err = json.Marshal(body)
	return request, err
}

func (c *PrivateIngress) Challenge(ctx context.Context, request ingress.ControlRequest, authority matrixproof.Authority) (matrixproof.Action, error) {
	if request.Validate(request.Scope, request.Epoch) != nil || authority.RequireActive(request.Scope, authority.Generation) != nil {
		return matrixproof.Action{}, ingress.ErrContract
	}
	intent, err := matrixproof.IntentFor(request.Scope, authority.Generation, request.Epoch, request.Kind, request.Target, request)
	if err != nil {
		return matrixproof.Action{}, err
	}
	wire, err := c.request(request.Scope, "challenge", request)
	if err != nil {
		return matrixproof.Action{}, err
	}
	action, err := JSON[matrixproof.Action](ctx, c.c, wire)
	if err != nil {
		return matrixproof.Action{}, err
	}
	if action.Intent() != intent || action.RootKey != authority.RootKey || !action.Valid(time.Now()) {
		return matrixproof.Action{}, ingress.ErrRevoked
	}
	return action, nil
}

func (c *PrivateIngress) Apply(ctx context.Context, request ingress.ControlProof) (ingress.ControlResult, error) {
	if request.Validate(request.Request.Scope, false) != nil {
		return ingress.ControlResult{}, ingress.ErrContract
	}
	wire, err := c.request(request.Request.Scope, "apply", request)
	if err != nil {
		return ingress.ControlResult{}, err
	}
	result, err := JSON[ingress.ControlResult](ctx, c.c, wire)
	if err != nil {
		return ingress.ControlResult{}, err
	}
	if err := result.Validate(request.Request, request.Proof.Action); err != nil {
		return ingress.ControlResult{}, err
	}
	return result, nil
}

func (c *PrivateIngress) Pending(ctx context.Context, scope storage.Scope, epoch, after string, limit int) (ingress.PendingPage, error) {
	if !storage.MigrationEpoch(epoch) || (after != "" && !storage.Identifier(after)) || limit < 1 || limit > ingress.MaxPendingTasks {
		return ingress.PendingPage{}, ingress.ErrContract
	}
	wire, err := c.request(scope, "pending", ingress.PendingRequest{Epoch: epoch, After: after, Limit: limit})
	if err != nil {
		return ingress.PendingPage{}, err
	}
	result, err := JSON[ingress.PendingPage](ctx, c.c, wire)
	if err != nil {
		return ingress.PendingPage{}, err
	}
	if err := result.Validate(scope, epoch, after); err != nil {
		return ingress.PendingPage{}, err
	}
	if len(result.Tasks) > limit {
		return ingress.PendingPage{}, ingress.ErrContract
	}
	return result, nil
}

func (c *PrivateIngress) Revalidate(ctx context.Context, request ingress.ControlProof) (matrixproof.Authorization, error) {
	if request.Validate(request.Request.Scope, true) != nil {
		return matrixproof.Authorization{}, ingress.ErrContract
	}
	wire, err := c.request(request.Request.Scope, "revalidate", request)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	result, err := JSON[matrixproof.Authorization](ctx, c.c, wire)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if err := result.Validate(request.Proof.Action); err != nil {
		return matrixproof.Authorization{}, err
	}
	return result, nil
}

func (c *PrivateIngress) Open(ctx context.Context, request ingress.ControlProof) (storage.StreamSourceRow, io.ReadCloser, error) {
	if request.Validate(request.Request.Scope, true) != nil {
		return storage.StreamSourceRow{}, nil, ingress.ErrContract
	}
	wire, err := c.request(request.Request.Scope, "body", request)
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	row := request.Claim.Task.Cipher
	body, err := openPrivateSourceResponse(ctx, c.c, wire, row.Size, row.SHA256)
	return row, body, err
}
