// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Both pre-epoch bootstrap and active resources use the ONE private SDK intent.
// No external package/request can implement this operation or put it on a raw
// room allowlist. In particular, neither kind exposes direct SDK Send.
type resourceOperation interface {
	operationContext(context.Context) context.Context
	operationOwner() *Client
	operationBinding() resource.SessionBinding
	operationActive() bool
	ActorScope() storage.Scope
	RoomID() string
	Revalidate(context.Context) error
}

func (r *ResourceRoom) operationOwner() *Client                   { return r.owner }
func (r *ResourceRoom) operationBinding() resource.SessionBinding { return r.current.Binding() }
func (r *ResourceRoom) operationActive() bool                     { return r != nil && !r.retired.Load() }

func cloneResourceJSON[T any](value T) (T, error) {
	var out T
	raw, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	defer clear(raw)
	err = json.Unmarshal(raw, &out)
	return out, err
}
