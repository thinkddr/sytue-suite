// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package ingress

import (
	"context"
	"errors"
	"io"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type lostLiveEnrollmentReply struct {
	*HTTPSource
	lost bool
}

func (s *lostLiveEnrollmentReply) Enroll(ctx context.Context, plan contract.KeyPlan, target storage.PublicationTarget, authority matrixproof.Authority) error {
	if err := s.HTTPSource.Enroll(ctx, plan, target, authority); err != nil {
		return err
	}
	if !s.lost {
		s.lost = true
		return io.ErrUnexpectedEOF
	}
	return nil
}

func TestIngressLiveEnrollmentRuntimeFixture(t *testing.T) {
	f := newLiveIngressFixture(t, func(ctx context.Context, setup liveIngressKeySetup) (contract.KeyPlan, error) {
		journal, err := offline.OpenWithSecrets(t.TempDir(), setup.device.Secrets)
		if err != nil {
			return contract.KeyPlan{}, err
		}
		state, err := setup.source.Enrollment(ctx)
		if err != nil {
			return contract.KeyPlan{}, err
		}
		if state.Status != "absent" || setup.device.KeyWrites.Load() != 0 {
			t.Fatal("first enrollment was not actually absent")
		}
		source := &lostLiveEnrollmentReply{HTTPSource: setup.source}
		cfg := EnrollmentConfig{Context: ctx, Scope: setup.configuration.Scope, Target: setup.target, Authority: setup.authority, Journal: journal, Lease: setup.configuration.Lease, Device: setup.device.Client, Authorize: func(ctx context.Context) error { return ctx.Err() }, Source: func(epoch string) (EnrollmentSource, error) {
			if epoch != setup.configuration.Epoch {
				return nil, contract.ErrContract
			}
			return source, nil
		}}
		enrollment, err := NewEnrollment(cfg)
		if err != nil {
			return contract.KeyPlan{}, err
		}
		if _, err := enrollment.Ensure(ctx, setup.configuration.Epoch); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatal("actual accepted signed enrollment reply was not lost", err)
		}
		pending, err := enrollment.Pending(ctx)
		if err != nil || pending.Count != 1 {
			t.Fatal("lost signed enrollment reply lost its real WAL", pending, err)
		}
		var original enrollmentIntent
		found, err := journal.Read(enrollment.owner(), storage.PrivateIngressEnrollmentNamespace, setup.configuration.Epoch, &original)
		if err != nil || !found || original.Plan == nil {
			t.Fatal("actual SDK candidate was not retained before signed HTTP effects", original, err)
		}
		enrollment.Close()
		enrollment, err = NewEnrollment(cfg)
		if err != nil {
			return contract.KeyPlan{}, err
		}
		defer enrollment.Close()
		plan, err := enrollment.Ensure(ctx, setup.configuration.Epoch)
		if err != nil {
			return plan, err
		}
		if plan != *original.Plan || setup.device.KeyWrites.Load() != 1 {
			t.Fatal("accepted enrollment retry generated another real SSSS recipient")
		}
		pending, err = enrollment.Pending(ctx)
		if err != nil || pending.Count != 0 {
			t.Fatal("confirmed signed enrollment remained pending", pending, err)
		}
		history, found, err := enrollment.history(setup.configuration.Epoch)
		if err != nil || !found || history.Plan == nil || *history.Plan != plan {
			t.Fatal("actual confirmed enrollment discarded encrypted original evidence", history, err)
		}
		return plan, nil
	})
	journal, err := offline.OpenWithSecrets(t.TempDir(), f.device.Secrets)
	if err != nil {
		t.Fatal(err)
	}
	executor := liveSDKExecutor{device: f.device.Client, journal: journal, scope: f.vector.Scope}
	worker, err := New(Config{Context: f.ctx, Scope: f.vector.Scope, Epoch: f.vector.Epoch, Journal: journal, Lease: f.lease, Source: f.source, Device: f.device.Client, Executor: executor})
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	result, err := worker.Poll(f.ctx)
	if err != nil || result.Completed != 1 {
		t.Fatal("actual queued ciphertext did not reach verified SDK commit and ACK", result, err)
	}
	pending, err := f.source.Pending(f.ctx, "", 10)
	if err != nil || len(pending.Tasks) != 0 {
		t.Fatal("actual verified ACK did not retire pending delivery", pending, err)
	}
	t.Log("INGRESS_LIVE_ENROLLMENT_RUNTIME_WAL_PASS; authenticated absence, actual SSSS candidate, signed PG enrollment ACK loss/retry, encrypted retained evidence, automatic worker publication and verified ACK")
}
