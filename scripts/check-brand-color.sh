#!/usr/bin/env bash
# Gobernanza Graphite & Brass: el NARANJA de marca (#f97316 / #ea580c) NO debe reaparecer
# en el cromo de marca (logos + sign-in del IdP). El acento de marca es el LATÓN
# (#b98f4f / #855f2c). El naranja quemado de PRODUCTO one (#c2410c) SÍ es legítimo, pero
# solo como --sytue-product-one / tile del app-switcher — nunca en logos ni sign-in.
# Engánchalo a CI o pre-commit. Ver docs/design-graphite-brass y el panel de marca 2026-07.
set -euo pipefail
cd "$(dirname "$0")/.."
hits=$(grep -rniE '#f97316|#ea580c' \
  branding/logo \
  services/id/internal/http/login.go \
  apps/frontend/packages/ui/src/assets/logos 2>/dev/null || true)
if [ -n "$hits" ]; then
  echo "FALLO: naranja de marca reintroducido (usa latón #b98f4f / #855f2c):" >&2
  echo "$hits" >&2
  exit 1
fi
echo "OK: sin naranja de marca en logos/sign-in."
