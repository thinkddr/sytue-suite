-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
-- A backend rollback must not restore permissive note writers. This records a
-- write contract, not proof of encryption and not a ciphertext format version.
-- The constant DEFAULT applies to existing rows too, without reading tenant
-- data or weakening RLS. DDL locks serialize with writers already in flight.
BEGIN;

ALTER TABLE notebooks ADD COLUMN min_write_version smallint NOT NULL DEFAULT 2
    CHECK (min_write_version >= 2);
ALTER TABLE notes ADD COLUMN min_write_version smallint NOT NULL DEFAULT 2
    CHECK (min_write_version >= 2);

CREATE FUNCTION notes_require_write_contract() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    required integer := 2;
    marker text := current_setting('app.notes_write_version', true);
BEGIN
    IF TG_OP <> 'INSERT' THEN
        required := GREATEST(required, OLD.min_write_version);
    END IF;
    IF TG_OP <> 'DELETE' THEN
        required := GREATEST(required, NEW.min_write_version);
        IF TG_OP = 'UPDATE' AND NEW.min_write_version < OLD.min_write_version THEN
            RAISE EXCEPTION 'notes write contract cannot decrease'
                USING ERRCODE = '55000', CONSTRAINT = 'notes_write_contract';
        END IF;
    END IF;
    -- Missing, empty and malformed settings all fail closed. Bound the number
    -- before casting; a pool reset leaves an empty custom setting, not NULL.
    IF marker IS NULL OR marker !~ '^[1-9][0-9]{0,4}$' THEN
        RAISE EXCEPTION 'notes backend write contract is required'
            USING ERRCODE = '55000', CONSTRAINT = 'notes_write_contract';
    END IF;
    IF marker::integer < required THEN
        RAISE EXCEPTION 'notes backend write contract is too old'
            USING ERRCODE = '55000', CONSTRAINT = 'notes_write_contract';
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER notebooks_write_contract BEFORE INSERT OR UPDATE OR DELETE ON notebooks
    FOR EACH ROW EXECUTE FUNCTION notes_require_write_contract();
CREATE TRIGGER notes_write_contract BEFORE INSERT OR UPDATE OR DELETE ON notes
    FOR EACH ROW EXECUTE FUNCTION notes_require_write_contract();
ALTER TABLE notebooks ENABLE ALWAYS TRIGGER notebooks_write_contract;
ALTER TABLE notes ENABLE ALWAYS TRIGGER notes_write_contract;

COMMIT;
