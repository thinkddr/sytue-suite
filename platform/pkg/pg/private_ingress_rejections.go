// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"crypto/sha256"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func readPrivateIngressRejectedHandoff(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, request ingress.ControlRequest) (privateIngressHandoff, error) {
	var raw []byte
	var taskID string
	err := tx.QueryRow(ctx, `SELECT task_id,rejection FROM private_ingress_handoff_rejections WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, request.Handoff.ID).Scan(&taskID, &raw)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	if request.Handoff.Phase != "begin" || taskID != request.Handoff.TaskID {
		return privateIngressHandoff{}, ingress.ErrClaim
	}
	var rejection ingress.HandoffRejection
	if json.Unmarshal(raw, &rejection) != nil || rejection.Validate(request, rejection.Authority) != nil {
		return privateIngressHandoff{}, ingress.ErrRevoked
	}
	stored, err := readPrivateIngressDelivery(ctx, tx, cfg, taskID, false)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	return rejectedPrivateIngressHandoff(stored.Task, request, rejection), nil
}

func rejectedPrivateIngressHandoff(task ingress.Task, request ingress.ControlRequest, rejection ingress.HandoffRejection) privateIngressHandoff {
	tokenHash := sha256.Sum256([]byte(request.Handoff.Token))
	return privateIngressHandoff{view: ingress.HandoffView{Version: 1, ID: request.Handoff.ID, Task: task, Previous: request.Handoff.Previous, Target: request.Target, Status: "not_started", Rejection: &rejection}, authority: rejection.Authority, digest: rejection.RequestSHA256, tokenHash: tokenHash[:]}
}

// The caller holds this task's row lock. A refusal and the nonce consumption
// commit together before any response can authorize a fresh local inspection.
func rejectPrivateIngressHandoff(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, task ingress.Task, request ingress.ControlRequest, authority matrixproof.Authority) (privateIngressHandoff, error) {
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	rejection := ingress.HandoffRejection{Version: 1, ID: request.Handoff.ID, RequestSHA256: digest, Authority: authority, Reason: "assignment_changed"}
	if err := rejection.Validate(request, authority); err != nil {
		return privateIngressHandoff{}, err
	}
	raw, err := json.Marshal(rejection)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO private_ingress_handoff_rejections(tenant,owner_sub,product,epoch,id,task_id,rejection) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb)`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, request.Handoff.ID, task.ID, raw)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	return rejectedPrivateIngressHandoff(task, request, rejection), nil
}
