// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"errors"
	"io/fs"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

//go:embed private_ingress_v1.sql
var privateIngressV1 []byte

//go:embed private_ingress_authorization_v1.sql
var privateIngressAuthorizationV1 []byte

//go:embed private_ingress_bodies_v1.sql
var privateIngressBodiesV1 []byte

//go:embed private_ingress_handoff_v1.sql
var privateIngressHandoffV1 []byte

//go:embed private_ingress_rejections_v1.sql
var privateIngressRejectionsV1 []byte

//go:embed private_ingress_epoch_v1.sql
var privateIngressEpochV1 []byte

//go:embed private_ingress_pq_v1.sql
var privateIngressPQV1 []byte

//go:embed private_ingress_rotation_v1.sql
var privateIngressRotationV1 []byte

const privateIngressMarkerV1 = "-- sytue:private-ingress-v1"

func WithPrivateIngressDDL(source fs.FS) fs.FS {
	return withMigrationDDL(source, append(privateStorageFragments(),
		migrationDDLFragment{marker: privateIngressMarkerV1, sql: append(append([]byte(nil), privateIngressV1...), privateIngressAuthorizationV1...)},
		migrationDDLFragment{marker: "-- sytue:private-ingress-bodies-v1", sql: privateIngressBodiesV1},
		migrationDDLFragment{marker: "-- sytue:private-ingress-handoff-v1", sql: privateIngressHandoffV1},
		migrationDDLFragment{marker: "-- sytue:private-ingress-rejections-v1", sql: privateIngressRejectionsV1},
		migrationDDLFragment{marker: "-- sytue:private-ingress-epoch-v1", sql: privateIngressEpochV1},
		migrationDDLFragment{marker: "-- sytue:private-ingress-pq-v1", sql: privateIngressPQV1},
		migrationDDLFragment{marker: "-- sytue:private-ingress-rotation-v1", sql: privateIngressRotationV1})...)
}

type PrivateIngressConfig struct {
	// Domains resolve this scope from their authenticated actor or authoritative
	// external-recipient record. JSON and request headers are never its source.
	Scope storage.Scope
	Epoch uuid.UUID
	// Sealed publishes the existing transactional outbox notification using the
	// same transaction. events imports pg, so this explicit port avoids a cycle.
	Sealed        func(context.Context, pgx.Tx, string) error
	VerifyReceipt func(context.Context, ingress.Claim, ingress.Receipt) error
}

type PrivateIngressDelivery struct {
	Task            ingress.Task
	BlobID          string
	Target          storage.PublicationTarget
	ClaimGeneration int64
	LeaseUntil      *time.Time
	Receipt         *ingress.Receipt
	claimHash       []byte
}

func ReadPrivateIngressDelivery(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, id string) (PrivateIngressDelivery, error) {
	if !storage.Identifier(id) {
		return PrivateIngressDelivery{}, ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return PrivateIngressDelivery{}, err
	}
	return readPrivateIngressDelivery(ctx, tx, cfg, id, false)
}

func readPrivateIngressDelivery(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, id string, lock bool) (PrivateIngressDelivery, error) {
	var out PrivateIngressDelivery
	var blob, sha, device, room, receiptRoom, receiptEvent *string
	var size *int64
	var ack *time.Time
	query := `SELECT t.kind,t.key_generation,k.key_id,t.blob_id,t.cipher_size,t.cipher_sha256,t.device_id,t.room_id,t.claim_generation,t.claim_token_sha256,t.lease_until,t.receipt_room,t.receipt_event,t.acknowledged_at
FROM private_ingress_tasks t JOIN private_ingress_key_plans k ON (k.tenant,k.owner_sub,k.product,k.epoch,k.key_generation)=(t.tenant,t.owner_sub,t.product,t.epoch,t.key_generation)
WHERE t.tenant=$1 AND t.owner_sub=$2 AND t.product=$3 AND t.epoch=$4 AND t.id=$5`
	if lock {
		query += ` FOR UPDATE OF t`
	}
	err := tx.QueryRow(ctx, query, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, id).Scan(&out.Task.Kind, &out.Task.KeyGeneration, &out.Task.KeyID, &blob, &size, &sha, &device, &room, &out.ClaimGeneration, &out.claimHash, &out.LeaseUntil, &receiptRoom, &receiptEvent, &ack)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrPrivateIngressMissing
	}
	if err != nil {
		return out, err
	}
	if blob == nil || size == nil || sha == nil {
		return out, ErrPrivateIngressPending
	}
	out.Task.Version = 1
	out.Task.Scope = cfg.Scope
	out.Task.Epoch = cfg.Epoch.String()
	out.Task.ID = id
	out.Task.Cipher = storage.StreamSourceRow{Kind: "ingress", ID: id, Size: *size, SHA256: *sha}
	out.BlobID = *blob
	if device != nil && room != nil {
		out.Target = storage.PublicationTarget{RoomID: *room, DeviceID: *device}
	}
	if ack != nil {
		if receiptRoom == nil || receiptEvent == nil {
			return out, ingress.ErrReceipt
		}
		out.Receipt = &ingress.Receipt{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), TaskID: id, CipherSHA256: *sha, ClaimGeneration: out.ClaimGeneration, Commit: storage.Reference{RoomID: *receiptRoom, EventID: *receiptEvent}}
	}
	return out, out.Task.Validate(cfg.Scope, cfg.Epoch.String())
}

