// SPDX-License-Identifier: Apache-2.0
// Package resource defines the single encrypted collaborative resource wire
// contract used by Rooms and Office. It contains no server plaintext fallback,
// role resolver, new cipher or implicit recipient enrollment.
package resource

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/resource/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

var (
	ErrInvalid              = model.ErrInvalid
	ErrRevoked              = errors.New("resource: authority or epoch has changed")
	ErrConflict             = errors.New("resource: committed base sequence has changed")
	ErrPending              = errors.New("resource: publication is durably pending")
	ErrFork                 = errors.New("resource: authenticated publication chain is ambiguous")
	ErrValidatorUnavailable = errors.New("resource: an authorized validation device is required")
)

// Public routing aliases keep the common model identical for certificates and
// application transports without a dependency cycle or a second wire codec.
type (
	Decimal  = model.Decimal
	Resource = model.Resource
	Position = model.Position
)

func Sequence(value int64) (Decimal, error) { return model.Sequence(value) }

// Actor names the producer's actual maintained SDK identity. A replacement
// device cannot retry as the original producer, including through an AS relay.
type Actor struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	RootKey  string `json:"root_key"`
}

// ValidDeviceID preserves the maintained SDK actor identifier contract.
func ValidDeviceID(device string) bool { return clean(device, 255) }

func (a Actor) Valid() bool {
	return matrixID(a.UserID, '@') && ValidDeviceID(a.DeviceID) && key(a.RootKey)
}

type Action string

const (
	Read     Action = "read"
	Publish  Action = "publish"
	Comment  Action = "comment"
	Manage   Action = "manage"
	Validate Action = "validate"
)

func actionsValid(actions []Action) bool {
	if len(actions) == 0 {
		return false
	}
	var previous Action
	for _, action := range actions {
		switch action {
		case Read, Publish, Comment, Manage, Validate:
		default:
			return false
		}
		if previous != "" && action <= previous {
			return false
		}
		previous = action
	}
	return true
}

// Capability is an immutable manager-signed grant, not an ACL response. Role
// retains the original application's meaning; Actions are derived by its shared
// resolver. Issuer eligibility is checked against the PREVIOUS pinned policy,
// never against this grant's self-declared role or a server-provided roster.
type Capability struct {
	Version          int      `json:"v"`
	Domain           string   `json:"domain"`
	Resource         Resource `json:"resource"`
	RoomID           string   `json:"room_id"`
	Epoch            string   `json:"epoch"`
	Generation       Decimal  `json:"generation"`
	MembershipSHA256 string   `json:"membership_sha256"`
	PolicySHA256     string   `json:"policy_sha256"`
	Subject          string   `json:"subject"`
	UserID           string   `json:"user_id"`
	ProductKeyID     string   `json:"product_key_id"`
	Role             string   `json:"role"`
	Actions          []Action `json:"actions"`
	From             Position `json:"from"`
	Issuer           Actor    `json:"issuer"`
}
type SignedCapability struct {
	Capability
	Signatures signatures.Signatures `json:"signatures"`
}

func (c Capability) Valid() bool {
	generation, err := c.Generation.Int64()
	return c.Version == 1 && c.Domain == "org.sytue.resource.capability.v1" && c.Resource.Valid() && matrixID(c.RoomID, '!') && storage.Identifier(c.Epoch) && err == nil && generation > 0 && digest(c.MembershipSHA256) && digest(c.PolicySHA256) && clean(c.Subject, 255) && matrixID(c.UserID, '@') && digest(c.ProductKeyID) && storage.Identifier(c.Role) && actionsValid(c.Actions) && c.From.Valid() && c.Issuer.Valid()
}

func (c Capability) Allows(action Action) bool {
	return c.Valid() && slices.Contains(c.Actions, action)
}

// HistoryPolicy is signed into an epoch. Joined-only grants never silently gain
// older epoch keys; full or explicit historical access requires separate signed
// grants for those epochs. Revocation cannot erase keys already received.
type HistoryPolicy string

const (
	HistoryFull     HistoryPolicy = "full"
	HistoryJoined   HistoryPolicy = "joined"
	HistoryExplicit HistoryPolicy = "explicit"
)

func (h HistoryPolicy) Valid() bool {
	return h == HistoryFull || h == HistoryJoined || h == HistoryExplicit
}

type ValidationPolicy struct {
	// PrivateRules means original Service/Proteccion must run on an authorized
	// manager/delegated device before commit. It never exposes management hashes.
	PrivateRules bool   `json:"private_rules"`
	RulesVersion string `json:"rules_version"`
}

func (v ValidationPolicy) Valid() bool { return digest(v.RulesVersion) }

