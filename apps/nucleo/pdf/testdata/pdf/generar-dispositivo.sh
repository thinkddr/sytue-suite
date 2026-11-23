#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Regenera los fixtures del firmador del NAVEGADOR (ADR 0086, «Firmar en el
# dispositivo»): `firmado-en-el-dispositivo.pdf` (invisible, B-B) y
# `firmado-en-el-dispositivo-visible-con-sello.pdf` (visible con rúbrica y sello de
# tiempo, B-T). Los firma el código TypeScript de `packages/office/src/pdf/dispositivo`
# (`firmar-en-el-dispositivo.ts`); la autoridad de sellado es `openssl ts -reply` con
# una PKI propia y efímera, de la que solo se guarda la raíz
# (`anclas/raiz-sello-dispositivo.crt`).
#
# Se puede lanzar suelto (no necesita ni LibreOffice ni pyHanko: parte de
# `sin-firmas.pdf` y `firma-valida-flujos.pdf`, ya commiteados) y lo lanza también
# `generar.sh` al final. Lo que debe salir de cada uno está en `manifiesto.json`.
#
# DEPENDENCIAS (solo de desarrollo): openssl 3.x y bun.
set -euo pipefail

AQUI="$(cd "$(dirname "$0")" && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT

DESDE=20260101000000Z
HASTA_RAIZ=20560101000000Z
HASTA=20360101000000Z

cat >"$T/ca.cnf" <<CNF
[ ca ]
default_ca = CA
[ CA ]
dir = $T
database = \$dir/index.txt
new_certs_dir = \$dir
rand_serial = yes
default_md = sha256
policy = pol
unique_subject = no
[ pol ]
organizationName = optional
commonName = supplied
[ ext_ca ]
basicConstraints = critical,CA:TRUE
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
[ ext_tsa ]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,timeStamping
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
CNF
: >"$T/index.txt"

# La raíz de la autoridad (EC) y la autoridad (RSA), con el uso timeStamping.
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$T/raiz.key"
openssl req -new -utf8 -key "$T/raiz.key" -subj "/O=Sytue pruebas/CN=Sytue pruebas raiz de sellos del dispositivo" \
  -out "$T/raiz.csr"
openssl ca -batch -utf8 -config "$T/ca.cnf" -selfsign -keyfile "$T/raiz.key" -in "$T/raiz.csr" \
  -extensions ext_ca -startdate "$DESDE" -enddate "$HASTA_RAIZ" -notext -out "$T/raiz.crt" 2>/dev/null
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$T/tsa.key" 2>/dev/null
openssl req -new -utf8 -key "$T/tsa.key" -subj "/O=Sytue pruebas/CN=TSA de prueba del dispositivo" -out "$T/tsa.csr"
openssl ca -batch -utf8 -config "$T/ca.cnf" -keyfile "$T/raiz.key" -cert "$T/raiz.crt" -in "$T/tsa.csr" \
  -extensions ext_tsa -startdate "$DESDE" -enddate "$HASTA" -notext -out "$T/tsa.crt" 2>/dev/null

echo 01 >"$T/tsa.serie"
cat >"$T/tsa.cnf" <<CNF
[ tsa ]
default_tsa = tsa_cfg
[ tsa_cfg ]
serial = $T/tsa.serie
signer_cert = $T/tsa.crt
certs = $T/raiz.crt
signer_key = $T/tsa.key
signer_digest = sha256
default_policy = 1.2.3.4.1
digests = sha256
accuracy = secs:1
ordering = no
tsa_name = no
ess_cert_id_chain = no
ess_cert_id_alg = sha256
crypto_device = builtin
CNF

bun "$AQUI/firmar-en-el-dispositivo.ts" "$T/tsa.cnf" "$T"
cp "$T/raiz.crt" "$AQUI/anclas/raiz-sello-dispositivo.crt"
