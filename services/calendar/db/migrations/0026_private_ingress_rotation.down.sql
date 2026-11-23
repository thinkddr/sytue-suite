-- SPDX-License-Identifier: Apache-2.0
-- La autoridad de cada plan de entrada es su historial: no se borra para bajar.
DO $$ BEGIN RAISE EXCEPTION 'private ingress key rotation history cannot be downgraded'; END $$;
