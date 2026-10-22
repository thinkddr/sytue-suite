#!/bin/sh
# Sytue One — entrypoint de desarrollo del frontend.
# Limpia el lock/artefactos obsoletos del dev-server de Astro (los volúmenes
# anónimos de .astro sobreviven al recrear el contenedor y provocan
# "Another astro dev server is already running" en rebuilds in-place), y luego
# ejecuta el comando que fija docker-compose (bun run --filter <app> dev ...).
rm -rf /app/apps/*/.astro 2>/dev/null || true
exec "$@"
