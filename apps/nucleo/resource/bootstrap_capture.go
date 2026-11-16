// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// BootstrapPlanEvidence retains the ORIGINAL public SDK chains and confirmed product
// binding before any network publication. A revoked producer's ACK can later be
// verified historically; these bytes never manufacture a new live SDK device.
type BootstrapPlanEvidence struct {
	Version              int                 `json:"v"`
	Domain               string              `json:"domain"`
	PlanSHA256           string              `json:"plan_sha256"`
	Plan                 StatementEvidence   `json:"plan"`
	MembersSHA256        string              `json:"members_sha256"`
	Recipients           []StatementEvidence `json:"recipients"`
	SessionBindingSHA256 string              `json:"session_binding_sha256"`
}
type CapturedBootstrapPlan struct {
	plan                  BootstrapPlan
	raw, signed, evidence json.RawMessage
	hash, evidenceHash    string
	records               []EvidenceRecord
}

func (c CapturedBootstrapPlan) Value() BootstrapPlan   { return c.plan }
func (c CapturedBootstrapPlan) Bytes() []byte          { return bytes.Clone(c.raw) }
func (c CapturedBootstrapPlan) SignedBytes() []byte    { return bytes.Clone(c.signed) }
func (c CapturedBootstrapPlan) EvidenceBytes() []byte  { return bytes.Clone(c.evidence) }
func (c CapturedBootstrapPlan) SHA256() string         { return c.hash }
func (c CapturedBootstrapPlan) EvidenceSHA256() string { return c.evidenceHash }
func (c CapturedBootstrapPlan) Records() []EvidenceRecord {
	result := make([]EvidenceRecord, len(c.records))
	for i, record := range c.records {
		result[i] = EvidenceRecord{SHA256: record.SHA256, Value: bytes.Clone(record.Value)}
	}
	return result
}

func (c CapturedBootstrapPlan) Valid() bool {
	return c.plan.Valid() && digest(c.hash) && digest(c.evidenceHash) && len(c.raw) > 0 && len(c.signed) > 0 && len(c.evidence) > 0
}

func CaptureBootstrapPlan(ctx context.Context, plan VerifiedBootstrapPlan) (CapturedBootstrapPlan, error) {
	if err := plan.Revalidate(ctx); err != nil {
		return CapturedBootstrapPlan{}, err
	}
	records := recordCollector{}
	statement, err := records.statement(plan.proof, plan.session.Device())
	if err != nil {
		return CapturedBootstrapPlan{}, err
	}
	descriptor := BootstrapPlanEvidence{Version: 1, Domain: "org.sytue.resource.bootstrap.evidence.v1", PlanSHA256: plan.hash, Plan: statement, MembersSHA256: plan.roster.recordHash}
	records[plan.roster.recordHash] = plan.roster.record
	keys := make([]string, 0, len(plan.roster.recipients))
	for key := range plan.roster.recipients {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		recipient, err := records.recipient(plan.roster.recipients[key])
		if err != nil {
			return CapturedBootstrapPlan{}, err
		}
		descriptor.Recipients = append(descriptor.Recipients, recipient)
	}
	descriptor.SessionBindingSHA256, err = records.add(plan.session.Binding())
	if err != nil {
		return CapturedBootstrapPlan{}, err
	}
	raw, err := matrixproof.CanonicalBytes(plan.value)
	if err != nil {
		return CapturedBootstrapPlan{}, err
	}
	evidence, err := matrixproof.CanonicalBytes(descriptor)
	if err != nil {
		return CapturedBootstrapPlan{}, err
	}
	evidenceHash, err := CipherJSONDigest(evidence)
	if err != nil {
		return CapturedBootstrapPlan{}, err
	}
	list := make([]EvidenceRecord, 0, len(records))
	for hash, value := range records {
		list = append(list, EvidenceRecord{SHA256: hash, Value: value})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].SHA256 < list[j].SHA256 })
	if err := plan.Revalidate(ctx); err != nil {
		return CapturedBootstrapPlan{}, err
	}
	return CapturedBootstrapPlan{plan: plan.value, raw: raw, signed: bytes.Clone(plan.proof), evidence: evidence, hash: plan.hash, evidenceHash: evidenceHash, records: list}, nil
}

func (c CapturedBootstrapPlan) EvidenceHashes() []string { return evidenceHashes(c.records) }
func (c CapturedBootstrapPlan) Evidence(hash string) (EvidenceRecord, error) {
	return evidenceRecord(c.records, hash)
}
