// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// A batch root exists before ANY SDK publication. These encrypted journal rows
// are replay data only, never SDK receipts, signer grants or ObjectVersions.
// Content references are fixed separately after exact maintained-SDK readback.
type batchPublication struct {
	Version          int                       `json:"v"`
	Scope            storage.Scope             `json:"scope"`
	Operation        string                    `json:"operation"`
	PlanID           string                    `json:"plan_id"`
	InputFingerprint string                    `json:"input_fingerprint"`
	MemberSHA256     string                    `json:"member_sha256"`
	Count            uint64                    `json:"count"`
	Target           storage.PublicationTarget `json:"target"`
	BatchID          string                    `json:"batch_id"`
	State            string                    `json:"state"`
	Captured         bool                      `json:"captured"`
	Root             *storage.Reference        `json:"root,omitempty"`
}

type batchInputIntent struct {
	Version      int           `json:"v"`
	Scope        storage.Scope `json:"scope"`
	PlanID       string        `json:"plan_id"`
	BatchID      string        `json:"batch_id"`
	MemberSHA256 string        `json:"member_sha256"`
	Ordinal      uint64        `json:"ordinal"`
	InputSHA256  string        `json:"input_sha256"`
}

func inputBatchKey(operation string) string { return "batch_" + atomicOperationKey(operation) }
func inputBatchMemberKey(operation string, ordinal uint64) string {
	return inputBatchKey(operation) + "_input_" + strconv.FormatUint(ordinal, 10)
}

func capturedInputStreamSHA(ctx context.Context, data *capturedInputPlan) (string, error) {
	digest := sha256.New()
	encoder := json.NewEncoder(digest)
	for ordinal := uint64(0); ordinal < data.root.Count; ordinal++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		value, found := data.members[ordinal]
		if !found || !atomicValidHash(value) {
			return "", storage.ErrRecord
		}
		if err := encoder.Encode(struct {
			Ordinal uint64 `json:"ordinal"`
			SHA256  string `json:"sha256"`
		}{ordinal, value}); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (root batchPublication) matches(data *capturedInputPlan, memberSHA string) bool {
	return root.Version == 1 && root.Scope == data.root.Scope && root.Operation == data.root.Operation && root.PlanID == data.root.ID && root.InputFingerprint == data.root.Fingerprint && root.Count == data.root.Count && root.Target == data.root.Target && root.MemberSHA256 == memberSHA && storage.Identifier(root.BatchID) && (root.State == StateStaging || root.State == "accepted" || root.State == "finalizing") && (root.State == StateStaging || root.Captured && root.Root != nil) && (root.Root == nil || root.Root.Valid() && root.Root.RoomID == root.Target.RoomID)
}

func (root batchPublication) intent(ordinal uint64, inputSHA string) batchInputIntent {
	return batchInputIntent{Version: 1, Scope: root.Scope, PlanID: root.PlanID, BatchID: root.BatchID, MemberSHA256: root.MemberSHA256, Ordinal: ordinal, InputSHA256: inputSHA}
}

// captureBatchPublication retains ALL exact InputMember hashes before any
// network/Prepare/Put. A captured missing intent is never silently repaired.
// Original bytes, targets, base references and explicit operation ID already
// live in the encrypted InputPlan. A different device cannot retarget this root.
func captureBatchPublication(ctx context.Context, plan InputPlan) (batchPublication, error) {
	data, err := plan.current(ctx)
	if err != nil {
		return batchPublication{}, err
	}
	memberSHA, err := capturedInputStreamSHA(ctx, data)
	if err != nil {
		return batchPublication{}, err
	}
	owner := atomicOwner(data.root.Scope)
	namespace := storage.AtomicJournalNamespace(data.root.Scope.Product)
	var result batchPublication
	err = data.journal.Exclusive(ctx, owner, namespace, inputBatchKey(data.root.Operation), func() error {
		if _, err := plan.current(ctx); err != nil {
			return err
		}
		root, err := loadOrInitBatchPublication(data, owner, namespace, memberSHA)
		if err != nil {
			return err
		}
		if err := captureBatchMemberIntents(ctx, plan, data, owner, namespace, root); err != nil {
			return err
		}
		root.Captured = true
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := data.journal.Write(owner, namespace, inputBatchKey(root.Operation), root); err != nil {
			return err
		}
		result = root
		return nil
	})
	if err != nil {
		return batchPublication{}, err
	}
	return result, nil
}

func loadOrInitBatchPublication(data *capturedInputPlan, owner, namespace, memberSHA string) (batchPublication, error) {
	var root batchPublication
	found, err := data.journal.Read(owner, namespace, inputBatchKey(data.root.Operation), &root)
	if err != nil {
		return batchPublication{}, err
	}
	if !found {
		batch := rand.Text()
		var work Work
		exists, err := data.journal.Read(owner, namespace, atomicOperationKey(inputIndexOperation(data.root.ID)), &work)
		if err != nil {
			return batchPublication{}, err
		}
		if exists {
			if !work.valid(data.root.Scope, inputIndexOperation(data.root.ID)) || work.Target != data.root.Target || work.Count != data.root.Count {
				return batchPublication{}, storage.ErrRecord
			}
			batch = work.BatchID
		}
		root = batchPublication{Version: 1, Scope: data.root.Scope, Operation: data.root.Operation, PlanID: data.root.ID, InputFingerprint: data.root.Fingerprint, MemberSHA256: memberSHA, Count: data.root.Count, Target: data.root.Target, BatchID: batch, State: StateStaging}
		if err := data.journal.Write(owner, namespace, inputBatchKey(data.root.Operation), root); err != nil {
			return batchPublication{}, err
		}
	}
	if !root.matches(data, memberSHA) {
		return batchPublication{}, storage.ErrRecord
	}
	return root, nil
}

func captureBatchMemberIntents(ctx context.Context, plan InputPlan, data *capturedInputPlan, owner, namespace string, root batchPublication) error {
	for ordinal := uint64(0); ordinal < root.Count; ordinal++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, _, err := plan.readCapturedMember(ctx, ordinal); err != nil {
			return err
		}
		expected := root.intent(ordinal, data.members[ordinal])
		var actual batchInputIntent
		exists, err := data.journal.Read(owner, namespace, inputBatchMemberKey(root.Operation, ordinal), &actual)
		if err != nil {
			return err
		}
		if !exists {
			if root.Captured {
				return storage.ErrRecord
			}
			if err := data.journal.Write(owner, namespace, inputBatchMemberKey(root.Operation, ordinal), expected); err != nil {
				return err
			}
		} else if actual != expected {
			return storage.ErrRecord
		}
	}
	return nil
}