func requirePrivateIngressDevice(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, target storage.PublicationTarget) error {
	var revoked *time.Time
	var room string
	err := tx.QueryRow(ctx, `SELECT room_id,revoked_at FROM private_ingress_devices WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND device_id=$5 FOR SHARE`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, target.DeviceID).Scan(&room, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPrivateIngressMissing
	}
	if err != nil {
		return err
	}
	if revoked != nil {
		return ingress.ErrRevoked
	}
	if room != target.RoomID {
		return ingress.ErrClaim
	}
	return nil
}

// ClaimPrivateIngress resumes one pinned device only. An expired lease cannot
// select a second publisher. A separate verified fence/reconciliation operation
// must settle the former device's outcome before any handoff is authorized.
func ClaimPrivateIngress(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, id string, target storage.PublicationTarget, token string, lease time.Duration) (ingress.Claim, error) {
	if !storage.Identifier(id) || !target.Valid() || !storage.Identifier(token) || lease <= 0 || lease > 5*time.Minute {
		return ingress.Claim{}, ingress.ErrClaim
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return ingress.Claim{}, err
	}
	if err := requirePrivateIngressDevice(ctx, tx, cfg, target); err != nil {
		return ingress.Claim{}, err
	}
	stored, err := readPrivateIngressDelivery(ctx, tx, cfg, id, true)
	if err != nil {
		return ingress.Claim{}, err
	}
	if stored.Receipt != nil {
		return ingress.Claim{}, ingress.ErrReceipt
	}
	digest := sha256.Sum256([]byte(token))
	generation := stored.ClaimGeneration
	if generation > 0 && (stored.Target != target || subtle.ConstantTimeCompare(stored.claimHash, digest[:]) != 1) {
		return ingress.Claim{}, ingress.ErrClaim
	}
	if generation == 0 {
		generation = 1
	}
	until := time.Now().UTC().Add(lease)
	_, err = tx.Exec(ctx, `UPDATE private_ingress_tasks SET device_id=$6,room_id=$7,claim_generation=$8,claim_token_sha256=$9,lease_until=$10 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, id, target.DeviceID, target.RoomID, generation, digest[:], until)
	if err != nil {
		return ingress.Claim{}, err
	}
	claim := ingress.Claim{Version: 1, Task: stored.Task, Target: target, Token: token, Generation: generation, ExpiresAt: until}
	return claim, claim.Validate(cfg.Scope, cfg.Epoch.String())
}

// AcknowledgePrivateIngress checks the real encrypted SDK receipt through the
// mandatory domain authority, then serializes its final ACK with revocation.
// It never deletes a blob, task, historical source or encrypted local original.
func AcknowledgePrivateIngress(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, claim ingress.Claim, receipt ingress.Receipt) error {
	if claim.Validate(cfg.Scope, cfg.Epoch.String()) != nil || receipt.Validate(claim) != nil || cfg.VerifyReceipt == nil {
		return ingress.ErrReceipt
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return err
	}
	// No row lock is held during the authority's Matrix/network verification.
	if err := cfg.VerifyReceipt(ctx, claim, receipt); err != nil {
		return err
	}
	if err := requirePrivateIngressDevice(ctx, tx, cfg, claim.Target); err != nil {
		return err
	}
	stored, err := readPrivateIngressDelivery(ctx, tx, cfg, claim.Task.ID, true)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(claim.Token))
	if stored.Task != claim.Task || stored.Target != claim.Target || stored.ClaimGeneration != claim.Generation || subtle.ConstantTimeCompare(stored.claimHash, digest[:]) != 1 {
		return ingress.ErrClaim
	}
	if stored.Receipt != nil {
		if *stored.Receipt != receipt {
			return ingress.ErrReceipt
		}
		return nil
	}
	if stored.LeaseUntil == nil || !stored.LeaseUntil.After(time.Now().UTC()) {
		return ingress.ErrClaim
	}
	_, err = tx.Exec(ctx, `UPDATE private_ingress_tasks SET receipt_room=$6,receipt_event=$7,acknowledged_at=now() WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, claim.Task.ID, receipt.Commit.RoomID, receipt.Commit.EventID)
	return err
}

