-- Shared hybrid post-quantum recipient support. Existing classical recipients
-- remain immutable and readable; enabling this profile never rewrites history.
ALTER TABLE private_ingress_key_plans
 DROP CONSTRAINT private_ingress_key_plans_profile_check,
 DROP CONSTRAINT private_ingress_key_plans_public_recipient_check,
 ADD CONSTRAINT private_ingress_key_plans_profile_check
 CHECK (profile IN ('age-X25519-v1','age-MLKEM768-X25519-v1')),
 ADD CONSTRAINT private_ingress_key_plans_public_recipient_check
 CHECK (octet_length(public_recipient) BETWEEN 1 AND
  CASE WHEN profile='age-X25519-v1' THEN 256 ELSE 4096 END);
