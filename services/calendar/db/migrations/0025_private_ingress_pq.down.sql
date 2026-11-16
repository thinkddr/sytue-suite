-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN RAISE EXCEPTION 'post-quantum ingress key history cannot be downgraded'; END $$;
