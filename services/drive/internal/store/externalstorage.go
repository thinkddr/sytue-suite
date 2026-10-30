// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// ExternalPrefix marca en la clave S3 los blobs que viven en el almacén de
// terceros del tenant (mig 0023). Va en la clave y no en una columna para que
// el enrutado no necesite consultar nada: la clave viaja con cada operación.
const ExternalPrefix = "ext/"

// EsExterno indica si una clave de blob vive en el almacén de terceros.
func EsExterno(storageKey string) bool { return strings.HasPrefix(storageKey, ExternalPrefix) }

// ExternalStorage es el bucket S3 propio de un tenant (mig 0023). SecretKeyEnc
// va sellado con la KEK del servicio; el store no lo abre ni lo cierra.
type ExternalStorage struct {
	LocationID uuid.UUID `json:"-"`
	Tenant     string
	Kind       string // "s3" | "webdav" | "gdrive" | "onedrive" | "dropbox" | "nas"
	InstanceID string // instancia OAuth del servicio de integraciones (gdrive/onedrive/dropbox)
	Endpoint   string
	// Clase nas: el túnel de Sytue Cloud (sitio y recurso en el proxy, agente y
	// dirección local del NAS).
	TunnelSiteID        int
	TunnelResourceID    int
	TunnelNewtID        string
	TunnelNewtSecretEnc string
	LocalEndpoint       string
	BasePath            string
	Region              string
	Bucket              string
	UseSSL              bool
	AccessKey           string
	SecretKeyEnc        string
	Label               string
	Enabled             bool
	CheckedAt           *time.Time
	CheckOK             bool
	CheckError          string
	// AcceptedTermsAt: cuándo se aceptó el aviso de responsabilidad (obligatorio).
	AcceptedTermsAt time.Time
	UpdatedBy       string
	UpdatedAt       time.Time
}

const externalStorageCols = `tenant, kind, instance_id, endpoint, base_path, region, bucket, use_ssl, access_key, secret_key_enc,
	label, enabled, checked_at, check_ok, check_error, accepted_terms_at, updated_by, updated_at,
	tunnel_site_id, tunnel_resource_id, tunnel_newt_id, tunnel_newt_secret_enc, local_endpoint, location_id`

func scanExternalStorage(row pgx.Row) (ExternalStorage, error) {
	var e ExternalStorage
	err := row.Scan(&e.Tenant, &e.Kind, &e.InstanceID, &e.Endpoint, &e.BasePath, &e.Region, &e.Bucket, &e.UseSSL, &e.AccessKey, &e.SecretKeyEnc,
		&e.Label, &e.Enabled, &e.CheckedAt, &e.CheckOK, &e.CheckError, &e.AcceptedTermsAt, &e.UpdatedBy, &e.UpdatedAt,
		&e.TunnelSiteID, &e.TunnelResourceID, &e.TunnelNewtID, &e.TunnelNewtSecretEnc, &e.LocalEndpoint, &e.LocationID)
	return e, err
}

// ExternalStorage devuelve el almacén de terceros del tenant del contexto, o
// domain.ErrNotFound si no tiene.
func (s *Store) ExternalStorage(ctx context.Context) (ExternalStorage, error) {
	t := tenancy.MustFrom(ctx)
	var e ExternalStorage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		e, err = scanExternalStorage(tx.QueryRow(ctx,
			`SELECT `+externalStorageCols+` FROM external_storage WHERE tenant=$1`, t.Slug))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ExternalStorage{}, domain.ErrNotFound
	}
	return e, err
}

// ExternalStorageFor es la lectura SIN contexto de tenant del enrutador de blobs
// (corre en cualquier petición, también en las que aún no fijaron el tenant en
// la transacción). Solo devuelve el almacén si está encendido.
func (s *Store) ExternalStorageFor(ctx context.Context, tenant string) (ExternalStorage, error) {
	var e ExternalStorage
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var err error
		e, err = scanExternalStorage(tx.QueryRow(ctx,
			`SELECT `+externalStorageCols+` FROM external_storage WHERE tenant=$1 AND enabled`, tenant))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ExternalStorage{}, domain.ErrNotFound
	}
	return e, err
}

// ExternalStorageLocation resolves an immutable physical destination. A nil ID
// means a legacy ext/ key, pinned to the configuration present at migration.
// Disabling new writes does not remove access to previously stored ciphertext.
func (s *Store) ExternalStorageLocation(ctx context.Context, tenant string, id uuid.UUID) (ExternalStorage, error) {
	var e ExternalStorage
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		var err error
		e, err = scanExternalStorage(tx.QueryRow(ctx, `SELECT `+externalStorageCols+`
   FROM jsonb_populate_record(NULL::external_storage, (
    SELECT configuration FROM external_storage_locations
    WHERE tenant=$1 AND (location_id=$2 OR ($2='00000000-0000-0000-0000-000000000000'::uuid AND legacy))
   )) WHERE tenant IS NOT NULL`, tenant, id))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ExternalStorage{}, domain.ErrNotFound
	}
	return e, err
}

