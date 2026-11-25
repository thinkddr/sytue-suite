// SPDX-License-Identifier: Apache-2.0
package accion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"maunium.net/go/mautrix/crypto/canonicaljson"
	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Action commits to an entire typed queue request. The authority generates the
// nonce/times and retains this exact expected transcript. Private message data
// is never part of the request: it belongs inside the age-encrypted body.
type Action struct {
	Version       int                       `json:"v"`
	Generation    int64                     `json:"authority_generation"`
	Scope         storage.Scope             `json:"scope"`
	Epoch         string                    `json:"epoch"`
	Kind          string                    `json:"kind"`
	Target        storage.PublicationTarget `json:"target"`
	RootKey       string                    `json:"root_key"`
	RequestSHA256 string                    `json:"request_sha256"`
	Nonce         string                    `json:"nonce"`
	IssuedAt      int64                     `json:"issued_at"`
	ExpiresAt     int64                     `json:"expires_at"`
}

// Intent is fixed by the client before requesting a server challenge. It binds
// purpose and the entire locally selected request; signing a server-selected
// digest without comparing this intent would authorize a substituted recipient.
type Intent struct {
	Generation    int64
	Scope         storage.Scope
	Epoch         string
	Kind          string
	Target        storage.PublicationTarget
	RequestSHA256 string
}

func IntentFor(scope storage.Scope, generation int64, epoch, kind string, target storage.PublicationTarget, request any) (Intent, error) {
	digest, err := RequestDigest(request)
	if err != nil {
		return Intent{}, err
	}
	intent := Intent{Generation: generation, Scope: scope, Epoch: epoch, Kind: kind, Target: target, RequestSHA256: digest}
	if !intent.Valid() {
		return Intent{}, ErrUnverified
	}
	return intent, nil
}

func (i Intent) Valid() bool {
	if i.Generation < 1 || i.Generation >= 1<<53 || !i.Scope.Valid() || !storage.MigrationEpoch(i.Epoch) || !i.Target.Valid() {
		return false
	}
	if (storage.StreamSourceRow{Kind: "request", ID: "request", Size: 0, SHA256: i.RequestSHA256}).Validate() != nil {
		return false
	}
	switch i.Kind {
	case "root.enroll", "root.rotate", "root.resume", "key.enroll", "task.claim", "task.ack", "task.handoff":
		return true
	default:
		return false
	}
}

func (a Action) Intent() Intent {
	return Intent{Generation: a.Generation, Scope: a.Scope, Epoch: a.Epoch, Kind: a.Kind, Target: a.Target, RequestSHA256: a.RequestSHA256}
}

type SignedAction struct {
	Action
	Signatures signatures.Signatures `json:"signatures"`
}

// CanonicalBytes preserves encoding/json wire semantics before the maintained
// SDK canonical JSON serializer. Native, Wasm and server sign the same bytes.
func CanonicalBytes(value any) ([]byte, error) {
	// The public API uses encoding/json's wire contract. Canonicalizing a Go
	// struct directly with the SDK's Go 1.27 JSON v2 changes omitempty semantics
	// and can hash fields that the browser never received. Preserve the actual
	// serialized value, then delegate canonicalization to the maintained SDK.
	wire, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	defer clear(wire)
	return canonicaljson.Marshal(json.RawMessage(wire))
}

func RequestDigest(value any) (string, error) {
	raw, err := CanonicalBytes(value)
	if err != nil {
		return "", err
	}
	defer clear(raw)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (a Action) Valid(now time.Time) bool {
	if a.Version != 1 || !a.Intent().Valid() || !ValidKey(a.RootKey) || !storage.Identifier(a.Nonce) {
		return false
	}
	return a.IssuedAt > 0 && a.ExpiresAt > a.IssuedAt && a.ExpiresAt-a.IssuedAt <= int64((5*time.Minute)/time.Millisecond) && a.ExpiresAt > now.UnixMilli() && a.IssuedAt <= now.Add(30*time.Second).UnixMilli()
}
