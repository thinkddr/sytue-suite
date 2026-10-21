#!/bin/sh
# ---------------------------------------------------------------------------
# Sytue One dev — crea los buckets S3 en MinIO (equivalentes a los buckets
# versionados de Scaleway Object Storage en prod). Idempotente.
#
# Blobs grandes (ficheros de Drive, adjuntos de Mail, documentos de Docs,
# build-cache del PaaS, backups) en S3. El estado vive en PostgreSQL.
# Ver docs/CONVENTIONS.md §0, §2.
#
# CON `curl`, NO CON el cliente `mc`. Es AGPL-3.0 —prohibida por el firewall
# clean-room— y su repositorio está ARCHIVADO desde noviembre de 2025:
# lo que hace es exactamente lo que este fichero necesitaba, y nada más, así que
# se traducen sus tres órdenes a las peticiones S3 que son por debajo. `curl`
# firma SigV4 desde la 7.75 y ya está en el catálogo (`managed-http-task`), así
# que esto NO añade una imagen de terceros: quita una.
# ---------------------------------------------------------------------------
set -eu

S3="${S3_ENDPOINT:-http://minio:9000}"
# `us-east-1` es la región que MinIO acepta por defecto; en Scaleway sería fr-par.
REGION="${S3_REGION:-us-east-1}"

# La credencial va por STDIN (`--config -`) y no en la línea de órdenes: con
# `--user` queda visible en `ps` para cualquier proceso de la máquina. Aquí es
# dev, pero el patrón se copia, y es lo que levanta el escáner de secretos.
s3() { # s3 <método> <ruta> [cuerpo]
  printf 'user = "%s:%s"\n' "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}" |
  curl -sS --config - --fail-with-body --max-time 30 \
    --aws-sigv4 "aws:amz:${REGION}:s3" \
    -X "$1" "${S3}$2" ${3:+--data "$3"} -o /dev/null -w '%{http_code}'
}

for bucket in \
  sytue-dev-drive-blobs \
  sytue-dev-mail-blobs \
  sytue-dev-docs-blobs \
  sytue-dev-cast-medios \
  sytue-dev-pages-sitios \
  sytue-dev-cloud \
  sytue-dev-build-cache \
  sytue-dev-backups \
  sytue-dev-tfstate
do
  # Crear es idempotente A MANO: S3 responde 409 si el bucket ya es tuyo, y eso
  # NO es un error aquí —este guion corre en cada `docker compose up`—. Cualquier
  # otro código sí lo es, y por eso no se traga la salida entera.
  codigo="$(s3 PUT "/${bucket}" || true)"
  case "$codigo" in
    200 | 409) ;;
    *)
      echo "[sytue] ERROR creando ${bucket}: HTTP ${codigo}" >&2
      exit 1
      ;;
  esac

  # Versionado (paridad con los buckets versionados de prod) en datos/backups/tfstate.
  case "$bucket" in
    sytue-dev-drive-blobs|sytue-dev-mail-blobs|sytue-dev-docs-blobs|sytue-dev-backups|sytue-dev-tfstate)
      codigo="$(s3 PUT "/${bucket}?versioning" \
        '<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>' || true)"
      case "$codigo" in
        200 | 204) ;;
        *)
          echo "[sytue] ERROR activando versionado en ${bucket}: HTTP ${codigo}" >&2
          exit 1
          ;;
      esac
      ;;
  esac
  echo "[sytue] bucket listo: ${bucket}"
done

echo "[sytue] MinIO: buckets dev creados."
