#!/bin/sh
# ---------------------------------------------------------------------------
# Sytue One dev — bootstrap de PostgreSQL 16.
# Crea una BD + rol dedicado por CADA servicio Go de la suite (aislamiento por
# servicio; multi-tenant por esquema/RLS dentro de cada BD). Las contraseñas
# llegan por variables de entorno (NUNCA literales). Se ejecuta una sola vez
# sobre un volumen vacío (docker-entrypoint-initdb.d).
#
# PostgreSQL es el ÚNICO datastore: estado + colas + caché. SIN Redis/Valkey.
# Ver docs/CONVENTIONS.md §0, §3, §6.
# ---------------------------------------------------------------------------
set -eu

create_role_db() {
  role="$1"; password="$2"; db="$3"
  # CREATEROLE (dev): algunas migraciones crean roles auxiliares de aplicación
  # (p. ej. Sytue Docs crea `docs_app` NOLOGIN para reforzar RLS y hace
  # `GRANT ... TO current_user`). El dueño de la BD debe poder crear ese rol al
  # aplicar sus migraciones al arranque. En prod el rol de la app NO lleva
  # CREATEROLE: el rol auxiliar se provisiona fuera de banda.
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<-SQL
    DO \$\$
    BEGIN
      IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${role}') THEN
        CREATE ROLE "${role}" LOGIN CREATEROLE PASSWORD '${password}';
      END IF;
    END
    \$\$;
    SELECT 'CREATE DATABASE "${db}" OWNER "${role}"'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${db}')\gexec
SQL
}

# --- Una BD/rol por servicio Go (services/<svc>) ----------------------------
create_role_db "${ID_DB_USER:-sytue_id}"             "${ID_DB_PASSWORD}"        "${ID_DB_NAME:-sytue_id}"
create_role_db "${DRIVE_DB_USER:-sytue_drive}"       "${DRIVE_DB_PASSWORD}"     "${DRIVE_DB_NAME:-sytue_drive}"
create_role_db "${DOCS_DB_USER:-sytue_docs}"         "${DOCS_DB_PASSWORD}"      "${DOCS_DB_NAME:-sytue_docs}"
create_role_db "${MAIL_DB_USER:-sytue_mail}"         "${MAIL_DB_PASSWORD}"      "${MAIL_DB_NAME:-sytue_mail}"
create_role_db "${CALENDAR_DB_USER:-sytue_calendar}" "${CALENDAR_DB_PASSWORD}"  "${CALENDAR_DB_NAME:-sytue_calendar}"
create_role_db "${CONTACTS_DB_USER:-sytue_contacts}" "${CONTACTS_DB_PASSWORD}"  "${CONTACTS_DB_NAME:-sytue_contacts}"
create_role_db "${NOTES_DB_USER:-sytue_notes}"       "${NOTES_DB_PASSWORD}"     "${NOTES_DB_NAME:-sytue_notes}"
create_role_db "${SYPHOTOS_DB_USER:-sytue_syphotos}" "${SYPHOTOS_DB_PASSWORD}"  "${SYPHOTOS_DB_NAME:-sytue_syphotos}"
create_role_db "${SYERP_DB_USER:-sytue_syerp}"       "${SYERP_DB_PASSWORD}"     "${SYERP_DB_NAME:-sytue_syerp}"
create_role_db "${CAST_DB_USER:-sytue_cast}"         "${CAST_DB_PASSWORD}"      "${CAST_DB_NAME:-sytue_cast}"
create_role_db "${PAGES_DB_USER:-sytue_pages}"       "${PAGES_DB_PASSWORD}"     "${PAGES_DB_NAME:-sytue_pages}"
create_role_db "${BASES_DB_USER:-sytue_bases}"       "${BASES_DB_PASSWORD}"     "${BASES_DB_NAME:-sytue_bases}"
# Sytue Rooms: UNA base para mensajería y videollamada (antes sytue_chat y sytue_meet).
create_role_db "${ROOMS_DB_USER:-sytue_rooms}"       "${ROOMS_DB_PASSWORD}"     "${ROOMS_DB_NAME:-sytue_rooms}"
create_role_db "${HUB_DB_USER:-sytue_hub}"           "${HUB_DB_PASSWORD}"       "${HUB_DB_NAME:-sytue_hub}"
create_role_db "${INTEGRATIONS_DB_USER:-sytue_integrations}" "${INTEGRATIONS_DB_PASSWORD}" "${INTEGRATIONS_DB_NAME:-sytue_integrations}"
create_role_db "${CLOUD_DB_USER:-sytue_cloud}"       "${CLOUD_DB_PASSWORD}"     "${CLOUD_DB_NAME:-sytue_cloud}"
create_role_db "${ANALYTICS_DB_USER:-sytue_analytics}" "${ANALYTICS_DB_PASSWORD}" "${ANALYTICS_DB_NAME:-sytue_analytics}"

# --- PostgREST: roles para la API REST automática de Sytue Cloud (DB Studio).
# Sobre la BD de cloud; el control plane Go expone proyectos vía PostgREST.
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "${CLOUD_DB_NAME:-sytue_cloud}" <<-SQL
  DO \$\$
  BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'web_anon') THEN
      CREATE ROLE web_anon NOLOGIN;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${POSTGREST_ROLE:-authenticator}') THEN
      CREATE ROLE "${POSTGREST_ROLE:-authenticator}" LOGIN NOINHERIT PASSWORD '${POSTGREST_ROLE_PASSWORD}';
    END IF;
  END
  \$\$;
  GRANT web_anon TO "${POSTGREST_ROLE:-authenticator}";
  GRANT USAGE ON SCHEMA public TO web_anon;
SQL

echo "[sytue] PostgreSQL inicializado: una BD por servicio Go (id, drive, docs, mail, calendar, contacts, chat, meet, hub, integrations, cloud) + roles PostgREST. Sin Redis/Valkey."
