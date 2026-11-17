#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# PUBLICA LA IMAGEN DE UN SERVICIO GO PARA LAS ARQUITECTURAS QUE SE LE PIDAN,
# sin demonio.
#
# QUIÉN DECIDE CUÁLES: `scripts/arquitecturas-imagen.sh`, que es de donde saca
# `ARQUITECTURAS` la release. Hoy todos los servicios van solo al piloto
# (amd64), así que aquí se construye UNA y es la cruzada. El defecto de este
# guion sigue siendo las dos, la nativa primero.
#
# EL PROBLEMA QUE RESUELVE. Los Dockerfiles no fijan plataforma, así que cada
# imagen salía con la arquitectura del runner que la construyó. Cuando los tres
# runners se mudaron a `sytue-rooms-media` —ARM64— la tubería empezó a publicar
# arm64 para un servidor que es x86_64. Una imagen de otra arquitectura NO falla
# al descargarse: falla al ARRANCAR, con `exec format error`, lejísimos de su
# causa. Medido el 2026-09-21 en `sytue-id`: v3.1.13 arm64, v3.2.0 amd64, v3.2.2
# y v3.2.3 arm64, y el servidor llevaba cuatro versiones sin poder actualizarse
# sin que nada saliera en rojo.
#
# POR QUÉ NO `--platform` NI QEMU. Emular amd64 con QEMU para compilar Go son 27
# imágenes a velocidad de emulación: no cabe en ninguna ventana de release. Y los
# Dockerfiles no cruzan (`go build` sin `$TARGETARCH`), así que pedirle a BuildKit
# otra plataforma sería emular.
#
# LO QUE SÍ. Un binario de Go se cruza NATIVO —`GOARCH=amd64 go build` en el
# runner ARM cuesta lo mismo que el nativo: medido, 64 s las DOS arquitecturas de
# sytue-id, descarga de módulos incluida— y la imagen se monta con `crane`, que
# habla con el registro por HTTPS y no necesita demonio. La base
# `gcr.io/distroless/static:nonroot` que usan los servicios es un índice
# multiarquitectura (amd64, arm64, arm, s390x, ppc64le, riscv64), así que la
# variante amd64 sale del MISMO digest ya fijado: no se toca el pinning.
#
# EL DOCKERFILE SIGUE MANDANDO. Todo lo que va en la configuración de la imagen
# —base, binarios, ENTRYPOINT, USER, WORKDIR, EXPOSE, ENV— se LEE de él, no se
# repite aquí: un segundo catálogo diverge solo, y el síntoma sería un pod que
# arranca con el entrypoint de otra versión. El Dockerfile se sigue usando tal
# cual para construir en local y en el servidor (bin/build-images.sh).
#
# Y SU ETAPA DE COMPILACIÓN NO SE PIERDE EN SILENCIO. Aquí no se ejecuta, así que
# cada RUN suyo tiene que ser algo que este guion sabe reproducir —`go mod
# download`, el `go build` del servicio, la generación de scripts/client-core-assets.sh—
# o la publicación se para diciendo cuál. La v3.20.3 salió con Rooms sin el núcleo
# de cliente que incrusta su binario, y la web sin cifrado de extremo a extremo
# (docs/operations.md#client-core-vacio).
#
# USO:
#   DESTINO=ghcr.io/thinkddr TAG=v3.2.4 COMMIT=$(git rev-parse HEAD) \
#     scripts/publicar-imagen-go.sh sytue-id services/id/Dockerfile
#
# Deja publicado el índice `$DESTINO/$nombre:$TAG` con las dos arquitecturas
# dentro, y las dos imágenes sueltas en `:$TAG-amd64` y `:$TAG-arm64`. Las
# etiquetas por arquitectura NO son basura: son las que hacen falta para montar
# el índice y las que se miran cuando hay que saber qué se publicó de verdad.
#
# ENSAYO, sin registro ni red:
#   ENSAYO=/tmp/ensayo TAG=v0 ARQUITECTURAS=amd64 \
#     scripts/publicar-imagen-go.sh sytue-rooms services/rooms/Dockerfile
# hace todo lo que no habla con el registro —lo que genera la etapa de compilación,
# los binarios cruzados y el contenido de la capa de cada arquitectura— y lo deja en
# `$ENSAYO/<arq>/` (p. ej. /tmp/ensayo/amd64/app/rooms). No hace falta DESTINO.
set -euo pipefail

