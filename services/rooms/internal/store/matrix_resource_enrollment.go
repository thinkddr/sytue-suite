// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type resourceSessionDirectory interface {
	ResourceSessionOrigin(context.Context, storage.Scope, string) (matrixproof.Origin, error)
	ResourceSessionDevice(context.Context, storage.Scope, string, string) (matrixproof.Device, error)
}
type resourceEnrollmentSnapshot struct {
	consentID      string
	versions       resourceLifecycleVersion
	rootKey, user  string
	rootGeneration int64
	consumed       bool
	existing       resource.SessionChallenge
}

func (s *Store) resourceEnrollmentSnapshot(ctx context.Context, scope storage.Scope, rawGrant, clientID string) (resourceEnrollmentSnapshot, error) {
	var out resourceEnrollmentSnapshot
	err := s.withResourceSessionRows(ctx, scope, func(ctx context.Context, tx pgx.Tx, versions resourceLifecycleVersion) error {
		out.versions = versions
		var consumed *time.Time
		err := tx.QueryRow(ctx, `SELECT id::text,consumed_at FROM matrix_resource_consents WHERE tenant=$1 AND subject=$2 AND product=$3 AND namespace='resources:'||product AND grant_hash=$4 AND client_id=$5 AND account_version=$6 AND tenant_version=$7 AND (expires_at>now() OR consumed_at IS NOT NULL)`, scope.Tenant, scope.Subject, scope.Product, domain.HashToken(rawGrant), clientID, versions.Account, versions.Tenant).Scan(&out.consentID, &consumed)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrForbidden
		}
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT matrix_user_id,master_key,generation FROM matrix_resource_authorities WHERE tenant=$1 AND subject=$2 AND product=$3 AND status='active'`, scope.Tenant, scope.Subject, scope.Product).Scan(&out.user, &out.rootKey, &out.rootGeneration)
		if errors.Is(err, pgx.ErrNoRows) {
			out.rootGeneration = 1
		} else if err != nil {
			return err
		}
		if consumed != nil {
			var raw []byte
			err = tx.QueryRow(ctx, `SELECT challenge FROM matrix_resource_session_challenges WHERE consent_id=$1 AND tenant=$2 AND subject=$3 AND product=$4 AND consumed_at IS NULL AND expires_at>now()`, out.consentID, scope.Tenant, scope.Subject, scope.Product).Scan(&raw)
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrForbidden
			}
			if err != nil {
				return err
			}
			if json.Unmarshal(raw, &out.existing) != nil {
				return domain.ErrForbidden
			}
			out.consumed = true
		}
		return nil
	})
	return out, err
}

// ResourceSessionChallenge first resolves the retained purpose and live SDK
// chain OUTSIDE data transactions, then repeats lifecycle/root checks under the
// same durable locks as consuming the one-use grant. It never pins a root here.
func (s *Store) ResourceSessionChallenge(ctx context.Context, scope storage.Scope, rawGrant, clientID string, signed resource.SignedRecipient) (resource.SessionChallenge, error) {
	var empty resource.SessionChallenge
	snapshot, binding, device, err := s.verifyResourceEnrollmentChallenge(ctx, scope, rawGrant, clientID, signed)
	if err != nil {
		return empty, err
	}
	if snapshot.consumed {
		return snapshot.existing, nil
	}
	now := time.Now()
	challenge, err := resource.NewSessionChallenge(ctx, binding, snapshot.rootKey == "", now.Add(5*time.Minute), now)
	if err != nil {
		return empty, err
	}
	raw, hash, recipient, public, err := encodeResourceChallengeEvidence(challenge, signed, device)
	if err != nil {
		return empty, err
	}
	err = s.withResourceSessionRows(ctx, scope, func(ctx context.Context, tx pgx.Tx, versions resourceLifecycleVersion) error {
		return insertResourceChallengeTx(ctx, tx, scope, snapshot, versions, binding, challenge, raw, hash, recipient, public)
	})
	if err != nil {
		return empty, err
	}
	return challenge, nil
}

func (s *Store) verifyResourceEnrollmentChallenge(ctx context.Context, scope storage.Scope, rawGrant, clientID string, signed resource.SignedRecipient) (resourceEnrollmentSnapshot, resource.SessionBinding, matrixproof.Device, error) {
	directory, ok := s.matrixGate.(resourceSessionDirectory)
	if !ok {
		return resourceEnrollmentSnapshot{}, resource.SessionBinding{}, matrixproof.Device{}, domain.ErrMatrixUnavailable
	}
	if !scope.Valid() || signed.Scope != scope || !signed.Valid() {
		return resourceEnrollmentSnapshot{}, resource.SessionBinding{}, matrixproof.Device{}, domain.ErrInvalidInput
	}
	snapshot, err := s.resourceEnrollmentSnapshot(ctx, scope, rawGrant, clientID)
	if err != nil {
		return resourceEnrollmentSnapshot{}, resource.SessionBinding{}, matrixproof.Device{}, err
	}
	origin, err := directory.ResourceSessionOrigin(ctx, scope, signed.Issuer.DeviceID)
	if err != nil {
		return resourceEnrollmentSnapshot{}, resource.SessionBinding{}, matrixproof.Device{}, err
	}
	root := id.Ed25519(snapshot.rootKey)
	if root == "" {
		root, err = matrixproof.EnrollmentUserRoot(ctx, origin, origin.UserID)
		if err != nil {
			return resourceEnrollmentSnapshot{}, resource.SessionBinding{}, matrixproof.Device{}, err
		}
	}
	device, err := matrixproof.VerifyUser(ctx, origin, origin.UserID, root)
	if err != nil {
		return resourceEnrollmentSnapshot{}, resource.SessionBinding{}, matrixproof.Device{}, err
	}
	enrolled, err := resource.VerifyAccountRecipient(ctx, signed, device)
	if err != nil {
		return resourceEnrollmentSnapshot{}, resource.SessionBinding{}, matrixproof.Device{}, err
	}
	if snapshot.rootKey != "" && (snapshot.rootKey != root.String() || snapshot.user != origin.UserID.String()) {
		return resourceEnrollmentSnapshot{}, resource.SessionBinding{}, matrixproof.Device{}, domain.ErrConflict
	}
	if snapshot.consumed {
		if snapshot.existing.Binding.DeviceID != signed.Issuer.DeviceID || snapshot.existing.Binding.EnrollmentSHA256 != enrolled.SHA256() {
			return resourceEnrollmentSnapshot{}, resource.SessionBinding{}, matrixproof.Device{}, domain.ErrConflict
		}
		return snapshot, resource.SessionBinding{}, device, nil
	}
	binding := resource.SessionBinding{Scope: scope, Namespace: "resources:" + scope.Product, UserID: origin.UserID.String(), DeviceID: origin.DeviceID.String(), RootKey: root.String(), Recipient: signed.Recipient, ProductKeyID: signed.KeyID, EnrollmentSHA256: enrolled.SHA256(), Generation: "1", ConsentID: snapshot.consentID}
	binding.RootGeneration, _ = resource.Sequence(snapshot.rootGeneration)
	binding.AccountVersion, _ = resource.Sequence(snapshot.versions.Account)
	binding.TenantVersion, _ = resource.Sequence(snapshot.versions.Tenant)
	return snapshot, binding, device, nil
}

func encodeResourceChallengeEvidence(challenge resource.SessionChallenge, signed resource.SignedRecipient, device matrixproof.Device) ([]byte, string, []byte, []byte, error) {
	raw, err := matrixproof.CanonicalBytes(challenge)
	if err != nil {
		return nil, "", nil, nil, err
	}
	hash, err := resource.CipherJSONDigest(raw)
	if err != nil {
		return nil, "", nil, nil, err
	}
	recipient, err := matrixproof.CanonicalBytes(signed)
	if err != nil {
		return nil, "", nil, nil, err
	}
	evidence, err := device.PublicEvidence()
	if err != nil {
		return nil, "", nil, nil, err
	}
	public, err := matrixproof.CanonicalBytes(evidence)
	return raw, hash, recipient, public, err
}

func insertResourceChallengeTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, snapshot resourceEnrollmentSnapshot, versions resourceLifecycleVersion, binding resource.SessionBinding, challenge resource.SessionChallenge, raw []byte, hash string, recipient, public []byte) error {
	if versions != snapshot.versions {
		return domain.ErrForbidden
	}
	var existingRoot string
	err := tx.QueryRow(ctx, `SELECT master_key FROM matrix_resource_authorities WHERE tenant=$1 AND subject=$2 AND product=$3 FOR UPDATE`, scope.Tenant, scope.Subject, scope.Product).Scan(&existingRoot)
	if snapshot.rootKey == "" {
		if !errors.Is(err, pgx.ErrNoRows) {
			if err != nil {
				return err
			}
			return domain.ErrConflict
		}
	} else if err != nil || existingRoot != snapshot.rootKey {
		if err != nil {
			return err
		}
		return domain.ErrConflict
	}
	command, err := tx.Exec(ctx, `UPDATE matrix_resource_consents SET consumed_at=now() WHERE id=$1 AND tenant=$4 AND subject=$5 AND product=$6 AND consumed_at IS NULL AND expires_at>now() AND account_version=$2 AND tenant_version=$3`, snapshot.consentID, versions.Account, versions.Tenant, scope.Tenant, scope.Subject, scope.Product)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	_, err = tx.Exec(ctx, `INSERT INTO matrix_resource_session_challenges(id,consent_id,tenant,subject,product,namespace,account_version,tenant_version,matrix_user_id,device_id,master_key,root_generation,generation,challenge,challenge_sha256,signed_recipient,device_evidence,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,1,$13,$14,$15,$16,$17)`, challenge.ID, snapshot.consentID, scope.Tenant, scope.Subject, scope.Product, binding.Namespace, versions.Account, versions.Tenant, binding.UserID, binding.DeviceID, binding.RootKey, snapshot.rootGeneration, raw, hash, recipient, public, time.Unix(challenge.ExpiresAt, 0))
	return err
}

type retainedResourceChallenge struct {
	challenge resource.SessionChallenge
	recipient resource.SignedRecipient
	client    string
	consumed  *time.Time
}

func (s *Store) retainedResourceChallenge(ctx context.Context, scope storage.Scope, challengeID string) (retainedResourceChallenge, error) {
	var out retainedResourceChallenge
	err := s.withResourceSessionRows(ctx, scope, func(ctx context.Context, tx pgx.Tx, versions resourceLifecycleVersion) error {
		var raw, recipient []byte
		err := tx.QueryRow(ctx, `SELECT c.challenge,c.signed_recipient,c.consumed_at,g.client_id FROM matrix_resource_session_challenges c JOIN matrix_resource_consents g ON g.id=c.consent_id AND g.namespace=c.namespace WHERE g.tenant=$2 AND g.subject=$3 AND g.product=$4 AND c.id=$1 AND c.tenant=$2 AND c.subject=$3 AND c.product=$4 AND c.account_version=$5 AND c.tenant_version=$6 AND g.consumed_at IS NOT NULL AND (c.expires_at>now() OR c.consumed_at IS NOT NULL)`, challengeID, scope.Tenant, scope.Subject, scope.Product, versions.Account, versions.Tenant).Scan(&raw, &recipient, &out.consumed, &out.client)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrForbidden
		}
		if err != nil {
			return err
		}
		if json.Unmarshal(raw, &out.challenge) != nil || json.Unmarshal(recipient, &out.recipient) != nil {
			return domain.ErrForbidden
		}
		return nil
	})
	return out, err
}

// ConfirmResourceSession requires an explicit SDK-signed root decision for a
// first pin and possession of this new device's own product recipient.
// A public/cross-signed Mail device cannot satisfy Docs' retained nonce.
func (s *Store) ConfirmResourceSession(ctx context.Context, scope storage.Scope, clientID string, proof resource.SignedSessionEnrollment) (resource.SessionRecord, error) {
	var empty resource.SessionRecord
	directory, ok := s.matrixGate.(resourceSessionDirectory)
	if !ok {
		return empty, domain.ErrMatrixUnavailable
	}
	retained, err := s.retainedResourceChallenge(ctx, scope, proof.ChallengeID)
	if err != nil {
		return empty, err
	}
	if retained.client != clientID || retained.challenge.Binding.Scope != scope {
		return empty, domain.ErrForbidden
	}
	binding := retained.challenge.Binding
	device, err := directory.ResourceSessionDevice(ctx, scope, binding.DeviceID, binding.RootKey)
	if err != nil {
		return empty, err
	}
	if retained.consumed != nil {
		return s.confirmConsumedResourceSession(ctx, scope, binding.DeviceID, proof)
	}
	verified, rawBinding, rawPublic, rawRecipient, err := verifyAndEncodeResourceSession(ctx, proof, retained, binding, device)
	if err != nil {
		return empty, err
	}
	err = s.withResourceSessionRows(ctx, scope, func(ctx context.Context, tx pgx.Tx, versions resourceLifecycleVersion) error {
		return insertConfirmedResourceSessionTx(ctx, tx, scope, retained, binding, versions, proof, verified, rawBinding, rawRecipient, rawPublic)
	})
	if err != nil {
		return empty, err
	}
	current, err := s.CurrentResourceSession(ctx, scope, binding.DeviceID)
	if err != nil {
		return empty, err
	}
	if current.ProofSHA256 != verified.SHA256() {
		return empty, domain.ErrConflict
	}
	return current, nil
}

func (s *Store) confirmConsumedResourceSession(ctx context.Context, scope storage.Scope, deviceID string, proof resource.SignedSessionEnrollment) (resource.SessionRecord, error) {
	var empty resource.SessionRecord
	current, err := s.CurrentResourceSession(ctx, scope, deviceID)
	if err != nil {
		return empty, err
	}
	raw, err := matrixproof.CanonicalBytes(proof)
	if err != nil {
		return empty, err
	}
	hash, err := resource.CipherJSONDigest(raw)
	if err != nil || hash != current.ProofSHA256 {
		return empty, domain.ErrConflict
	}
	return current, nil
}

func verifyAndEncodeResourceSession(ctx context.Context, proof resource.SignedSessionEnrollment, retained retainedResourceChallenge, binding resource.SessionBinding, device matrixproof.Device) (resource.VerifiedSessionEnrollment, []byte, []byte, []byte, error) {
	verified, err := resource.VerifySessionEnrollment(ctx, proof, retained.recipient, retained.challenge, device, time.Now())
	if err != nil {
		return resource.VerifiedSessionEnrollment{}, nil, nil, nil, err
	}
	rawBinding, err := matrixproof.CanonicalBytes(binding)
	if err != nil {
		return resource.VerifiedSessionEnrollment{}, nil, nil, nil, err
	}
	public, err := device.PublicEvidence()
	if err != nil {
		return resource.VerifiedSessionEnrollment{}, nil, nil, nil, err
	}
	rawPublic, err := matrixproof.CanonicalBytes(public)
	if err != nil {
		return resource.VerifiedSessionEnrollment{}, nil, nil, nil, err
	}
	rawRecipient, err := matrixproof.CanonicalBytes(retained.recipient)
	return verified, rawBinding, rawPublic, rawRecipient, err
}

func insertConfirmedResourceSessionTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, retained retainedResourceChallenge, binding resource.SessionBinding, versions resourceLifecycleVersion, proof resource.SignedSessionEnrollment, verified resource.VerifiedSessionEnrollment, rawBinding, rawRecipient, rawPublic []byte) error {
	rootGeneration, _ := binding.RootGeneration.Int64()
	generation, _ := binding.Generation.Int64()
	account, _ := binding.AccountVersion.Int64()
	tenant, _ := binding.TenantVersion.Int64()
	if versions.Account != account || versions.Tenant != tenant {
		return domain.ErrForbidden
	}
	var previousRoot, status string
	var previousGeneration int64
	err := tx.QueryRow(ctx, `SELECT master_key,generation,status FROM matrix_resource_authorities WHERE tenant=$1 AND subject=$2 AND product=$3 FOR UPDATE`, scope.Tenant, scope.Subject, scope.Product).Scan(&previousRoot, &previousGeneration, &status)
	missing := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !missing {
		return err
	}
	if !missing && (previousRoot != binding.RootKey || previousGeneration != rootGeneration || status != "active") {
		return domain.ErrConflict
	}
	if missing && (!retained.challenge.RequiresRootConfirmation || proof.RootDecision != "confirm_current_root" || rootGeneration != 1) {
		return domain.ErrForbidden
	}
	command, err := tx.Exec(ctx, `UPDATE matrix_resource_session_challenges SET consumed_at=now() WHERE id=$1 AND tenant=$3 AND subject=$4 AND product=$5 AND consumed_at IS NULL AND expires_at>now() AND challenge_sha256=$2`, proof.ChallengeID, proof.ChallengeSHA256, scope.Tenant, scope.Subject, scope.Product)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.resource_session_confirm','1',true)`); err != nil {
		return err
	}
	if missing {
		_, err = tx.Exec(ctx, `INSERT INTO matrix_resource_authorities(tenant,subject,product,namespace,account_version,tenant_version,matrix_user_id,master_key,recipient,product_key_id,generation,status,enrollment_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1,'active',$11)`, scope.Tenant, scope.Subject, scope.Product, binding.Namespace, account, tenant, binding.UserID, binding.RootKey, binding.Recipient, binding.ProductKeyID, verified.SHA256())
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO matrix_resource_devices(tenant,subject,product,namespace,account_version,tenant_version,matrix_user_id,device_id,master_key,recipient,product_key_id,root_generation,generation,status,challenge_id,binding,enrollment,enrollment_sha256,signed_recipient,device_evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'active',$14,$15,$16,$17,$18,$19)`, scope.Tenant, scope.Subject, scope.Product, binding.Namespace, account, tenant, binding.UserID, binding.DeviceID, binding.RootKey, binding.Recipient, binding.ProductKeyID, rootGeneration, generation, proof.ChallengeID, rawBinding, verified.Bytes(), verified.SHA256(), rawRecipient, rawPublic)
	if uniqueViolation(err) {
		return domain.ErrConflict
	}
	return err
}