type Epoch struct {
	Version              int              `json:"v"`
	Domain               string           `json:"domain"`
	Resource             Resource         `json:"resource"`
	RoomID               string           `json:"room_id"`
	ID                   string           `json:"epoch"`
	Generation           Decimal          `json:"generation"`
	Recipient            string           `json:"recipient"`
	MembershipSHA256     string           `json:"membership_sha256"`
	History              HistoryPolicy    `json:"history"`
	Cutover              Position         `json:"cutover"`
	Baseline             Baseline         `json:"baseline"`
	Checkpoint           Checkpoint       `json:"checkpoint"`
	PreviousEpoch        string           `json:"previous_epoch,omitempty"`
	PreviousPolicySHA256 string           `json:"previous_policy_sha256,omitempty"`
	Validation           ValidationPolicy `json:"validation"`
	Issuer               Actor            `json:"issuer"`
}
type SignedEpoch struct {
	Epoch
	Signatures signatures.Signatures `json:"signatures"`
}

func (e Epoch) validAnchors() bool {
	return e.Cutover.Valid() && e.Baseline.Valid() && e.Checkpoint.Valid() && e.Baseline.Reference.RoomID == e.RoomID && e.Checkpoint.Reference.RoomID == e.RoomID && e.Checkpoint.SourceCutover == e.Cutover && e.Baseline.Reference != e.Checkpoint.Reference && e.Validation.Valid() && e.Issuer.Valid()
}

func (e Epoch) validPrevious() bool {
	if e.PreviousEpoch == "" {
		return e.PreviousPolicySHA256 == ""
	}
	return storage.Identifier(e.PreviousEpoch) && e.PreviousEpoch != e.ID && digest(e.PreviousPolicySHA256)
}

func (e Epoch) Valid() bool {
	generation, err := e.Generation.Int64()
	if e.Version != 1 || e.Domain != "org.sytue.resource.epoch.v1" || !e.Resource.Valid() || !matrixID(e.RoomID, '!') || !storage.Identifier(e.ID) || err != nil || generation < 1 || !digest(e.MembershipSHA256) || !e.History.Valid() || !e.validAnchors() {
		return false
	}
	if _, err := pqc.ParseRecipient(pqc.HybridProfile, e.Recipient); err != nil {
		return false
	}
	return e.validPrevious()
}

// Publication is metadata authenticated both inside the hybrid envelope and by
// the producer's Matrix SDK signature. Payload bytes never enter this header.
// OpCount preserves original Office AppendOps semantics, not a new sequencer.
type Publication struct {
	TransactionID    string   `json:"transaction_id"`
	Version          int      `json:"v"`
	Domain           string   `json:"domain"`
	Resource         Resource `json:"resource"`
	RoomID           string   `json:"room_id"`
	Epoch            string   `json:"epoch"`
	Generation       Decimal  `json:"generation"`
	MembershipSHA256 string   `json:"membership_sha256"`
	PolicySHA256     string   `json:"policy_sha256"`
	CapabilitySHA256 string   `json:"capability_sha256"`
	Actor            Actor    `json:"actor"`
	OpID             string   `json:"op_id"`
	Action           Action   `json:"action"`
	ExpectedBase     Position `json:"expected_base"`
	OpCount          Decimal  `json:"op_count"`
}

func (p Publication) End() (Decimal, error) {
	count, err := p.OpCount.Int64()
	if err != nil {
		return "", err
	}
	return p.ExpectedBase.Seq.Next(count)
}

func (p Publication) Valid() bool {
	generation, err := p.Generation.Int64()
	_, endErr := p.End()
	return canonicalTransaction(p.TransactionID) && (p.Action == Publish || p.Action == Comment) && p.Version == 1 && p.Domain == "org.sytue.resource.publication.v1" && p.Resource.Valid() && matrixID(p.RoomID, '!') && storage.Identifier(p.Epoch) && err == nil && generation > 0 && digest(p.MembershipSHA256) && digest(p.PolicySHA256) && digest(p.CapabilitySHA256) && p.Actor.Valid() && storage.Identifier(p.OpID) && p.ExpectedBase.Valid() && endErr == nil
}

// CipherPublication includes hashes of the exact inner and outer ciphertext.
// The relay cannot replace a sender, product, SDK device, opID or ciphertext.
type CipherPublication struct {
	SessionCertificateID string `json:"session_certificate_id"`
	Publication
	InnerCipherSHA256 string `json:"inner_cipher_sha256"`
	OuterCipherSHA256 string `json:"outer_cipher_sha256"`
}

func (p CipherPublication) Valid() bool {
	return p.Publication.Valid() && strings.HasPrefix(p.SessionCertificateID, "sessioncert1_") && digest(strings.TrimPrefix(p.SessionCertificateID, "sessioncert1_")) && digest(p.InnerCipherSHA256) && digest(p.OuterCipherSHA256)
}

type SignedPublication struct {
	CipherPublication
	Signatures signatures.Signatures `json:"signatures"`
}

