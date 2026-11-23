-- Rotación de la clave de entrada de una época ya dada de alta (ADR 0087).
--
-- Cada plan guarda la autoridad (Rooms) que lo autorizó. Un plan de generación
-- siguiente solo entra si lo autoriza una autoridad de bóveda POSTERIOR a la del
-- plan vigente: tras un traspaso de Matrix a la bóveda, o tras vaciar la bóveda.
-- Los planes de antes de esta migración quedan sin autoridad (NULL): solo los
-- sucede una autoridad de bóveda. Nada se reescribe ni se borra: las entregas
-- cifradas para el plan anterior siguen en la cola tal cual.
ALTER TABLE private_ingress_key_plans
 ADD COLUMN IF NOT EXISTS authority_user text,
 ADD COLUMN IF NOT EXISTS authority_generation bigint;
ALTER TABLE private_ingress_key_plans
 ADD CONSTRAINT private_ingress_key_plans_authority_check
 CHECK((authority_user IS NULL AND authority_generation IS NULL)
  OR (octet_length(authority_user) BETWEEN 3 AND 255 AND authority_generation>0 AND authority_generation<9007199254740992));

CREATE OR REPLACE FUNCTION sytue_private_ingress_key_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (OLD.key_generation,OLD.key_id,OLD.profile,OLD.public_recipient,OLD.created_at,OLD.authority_user,OLD.authority_generation) IS DISTINCT FROM (NEW.key_generation,NEW.key_id,NEW.profile,NEW.public_recipient,NEW.created_at,NEW.authority_user,NEW.authority_generation)
 OR (OLD.revoked_at IS NOT NULL AND OLD.revoked_at IS DISTINCT FROM NEW.revoked_at) THEN
  RAISE EXCEPTION 'private ingress key plan is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
