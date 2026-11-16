// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// MatrixPublicationRoute comes only from retained room registrations. The
// event cannot choose its account, tenant, product or identity version.
type MatrixPublicationRoute struct {
	Scope    storage.Scope
	Identity storage.IdentityVersion
	// Resource owns routing; Scope.Subject remains empty for collaborative rooms.
	// The actual producer scope must be resolved from its retained SDK request.
	Resource           resource.Resource
	ServiceID          string
	PlanSHA256         string
	RegistrationSHA256 string
}

func (s *Store) MatrixPublicationRoomRoute(ctx context.Context, room string) (MatrixPublicationRoute, bool, error) {
	var route MatrixPublicationRoute
	found := false
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_resource_route_room',$1,true)`, room); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT tenant,subject,product,2,'','','','','' FROM matrix_storage_isolated_rooms WHERE room_id=$1 UNION ALL SELECT tenant,subject,product,1,'','','','','' FROM matrix_storage_rooms WHERE room_id=$1 UNION ALL SELECT tenant,'',product,0,kind,resource_id,service_id,COALESCE(plan_sha256,''),registration_sha256 FROM matrix_resource_routes WHERE room_id=$1`, room)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var next MatrixPublicationRoute
			if err := rows.Scan(&next.Scope.Tenant, &next.Scope.Subject, &next.Scope.Product, &next.Identity, &next.Resource.Kind, &next.Resource.ID, &next.ServiceID, &next.PlanSHA256, &next.RegistrationSHA256); err != nil {
				return err
			}
			valid := next.Scope.Valid() && next.Identity.Valid() && next.Resource.Kind == "" && next.Resource.ID == "" && next.ServiceID == "" && next.PlanSHA256 == "" && next.RegistrationSHA256 == ""
			if next.Resource.Kind != "" || next.Resource.ID != "" {
				next.Resource.Tenant = next.Scope.Tenant
				next.Resource.Product = next.Scope.Product
				valid = next.Scope.Subject == "" && next.Identity == 0 && next.Resource.Valid() && next.ServiceID == resource.OriginServiceID(next.Resource.Product)
			}
			if !valid || found && route != next {
				return domain.ErrConflict
			}
			route, found = next, true
		}
		return rows.Err()
	})
	if err != nil {
		return MatrixPublicationRoute{}, false, err
	}
	return route, found, nil
}

func (s *Store) MatrixPublicationRoomScope(ctx context.Context, room string) (storage.Scope, bool, error) {
	route, found, err := s.MatrixPublicationRoomRoute(ctx, room)
	if err == nil && found && route.Resource.Valid() {
		return storage.Scope{}, false, domain.ErrForbidden
	}
	return route.Scope, found, err
}

func matrixPublicationExistingOriginTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, eventID, digest, user string) (bool, error) {
	var hash, sender string
	err := tx.QueryRow(ctx, `SELECT cipher_sha256,matrix_user_id FROM matrix_storage_publications WHERE tenant=$1 AND subject=$2 AND product=$3 AND event_id=$4 FOR UPDATE`, scope.Tenant, scope.Subject, scope.Product, eventID).Scan(&hash, &sender)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if hash != digest || sender != user {
		return false, domain.ErrConflict
	}
	return true, nil
}

// AdmitMatrixPublicationOrigin is called before the homeserver persists the
// actual encrypted event. Its scope and expected sender came from trusted
// routing. A reserved marker can be claimed by exactly one immutable event.
func (s *Store) AdmitMatrixPublicationOrigin(ctx context.Context, scope storage.Scope, expectedUser string, event publication.OriginEvent, requireMarker bool) error {
	if !event.Valid() || event.Type != "m.room.encrypted" || event.Sender != expectedUser {
		return domain.ErrForbidden
	}
	digest, marker, err := publication.WireDigest(event.Content, requireMarker)
	if err != nil {
		return err
	}
	return s.withMatrixAuthority(ctx, scope.Subject, scope.Product, func(ctx context.Context, tx pgx.Tx, actual storage.Scope) error {
		return s.admitMatrixPublicationOriginTx(ctx, tx, scope, actual, expectedUser, event, digest, marker)
	})
}

func (s *Store) admitMatrixPublicationOriginTx(ctx context.Context, tx pgx.Tx, scope, actual storage.Scope, expectedUser string, event publication.OriginEvent, digest string, marker publication.Marker) error {
	if actual != scope {
		return domain.ErrForbidden
	}
	if err := matrixPublicationContractTx(ctx, tx); err != nil {
		return err
	}
	var owns bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_storage_isolated_rooms WHERE tenant=$1 AND subject=$2 AND product=$3 AND room_id=$4)`, scope.Tenant, scope.Subject, scope.Product, event.Reference.RoomID).Scan(&owns); err != nil {
		return err
	}
	if !owns {
		return domain.ErrForbidden
	}
	existing, err := matrixPublicationExistingOriginTx(ctx, tx, scope, event.Reference.EventID, digest, expectedUser)
	if err != nil {
		return err
	}
	if existing {
		return nil
	}
	if marker.Valid() {
		return s.claimMarkedPublicationOriginTx(ctx, tx, scope, expectedUser, event, digest, marker)
	}
	return insertUnmarkedLegacyOriginTx(ctx, tx, scope, expectedUser, event, digest)
}

func (s *Store) claimMarkedPublicationOriginTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, expectedUser string, event publication.OriginEvent, digest string, marker publication.Marker) error {
	signed, err := loadReservedMarkedPublicationTx(ctx, tx, scope, expectedUser, event.Reference.RoomID, digest, marker.ID)
	if err != nil {
		return err
	}
	if err := s.verifyMarkedPublicationOriginTx(ctx, tx, scope, expectedUser, signed); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE matrix_storage_publications SET status='sending',event_id=$5 WHERE tenant=$1 AND subject=$2 AND product=$3 AND marker_id=$4 AND status='reserved'`, scope.Tenant, scope.Subject, scope.Product, marker.ID, event.Reference.EventID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}

func loadReservedMarkedPublicationTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, expectedUser, roomID, digest, markerID string) (publication.SignedRequest, error) {
	var raw []byte
	var status, hash, sender string
	var attached *string
	err := tx.QueryRow(ctx, `SELECT request,status,cipher_sha256,matrix_user_id,event_id FROM matrix_storage_publications WHERE tenant=$1 AND subject=$2 AND product=$3 AND marker_id=$4 AND origin_kind='signed' FOR UPDATE`, scope.Tenant, scope.Subject, scope.Product, markerID).Scan(&raw, &status, &hash, &sender, &attached)
	if errors.Is(err, pgx.ErrNoRows) {
		return publication.SignedRequest{}, domain.ErrForbidden
	}
	if err != nil {
		return publication.SignedRequest{}, err
	}
	var signed publication.SignedRequest
	if json.Unmarshal(raw, &signed) != nil || signed.Scope != scope || signed.Namespace != "storage:"+scope.Product || signed.Target.RoomID != roomID || signed.CipherSHA256 != digest || hash != digest || sender != expectedUser || status != "reserved" || attached != nil {
		return publication.SignedRequest{}, domain.ErrForbidden
	}
	return signed, nil
}

func (s *Store) verifyMarkedPublicationOriginTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, expectedUser string, signed publication.SignedRequest) error {
	authority, err := matrixAuthorityTx(ctx, tx, scope)
	if err != nil {
		return err
	}
	if authority.RequireActive(scope, signed.AuthorityGeneration) != nil {
		return domain.ErrForbidden
	}
	directory, ok := s.matrixGate.(matrixIngressDirectory)
	if !ok {
		return domain.ErrMatrixUnavailable
	}
	origin, err := directory.IngressDeviceOrigin(ctx, scope, signed.Target)
	if err != nil {
		return err
	}
	if origin.UserID.String() != expectedUser {
		return domain.ErrForbidden
	}
	device, err := matrixproof.Verify(ctx, origin, id.Ed25519(authority.RootKey))
	if err != nil {
		return err
	}
	_, err = publication.VerifyRequest(ctx, signed, signed.Request, device)
	return err
}

func insertUnmarkedLegacyOriginTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, expectedUser string, event publication.OriginEvent, digest string) error {
	var fenced bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_storage_device_fences WHERE tenant=$1 AND subject=$2 AND product=$3)`, scope.Tenant, scope.Subject, scope.Product).Scan(&fenced); err != nil {
		return err
	}
	if fenced {
		return domain.ErrForbidden
	}
	txn, err := matrixproof.RequestDigest(event.Reference)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO matrix_storage_publications(tenant,subject,product,id,namespace,origin_kind,matrix_user_id,device_id,room_id,transaction_id,cipher_sha256,authority_generation,status,event_id) VALUES($1,$2,$3,$4,$5,'legacy',$6,'',$7,$8,$9,0,'sending',$10)`, scope.Tenant, scope.Subject, scope.Product, uuid.New(), "storage:"+scope.Product, expectedUser, event.Reference.RoomID, "legacy_"+txn, digest, event.Reference.EventID)
	return err
}

// AcceptMatrixPublicationOrigin only acknowledges the event already admitted.
// It works after producer revocation and creates no live authorization.
func (s *Store) AcceptMatrixPublicationOrigin(ctx context.Context, scope storage.Scope, expectedUser string, event publication.OriginEvent) error {
	if !event.Valid() || event.Type != "m.room.encrypted" || event.Sender != expectedUser {
		return domain.ErrForbidden
	}
	digest, _, err := publication.WireDigest(event.Content, false)
	if err != nil {
		return err
	}
	// Do not use withMatrixAuthority here: old accepted acknowledgements must
	// remain reconcilable after the identity has been durably denied.
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, scope.Tenant); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true)`, scope.Subject, scope.Product); err != nil {
			return err
		}
		if err := matrixPublicationContractTx(ctx, tx); err != nil {
			return err
		}
		existing, err := matrixPublicationExistingOriginTx(ctx, tx, scope, event.Reference.EventID, digest, expectedUser)
		if err != nil {
			return err
		}
		if !existing {
			return domain.ErrNotFound
		}
		_, err = tx.Exec(ctx, `UPDATE matrix_storage_publications SET status='accepted',accepted_at=clock_timestamp() WHERE tenant=$1 AND subject=$2 AND product=$3 AND event_id=$4 AND status='sending'`, scope.Tenant, scope.Subject, scope.Product, event.Reference.EventID)
		return err
	})
}
