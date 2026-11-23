// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	contract "git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type HTTPControl interface {
	Challenge(context.Context, contract.ControlRequest, matrixproof.Authority) (matrixproof.Action, error)
	Apply(context.Context, contract.ControlProof) (contract.ControlResult, error)
	Pending(context.Context, storage.Scope, string, string, int) (contract.PendingPage, error)
	Revalidate(context.Context, contract.ControlProof) (matrixproof.Authorization, error)
	Open(context.Context, contract.ControlProof) (storage.StreamSourceRow, io.ReadCloser, error)
}
type (
	HTTPAuthority interface {
		MatrixStorageAuthority(context.Context, storage.Scope) (matrixproof.Authority, error)
	}
	HTTPActionSigner interface {
		SignIngressAction(context.Context, offline.WorkLease, matrixproof.Intent, matrixproof.Action) (matrixproof.SignedAction, error)
	}
	HTTPSourceConfig struct {
		Scope     storage.Scope
		Epoch     string
		Control   HTTPControl
		Authority HTTPAuthority
		Signer    HTTPActionSigner
		Lease     offline.WorkLease
	}
)

type (
	claimProofKey struct {
		ID, Token  string
		Target     storage.PublicationTarget
		Generation int64
	}
	HTTPSource struct {
		cfg    HTTPSourceConfig
		mu     sync.Mutex
		proofs map[claimProofKey]contract.ControlProof
	}
)

func NewHTTPSource(cfg HTTPSourceConfig) (*HTTPSource, error) {
	if !cfg.Scope.Valid() || !contract.Product(cfg.Scope.Product) || !storage.MigrationEpoch(cfg.Epoch) || cfg.Control == nil || cfg.Authority == nil || cfg.Signer == nil || cfg.Lease == nil {
		return nil, contract.ErrContract
	}
	return &HTTPSource{cfg: cfg, proofs: make(map[claimProofKey]contract.ControlProof)}, nil
}

func (s *HTTPSource) Authority(ctx context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	if scope != s.cfg.Scope {
		return matrixproof.Authority{}, contract.ErrContract
	}
	result, err := s.cfg.Authority.MatrixStorageAuthority(ctx, scope)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	if result.RequireActive(scope, result.Generation) != nil {
		return matrixproof.Authority{}, contract.ErrRevoked
	}
	return result, nil
}

func (s *HTTPSource) sign(ctx context.Context, request contract.ControlRequest, authority matrixproof.Authority) (contract.ControlProof, error) {
	if request.Validate(s.cfg.Scope, s.cfg.Epoch) != nil || authority.RequireActive(s.cfg.Scope, authority.Generation) != nil {
		return contract.ControlProof{}, contract.ErrRevoked
	}
	intent, err := matrixproof.IntentFor(s.cfg.Scope, authority.Generation, s.cfg.Epoch, request.Kind, request.Target, request)
	if err != nil {
		return contract.ControlProof{}, err
	}
	action, err := s.cfg.Control.Challenge(ctx, request, authority)
	if err != nil {
		return contract.ControlProof{}, ingressHTTPError(err)
	}
	if action.Intent() != intent || action.RootKey != authority.RootKey {
		return contract.ControlProof{}, contract.ErrRevoked
	}
	proof, err := s.cfg.Signer.SignIngressAction(ctx, s.cfg.Lease, intent, action)
	if err != nil {
		return contract.ControlProof{}, err
	}
	if proof.Action != action {
		return contract.ControlProof{}, contract.ErrContract
	}
	result := contract.ControlProof{Request: request, Proof: proof}
	if err := result.Validate(s.cfg.Scope, false); err != nil {
		return contract.ControlProof{}, err
	}
	return result, nil
}

func (s *HTTPSource) Enroll(ctx context.Context, plan contract.KeyPlan, target storage.PublicationTarget, authority matrixproof.Authority) error {
	if plan.Validate(s.cfg.Scope, s.cfg.Epoch) != nil {
		return contract.ErrContract
	}
	request := contract.ControlRequest{Version: 1, Scope: s.cfg.Scope, Epoch: s.cfg.Epoch, Kind: "key.enroll", Target: target, Enrollment: &plan}
	proof, err := s.sign(ctx, request, authority)
	if err != nil {
		return err
	}
	result, err := s.cfg.Control.Apply(ctx, proof)
	if err != nil {
		return ingressHTTPError(err)
	}
	return validateHTTPControlResult(result, proof, authority)
}

func (s *HTTPSource) Pending(ctx context.Context, after string, limit int) (contract.PendingPage, error) {
	return s.cfg.Control.Pending(ctx, s.cfg.Scope, s.cfg.Epoch, after, limit)
}

func (s *HTTPSource) Claim(ctx context.Context, scope storage.Scope, epoch, id string, target storage.PublicationTarget, token string, authority matrixproof.Authority) (contract.Claim, error) {
	if scope != s.cfg.Scope || epoch != s.cfg.Epoch {
		return contract.Claim{}, contract.ErrContract
	}
	request := s.claimRequest(id, target, token)
	proof, err := s.sign(ctx, request, authority)
	if err != nil {
		return contract.Claim{}, err
	}
	result, err := s.cfg.Control.Apply(ctx, proof)
	if err != nil {
		return contract.Claim{}, ingressHTTPError(err)
	}
	if validateHTTPControlResult(result, proof, authority) != nil || result.Claim == nil {
		return contract.Claim{}, contract.ErrClaim
	}
	claim := *result.Claim
	s.mu.Lock()
	s.proofs[proofKey(claim)] = proof
	s.mu.Unlock()
	return claim, nil
}

