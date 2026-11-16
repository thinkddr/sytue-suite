-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN RAISE EXCEPTION 'private ingress epoch history cannot be downgraded'; END $$;
