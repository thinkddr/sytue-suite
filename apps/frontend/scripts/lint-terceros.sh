#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Gate: la web pública no compara Sytue con marcas ajenas.
#
# REGLA DEL PROPIETARIO (2026-09-09): en www solo se nombra a un TERCERO cuando
# se habla de INTEROPERABILIDAD (abrir un .xlsx, un cliente CalDAV), de
# MIGRACIÓN en dos clics o de una INTEGRACIÓN real. Para todo lo demás
# —titulares, lemas, SEO, pie— no se menciona ninguna marca ni servicio ajeno.
#
# Scaleway NO entra aquí: es el proveedor de infraestructura y partner
# estratégico de Sytue —lo dijo el propietario ese mismo día—, así que nombrarlo
# es hablar de la casa, no de un tercero. Por eso está en el reclamo, en el
# bloque de soberanía, en el pie y en los textos legales.
#
# Lo que este gate caza es la reincidencia: los nombres de la lista de abajo son
# comparaciones de producto («despliega como en X, datos como en Y») y no tienen
# lectura de interoperabilidad, así que en www no deben aparecer NUNCA.
set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.." || exit 1

WWW=apps/www/src
COMPARACIONES='Vercel|Supabase|Netlify|Heroku|Firebase|PlanetScale|DigitalOcean|Coolify|Render\.com|Fly\.io|Railway\.app'

hits="$(grep -rnoiE "$COMPARACIONES" "$WWW" --include='*.ts' --include='*.tsx' --include='*.astro' 2>/dev/null || true)"
if [ -n "$hits" ]; then
  echo "✗ terceros: la web pública no compara Sytue con marcas ajenas." >&2
  echo "  Se puede nombrar a un tercero SOLO en interoperabilidad, migración o integración." >&2
  echo "$hits" >&2
  exit 1
fi

echo "✓ terceros: la web pública no compara con marcas ajenas."
