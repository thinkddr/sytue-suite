// SPDX-License-Identifier: Apache-2.0
// Package ingress describes opaque durable input for authorized application
// devices. It has no UI, session, database, local journal or private cryptographic keys.
package ingress

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

const (
	// Profile is retained only for existing classical enrollments.
	Profile          = pqc.LegacyProfile
	HybridProfile    = pqc.HybridProfile
	ReceiptRole      = "ingress_receipt"
	MaxMetadataBytes = storage.MaxRecordBytes / 2
)

var (
	ErrContract = errors.New("ingress: invalid or changed delivery binding")
	ErrRevoked  = errors.New("ingress: device authorization was revoked")
	ErrClaim    = errors.New("ingress: delivery requires its original claim or a verified device fence")
	ErrReceipt  = errors.New("ingress: authenticated application commit is required")
)

// KeyPlan is the authenticated public enrollment retained by the server. The
// recipient identity's private key is exclusively held/recovered by devices.
type KeyPlan struct {
	Version    int           `json:"v"`
	Scope      storage.Scope `json:"scope"`
	Epoch      string        `json:"epoch"`
	Generation int64         `json:"generation"`
	KeyID      string        `json:"key_id"`
	Profile    string        `json:"profile"`
	Recipient  string        `json:"recipient"`
}

func PublicFingerprint(recipient string) string {
	sum := sha256.Sum256([]byte(recipient))
	return hex.EncodeToString(sum[:])
}

func digestValid(value string) bool {
	return (storage.StreamSourceRow{Kind: "key", ID: "fingerprint", Size: 0, SHA256: value}).Validate() == nil
}

func (p KeyPlan) Validate(scope storage.Scope, epoch string) error {
	if p.Version != 1 || p.Scope != scope || !scope.Valid() || p.Epoch != epoch || !storage.MigrationEpoch(epoch) || p.Generation < 1 || p.Generation >= 1<<53 || !pqc.ValidProfile(p.Profile) || p.Profile == pqc.LegacyProfile && len(p.Recipient) > 256 || len(p.Recipient) < 1 || len(p.Recipient) > pqc.MaxRecipientBytes || !digestValid(p.KeyID) || PublicFingerprint(p.Recipient) != p.KeyID {
		return ErrContract
	}
	return nil
}

// Task contains routing and ciphertext commitments only. Mail addresses,
// subjects, original headers, calendar data and plaintext digests belong inside
// its encrypted body. ID is allocated durably before the first seal attempt.
type Task struct {
	Version       int                     `json:"v"`
	Scope         storage.Scope           `json:"scope"`
	Epoch         string                  `json:"epoch"`
	ID            string                  `json:"id"`
	Kind          string                  `json:"kind"`
	KeyID         string                  `json:"key_id"`
	KeyGeneration int64                   `json:"key_generation"`
	Cipher        storage.StreamSourceRow `json:"cipher"`
}

// Product dice si un producto recibe por la entrada privada: Mail, Calendar y los
// avisos de One (hub, ADR 0086). Es la única lista; los demás paquetes la consultan.
func Product(product string) bool {
	return product == "mail" || product == "calendar" || product == "hub"
}

func kindProduct(kind string) string {
	switch kind {
	case "mail.receive":
		return "mail"
	case "calendar.receive":
		return "calendar"
	case "hub.receive":
		// Un aviso que un servicio deja a una persona cuya fuente de One ya no es
		// legacy (ADR 0086): el aviso entero va cifrado en los metadatos.
		return "hub"
	default:
		return ""
	}
}

func (t Task) Validate(scope storage.Scope, epoch string) error {
	if t.ValidateBinding(scope, epoch) != nil || t.Cipher.Kind != "ingress" || t.Cipher.ID != t.ID || t.Cipher.Size < 1 || t.Cipher.Validate() != nil {
		return ErrContract
	}
	return nil
}

func (t Task) ValidateBinding(scope storage.Scope, epoch string) error {
	if t.Version != 1 || t.Scope != scope || !scope.Valid() || t.Epoch != epoch || !storage.MigrationEpoch(epoch) || !storage.Identifier(t.ID) || kindProduct(t.Kind) != scope.Product || !digestValid(t.KeyID) || t.KeyGeneration < 1 || t.KeyGeneration >= 1<<53 {
		return ErrContract
	}
	return nil
}

// Claim pins SDK idempotency to one concrete room/device endpoint. Expiry alone
// cannot authorize publication from another device: Matrix transaction IDs are
// device scoped. Handoff requires a remote fence and catalog reconciliation.
type Claim struct {
	Version    int                       `json:"v"`
	Task       Task                      `json:"task"`
	Target     storage.PublicationTarget `json:"target"`
	Token      string                    `json:"token"`
	Generation int64                     `json:"generation"`
	ExpiresAt  time.Time                 `json:"expires_at"`
}

func (c Claim) Validate(scope storage.Scope, epoch string) error {
	if c.Version != 1 || c.Task.Validate(scope, epoch) != nil || !c.Target.Valid() || !storage.Identifier(c.Token) || c.Generation < 1 || c.Generation >= 1<<53 || c.ExpiresAt.IsZero() {
		return ErrClaim
	}
	return nil
}

// Receipt is sent only after the device re-read its authenticated SDK commit.
// It deliberately contains no plaintext hash or domain metadata. The server
// verifies this encrypted event's sender and route without decrypting content.
type Receipt struct {
	Version         int               `json:"v"`
	Scope           storage.Scope     `json:"scope"`
	Epoch           string            `json:"epoch"`
	TaskID          string            `json:"task_id"`
	CipherSHA256    string            `json:"cipher_sha256"`
	ClaimGeneration int64             `json:"claim_generation"`
	Commit          storage.Reference `json:"commit"`
}

func (r Receipt) Validate(claim Claim) error {
	if claim.Validate(r.Scope, r.Epoch) != nil || r.Version != 1 || r.TaskID != claim.Task.ID || r.CipherSHA256 != claim.Task.Cipher.SHA256 || r.ClaimGeneration != claim.Generation || !r.Commit.Valid() || r.Commit.RoomID != claim.Target.RoomID {
		return ErrReceipt
	}
	return nil
}
