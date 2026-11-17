// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// SessionBinding is public product/device enrollment metadata. ConsentID comes
// from a consumed purpose-specific SytueID grant, never the finish request or
// an SDK display name. Recipient/ProductKeyID identify this SDK device's leaf
// key, never a private key shared by every product device. A binding copied
// from JSON is not a VerifiedSession.
type SessionBinding struct {
	Scope            storage.Scope `json:"scope"`
	Namespace        string        `json:"namespace"`
	UserID           string        `json:"user_id"`
	DeviceID         string        `json:"device_id"`
	RootKey          string        `json:"root_key"`
	Recipient        string        `json:"recipient"`
	ProductKeyID     string        `json:"product_key_id"`
	EnrollmentSHA256 string        `json:"enrollment_sha256"`
	Generation       Decimal       `json:"generation"`
	RootGeneration   Decimal       `json:"root_generation"`
	AccountVersion   Decimal       `json:"account_version"`
	TenantVersion    Decimal       `json:"tenant_version"`
	ConsentID        string        `json:"consent_id"`
}

func (b SessionBinding) Actor() Actor {
	return Actor{UserID: b.UserID, DeviceID: b.DeviceID, RootKey: b.RootKey}
}

func (b SessionBinding) Valid() bool {
	generation, err := b.Generation.Int64()
	rootGeneration, rootErr := b.RootGeneration.Int64()
	_, accountErr := b.AccountVersion.Int64()
	_, tenantErr := b.TenantVersion.Int64()
	recipient, parseErr := pqc.ParseRecipient(pqc.HybridProfile, b.Recipient)
	if parseErr != nil {
		return false
	}
	keyID, idErr := pqc.RecipientID(recipient)
	return b.Scope.Valid() && b.Namespace == "resources:"+b.Scope.Product && b.Actor().Valid() && err == nil && generation > 0 && rootErr == nil && rootGeneration > 0 && accountErr == nil && tenantErr == nil && idErr == nil && b.ProductKeyID == keyID && digest(b.EnrollmentSHA256) && canonicalTransaction(b.ConsentID)
}

type SessionChallengeHeader struct {
	Version                  int            `json:"v"`
	Domain                   string         `json:"domain"`
	ID                       string         `json:"id"`
	Binding                  SessionBinding `json:"binding"`
	NonceSHA256              string         `json:"nonce_sha256"`
	ExpiresAt                int64          `json:"expires_at"`
	RequiresRootConfirmation bool           `json:"requires_root_confirmation"`
}
type SessionChallenge struct {
	SessionChallengeHeader
	Ciphertext pqc.Envelope `json:"ciphertext"`
}

func (c SessionChallengeHeader) Valid() bool {
	return c.Version == 1 && c.Domain == "org.sytue.resource.session.challenge.v1" && canonicalTransaction(c.ID) && c.Binding.Valid() && digest(c.NonceSHA256) && c.ExpiresAt > 0 && c.ExpiresAt < (1<<53)
}

func sessionChallengeContext(header SessionChallengeHeader) ([]byte, error) {
	if !header.Valid() {
		return nil, ErrInvalid
	}
	return matrixproof.CanonicalBytes(header)
}

func SessionChallengeSHA256(challenge SessionChallenge) (string, error) {
	if !challenge.Valid() {
		return "", ErrInvalid
	}
	return matrixproof.RequestDigest(challenge)
}

