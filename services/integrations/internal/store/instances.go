// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// instanceCols es la proyección pública de una instancia (SIN valores de
// secretos): HasSecrets se deriva con un EXISTS sobre instance_secrets (bajo RLS).
const instanceCols = `id, org_id, template_key, name, config, status, level_enabled,
	EXISTS (SELECT 1 FROM instance_secrets s WHERE s.org_id = app_instances.org_id AND s.instance_id = app_instances.id),
	created_at, updated_at,
	(SELECT manifest FROM app_templates t WHERE t.key=app_instances.template_key),
	EXISTS (SELECT 1 FROM instance_secrets s WHERE s.org_id=app_instances.org_id AND s.instance_id=app_instances.id AND s.name='oauth_client_secret'),
	EXISTS (SELECT 1 FROM instance_secrets s WHERE s.org_id=app_instances.org_id AND s.instance_id=app_instances.id AND s.name='bearer'),
	(SELECT enabled FROM app_templates t WHERE t.key=app_instances.template_key)`

// scanInstance escanea una fila pública de instancia a un DTO de dominio.
func scanInstance(row pgx.Row, i *domain.AppInstance) error {
	var status string
	if err := row.Scan(&i.ID, &i.OrgID, &i.TemplateKey, &i.Name, &i.Config, &status,
		&i.LevelEnabled, &i.HasSecrets, &i.CreatedAt, &i.UpdatedAt, &i.ConsentManifest, &i.HasOAuthClientSecret, &i.HasOAuthToken, &i.ConsentTemplateEnabled); err != nil {
		return err
	}
	i.Status = domain.Status(status)
	return nil
}

// CreateInstance inserta una instancia (estado inicial 'draft') en la org activa.
// Devuelve domain.ErrInvalidInput si la plantilla no existe/está deshabilitada.
//
// Quien la crea es quien llama, como la organización es la del contexto: su `sub` queda
// en `created_by`, que es lo que le deja consultarla y gestionar su lista (NEG-06).
func (s *Store) CreateInstance(ctx context.Context, templateKey, name string, config json.RawMessage, levelEnabled []string) (*domain.AppInstance, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	quien, _ := authn.FromContext(ctx)
	var inst domain.AppInstance
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM app_templates WHERE key = $1 AND enabled = true)`,
			templateKey).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return domain.ErrInvalidInput
		}
		return scanInstance(tx.QueryRow(ctx, `
			INSERT INTO app_instances (org_id, template_key, name, config, status, level_enabled, created_by)
			VALUES ($1, $2, $3, $4, 'draft', $5, $6)
			RETURNING `+instanceCols,
			t.Slug, templateKey, name, coalesceJSON(config), coalesceStrings(levelEnabled), quien.Subject), &inst)
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidInput) {
			return nil, domain.ErrInvalidInput
		}
		return nil, fmt.Errorf("store: crear instancia: %w", err)
	}
	return &inst, nil
}

// ListInstances lista las instancias de la org activa (sin valores de secretos).
func (s *Store) ListInstances(ctx context.Context) ([]domain.AppInstance, error) {
	out := make([]domain.AppInstance, 0, 16)
	actor, _ := authn.FromContext(ctx)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+instanceCols+` FROM app_instances WHERE org_id = current_setting('app.tenant', true) AND (external_chat_actor='' OR external_chat_actor=$1) ORDER BY created_at DESC, id DESC`, actor.Subject)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var inst domain.AppInstance
			if err := scanInstance(rows, &inst); err != nil {
				return err
			}
			out = append(out, inst)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("store: listar instancias: %w", err)
	}
	return out, nil
}

// GetInstance carga una instancia por id (sin secretos). ErrNotFound si no existe.
func (s *Store) GetInstance(ctx context.Context, id string) (*domain.AppInstance, error) {
	var inst domain.AppInstance
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return scanInstance(tx.QueryRow(ctx, `SELECT `+instanceCols+` FROM app_instances WHERE org_id = current_setting('app.tenant', true) AND id = $1`, id), &inst)
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: obtener instancia: %w", err)
	}
	return &inst, nil
}

// UpdateInstance sustituye nombre/config/estado/niveles de una instancia y
// devuelve su nueva forma pública. ErrNotFound si no existe en la org.
func (s *Store) UpdateInstance(ctx context.Context, id, name string, config json.RawMessage, status domain.Status, levelEnabled []string) (*domain.AppInstance, error) {
	var inst domain.AppInstance
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		return updateInstanceTx(ctx, tx, id, name, config, status, levelEnabled, &inst)
	})
	if isNoRows(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: actualizar instancia: %w", err)
	}
	return &inst, nil
}

// SetStatus fija el estado de una instancia (p. ej. tras un Test). ErrNotFound si
// no existe en la org.
func (s *Store) SetStatus(ctx context.Context, id string, status domain.Status) error {
	return s.execAffecting(ctx, "fijar estado",
		`UPDATE app_instances SET status = $2, updated_at = now() WHERE org_id = current_setting('app.tenant', true) AND id = $1`, id, string(status))
}

// ConfigKeyConsentimiento guarda en la config de la instancia el code_challenge del
// último consentimiento OAuth arrancado (consent.Start.Challenge). No es secreto:
// viaja en la URL de autorización.
const ConfigKeyConsentimiento = "_oauth_pending"

// ConfigKeyConsentGeneration survives consuming a state, so a slow older
// exchange cannot replace a connection chosen by a newer consent.
const ConfigKeyConsentGeneration = "_oauth_generation"

// MarcarConsentimiento deja `challenge` como el consentimiento pendiente de la
// instancia. El de un arranque anterior deja de valer.
func (s *Store) MarcarConsentimiento(ctx context.Context, id, challenge string) error {
	return s.execAffecting(ctx, "marcar consentimiento",
		`UPDATE app_instances SET config = config || jsonb_build_object($2::text, $3::text, $4::text, $3::text) WHERE org_id = current_setting('app.tenant', true) AND id = $1`,
		id, ConfigKeyConsentimiento, challenge, ConfigKeyConsentGeneration)
}

// ConsumirConsentimiento quita el consentimiento pendiente si es `challenge`, y dice si
// lo era. Es una sola sentencia: de dos canjes con el mismo estado, solo uno lo consume.
func (s *Store) ConsumirConsentimiento(ctx context.Context, id, challenge string) (bool, error) {
	var consumido bool
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE app_instances SET config = (config - $2::text) || jsonb_build_object($4::text,coalesce(nullif(config->>$4::text,''),$3::text)) WHERE org_id = current_setting('app.tenant', true) AND id = $1 AND config->>$2::text = $3`,
			id, ConfigKeyConsentimiento, challenge, ConfigKeyConsentGeneration)
		consumido = tag.RowsAffected() == 1
		return err
	})
	if err != nil {
		return false, fmt.Errorf("store: consumir consentimiento: %w", err)
	}
	return consumido, nil
}

