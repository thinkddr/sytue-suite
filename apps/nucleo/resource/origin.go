// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// VerifiedOriginRequest is a live producer's exact request for an already
// durable resource hold. It does not prove that the origin accepted an event;
// the shared publication ledger must claim its marker atomically first.
// Historical reservations deliberately cannot create this value without the
// ORIGINAL current device and independently enrolled product session.
type VerifiedOriginRequest struct {
	reservation VerifiedReservation
	session     VerifiedSession
	request     publication.VerifiedRequest
}

func originRequestMatchesReservation(retained VerifiedReservation, current VerifiedSession, signed publication.SignedRequest) bool {
	binding := current.Binding()
	value := retained.value
	generation, err := binding.Generation.Int64()
	return err == nil && generation >= 1 && binding.Scope.Product == value.Resource.Product && binding.Scope == retained.descriptor.Producer.Pin.Scope && current.Actor() == value.Actor && current.Device().Identity() == retained.descriptor.Producer.Pin && signed.Scope == binding.Scope && signed.Namespace == binding.Namespace && signed.Namespace == "resources:"+value.Resource.Product && signed.Target.RoomID == value.RoomID && signed.Target.DeviceID == value.Actor.DeviceID && signed.TransactionID == value.TransactionID && signed.TransactionID == retained.descriptor.AttemptID && signed.CipherSHA256 == value.OuterCipherSHA256 && signed.AuthorityGeneration == generation
}

func VerifyOriginRequest(ctx context.Context, retained VerifiedReservation, current VerifiedSession, signed publication.SignedRequest) (VerifiedOriginRequest, error) {
	if !retained.valid() || !current.valid() || !originRequestMatchesReservation(retained, current, signed) {
		return VerifiedOriginRequest{}, ErrInvalid
	}
	if err := current.Revalidate(ctx); err != nil {
		return VerifiedOriginRequest{}, err
	}
	request, err := publication.VerifyRequest(ctx, signed, signed.Request, current.Device())
	if err != nil {
		return VerifiedOriginRequest{}, err
	}
	if err := current.Revalidate(ctx); err != nil {
		return VerifiedOriginRequest{}, err
	}
	return VerifiedOriginRequest{reservation: retained, session: current, request: request}, nil
}
func (o VerifiedOriginRequest) Request() publication.VerifiedRequest { return o.request }
func (o VerifiedOriginRequest) Reservation() VerifiedReservation     { return o.reservation }
func (o VerifiedOriginRequest) SessionBinding() SessionBinding       { return o.session.Binding() }
func (o VerifiedOriginRequest) Revalidate(ctx context.Context) error {
	if !o.reservation.valid() || !o.request.Value().Valid() || o.request.SHA256() == "" {
		return ErrRevoked
	}
	return o.session.Revalidate(ctx)
}
