// SPDX-License-Identifier: Apache-2.0
package store

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// RegisterMatrixResourceBootstrapRoom retains routing BEFORE the first event,
// while the ORIGINAL product source and independently enrolled current device
// authorize the pre-epoch plan. The signed plan supplies no backend URL. This
// operation never creates an epoch, member grant, send marker or origin permit.
func (s *Store) RegisterMatrixResourceBootstrapRoom(ctx context.Context, plan resource.VerifiedBootstrapPlan) error {
	if err := plan.Revalidate(ctx); err != nil {
		return err
	}
	value := plan.Value()
	binding := plan.SessionBinding()
	service := resource.OriginServiceID(value.Source.Resource.Product)
	if service == "" || binding.Scope != value.CreatorScope || binding.Actor() != value.Creator || value.Source.Resource.Tenant != value.CreatorScope.Tenant {
		return domain.ErrForbidden
	}
	provider, err := s.ResourceSessionProvider()
	if err != nil {
		return err
	}
	current, err := provider.Resolve(ctx, binding.Scope, binding.DeviceID)
	if err != nil {
		return err
	}
	if current.Binding() != binding || current.Actor() != value.Creator {
		return domain.ErrForbidden
	}
	raw, hash, generation, rootGeneration, err := prepareBootstrapRoutePlan(plan, value, binding)
	if err != nil {
		return err
	}
	sourceSeq, _ := value.Source.SourceSeq.Int64()
	acl, _ := value.Source.ACLVersion.Int64()
	err = s.withResourceSessionRows(ctx, binding.Scope, func(ctx context.Context, tx pgx.Tx, versions resourceLifecycleVersion) error {
		return registerBootstrapRouteTx(ctx, tx, versions, plan, value, binding, service, raw, hash, generation, rootGeneration, sourceSeq, acl)
	})
	if err != nil {
		return err
	}
	if err := current.Revalidate(ctx); err != nil {
		return err
	}
	return plan.Revalidate(ctx)
}

func prepareBootstrapRoutePlan(plan resource.VerifiedBootstrapPlan, value resource.BootstrapPlan, binding resource.SessionBinding) ([]byte, string, int64, int64, error) {
	raw := plan.PublicPlanBytes()
	var signed resource.SignedBootstrapPlan
	if json.Unmarshal(raw, &signed) != nil || signed.BootstrapPlan != value {
		return nil, "", 0, 0, domain.ErrForbidden
	}
	hash, err := resource.CipherJSONDigest(raw)
	if err != nil {
		return nil, "", 0, 0, err
	}
	generation, err := binding.Generation.Int64()
	if err != nil {
		return nil, "", 0, 0, err
	}
	rootGeneration, err := binding.RootGeneration.Int64()
	return raw, hash, generation, rootGeneration, err
}

func registerBootstrapRouteTx(ctx context.Context, tx pgx.Tx, versions resourceLifecycleVersion, plan resource.VerifiedBootstrapPlan, value resource.BootstrapPlan, binding resource.SessionBinding, service string, raw []byte, hash string, generation, rootGeneration, sourceSeq, acl int64) error {
	account, _ := binding.AccountVersion.Int64()
	tenant, _ := binding.TenantVersion.Int64()
	if account != versions.Account || tenant != versions.Tenant {
		return domain.ErrForbidden
	}
	// The same current enrollment/authority rows are locked before inserting a
	// route. A concurrent product-device withdrawal cannot win between this
	// independent binding read and registration.
	var retained []byte
	err := tx.QueryRow(ctx, `SELECT d.binding FROM matrix_resource_devices d JOIN matrix_resource_authorities a USING(tenant,subject,product) WHERE d.tenant=$1 AND d.subject=$2 AND d.product=$3 AND d.device_id=$4 AND d.status='active' AND d.generation=$5 AND d.root_generation=$6 AND a.tenant=$1 AND a.subject=$2 AND a.product=$3 AND a.status='active' AND a.generation=$6 AND a.master_key=d.master_key AND a.matrix_user_id=d.matrix_user_id FOR SHARE OF d,a`, binding.Scope.Tenant, binding.Scope.Subject, binding.Scope.Product, binding.DeviceID, generation, rootGeneration).Scan(&retained)
	if err != nil {
		return err
	}
	var exact resource.SessionBinding
	if json.Unmarshal(retained, &exact) != nil || exact != binding {
		return domain.ErrForbidden
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('app.matrix_resource_route_contract','1',true)`); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO matrix_resource_routes(room_id,tenant,product,kind,resource_id,service_id,registration,registration_sha256,registration_kind,plan_id,plan_sha256,actor_subject,actor_user,actor_device,actor_root,actor_generation,actor_binding_sha256,source_epoch,source_seq,acl_version)VALUES($1,$2,$3,$4,$5,$6,$7,$8,'bootstrap-v1',$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)ON CONFLICT DO NOTHING`, value.RoomID, value.Source.Resource.Tenant, value.Source.Resource.Product, value.Source.Resource.Kind, value.Source.Resource.ID, service, raw, hash, value.ID, plan.SHA256(), binding.Scope.Subject, value.Creator.UserID, value.Creator.DeviceID, value.Creator.RootKey, generation, value.SessionBindingSHA256, value.Source.SourceEpoch, sourceSeq, acl)
	if err != nil {
		return err
	}
	var stored []byte
	var storedHash, planHash, serviceID, planID string
	err = tx.QueryRow(ctx, `SELECT registration,registration_sha256,plan_sha256,service_id,plan_id::text FROM matrix_resource_routes WHERE tenant=$1 AND product=$2 AND kind=$3 AND resource_id=$4 AND room_id=$5 AND registration_kind='bootstrap-v1'`, value.Source.Resource.Tenant, value.Source.Resource.Product, value.Source.Resource.Kind, value.Source.Resource.ID, value.RoomID).Scan(&stored, &storedHash, &planHash, &serviceID, &planID)
	if err != nil {
		return err
	}
	if !bytes.Equal(stored, raw) || storedHash != hash || planHash != plan.SHA256() || serviceID != service || planID != value.ID {
		return domain.ErrConflict
	}
	return nil
}
