-- SPDX-License-Identifier: Apache-2.0
-- This is a dormant schema fence, not activation or evidence of migration.
-- A future backend must atomically verify the source, seal it, delete its rows,
-- and raise min_write_version to 4 before it can set the marker.
BEGIN;
ALTER TABLE notes ADD COLUMN private_links_sealed boolean NOT NULL DEFAULT false;
ALTER TABLE notes ADD CONSTRAINT notes_sealed_links_contract
    CHECK (NOT private_links_sealed OR (min_write_version >= 4 AND content_cipher <> ''));

CREATE FUNCTION notes_guard_sealed_links() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.private_links_sealed AND NOT NEW.private_links_sealed THEN
        RAISE EXCEPTION 'sealed note links cannot be downgraded' USING ERRCODE = '55000', CONSTRAINT = 'notes_sealed_links';
    END IF;
    IF NEW.private_links_sealed AND NOT OLD.private_links_sealed
       AND EXISTS (SELECT 1 FROM note_links WHERE tenant = NEW.tenant AND note_id = NEW.id) THEN
        RAISE EXCEPTION 'sealed note still has clear links' USING ERRCODE = '55000', CONSTRAINT = 'notes_sealed_links';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER notes_sealed_links_guard BEFORE UPDATE OF private_links_sealed ON notes
    FOR EACH ROW EXECUTE FUNCTION notes_guard_sealed_links();
ALTER TABLE notes ENABLE ALWAYS TRIGGER notes_sealed_links_guard;

-- Lock both source and destination parents in stable order, including DELETE.
-- The future CAS can then compare a complete snapshot while holding the parent.
-- During cascade the deleted parent may already be invisible; DELETE remains
-- allowed because it only removes cleartext and is required for retention.
CREATE FUNCTION notes_guard_link_child() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    old_parent uuid;
    new_parent uuid;
    seen_old boolean := false;
    seen_new boolean := false;
    parent record;
BEGIN
    IF TG_OP <> 'INSERT' THEN old_parent := OLD.note_id; END IF;
    IF TG_OP <> 'DELETE' THEN new_parent := NEW.note_id; END IF;
    -- Tenant merges can move parent and child in separate statements inside
    -- one authorized transaction. RLS restricts ordinary callers; globally
    -- unique ids let authorized merges operate in either order.
    FOR parent IN SELECT id, private_links_sealed FROM notes
        WHERE id = old_parent OR id = new_parent ORDER BY id FOR UPDATE
    LOOP
        IF parent.id = old_parent THEN seen_old := true; END IF;
        IF parent.id = new_parent THEN
            seen_new := true;
            IF parent.private_links_sealed THEN
                RAISE EXCEPTION 'clear links are not allowed for sealed notes' USING ERRCODE = '55000', CONSTRAINT = 'notes_sealed_links';
            END IF;
        END IF;
    END LOOP;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    IF NOT seen_new OR (TG_OP = 'UPDATE' AND NOT seen_old) THEN
        RAISE EXCEPTION 'link parent is not visible' USING ERRCODE = '55000', CONSTRAINT = 'notes_sealed_links';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER notes_sealed_link_child_guard BEFORE INSERT OR UPDATE OR DELETE ON note_links
    FOR EACH ROW EXECUTE FUNCTION notes_guard_link_child();
ALTER TABLE note_links ENABLE ALWAYS TRIGGER notes_sealed_link_child_guard;
COMMIT;
