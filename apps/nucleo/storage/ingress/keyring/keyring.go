// SPDX-License-Identifier: Apache-2.0
// Package keyring shares the exact ingress SSSS secret and PQ body algorithm
// between maintained native and browser SDK adapters. It performs no network,
// trust decisions, local persistence, or encryption of its own.
package keyring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

var (
	ErrRecoveryRequired = errors.New("ingress: recipient recovery is required")
	ErrUnverified       = errors.New("ingress: secret does not authenticate its key plan")
	ErrEncoding         = errors.New("ingress: could not encode its recipient secret")
)

// Secrets is the maintained SDK's SSSS raw-byte boundary. Native and browser
// adapters pass identical JSON UTF-8 bytes, without a vault/base64 wrapper.
// The owner must hold its real device lease and verified isolated account.
type Secrets interface {
	Read(context.Context, string) ([]byte, error)
	Write(context.Context, string, []byte) error
}
type Secret struct {
	Version    int           `json:"v"`
	Scope      storage.Scope `json:"scope"`
	Epoch      string        `json:"epoch"`
	Generation int64         `json:"generation"`
	KeyID      string        `json:"key_id"`
	Identity   string        `json:"identity"`
}

func SecretName(scope storage.Scope, epoch string, generation int64, keyID string) string {
	sum := sha256.Sum256([]byte(scope.Tenant + "\x00" + scope.Subject + "\x00" + scope.Product + "\x00" + epoch + "\x00" + strconv.FormatInt(generation, 10) + "\x00" + keyID))
	return "org.sytue.ingress.key.v1." + hex.EncodeToString(sum[:])
}

func Create(ctx context.Context, secrets Secrets, scope storage.Scope, epoch string, generation int64) (ingress.KeyPlan, error) {
	if secrets == nil || !scope.Valid() || !storage.MigrationEpoch(epoch) || generation < 1 || generation >= 1<<53 {
		return ingress.KeyPlan{}, ingress.ErrContract
	}
	identity, err := pqc.GenerateIdentity()
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	plan := ingress.KeyPlan{Version: 1, Scope: scope, Epoch: epoch, Generation: generation, Profile: identity.Profile(), Recipient: identity.Recipient().String()}
	plan.KeyID = ingress.PublicFingerprint(plan.Recipient)
	raw, err := json.Marshal(Secret{Version: 1, Scope: scope, Epoch: epoch, Generation: generation, KeyID: plan.KeyID, Identity: identity.String()})
	if err != nil {
		return ingress.KeyPlan{}, ErrEncoding
	}
	defer clear(raw)
	if err := secrets.Write(ctx, SecretName(scope, epoch, generation, plan.KeyID), raw); err != nil {
		return ingress.KeyPlan{}, err
	}
	checked, err := ReadIdentity(ctx, secrets, scope, epoch, generation, plan.KeyID)
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	if checked.Recipient().String() != plan.Recipient {
		return ingress.KeyPlan{}, ErrUnverified
	}
	return plan, plan.Validate(scope, epoch)
}

func ReadIdentity(ctx context.Context, secrets Secrets, scope storage.Scope, epoch string, generation int64, keyID string) (*pqc.Identity, error) {
	if secrets == nil {
		return nil, ErrUnverified
	}
	raw, err := secrets.Read(ctx, SecretName(scope, epoch, generation, keyID))
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	var secret Secret
	if len(raw) > 2048 || json.Unmarshal(raw, &secret) != nil || secret.Version != 1 || secret.Scope != scope || secret.Epoch != epoch || secret.Generation != generation || secret.KeyID != keyID {
		return nil, ErrUnverified
	}
	identity, err := pqc.ParseIdentity(secret.Identity)
	if err != nil || ingress.PublicFingerprint(identity.Recipient().String()) != keyID {
		return nil, ErrUnverified
	}
	return identity, nil
}

func RecoverTuple(ctx context.Context, secrets Secrets, scope storage.Scope, epoch string, generation int64, keyID string) (ingress.KeyPlan, error) {
	identity, err := ReadIdentity(ctx, secrets, scope, epoch, generation, keyID)
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	plan := ingress.KeyPlan{Version: 1, Scope: scope, Epoch: epoch, Generation: generation, KeyID: keyID, Profile: identity.Profile(), Recipient: identity.Recipient().String()}
	return plan, agecipher.ValidateKeyPlan(plan)
}

func Recover(ctx context.Context, secrets Secrets, expected ingress.KeyPlan) (ingress.KeyPlan, error) {
	if agecipher.ValidateKeyPlan(expected) != nil {
		return ingress.KeyPlan{}, ingress.ErrContract
	}
	actual, err := RecoverTuple(ctx, secrets, expected.Scope, expected.Epoch, expected.Generation, expected.KeyID)
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	if actual != expected {
		return ingress.KeyPlan{}, ingress.ErrContract
	}
	return actual, nil
}

func Open(ctx context.Context, secrets Secrets, task ingress.Task, source io.ReaderAt) ([]byte, *io.SectionReader, error) {
	if task.Validate(task.Scope, task.Epoch) != nil {
		return nil, nil, ingress.ErrContract
	}
	identity, err := ReadIdentity(ctx, secrets, task.Scope, task.Epoch, task.KeyGeneration, task.KeyID)
	if err != nil {
		return nil, nil, err
	}
	return agecipher.Open(ctx, source, task, identity)
}