# shellcheck source=scripts/lib-portabilidad.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib-portabilidad.sh"
construir="$(dirname "${BASH_SOURCE[0]}")/construir-imagen.sh"

nombre="${1:?uso: publicar-imagen-go.sh <nombre-imagen> <dockerfile>}"
df="${2:?uso: publicar-imagen-go.sh <nombre-imagen> <dockerfile>}"
ENSAYO="${ENSAYO:-}"
if [ -n "$ENSAYO" ]; then DESTINO="${DESTINO:-ensayo.invalid}"; fi
: "${DESTINO:?falta DESTINO (p. ej. ghcr.io/thinkddr)}"
: "${TAG:?falta TAG}"
COMMIT="${COMMIT:-}"
# LA NATIVA VA PRIMERA, y no es un detalle de estilo: es la que sirve de base
# cuando hay que caer al camino del Dockerfile entero (ver más abajo), donde
# compilar en la otra arquitectura costaría una emulación.
nativa="$(go env GOARCH)"
if [ -z "${ARQUITECTURAS:-}" ]; then
  case "$nativa" in
    arm64) ARQUITECTURAS="arm64 amd64" ;;
    *) ARQUITECTURAS="amd64 arm64" ;;
  esac
fi

raiz="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$raiz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# ---------------------------------------------------------------- el Dockerfile
# La etapa de runtime es la que empieza en el ÚLTIMO FROM: lo de antes son las de
# compilación, que aquí no se ejecutan.
desde="$(grep -n '^FROM ' "$df" | tail -1 | cut -d: -f1)"
runtime="$(tail -n "+$desde" "$df")"
# Sin `printf … | head -1`: bash escribe línea a línea y, si head cierra antes de que
# acabe, printf recibe SIGPIPE; con pipefail la asignación falla y `set -e` sale SIN
# decir nada. Pasó en el CI con el frontal cargado (security 37419208658, sytue-bases).
read -r _ base _ <<<"$runtime"

# Los paquetes que se compilan salen de las rutas que el propio Dockerfile pasa a
# `go build`. El nombre del binario es el del directorio, que es lo que hace Go.
mapfile -t paquetes < <(grep -oE '\./services/[a-z0-9-]+/cmd/[a-z0-9-]+' "$df" | sort -u)
[ "${#paquetes[@]}" -gt 0 ] || { echo "$nombre: no encuentro ningún ./services/*/cmd/* en $df" >&2; exit 1; }

