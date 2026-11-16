// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func (r *Rooms) MatrixStorageAuthority(ctx context.Context, scope storage.Scope) (matrixproof.Authority, error) {
	request, err := privateAppRequest(scope, scope.Product, "/matrix/storage/"+scope.Product+"/authority")
	if err != nil {
		return matrixproof.Authority{}, err
	}
	authority, err := JSON[matrixproof.Authority](ctx, r.c, request)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	if err := authority.Validate(scope); err != nil {
		return matrixproof.Authority{}, err
	}
	return authority, nil
}

// MatrixStorageRootChallenge binds a user-approved, locally constructed root
// request before HTTP. The returned challenge cannot choose a new destination,
// root, generation or request digest on the caller's behalf.
func (r *Rooms) MatrixStorageRootChallenge(ctx context.Context, scope storage.Scope, epoch, kind string, expected matrixproof.RootEnrollment) (matrixproof.RootChallenge, error) {
	if expected.Scope != scope || !scope.Valid() || !storage.MigrationEpoch(epoch) || !expected.Target.Valid() {
		return matrixproof.RootChallenge{}, matrixproof.ErrUnverified
	}
	generation := expected.Generation
	if kind == "root.resume" || kind == "root.rotate" {
		generation = expected.PreviousGeneration
	} else if kind != "root.enroll" || generation != 1 || expected.PreviousGeneration != 0 || expected.PreviousRootKey != "" {
		return matrixproof.RootChallenge{}, matrixproof.ErrUnverified
	}
	intent, err := matrixproof.IntentFor(scope, generation, epoch, kind, expected.Target, expected)
	if err != nil {
		return matrixproof.RootChallenge{}, err
	}
	request, err := privateAppRequest(scope, scope.Product, "/matrix/storage/"+scope.Product+"/authority/challenge")
	if err != nil {
		return matrixproof.RootChallenge{}, err
	}
	request.Method = http.MethodPost
	request.Header.Set("Content-Type", "application/json")
	request.Body, _ = json.Marshal(struct {
		Kind   string                    `json:"kind"`
		Epoch  string                    `json:"epoch"`
		Target storage.PublicationTarget `json:"target"`
	}{kind, epoch, expected.Target})
	challenge, err := JSON[matrixproof.RootChallenge](ctx, r.c, request)
	if err != nil {
		return matrixproof.RootChallenge{}, err
	}
	if challenge.Request != expected || challenge.Action.Intent() != intent || challenge.Action.RootKey != expected.RootKey || !challenge.Action.Valid(time.Now()) {
		return matrixproof.RootChallenge{}, matrixproof.ErrUnverified
	}
	return challenge, nil
}

func (r *Rooms) ConfirmMatrixStorageRoot(ctx context.Context, scope storage.Scope, proof matrixproof.SignedAction) (matrixproof.Authority, error) {
	if proof.Scope != scope || !proof.Valid(time.Now()) || (proof.Kind != "root.enroll" && proof.Kind != "root.resume" && proof.Kind != "root.rotate") {
		return matrixproof.Authority{}, matrixproof.ErrUnverified
	}
	request, err := privateAppRequest(scope, scope.Product, "/matrix/storage/"+scope.Product+"/authority/confirm")
	if err != nil {
		return matrixproof.Authority{}, err
	}
	request.Method = http.MethodPost
	request.Header.Set("Content-Type", "application/json")
	request.Body, _ = json.Marshal(proof)
	authority, err := JSON[matrixproof.Authority](ctx, r.c, request)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	generation := proof.Generation
	if proof.Kind != "root.enroll" {
		generation++
	}
	if authority.RequireActive(scope, generation) != nil || authority.RootKey != proof.RootKey {
		return matrixproof.Authority{}, matrixproof.ErrUnverified
	}
	return authority, nil
}
