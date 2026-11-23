-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- El traspaso de la autoridad del correo de Matrix a la bóveda (ADR 0087).
--
-- Quien ya no puede abrir su correo de Matrix registra en Sytue ID una clave de
-- bóveda con el motivo `traspaso` (siempre con identificación reciente). Cuando
-- Rooms la ve activa y posterior al último cambio de la autoridad de Matrix, deja
-- aquí constancia, UNA vez y para siempre: desde entonces la autoridad del ámbito
-- es la de la bóveda, aunque la clave cambie después, y las pruebas de Matrix del
-- ámbito se rechazan. La fila de Matrix no se toca: es el historial.
--
-- `matrix_generation` es la generación de Matrix que había al traspasar. La de la
-- bóveda se cuenta encima (matrix_generation + generación en Sytue ID), para que
-- la generación que ve el correo nunca baje (la bóveda empieza en 1; Matrix puede
-- ir por 5). `key_id`/`key_generation` son la clave del traspaso, para auditar.
CREATE TABLE matrix_storage_vault_handovers (
 tenant text NOT NULL,
 subject text NOT NULL,
 product text NOT NULL CHECK(product='mail'),
 matrix_generation bigint NOT NULL CHECK(matrix_generation>0 AND matrix_generation<4503599627370496),
 key_generation bigint NOT NULL CHECK(key_generation>0 AND key_generation<4503599627370496),
 key_id text NOT NULL CHECK(key_id ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant,subject,product),
 FOREIGN KEY(tenant,subject,product) REFERENCES matrix_storage_authorities(tenant,subject,product)
);
ALTER TABLE matrix_storage_vault_handovers ENABLE ROW LEVEL SECURITY;
ALTER TABLE matrix_storage_vault_handovers FORCE ROW LEVEL SECURITY;
CREATE POLICY matrix_storage_vault_handovers_owner ON matrix_storage_vault_handovers
 USING(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true))
 WITH CHECK(tenant=current_setting('app.tenant',true) AND subject=current_setting('app.matrix_authority_owner',true) AND product=current_setting('app.matrix_authority_product',true));
CREATE FUNCTION matrix_storage_vault_handover_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'vault handover is retained and immutable' USING ERRCODE='55000'; END IF;
 IF current_setting('app.tenant',true) IS DISTINCT FROM NEW.tenant
 OR current_setting('app.matrix_authority_owner',true) IS DISTINCT FROM NEW.subject
 OR current_setting('app.matrix_authority_product',true) IS DISTINCT FROM NEW.product
 OR coalesce(current_setting('app.merge',true),'') NOT IN('','false','off','0') THEN
  RAISE EXCEPTION 'vault handover scope mismatch' USING ERRCODE='42501';
 END IF;
 -- Se cuenta sobre la generación de Matrix de ESE momento, nunca sobre otra.
 IF NOT EXISTS(SELECT 1 FROM matrix_storage_authorities a WHERE a.tenant=NEW.tenant AND a.subject=NEW.subject AND a.product=NEW.product AND a.generation=NEW.matrix_generation) THEN
  RAISE EXCEPTION 'vault handover generation mismatch' USING ERRCODE='42501';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER matrix_storage_vault_handover_guard BEFORE INSERT OR UPDATE OR DELETE ON matrix_storage_vault_handovers
 FOR EACH ROW EXECUTE FUNCTION matrix_storage_vault_handover_guard();
SELECT sytue_scope_merge_register('matrix_storage_vault_handovers'::regclass);
SELECT sytue_scope_admission_register('matrix_storage_vault_handovers'::regclass,false);
