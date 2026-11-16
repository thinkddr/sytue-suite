-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN RAISE EXCEPTION 'private ingress claims, fences and receipts must be retained' USING ERRCODE='55000'; END $$;
