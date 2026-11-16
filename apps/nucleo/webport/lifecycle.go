// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"errors"
	"sync"
)

// PortRevocations is populated during command initialization. Each hook cancels
// and joins its own repositories/readers/jobs; cancellation never permits an
// early success while data IO remains alive. All hooks finish before ACK/erasure.
type PortRevocations []func(context.Context, string) error

func (hooks PortRevocations) Revoke(ctx context.Context, port string) error {
	if !ValidID(port) {
		return ErrRejected
	}
	var wait sync.WaitGroup
	results := make(chan error, len(hooks))
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					results <- errors.New("port resource cleanup failed")
				}
			}()
			if err := hook(ctx, port); err != nil {
				results <- err
			}
		}()
	}
	wait.Wait()
	close(results)
	var failures []error
	for err := range results {
		failures = append(failures, err)
	}
	if ctx.Err() != nil {
		failures = append(failures, ctx.Err())
	}
	return errors.Join(failures...)
}
