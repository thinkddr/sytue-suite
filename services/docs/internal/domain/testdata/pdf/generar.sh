#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Regenera la BATERÍA DE FIXTURES CRIPTOGRÁFICOS del verificador de firmas PDF
# (`pdfverificar.go`): los *.pdf y anclas/*.crt de este directorio. El test que los
# recorre es `pdfverificar_corpus_test.go`; lo que debe salir de cada uno está en
# `manifiesto.json`, que se escribe A MANO y no lo toca este guion.
#
# POR QUÉ EXISTE. Los tests de `pdfverificar_test.go` generan sus firmas con el mismo
# código de la casa (`FirmarIncrementalPDF`): si firmar y verificar comparten un
# malentendido de la norma, los dos tests pasan. Esta batería está firmada por
# HERRAMIENTAS AJENAS —lo que contrasta de verdad—:
#
#   * OpenSSL (`openssl ca`, `openssl ts -reply`, `openssl cms -sign`): la PKI de
#     prueba, la autoridad de sellado de tiempo RFC 3161 y UNA firma montada a mano
#     (fixture «firma-openssl-cms»), sin ninguna biblioteca de PDF por medio.
#   * LibreOffice (`soffice`): el PDF base que se firma, con la estructura que
#     escribe una suite ofimática real (xref clásico, fuentes incrustadas).
#   * pyHanko (generar.py): el firmador PAdES independiente que más se usa, para las
#     firmas, las contrafirmas, la firma de certificación (DocMDP), el sello de
#     documento y las actualizaciones incrementales. También genera un PDF base con
#     flujos de referencias cruzadas y de objetos (la forma que escribe Word/Acrobat).
#
# CLAVES. Las privadas son EFÍMERAS: se crean en un directorio temporal que se
# borra al salir y NO se commitean (el .gitignore las excluye, y gitleaks no tiene
# nada que mirar). Solo se guardan los PDF y los certificados RAÍZ (anclas/*.crt).
# Cada regeneración cambia TODOS los bytes —claves nuevas, firmas nuevas—, pero las
# vigencias son absolutas y el manifiesto fija la hora de la verificación
# («ahora»), así que el resultado esperado no cambia ni caduca con el reloj.
#
# DEPENDENCIAS (solo de desarrollo; NADA de esto entra en producción ni en el
# build): openssl 3.x, soffice, python3 con venv y red hacia PyPI para pyHanko. Las
# versiones están fijadas abajo; los PDF commiteados dicen con cuáles se hicieron.
#
# USO (en el contenedor de agentes, no en el portátil):
#   services/docs/internal/domain/testdata/pdf/generar.sh
#   SYTUE_PDF_VENV=/ruta/venv  ...   # reutiliza un venv que ya tenga pyHanko
set -euo pipefail

AQUI="$(cd "$(dirname "$0")" && pwd)"
PYHANKO_PINS=(pyHanko==0.37.0 pyhanko-certvalidator==0.32.1 asn1crypto==1.5.1 cryptography==50.0.2 oscrypto==1.3.0)

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/pki" "$T/out"
PKI="$T/pki"

# --- 1. La PKI de prueba -------------------------------------------------------
# Vigencias ABSOLUTAS (el manifiesto verifica a una hora fija):
#   CA y raíces ........ 2026-01-01 .. 2056-01-01
#   firmantes y TSA .... 2026-01-01 .. 2036-01-01
#   «carla» (caducada) . 2026-01-01 .. 2026-03-01
#   «dario» (futura) ... 2040-01-01 .. 2045-01-01
CA_DESDE=20260101000000Z CA_HASTA=20560101000000Z
F_DESDE=20260101000000Z F_HASTA=20360101000000Z

# nueva_clave <rsa|ec> <fichero>
nueva_clave() {
  case "$1" in
    rsa) openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$2" 2>/dev/null ;;
    ec) openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$2" ;;
  esac
}

# conf_ca <dir>: configuración mínima de `openssl ca` para una autoridad.
conf_ca() {
  mkdir -p "$1/nuevos"
  : >"$1/index.txt"
  cat >"$1/ca.cnf" <<CNF
[ ca ]
default_ca = CA
[ CA ]
dir = $1
database = \$dir/index.txt
new_certs_dir = \$dir/nuevos
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
[ ext_inter ]
basicConstraints = critical,CA:TRUE,pathlen:0
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
[ ext_firma ]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature,nonRepudiation
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
[ ext_tsa ]
basicConstraints = CA:FALSE
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,timeStamping
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid
CNF
}

# raiz <nombre> <alg> <CN>
raiz() {
  nueva_clave "$2" "$PKI/$1.key"
  conf_ca "$T/ca-$1"
  openssl req -new -utf8 -key "$PKI/$1.key" -subj "/O=Sytue pruebas/CN=$3" -out "$PKI/$1.csr"
  openssl ca -batch -utf8 -config "$T/ca-$1/ca.cnf" -selfsign -keyfile "$PKI/$1.key" -in "$PKI/$1.csr" \
    -extensions ext_ca -startdate "$CA_DESDE" -enddate "$CA_HASTA" -notext -out "$PKI/$1.crt" 2>/dev/null
}

# emitir <emisor> <nombre> <alg> <CN> <extensiones> <desde> <hasta>
emitir() {
  nueva_clave "$3" "$PKI/$2.key"
  conf_ca "$T/ca-$2"
  openssl req -new -utf8 -key "$PKI/$2.key" -subj "/O=Sytue pruebas/CN=$4" -out "$PKI/$2.csr"
  openssl ca -batch -utf8 -config "$T/ca-$1/ca.cnf" -keyfile "$PKI/$1.key" -cert "$PKI/$1.crt" -in "$PKI/$2.csr" \
    -extensions "$5" -startdate "$6" -enddate "$7" -notext -out "$PKI/$2.crt" 2>/dev/null
}

