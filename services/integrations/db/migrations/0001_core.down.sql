-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Revierte 0001_core: revoca/borra el rol de aplicación y suelta las tablas del
-- núcleo en orden inverso de dependencias (las políticas RLS caen con su tabla).

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'integrations_app') THEN
        EXECUTE 'REVOKE ALL ON ALL TABLES IN SCHEMA public FROM integrations_app';
        EXECUTE 'REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM integrations_app';
        EXECUTE 'REVOKE ALL ON SCHEMA public FROM integrations_app';
        EXECUTE 'DROP ROLE integrations_app';
    END IF;
END
$$;

DROP TABLE IF EXISTS platform_rate_limits;
DROP TABLE IF EXISTS events_outbox;
DROP TABLE IF EXISTS egress_audit;
DROP TABLE IF EXISTS instance_secrets;
DROP TABLE IF EXISTS app_instances;
DROP TABLE IF EXISTS app_templates;