# Si el Dockerfile hornea la versión con -ldflags -X, se hornea igual aquí: hay
# servicios que leen su versión de ahí y no del entorno.
#
# Las comillas simples se quitan ANTES de buscar. Sin eso, un Dockerfile que
# escribe `-X 'main.version=${VERSION}'` —como hacía el de `docs`, donde los
# demás no las ponen— casa hasta `${VERSION}` y se trae la comilla de apertura
# sin la de cierre. El `go build` de dentro muere entonces con «invalid value
# "-s -w -X 'main.version=v3.3.6 " for flag -ldflags: unterminated ' string», y
# el error no menciona ni el Dockerfile ni este guion. Tumbó la publicación de
# v3.3.6. Quitarlas aquí vale para los dos estilos y no hay un tercero: lo que
# va dentro de un `-X` es `paquete.Símbolo=valor`, sin espacios.
equis="$(tr -d "'" < "$df" | grep -oE '\-X [^ "]+=\$\{VERSION\}' | sed "s/\${VERSION}/$TAG/" | tr '\n' ' ' || true)"

# QUÉ VA DENTRO DE LA IMAGEN, leído de los COPY de la etapa de runtime. Los que
# traen un binario compilado se rellenan aquí; cualquier otro lo produce la
# etapa de compilación del Dockerfile y NO se puede reproducir sin ejecutarla.
declare -a destinos=()
ajeno=0
while read -r origen destino; do
  binario="$(basename "$origen")"
  encaja=0
  for p in "${paquetes[@]}"; do [ "$(basename "$p")" = "$binario" ] && encaja=1; done
  if [ "$encaja" = 1 ]; then destinos+=("$binario:$destino"); else ajeno=1; fi
done < <(printf '%s\n' "$runtime" | grep -E '^COPY --from=' | awk '{print $3" "$4}')
# Un COPY/ADD del contexto también es algo que no es un binario: sin esto, el
# camino normal lo tiraba sin decir nada.
# (Sin `| grep -q`: con pipefail, el SIGPIPE de quien escribe convierte el acierto en
# fallo y el guion caía a veces con un Dockerfile cualquiera, docs/operations.md#ci-sin-azar.)
del_contexto="$(grep -E '^(COPY|ADD) ' <<<"$runtime" | grep -v -- '--from=' || true)"
[ -z "$del_contexto" ] || ajeno=1

# LO QUE LA ETAPA DE RUNTIME EJECUTA. Un RUN —el `apt-get install ffmpeg` del
# transcodificador de Cast— deja en la imagen ficheros de la arquitectura en la
# que corrió. El camino normal (`crane append` sobre la base) no lo ejecutaba: la
# v3.8.2 publicó `sytue-cast-transcoder` como debian-slim + binario, sin ffmpeg,
# y el pod murió en CrashLoopBackOff. Y el de Docker nativo + binarios cruzados
# encima daría un ffmpeg arm64 con la etiqueta amd64. Así que esa etapa se
# construye con Docker PARA CADA arquitectura, pero ella sola: los binarios ya
# cruzados entran por el contexto y lo único emulado (qemu-user-static en el
# frontal, ver deploy/rooms-sfu/instalar-docker-ci.sh) son sus RUN, no `go build`.
ejecuta=0
grep -qE '^RUN ' <<<"$runtime" && ejecuta=1

# --------------------------------------------- lo que ejecuta la compilación
# LA ETAPA DE COMPILACIÓN NO SE EJECUTA AQUÍ, así que cada RUN suyo tiene que ser
# algo que este guion sabe hacer por su cuenta, o se para. La v3.20.3 salió con
# Rooms contestando 503 «client core unavailable» —la web no podía configurar el
# cifrado E2E— porque el RUN que generaba los WebAssembly que incrusta su binario
# estaba ahí y aquí se saltaba sin decir nada (docs/operations.md#client-core-vacio).
# Lo que se sabe hacer:
#   · `go mod download`: el `go build` de abajo descarga lo que haga falta.
#   · el `go build` de los binarios del servicio, SOLO: es el que se cruza abajo,
#     con su `-ldflags -X` y sus `-tags`. Encadenado a otra orden no se reconoce.
#   · `scripts/client-core-assets.sh <dir>`: se ejecuta el mismo guion, UNA vez y
#     antes de cruzar —los wasm no dependen de la arquitectura—.
#   · otro RUN DESPUÉS del `go build`, solo si la imagen pasa por su Dockerfile
#     (`ajeno`): no puede entrar en el binario, y lo que deja llega a la imagen por
#     un COPY --from que construye BuildKit. Es la tabla de países de Analytics.
# Las instrucciones se leen como las lee Docker: continuaciones `\` unidas y sin
# las líneas de comentario, que pueden ir en medio de una continuación.
declare -a generar=()
etiquetas=""
compilado=0
re_genera='^(bash )?scripts/client-core-assets\.sh ([^ ;&|]+)$'
re_compila='^([A-Z_]+=[^ ]+ )*go build .*\./services/[a-z0-9-]+/cmd/'
while IFS= read -r instruccion; do
  case "$instruccion" in "RUN "*) ;; *) continue ;; esac
  orden="${instruccion#RUN }"
  # `--mount=…`, `--network=…`: cambian dónde corre, no qué.
  while [[ "$orden" == --*" "* ]]; do orden="${orden#* }"; done
  if [ "$orden" = "go mod download" ]; then
    continue
  elif [[ "$orden" =~ $re_genera ]]; then
    generar+=("${BASH_REMATCH[2]}")
  elif [[ "$orden" =~ $re_compila ]] && [[ "$orden" != *[\;\&\|]* ]]; then
    compilado=1
    etiquetas="$({ grep -oE -- '-tags[= ]+[^ ]+' <<<"$orden" || true; } | tail -1 | sed -E "s/^-tags[= ]+//; s/[\"']//g")"
  elif [ "$compilado" = 1 ] && [ "$ajeno" = 1 ]; then
    echo "   $nombre: «${orden:0:50}…» va tras el go build y lo ejecuta su Dockerfile"
  else
    {
      echo "$nombre: la etapa de compilación de $df ejecuta algo que este guion no reproduce:"
      echo "    RUN $orden"
      echo "  La release monta la imagen sin ejecutar esa etapa, así que la imagen saldría sin"
      echo "  lo que haga (la v3.20.3 salió así sin el núcleo de cliente de Rooms)."
      echo "  Si genera algo que el binario necesita, que lo haga un guion de scripts/ que"
      echo "  llamen el Dockerfile y este guion (como scripts/client-core-assets.sh)."
    } >&2
    exit 1
  fi
