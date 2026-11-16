-- SPDX-License-Identifier: Apache-2.0
BEGIN;
LOCK TABLE organizations,org_renames,org_slug_claims IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM org_renames) OR EXISTS(SELECT 1 FROM org_slug_claims WHERE retired)
 OR EXISTS(SELECT 1 FROM org_slug_claims c LEFT JOIN organizations o ON o.id=c.org_id AND o.slug=c.slug WHERE o.id IS NULL) THEN
  RAISE EXCEPTION 'organization rename evidence must be retained; rollback refused' USING ERRCODE='55000';
 END IF;
END $$;
DROP TRIGGER organization_claim_slug ON organizations;
DROP FUNCTION organization_claim_slug();
DROP TABLE org_renames;
DROP FUNCTION org_rename_intent_guard();
DROP TABLE org_slug_claims;
DROP FUNCTION org_slug_claim_guard();
COMMIT;
