// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"io/fs"
	"path"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// La 0023 tal como salió en la v2.81.0, solo con S3: lo que producción tenía
// aplicado cuando la 0023 se reescribió (4d8719ab5).
const external0023Original = `
CREATE TABLE external_storage (
    tenant     text PRIMARY KEY,
    endpoint   text NOT NULL,
    region     text NOT NULL DEFAULT '',
    bucket     text NOT NULL,
    use_ssl    boolean NOT NULL DEFAULT true,
    access_key text NOT NULL,
    secret_key_enc text NOT NULL,
    label      text NOT NULL DEFAULT '',
    enabled    boolean NOT NULL DEFAULT true,
    checked_at  timestamptz,
    check_ok    boolean NOT NULL DEFAULT false,
    check_error text    NOT NULL DEFAULT '',
    updated_by text        NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT external_storage_endpoint_not_blank CHECK (length(endpoint) > 0),
    CONSTRAINT external_storage_bucket_not_blank   CHECK (length(bucket) > 0)
);
ALTER TABLE external_storage ENABLE ROW LEVEL SECURITY;
ALTER TABLE external_storage FORCE ROW LEVEL SECURITY;
CREATE POLICY external_storage_isolation ON external_storage
    USING (tenant = current_setting('app.tenant', true))
    WITH CHECK (tenant = current_setting('app.tenant', true));`

// LO QUE SE PRUEBA: que la 0024 deja utilizable una base con la 0023 ORIGINAL y
// una conexión S3 guardada —el estado de producción del 2026-09-15, donde leerla
// daba «column "kind" does not exist»—, que repetirla no rompe nada y que en una
// base nueva, que ya la aplicó `pgtest.New` sobre la 0023 completa, no estorba.
func TestExternalStorageReparaLa0023Original(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(ctx) })
	reparar, err := fs.ReadFile(db.Migrations, path.Join(db.MigrationsDir, "0024_external_storage_reparar.up.sql"))
	if err != nil {
		t.Fatal(err)
	}

	for _, sql := range []string{
		`DROP TABLE external_objects; DROP TABLE external_storage;`,
		external0023Original,
		`INSERT INTO external_storage (tenant, endpoint, bucket, access_key, secret_key_enc)
		 VALUES ('acme', 's3.example.eu', 'copias', 'AK', 'sellado')`,
		string(reparar),
		string(reparar),
	} {
		if _, err := admin.Exec(ctx, sql); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	// Later code also reads the pinned physical location. Replay that migration
	// after rebuilding the historical schema, as a production upgrade does.
	locations, err := fs.ReadFile(db.Migrations, path.Join(db.MigrationsDir, "0034_external_storage_locations.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, `DROP FUNCTION pin_external_storage_location(); DROP FUNCTION archive_external_storage_location(); DROP TABLE external_storage_locations;`); err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, string(locations)); err != nil {
		t.Fatal(err)
	}

	st := store.New(h.Pool)
	e, err := st.ExternalStorage(tenantCtx("acme"))
	if err != nil {
		t.Fatalf("leer la conexión guardada antes de la reescritura: %v", err)
	}
	if e.Kind != "s3" || e.Bucket != "copias" || e.SecretKeyEnc != "sellado" {
		t.Fatalf("la conexión de antes no se conserva: %+v", e)
	}
	if !e.AcceptedTermsAt.Equal(time.Unix(0, 0)) {
		t.Fatalf("sin aviso aceptado debe constar 'epoch', no una fecha inventada: %v", e.AcceptedTermsAt)
	}

	e.Kind, e.BasePath, e.AcceptedTermsAt = "webdav", "/copias", time.Now()
	if _, err := st.SaveExternalStorage(tenantCtx("acme"), e, "test"); err != nil {
		t.Fatalf("guardar con las columnas nuevas: %v", err)
	}
	// La 0031 completa la reparación para clases que no tienen bucket/endpoint.
	proveedores, err := fs.ReadFile(db.Migrations, path.Join(db.MigrationsDir, "0031_dropbox_storage.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, string(proveedores)); err != nil {
		t.Fatalf("ampliar los proveedores sobre la 0023 original reparada: %v", err)
	}
	e.Kind, e.InstanceID, e.Endpoint, e.Bucket = "dropbox", "cuenta-consentida", "", ""
	if _, err := st.SaveExternalStorage(tenantCtx("acme"), e, "test"); err != nil {
		t.Fatalf("la base original reparada debe admitir una cuenta por consentimiento: %v", err)
	}
}
