<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos -->

# Sellado de tiempo de las firmas

## El hueco

Las imágenes se firman con `cosign` y una clave propia, sin subir nada al registro
público de transparencia. Esa decisión está razonada en `deploy/production/03-desplegar.sh`
y no se discute aquí: publicar en Rekor exponía el inventario de servicios y la
cadencia de releases de un producto que aún no ha salido.

Lo que esa decisión deja abierto, y no estaba escrito en ninguna parte: **una firma
no prueba cuándo se hizo**. Si la clave de release se compromete, no hay forma de
distinguir lo que se firmó antes del robo de lo que se firmó después. La respuesta a
ese incidente pasa a ser revocar y volver a firmar TODO, sin poder salvar nada, y sin
poder demostrar a un cliente que la versión que tiene desplegada es anterior al robo.

## Lo que lo cubre, y lo que no

Un sello de tiempo RFC 3161 dice «esta firma existía a esta hora» y nada más: no
lleva el nombre de la imagen ni su digest, así que **no reintroduce el problema que
nos sacó de Rekor**. No es un registro público: es un tercero de confianza que
contrafirma un hash.

No sustituye a la custodia de la clave. Con la clave robada se pueden seguir firmando
imágenes nuevas; lo que se gana es poder acotar el daño en el tiempo en vez de dar por
comprometido todo lo firmado desde siempre.

## Receta, probada con cosign 3.1.3

Verificada de punta a punta el 2026-08-18 contra una TSA pública de pruebas.

Firmar —el sello OBLIGA al formato nuevo de bundle, no es opcional—:

```bash
cosign sign-blob --key "$CLAVE" --tlog-upload=false --use-signing-config=false --yes \
  --timestamp-server-url "$SYTUE_TSA_URL" \
  --new-bundle-format --bundle firma.bundle artefacto
```

Construir el material de confianza. **La cadena va con la HOJA PRIMERO**: si se pone
la CA raíz al principio, `trusted-root create` falla con «first certificate in chain
must be a leaf certificate», que no dice en ningún momento que el problema sea el
orden.

```bash
cat tsa-hoja.crt tsa-ca.pem > tsa-cadena.pem
cosign trusted-root create --no-default-fulcio --no-default-rekor --no-default-ctfe \
  --tsa="url=$SYTUE_TSA_URL,certificate-chain=tsa-cadena.pem" --out trusted-root.json
```

Verificar:

```bash
cosign verify-blob --key cosign.pub --bundle firma.bundle \
  --trusted-root trusted-root.json --use-signed-timestamps \
  --insecure-ignore-tlog=true artefacto
```

`--insecure-ignore-tlog` sigue haciendo falta aunque haya sello: sin Rekor, cosign
exige inclusión en el log y falla con «not enough verified log entries from
transparency log: 0 < 1». El sello de tiempo no lo sustituye.

## Lo que falta para activarlo

1. **Elegir una TSA cualificada de la UE.** Es una decisión de negocio: hay
   prestadores cualificados eIDAS en España, Italia, Alemania y Polonia, con contrato
   y coste. La de pruebas usada para validar la receta no sirve para producción, ni
   por jurisdicción ni por garantías.
2. **Migrar las firmas al formato bundle**, que es lo que el sello obliga. Eso toca
   `sign`, `attest` y la verificación de `03-desplegar.sh` A LA VEZ, más los cuatro
   sitios que vigila `release-workflow-test.sh`. De regalo desaparece el aviso de
   `--tlog-upload` deprecado.
3. **Probarlo entero en el efímero**, no en una release. Si la verificación falla tras
   el cambio de formato, no se despliega nada.

## Custodia de la clave

`COSIGN_PRIVATE_KEY` y `COSIGN_PASSWORD` viven como secretos del repositorio en la
forja, así que quien comprometa la forja tiene la clave con la que se firma
producción. Mitigación aplicada: **entregar el secreto solo a quien lo necesita**.

Los únicos workflows que firman son cuatro, y solo con dos eventos:

| | |
| --- | --- |
| Workflows | `release.yml`, `imagenes-rama.yml`, `espejo-upstream.yml`, `release-desktop.yml` |
| Eventos | `push`, `workflow_dispatch` |

Con eso, una pull request —propia o de un fork— y cualquier workflow nuevo dejan de
poder leer la clave de firma. Se configura en cada secreto desde la forja, en
«Limitar a eventos» y «Workflows».

Lo que NO resuelve: quien tenga acceso administrativo a la forja sigue pudiendo
leerla. Eliminar eso requiere una clave no extraíble en un HSM o KMS, y cosign no
soporta hoy el Key Manager de Scaleway.
