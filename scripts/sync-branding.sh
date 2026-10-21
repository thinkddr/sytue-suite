#!/usr/bin/env bash
# Sincroniza los assets de marca canónicos (branding/) dentro del frontend.
# branding/ es la FUENTE DE VERDAD; esta copia vendorizada permite que el bundler
# (Astro/Vite/Bun) resuelva SVG y tokens en dev y dentro de las imágenes Docker
# del frontend (contexto apps/frontend, sin acceso a branding/ raíz).
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
# ImageMagick hace falta para los `.ico` de abajo. Se comprueba AQUÍ y se aborta,
# en vez de saltarse esa parte con un aviso: un `.ico` que no se regenera no da
# ningún error —el fichero viejo sigue ahí y el sitio funciona— y vuelve a pasar
# exactamente lo que este script existe para impedir, que es que una superficie
# se quede con la familia de iconos anterior sin que nadie se entere.
#
# Es la misma exigencia que tenían los iconos del escritorio de Wails: no es una
# dependencia nueva de la cadena, es la que ya se usaba para los iconos nativos.
command -v magick >/dev/null || {
  echo "Falta ImageMagick (magick): sin él no se pueden regenerar los favicon.ico." >&2
  echo "  brew install imagemagick" >&2
  exit 1
}
ui="$root/apps/frontend/packages/ui/src/assets"
cfg="$root/apps/frontend/packages/config/src/tokens"
mkdir -p "$ui/icons" "$ui/logos" "$ui/illustrations" "$cfg"
# EL DESTINO SE VACÍA ANTES DE COPIAR. `cp` sobrescribe pero no borra, así que un
# icono retirado de `branding/icons/` sobrevivía dentro del paquete para siempre,
# sin generador que lo mantuviera: `pdf.svg` y `pdf-sm.svg` se quedaron ahí con un
# color retirado de la paleta y solo los delató el test que compara el fallback de
# cada SVG con su token. Un huérfano no molesta hasta que alguien lo importa.
rm -f "$ui"/icons/*.svg
cp -f "$root"/branding/icons/*.svg "$ui/icons/"
# Y las MARCAS también dentro del paquete. Antes no se vendorizaban porque «nadie
# las importa desde el paquete», y eso dejó al conmutador de aplicaciones con la
# familia ANTERIOR: `branding/icons/` no se toca desde que nació Notes (06/09
# 21:15) y las marcas se rehicieron enteras esa misma noche —logotipo «sy:», luego
# el glifo del producto en vez del monograma, el bocadillo de Rooms y el centrado
# óptico—. Resultado: la pestaña, el dock y el móvil enseñaban un icono y el
# conmutador otro, para el mismo producto.
mkdir -p "$ui/marks"
for m in "$root"/branding/marks/*.svg; do
  case "$m" in *-maskable.svg) continue ;; esac  # la maskable es para el sistema, no para la UI
  cp -f "$m" "$ui/marks/"
done
# Marcas de APLICACION (branding/marks/, generadas por scripts/gen-marks.sh): el
# logotipo «sy:» con el color de cada producto. Es lo que identifica la app en la
# pestana, el dock y el movil; los de icons/ son las teselas del conmutador.
#
# NO se vendorizan dentro de @sytue/ui como los iconos: nadie las importa desde el
# paquete. Van directas a `public/` de cada superficie, que es donde el navegador y
# los manifiestos PWA las buscan por una ruta fija.
#
# Se COPIA en vez de importarse desde @sytue/ui porque tiene que quedar en una ruta
# publica ESTABLE: los manifiestos PWA y los service workers de portal, rooms y
# cloud-dashboard nombran `/favicon.svg` como texto y no pueden importar nada; un
# `?url` con hash ahi no vale. Copiarla desde la familia generada es lo que impide
# que vuelvan a derivar — los favicons escritos a mano acabaron con siete colores
# de ocho fuera de la paleta (calendar en #7c5cff con la paleta diciendo #7e22ce).
#
# `www` es el sitio de MARCA, no un producto: lleva el logotipo de la casa (grafito
# y laton), que es distinto de los quince colores de producto.
apps="$root/apps/frontend/apps"

# EL .ico NO ES NOSTALGIA. El SVG lo declara el HTML y basta para un navegador
# que lo parsea; pero cuando algo pide `/favicon.ico` a pelo —previsualizacion de
# un enlace, lector de feeds, atajo del sistema, anclaje a la barra de tareas de
# Windows— no hay HTML que leer. Sin fichero eso es un 404 y un icono en blanco.
#
# Sytue ID ya lo resolvio (services/id/internal/http/favicon.go) con un 301 al
# SVG. Aqui se genera el binario de verdad en vez de redirigir por una razon
# concreta: Windows NO acepta un SVG para anclar a la barra de tareas, y esa es
# justo la superficie que solo pide `.ico`.
#
# `magick` y no otra cosa: es la MISMA herramienta y los MISMOS parametros que
# usaban los iconos del escritorio de Wails, asi que no entra ninguna dependencia
# nueva en la cadena.
ico_desde_svg() { # $1 svg de origen  $2 .ico destino
  # Densidad alta y luego reduccion = oversampling: sin ella el rasterizador deja
  # motas en los tamanos pequenos, que son justo los que se ven en la pestana.
  magick -background none -density 2048 "$1" -resize 256x256 PNG32:"$2.png"
  # Multi-resolucion en un solo fichero: Windows elige 16 para la pestana y 256
  # para el anclaje. Con un unico tamano, el escalado lo hace el SO y se nota.
  # Solo hasta 64: los tamaños de pestaña, barra de tareas y acceso directo. Con 256
  # y 128 sin comprimir dentro, cada .ico pesaba 370 KB y se descargaba en cada
  # visita; lo grande ya lo dan el SVG, el apple-touch-icon y las marcas PNG.
  magick "$2.png" -define icon:auto-resize=64,48,32,16 "$2"
  rm -f "$2.png"
}

# EL apple-touch-icon TIENE QUE SER PNG. Safari (macOS e iOS) no rasteriza SVG
# ni para la pestaña ni para «añadir a inicio»: Rooms y Cloud lo declaraban como
# SVG y las otras ocho apps no lo declaraban, así que Safari caía al
# `/apple-touch-icon.png` de la raíz del host, que no existía. Medido el
# 2026-09-08: en Safari solo tenían icono las superficies con `.ico` en la raíz.
#
# Se parte de la variante MASKABLE (color a sangre, sin esquinas): iOS recorta
# con su propia máscara y rellena de negro lo transparente, así que la marca
# con tesela redondeada saldría con las esquinas negras.
# SIN LA FECHA DENTRO DEL PNG. ImageMagick escribe la hora de creación en el
# fichero, así que dos ejecuciones seguidas daban bytes distintos con la misma
# entrada. Eso rompía `lint:marca` —que compara lo versionado con lo que producen
# los generadores— en CUALQUIER rama, tocara lo que tocara el commit: el gate no
# lo podía pasar nadie, y un gate que siempre falla se acaba ignorando.
#
# Es el mismo arreglo que `gen-marks.sh` ya tenía para los cuatro iconos PWA
# (`SIN_FECHA`, allí); aquí faltaba. Medido el 2026-09-09: con estos parámetros
# dos pasadas dan bytes idénticos; sin ellos, distintos.
#
# El `.ico` no lo necesita: ese formato no lleva fecha y ya era determinista.
# shellcheck disable=SC2054  # la coma es de ImageMagick («date,time»), no separa elementos
SIN_FECHA=(-strip -define png:exclude-chunk=date,time)

png_apple() { # $1 svg de origen  $2 png destino (180×180)
  magick -background none -density 1024 "$1" -resize 180x180 "${SIN_FECHA[@]}" PNG32:"$2"
}

# Un PNG de 64 px con alfa al lado del ICO (`favicon-64.png`): el formato de icono
# que Safari trata mejor. Se declara con `sizes="64x64"` y WebKit, que ordena los
# iconos de mayor a menor, lo elige antes que el ICO; el ICO sigue para lo que pide
# `/favicon.ico` a pelo y para navegadores viejos. Mismo rasterizado que el ICO.
png_64() { # $1 svg de la marca  $2 destino .png
  magick -background none -density 2048 "$1" -resize 64x64 "${SIN_FECHA[@]}" PNG32:"$2"
}

sync_favicon() { # $1 directorio de la app  $2 fichero de marca  [$3 svg para apple-touch-icon]
  [ -d "$apps/$1" ] || return 0
  mkdir -p "$apps/$1/public"
  cp -f "$2" "$apps/$1/public/favicon.svg"
  ico_desde_svg "$2" "$apps/$1/public/favicon.ico"
  png_64 "$2" "$apps/$1/public/favicon-64.png"
  png_apple "${3:-$2}" "$apps/$1/public/apple-touch-icon.png"
}
# `account:id` — «Mi cuenta» es la cara de Sytue ID y lleva SU marca. Faltaba
# desde que nació la app, así que sus cuatro iconos eran los que se copiaron de
# otra al crearla: Safari enseñaba el de Notepads en su pestaña. Medido el
# 2026-09-12: `account/favicon.ico` y `notes/favicon.ico` tenían el mismo md5, y
# el `favicon-64.png` también. Es el MISMO fallo que SyERP, que se arregló esa
# misma tarde: los iconos aquí no se escriben, se generan.
for par in portal:one console:console cloud-dashboard:cloud mail:mail drive:drive \
           calendar:calendar contacts:contacts notes:notes rooms:rooms \
           photos:photos erp:erp cast:cast pages:pages bases:bases account:id \
           docus:docs calcus:sheets diapos:slides certipdf:pdf; do
  sync_favicon "${par%%:*}" "$root/branding/marks/${par##*:}.svg" "$root/branding/marks/${par##*:}-maskable.svg"
done
# El logotipo de la casa no tiene variante maskable: se usa tal cual.
sync_favicon www "$root/branding/logo/favicon.svg"

# SYTUE ID no es una app de Astro: es un servicio Go que compone su HTML a mano,
# así que su marca se vendoriza AQUÍ y se embebe con `go:embed`.
#
# Y hacía falta: el login del IdP no declaraba ningún icono —comprobado con
# navegador el 2026-09-08— y en la raíz de ese host tampoco había respaldo, así
# que la pantalla que más se ve de toda la suite salía con el icono genérico. Y
# se ve mucho: entrar en `corporate.sytue.eu/mail` sin sesión termina ahí.
cp -f "$root/branding/marks/id.svg" "$root/services/id/internal/http/marca.svg"
# Y su `.ico`, por lo mismo que las once superficies: el IdP servía una redirección
# al SVG, y eso vale para casi todo MENOS para lo único que pide `.ico` de verdad
# —anclar a la barra de tareas de Windows—. Siendo la pantalla más vista de la
# suite, es la que menos sentido tiene dejar a medias.
ico_desde_svg "$root/branding/marks/id.svg" "$root/services/id/internal/http/marca.ico"
# Y el PNG para Safari/iOS, por lo mismo que las once superficies de Astro.
png_apple "$root/branding/marks/id-maskable.svg" "$root/services/id/internal/http/marca-apple.png"

# Y la variante MASKABLE donde el manifiesto PWA la declara: color a sangre, sin
# esquinas —las pone la mascara del sistema— y el monograma en la zona segura.
for par in rooms:rooms cloud-dashboard:cloud; do
  app="${par%%:*}"; marca="${par##*:}"
  [ -d "$apps/$app/public" ] || continue
  cp -f "$root/branding/marks/$marca-maskable.svg" "$apps/$app/public/icon-maskable.svg"
done
cp -f "$root"/branding/logo/*.svg  "$ui/logos/"
# Ilustraciones (empty states, hero, spots) + og/favicon maskable. Superficie
# de servicio consumida por EmptyState y www; barrel en assets/illustrations.ts.
cp -f "$root"/branding/illustrations/*.svg "$ui/illustrations/"
cp -f "$root"/branding/og/*.svg            "$ui/illustrations/"
# theme.css importa la hoja y tokens.ts importa los metadatos canónicos. Las copias
# permiten resolverlos dentro del contexto de build apps/frontend, sin valores a mano.
cp -f "$root"/branding/tokens/*.css "$cfg/"
cp -f "$root"/branding/tokens/colors.json "$root"/branding/tokens/spacing.json "$cfg/"
# Las superficies autónomas incrustan el mismo tema y su inicializador puro.
bun "$root/scripts/sync-ui-standalone.ts"
# Los clientes nativos leen la MISMA hoja (apps/nucleo/tokens la interpreta al arrancar). La copia
# existe porque go:embed no sale de su módulo; un test la compara con esta fuente.
cp -f "$root"/branding/tokens/colors.css "$root/apps/nucleo/tokens/"
# Y la hoja de la forma, que trae el carril de color del menú lateral: las nativas la
# leen encima de colors.css, en el mismo orden en que la carga la web.
cp -f "$root"/branding/tokens/appearance.css "$root/apps/nucleo/tokens/"
# Y las MEDIDAS de la forma (radios, rellenos, pesos), que la web declara en theme.css,
# en su bloque «Tokens de forma». Esa hoja nace en el paquete de configuración del
# frontend, no en branding/: la copia va en ese sentido, y otro test la compara.
cp -f "$root"/apps/frontend/packages/config/src/theme.css "$root/apps/nucleo/tokens/"
# Los radios también salen del canon: el núcleo lee radii de spacing.json.
cp -f "$root"/branding/tokens/spacing.json "$root/apps/nucleo/tokens/"
# El IdP sirve HTML desde Go: incrusta la misma hoja y solo añade su estructura.
# login_colores_test.go comprueba que esta copia no diverge de la fuente canónica.
cp -f "$root"/branding/tokens/colors.css "$root/services/id/internal/http/colores.css"
cp -f "$root"/branding/tokens/appearance.css "$root/services/id/internal/http/appearance.css"
# Las marcas de producto para la interfaz en Fyne (apps/fyne/iconos), que las rasteriza
# con oksvg; sin las `-maskable`, que son para el lanzador del sistema. Copia y no
# enlace por lo mismo que arriba: go:embed no sale de su módulo.
# `iconos.TestLasMarcasSonLasDeBranding` compara las dos carpetas byte a byte y también
# se queja de los huérfanos, así que si esta lista cambia el gate lo dice.
fyne_marcas="$root/apps/fyne/iconos/marcas"
mkdir -p "$fyne_marcas"
rm -f "$fyne_marcas"/*.svg
for m in "$root"/branding/marks/*.svg; do
  case "$m" in *-maskable.svg) continue ;; esac
  cp -f "$m" "$fyne_marcas/"
done
# Y los ICONOS DEL SISTEMA de cada app de Fyne (su `Icon.png`), con el contorno del
# selector: ver el propio guion.
"$root/scripts/gen-iconos-fyne.sh"
# Y las ILUSTRACIONES, por lo mismo. Van enteras —las trece, `hero-suite` incluida—
# porque el conjunto es el de `branding/illustrations` y el guardián compara la carpeta,
# no una lista escrita a mano: una ilustración nueva se copia sola.
#
# Sus `var(--sytue-*)` NO se tocan aquí: `branding/` es la fuente compartida con la web
# y se queda como está. Se resuelven AL LEER, en `iconos.conTokens`.
fyne_ilus="$root/apps/fyne/iconos/ilustraciones"
mkdir -p "$fyne_ilus"
rm -f "$fyne_ilus"/*.svg
cp -f "$root"/branding/illustrations/*.svg "$fyne_ilus/"
# El recuento se hace con globs a un array y no con `ls | wc -l`: contar la salida de
# `ls` se rompe con cualquier nombre raro, y aquí además evita cuatro subshells.
contar() {
  local -a coincidencias=("$@")
  # Un glob sin coincidencias llega literal: eso son cero ficheros, no uno.
  if [ "${#coincidencias[@]}" -eq 1 ] && [ ! -e "${coincidencias[0]}" ]; then
    echo 0
  else
    echo "${#coincidencias[@]}"
  fi
}
echo "sync-branding: $(contar "$ui/icons"/*.svg) iconos, $(contar "$root/branding/marks"/*.svg) marcas, $(contar "$ui/logos"/*.svg) logos, $(contar "$ui/illustrations"/*.svg) ilustraciones, $(contar "$cfg"/*) tokens vendorizados"
