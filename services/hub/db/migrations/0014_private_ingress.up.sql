-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Entrada privada de avisos de One (ADR 0086). Mientras la fuente de una persona no
-- es `legacy`, un aviso que deja un servicio no se escribe en `notifications`: va a
-- la cola de entrada compartida (las mismas tablas y reglas que Mail y Calendar),
-- cifrado con la clave de entrada de su época, y un aparato lo reclama, lo guarda en
-- su almacén privado y lo confirma. Con eso la guarda de escritura (0015) ya puede
-- cerrar `notifications` sin tirar ningún aviso.
--
-- La DDL compartida de la entrada se expande aquí (pg.WithPrivateIngressDDL):
-- sytue:private-ingress-v1
-- sytue:private-ingress-bodies-v1
-- sytue:private-ingress-handoff-v1
-- sytue:private-ingress-rejections-v1
-- sytue:private-ingress-epoch-v1
-- sytue:private-ingress-pq-v1
-- sytue:private-ingress-rotation-v1

-- En la base del Hub solo hay un tipo de entrega: el aviso. La restricción
-- compartida nombra los de Mail y Calendar; se cambia por la propia.
DO $$
DECLARE c text;
BEGIN
 FOR c IN SELECT conname FROM pg_constraint
           WHERE conrelid = 'private_ingress_tasks'::regclass AND contype = 'c'
             AND pg_get_constraintdef(oid) LIKE '%mail.receive%'
 LOOP
  EXECUTE format('ALTER TABLE private_ingress_tasks DROP CONSTRAINT %I', c);
 END LOOP;
END $$;
ALTER TABLE private_ingress_tasks ADD CONSTRAINT private_ingress_tasks_hub_kind
 CHECK (product = 'hub' AND kind = 'hub.receive');

-- La admisión de ámbito se instaló en la 0011, antes que estas tablas: se registran
-- ahora las que la DDL compartida no registró por sí misma.
DO $$
DECLARE tab record;
BEGIN
 FOR tab IN SELECT * FROM sytue_scope_admission_tables() s
             WHERE NOT EXISTS (SELECT 1 FROM pg_trigger t WHERE t.tgrelid = s.scope_table
                                AND t.tgname = 'scope_bound_admission')
 LOOP
  PERFORM sytue_scope_admission_register(tab.scope_table::regclass, tab.state_only);
 END LOOP;
END $$;

-- Copia del aviso mientras ningún aparato lo ha confirmado. Es lo que garantiza que
-- no se pierde ninguno:
--   · sin clave de entrada todavía (`sealed_at` NULL), es la cola: en cuanto la
--     persona da de alta una, se cifra para ella;
--   · cifrado pero sin confirmar, es la reserva: si la persona suelta la fuente
--     (vuelve a `legacy`), estos avisos vuelven a `notifications` y no se quedan
--     cifrados para una clave que ya no abre nadie;
--   · al confirmarlo un aparato (ACK de su entrega), la copia se borra en la misma
--     transacción: desde ahí solo existe cifrado.
-- No es parte de la fuente congelada (no la captura hub_freeze_private_storage ni la
-- toca la guarda) y lleva los mismos datos que `notifications` en legacy. La lista de
-- avisos de la API la enseña junto a `notifications`: la campana web no deja de ver
-- los avisos nuevos de quien tiene la fuente congelada.
CREATE TABLE hub_private_notices (
 id         uuid        PRIMARY KEY,
 tenant     text        NOT NULL,
 subject    text        NOT NULL CHECK (subject <> '' AND octet_length(subject) <= 1024 AND subject NOT LIKE 'email:%'),
 epoch      uuid        NOT NULL,
 source     text        NOT NULL,
 type       text        NOT NULL,
 level      text        NOT NULL CHECK (level IN ('info', 'success', 'warning', 'error')),
 title      text        NOT NULL CHECK (length(title) > 0),
 body       text        NOT NULL DEFAULT '',
 url        text        NOT NULL DEFAULT '',
 params     jsonb       NOT NULL DEFAULT '{}'::jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 sealed_at  timestamptz,
 -- Leído en la campana web mientras espera (la web no abre el almacén privado). Solo
 -- es la vista del servidor: no viaja cifrado, y vuelve con el aviso al soltar la fuente.
 read_at    timestamptz
);
CREATE INDEX hub_private_notices_owner ON hub_private_notices (tenant, subject, epoch, created_at, id);
CREATE INDEX hub_private_notices_recent ON hub_private_notices (subject, created_at);

ALTER TABLE hub_private_notices ENABLE ROW LEVEL SECURITY;
ALTER TABLE hub_private_notices FORCE ROW LEVEL SECURITY;
CREATE POLICY hub_private_notices_tenant_isolation ON hub_private_notices
 USING (tenant = current_setting('app.tenant', true))
 WITH CHECK (tenant = current_setting('app.tenant', true));
-- El tope por persona en todas sus organizaciones (store.DeLaPersona) los cuenta.
CREATE POLICY hub_private_notices_destinatario_lee ON hub_private_notices FOR SELECT
 USING (subject = ANY (string_to_array(nullif(current_setting('app.destinatarios', true), ''), chr(31))));
-- Y la persona los marca leídos desde la campana de cualquiera de sus organizaciones.
CREATE POLICY hub_private_notices_destinatario_marca ON hub_private_notices FOR UPDATE
 USING (subject = ANY (string_to_array(nullif(current_setting('app.destinatarios', true), ''), chr(31))))
 WITH CHECK (subject = ANY (string_to_array(nullif(current_setting('app.destinatarios', true), ''), chr(31))));
-- Fusión de organizaciones y baja de la cuenta (services/id merge.BorrarPersona).
CREATE POLICY hub_private_notices_platform_merge ON hub_private_notices
 USING (current_setting('app.merge', true) = 'on')
 WITH CHECK (current_setting('app.merge', true) = 'on');
