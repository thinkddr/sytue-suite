// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// Assignment is public routing retained before fencing. It deliberately omits
// the original claim token, which is never handed to another device.
type Assignment struct {
	Target     storage.PublicationTarget `json:"target"`
	Generation int64                     `json:"generation"`
	SigningKey string                    `json:"signing_key"`
	ExpiresAt  time.Time                 `json:"expires_at"`
}

func (a Assignment) Valid() bool {
	return a.Target.Valid() && a.Generation > 0 && a.Generation < 1<<53 && len(a.SigningKey) == 43 && !a.ExpiresAt.IsZero()
}

func (a Assignment) Same(other Assignment) bool {
	return a.Target == other.Target && a.Generation == other.Generation && a.SigningKey == other.SigningKey && a.ExpiresAt.Equal(other.ExpiresAt)
}

type TaskState struct {
	Version    int         `json:"v"`
	Task       Task        `json:"task"`
	Plan       KeyPlan     `json:"plan"`
	Assignment *Assignment `json:"assignment,omitempty"`
	Receipt    *Receipt    `json:"receipt,omitempty"`
}

func (s TaskState) Validate(scope storage.Scope, epoch, id string) error {
	if s.Version != 1 || s.Task.Validate(scope, epoch) != nil || s.Task.ID != id || s.Plan.Validate(scope, epoch) != nil || s.Plan.KeyID != s.Task.KeyID || s.Plan.Generation != s.Task.KeyGeneration {
		return ErrContract
	}
	if s.Assignment != nil && !s.Assignment.Valid() {
		return ErrClaim
	}
	if s.Receipt != nil && (s.Assignment == nil || !handoffReceiptMatches(s.Task, *s.Assignment, *s.Receipt)) {
		return ErrReceipt
	}
	return nil
}

// Both phases sign the complete immutable original and destination. Resolve
// reports a verified SDK reconciliation, never expiry as a publication permit.
type HandoffRequest struct {
	ID           string     `json:"id"`
	Phase        string     `json:"phase"`
	TaskID       string     `json:"task_id"`
	CipherSHA256 string     `json:"cipher_sha256"`
	Previous     Assignment `json:"previous"`
	Token        string     `json:"token"`
	Resolution   string     `json:"resolution,omitempty"`
	Receipt      *Receipt   `json:"receipt,omitempty"`
	LeaseSeconds int64      `json:"lease_seconds,omitempty"`
}

func (h HandoffRequest) validate(scope storage.Scope, epoch string, target storage.PublicationTarget) bool {
	if !h.validBinding(target) {
		return false
	}
	switch h.Phase {
	case "begin":
		return h.Resolution == "" && h.Receipt == nil && h.LeaseSeconds == 0
	case "resolve":
		return h.validResolution(scope, epoch)
	}
	return false
}

func (h HandoffRequest) validBinding(target storage.PublicationTarget) bool {
	return storage.MigrationEpoch(h.ID) && storage.Identifier(h.TaskID) && digestValid(h.CipherSHA256) && h.Previous.Valid() && h.Previous.Target.DeviceID != target.DeviceID && storage.Identifier(h.Token)
}

func (h HandoffRequest) validResolution(scope storage.Scope, epoch string) bool {
	switch h.Resolution {
	case "absent":
		return h.Receipt == nil && h.LeaseSeconds >= 1 && h.LeaseSeconds <= 300
	case "committed":
		return h.validCommitted(scope, epoch)
	default:
		return false
	}
}

func (h HandoffRequest) validCommitted(scope storage.Scope, epoch string) bool {
	return h.LeaseSeconds == 0 && h.Receipt != nil && h.Receipt.Version == 1 && h.Receipt.Scope == scope && h.Receipt.Epoch == epoch && h.Receipt.TaskID == h.TaskID && h.Receipt.CipherSHA256 == h.CipherSHA256 && h.Receipt.ClaimGeneration == h.Previous.Generation && h.Receipt.Commit.Valid() && h.Receipt.Commit.RoomID == h.Previous.Target.RoomID
}

// BeginHandoffRequest reconstructs the locally retained first intent. A resolve
// cannot substitute another token, original device, key, task or ciphertext.
func (r ControlRequest) BeginHandoffRequest() (ControlRequest, error) {
	if r.Kind != "task.handoff" || r.Handoff == nil || r.Validate(r.Scope, r.Epoch) != nil {
		return ControlRequest{}, ErrContract
	}
	begin := *r.Handoff
	begin.Phase = "begin"
	begin.Resolution = ""
	begin.Receipt = nil
	begin.LeaseSeconds = 0
	r.Handoff = &begin
	return r, nil
}

type DeviceFence struct {
	Version       int                       `json:"v"`
	Scope         storage.Scope             `json:"scope"`
	Epoch         string                    `json:"epoch"`
	ID            string                    `json:"id"`
	TaskID        string                    `json:"task_id"`
	CipherSHA256  string                    `json:"cipher_sha256"`
	Previous      Assignment                `json:"previous"`
	Target        storage.PublicationTarget `json:"target"`
	Authority     matrixproof.Authority     `json:"authority"`
	RequestSHA256 string                    `json:"request_sha256"`
	Status        string                    `json:"status"`
	ConfirmedAt   *time.Time                `json:"confirmed_at,omitempty"`
}

