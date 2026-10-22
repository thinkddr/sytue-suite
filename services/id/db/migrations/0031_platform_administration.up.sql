-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- ADMINISTRACIÓN DE PLATAFORMA. Hasta aquí el modelo NO tenía super-admin: cada
-- decisión de autorización se anclaba a una organización y ningún rol veía más de
-- una (`admin_tenants.go`, `store/orgs.go`). Esa propiedad se conserva para todos
-- los roles de organización y se abre UNA puerta nueva, explícita y estrecha, para
-- que soporte y dirección puedan operar la plataforma: dar de alta y suspender
-- organizaciones, fusionarlas, medir uso y regalar plazas.
--
-- La puerta es una allowlist propia (`platform_admins`), no un rol de organización:
-- así el robo de una sesión de admin de un cliente sigue sin conceder nada global,
-- y quién tiene plataforma se responde con un SELECT de una tabla, no razonando
-- sobre pertenencias. Ninguna API de organización la escribe.

-- 1) Admins de plataforma -----------------------------------------------------
-- MFA: no se replica aquí ninguna política. El rol de organización ya exige
-- step-up (`requiresStepUp`) y la API de plataforma lo exigirá SIEMPRE, tenga o no
-- rol elevado el usuario: es la superficie más poderosa del producto.
CREATE TABLE platform_admins (
    user_id     uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    -- Quién la concedió. NULL = el bootstrap del despliegue (no hay un humano
    -- anterior al primero). No es FK a users para que borrar a un admin no
    -- reescriba la procedencia de los que él concedió.
    granted_by  uuid,
    reason      text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER platform_admins_set_updated_at BEFORE UPDATE ON platform_admins
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 2) Plazas concedidas --------------------------------------------------------
-- Gestos comerciales y plazas de empleados de Sytue. NO se toca `seats`: lo
-- vendido y lo regalado se cuentan aparte para que la factura siga siendo
-- `seats × unit_price_cents` y para poder responder qué se regaló, a quién, por
-- qué y hasta cuándo. El aforo efectivo es `seats + Σ(concesiones vigentes)`.
--
-- Tabla y no columna: una organización acumula varias concesiones con motivos y
-- caducidades distintas, y una columna las aplanaría perdiendo justo la
-- trazabilidad que un descuento manual necesita.
CREATE TABLE seat_grants (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    seats       integer NOT NULL CHECK (seats > 0 AND seats <= 10000),
    -- Por qué se concedió. Obligatorio y no vacío: una plaza gratis sin motivo es
    -- indistinguible de un error de facturación seis meses después.
    reason      text NOT NULL CHECK (length(btrim(reason)) > 0),
    -- Clasificación para poder separar coste comercial de coste interno.
    kind        text NOT NULL DEFAULT 'goodwill'
                CHECK (kind IN ('goodwill', 'employee', 'trial', 'migration')),
    granted_by  uuid NOT NULL REFERENCES users (id),
    -- NULL = sin caducidad (el caso de un empleado).
    expires_at  timestamptz,
    revoked_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Las consultas de aforo solo miran concesiones vigentes de una organización.
CREATE INDEX seat_grants_org_vigentes_idx ON seat_grants (org_id)
    WHERE revoked_at IS NULL;

CREATE TRIGGER seat_grants_set_updated_at BEFORE UPDATE ON seat_grants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 3) Fusión de organizaciones -------------------------------------------------
-- La fusión mueve identidad Y datos de los diez servicios, así que no es un UPDATE:
-- es un proceso largo, por servicio, reintentable y auditable. Su estado vive aquí
-- para que se pueda reanudar tras un fallo y para que nadie lance dos fusiones
-- solapadas sobre la misma organización.
CREATE TABLE org_merges (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_org_id   uuid NOT NULL REFERENCES organizations (id),
    target_org_id   uuid NOT NULL REFERENCES organizations (id),
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'aborted')),
    requested_by    uuid NOT NULL REFERENCES users (id),
    -- Progreso por servicio: {"mail":"completed","drive":"running",...}. Permite
    -- reanudar sin repetir lo ya movido y decir exactamente dónde se quedó.
    services        jsonb NOT NULL DEFAULT '{}'::jsonb,
    error           text NOT NULL DEFAULT '',
    started_at      timestamptz,
    finished_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT org_merges_distintas CHECK (source_org_id <> target_org_id)
);

-- Una organización no puede estar en dos fusiones abiertas a la vez, ni como
-- origen ni como destino: dos procesos moviendo las mismas filas dejarían datos a
-- medio camino sin forma de saber de quién son.
CREATE UNIQUE INDEX org_merges_origen_abierta_idx ON org_merges (source_org_id)
    WHERE status IN ('pending', 'running');
CREATE UNIQUE INDEX org_merges_destino_abierta_idx ON org_merges (target_org_id)
    WHERE status IN ('pending', 'running');

CREATE TRIGGER org_merges_set_updated_at BEFORE UPDATE ON org_merges
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Estado terminal de la organización absorbida. `merged` es un estado propio y no
-- un `suspended` con una nota: una suspensión se revierte y una fusión no, y el
-- login debe poder distinguirlas para decir al usuario a dónde ir.
ALTER TABLE organizations
    ADD COLUMN merged_into_org_id uuid REFERENCES organizations (id),
    ADD COLUMN merged_at timestamptz;

-- DROP + ADD del mismo nombre en la misma migración: amplía el dominio sin perder
-- la comprobación (el binario anterior nunca escribe 'merged', así que conviven).
ALTER TABLE organizations DROP CONSTRAINT organizations_status_check;
ALTER TABLE organizations ADD CONSTRAINT organizations_status_check
    CHECK (status IN ('active', 'suspended', 'deleted', 'merged'));

-- Coherencia: o está fusionada del todo, o no lo está. Un destino a medias sin
-- fecha (o al revés) es el estado en el que nadie sabe si la fusión ocurrió.
ALTER TABLE organizations ADD CONSTRAINT organizations_merge_coherente
    CHECK ((merged_into_org_id IS NULL) = (merged_at IS NULL));

-- El primer admin de plataforma NO se siembra aquí: en una instalación nueva
-- todavía no existe ningún usuario al que apuntar (`platform_admins.user_id` es
-- FK). Lo hace `id bootstrap-owner`, que crea el usuario y lo inscribe en la misma
-- transacción; su correo por defecto es admin@sytue.eu.
