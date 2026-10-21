-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Los dominios de correo de una organización, en plural.
--
-- Hasta ahora cabía UNO: tres columnas en `org_profiles` (domain, domain_token,
-- domain_verified_at). Con eso, una empresa que tiene dos —el de siempre y el de
-- la marca que compró, o el .es y el .com que apuntan a la misma gente— no tenía
-- ninguna puerta: conectar el segundo significaba desconectar el primero, y con
-- él las direcciones que ya usaba su plantilla. El caso no es exótico, es lo
-- normal en cuanto una empresa pasa de cierto tamaño.
--
-- EL SUBDOMINIO DE LA SUITE NO VIVE AQUÍ. `<slug>.sytue.eu` no es un dominio
-- conectado sino una propiedad de la organización: existe siempre, no se verifica
-- —es nuestro— y no se puede quitar (ver domain.OrgBaseMailDomain). Guardarlo
-- como una fila más obligaría a defenderlo con reglas en cada operación de esta
-- tabla, y la primera que se olvidara dejaría a una organización sin ninguna
-- dirección en un dominio que servimos nosotros.
--
-- UNO ES EL PRINCIPAL, y es el que estrenan las altas nuevas. Los demás sirven
-- para recibir y para enviar-como: una dirección en cualquiera de ellos entrega
-- en el buzón de su dueño. Cambiar cuál es el principal muda a toda la plantilla
-- (PromoteOrgMailDomain) y conserva la anterior como alias, igual que hacía
-- conectar el primero.
--
-- Sin RLS, como el resto de tablas de este servicio: aquí la tenencia no se
-- filtra por política de fila, la resuelve el propio ID, que es quien la define.
CREATE TABLE org_mail_domains (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- Siempre en minúsculas y ya normalizado por domain.NormalizeDomain: el CHECK
    -- es la última barrera para que dos filas no digan lo mismo con distinta caja
    -- y se salten los índices únicos de abajo.
    domain      text NOT NULL CHECK (domain <> '' AND domain = lower(domain)),
    -- El valor del registro TXT que prueba la titularidad. Se emite al reclamar y
    -- se conserva mientras el dominio siga reclamado: reintentar la comprobación
    -- no puede invalidar el registro que el cliente ya publicó.
    token       text NOT NULL DEFAULT '',
    verified_at timestamptz,
    is_primary  boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Una organización no reclama dos veces el mismo dominio.
CREATE UNIQUE INDEX org_mail_domains_org_idx ON org_mail_domains (org_id, domain);

-- Un dominio VERIFICADO pertenece a UNA sola organización. Cubre solo lo
-- verificado a propósito, como hacía el índice de org_profiles: reclamar no
-- prueba nada —cualquiera escribe «acme.com» en un formulario—, así que una
-- reclamación sin verificar NO puede bloquear al dueño real, que sería un
-- secuestro trivial. Quien publica el TXT gana.
CREATE UNIQUE INDEX org_mail_domains_verified_idx ON org_mail_domains (domain)
    WHERE verified_at IS NOT NULL;

-- Y un solo principal por organización: es el que decide dónde nacen las
-- direcciones nuevas, y dos a la vez no significan nada.
CREATE UNIQUE INDEX org_mail_domains_primary_idx ON org_mail_domains (org_id)
    WHERE is_primary;

CREATE TRIGGER org_mail_domains_set_updated_at BEFORE UPDATE ON org_mail_domains
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE org_mail_domains IS
    'Dominios de correo propios de cada organización. El principal es el que estrenan las altas; el subdominio de la suite no vive aquí porque existe siempre.';

-- Los que ya estaban conectados pasan a ser el principal de su organización. Un
-- dominio reclamado y todavía sin verificar se copia igual —para no perder el
-- token que el cliente puede estar publicando ahora mismo— pero sin ser
-- principal: principal solo lo es un dominio del que consta la titularidad.
INSERT INTO org_mail_domains (org_id, domain, token, verified_at, is_primary)
SELECT org_id, lower(domain), domain_token, domain_verified_at, domain_verified_at IS NOT NULL
FROM org_profiles
WHERE domain <> '';

-- LAS COLUMNAS VIEJAS DE `org_profiles` SE QUEDAN, y esto es un expand a medias
-- a propósito. Ya no las escribe ni las lee nadie —OrgProfile compone esos tres
-- campos con el dominio PRINCIPAL de esta tabla—, pero durante el rollout la
-- versión ANTERIOR del servicio sigue viva y consultándolas, y lo mismo pasa tras
-- un rollback de Helm: borrarlas aquí la tumbaría.
--
-- El contract —`DROP COLUMN domain, domain_token, domain_verified_at` y su
-- índice— va en una migración POSTERIOR, cuando la versión que las usaba ya no
-- corra en ninguna parte. Hasta entonces se quedan congeladas con el valor que
-- tuvieran: si alguien conecta un dominio durante la ventana del rollout, la
-- versión vieja no lo verá, que es la asimetría que este patrón acepta a cambio
-- de no cortar el servicio.
