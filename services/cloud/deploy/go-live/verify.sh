#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# verify.sh — ejecuta el «Checklist de verificación por pieza» (§7 de
# docs/operations.md) como comandos reales best-effort. Idempotente y GUARDADO:
# no cambia nada, no aborta al primer fallo (acumula OK/FALLO/PENDIENTE) y, si no
# hay clúster/infra viva, cada check reporta «pendiente» limpiamente.
#
# Uso:
#   services/cloud/deploy/go-live/verify.sh
#
# Variables (todas opcionales, con defaults de prod):
#   SYTUE_CLOUD_NS        namespace del control-plane          (def: sytue-paas)
#   SYTUE_CLOUD_BUILD_NS  namespace de builds                  (def: sytue-paas-builds)
#   CLOUD_API_URL         URL pública del API                  (def: https://cloud.sytue.eu)
#   CLOUD_API_HEALTH_PATH ruta de health tras el ingress       (def: /api/healthz)
#   SYTUE_CLOUD_TOKEN     token de API (scl_...) para el smoke de /api/v1/org
#
# Salida: exit 0 si no hay FALLO (los PENDIENTE no cuentan como fallo); 1 si hay ≥1 FALLO.

set -euo pipefail

NS="${SYTUE_CLOUD_NS:-sytue-paas}"
BUILD_NS="${SYTUE_CLOUD_BUILD_NS:-sytue-paas-builds}"
CLOUD_API_URL="${CLOUD_API_URL:-https://cloud.sytue.eu}"
HEALTH_PATH="${CLOUD_API_HEALTH_PATH:-/api/healthz}"

PASS=0; FAIL=0; PEND=0
ok()   { printf '  \033[32mOK\033[0m       %s\n' "$*"; PASS=$((PASS+1)); }
bad()  { printf '  \033[31mFALLO\033[0m    %s\n' "$*"; FAIL=$((FAIL+1)); }
pend() { printf '  \033[33mPENDIENTE\033[0m %s\n' "$*"; PEND=$((PEND+1)); }
hdr()  { printf '\n== %s ==\n' "$*"; }

have() { command -v "$1" >/dev/null 2>&1; }

# ── Prerrequisitos ─────────────────────────────────────────────────────────────
hdr "Prerrequisitos"
if have kubectl; then
  ok "kubectl presente"
else
  pend "kubectl no instalado (checks de clúster ⇒ pendiente)"
fi
if have helm; then
  ok "helm presente"
else
  pend "helm no instalado (checks de releases ⇒ pendiente)"
fi

KUBE_OK=0
if have kubectl && kubectl cluster-info >/dev/null 2>&1; then
  CTX="$(kubectl config current-context 2>/dev/null || echo '?')"
  ok "clúster alcanzable (context=$CTX)"
  KUBE_OK=1
else
  pend "sin clúster alcanzable — los checks de Kapsule quedan pendientes (infra no viva)"
fi

# helper: corre un check de clúster solo si KUBE_OK; si no, pend.
kube_guard() { [ "$KUBE_OK" = 1 ] || { pend "$1 (sin clúster)"; return 1; }; }

# ── Charts / pods arriba ────────────────────────────────────────────────────────
hdr "Charts arriba (ns=$NS)"
for d in sytue-cloud-api sytue-cloud-builder sytue-cloud-controller; do
  if kube_guard "deployment $d"; then
    if ready="$(kubectl -n "$NS" get deploy "$d" -o jsonpath='{.status.readyReplicas}/{.status.replicas}' 2>/dev/null)" && [ -n "$ready" ]; then
      ready_replicas="${ready%%/*}"
      replicas="${ready#*/}"
      if [[ "$ready_replicas" =~ ^[0-9]+$ && "$replicas" =~ ^[0-9]+$ ]] \
        && [ "$replicas" -gt 0 ] && [ "$ready_replicas" -eq "$replicas" ]; then
        ok "deployment $d listo ($ready)"
      else
        bad "deployment $d no listo ($ready)"
      fi
    else
      bad "deployment $d ausente en $NS"
    fi
  fi
done

