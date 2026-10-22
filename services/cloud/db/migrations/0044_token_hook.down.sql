-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos

DELETE FROM api_tokens WHERE scope = 'hook';
ALTER TABLE api_tokens DROP CONSTRAINT api_tokens_scope_check;
ALTER TABLE api_tokens ADD CONSTRAINT api_tokens_scope_check
    CHECK (scope IN ('read', 'write', 'deploy'));
