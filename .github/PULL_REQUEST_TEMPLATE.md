<!-- SPDX-License-Identifier: Apache-2.0 -->

## Descripción

<!-- Qué cambia y por qué. Referencia a la incidencia si la hay. -->

## Checklist de paridad y versiones

- [ ] **Paridad web → nativas (CONVENTIONS §13.5)**: si este PR añade o toca una función de las web apps, el registro `apps/frontend/packages/config/src/paridad.ts` la incorpora con estado en `escritorio` y `movil` ('si', o 'planificado' con `deuda`). TopBar = única excepción.
- [ ] En cada ola con funciones de web pendientes en el registro: ci-desktop y ci-mobile corren al crear la release — no cortes una con 'planificado' nuevo sin su deuda.
- [ ] Dependencias/Action/imágenes a la última estable, con su fila en `compliance/supply-chain/blocked-baseline.tsv` si introduce host no-UE.
- [ ] Gates en local ANTES de pushear (el hook de pre-push los corre).
- [ ] Prueba dejada detrás (§13.4) y, si hay SQL nuevo, test de integración contra Postgres.
