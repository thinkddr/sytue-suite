// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func matrixPublicationContractTx(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT set_config('app.matrix_publication_contract','1',true)`)
	return err
}

// The caller already holds the live authority/identity transaction.
func matrixPublicationPendingTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, device string) (bool, error) {
	if err := matrixPublicationContractTx(ctx, tx); err != nil {
		return false, err
	}
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_storage_publications WHERE tenant=$1 AND subject=$2 AND product=$3 AND status='sending' AND (device_id=$4 OR origin_kind='legacy'))`, scope.Tenant, scope.Subject, scope.Product, device).Scan(&pending)
	return pending, err
}

func matrixPublicationReceiptTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, room, txn string) (publication.Receipt, publication.SignedRequest, error) {
	var receipt publication.Receipt
	var stored []byte
	var event *string
	receipt.Version, receipt.Marker.Version = 1, 1
	err := tx.QueryRow(ctx, `SELECT marker_id,status,request_sha256,event_id,request FROM matrix_storage_publications WHERE tenant=$1 AND subject=$2 AND product=$3 AND room_id=$4 AND transaction_id=$5 AND origin_kind='signed' FOR UPDATE`, scope.Tenant, scope.Subject, scope.Product, room, txn).Scan(&receipt.Marker.ID, &receipt.Status, &receipt.RequestSHA256, &event, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return publication.Receipt{}, publication.SignedRequest{}, domain.ErrNotFound
	}
	if err != nil {
		return publication.Receipt{}, publication.SignedRequest{}, err
	}
	var signed publication.SignedRequest
	if json.Unmarshal(stored, &signed) != nil || !signed.Valid() || len(signed.Signatures) == 0 || signed.Scope != scope || signed.Target.RoomID != room || signed.TransactionID != txn {
		return publication.Receipt{}, publication.SignedRequest{}, domain.ErrConflict
	}
	digest, err := matrixproof.RequestDigest(signed.Request)
	if err != nil || digest != receipt.RequestSHA256 {
		return publication.Receipt{}, publication.SignedRequest{}, domain.ErrConflict
	}
	if receipt.Status == "accepted" && event != nil {
		receipt.Reference = &storage.Reference{RoomID: room, EventID: *event}
	}
	if !receipt.Valid() {
		return publication.Receipt{}, publication.SignedRequest{}, domain.ErrConflict
	}
	return receipt, signed, nil
}

// ReserveMatrixStoragePublication commits an immutable hold before its public
// marker is returned. Namespace, room ownership, active root generation and the
// SDK producer are independently resolved; the request supplies no trust anchor.
func (s *Store) ReserveMatrixStoragePublication(ctx context.Context, subject, product string, signed publication.SignedRequest) (publication.Receipt, error) {
	return s.matrixStoragePublicationReservation(ctx, subject, product, signed, false)
}

// CancelMatrixStoragePublication commits a negative result only while the
// origin has not claimed the reservation. A missing row is created and cancelled
// in the same transaction, so a lost reserve reply never becomes a guessed 404.
// Sending and accepted receipts are returned unchanged; they are never cancelled.
func (s *Store) CancelMatrixStoragePublication(ctx context.Context, subject, product string, signed publication.SignedRequest) (publication.Receipt, error) {
	return s.matrixStoragePublicationReservation(ctx, subject, product, signed, true)
}

func (s *Store) matrixStoragePublicationReservation(ctx context.Context, subject, product string, signed publication.SignedRequest, cancel bool) (publication.Receipt, error) {
	var result publication.Receipt
	if !signed.Valid() || len(signed.Signatures) == 0 {
		return result, domain.ErrInvalidInput
	}
	directory, ok := s.matrixGate.(matrixIngressDirectory)
	if !ok {
		return result, domain.ErrMatrixUnavailable
	}
	err := s.withMatrixAuthority(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		authority, verified, err := verifyMatrixStoragePublicationTarget(ctx, tx, directory, scope, signed)
		if err != nil {
			return err
		}
		var txErr error
		result, txErr = upsertMatrixStoragePublicationTx(ctx, tx, scope, authority, signed, verified, cancel)
		return txErr
	})
	if err != nil {
		return publication.Receipt{}, err
	}
	return result, nil
}

func verifyMatrixStoragePublicationTarget(ctx context.Context, tx pgx.Tx, directory matrixIngressDirectory, scope storage.Scope, signed publication.SignedRequest) (matrixproof.Authority, publication.VerifiedRequest, error) {
	if signed.Scope != scope || signed.Namespace != "storage:"+scope.Product {
		return matrixproof.Authority{}, publication.VerifiedRequest{}, domain.ErrForbidden
	}
	if err := matrixPublicationContractTx(ctx, tx); err != nil {
		return matrixproof.Authority{}, publication.VerifiedRequest{}, err
	}
	if err := matrixAuthorityOwnsTarget(ctx, tx, scope, signed.Target); err != nil {
		return matrixproof.Authority{}, publication.VerifiedRequest{}, err
	}
	authority, err := matrixAuthorityTx(ctx, tx, scope)
	if err != nil {
		return matrixproof.Authority{}, publication.VerifiedRequest{}, err
	}
	if authority.RequireActive(scope, signed.AuthorityGeneration) != nil {
		return matrixproof.Authority{}, publication.VerifiedRequest{}, domain.ErrForbidden
	}
	origin, err := directory.IngressDeviceOrigin(ctx, scope, signed.Target)
	if err != nil {
		return matrixproof.Authority{}, publication.VerifiedRequest{}, err
	}
	if origin.UserID.String() != authority.UserID {
		return matrixproof.Authority{}, publication.VerifiedRequest{}, domain.ErrForbidden
	}
	device, err := matrixproof.Verify(ctx, origin, id.Ed25519(authority.RootKey))
	if err != nil {
		return matrixproof.Authority{}, publication.VerifiedRequest{}, err
	}
	verified, err := publication.VerifyRequest(ctx, signed, signed.Request, device)
	return authority, verified, err
}

func upsertMatrixStoragePublicationTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, authority matrixproof.Authority, signed publication.SignedRequest, verified publication.VerifiedRequest, cancel bool) (publication.Receipt, error) {
	existing, request, err := matrixPublicationReceiptTx(ctx, tx, scope, signed.Target.RoomID, signed.TransactionID)
	if err == nil {
		if request.Request != signed.Request || existing.RequestSHA256 != verified.SHA256() {
			return publication.Receipt{}, domain.ErrConflict
		}
		if cancel {
			if err := cancelMatrixPublicationReservationTx(ctx, tx, scope, &existing); err != nil {
				return publication.Receipt{}, err
			}
		}
		return existing, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return publication.Receipt{}, err
	}
	var retired bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_storage_device_fences WHERE tenant=$1 AND subject=$2 AND product=$3 AND fence#>>'{previous,target,device_id}'=$4)`, scope.Tenant, scope.Subject, scope.Product, signed.Target.DeviceID).Scan(&retired); err != nil {
		return publication.Receipt{}, err
	}
	if retired {
		return publication.Receipt{}, domain.ErrForbidden
	}
	marker, _, err := domain.NewOpaqueToken()
	if err != nil {
		return publication.Receipt{}, err
	}
	result := publication.Receipt{Version: 1, Marker: publication.Marker{Version: 1, ID: marker}, Status: "reserved", RequestSHA256: verified.SHA256()}
	if !result.Valid() {
		return publication.Receipt{}, domain.ErrConflict
	}
	raw, err := json.Marshal(signed)
	if err != nil {
		return publication.Receipt{}, err
	}
	evidence, err := verified.PublicEvidence()
	if err != nil {
		return publication.Receipt{}, err
	}
	public, err := json.Marshal(evidence)
	if err != nil {
		return publication.Receipt{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO matrix_storage_publications(tenant,subject,product,id,namespace,origin_kind,matrix_user_id,device_id,room_id,transaction_id,marker_id,cipher_sha256,request_sha256,authority_generation,request,producer_evidence,status) VALUES($1,$2,$3,$4,$5,'signed',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'reserved')`, scope.Tenant, scope.Subject, scope.Product, uuid.New(), signed.Namespace, authority.UserID, signed.Target.DeviceID, signed.Target.RoomID, signed.TransactionID, marker, signed.CipherSHA256, verified.SHA256(), authority.Generation, raw, public)
	if err != nil {
		return publication.Receipt{}, err
	}
	if cancel {
		if err := cancelMatrixPublicationReservationTx(ctx, tx, scope, &result); err != nil {
			return publication.Receipt{}, err
		}
	}
	return result, nil
}

func cancelMatrixPublicationReservationTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, result *publication.Receipt) error {
	if result.Status != "reserved" {
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE matrix_storage_publications SET status='cancelled' WHERE tenant=$1 AND subject=$2 AND product=$3 AND marker_id=$4 AND status='reserved'`, scope.Tenant, scope.Subject, scope.Product, result.Marker.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	result.Status = "cancelled"
	return nil
}

// ReadMatrixStoragePublication does not revive a producer, publish ciphertext or
// release a sending hold. The authenticated owner can reconcile a retained
// transaction from a newly authorized device, followed by exact SDK readback.
func (s *Store) ReadMatrixStoragePublication(ctx context.Context, subject, product, room, txn string) (publication.Receipt, error) {
	receipt, _, err := s.readMatrixStoragePublication(ctx, subject, product, room, txn)
	return receipt, err
}

// ReadMatrixStoragePublicationRecovery returns retained public data without a
// send marker. No live producer device, new signature or publication is created.
func (s *Store) ReadMatrixStoragePublicationRecovery(ctx context.Context, subject, product, room, txn string) (publication.RecoveryState, error) {
	receipt, original, err := s.readMatrixStoragePublication(ctx, subject, product, room, txn)
	if err != nil {
		return publication.RecoveryState{}, err
	}
	return publication.RecoveryFromStored(original, receipt)
}

func (s *Store) readMatrixStoragePublication(ctx context.Context, subject, product, room, txn string) (publication.Receipt, publication.SignedRequest, error) {
	var result publication.Receipt
	var original publication.SignedRequest
	scope, err := cutoverScope(ctx, subject, product)
	if err != nil {
		return result, original, err
	}
	selection := publication.RecoverySelection{Scope: scope, RoomID: room, TransactionID: txn}
	if !selection.Valid() {
		return result, original, domain.ErrInvalidInput
	}
	err = s.withMatrixAuthority(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		if err := matrixPublicationContractTx(ctx, tx); err != nil {
			return err
		}
		var err error
		result, original, err = matrixPublicationReceiptTx(ctx, tx, scope, room, txn)
		return err
	})
	if err != nil {
		return publication.Receipt{}, publication.SignedRequest{}, err
	}
	return result, original, nil
}