// NewSessionChallenge seals only a random nonce using the suite's one age
// MLKEM768-X25519 implementation. The server retains the hash/ciphertext, not a
// product private key. Root enrollment remains an explicit independent step.
func NewSessionChallenge(ctx context.Context, binding SessionBinding, confirmRoot bool, expires, now time.Time) (SessionChallenge, error) {
	if err := ctx.Err(); err != nil {
		return SessionChallenge{}, err
	}
	if !binding.Valid() || !expires.After(now) || expires.After(now.Add(10*time.Minute)) {
		return SessionChallenge{}, ErrInvalid
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return SessionChallenge{}, err
	}
	defer clear(nonce)
	hash := sha256.Sum256(nonce)
	header := SessionChallengeHeader{Version: 1, Domain: "org.sytue.resource.session.challenge.v1", ID: uuid.NewString(), Binding: binding, NonceSHA256: hex.EncodeToString(hash[:]), ExpiresAt: expires.Unix(), RequiresRootConfirmation: confirmRoot}
	contextBytes, err := sessionChallengeContext(header)
	if err != nil {
		return SessionChallenge{}, err
	}
	recipient, err := pqc.ParseRecipient(pqc.HybridProfile, binding.Recipient)
	if err != nil {
		return SessionChallenge{}, ErrInvalid
	}
	envelope, err := pqc.Seal(recipient, contextBytes, nonce)
	if err != nil {
		return SessionChallenge{}, err
	}
	if err := ctx.Err(); err != nil {
		return SessionChallenge{}, err
	}
	return SessionChallenge{SessionChallengeHeader: header, Ciphertext: envelope}, nil
}

// OpenSessionChallenge is a crypto-only operation. A product key broker must
// validate its current scope/device lease before supplying identity; this never
// grants a role, a product session or a root from a caller's challenge metadata.
func OpenSessionChallenge(ctx context.Context, challenge SessionChallenge, identity *pqc.Identity, now time.Time) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if identity == nil || !challenge.Valid() || !now.Before(time.Unix(challenge.ExpiresAt, 0)) {
		return "", ErrInvalid
	}
	keyID, err := pqc.RecipientID(identity.Recipient())
	if err != nil || keyID != challenge.Binding.ProductKeyID {
		return "", ErrInvalid
	}
	contextBytes, err := sessionChallengeContext(challenge.SessionChallengeHeader)
	if err != nil {
		return "", err
	}
	nonce, err := pqc.Open(identity, contextBytes, challenge.Ciphertext)
	if err != nil {
		return "", err
	}
	defer clear(nonce)
	if len(nonce) != 32 {
		return "", ErrInvalid
	}
	actual := sha256.Sum256(nonce)
	expected, _ := hex.DecodeString(challenge.NonceSHA256)
	if subtle.ConstantTimeCompare(actual[:], expected) != 1 {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(nonce), nil
}

type SessionEnrollment struct {
	Version         int            `json:"v"`
	Domain          string         `json:"domain"`
	ChallengeID     string         `json:"challenge_id"`
	ChallengeSHA256 string         `json:"challenge_sha256"`
	Binding         SessionBinding `json:"binding"`
	Nonce           string         `json:"nonce"`
	RootDecision    string         `json:"root_decision"`
}
type SignedSessionEnrollment struct {
	SessionEnrollment
	Signatures signatures.Signatures `json:"signatures"`
}
type VerifiedSessionEnrollment struct {
	binding   SessionBinding
	recipient VerifiedRecipient
	device    matrixproof.Device
	proof     json.RawMessage
	hash      string
}

func (e VerifiedSessionEnrollment) Binding() SessionBinding      { return e.binding }
func (e VerifiedSessionEnrollment) Recipient() VerifiedRecipient { return e.recipient }
func (e VerifiedSessionEnrollment) Device() matrixproof.Device   { return e.device }
func (e VerifiedSessionEnrollment) SHA256() string               { return e.hash }
func (e VerifiedSessionEnrollment) Bytes() []byte                { return bytes.Clone(e.proof) }

func validSessionEnrollmentTranscript(proof SignedSessionEnrollment, expected SessionChallenge, hash string, now time.Time) bool {
	return now.Before(time.Unix(expected.ExpiresAt, 0)) && proof.Version == 1 && proof.Domain == "org.sytue.resource.session.enrollment.v1" && proof.ChallengeID == expected.ID && proof.ChallengeSHA256 == hash && proof.Binding == expected.Binding && sessionRootDecision(expected, proof.RootDecision)
}

