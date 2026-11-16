// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// BootstrapCaptureEvidence retains only PUBLIC SDK evidence captured while the
// original producer was current. It is not a VerifiedPlan or a publication hold.
type BootstrapCaptureEvidence struct {
	Version              int               `json:"v"`
	Domain               string            `json:"domain"`
	PlanSHA256           string            `json:"plan_sha256"`
	ManifestSHA256       string            `json:"manifest_sha256"`
	Plan                 StatementEvidence `json:"plan"`
	Manifest             StatementEvidence `json:"manifest"`
	SessionBindingSHA256 string            `json:"session_binding_sha256"`
}
type CapturedBootstrapRosterEvidence struct {
	value   BootstrapCaptureEvidence
	raw     []byte
	hash    string
	pin     matrixproof.Identity
	binding SessionBinding
	records recordCollector
}

func CaptureBootstrapRosterEvidence(ctx context.Context, pending VerifiedBootstrapRosterManifest) (CapturedBootstrapRosterEvidence, error) {
	if err := pending.Revalidate(ctx); err != nil {
		return CapturedBootstrapRosterEvidence{}, err
	}
	records := recordCollector{}
	plan, err := records.statement(pending.planProof, pending.current.Device())
	if err != nil {
		return CapturedBootstrapRosterEvidence{}, err
	}
	manifest, err := records.statement(pending.manifestProof, pending.current.Device())
	if err != nil {
		return CapturedBootstrapRosterEvidence{}, err
	}
	binding, err := records.add(pending.current.Binding())
	if err != nil {
		return CapturedBootstrapRosterEvidence{}, err
	}
	value := BootstrapCaptureEvidence{Version: 1, Domain: "org.sytue.resource.bootstrap.capture-evidence.v1", PlanSHA256: pending.Value().PlanSHA256, ManifestSHA256: pending.SHA256(), Plan: plan, Manifest: manifest, SessionBindingSHA256: binding}
	raw, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return CapturedBootstrapRosterEvidence{}, err
	}
	sha, err := CipherJSONDigest(raw)
	if err != nil {
		return CapturedBootstrapRosterEvidence{}, err
	}
	if err := pending.Revalidate(ctx); err != nil {
		return CapturedBootstrapRosterEvidence{}, err
	}
	return CapturedBootstrapRosterEvidence{value: value, raw: raw, hash: sha, pin: pending.current.Device().Identity(), binding: pending.current.Binding(), records: records}, nil
}

func (c CapturedBootstrapRosterEvidence) Valid() bool {
	return c.value.Version == 1 && c.value.Domain == "org.sytue.resource.bootstrap.capture-evidence.v1" && digest(c.value.PlanSHA256) && digest(c.value.ManifestSHA256) && digest(c.hash) && len(c.raw) > 0 && len(c.records) > 0 && c.binding.Valid() && identityMatches(c.pin, c.binding.Actor(), c.binding.Scope)
}

