// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"io"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/keyring"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func (s *Storage) StoragePublicationTarget() (storage.PublicationTarget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.binding.Valid() || s.binding.Scope != s.identity.Scope || s.binding.Target.DeviceID != s.identity.Binding.DeviceID {
		return storage.PublicationTarget{}, ErrRejected
	}
	return s.binding.Target, nil
}

func (s *Storage) recoverableIngress(ctx context.Context, scope storage.Scope) (keyring.Secrets, error) {
	if s.StorageIdentityVersion() != storage.IsolatedIdentity || scope != s.identity.Scope {
		return nil, ErrRejected
	}
	binding, err := s.PrepareStorage(ctx, scope)
	if err != nil {
		return nil, err
	}
	if _, err = s.RequireRecoverableIngress(ctx, scope, binding.Target); err != nil {
		return nil, err
	}
	return s.IngressSecrets()
}

func (s *Storage) CreateIngressKey(ctx context.Context, scope storage.Scope, epoch string, generation int64) (ingress.KeyPlan, error) {
	secrets, err := s.recoverableIngress(ctx, scope)
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	return keyring.Create(ctx, secrets, scope, epoch, generation)
}

func (s *Storage) RecoverIngressEnrollment(ctx context.Context, plan ingress.KeyPlan) (ingress.KeyPlan, error) {
	if plan.Validate(s.identity.Scope, plan.Epoch) != nil {
		return ingress.KeyPlan{}, ErrRejected
	}
	secrets, err := s.recoverableIngress(ctx, plan.Scope)
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	return keyring.Recover(ctx, secrets, plan)
}

func (s *Storage) OpenIngressBody(ctx context.Context, task ingress.Task, source io.ReaderAt) ([]byte, *io.SectionReader, error) {
	if source == nil || task.Validate(s.identity.Scope, task.Epoch) != nil {
		return nil, nil, ErrRejected
	}
	secrets, err := s.recoverableIngress(ctx, task.Scope)
	if err != nil {
		return nil, nil, err
	}
	return keyring.Open(ctx, secrets, task, source)
}

type controlIntentWire struct {
	Generation    int64                     `json:"authority_generation"`
	Scope         storage.Scope             `json:"scope"`
	Epoch         string                    `json:"epoch"`
	Kind          string                    `json:"kind"`
	Target        storage.PublicationTarget `json:"target"`
	RequestSHA256 string                    `json:"request_sha256"`
}

// SignIngressAction preserves native intent/lease checks and delegates only the
// maintained SDK signing action. It cannot enroll or rotate an owner's root.
func (s *Storage) SignIngressAction(ctx context.Context, lease offline.WorkLease, expected matrixproof.Intent, action matrixproof.Action) (matrixproof.SignedAction, error) {
	var proof matrixproof.SignedAction
	if !expected.Valid() || action.Intent() != expected || !action.Valid(time.Now()) || action.Scope != s.identity.Scope || s.StorageIdentityVersion() != storage.IsolatedIdentity {
		return proof, ErrRejected
	}
	switch action.Kind {
	case "key.enroll", "task.claim", "task.ack", "task.handoff":
	default:
		return proof, ErrRejected
	}
	if lease == nil {
		return proof, offline.ErrWorkLease
	}
	release, err := lease(ctx)
	if err != nil {
		return proof, err
	}
	if release == nil {
		return proof, offline.ErrWorkLease
	}
	defer release()
	if _, err = s.recoverableIngress(ctx, action.Scope); err != nil {
		return proof, err
	}
	if err := s.CheckStoragePublicationTarget(action.Target); err != nil {
		return proof, err
	}
	intent := controlIntentWire{expected.Generation, expected.Scope, expected.Epoch, expected.Kind, expected.Target, expected.RequestSHA256}
	err = s.call(ctx, IngressActionSign, struct {
		Expected controlIntentWire  `json:"expected"`
		Action   matrixproof.Action `json:"action"`
	}{intent, action}, &proof)
	if err == nil && (proof.Action != action || len(proof.Signatures) == 0) {
		err = ErrRejected
	}
	if err != nil {
		return matrixproof.SignedAction{}, err
	}
	return proof, ctx.Err()
}