// VerifySessionEnrollment verifies the EXACT retained challenge and its SDK
// producer. The registry consumes challenge/consent atomically; this verifier
// alone intentionally cannot create current VerifiedSession authority.
func VerifySessionEnrollment(ctx context.Context, proof SignedSessionEnrollment, recipient SignedRecipient, expected SessionChallenge, device matrixproof.Device, now time.Time) (VerifiedSessionEnrollment, error) {
	if err := ctx.Err(); err != nil {
		return VerifiedSessionEnrollment{}, err
	}
	hash, err := SessionChallengeSHA256(expected)
	if err != nil || !validSessionEnrollmentTranscript(proof, expected, hash, now) {
		return VerifiedSessionEnrollment{}, ErrInvalid
	}
	binding := expected.Binding
	if !identityMatches(device.Identity(), binding.Actor(), binding.Scope) {
		return VerifiedSessionEnrollment{}, ErrInvalid
	}
	enrolled, err := VerifyAccountRecipient(ctx, recipient, device)
	if err != nil || enrolled.SHA256() != binding.EnrollmentSHA256 || enrolled.Value().Recipient != binding.Recipient || enrolled.Value().KeyID != binding.ProductKeyID {
		return VerifiedSessionEnrollment{}, ErrInvalid
	}
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(proof.Nonce)
	if err != nil || len(nonce) != 32 {
		return VerifiedSessionEnrollment{}, ErrInvalid
	}
	defer clear(nonce)
	actual := sha256.Sum256(nonce)
	expectedHash, _ := hex.DecodeString(expected.NonceSHA256)
	if subtle.ConstantTimeCompare(actual[:], expectedHash) != 1 {
		return VerifiedSessionEnrollment{}, ErrInvalid
	}
	if err := device.VerifyAccountJSON(ctx, binding.Scope, proof); err != nil {
		return VerifiedSessionEnrollment{}, err
	}
	canonical, err := matrixproof.CanonicalBytes(proof)
	if err != nil {
		return VerifiedSessionEnrollment{}, err
	}
	proofHash, err := CipherJSONDigest(canonical)
	if err != nil {
		return VerifiedSessionEnrollment{}, err
	}
	return VerifiedSessionEnrollment{binding: binding, recipient: enrolled, device: device, proof: canonical, hash: proofHash}, nil
}

// PrepareSessionEnrollment returns the exact unsigned Go transcript for the
// maintained SDK signer, preserving decimal generation precision in Web/Wasm.
func PrepareSessionEnrollment(challenge SessionChallenge, nonce, rootDecision string, now time.Time) (SessionEnrollment, error) {
	hash, err := SessionChallengeSHA256(challenge)
	if err != nil || !now.Before(time.Unix(challenge.ExpiresAt, 0)) || !sessionRootDecision(challenge, rootDecision) {
		return SessionEnrollment{}, ErrInvalid
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(nonce)
	if err != nil || len(raw) != 32 {
		return SessionEnrollment{}, ErrInvalid
	}
	defer clear(raw)
	sum := sha256.Sum256(raw)
	expected, _ := hex.DecodeString(challenge.NonceSHA256)
	if subtle.ConstantTimeCompare(sum[:], expected) != 1 {
		return SessionEnrollment{}, ErrInvalid
	}
	return SessionEnrollment{Version: 1, Domain: "org.sytue.resource.session.enrollment.v1", ChallengeID: challenge.ID, ChallengeSHA256: hash, Binding: challenge.Binding, Nonce: nonce, RootDecision: rootDecision}, nil
}

func sessionRootDecision(challenge SessionChallenge, decision string) bool {
	if challenge.RequiresRootConfirmation {
		return decision == "confirm_current_root"
	}
	return decision == "retain_confirmed_root"
}