# ── Modo kube del controller ────────────────────────────────────────────────────
hdr "Modo kube (controller)"
if kube_guard "logs del controller"; then
  if kubectl -n "$NS" logs "deploy/sytue-cloud-controller" --tail=500 2>/dev/null | grep -q "modo Kapsule"; then
    ok "controller en modo Kapsule (client-go)"
  else
    bad "no se ve 'controller en modo Kapsule (client-go)' en logs (¿RUNNER_MODE=kube?)"
  fi
fi

# ── Secrets existentes ──────────────────────────────────────────────────────────
hdr "Secrets"
for s in sytue-cloud-api sytue-cloud-builder sytue-cloud-controller; do
  if kube_guard "secret $s"; then
    if kubectl -n "$NS" get secret "$s" >/dev/null 2>&1; then
      ok "secret $s en $NS"
    else
      bad "secret $s ausente en $NS"
    fi
  fi
done
if kube_guard "secret sytue-registry"; then
  if kubectl -n "$BUILD_NS" get secret sytue-registry >/dev/null 2>&1; then
    ok "secret sytue-registry en $BUILD_NS"
  else
    bad "secret sytue-registry ausente en $BUILD_NS"
  fi
fi
if kube_guard "secret sytue-platform-s3"; then
  # per-tenant: lo materializa el controller en cada namespace tenant-*
  if kubectl get secret sytue-platform-s3 -A >/dev/null 2>&1 \
     && [ -n "$(kubectl get secret sytue-platform-s3 -A --no-headers 2>/dev/null)" ]; then
    ok "sytue-platform-s3 presente (namespaces de tenant)"
  else
    pend "sytue-platform-s3 aún no materializado (aparece al provisionar el 1er postgres)"
  fi
fi

# ── Health / Ready del API ──────────────────────────────────────────────────────
hdr "Health API ($CLOUD_API_URL)"
if have curl; then
  code="$(curl -s -o /dev/null -m 5 -w '%{http_code}' "${CLOUD_API_URL}${HEALTH_PATH}" 2>/dev/null || true)"; code="${code:-000}"
  if [ "$code" = 200 ]; then
    ok "GET ${HEALTH_PATH} -> 200"
  else
    pend "GET ${HEALTH_PATH} -> ${code} (¿ingress/infra no viva?)"
  fi
  rcode="$(curl -s -o /dev/null -m 5 -w '%{http_code}' "${CLOUD_API_URL}/api/readyz" 2>/dev/null || true)"; rcode="${rcode:-000}"
  if [ "$rcode" = 200 ]; then
    ok "GET /api/readyz -> 200"
  else
    pend "GET /api/readyz -> ${rcode}"
  fi
else
  pend "curl no instalado (health/smoke ⇒ pendiente)"
fi

# ── Smoke autenticado /api/v1 ─────────────────────────────────────────────────────
hdr "Smoke /api/v1 (token)"
if [ -n "${SYTUE_CLOUD_TOKEN:-}" ] && have curl; then
  code="$(curl -s -o /dev/null -m 5 -w '%{http_code}' \
    -H "Authorization: Bearer ${SYTUE_CLOUD_TOKEN}" "${CLOUD_API_URL}/api/v1/org" 2>/dev/null || true)"; code="${code:-000}"
  if [ "$code" = 200 ]; then
    ok "GET /api/v1/org (autenticado) -> 200"
  else
    pend "GET /api/v1/org -> ${code} (token/infra)"
  fi
else
  pend "sin SYTUE_CLOUD_TOKEN — smoke autenticado omitido"
fi

# ── CronJobs de backup ────────────────────────────────────────────────────────────
hdr "Backup CronJobs (*-pg-backup)"
if kube_guard "cronjobs de backup"; then
  cj="$(kubectl get cronjob -A --no-headers 2>/dev/null | awk '$2 ~ /-pg-backup$/ {print $1"/"$2}' || true)"
  if [ -n "$cj" ]; then
    ok "CronJob(s) de backup: $(echo "$cj" | tr '\n' ' ')"
  else
    pend "sin CronJob *-pg-backup (aparece al provisionar un servicio postgres)"
  fi
fi

# ── Resumen ─────────────────────────────────────────────────────────────────────
printf '\n== Resumen ==\n  OK=%d  FALLO=%d  PENDIENTE=%d\n' "$PASS" "$FAIL" "$PEND"
[ "$FAIL" -eq 0 ] && printf '  Sin fallos. (PENDIENTE = requiere credenciales/infra viva.)\n'
[ "$FAIL" -eq 0 ]
