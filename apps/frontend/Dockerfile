# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Sytue One — imagen del monorepo frontend (Astro 7 + Preact 10, Bun workspaces).
# Bun sustituye a Node.js + pnpm (runtime, install, workspaces y bundler).
#
# Un solo Dockerfile con dos targets:
#   - dev:     instala TODO el workspace y sirve `astro dev` de la app elegida
#              (el comando lo fija docker-compose: `bun run --filter <app> dev`).
#              El código fuente se bind-montea; `node_modules` viven en la imagen
#              y se preservan con volúmenes anónimos (ver docker-compose.yml).
#   - runtime: servidor Astro SSR compilado por app, o servidor estático para
#              `www`. Lo usan tanto producción como el overlay frontlite local.
#
# docs/CONVENTIONS.md §3, §4.
# ---- Base con Bun ----
FROM oven/bun:1.4.3-alpine@sha256:629e17411f1f129dbec3af78d5af9c9f2a937435c80349437206c6b0b7422373 AS base
WORKDIR /app
ENV ASTRO_TELEMETRY_DISABLED=1
ENV DO_NOT_TRACK=1

# ---- Dependencias: instala TODO el workspace (packages + apps) -----------------
# Copiamos primero los manifiestos para aprovechar la caché de capas de Docker.
# Se necesitan los package.json de packages y de TODAS las apps para que
# `bun install` resuelva los enlaces de workspace (workspace:*).
FROM base AS deps
COPY package.json bun.lock bunfig.toml ./
# Los parches de dependencias (`patchedDependencies`): sin ellos `bun install` falla.
COPY patches ./patches
COPY packages ./packages
COPY apps ./apps
# Instala con el lockfile congelado (reproducible). Bun coloca node_modules en la
# raíz y en cada workspace (incl. los binarios .bin/astro de cada app).
RUN bun install --frozen-lockfile

# ---- prod-deps: el MISMO lockfile, sin las dependencias de desarrollo ----------
#
# POR QUÉ EXISTE ESTA ETAPA. La de `runtime` copiaba `node_modules` desde `build`,
# que instala con TODO el workspace: lighthouse, puppeteer, astro-check, eslint,
# typescript y su árbol entero viajaban dentro de las diez imágenes de producción
# —807 MB medidos en local—. Inerte, porque nadie ejecuta lighthouse en un pod que
# sirve HTML, pero es superficie regalada y peso en cada pull. Salió al investigar
# un CVE de `extract-zip`, que entra por `@lhci/cli` y acabó en producción.
#
# QUÉ NECESITA DE VERDAD EL RUNTIME. Medido el 2026-08-13 construyendo las diez
# apps y extrayendo los imports externos de `dist/server`: SIETE paquetes, no 807
# MB — `jose`, `picomatch`, `preact` y `sharp` en las nueve SSR, más
# `@xmldom/xmldom` (calendar, contacts), `livekit-client` (rooms) y `@sytue/cloud`
# (cloud-dashboard). `@astrojs/node` NO aparece: el modo standalone lo bundlea.
#
# Y ojo con esto, que es lo que lo hacía peligroso: `picomatch` y `sharp` no los
# declara nadie, cuelgan de `astro`. Sobreviven a `--production` solo porque
# docs, drive y portal dejaron de declarar `astro` como devDependency (cd2686bf).
# Sin ese cambio previo, esos tres frontends se caen al arrancar y los otros seis
# siguen en pie: un fallo parcial y solo en producción.
FROM base AS prod-deps
COPY package.json bun.lock bunfig.toml ./
# Los parches de dependencias (`patchedDependencies`): sin ellos `bun install` falla.
COPY patches ./patches
COPY packages ./packages
COPY apps ./apps
# LA ARQUITECTURA DEL SERVIDOR, QUE NO ES LA DE QUIEN CONSTRUYE. Estos
# `node_modules` NO son solo JavaScript: medido el 2026-09-21 en una instalación
# de producción hay DIECISÉIS binarios nativos —`@img/sharp-linuxmusl-*` (el que
# el SSR carga para servir imágenes), `lightningcss`, el compilador de Astro,
# `rolldown`, `satteri`—, así que un árbol instalado en el runner ARM64 no sirve
# en el servidor x86_64 y el fallo aparece al pedir una imagen, no al arrancar.
#
# `--cpu` solo afecta a las dependencias OPCIONALES, que es exactamente donde
# viven esos binarios (cada paquete napi se declara así). La libc NO se fuerza:
# sale de esta imagen, que es Alpine, de modo que se instalan las variantes
# `linuxmusl`, que son las correctas aquí.
#
# Vacío = la del que construye, que es lo que hace falta en local y en el
# servidor (bin/build-images.sh construye donde va a ejecutar).
ARG CPU_DESTINO
RUN bun install --frozen-lockfile --production ${CPU_DESTINO:+--cpu "$CPU_DESTINO" --os linux}

