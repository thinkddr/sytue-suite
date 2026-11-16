-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN RAISE EXCEPTION 'private ingress ciphertext requires a verified preserving rollback' USING ERRCODE='55000'; END $$;