func (s *HTTPSource) claimRequest(id string, target storage.PublicationTarget, token string) contract.ControlRequest {
	return contract.ControlRequest{Version: 1, Scope: s.cfg.Scope, Epoch: s.cfg.Epoch, Kind: "task.claim", Target: target, Claim: &contract.ClaimRequest{TaskID: id, Token: token, LeaseSeconds: 300}}
}

func proofKey(claim contract.Claim) claimProofKey {
	return claimProofKey{ID: claim.Task.ID, Token: claim.Token, Target: claim.Target, Generation: claim.Generation}
}

func (s *HTTPSource) proofFor(ctx context.Context, claim contract.Claim, authority matrixproof.Authority) (contract.ControlProof, error) {
	if claim.Validate(s.cfg.Scope, s.cfg.Epoch) != nil || authority.RequireActive(s.cfg.Scope, authority.Generation) != nil {
		return contract.ControlProof{}, contract.ErrClaim
	}
	key := proofKey(claim)
	s.mu.Lock()
	proof, ok := s.proofs[key]
	s.mu.Unlock()
	if !ok || proof.Proof.RootKey != authority.RootKey || proof.Proof.Generation != authority.Generation || !proof.Proof.Valid(time.Now().Add(15*time.Second)) {
		var err error
		proof, err = s.sign(ctx, s.claimRequest(claim.Task.ID, claim.Target, claim.Token), authority)
		if err != nil {
			return contract.ControlProof{}, err
		}
		s.mu.Lock()
		s.proofs[key] = proof
		s.mu.Unlock()
	}
	proof.Claim = &claim
	if err := proof.Validate(s.cfg.Scope, true); err != nil {
		return contract.ControlProof{}, err
	}
	return proof, nil
}

func (s *HTTPSource) Revalidate(ctx context.Context, claim contract.Claim, authority matrixproof.Authority) error {
	proof, err := s.proofFor(ctx, claim, authority)
	if err != nil {
		return err
	}
	result, err := s.cfg.Control.Revalidate(ctx, proof)
	if err != nil {
		return ingressHTTPError(err)
	}
	if result.Validate(proof.Proof.Action) != nil || result.Authority != authority {
		return contract.ErrRevoked
	}
	return nil
}

func (s *HTTPSource) Open(ctx context.Context, claim contract.Claim, authority matrixproof.Authority) (storage.StreamSourceRow, io.ReadCloser, error) {
	proof, err := s.proofFor(ctx, claim, authority)
	if err != nil {
		return storage.StreamSourceRow{}, nil, err
	}
	row, body, err := s.cfg.Control.Open(ctx, proof)
	if err != nil {
		return row, body, ingressHTTPError(err)
	}
	if row != claim.Task.Cipher {
		if body != nil {
			_ = body.Close()
		}
		return storage.StreamSourceRow{}, nil, contract.ErrContract
	}
	return row, body, nil
}

func (s *HTTPSource) Acknowledge(ctx context.Context, claim contract.Claim, receipt contract.Receipt, authority matrixproof.Authority) (contract.Receipt, error) {
	if receipt.Validate(claim) != nil {
		return contract.Receipt{}, contract.ErrReceipt
	}
	request := contract.ControlRequest{Version: 1, Scope: s.cfg.Scope, Epoch: s.cfg.Epoch, Kind: "task.ack", Target: claim.Target, Acknowledgment: &contract.AcknowledgmentRequest{Claim: claim, Receipt: receipt}}
	proof, err := s.sign(ctx, request, authority)
	if err != nil {
		return contract.Receipt{}, err
	}
	result, err := s.cfg.Control.Apply(ctx, proof)
	if err != nil {
		return contract.Receipt{}, ingressHTTPError(err)
	}
	if validateHTTPControlResult(result, proof, authority) != nil || result.Receipt == nil || *result.Receipt != receipt {
		return contract.Receipt{}, contract.ErrReceipt
	}
	s.mu.Lock()
	delete(s.proofs, proofKey(claim))
	s.mu.Unlock()
	return receipt, nil
}

func ingressHTTPError(err error) error {
	var failure *api.Error
	if !errors.As(err, &failure) {
		return err
	}
	switch failure.Code() {
	case "private_ingress_authorization_changed":
		return errors.Join(contract.ErrRevoked, err)
	case "private_ingress_claim_changed":
		return errors.Join(contract.ErrClaim, err)
	case "private_ingress_receipt_required":
		return errors.Join(contract.ErrReceipt, err)
	default:
		return err
	}
}

var _ Source = (*HTTPSource)(nil)

func validateHTTPControlResult(result contract.ControlResult, proof contract.ControlProof, authority matrixproof.Authority) error {
	if err := result.Validate(proof.Request, proof.Proof.Action); err != nil {
		return err
	}
	if result.Authorization.Authority != authority {
		return contract.ErrRevoked
	}
	return nil
}