# ---- dev: entorno de desarrollo (astro dev por app) ----------------------------
# El comando concreto (`bun run --filter <app> dev --host 0.0.0.0 --port <n>`) y
# el código fuente (bind mount) los aporta docker-compose. Aquí solo dejamos el
# workspace instalado. Bun ya trae el usuario no-root `bun` en la imagen oficial.
FROM deps AS dev
ENV NODE_ENV=development
# Entrypoint que limpia el lock obsoleto del dev-server de Astro antes del comando
# de compose. Va a /usr/local/bin (fuera de /app, que el bind mount sobreescribe).
COPY docker-dev-entrypoint.sh /usr/local/bin/sytue-dev-entrypoint.sh
RUN chmod +x /usr/local/bin/sytue-dev-entrypoint.sh
# Copiamos el resto del código para que la imagen sea funcional también sin bind
# mount (CI); en compose lo sobreescribe el volumen `../../apps/frontend:/app`.
COPY . .
# En dev NO cambiamos a USER bun: el código y `.astro` (donde Astro escribe
# types.d.ts y el lock del dev-server) se bind-montean desde el host y los
# volúmenes anónimos de `.astro` son propiedad de root; el proceso debe poder
# escribir en ambos. La imagen de runtime (prod) sí corre como usuario `bun`.
# Puerto genérico expuesto (cada app usa el suyo, 4321-4331, según compose).
EXPOSE 4321
ENTRYPOINT ["/usr/local/bin/sytue-dev-entrypoint.sh"]
CMD ["bun", "run", "dev"]

# ---- build (prod): compila una app concreta (ARG APP) --------------------------
FROM deps AS build
COPY . .
ARG APP=portal
# PUBLIC_* que Vite/Astro HORNEAN en el bundle (cliente + SSR) en tiempo de build
# (import.meta.env.PUBLIC_*). Sin --build-arg NO entran al entorno del RUN (BuildKit
# omite los args no pasados) => quedan undefined => el código cae a su default de
# PROD (*.sytue.eu). Prod-neutral: el build por defecto (Helm/CI, sin args) no cambia.
# deploy/compose/frontlite.yml los pasa con *.sytue.localhost para el dev compilado.
ARG PUBLIC_OIDC_ISSUER
ARG PUBLIC_HUB_URL
ARG PUBLIC_SYTUE_HUB_BASE_URL
ARG PUBLIC_SYTUE_DRIVE_BASE_URL
ARG PUBLIC_SYTUE_DOCS_BASE_URL
ARG PUBLIC_SYTUE_CLOUD_API_URL
# Base pública de los instaladores nativos. Va AQUÍ y no como variable del
# contenedor porque `www` es SSG (output: 'static'): su HTML se genera en este
# RUN y después no hay servidor que lea nada. El chart la pasaba como env de
# runtime, así que no llegaba: la página de descargas se quedaba en
# «próximamente» para siempre, aunque el bucket tuviera los binarios.
ARG PUBLIC_SYTUE_DOWNLOADS_BASE
# Plataformas realmente publicadas (`windows,linux`). Vacía ⇒ las tres. macOS
# necesita compilarse en un Mac, así que no siempre llegan a la vez y encender
# las tres a ciegas dejaría un botón que da 404.
ARG PUBLIC_SYTUE_DOWNLOADS_PLATFORMS
# Versión y commit TAMBIÉN en el build, no solo en el runtime de más abajo, por
# el mismo motivo que la base de descargas: `www` es SSG y su `/healthz` se
# genera en este RUN. Sin esto, la única superficie pública de la suite sería la
# única incapaz de decir qué versión sirve.
#
# Se reaprovechan los `--build-arg VERSION/COMMIT` que la release ya pasa a TODAS
# las imágenes: no hay que tocar el workflow. Se reexponen con prefijo PUBLIC_
# porque es el único que Astro hornea en el bundle.
ARG VERSION
ARG COMMIT
ENV PUBLIC_SYTUE_RELEASE=${VERSION}
ENV PUBLIC_SYTUE_COMMIT=${COMMIT}
# Filtro por RUTA (./apps/<dir>): casa por DIRECTORIO siempre, incl. cuando el nombre
# del paquete (@sytue/app-console, @sytue/app-cloud-dashboard, @sytue/app-www) ≠ dir.
RUN bun run --filter "./apps/${APP}" build