// DeleteInstance borra una instancia (y en cascada sus secretos). ErrNotFound si
// no existe en la org.
func (s *Store) DeleteInstance(ctx context.Context, id string) error {
	return s.execAffecting(ctx, "borrar instancia", `DELETE FROM app_instances WHERE org_id = current_setting('app.tenant', true) AND id = $1`, id)
}

// PutSecrets REEMPLAZA el conjunto completo de secretos de una instancia (API
// write-only): sella cada valor con keycrypto y reescribe instance_secrets. Un
// mapa vacío borra todos los secretos. Requiere KEK (ErrNoCipher si falta).
// ErrNotFound si la instancia no existe en la org.
func (s *Store) PutSecrets(ctx context.Context, id string, secrets map[string]string) error {
	sealed, err := s.sealSecrets(secrets)
	if err != nil {
		return err
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := lockConnection(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM instance_secrets WHERE org_id = current_setting('app.tenant', true) AND instance_id = $1`, id); err != nil {
			return err
		}
		return mergeSealedSecrets(ctx, tx, id, sealed)
	})
	if err != nil {
		return fmt.Errorf("store: escribir secretos: %w", normalizeConnectionError(err))
	}
	return nil
}

// MergeSecrets updates only the indicated keys. All vault writers lock the same
// parent row so a connection reader cannot combine different credential versions.
func (s *Store) MergeSecrets(ctx context.Context, id string, secrets map[string]string) error {
	sealed, err := s.sealSecrets(secrets)
	if err != nil {
		return err
	}
	if len(sealed) == 0 {
		return nil
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := lockConnection(ctx, tx, id); err != nil {
			return err
		}
		return mergeSealedSecrets(ctx, tx, id, sealed)
	})
	if err != nil {
		return fmt.Errorf("store: fusionar secretos: %w", normalizeConnectionError(err))
	}
	return nil
}

// LoadInstanceWithSecrets carga una instancia con sus secretos DESCIFRADOS en
// memoria — la ÚNICA lectura que abre secretos. Se usa para fabricar el Runtime
// (Test/Sync…); su resultado JAMÁS se serializa a la API. ErrNotFound si no existe.
func (s *Store) LoadInstanceWithSecrets(ctx context.Context, id string) (connector.Instance, error) {
	var (
		orgID, tmpl string
		cfgRaw      json.RawMessage
	)
	secrets := make(map[string]string)
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT org_id, template_key, config FROM app_instances WHERE org_id = current_setting('app.tenant', true) AND id = $1 FOR SHARE`, id).
			Scan(&orgID, &tmpl, &cfgRaw); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT name, value_enc FROM instance_secrets WHERE org_id = current_setting('app.tenant', true) AND instance_id = $1`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name, enc string
			if err := rows.Scan(&name, &enc); err != nil {
				return err
			}
			plain, err := s.cipher.Decrypt(enc)
			if err != nil {
				return fmt.Errorf("descifrar secreto %q: %w", name, err)
			}
			secrets[name] = plain
		}
		return rows.Err()
	})
	if isNoRows(err) {
		return connector.Instance{}, domain.ErrNotFound
	}
	if err != nil {
		return connector.Instance{}, fmt.Errorf("store: cargar instancia con secretos: %w", err)
	}
	return connector.Instance{
		ID:          id,
		OrgID:       orgID,
		TemplateKey: tmpl,
		Config:      flattenConfig(cfgRaw),
		Secrets:     secrets,
		// El HECHO de que haya bearer viaja con la instancia (no el token, que se le
		// retira al driver aguas abajo): un driver con credencial propia —Jira con
		// correo+API token, ServiceNow con usuario+contraseña— necesita saberlo para
		// apartarse cuando la conexión se ha autorizado por consentimiento. Se marca
		// AQUÍ, en la única lectura que abre secretos, y no en cada punto de despacho:
		// así lo tienen también los trabajadores de sync y migración, que cargan la
		// instancia por su cuenta y se lo habrían dejado.
		HasBearer: secrets[SecretKeyBearer] != "",
	}, nil
}

// coalesceJSON devuelve un objeto JSON vacío cuando raw viene vacío/nulo.
func coalesceJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

// coalesceStrings evita enviar NULL a una columna text[] NOT NULL (nil ⇒ '{}').
func coalesceStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// flattenConfig aplana la config pública (objeto JSON de strings) a un mapa. Por
// contrato la config es un objeto plano de strings; un JSON incompatible degrada
// a mapa vacío (no rompe la fabricación del Runtime).
func flattenConfig(raw json.RawMessage) map[string]string {
	m := make(map[string]string)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}