raiz raiz-a rsa "Sytue pruebas raiz A"
emitir raiz-a inter-a rsa "Sytue pruebas intermedia A" ext_inter "$CA_DESDE" "$CA_HASTA"
emitir inter-a ana rsa "Ana Pérez" ext_firma "$F_DESDE" "$F_HASTA"
emitir inter-a bruno ec "Bruno Gómez" ext_firma "$F_DESDE" "$F_HASTA"
emitir inter-a elena rsa "Elena Ruiz" ext_firma "$F_DESDE" "$F_HASTA"
emitir inter-a carla rsa "Carla Díaz (caducada)" ext_firma 20260101000000Z 20260301000000Z
emitir inter-a dario ec "Darío Vega (futuro)" ext_firma 20400101000000Z 20450101000000Z

# Una raíz que NO está en las anclas de los firmantes.
raiz raiz-b ec "Sytue pruebas raiz B (desconocida)"
emitir raiz-b intruso rsa "Ivo Intruso" ext_firma "$F_DESDE" "$F_HASTA"

# Las autoridades de sellado: una de confianza y otra que no.
raiz raiz-sello ec "Sytue pruebas raiz de sellos"
emitir raiz-sello tsa rsa "TSA de prueba" ext_tsa "$F_DESDE" "$F_HASTA"
raiz raiz-sello-otra ec "Sytue pruebas raiz de sellos ajena"
emitir raiz-sello-otra tsa-otra ec "TSA ajena" ext_tsa "$F_DESDE" "$F_HASTA"

# Cadenas que viajan dentro de las firmas (el firmante va aparte).
cat "$PKI/inter-a.crt" "$PKI/raiz-a.crt" >"$PKI/cadena-a.pem"
cp "$PKI/inter-a.crt" "$PKI/cadena-a-sin-raiz.pem"
cp "$PKI/raiz-b.crt" "$PKI/cadena-b.pem"
cp "$PKI/raiz-sello.crt" "$PKI/cadena-sello.pem"
cp "$PKI/raiz-sello-otra.crt" "$PKI/cadena-sello-otra.pem"

# Autoridad de sellado RFC 3161 de OpenSSL (`openssl ts -reply`); la invoca generar.py.
for tsa in tsa:raiz-sello tsa-otra:raiz-sello-otra; do
  n="${tsa%%:*}"
  echo 01 >"$PKI/$n.serie"
  cat >"$PKI/$n.tsa.cnf" <<CNF
[ tsa ]
default_tsa = tsa_cfg
[ tsa_cfg ]
serial = $PKI/$n.serie
signer_cert = $PKI/$n.crt
certs = $PKI/cadena-sello$([ "$n" = tsa-otra ] && echo -otra).pem
signer_key = $PKI/$n.key
signer_digest = sha256
default_policy = 1.2.3.4.1
other_policies = 1.2.3.4.2
digests = sha256, sha384, sha512
accuracy = secs:1
ordering = no
tsa_name = no
ess_cert_id_chain = no
ess_cert_id_alg = sha256
crypto_device = builtin
CNF
done

# Anclas que se commitean: SOLO certificados públicos.
cp "$PKI/raiz-a.crt" "$AQUI/anclas/raiz-a.crt"
cp "$PKI/raiz-b.crt" "$AQUI/anclas/raiz-b.crt"
cp "$PKI/raiz-sello.crt" "$AQUI/anclas/raiz-sello.crt"
cp "$PKI/raiz-sello-otra.crt" "$AQUI/anclas/raiz-sello-otra.crt"

# --- 2. El PDF base, de LibreOffice --------------------------------------------
# Dos páginas: el cambio posterior de «solo la página 2» se distingue del de la 1.
cat >"$T/contrato.fodt" <<'XML'
<?xml version="1.0" encoding="UTF-8"?>
<office:document xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
 xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0"
 xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0"
 xmlns:dc="http://purl.org/dc/elements/1.1/"
 office:version="1.3" office:mimetype="application/vnd.oasis.opendocument.text">
 <office:meta><dc:title>Contrato de prueba</dc:title></office:meta>
 <office:automatic-styles>
  <style:style style:name="Salto" style:family="paragraph">
   <style:paragraph-properties fo:break-before="page"/>
  </style:style>
 </office:automatic-styles>
 <office:body><office:text>
  <text:h text:outline-level="1">Contrato de arrendamiento de prueba</text:h>
  <text:p>Primera página: las partes acuerdan el alquiler de un local por doce meses.</text:p>
  <text:p text:style-name="Salto">Segunda página: la renta mensual es de mil euros.</text:p>
 </office:text></office:body>
</office:document>
XML
soffice -env:UserInstallation="file://$T/lo-perfil" --headless --convert-to pdf:writer_pdf_Export \
  --outdir "$T/out" "$T/contrato.fodt" >/dev/null 2>&1
mv "$T/out/contrato.pdf" "$T/base-libreoffice.pdf"

# --- 3. pyHanko ----------------------------------------------------------------
VENV="${SYTUE_PDF_VENV:-$T/venv}"
if [ ! -x "$VENV/bin/python" ]; then
  python3 -m venv "$VENV"
  "$VENV/bin/pip" install -q "${PYHANKO_PINS[@]}"
fi

"$VENV/bin/python" "$AQUI/generar.py" --pki "$PKI" --base "$T/base-libreoffice.pdf" \
  --salida "$AQUI" --trabajo "$T"

echo "generados: $(find "$AQUI" -maxdepth 1 -name '*.pdf' | wc -l) PDF, $(cat "$AQUI"/*.pdf | wc -c) bytes"
