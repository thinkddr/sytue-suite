DROP TRIGGER mail_private_aliases_guard ON aliases;
CREATE TRIGGER mail_private_aliases_guard BEFORE INSERT OR UPDATE OR DELETE ON aliases
 FOR EACH ROW EXECUTE FUNCTION mail_private_storage_row_guard();
DROP FUNCTION mail_private_alias_routing_guard();
