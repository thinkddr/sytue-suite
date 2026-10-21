-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- LO QUE UNA MIGRACIÓN PUEDE SER, Y EN QUÉ DIRECCIÓN VA.
--
-- Dos cosas, y la primera es un fallo que estaba servido:
--
-- 1. Los CHECK de `migration_jobs` se escribieron cuando solo existían Drive y
--    Contactos desde Microsoft 365 y Google. Desde entonces el motor aprendió
--    repositorios, gists, equipos, ficheros de canal, correo, agenda y las
--    aplicaciones de Google Cloud, y aprendió GitHub como origen — pero la tabla
--    seguía rechazándolos. Nada de eso podía INSERTARSE en una base real: los
--    tests usan un doble en memoria, así que el CHECK no reventaba hasta
--    producción. Se amplían los dos.
--
-- 2. `direction` abre el camino de salida. Hasta ahora un trabajo solo podía
--    TRAER contenido; con 'out' el mismo motor lo LLEVA a un tercero. La
--    columna `source` pasa a significar «el SaaS de terceros implicado»: de
--    dónde se trae ('in') o adónde se lleva ('out'). No se renombra a propósito:
--    renombrarla obligaría a reescribir el store y las consultas de todos los
--    trabajos ya guardados para no ganar nada.
--
-- Poder irse es parte del producto, no una concesión: una plataforma de la que
-- no puedes salir no es soberana, es otra jaula.

ALTER TABLE migration_jobs DROP CONSTRAINT IF EXISTS migration_jobs_source_check;
ALTER TABLE migration_jobs ADD CONSTRAINT migration_jobs_source_check
    CHECK (source IN ('m365', 'google', 'github'));

ALTER TABLE migration_jobs DROP CONSTRAINT IF EXISTS migration_jobs_domain_check;
ALTER TABLE migration_jobs ADD CONSTRAINT migration_jobs_domain_check
    CHECK (domain IN (
        'drive', 'contacts', 'repos', 'gists', 'teams', 'team_files',
        'mail', 'calendar', 'cloud'
    ));

ALTER TABLE migration_jobs
    ADD COLUMN IF NOT EXISTS direction text NOT NULL DEFAULT 'in'
        CHECK (direction IN ('in', 'out'));

COMMENT ON COLUMN migration_jobs.direction IS
    'in = se trae contenido del tercero a Sytue; out = se lleva de Sytue al tercero.';
COMMENT ON COLUMN migration_jobs.source IS
    'El SaaS de terceros implicado: de dónde se trae (direction=in) o adónde se lleva (out).';
COMMENT ON COLUMN migration_jobs.source_user IS
    'La identidad en el TERCERO: de la que se lee (in) o en la que se escribe (out).';
