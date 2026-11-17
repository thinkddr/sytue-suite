// SPDX-License-Identifier: Apache-2.0
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/mail"
	"slices"
	"strings"

	"github.com/google/uuid"
)

const (
	SMTPConsent             = "smtp-transit-v1"
	SubmissionHeader        = "X-Sytue-Mail-Submission"
	MaxSubmissionRecipients = 100
)

var (
	ErrSubmission         = errors.New("mail: invalid private submission")
	ErrSubmissionConflict = errors.New("mail: submission operation changed")
	ErrSubmissionControl  = errors.New("mail: submission control generation changed")
	ErrSubmissionStarted  = errors.New("mail: submission may already have started")
)

// Submission describes one immutable, explicitly consented external delivery.
// MIME is streamed separately and is never part of a server-side receipt.
type Submission struct {
	Operation  string   `json:"operation"`
	Epoch      string   `json:"epoch"`
	From       string   `json:"from"`
	Recipients []string `json:"recipients"`
	Consent    string   `json:"consent"`
	Generation int64    `json:"generation,omitempty"`
	Control    string   `json:"control,omitempty"`
}

type SubmissionReceipt struct {
	Operation   string   `json:"operation"`
	Fingerprint string   `json:"fingerprint"`
	States      []string `json:"states"`
	Generation  int64    `json:"generation,omitempty"`
	Control     string   `json:"control,omitempty"`
	Canceled    bool     `json:"canceled,omitempty"`
}

func (s Submission) Validate() error {
	for _, id := range []string{s.Operation, s.Epoch} {
		p, err := uuid.Parse(id)
		if err != nil || p == uuid.Nil || p.String() != id {
			return ErrSubmission
		}
	}
	if s.Consent != SMTPConsent || len(s.Recipients) == 0 || len(s.Recipients) > MaxSubmissionRecipients {
		return ErrSubmission
	}
	if s.Generation < 0 || s.Generation == 0 && s.Control != "" || s.Generation > 0 && !submissionDigest(s.Control) {
		return ErrSubmission
	}
	seen := map[string]bool{}
	for _, value := range append([]string{s.From}, s.Recipients...) {
		parsed, err := mail.ParseAddress(value)
		if err != nil || parsed.Address != value || len(value) > 320 || strings.ContainsAny(value, "\x00\r\n") {
			return ErrSubmission
		}
	}
	for _, address := range s.Recipients {
		key := strings.ToLower(address)
		if seen[key] {
			return ErrSubmission
		}
		seen[key] = true
	}
	return nil
}

func (s Submission) Fingerprint(raw []byte) (string, error) {
	if len(raw) == 0 {
		return "", ErrSubmission
	}
	hash := sha256.Sum256(raw)
	return s.FingerprintSHA256(hex.EncodeToString(hash[:]))
}

func submissionDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}

// FingerprintSHA256 preserves the immutable message commitment across mutable
// opaque delivery generations. It does not reveal the scheduled time.
func (s Submission) FingerprintSHA256(checksum string) (string, error) {
	if s.Validate() != nil || !submissionDigest(checksum) {
		return "", ErrSubmission
	}
	s.Generation, s.Control = 0, ""
	value := struct {
		Submission Submission `json:"submission"`
		SHA256     string     `json:"sha256"`
	}{s, checksum}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	defer clear(encoded)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func (r SubmissionReceipt) Validate(input Submission, fingerprint string) error {
	if input.Validate() != nil || r.Operation != input.Operation || r.Fingerprint != fingerprint || len(r.States) != len(input.Recipients) {
		return ErrSubmission
	}
	if r.Generation < 0 || r.Generation == 0 && r.Control != "" || r.Generation > 0 && !submissionDigest(r.Control) {
		return ErrSubmission
	}
	if input.Generation > 0 && (r.Generation != input.Generation || r.Control != input.Control) {
		return ErrSubmissionControl
	}
	for _, state := range r.States {
		if !slices.Contains([]string{"ready", "sending", "accepted", "failed", "uncertain", "canceled"}, state) {
			return ErrSubmission
		}
	}
	if r.Canceled {
		if slices.ContainsFunc(r.States, func(state string) bool { return state != "canceled" }) {
			return ErrSubmission
		}
	} else if slices.Contains(r.States, "canceled") {
		return ErrSubmission
	}
	return nil
}

func (r SubmissionReceipt) Accepted() bool {
	return !r.Canceled && len(r.States) > 0 && !slices.ContainsFunc(r.States, func(state string) bool { return state != "accepted" })
}

// Failed is a durable proof that every recipient rejected delivery before
// acceptance. It never includes an ambiguous DATA outcome.
func (r SubmissionReceipt) Failed() bool {
	return len(r.States) > 0 && !slices.ContainsFunc(r.States, func(state string) bool { return state != "failed" })
}

// SubmissionRegistration reserves only an immutable message commitment and an
// opaque generation. It intentionally has no scheduled time or MIME.
type SubmissionRegistration struct {
	Input       Submission `json:"input"`
	Fingerprint string     `json:"fingerprint"`
}

type SubmissionControlChange struct {
	Epoch       string `json:"epoch"`
	Operation   string `json:"operation"`
	Fingerprint string `json:"fingerprint"`
	Generation  int64  `json:"generation"`
	Control     string `json:"control"`
	NextControl string `json:"next_control"`
	Cancel      bool   `json:"cancel"`
}

func (c SubmissionControlChange) Validate() error {
	for _, value := range []string{c.Epoch, c.Operation} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil || id.String() != value {
			return ErrSubmission
		}
	}
	if c.Generation <= 0 || !submissionDigest(c.Fingerprint) || !submissionDigest(c.Control) || !submissionDigest(c.NextControl) {
		return ErrSubmission
	}
	return nil
}