var (
	ErrPrivateIngressMissing = errors.New("pg: authenticated private ingress enrollment is required")
	ErrPrivateIngressPending = errors.New("pg: private ingress body is not durably sealed")
)

// BindPrivateIngressScope uses the existing tenant transaction. Read committed
// and exact GUCs make revocation visible; a stale repeatable snapshot is refused.
func BindPrivateIngressScope(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig) error {
	tenant, err := tenancy.From(ctx)
	if tx == nil || err != nil || !cfg.Scope.Valid() || tenant.Slug != cfg.Scope.Tenant || cfg.Epoch == uuid.Nil {
		return ingress.ErrContract
	}
	var isolation, maintenance, merge string
	if err := tx.QueryRow(ctx, `SELECT current_setting('transaction_isolation'),coalesce(current_setting('app.maintenance',true),''),coalesce(current_setting('app.merge',true),'')`).Scan(&isolation, &maintenance, &merge); err != nil {
		return err
	}
	if isolation != "read committed" || enabledPrivateSetting(maintenance) || enabledPrivateSetting(merge) {
		return ingress.ErrContract
	}
	_, err = tx.Exec(ctx, `SELECT set_config('app.private_ingress_owner',$1,true),set_config('app.private_ingress_product',$2,true),set_config('app.private_ingress_epoch',$3,true)`, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch.String())
	return err
}

func enabledPrivateSetting(value string) bool {
	return value != "" && value != "false" && value != "off" && value != "0"
}

// RevokePrivateIngressDevice serializes with claim/ACK validation. Revocation
// closes local authorization immediately, but does not claim the external
// Matrix token is fenced: only a verified remote fence may authorize handoff.
func RevokePrivateIngressDevice(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, device string) error {
	if !storage.Identifier(device) {
		return ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE private_ingress_devices SET revoked_at=now(),authorization_generation=authorization_generation+1 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND device_id=$5 AND revoked_at IS NULL`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, device)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 1 {
		return nil
	}
	var revoked *time.Time
	err = tx.QueryRow(ctx, `SELECT revoked_at FROM private_ingress_devices WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND device_id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, device).Scan(&revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPrivateIngressMissing
	}
	if err != nil {
		return err
	}
	if revoked == nil {
		return ingress.ErrContract
	}
	return nil
}