// SaveExternalStorage crea o sustituye el almacén de terceros del tenant.
func (s *Store) SaveExternalStorage(ctx context.Context, e ExternalStorage, actor string) (ExternalStorage, error) {
	t := tenancy.MustFrom(ctx)
	var out ExternalStorage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanExternalStorage(tx.QueryRow(ctx, `
			INSERT INTO external_storage
			  (tenant, kind, instance_id, endpoint, base_path, region, bucket, use_ssl, access_key, secret_key_enc, label, enabled,
			   checked_at, check_ok, check_error, accepted_terms_at, updated_by, updated_at,
			   tunnel_site_id, tunnel_resource_id, tunnel_newt_id, tunnel_newt_secret_enc, local_endpoint)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, now(),
			        $18, $19, $20, $21, $22)
			ON CONFLICT (tenant) DO UPDATE
			   SET kind = EXCLUDED.kind, instance_id = EXCLUDED.instance_id, endpoint = EXCLUDED.endpoint, base_path = EXCLUDED.base_path,
			       region = EXCLUDED.region, bucket = EXCLUDED.bucket,
			       use_ssl = EXCLUDED.use_ssl, access_key = EXCLUDED.access_key,
			       secret_key_enc = EXCLUDED.secret_key_enc, label = EXCLUDED.label,
			       enabled = EXCLUDED.enabled, checked_at = EXCLUDED.checked_at,
			       check_ok = EXCLUDED.check_ok, check_error = EXCLUDED.check_error,
			       accepted_terms_at = EXCLUDED.accepted_terms_at,
			       tunnel_site_id = EXCLUDED.tunnel_site_id, tunnel_resource_id = EXCLUDED.tunnel_resource_id,
			       tunnel_newt_id = EXCLUDED.tunnel_newt_id, tunnel_newt_secret_enc = EXCLUDED.tunnel_newt_secret_enc,
			       local_endpoint = EXCLUDED.local_endpoint,
			       updated_by = EXCLUDED.updated_by, updated_at = now()
			RETURNING `+externalStorageCols,
			t.Slug, e.Kind, e.InstanceID, e.Endpoint, e.BasePath, e.Region, e.Bucket, e.UseSSL, e.AccessKey, e.SecretKeyEnc, e.Label, e.Enabled,
			e.CheckedAt, e.CheckOK, e.CheckError, e.AcceptedTermsAt, actor,
			e.TunnelSiteID, e.TunnelResourceID, e.TunnelNewtID, e.TunnelNewtSecretEnc, e.LocalEndpoint))
		return err
	})
	if err != nil {
		return ExternalStorage{}, fmt.Errorf("store: guardar almacén de terceros: %w", err)
	}
	return out, nil
}

// DeleteExternalStorage desconecta el almacén de terceros. Se niega mientras
// queden ficheros escritos allí (domain.ErrConflict): sin la configuración no
// habría forma de leerlos, y borrarlos por el usuario no es decisión nuestra.
//
// «Queden» es en TODA la organización: las unidades de equipo y los canales
// escriben en el mismo almacén, y una versión archivada también vive allí. En el
// ámbito de quien desconecta («Mi unidad») no se verían, y se desconectaría un
// almacén que otros siguen usando (ver conTodaLaOrganizacion).
func (s *Store) DeleteExternalStorage(ctx context.Context) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var quedan bool
		if err := conTodaLaOrganizacion(ctx, tx, func() error {
			return tx.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM files         WHERE tenant=$1 AND storage_key LIKE $2)
				    OR EXISTS (SELECT 1 FROM file_versions WHERE tenant=$1 AND storage_key LIKE $2)
				    OR EXISTS (SELECT 1 FROM cipher_blobs WHERE tenant=$1 AND storage_key LIKE $2)`,
				t.Slug, ExternalPrefix+"%").Scan(&quedan)
		}); err != nil {
			return fmt.Errorf("store: contar ficheros externos: %w", err)
		}
		if quedan {
			return domain.ErrConflict
		}
		tag, err := tx.Exec(ctx, `DELETE FROM external_storage WHERE tenant=$1`, t.Slug)
		if err != nil {
			return fmt.Errorf("store: desconectar almacén de terceros: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// Mapa de objetos externos (Google Drive): clave de blob → id del proveedor.
// Sin contexto de tenant (el enrutador de blobs corre también en trabajos de
// fondo), por eso el tenant viaja explícito y la transacción es de mantenimiento.

func (s *Store) PutExternalObject(ctx context.Context, tenant, key, providerID string) error {
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO external_objects (tenant, storage_key, provider_id) VALUES ($1, $2, $3)
			ON CONFLICT (tenant, storage_key) DO UPDATE SET provider_id = EXCLUDED.provider_id`, tenant, key, providerID)
		return err
	})
}

func (s *Store) GetExternalObject(ctx context.Context, tenant, key string) (string, error) {
	var id string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT provider_id FROM external_objects WHERE tenant=$1 AND storage_key=$2`, tenant, key).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return id, err
}

func (s *Store) DeleteExternalObject(ctx context.Context, tenant, key string) error {
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM external_objects WHERE tenant=$1 AND storage_key=$2`, tenant, key)
		return err
	})
}

// MarkExternalStorageChecked anota el resultado de una comprobación y, si fue
// bien, enciende el almacén: es lo que pasa cuando el túnel del NAS empieza a
// responder.
func (s *Store) MarkExternalStorageChecked(ctx context.Context, ok bool, msg string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE external_storage SET checked_at = now(), check_ok = $2, check_error = $3,
			enabled = (enabled OR $2), updated_at = now() WHERE tenant = $1`, t.Slug, ok, msg)
		return err
	})
}
