#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Arranque del contenedor «todo en uno»: Docker interno + ./sytue-deploy --mode servidor.
# Ver deploy/todo-en-uno/Dockerfile para las variables y el modo de uso.
set -euo pipefail

SUITE=/opt/sytue-suite
COMPOSE="${SUITE}/deploy/compose"
DATOS=/datos
DOMINIO="${SYTUE_DOMINIO:-suite.localhost}"

log() { printf '[todo-en-uno] %s\n' "$*"; }

# 1. Todo lo que hay que conservar, en el volumen /datos.
mkdir -p "${DATOS}/docker" "${DATOS}/config/certs" "${DATOS}/ca"
ln -sfn "${DATOS}/config/.env" "${COMPOSE}/.env"
rm -rf "${COMPOSE}/traefik/certs"
ln -sfn "${DATOS}/config/certs" "${COMPOSE}/traefik/certs"
export CAROOT="${DATOS}/ca"

# 2. Docker interno, con sus imágenes y volúmenes en /datos/docker.
log "Arrancando el Docker interno..."
dockerd-entrypoint.sh dockerd --data-root "${DATOS}/docker" >/var/log/dockerd.log 2>&1 &
DOCKERD=$!
parar() {
  log "Parando la suite..."
  kill -TERM "$DOCKERD" 2>/dev/null || true
  wait "$DOCKERD" 2>/dev/null || true
  exit 0
}
trap parar TERM INT
for _ in $(seq 1 60); do docker info >/dev/null 2>&1 && break; sleep 1; done
docker info >/dev/null 2>&1 || { log "El Docker interno no arranca (¿falta --privileged?):"; tail -n 20 /var/log/dockerd.log; exit 1; }

# 3. El asistente, sin preguntas. Idempotente: la primera vez construye y da de
#    alta al propietario; después solo vuelve a levantar lo que haya parado.
TLS="${SYTUE_TLS:-}"
if [ -z "$TLS" ]; then
  case "$DOMINIO" in *.localhost | localhost) TLS=interno ;; *) TLS=letsencrypt ;; esac
fi
# Videollamadas: la IP que el SFU anuncia a los navegadores. Dentro del contenedor
# solo se ve la IP interna de Docker; con *.localhost basta 127.0.0.1.
if [ -z "${SYTUE_IP_MEDIOS:-}" ]; then
  case "$DOMINIO" in *.localhost | localhost) export SYTUE_IP_MEDIOS=127.0.0.1 ;; esac
fi
args=(--mode servidor --yes --domain "$DOMINIO" --tls "$TLS")
[ -n "${SYTUE_ACME_EMAIL:-}" ] && args+=(--acme-email "$SYTUE_ACME_EMAIL")
[ -n "${SYTUE_PROPIETARIO:-}" ] && args+=(--admin-email "$SYTUE_PROPIETARIO")
[ -n "${SYTUE_ORGANIZACION:-}" ] && args+=(--org-slug "$SYTUE_ORGANIZACION")
[ -n "${SYTUE_USUARIOS:-}" ] && args+=(--usuarios "$SYTUE_USUARIOS")
[ "${SYTUE_REGISTRO_ABIERTO:-}" = "true" ] && args+=(--registro-abierto)
[ -f "${DATOS}/config/construida" ] && args+=(--no-build)
# shellcheck disable=SC2206 # SYTUE_EXTRA son opciones separadas por espacios
[ -n "${SYTUE_EXTRA:-}" ] && args+=(${SYTUE_EXTRA})

rm -f /run/sytue-lista
cd "$SUITE"
if ./sytue-deploy "${args[@]}"; then
  case " ${SYTUE_EXTRA:-} " in
    *" --only-config "*) log "Configuración lista (--only-config): no se arranca nada."; exec sleep infinity ;;
  esac
  touch "${DATOS}/config/construida" /run/sytue-lista
  [ "$TLS" = "interno" ] && log "CA para confiar en los certificados: docker cp <contenedor>:${CAROOT}/rootCA.pem ."
  log "Suite en marcha: https://one.${DOMINIO}"
else
  log "El asistente terminó con errores; los contenedores siguen vivos para revisarlos:"
  log "  docker exec -it <contenedor> make -C ${SUITE} dev-ps"
fi

wait "$DOCKERD"