func (c CapturedBootstrapRosterEvidence) Bytes() []byte                  { return bytes.Clone(c.raw) }
func (c CapturedBootstrapRosterEvidence) SHA256() string                 { return c.hash }
func (c CapturedBootstrapRosterEvidence) OwnerPin() matrixproof.Identity { return c.pin }
func (c CapturedBootstrapRosterEvidence) SessionBinding() SessionBinding { return c.binding }
func (c CapturedBootstrapRosterEvidence) EvidenceHashes() []string {
	keys := make([]string, 0, len(c.records))
	for key := range c.records {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (c CapturedBootstrapRosterEvidence) Evidence(sha string) (EvidenceRecord, error) {
	raw, ok := c.records[sha]
	if !ok {
		return EvidenceRecord{}, ErrInvalid
	}
	return EvidenceRecord{SHA256: sha, Value: bytes.Clone(raw)}, nil
}

// RetainedBootstrapCapture is read under ORIGINAL Source owner/RLS from durable
// scalar columns, separately from its canonical descriptor. Request JSON, a
// directory query and a historical flag can never supply an independent pin.
type RetainedBootstrapCapture struct {
	State                string
	Resource             Resource
	PlanID               string
	Source               BootstrapSourceSnapshot
	Creator              Actor
	RoomID               string
	SessionBindingSHA256 string
	SessionGeneration    Decimal
	RootGeneration       Decimal
	OwnerPin             matrixproof.Identity
	PlanSHA256           string
	ManifestSHA256       string
	EvidenceSHA256       string
	Evidence             json.RawMessage
}
type TrustedBootstrapCaptureSource interface {
	PublicEvidenceSource
	RetainedBootstrapCapture(context.Context, Resource, string) (RetainedBootstrapCapture, error)
}

// HistoricalBootstrapCapture permits only metadata proof checks/cancellation.
// It exposes no live device, epoch identity, grant or VerifiedPlan constructor.
type HistoricalBootstrapCapture struct {
	retained RetainedBootstrapCapture
	plan     SignedBootstrapPlan
	manifest SignedBootstrapRosterManifest
}

func (h HistoricalBootstrapCapture) Plan() BootstrapPlan { return h.plan.BootstrapPlan }
func (h HistoricalBootstrapCapture) Manifest() BootstrapRosterManifest {
	return h.manifest.BootstrapRosterManifest
}
func (h HistoricalBootstrapCapture) ManifestSHA256() string { return h.retained.ManifestSHA256 }
func (h HistoricalBootstrapCapture) EvidenceSHA256() string { return h.retained.EvidenceSHA256 }
func RestoreBootstrapCapture(ctx context.Context, key Resource, planID string, source TrustedBootstrapCaptureSource) (HistoricalBootstrapCapture, error) {
	if source == nil || !key.Valid() || !canonicalTransaction(planID) {
		return HistoricalBootstrapCapture{}, ErrInvalid
	}
	retained, err := source.RetainedBootstrapCapture(ctx, key, planID)
	if err != nil {
		return HistoricalBootstrapCapture{}, err
	}
	if retained.State != "pending" {
		return HistoricalBootstrapCapture{}, ErrInvalid
	}
	return restoreBootstrapCaptureEvidence(ctx, key, planID, retained, source)
}

func validCaptureEvidenceDescriptor(retained RetainedBootstrapCapture) (BootstrapCaptureEvidence, error) {
	sha, err := CipherJSONDigest(retained.Evidence)
	if err != nil || sha != retained.EvidenceSHA256 {
		return BootstrapCaptureEvidence{}, ErrInvalid
	}
	var descriptor BootstrapCaptureEvidence
	if err := decodeReservationValue(retained.Evidence, &descriptor); err != nil {
		return BootstrapCaptureEvidence{}, err
	}
	if descriptor.Version != 1 || descriptor.Domain != "org.sytue.resource.bootstrap.capture-evidence.v1" || descriptor.PlanSHA256 != retained.PlanSHA256 || descriptor.ManifestSHA256 != retained.ManifestSHA256 || descriptor.SessionBindingSHA256 != retained.SessionBindingSHA256 || descriptor.Plan.Signer.Pin != retained.OwnerPin || descriptor.Manifest.Signer.Pin != retained.OwnerPin {
		return BootstrapCaptureEvidence{}, ErrInvalid
	}
	return descriptor, nil
}

func validCapturePlanAndBinding(plan SignedBootstrapPlan, binding SessionBinding, planID string, retained RetainedBootstrapCapture) bool {
	planSHA, err := BootstrapPlanSHA256(plan.BootstrapPlan)
	if err != nil || planSHA != retained.PlanSHA256 || plan.ID != planID || plan.Source != retained.Source || plan.Creator != retained.Creator || plan.RoomID != retained.RoomID || plan.SessionBindingSHA256 != retained.SessionBindingSHA256 {
		return false
	}
	return binding.Valid() && binding.Scope == plan.CreatorScope && binding.Actor() == plan.Creator && binding.Generation == retained.SessionGeneration && binding.RootGeneration == retained.RootGeneration
}

func restoreBootstrapCaptureEvidence(ctx context.Context, key Resource, planID string, retained RetainedBootstrapCapture, source PublicEvidenceSource) (HistoricalBootstrapCapture, error) {
	if retained.Resource != key || retained.PlanID != planID || !retained.Source.Valid() || retained.Source.Resource != key || !identityMatches(retained.OwnerPin, retained.Creator, retained.Source.OwnerScope) || !digest(retained.EvidenceSHA256) {
		return HistoricalBootstrapCapture{}, ErrInvalid
	}
	descriptor, err := validCaptureEvidenceDescriptor(retained)
	if err != nil {
		return HistoricalBootstrapCapture{}, err
	}
	records := reservationReader{source: source, resource: key, cache: map[string]json.RawMessage{}}
	var plan SignedBootstrapPlan
	if _, err := records.statement(ctx, descriptor.Plan, &plan, retained.Creator, retained.Source.OwnerScope, retained.RoomID); err != nil {
		return HistoricalBootstrapCapture{}, err
	}
	var manifest SignedBootstrapRosterManifest
	if _, err := records.statement(ctx, descriptor.Manifest, &manifest, retained.Creator, retained.Source.OwnerScope, retained.RoomID); err != nil {
		return HistoricalBootstrapCapture{}, err
	}
	if !manifest.ValidFor(plan.BootstrapPlan) || descriptor.Manifest.StatementSHA256 != retained.ManifestSHA256 {
		return HistoricalBootstrapCapture{}, ErrInvalid
	}
	var binding SessionBinding
	if err := records.record(ctx, descriptor.SessionBindingSHA256, &binding); err != nil {
		return HistoricalBootstrapCapture{}, err
	}
	if !validCapturePlanAndBinding(plan, binding, planID, retained) {
		return HistoricalBootstrapCapture{}, ErrInvalid
	}
	return HistoricalBootstrapCapture{retained: retained, plan: plan, manifest: manifest}, ctx.Err()
}

type BootstrapCaptureCancellation struct {
	Version              int                       `json:"v"`
	Domain               string                    `json:"domain"`
	Resource             Resource                  `json:"resource"`
	PlanID               string                    `json:"plan_id"`
	ManifestSHA256       string                    `json:"manifest_sha256"`
	Source               BootstrapSourceSnapshot   `json:"source"`
	Actor                Actor                     `json:"actor"`
	Scope                storage.Scope             `json:"scope"`
	Target               storage.PublicationTarget `json:"target"`
	SessionBindingSHA256 string                    `json:"session_binding_sha256"`
}
type SignedBootstrapCaptureCancellation struct {
	BootstrapCaptureCancellation
	Signatures signatures.Signatures `json:"signatures"`
}

func (q BootstrapCaptureCancellation) Valid() bool {
	return q.Version == 1 && q.Domain == "org.sytue.resource.bootstrap.capture.cancel.v1" && q.Resource.Valid() && canonicalTransaction(q.PlanID) && digest(q.ManifestSHA256) && q.Source.Valid() && q.Source.Resource == q.Resource && q.Scope == q.Source.OwnerScope && q.Actor.Valid() && q.Target.Valid() && q.Target.DeviceID == q.Actor.DeviceID && digest(q.SessionBindingSHA256)
}

func PrepareBootstrapCaptureCancellation(ctx context.Context, current VerifiedSession, old HistoricalBootstrapCapture) (BootstrapCaptureCancellation, error) {
	if !current.valid() || old.retained.State != "pending" || current.Scope() != old.Plan().CreatorScope || current.Actor().UserID != old.Plan().Creator.UserID || current.Actor().RootKey != old.Plan().Creator.RootKey || current.Binding().RootGeneration != old.retained.RootGeneration {
		return BootstrapCaptureCancellation{}, ErrRevoked
	}
	if err := current.Revalidate(ctx); err != nil {
		return BootstrapCaptureCancellation{}, err
	}
	sha, err := matrixproof.RequestDigest(current.Binding())
	if err != nil {
		return BootstrapCaptureCancellation{}, err
	}
	q := BootstrapCaptureCancellation{Version: 1, Domain: "org.sytue.resource.bootstrap.capture.cancel.v1", Resource: old.Plan().Source.Resource, PlanID: old.Plan().ID, ManifestSHA256: old.ManifestSHA256(), Source: old.Plan().Source, Actor: current.Actor(), Scope: current.Scope(), Target: storage.PublicationTarget{RoomID: old.Plan().RoomID, DeviceID: current.Actor().DeviceID}, SessionBindingSHA256: sha}
	if !q.Valid() {
		return BootstrapCaptureCancellation{}, ErrInvalid
	}
	return q, nil
}

type VerifiedBootstrapCaptureCancellation struct {
	value    BootstrapCaptureCancellation
	retained RetainedBootstrapCapture
	current  VerifiedSession
	original OriginalBootstrapSource
}

func VerifyBootstrapCaptureCancellation(ctx context.Context, signed SignedBootstrapCaptureCancellation, current VerifiedSession, old HistoricalBootstrapCapture, original OriginalBootstrapSource) (VerifiedBootstrapCaptureCancellation, error) {
	if original == nil || !signed.Valid() {
		return VerifiedBootstrapCaptureCancellation{}, ErrInvalid
	}
	expected, err := PrepareBootstrapCaptureCancellation(ctx, current, old)
	if err != nil {
		return VerifiedBootstrapCaptureCancellation{}, err
	}
	if signed.BootstrapCaptureCancellation != expected {
		return VerifiedBootstrapCaptureCancellation{}, ErrInvalid
	}
	if err := current.Device().VerifyJSON(ctx, current.Scope(), signed.Target, signed); err != nil {
		return VerifiedBootstrapCaptureCancellation{}, err
	}
	out := VerifiedBootstrapCaptureCancellation{value: expected, retained: old.retained, current: current, original: original}
	if err := out.Revalidate(ctx); err != nil {
		return VerifiedBootstrapCaptureCancellation{}, err
	}
	return out, nil
}

func (c VerifiedBootstrapCaptureCancellation) Value() BootstrapCaptureCancellation { return c.value }

func (c VerifiedBootstrapCaptureCancellation) OriginalEvidenceSHA256() string {
	return c.retained.EvidenceSHA256
}

func (c VerifiedBootstrapCaptureCancellation) OriginalOwnerPin() matrixproof.Identity {
	return c.retained.OwnerPin
}

func (c VerifiedBootstrapCaptureCancellation) Revalidate(ctx context.Context) error {
	if !c.value.Valid() || c.original == nil || !c.current.valid() || c.value.Actor != c.current.Actor() || c.value.Scope != c.current.Scope() || c.value.ManifestSHA256 != c.retained.ManifestSHA256 || c.value.PlanID != c.retained.PlanID {
		return ErrRevoked
	}
	if err := c.current.Revalidate(ctx); err != nil {
		return err
	}
	if err := c.original.CheckBootstrapSource(ctx, c.value.Source); err != nil {
		return err
	}
	return c.current.Revalidate(ctx)
}