func (f DeviceFence) Validate(request ControlRequest, authority matrixproof.Authority) error {
	begin, err := request.BeginHandoffRequest()
	if err != nil {
		return err
	}
	digest, err := matrixproof.RequestDigest(begin)
	if err != nil {
		return err
	}
	if !f.matches(begin, authority, digest) {
		return ErrRevoked
	}
	if f.Status == "planned" && f.ConfirmedAt == nil {
		return nil
	}
	if f.Status == "confirmed" && f.ConfirmedAt != nil && !f.ConfirmedAt.IsZero() {
		return nil
	}
	return ErrClaim
}

func (f DeviceFence) matches(begin ControlRequest, authority matrixproof.Authority, digest string) bool {
	h := begin.Handoff
	return f.Version == 1 && f.Scope == begin.Scope && f.Epoch == begin.Epoch && f.ID == h.ID && f.TaskID == h.TaskID && f.CipherSHA256 == h.CipherSHA256 && f.Previous.Same(h.Previous) && f.Target == begin.Target && f.Authority == authority && authority.RequireActive(begin.Scope, authority.Generation) == nil && f.RequestSHA256 == digest
}

type HandoffView struct {
	Version   int                       `json:"v"`
	ID        string                    `json:"id"`
	Task      Task                      `json:"task"`
	Previous  Assignment                `json:"previous"`
	Target    storage.PublicationTarget `json:"target"`
	Status    string                    `json:"status"`
	Fence     *DeviceFence              `json:"fence,omitempty"`
	Claim     *Claim                    `json:"claim,omitempty"`
	Receipt   *Receipt                  `json:"receipt,omitempty"`
	Rejection *HandoffRejection         `json:"rejection,omitempty"`
}

func (v HandoffView) Validate(request ControlRequest, authority matrixproof.Authority) error {
	if request.Kind != "task.handoff" || request.Handoff == nil || request.Validate(request.Scope, request.Epoch) != nil {
		return ErrContract
	}
	if authority.RequireActive(request.Scope, authority.Generation) != nil {
		return ErrRevoked
	}
	if !v.matches(request) {
		return ErrClaim
	}
	if v.Fence != nil && v.Fence.Validate(request, authority) != nil {
		return ErrRevoked
	}
	if v.Receipt != nil && !handoffReceiptMatches(v.Task, v.Previous, *v.Receipt) {
		return ErrReceipt
	}
	if v.Rejection != nil && v.Rejection.Validate(request, authority) != nil {
		return ErrRevoked
	}
	if !v.validPhase(request) {
		return ErrClaim
	}
	return nil
}

func (v HandoffView) matches(request ControlRequest) bool {
	h := request.Handoff
	return v.Version == 1 && v.ID == h.ID && v.Task.Validate(request.Scope, request.Epoch) == nil && v.Task.ID == h.TaskID && v.Task.Cipher.SHA256 == h.CipherSHA256 && v.Previous.Same(h.Previous) && v.Target == request.Target
}

func (v HandoffView) validPhase(request ControlRequest) bool {
	h := request.Handoff
	if v.Status == "not_started" {
		return h.Phase == "begin" && v.Fence == nil && v.Claim == nil && v.Receipt == nil && v.Rejection != nil
	}
	if v.Rejection != nil {
		return false
	}
	switch v.Status {
	case "fencing":
		return v.validFencing(h)
	case "readable":
		return v.validReadable(h)
	case "claimed":
		return v.validNewClaim(request)
	case "acknowledged":
		return v.validAcknowledged(h)
	default:
		return false
	}
}

func (v HandoffView) validFencing(h *HandoffRequest) bool {
	return h.Phase == "begin" && v.Claim == nil && v.Receipt == nil && (v.Fence == nil || v.Fence.Status == "planned")
}

func (v HandoffView) validReadable(h *HandoffRequest) bool {
	return h.Phase == "begin" && v.Claim == nil && (v.Receipt != nil || (v.Fence != nil && v.Fence.Status == "confirmed"))
}

func (v HandoffView) validAcknowledged(h *HandoffRequest) bool {
	return h.Phase == "resolve" && h.Resolution == "committed" && v.Claim == nil && v.Receipt != nil && h.Receipt != nil && *v.Receipt == *h.Receipt
}

func (v HandoffView) validNewClaim(request ControlRequest) bool {
	h := request.Handoff
	return h.Phase == "resolve" && h.Resolution == "absent" && v.Receipt == nil && v.Fence != nil && v.Fence.Status == "confirmed" && v.Claim != nil && v.Claim.Validate(request.Scope, request.Epoch) == nil && v.Claim.Task == v.Task && v.Claim.Target == request.Target && v.Claim.Token == h.Token && v.Claim.Generation == h.Previous.Generation+1 && v.Claim.ExpiresAt.After(time.Now())
}

func handoffReceiptMatches(task Task, previous Assignment, receipt Receipt) bool {
	return receipt.Version == 1 && receipt.Scope == task.Scope && receipt.Epoch == task.Epoch && receipt.TaskID == task.ID && receipt.CipherSHA256 == task.Cipher.SHA256 && receipt.ClaimGeneration == previous.Generation && receipt.Commit.Valid() && receipt.Commit.RoomID == previous.Target.RoomID
}
