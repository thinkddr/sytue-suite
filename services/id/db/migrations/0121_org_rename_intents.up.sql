-- SPDX-License-Identifier: Apache-2.0
-- Retain the original identity and reserve both slugs before any domain moves.
CREATE TABLE org_slug_claims (
 slug text PRIMARY KEY,
 -- Historical claims survive physical removal of a disposable organization.
 org_id uuid NOT NULL,
 retired boolean NOT NULL DEFAULT false
);
INSERT INTO org_slug_claims(slug,org_id) SELECT slug,id FROM organizations;
CREATE TABLE org_renames (
 id uuid PRIMARY KEY,
 org_id uuid NOT NULL REFERENCES organizations(id),
 source_slug text NOT NULL,
 target_slug text NOT NULL,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN('pending','identity_committed','completed')),
 CHECK(source_slug<>target_slug),
 UNIQUE(source_slug,target_slug)
);
CREATE UNIQUE INDEX org_renames_open_org ON org_renames(org_id) WHERE status<>'completed';
CREATE FUNCTION org_slug_claim_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'organization slug claim is retained' USING ERRCODE='55000'; END IF;
 IF TG_OP='UPDATE' AND (OLD.slug IS DISTINCT FROM NEW.slug OR OLD.org_id IS DISTINCT FROM NEW.org_id OR (OLD.retired AND NOT NEW.retired)) THEN
  RAISE EXCEPTION 'organization slug claim is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_slug_claim_guard BEFORE UPDATE OR DELETE ON org_slug_claims FOR EACH ROW EXECUTE FUNCTION org_slug_claim_guard();
CREATE FUNCTION organization_claim_slug() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE claimed text;
BEGIN
 -- ON CONFLICT checks the current unique row, also for a stale transaction.
 -- Merely SELECTing a reservation could miss a concurrently committed intent.
 INSERT INTO org_slug_claims(slug,org_id) VALUES(NEW.slug,NEW.id)
 ON CONFLICT(slug) DO UPDATE SET slug=EXCLUDED.slug
 WHERE org_slug_claims.org_id=EXCLUDED.org_id AND NOT org_slug_claims.retired
 RETURNING slug INTO claimed;
 IF claimed IS NULL THEN RAISE EXCEPTION 'organization slug is reserved or retired' USING ERRCODE='23505'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER organization_claim_slug BEFORE INSERT OR UPDATE OF slug ON organizations FOR EACH ROW EXECUTE FUNCTION organization_claim_slug();
CREATE FUNCTION org_rename_intent_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE claimed text;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'organization rename intent is retained' USING ERRCODE='55000'; END IF;
 IF TG_OP='INSERT' THEN
  IF NEW.status<>'pending' THEN RAISE EXCEPTION 'organization rename must start pending' USING ERRCODE='55000'; END IF;
  PERFORM 1 FROM organizations WHERE id=NEW.org_id AND slug=NEW.source_slug AND status NOT IN('merged','deleted') FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'organization rename source changed' USING ERRCODE='55000'; END IF;
  PERFORM 1 FROM org_slug_claims WHERE slug=NEW.source_slug AND org_id=NEW.org_id AND NOT retired FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'organization rename source claim missing' USING ERRCODE='55000'; END IF;
  INSERT INTO org_slug_claims(slug,org_id) VALUES(NEW.target_slug,NEW.org_id)
  ON CONFLICT(slug) DO UPDATE SET slug=EXCLUDED.slug
  WHERE org_slug_claims.org_id=EXCLUDED.org_id AND NOT org_slug_claims.retired
  RETURNING slug INTO claimed;
  IF claimed IS NULL THEN RAISE EXCEPTION 'organization rename target is reserved' USING ERRCODE='23505'; END IF;
 ELSE
  IF (OLD.id,OLD.org_id,OLD.source_slug,OLD.target_slug) IS DISTINCT FROM (NEW.id,NEW.org_id,NEW.source_slug,NEW.target_slug)
  OR NOT ((OLD.status='pending' AND NEW.status='identity_committed') OR (OLD.status='identity_committed' AND NEW.status='completed')) THEN
   RAISE EXCEPTION 'organization rename intent is immutable' USING ERRCODE='55000';
  END IF;
  PERFORM 1 FROM organizations WHERE id=NEW.org_id AND slug=NEW.target_slug FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'organization rename identity not committed' USING ERRCODE='55000'; END IF;
  PERFORM 1 FROM org_slug_claims WHERE slug=NEW.source_slug AND org_id=NEW.org_id AND retired;
  IF NOT FOUND THEN RAISE EXCEPTION 'organization rename source not retired' USING ERRCODE='55000'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER org_rename_intent_guard BEFORE INSERT OR UPDATE OR DELETE ON org_renames FOR EACH ROW EXECUTE FUNCTION org_rename_intent_guard();
