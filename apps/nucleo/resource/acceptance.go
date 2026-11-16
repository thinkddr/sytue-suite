// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// AcceptanceSource fetches only the configured homeserver's exact room/event
// endpoint. It must never return user-submitted JSON or use an origin from a
// request. No encryption key or user token crosses this boundary.
type AcceptanceSource interface {
	AcceptedEvent(context.Context, storage.Reference) (*event.Event, error)
}
type VerifiedAcceptance struct {
	reference         storage.Reference
	resource          Resource
	publicationSHA256 string
	cipherSHA256      string
	reservationSHA256 string
}

func (a VerifiedAcceptance) Reference() storage.Reference { return a.reference }
func (a VerifiedAcceptance) Resource() Resource           { return a.resource }
func (a VerifiedAcceptance) PublicationSHA256() string    { return a.publicationSHA256 }
func (a VerifiedAcceptance) CipherSHA256() string         { return a.cipherSHA256 }
func (a VerifiedAcceptance) ReservationSHA256() string    { return a.reservationSHA256 }

// ConfirmAcceptance checks a fetched SDK event before logical commit. It is not
// a grant, a replacement for the original ACL/CAS, or indefinite Matrix dedup.
func ConfirmAcceptance(ctx context.Context, publication VerifiedPublication, reference storage.Reference, source AcceptanceSource) (VerifiedAcceptance, error) {
	if !digest(publication.SHA256()) || !publication.pending.epoch.valid() {
		return VerifiedAcceptance{}, ErrInvalid
	}
	return confirmAcceptedEvent(ctx, publication.Value(), publication.SHA256(), "", reference, source)
}

// ConfirmReservedAcceptance reconciles ONLY an existing immutable hold. It
// performs no publication and never impersonates revoked producer A as B. SQL
// still compares ReservationSHA256/attempt/op/cipher/base against that same hold
// in the atomic receipt/CAS commit; a manager cannot turn it into a new attempt.
func ConfirmReservedAcceptance(ctx context.Context, reservation VerifiedReservation, reference storage.Reference, source AcceptanceSource) (VerifiedAcceptance, error) {
	if !reservation.valid() {
		return VerifiedAcceptance{}, ErrInvalid
	}
	return confirmAcceptedEvent(ctx, reservation.value, reservation.descriptor.PublicationSHA256, reservation.hash, reference, source)
}

func confirmAcceptedEvent(ctx context.Context, value CipherPublication, publicationHash, reservationHash string, reference storage.Reference, source AcceptanceSource) (VerifiedAcceptance, error) {
	if err := ctx.Err(); err != nil {
		return VerifiedAcceptance{}, err
	}
	if !value.Valid() || !digest(publicationHash) || !reference.Valid() || reference.RoomID != value.RoomID || source == nil {
		return VerifiedAcceptance{}, ErrInvalid
	}
	if err := confirmAcceptedCipher(ctx, value.Actor, value.OuterCipherSHA256, reference, source); err != nil {
		return VerifiedAcceptance{}, err
	}
	return VerifiedAcceptance{reference: reference, resource: value.Resource, publicationSHA256: publicationHash, cipherSHA256: value.OuterCipherSHA256, reservationSHA256: reservationHash}, nil
}

// All publication phases verify the same exact SDK event endpoint and wire.
func confirmAcceptedCipher(ctx context.Context, actor Actor, cipherHash string, reference storage.Reference, source AcceptanceSource) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !actor.Valid() || !digest(cipherHash) || !reference.Valid() || source == nil {
		return ErrInvalid
	}
	wire, err := source.AcceptedEvent(ctx, reference)
	if err != nil {
		return err
	}
	if wire == nil || wire.ID.String() != reference.EventID || wire.RoomID.String() != reference.RoomID || wire.Sender.String() != actor.UserID || wire.Type.Type != event.EventEncrypted.Type {
		return ErrInvalid
	}
	raw, err := json.Marshal(&wire.Content)
	if err != nil {
		return ErrInvalid
	}
	defer clear(raw)
	hash, err := OuterCipherDigest(raw)
	if err != nil || hash != cipherHash {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
