// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

func storagePublicationRequest(expected publication.Request) (Request, error) {
	if !expected.Valid() || expected.Namespace != "storage:"+expected.Scope.Product {
		return Request{}, publication.ErrInvalid
	}
	return privateAppRequest(expected.Scope, expected.Scope.Product, "/matrix/storage/"+expected.Scope.Product+"/publications")
}

func (r *Rooms) publicationReceipt(ctx context.Context, request Request, expected publication.Request) (publication.Receipt, error) {
	response, err := r.c.Do(ctx, request)
	if err != nil {
		return publication.Receipt{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil {
		return publication.Receipt{}, err
	}
	defer clear(raw)
	return publication.DecodeReceipt(raw, expected)
}

// ReserveMatrixStoragePublication returns only a receipt, never implicit send
// permission. The caller must require ReservedFor on its original exact request.
func (r *Rooms) ReserveMatrixStoragePublication(ctx context.Context, proof publication.SignedRequest) (publication.Receipt, error) {
	return r.writeStoragePublication(ctx, proof, "/reserve")
}

func (r *Rooms) CancelMatrixStoragePublication(ctx context.Context, proof publication.SignedRequest) (publication.Receipt, error) {
	return r.writeStoragePublication(ctx, proof, "/cancel")
}

func (r *Rooms) writeStoragePublication(ctx context.Context, proof publication.SignedRequest, suffix string) (publication.Receipt, error) {
	request, err := storagePublicationRequest(proof.Request)
	if err != nil || len(proof.Signatures) == 0 {
		return publication.Receipt{}, publication.ErrInvalid
	}
	raw, err := json.Marshal(proof)
	if err != nil || len(raw) > 16384 {
		clear(raw)
		return publication.Receipt{}, publication.ErrInvalid
	}
	defer clear(raw)
	request.Method = http.MethodPost
	request.Path += suffix
	request.Header.Set("Content-Type", "application/json")
	request.Body = raw
	return r.publicationReceipt(ctx, request, proof.Request)
}

// ReadMatrixStoragePublication is ACK/status only. It can recover A's retained
// receipt from B without obtaining A's access token or producing new ciphertext.
func (r *Rooms) ReadMatrixStoragePublication(ctx context.Context, expected publication.Request) (publication.Receipt, error) {
	request, err := storagePublicationRequest(expected)
	if err != nil {
		return publication.Receipt{}, err
	}
	request.Query = url.Values{"room_id": {expected.Target.RoomID}, "transaction_id": {expected.TransactionID}}
	return r.publicationReceipt(ctx, request, expected)
}

// ReadMatrixStoragePublicationRecovery needs retained routing, not A's original
// in-memory request. The hint cannot authorize sending or complete a local WAL;
// the caller must authenticate SDK readback and compare its original intent.
func (r *Rooms) ReadMatrixStoragePublicationRecovery(ctx context.Context, selection publication.RecoverySelection) (publication.RecoveryHint, error) {
	if !selection.Valid() {
		return publication.RecoveryHint{}, publication.ErrInvalid
	}
	request, err := privateAppRequest(selection.Scope, selection.Scope.Product, "/matrix/storage/"+selection.Scope.Product+"/publications/recovery")
	if err != nil {
		return publication.RecoveryHint{}, err
	}
	request.Query = url.Values{"room_id": {selection.RoomID}, "transaction_id": {selection.TransactionID}}
	response, err := r.c.Do(ctx, request)
	if err != nil {
		return publication.RecoveryHint{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 32769))
	if err != nil {
		return publication.RecoveryHint{}, err
	}
	defer clear(raw)
	return publication.DecodeRecoveryHint(raw, selection)
}
