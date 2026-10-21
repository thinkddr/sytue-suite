-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Grupos DINÁMICOS (paridad Entra/Google dynamic groups): la membresía de un grupo
-- se CALCULA por una REGLA sobre atributos del usuario (department/title/location/
-- email/name) en vez de asignarse a mano. Extiende el modelo de grupos existente
-- (mig 0001), NO lo duplica: la membresía dinámica se MATERIALIZA en group_members
-- igual que la manual, de modo que GroupsForUser (→ claim `groups` del JWT → SCIM
-- saliente) la refleja SIN tocar ningún lector (ListGroupMembers, member_count, etc.).
--
-- MARCA DE GRUPO DINÁMICO: groups.membership_rule (jsonb). NULL ⇒ grupo MANUAL
-- (membresía asignada, como hasta ahora). NO NULL ⇒ grupo DINÁMICO: su membresía la
-- manda la regla y NO admite asignación manual (el servicio rechaza add/remove sobre
-- él). NO hace falta una marca por-fila en group_members: un grupo es dinámico O
-- manual (nunca mezcla, como en Entra/Google), luego «membership_rule IS NOT NULL» a
-- nivel de GRUPO es el único discriminador — una sola fuente de verdad, sin estado
-- redundante que sincronizar. El recómputo borra e inserta SOLO filas de grupos con
-- regla (gate IS NOT NULL); las mutaciones manuales se gatean con IS NULL. Sin índice
-- nuevo: los grupos dinámicos por org son decenas y el índice UNIQUE(org_id,name)
-- (mig 0001) ya sirve el prefijo org_id de «WHERE org_id=$1 AND membership_rule ...».
--
-- ATRIBUTOS DE USUARIO para las reglas: department/title/location (nullable). Son los
-- atributos canónicos de dynamic groups; se pueblan por el perfil admin y (a futuro)
-- por SCIM. email/name ya existían. Aislamiento por tenant idéntico al resto (mig
-- 0001/0019/0020): a nivel de aplicación, por org_id en cada consulta; el recómputo
-- solo evalúa usuarios CON membresía en la org (nunca cruza tenants).

ALTER TABLE groups ADD COLUMN membership_rule jsonb;

ALTER TABLE users ADD COLUMN department text;
ALTER TABLE users ADD COLUMN title      text;
ALTER TABLE users ADD COLUMN location   text;