# ---- runtime (prod): servidor Astro SSR standalone de la app -------------------
FROM base AS runtime
ENV NODE_ENV=production
ENV HOST=0.0.0.0
ENV PORT=8080
ARG APP=portal
ENV SYTUE_FRONTEND_APP=${APP}
# Qué versión es ESTA imagen. Se hornea aquí y no se inyecta desde el chart a
# propósito: la pregunta que responde la pantalla de plataforma es «qué está
# corriendo», y un valor puesto por Helm dice «qué creía Helm que iba a correr».
# No es lo mismo cuando un despliegue va a medias, cuando alguien actualiza con
# `--reuse-values` o cuando el tag de versión se ha movido a otro commit — que
# es justo cuando alguien va a mirarlo.
#
# Van en la etapa RUNTIME y no en la de build: cambiar de versión no debe
# invalidar la caché de `bun build`, que es lo caro. Vacías ⇒ la pantalla dice
# que la imagen se construyó a mano, que también es una respuesta.
ARG VERSION
ARG COMMIT
ENV SYTUE_RELEASE=${VERSION}
ENV SYTUE_COMMIT=${COMMIT}
# node_modules del workspace (raíz) + de la app: Bun NO siempre hoista todo a la
# raíz (preact acabó en apps/<APP>/node_modules), así que el resolver debe ver
# AMBOS. Se sirve el dist DESDE el dir de la app (WORKDIR) para que `bun` resuelva
# subiendo apps/<APP>/node_modules -> node_modules (con /app/dist en la raíz no
# veía apps/<APP>/node_modules y fallaba con "Cannot find package 'preact'").
# Desde `prod-deps`, NO desde `build`: es el mismo lockfile sin lo de desarrollo.
COPY --from=prod-deps --chown=bun:bun /app/node_modules ./node_modules
COPY --from=prod-deps --chown=bun:bun /app/apps/${APP}/node_modules ./apps/${APP}/node_modules
COPY --from=build --chown=bun:bun /app/apps/${APP}/dist ./apps/${APP}/dist
COPY --from=build --chown=bun:bun /app/static-serve.mjs ./apps/${APP}/static-serve.mjs
# `static-serve.mjs` negocia el idioma de la raíz con `@sytue/i18n`, y el
# node_modules de `prod-deps` solo trae el ENLACE del workspace a packages/i18n:
# sin el paquete al otro lado, www no arranca («Cannot find package»).
COPY --from=build --chown=bun:bun /app/packages/i18n ./packages/i18n
WORKDIR /app/apps/${APP}
USER bun
EXPOSE 8080
# Nueve apps generan Astro SSR. `www` es SSG y usa el servidor estático común.
# La selección depende solo del ARG horneado, no de una imagen diferente.
CMD ["sh", "-c", "if [ \"$SYTUE_FRONTEND_APP\" = www ]; then exec bun ./static-serve.mjs; else exec bun ./dist/server/entry.mjs; fi"]