// ValidationCertificate contains public cipher commitments only. Plaintext state
// or operation hashes and protection credentials remain in validator-only E2E.
// Its issuer must hold an independently verified validator/manager capability;
// a protected operation's editor signature is never sufficient by itself.
type ValidationCertificate struct {
	Version                   int                   `json:"v"`
	Domain                    string                `json:"domain"`
	PublicationSHA256         string                `json:"publication_sha256"`
	ValidatorCapabilitySHA256 string                `json:"validator_capability_sha256"`
	RulesVersion              string                `json:"rules_version"`
	Allow                     bool                  `json:"allow"`
	Issuer                    Actor                 `json:"issuer"`
	Signatures                signatures.Signatures `json:"signatures"`
}

func (v ValidationCertificate) Valid() bool {
	return v.Version == 1 && v.Domain == "org.sytue.resource.validation.v1" && digest(v.PublicationSHA256) && digest(v.ValidatorCapabilitySHA256) && digest(v.RulesVersion) && v.Issuer.Valid()
}

type Candidate struct {
	SessionCertificate json.RawMessage   `json:"session_certificate"`
	Proof              SignedPublication `json:"proof"`
	// Exact SDK m.room.encrypted JSON, prepared only AFTER its key backup and
	// public session certificate ACK. The AS is a transport, never a recipient.
	EncryptedEvent json.RawMessage        `json:"encrypted_event"`
	Validation     *ValidationCertificate `json:"validation,omitempty"`
}

func (c Candidate) Valid() bool {
	if !c.Proof.Valid() || !json.Valid(c.EncryptedEvent) || !json.Valid(c.SessionCertificate) {
		return false
	}
	return c.Validation == nil || c.Validation.Valid()
}

type Receipt struct {
	Resource          Resource          `json:"resource"`
	Epoch             string            `json:"epoch"`
	OpID              string            `json:"op_id"`
	PublicationSHA256 string            `json:"publication_sha256"`
	Reference         storage.Reference `json:"reference"`
	Base              Position          `json:"base"`
	Head              Position          `json:"head"`
}

func (r Receipt) Valid() bool {
	base, e1 := r.Base.Seq.Int64()
	head, e2 := r.Head.Seq.Int64()
	return r.Resource.Valid() && storage.Identifier(r.Epoch) && storage.Identifier(r.OpID) && digest(r.PublicationSHA256) && r.Reference.Valid() && r.Base.Valid() && r.Head.Valid() && e1 == nil && e2 == nil && head > base
}

type CandidateState string

const (
	WaitingValidation CandidateState = "waiting_validation"
	Prepared          CandidateState = "prepared"
	Sending           CandidateState = "sending"
	Ambiguous         CandidateState = "ambiguous"
	Committed         CandidateState = "committed"
	Stale             CandidateState = "stale"
	Cancelled         CandidateState = "cancelled"
)

// JournalEntry must be persisted BEFORE the first relay HTTP attempt. AttemptID
// is immutable per producer device; handoff of an unaccepted attempt requires a
// fenced new publication, never impersonation of a revoked device. AS dedup TTL
// is not an indefinite exactly-once guarantee. Receipt/OpID CAS is durable.
type JournalEntry struct {
	Candidate Candidate      `json:"candidate"`
	State     CandidateState `json:"state"`
	AttemptID string         `json:"attempt_id"`
	Receipt   *Receipt       `json:"receipt,omitempty"`
}

type Cursor struct {
	Position Position `json:"position"`
}
type Page struct {
	Records []Receipt `json:"records"`
	Next    Cursor    `json:"next"`
	More    bool      `json:"more"`
}

// RelayStore implementations preserve original ACL/CAS in ONE transaction and
// serialize revocation/cutover with in-flight receipts. Commit requires actual
// Matrix acceptance and a valid certificate when PrivateRules is true. Pending
// candidates are not assigned fictitious committed seq or success receipts.
// No method receives plaintext, private epoch identities or user SDK tokens.
type RelayStore interface {
	Stage(context.Context, Candidate) (JournalEntry, error)
	Pending(context.Context, Resource, string) (JournalEntry, error)
	Commit(context.Context, JournalEntry, storage.Reference) (Receipt, error)
	Page(context.Context, Resource, Cursor, int) (Page, error)
	Cancel(context.Context, Resource, string) error
}

func digest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}

func key(value string) bool {
	raw, err := base64.RawStdEncoding.DecodeString(value)
	return err == nil && len(raw) == 32 && base64.RawStdEncoding.EncodeToString(raw) == value
}

func clean(value string, maxLen int) bool {
	return value != "" && len(value) <= maxLen && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n\t ")
}

func matrixID(value string, sigil byte) bool {
	return len(value) > 1 && value[0] == sigil && clean(value, 255) && (sigil != '@' || strings.IndexByte(value, ':') > 1)
}

func canonicalTransaction(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}