// EnrollPrivateIngress retains public routing after the domain's Matrix
// authority verified the enrollment signature and cross-signed device. It
// cannot revive a revoked device ID or replace a previously published key plan.
func EnrollPrivateIngress(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, plan ingress.KeyPlan, target storage.PublicationTarget, signingKey string) error {
	if !target.Valid() || signingKey == "" || len(signingKey) > 128 || plan.Validate(cfg.Scope, cfg.Epoch.String()) != nil || agecipher.ValidateKeyPlan(plan) != nil {
		return ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO private_ingress_key_plans(tenant,owner_sub,product,epoch,key_generation,key_id,profile,public_recipient) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(tenant,owner_sub,product,epoch,key_generation) DO NOTHING`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, plan.Generation, plan.KeyID, plan.Profile, plan.Recipient)
	if err != nil {
		return privateIngressEnrollmentError(err)
	}
	stored, err := ReadPrivateIngressKey(ctx, tx, cfg, plan.Generation)
	if err != nil {
		return err
	}
	if stored != plan {
		return ingress.ErrContract
	}
	_, err = tx.Exec(ctx, `INSERT INTO private_ingress_devices(tenant,owner_sub,product,epoch,device_id,room_id,signing_key) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant,owner_sub,product,epoch,device_id) DO NOTHING`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, target.DeviceID, target.RoomID, signingKey)
	if err != nil {
		return err
	}
	var actual, room string
	var revoked *time.Time
	err = tx.QueryRow(ctx, `SELECT signing_key,room_id,revoked_at FROM private_ingress_devices WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND device_id=$5 FOR SHARE`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, target.DeviceID).Scan(&actual, &room, &revoked)
	if err != nil {
		return err
	}
	if actual != signingKey || room != target.RoomID || revoked != nil {
		return ingress.ErrRevoked
	}
	return nil
}

func ReadPrivateIngressKey(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, generation int64) (ingress.KeyPlan, error) {
	if generation < 1 || generation >= 1<<53 {
		return ingress.KeyPlan{}, ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return ingress.KeyPlan{}, err
	}
	plan := ingress.KeyPlan{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Generation: generation}
	var revoked *time.Time
	err := tx.QueryRow(ctx, `SELECT key_id,profile,public_recipient,revoked_at FROM private_ingress_key_plans WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND key_generation=$5 FOR SHARE`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, generation).Scan(&plan.KeyID, &plan.Profile, &plan.Recipient, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return ingress.KeyPlan{}, ErrPrivateIngressMissing
	}
	if err != nil {
		return ingress.KeyPlan{}, err
	}
	if revoked != nil {
		return ingress.KeyPlan{}, ingress.ErrRevoked
	}
	if err := agecipher.ValidateKeyPlan(plan); err != nil {
		return ingress.KeyPlan{}, err
	}
	return plan, nil
}

// PlanPrivateIngress fixes the original task identity/key before ciphertext is
// written. The transaction commits independently from body I/O. A duplicate
// cannot switch the plan to another kind or key generation.
func PlanPrivateIngress(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, plan ingress.KeyPlan, id, kind string) error {
	task := ingress.Task{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), ID: id, Kind: kind, KeyID: plan.KeyID, KeyGeneration: plan.Generation}
	if task.ValidateBinding(cfg.Scope, cfg.Epoch.String()) != nil || plan.Validate(cfg.Scope, cfg.Epoch.String()) != nil {
		return ingress.ErrContract
	}
	stored, err := ReadPrivateIngressKey(ctx, tx, cfg, plan.Generation)
	if err != nil {
		return err
	}
	if stored != plan {
		return ingress.ErrContract
	}
	_, err = tx.Exec(ctx, `INSERT INTO private_ingress_tasks(tenant,owner_sub,product,epoch,id,kind,key_generation) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant,owner_sub,product,epoch,id) DO NOTHING`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, id, kind, plan.Generation)
	if err != nil {
		return err
	}
	var actualKind string
	var actualGeneration int64
	err = tx.QueryRow(ctx, `SELECT kind,key_generation FROM private_ingress_tasks WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5 FOR UPDATE`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, id).Scan(&actualKind, &actualGeneration)
	if err != nil {
		return err
	}
	if actualKind != kind || actualGeneration != plan.Generation {
		return ingress.ErrContract
	}
	return nil
}

// CommitPrivateIngressBody makes one immutable ciphertext eligible for devices.
// The caller already verified durable blob bytes against this descriptor. The
// existing transactional outbox carries only the task ID, never domain content.
func CommitPrivateIngressBody(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, task ingress.Task, blobID string) error {
	if task.Validate(cfg.Scope, cfg.Epoch.String()) != nil || !storage.Identifier(blobID) || cfg.Sealed == nil {
		return ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return err
	}
	var kind string
	var generation int64
	var storedBlob, storedSHA *string
	var storedSize *int64
	err := tx.QueryRow(ctx, `SELECT kind,key_generation,blob_id,cipher_sha256,cipher_size FROM private_ingress_tasks WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5 FOR UPDATE`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, task.ID).Scan(&kind, &generation, &storedBlob, &storedSHA, &storedSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPrivateIngressMissing
	}
	if err != nil {
		return err
	}
	if kind != task.Kind || generation != task.KeyGeneration {
		return ingress.ErrContract
	}
	plan, err := ReadPrivateIngressKey(ctx, tx, cfg, generation)
	if err != nil {
		return err
	}
	if plan.KeyID != task.KeyID {
		return ingress.ErrContract
	}
	if storedBlob != nil {
		if *storedBlob != blobID || storedSHA == nil || *storedSHA != task.Cipher.SHA256 || storedSize == nil || *storedSize != task.Cipher.Size {
			return ingress.ErrContract
		}
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE private_ingress_tasks SET blob_id=$6,cipher_size=$7,cipher_sha256=$8 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, task.ID, blobID, task.Cipher.Size, task.Cipher.SHA256)
	if err != nil {
		return err
	}
	return cfg.Sealed(ctx, tx, task.ID)
}