done < <(head -n "$((desde - 1))" "$df" | awk '
  /^[[:space:]]*#/ { next }
  { linea = $0
    if (sub(/\\[[:space:]]*$/, "", linea)) { acumulado = acumulado linea " "; next }
    print acumulado linea; acumulado = "" }
  END { if (acumulado != "") print acumulado }' | tr -s ' \t' ' ' | sed 's/ $//')

# ------------------------------------------------------- la configuración final
# `crane append` conserva la configuración de la base, que es la de distroless:
# lo que el Dockerfile añade encima hay que ponerlo. Se lee de él, no se escribe.
declare -a mutaciones=()
entrada="$(printf '%s\n' "$runtime" | grep -E '^ENTRYPOINT' | tail -1 | grep -oE '"[^"]+"' | tr -d '"' | paste -sd, - || true)"
[ -n "$entrada" ] && mutaciones+=(--entrypoint "$entrada")
trabajo="$(printf '%s\n' "$runtime" | grep -E '^WORKDIR ' | tail -1 | awk '{print $2}' || true)"
[ -n "$trabajo" ] && mutaciones+=(--workdir "$trabajo")
usuario="$(printf '%s\n' "$runtime" | grep -E '^USER ' | tail -1 | awk '{print $2}' || true)"
[ -n "$usuario" ] && mutaciones+=(--user "$usuario")
# EXPOSE 8086 25 465 587 993 → 8086/tcp,25/tcp,… (tcp es el defecto de Docker).
puertos="$(printf '%s\n' "$runtime" | grep -E '^EXPOSE ' | sed 's/^EXPOSE //' | tr ' ' '\n' |
  grep -E '^[0-9]' | sed 's#$#/tcp#' | paste -sd, - || true)"
[ -n "$puertos" ] && mutaciones+=(--exposed-ports "$puertos")
while read -r asignacion; do
  [ -n "$asignacion" ] || continue
  mutaciones+=(-e "$asignacion")
done < <(printf '%s\n' "$runtime" | grep -E '^ENV [A-Z_]+=' | sed 's/^ENV //' |
  sed "s/\${VERSION}/$TAG/g; s/\${COMMIT}/$COMMIT/g")

# ------------------------------------------------------------------ construcción
# Lo que el binario incrusta, antes de cruzarlo: una vez, vale para todas.
for dir in ${generar[@]+"${generar[@]}"}; do
  echo "── $nombre: núcleo de cliente en $dir"
  bash scripts/client-core-assets.sh "$dir"
done

declare -a publicadas=()
read -ra arquitecturas <<<"$ARQUITECTURAS"
for arq in "${arquitecturas[@]}"; do
  ref="$DESTINO/$nombre:$TAG-$arq"
  echo "── $nombre ($arq)"

  mkdir -p "$tmp/bin-$arq"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arq" \
    go build -trimpath ${etiquetas:+-tags "$etiquetas"} -ldflags "-s -w $equis" -o "$tmp/bin-$arq/" "${paquetes[@]}"

  etapa="$tmp/fs-$arq"
  for d in "${destinos[@]}"; do
    # `install -D` es de GNU y el `install` de BSD NO lo tiene, así que en un Mac
    # esto moría con «target directory does not exist» y dejaba en rojo el hook de
    # pre-push —el de TODOS, porque `publicar-imagen-go-test.sh` ejecuta el guion—
    # mientras en Linux salía verde. Un gate que solo falla en la máquina de quien
    # empuja acaba normalizando el `--no-verify`, que es peor que no tenerlo.
    mkdir -p "$(dirname "$etapa/${d#*:}")"
    install -m 0755 "$tmp/bin-$arq/${d%%:*}" "$etapa/${d#*:}"
  done
  mapfile -t raices < <(cd "$etapa" && ls)
  tar --owner=0 --group=0 --numeric-owner -C "$etapa" -cf "$tmp/capa-$arq.tar" "${raices[@]}"

  if [ -n "$ENSAYO" ]; then
    mkdir -p "$ENSAYO/$arq"
    cp -R "$etapa/." "$ENSAYO/$arq/"
    echo "   ensayo: la capa de $arq queda en $ENSAYO/$arq (nada publicado)"
    [ "$ajeno$ejecuta" = 00 ] ||
      echo "   ensayo: $nombre lleva algo que construye su Dockerfile; eso no está en el ensayo"
    continue
  fi

  if [ "$ejecuta" = 1 ]; then
    # La base por el digest de SU arquitectura, no el del índice: el constructor
    # clásico con el almacén de containerd ignoraba `--platform` ante un índice y
    # resolvía la nativa (medido en el frontal: `uname -m` → aarch64). BuildKit sí
    # lo respeta, pero con el digest de la variante no depende de nadie.
    ctx="$tmp/ctx-$arq"
    mkdir -p "$ctx"
    digest="$(timeout 300 crane digest --platform "linux/$arq" "$base" 2>/dev/null || true)"
    if [ -z "$digest" ]; then
      case "${base%@*}@$arq" in
        debian:bookworm-slim*@amd64) digest="sha256:f3034a6ec3c1205360777c4aae76234998866ad18806ae62b63a3f84ccad782b" ;;
        debian:bookworm-slim*@arm64) digest="sha256:0c8bbb8e987a035fe1d9704eb2e571b7e9a836e1caa46345290674b45b69e417" ;;
        *) digest="$(timeout 300 crane digest --platform "linux/$arq" "$base")" ;;
      esac
    fi
    { echo "FROM ${base%@*}@$digest"
      printf '%s\n' "$runtime" | tail -n +2 | while IFS= read -r linea; do
        case "$linea" in
          "COPY --from="*)
            read -r _ _ origen destino <<<"$linea"
            binario="$(basename "$origen")"
            [ -f "$tmp/bin-$arq/$binario" ] || {
              echo "$nombre: la etapa de runtime tiene RUN y copia $origen, que no es un binario de Go: no sé construirla por arquitectura" >&2
              exit 1
            }
            cp "$tmp/bin-$arq/$binario" "$ctx/$binario"
            echo "COPY $binario $destino" ;;
          "COPY "* | "ADD "*)
            echo "$nombre: la etapa de runtime tiene RUN y copia del contexto ($linea): no sé construirla por arquitectura" >&2
            exit 1 ;;
          *) printf '%s\n' "$linea" ;;
        esac
      done; } >"$ctx/Dockerfile"
    # Con el BuildKit del CI (ADR 0066); sus RUN de otra arquitectura los emula el
    # qemu-user-static del frontal (binfmt con la bandera F, que vale dentro de su
    # espacio de usuario).
    reintentar 3 15 timeout 3000 "$construir" --platform "linux/$arq" --build-arg "VERSION=$TAG" \
      --build-arg "COMMIT=$COMMIT" -f "$ctx/Dockerfile" --push "$ref" "$ctx"
  elif [ "$ajeno" = 0 ]; then
    # EL CAMINO NORMAL: la imagen entera es la base + los binarios.
    timeout 600 crane append --platform "linux/$arq" -b "$base" -f "$tmp/capa-$arq.tar" -t "$ref"
    timeout 300 crane mutate "$ref" -t "$ref" --set-platform "linux/$arq" "${mutaciones[@]}"
  elif [ "${#publicadas[@]}" -eq 0 ] && [ "$arq" != "$nativa" ]; then
    # EL SERVICIO LLEVA ALGO QUE NO ES UN BINARIO y la lista NO trae la nativa
    # (Analytics en el perfil completo va solo a amd64). Construir el Dockerfile aquí
    # daría una imagen de la arquitectura del runner con la etiqueta de otra: la v3.7.13
    # publicó `sytue-analytics:…-amd64` siendo arm64. La base se construye en la
    # nativa con su propia etiqueta y los binarios cruzados se montan encima.
    base_nativa="$DESTINO/$nombre:$TAG-base-$nativa"
    # Construir y subir es UNA orden (BuildKit sube desde su caché, sin daemon), así
    # que el reintento la repite entera: si falló el push, lo construido ya está en
    # caché y la repetición solo vuelve a subir.
    reintentar 3 15 timeout 3000 "$construir" --build-arg "VERSION=$TAG" --build-arg "COMMIT=$COMMIT" \
      -f "$df" --push "$base_nativa" .
    timeout 600 crane append -b "$base_nativa" -f "$tmp/capa-$arq.tar" -t "$ref"
    timeout 300 crane mutate "$ref" -t "$ref" --set-platform "linux/$arq"
  elif [ "${#publicadas[@]}" -eq 0 ]; then
    # EL SERVICIO LLEVA ALGO QUE NO ES UN BINARIO. Eso lo produce la etapa de
    # compilación del Dockerfile y reproducirlo aquí sería copiarlo: la
    # primera arquitectura —la NATIVA— se construye con su Dockerfile entero.
    reintentar 3 15 timeout 3000 "$construir" --build-arg "VERSION=$TAG" --build-arg "COMMIT=$COMMIT" \
      -f "$df" --push "$ref" .
  else
    # …y la cruzada se monta ENCIMA de ella, cambiando solo los binarios. La
    # base es la del otro índice; funciona porque distroless no trae un solo
    # ejecutable.
    # ponytail: techo conocido. Se quita el día que la parte que no es binario
    # salga de un guion que compartan el Dockerfile y este fichero.
    timeout 600 crane append -b "${publicadas[0]}" -f "$tmp/capa-$arq.tar" -t "$ref"
    timeout 300 crane mutate "$ref" -t "$ref" --set-platform "linux/$arq"
  fi
  publicadas+=("$ref")
done

[ -z "$ENSAYO" ] || exit 0

# EL ÍNDICE ES LO QUE ARREGLA EL PROBLEMA: un solo `:$TAG` que sirve a los dos
# destinos, así que el servidor (x86_64) y sytue.com (ARM64) piden lo mismo y
# cada uno se lleva lo suyo. Sin él habría que acordarse de escribir el sufijo.
declare -a manifiestos=()
for ref in "${publicadas[@]}"; do manifiestos+=(-m "$ref"); done
timeout 300 crane index append "${manifiestos[@]}" -t "$DESTINO/$nombre:$TAG"
echo "   ✓ $DESTINO/$nombre:$TAG → ${publicadas[*]}"
