// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func (s *Store) ResourceSessionProvider() (*resource.SessionProvider, error) {
	directory, ok := s.matrixGate.(resource.SessionDirectory)
	if !ok {
		return nil, domain.ErrMatrixUnavailable
	}
	return resource.NewSessionProvider(s, directory)
}

// CurrentResourceSession returns ONLY retained public evidence under normal RLS.
// The shared provider still verifies every SDK signature against the independent
// root pin and revalidates this exact lifecycle/generation snapshot afterwards.
func (s *Store) CurrentResourceSession(ctx context.Context, scope storage.Scope, device string) (resource.SessionRecord, error) {
	var out resource.SessionRecord
	err := s.withResourceSessionRows(ctx, scope, func(ctx context.Context, tx pgx.Tx, versions resourceLifecycleVersion) error {
		var binding, proof, challenge, recipient []byte
		var user, root, key, publicRecipient, consent string
		var generation, rootGeneration, account, tenant int64
		err := tx.QueryRow(ctx, `SELECT d.binding,d.enrollment,d.enrollment_sha256,c.challenge,d.signed_recipient,c.consumed_at,
 a.matrix_user_id,a.master_key,d.product_key_id,d.recipient,c.consent_id::text,d.generation,a.generation,d.account_version,d.tenant_version
 FROM matrix_resource_devices d JOIN matrix_resource_authorities a USING(tenant,subject,product)
 JOIN matrix_resource_session_challenges c ON c.id=d.challenge_id
 JOIN matrix_resource_consents g ON g.id=c.consent_id AND (g.tenant,g.subject,g.product,g.namespace)=(d.tenant,d.subject,d.product,d.namespace)
 WHERE a.tenant=$1 AND a.subject=$2 AND a.product=$3
 AND c.tenant=$1 AND c.subject=$2 AND c.product=$3 AND c.namespace=d.namespace
 AND g.tenant=$1 AND g.subject=$2 AND g.product=$3
 AND d.tenant=$1 AND d.subject=$2 AND d.product=$3 AND d.device_id=$4 AND d.status='active' AND a.status='active'
 AND d.matrix_user_id=a.matrix_user_id AND d.master_key=a.master_key
 AND d.root_generation=a.generation AND c.consumed_at IS NOT NULL AND c.consumed_at<c.expires_at AND g.consumed_at IS NOT NULL
 AND d.account_version=$5 AND d.tenant_version=$6`, scope.Tenant, scope.Subject, scope.Product, device, versions.Account, versions.Tenant).Scan(&binding, &proof, &out.ProofSHA256, &challenge, &recipient, &out.ConfirmedAt, &user, &root, &key, &publicRecipient, &consent, &generation, &rootGeneration, &account, &tenant)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrForbidden
		}
		if err != nil {
			return err
		}
		if json.Unmarshal(binding, &out.Binding) != nil || json.Unmarshal(proof, &out.Proof) != nil || json.Unmarshal(challenge, &out.Challenge) != nil || json.Unmarshal(recipient, &out.Recipient) != nil {
			return domain.ErrForbidden
		}
		expected := resource.SessionBinding{Scope: scope, Namespace: "resources:" + scope.Product, UserID: user, DeviceID: device, RootKey: root, Recipient: publicRecipient, ProductKeyID: key, EnrollmentSHA256: out.Binding.EnrollmentSHA256, ConsentID: consent}
		expected.Generation, _ = resource.Sequence(generation)
		expected.RootGeneration, _ = resource.Sequence(rootGeneration)
		expected.AccountVersion, _ = resource.Sequence(account)
		expected.TenantVersion, _ = resource.Sequence(tenant)
		if !expected.Valid() || out.Binding != expected {
			return domain.ErrForbidden
		}
		out.PinnedRoot = root
		out.RootGeneration = expected.RootGeneration
		return nil
	})
	return out, err
}

func (s *Store) CheckResourceSession(ctx context.Context, binding resource.SessionBinding) error {
	if !binding.Valid() {
		return domain.ErrForbidden
	}
	current, err := s.CurrentResourceSession(ctx, binding.Scope, binding.DeviceID)
	if err != nil {
		return err
	}
	if current.Binding != binding {
		return domain.ErrForbidden
	}
	return nil
}

var _ resource.SessionRegistry = (*Store)(nil)
