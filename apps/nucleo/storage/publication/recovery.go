// SPDX-License-Identifier: Apache-2.0
package publication

import (
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// RecoverySelection is the original logical operation's retained routing. A
// current device may select the previous producer's transaction, not a new one.
type RecoverySelection struct {
	Scope         storage.Scope
	RoomID        string
	TransactionID string
}

func (s RecoverySelection) Valid() bool {
	return s.Scope.Valid() && (storage.Reference{RoomID: s.RoomID, EventID: "$selection"}).Valid() && validTransactionID(s.TransactionID)
}

// RecoveryState contains public retained data only. It deliberately has NO
// admission marker. Reading this state cannot authorize another message HTTP.
type RecoveryState struct {
	Version       int                `json:"v"`
	Original      SignedRequest      `json:"original"`
	Status        string             `json:"status"`
	RequestSHA256 string             `json:"request_sha256"`
	Reference     *storage.Reference `json:"reference,omitempty"`
}

// Valid checks public routing/receipt consistency only; it grants no role.
func (value RecoveryState) Valid(selection RecoverySelection) bool {
	original := value.Original.Request
	if !selection.Valid() || value.Version != 1 || !original.Valid() || len(value.Original.Signatures) == 0 || original.Namespace != "storage:"+selection.Scope.Product || original.Scope != selection.Scope || original.Target.RoomID != selection.RoomID || original.TransactionID != selection.TransactionID || !validReceiptState(value.Status, value.Reference) {
		return false
	}
	digest, err := matrixproof.RequestDigest(original)
	return err == nil && digest == value.RequestSHA256 && (value.Reference == nil || value.Reference.RoomID == selection.RoomID)
}

func RecoveryFromStored(original SignedRequest, receipt Receipt) (RecoveryState, error) {
	if !receipt.Valid() {
		return RecoveryState{}, ErrInvalid
	}
	value := RecoveryState{Version: 1, Original: original, Status: receipt.Status, RequestSHA256: receipt.RequestSHA256}
	if receipt.Reference != nil {
		cloned := *receipt.Reference
		value.Reference = &cloned
	}
	selection := RecoverySelection{Scope: original.Scope, RoomID: original.Target.RoomID, TransactionID: original.TransactionID}
	if !value.Valid(selection) {
		return RecoveryState{}, ErrInvalid
	}
	return value, nil
}

// RecoveryHint is NOT a verified producer, current SDK, permission or receipt
// for another send. AcceptedReference is only a hint: the authorized SDK must
// fetch it, and the original Go application must compare the logical intent and
// data before acknowledging its WAL. Public server metadata cannot do that.
type RecoveryHint struct {
	original      Request
	status        string
	requestSHA256 string
	reference     storage.Reference
}

func DecodeRecoveryHint(raw []byte, selection RecoverySelection) (RecoveryHint, error) {
	var value RecoveryState
	if decodeControlJSON(raw, 32768, &value) != nil || !value.Valid(selection) {
		return RecoveryHint{}, ErrInvalid
	}
	hint := RecoveryHint{original: value.Original.Request, status: value.Status, requestSHA256: value.RequestSHA256}
	if value.Reference != nil {
		hint.reference = *value.Reference
	}
	return hint, nil
}

func (h RecoveryHint) Status() string        { return h.status }
func (h RecoveryHint) Original() Request     { return h.original }
func (h RecoveryHint) RequestSHA256() string { return h.requestSHA256 }
func (h RecoveryHint) AcceptedReference() (storage.Reference, bool) {
	return h.reference, h.status == "accepted" && h.original.Valid() && h.reference.Valid() && h.reference.RoomID == h.original.Target.RoomID
}
