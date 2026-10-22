// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// sealSecrets is shared by every vault writer. Encryption happens before the
// transaction; no plaintext or provider reply appears in SQL or returned errors.
func (s *Store) sealSecrets(secrets map[string]string) (map[string]string, error) {
	if !s.cipher.Enabled() {
		return nil, ErrNoCipher
	}
	sealed := make(map[string]string, len(secrets))
	for name, value := range secrets {
		encrypted, err := s.cipher.Encrypt(value)
		if err != nil {
			return nil, fmt.Errorf("store: sellar secreto: %w", err)
		}
		sealed[name] = encrypted
	}
	return sealed, nil
}

// Readers take FOR SHARE before reading config and secrets. Writers take this
// same row FOR UPDATE before touching either, including partial vault updates.
func lockConnection(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	var owner string
	err := tx.QueryRow(ctx, `SELECT external_chat_actor FROM app_instances WHERE org_id=current_setting('app.tenant',true) AND id=$1 FOR UPDATE`, id).Scan(&owner)
	return owner, normalizeConnectionError(err)
}

func normalizeConnectionError(err error) error {
	if isNoRows(err) {
		return domain.ErrNotFound
	}
	return err
}

func mergeSealedSecrets(ctx context.Context, tx pgx.Tx, id string, sealed map[string]string) error {
	for name, encrypted := range sealed {
		if _, err := tx.Exec(ctx, `INSERT INTO instance_secrets (instance_id,org_id,name,value_enc)
			VALUES ($1,current_setting('app.tenant',true),$2,$3)
			ON CONFLICT (instance_id,name) DO UPDATE SET value_enc=EXCLUDED.value_enc,updated_at=now()`, id, name, encrypted); err != nil {
			return err
		}
	}
	return nil
}

func updateInstanceTx(ctx context.Context, tx pgx.Tx, id, name string, config json.RawMessage, status domain.Status, levels []string, out *domain.AppInstance) error {
	return scanInstance(tx.QueryRow(ctx, `UPDATE app_instances
		SET name=$2,config=($3::jsonb - $6::text - $7::text)
          || jsonb_strip_nulls(jsonb_build_object($6::text,config->$6::text,$7::text,config->$7::text)),
          status=$4,level_enabled=$5,updated_at=now()
		WHERE org_id=current_setting('app.tenant',true) AND id=$1 RETURNING `+instanceCols,
		id, name, coalesceJSON(config), string(status), coalesceStrings(levels), ConfigKeyConsentimiento, ConfigKeyConsentGeneration), out)
}

func connectionOwner(ctx context.Context, owner string) error {
	actor, _ := authn.FromContext(ctx)
	if owner != "" && owner != actor.Subject {
		return domain.ErrForbidden
	}
	return nil
}

// CommitInstanceConnection replaces connection metadata and merges its encrypted
// credentials in ONE transaction. A failure leaves the complete previous pair.
// A private account remains owned by its actor, including for organization admins.
func (s *Store) CommitInstanceConnection(ctx context.Context, id, name string, config json.RawMessage, status domain.Status, levels []string, secrets map[string]string) (domain.AppInstance, error) {
	return s.commitInstanceConnection(ctx, id, name, config, status, levels, secrets, "")
}

// CommitInstanceConsent commits only the latest consumed consent. The generation
// check and connection write share the same row lock and transaction. A newer
// pending or completed choice remains untouched when an older exchange returns.
func (s *Store) CommitInstanceConsent(ctx context.Context, id, name string, config json.RawMessage, status domain.Status, levels []string, secrets map[string]string, expectedChallenge string) (domain.AppInstance, error) {
	if expectedChallenge == "" {
		return domain.AppInstance{}, domain.ErrInvalidInput
	}
	return s.commitInstanceConnection(ctx, id, name, config, status, levels, secrets, expectedChallenge)
}

func (s *Store) commitInstanceConnection(ctx context.Context, id, name string, config json.RawMessage, status domain.Status, levels []string, secrets map[string]string, expectedChallenge string) (domain.AppInstance, error) {
	sealed, err := s.sealSecrets(secrets)
	if err != nil {
		return domain.AppInstance{}, err
	}
	var instance domain.AppInstance
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		owner, err := lockConnection(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := connectionOwner(ctx, owner); err != nil {
			return err
		}
		if expectedChallenge != "" {
			var generation, pending string
			if err := tx.QueryRow(ctx, `SELECT coalesce(config->>$2,''),coalesce(config->>$3,'') FROM app_instances WHERE org_id=current_setting('app.tenant',true) AND id=$1`, id, ConfigKeyConsentGeneration, ConfigKeyConsentimiento).Scan(&generation, &pending); err != nil {
				return err
			}
			if generation != expectedChallenge || pending != "" {
				return domain.ErrConflict
			}
			var values map[string]string
			if json.Unmarshal(config, &values) != nil || values == nil {
				return domain.ErrInvalidInput
			}
			values[ConfigKeyConsentGeneration] = expectedChallenge
			delete(values, ConfigKeyConsentimiento)
			config, _ = json.Marshal(values)
		}
		if err := mergeSealedSecrets(ctx, tx, id, sealed); err != nil {
			return err
		}
		return updateInstanceTx(ctx, tx, id, name, config, status, levels, &instance)
	})
	if err != nil {
		return domain.AppInstance{}, fmt.Errorf("store: guardar conexión: %w", normalizeConnectionError(err))
	}
	return instance, nil
}

// RefreshInstanceConnection applies a token refresh only if its original bearer
// is still current. A later reconnect wins over a slow refresh. false means the
// caller must reload the current connection, without renewing or replaying a send.
// Only the provided config patch is merged; stale names/account metadata are not
// written back from an earlier read.
func (s *Store) RefreshInstanceConnection(ctx context.Context, id, expectedBearer string, configPatch, secrets map[string]string) (bool, error) {
	if expectedBearer == "" {
		return false, domain.ErrInvalidInput
	}
	sealed, err := s.sealSecrets(secrets)
	if err != nil {
		return false, err
	}
	patch, err := json.Marshal(configPatch)
	if err != nil {
		return false, err
	}
	if configPatch == nil {
		patch = []byte("{}")
	}
	applied := false
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		owner, err := lockConnection(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := connectionOwner(ctx, owner); err != nil {
			return err
		}
		var encrypted string
		if err := tx.QueryRow(ctx, `SELECT value_enc FROM instance_secrets WHERE org_id=current_setting('app.tenant',true) AND instance_id=$1 AND name=$2`, id, SecretKeyBearer).Scan(&encrypted); err != nil {
			if isNoRows(err) {
				return nil
			}
			return err
		}
		current, err := s.cipher.Decrypt(encrypted)
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(current), []byte(expectedBearer)) != 1 {
			return nil
		}
		if err := mergeSealedSecrets(ctx, tx, id, sealed); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE app_instances SET config=config || $2::jsonb,updated_at=now() WHERE org_id=current_setting('app.tenant',true) AND id=$1`, id, patch)
		applied = err == nil
		return err
	})
	if err != nil {
		return false, fmt.Errorf("store: renovar conexión: %w", normalizeConnectionError(err))
	}
	return applied, nil
}
