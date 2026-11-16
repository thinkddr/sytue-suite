// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

type (
	driveOperationContextKey   struct{}
	drivePublicationContextKey struct{}
)

func TestDrivePublicationContextBindsOnlyTheOriginalAdmittedOperation(t *testing.T) {
	active, called := 0, 0
	life, cancel := context.WithCancel(t.Context())
	gate := offline.NewWorkGateWithLease(life, func(context.Context) (func(), error) { active++; return func() { active-- }, nil })
	r := &Repository{ctx: life, cancel: cancel, gate: gate, cfg: Config{BindPublicationContext: func(ctx context.Context) (context.Context, error) {
		called++
		if active != 1 || ctx.Value(driveOperationContextKey{}) != "original" {
			t.Fatal("Drive binding lost original lease/caller")
		}
		return context.WithValue(ctx, drivePublicationContextKey{}, "bound"), nil
	}}}
	defer r.Close()
	parent, stop := context.WithCancel(context.WithValue(t.Context(), driveOperationContextKey{}, "original"))
	job, done, err := r.start(parent)
	if done != nil {
		defer done()
	}
	if err != nil || called != 1 || active != 1 || job.Value(drivePublicationContextKey{}) != "bound" {
		t.Fatal("Drive operation skipped original publication binding", err)
	}
	stop()
	if !errors.Is(job.Err(), context.Canceled) {
		t.Fatal("Drive bound operation lost caller cancellation")
	}
	done()
	if active != 0 {
		t.Fatal("Drive bound operation leaked lease")
	}
	r.cfg.BindPublicationContext = func(context.Context) (context.Context, error) { return context.Background(), nil }
	_, failedDone, err := r.start(t.Context())
	if failedDone != nil {
		failedDone()
	}
	if err == nil || failedDone != nil || active != 0 {
		t.Fatal("Drive admitted a replacement caller or retained its failed lease", err)
	}
	rejected := errors.New("publication authority retired")
	r.cfg.BindPublicationContext = func(context.Context) (context.Context, error) { return nil, rejected }
	if _, done, err := r.start(t.Context()); !errors.Is(err, rejected) || done != nil || active != 0 {
		t.Fatal("Drive hid publication rejection or retained lease", err)
	}
}
