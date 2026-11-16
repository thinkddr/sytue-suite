// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

const bootstrapRecoveryKind = "org.sytue.resource.bootstrap.recovery.v1"

type bootstrapRecoverySecret struct {
	Version    int                 `json:"v"`
	Domain     string              `json:"domain"`
	PlanSHA256 string              `json:"plan_sha256"`
	Plan       SignedBootstrapPlan `json:"plan"`
	Identity   string              `json:"identity"`
}

// BootstrapRecoverySecretID addresses one immutable plan. It is public routing,
// never an enrollment, current ACL, epoch key or a replacement SSSS root.
func BootstrapRecoverySecretID(planSHA256 string) (string, error) {
	if !digest(planSHA256) {
		return "", ErrInvalid
	}
	return "resource_bootstrap_" + planSHA256, nil
}

// EncodeBootstrapRecovery prepares private bytes ONLY for an authenticated,
// product-isolated SDK vault. Never place them in public evidence or a widget.
// The verified original source and producer are checked before and after capture.
func EncodeBootstrapRecovery(ctx context.Context, plan VerifiedBootstrapPlan, identity *pqc.Identity) ([]byte, error) {
	if identity == nil || identity.Profile() != pqc.HybridProfile || identity.Recipient().String() != plan.Value().Recipient {
		return nil, ErrInvalid
	}
	if err := plan.Revalidate(ctx); err != nil {
		return nil, err
	}
	var signed SignedBootstrapPlan
	if err := decodeBootstrapRecoveryJSON(plan.PublicPlanBytes(), &signed); err != nil {
		return nil, err
	}
	value := bootstrapRecoverySecret{Version: 1, Domain: bootstrapRecoveryKind, PlanSHA256: plan.SHA256(), Plan: signed, Identity: identity.String()}
	raw, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return nil, err
	}
	if len(raw) > storage.MaxRecordBytes {
		clear(raw)
		return nil, ErrInvalid
	}
	if err := plan.Revalidate(ctx); err != nil {
		clear(raw)
		return nil, err
	}
	return raw, nil
}

func bootstrapRecoveryMatchesEpoch(secret bootstrapRecoverySecret, expectedPlanSHA256 string, ownerScope storage.Scope, epoch Epoch) bool {
	plan := secret.Plan.BootstrapPlan
	hash, err := BootstrapPlanSHA256(plan)
	return err == nil && secret.Version == 1 && secret.Domain == bootstrapRecoveryKind && secret.PlanSHA256 == expectedPlanSHA256 && hash == expectedPlanSHA256 && plan.CreatorScope == ownerScope && plan.Source.Resource == epoch.Resource && plan.RoomID == epoch.RoomID && plan.Epoch == epoch.ID && plan.Recipient == epoch.Recipient && plan.Creator == epoch.Issuer && plan.MembershipSHA256 == epoch.MembershipSHA256 && plan.History == epoch.History && plan.Validation == epoch.Validation && epoch.Generation == "1" && epoch.PreviousEpoch == "" && epoch.Baseline.Position.Seq == plan.Source.SourceSeq
}

func revalidateRestoredWindow(ctx context.Context, window EpochWindow) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if window.epoch.restored == nil {
		return nil
	}
	return window.epoch.restored.revalidate(ctx, window.epoch.restored.current)
}

// RestoreBootstrapRecovery opens only the original owner's bootstrap epoch.
// The caller must already possess an independently verified epoch window and
// obtain raw through that SAME product's authenticated private SDK vault.
// The old producer's captured SDK signature is checked without resolving it as
// live. This returns only an EpochHandle: every use still needs current access,
// exact committed evidence and the original resource fence.
func RestoreBootstrapRecovery(ctx context.Context, raw []byte, expectedPlanSHA256 string, ownerScope storage.Scope, window EpochWindow) (*EpochHandle, error) {
	if !digest(expectedPlanSHA256) || !ownerScope.Valid() || !window.epoch.valid() || len(raw) == 0 || len(raw) > storage.MaxRecordBytes {
		return nil, ErrInvalid
	}
	if err := revalidateRestoredWindow(ctx, window); err != nil {
		return nil, err
	}
	var secret bootstrapRecoverySecret
	if err := decodeBootstrapRecoveryJSON(raw, &secret); err != nil {
		return nil, err
	}
	canonical, err := matrixproof.CanonicalBytes(secret)
	if err != nil {
		return nil, err
	}
	defer clear(canonical)
	if !bytes.Equal(canonical, raw) {
		return nil, ErrInvalid
	}
	plan := secret.Plan.BootstrapPlan
	epoch := window.epoch.value
	if !bootstrapRecoveryMatchesEpoch(secret, expectedPlanSHA256, ownerScope, epoch) {
		return nil, ErrInvalid
	}
	if err := window.epoch.device.VerifyJSON(ctx, ownerScope, storage.PublicationTarget{RoomID: plan.RoomID, DeviceID: plan.Creator.DeviceID}, secret.Plan); err != nil {
		return nil, err
	}
	baseline, err := NewBootstrapContext(BootstrapBaseline, plan.Source.Resource, plan.RoomID, plan.Epoch, plan.Recipient, Position{Seq: plan.Source.SourceSeq}, plan.CreatorScope, plan.Creator)
	if err != nil {
		return nil, err
	}
	baselineHash, err := BootstrapContextSHA256(baseline)
	if err != nil || baselineHash != epoch.Baseline.ContextSHA256 {
		return nil, ErrInvalid
	}
	identity, err := pqc.ParseIdentity(secret.Identity)
	if err != nil || identity.Profile() != pqc.HybridProfile || identity.Recipient().String() != plan.Recipient {
		return nil, ErrInvalid
	}
	if err := revalidateRestoredWindow(ctx, window); err != nil {
		return nil, err
	}
	return BindOwnedEpoch(window, identity)
}

func decodeBootstrapRecoveryJSON(raw []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return ErrInvalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	return nil
}
