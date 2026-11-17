-- SPDX-License-Identifier: Apache-2.0
-- Preserve original ciphertext, receipts and signed authorization history.
DO $$ BEGIN RAISE EXCEPTION 'private ingress requires a verified ciphertext-preserving rollback' USING ERRCODE='55000'; END $$;
