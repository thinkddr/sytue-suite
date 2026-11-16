-- SPDX-License-Identifier: Apache-2.0
DO $$ BEGIN RAISE EXCEPTION 'private ingress rejected intents must be retained' USING ERRCODE='55000'; END $$;
