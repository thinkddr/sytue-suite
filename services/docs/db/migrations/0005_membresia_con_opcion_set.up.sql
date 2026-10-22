-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Dar a la membresía de `docs_app` la opción SET, sin la cual el rol no
-- se puede asumir y el servicio no puede leer ni escribir NADA.
--
-- QUÉ PASÓ. Desde PostgreSQL 16, ser miembro de un rol ya no basta para hacerle
-- `SET ROLE`: la membresía lleva tres opciones separadas —ADMIN, INHERIT y SET—
-- y solo la última lo autoriza. El `GRANT docs_app TO <rol>` de 0001 se
-- escribió cuando esa distinción no existía, así que en producción quedó una
-- membresía con `set_option = false`.
--
-- El store abre CADA transacción con `SET LOCAL ROLE docs_app` —es lo que hace
-- que RLS se evalúe de verdad, porque las políticas no se aplican a un
-- superusuario—, de modo que la consecuencia no fue un permiso de menos en algún
-- sitio: fue que ninguna consulta llegaba a ejecutarse. En producción no se podía
-- crear ni abrir un documento, y en el log solo se veía «permission denied to set
-- role». Se descubrió persiguiendo el mismo fallo en Sytue Integrations, que
-- comparte este patrón de rol de aplicación (su migración 0011).
--
-- POR QUÉ NO SE VIO ANTES: en desarrollo y en CI las migraciones y el servicio
-- corren como superusuario, y un superusuario puede asumir cualquier rol sin
-- necesitar la opción. El fallo solo existe donde el rol de la aplicación NO es
-- superusuario, que es exactamente producción y exactamente donde importa.
--
-- INHERIT se deja como está, a propósito: que el rol NO se herede por defecto y
-- haya que asumirlo explícitamente es el diseño, no un descuido. Aquí solo se
-- concede poder asumirlo.
--
-- Idempotente y condicionada a la versión: `WITH SET TRUE` es sintaxis de
-- PostgreSQL 16 en adelante y una base anterior no la entiende, aunque tampoco
-- la necesite —antes de 16 la membresía ya autorizaba el SET ROLE—.

DO $$
BEGIN
    IF current_setting('server_version_num')::int >= 160000 THEN
        EXECUTE format('GRANT docs_app TO %I WITH SET TRUE', current_user);
    ELSE
        EXECUTE format('GRANT docs_app TO %I', current_user);
    END IF;
END $$;
