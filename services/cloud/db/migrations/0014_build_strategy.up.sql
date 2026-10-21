-- SPDX-License-Identifier: Apache-2.0
-- Copyright (c) 2026 Daniel Delgado Ramos
--
-- Wave 9 — estrategia de build por proyecto: Dockerfile+Kaniko vs Cloud Native
-- Buildpacks (CNB / Paketo), para desplegar SIN escribir Dockerfile (paridad
-- DigitalOcean App Platform / Heroku / Coolify Nixpacks).
--
-- projects.build_strategy: '' = auto (el builder elige según el framework: solo
-- 'auto'/detect sin resolver => buildpacks; los gestionados —docker + astro/
-- static/node/go— conservan su Dockerfile => dockerfile); 'dockerfile' fuerza
-- Kaniko; 'buildpacks' fuerza el lifecycle de CNB. No es sensible. Default ''
-- preserva el comportamiento de los proyectos existentes (que ya construían con
-- Dockerfile) sin backfill; CNB es opt-in salvo para 'auto', que antes no tenía
-- Dockerfile generado.

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS build_strategy text NOT NULL DEFAULT ''
        CHECK (build_strategy IN ('', 'dockerfile', 'buildpacks'));
