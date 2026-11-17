// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
)

// RevalidateIdentity checks fresh host authority for a sensitive local operation.
// Current is routing metadata; it cannot authorize retained bytes or callbacks.
func RevalidateIdentity(ctx context.Context, port Port, expected Identity) error {
	if port == nil || expected.Validate() != nil {
		return ErrRejected
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := port.Current(ctx)
	if err != nil {
		return err
	}
	if current != expected {
		return ErrRejected
	}
	raw, err := port.Call(ctx, expected.Scope, Authorize, nil)
	defer clear(raw)
	if err != nil {
		return err
	}
	if len(raw) != 0 {
		return ErrRejected
	}
	current, err = port.Current(ctx)
	if err != nil {
		return err
	}
	if current != expected {
		return ErrRejected
	}
	return ctx.Err()
}
