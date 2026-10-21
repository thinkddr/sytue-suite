#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# ¿Cabe este job en el disco del CI? Si no, falla YA, con un mensaje que dice
# qué pasa, en vez de a los veinte minutos con «no space left on device».
#
#   scripts/ci-disco-libre.sh [GB mínimos, 20 por defecto]
#
# Dentro del contenedor del job, `/` es la capa del Docker del CI, que vive en
# /srv/ci del frontal: el mismo volumen donde van a parar las imágenes que el
# job construya por el socket. Se miran también los inodos: un build ya murió
# con 31 GB libres y la tabla de inodos llena. El 28/09, con el disco al 100 %,
# los tres `trivy image` de la v3.7.14 cayeron a medio construir.
set -euo pipefail
min="${1:-20}"
ruta="${SYTUE_DISCO_RUTA:-/}"
libre="$(df -Pk "$ruta" | awk 'NR==2{print int($4/1048576)}')"
inodos="$(df -Pi "$ruta" | awk 'NR==2{if($2>0)print int($3*100/$2); else print 0}')"
echo "disco del CI: ${libre} GB libres, inodos al ${inodos} % (este job pide ${min} GB)"
if [ "$libre" -lt "$min" ] || [ "$inodos" -ge 95 ]; then
  echo "::error::El disco del CI (/srv/ci en el frontal) tiene ${libre} GB libres y los inodos al ${inodos} %; este job necesita ${min} GB. No se empieza para no morir a medias con «no space left on device». Lo poda sytue-podar-ci cada 15 min (docs/operations.md#poda-ci-frontal): repite el job cuando baje, o mira 'journalctl -u sytue-podar-ci' en el frontal." >&2
  exit 1
fi
