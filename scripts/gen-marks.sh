#!/usr/bin/env bash
# Genera la MARCA DE APLICACIÓN de cada producto: branding/marks/<clave>.svg y su
# variante maskable.
#
# Es el badge de la marca —esquinas y cuña del logotipo de la casa— con el COLOR del
# producto y su GLIFO. Identifica la app en la pestaña del navegador, en el dock y en
# la pantalla de inicio.
#
# POR QUÉ EL GLIFO Y NO EL MONOGRAMA «sy:». Se probaron las dos y una tercera con
# ambos, rasterizadas a 16 px, que es donde vive un favicon: con el glifo se
# distinguen las doce superficies; con el monograma solo cambia el color; con los dos
# juntos no se lee ninguna de las dos cosas. La familia la marcan el badge, la cuña y
# el color, que están en las quince.
#
# POR QUÉ LOS GLIFOS SON SÓLIDOS Y NO DE LÍNEA como los de branding/icons/. El
# rasterizador de la casa (ImageMagick sin delegado; `rsvg-convert` no está instalado
# y el CI tampoco lo trae) NO DIBUJA TRAZOS: los ignora en silencio. Por eso los
# iconos de escritorio y de móvil anteriores eran un cuadrado de color sobre negro,
# sin glifo — se veía en el .app publicado y en ningún gate. Con relleno, los pinta
# cualquier rasterizador. Los huecos van con `fill-rule="evenodd"`, que sí soporta.
#
# NO sustituye a branding/icons/: aquellos son las TESELAS del conmutador, de línea y
# sobre un badge con margen. Son dos trabajos distintos y por eso son dos familias.
#
# POR QUÉ UN GENERADOR. El color se LEE de la paleta, así que no puede divergir — que
# es exactamente lo que pasó con los favicons escritos a mano: siete de los ocho que
# existían usaban colores retirados de branding/tokens/colors.json.
#
# Uso:  bash scripts/gen-marks.sh              genera las marcas
#       bash scripts/gen-marks.sh --comprobar   mide lo generado y falla si se ha ido
set -euo pipefail

RAIZ="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PALETA="$RAIZ/branding/tokens/colors.json"
DESTINO="$RAIZ/branding/marks"

command -v jq >/dev/null || { echo "hace falta jq" >&2; exit 1; }
mkdir -p "$DESTINO"

# --- Los glifos, sobre el grid 40 y centrados en (20,20) ----------------------
# Misma figura que branding/icons/<clave>.svg, redibujada como silueta.
glifo() {
  case "$1" in
    one) cat <<'G'
<rect x="9.6" y="9.6" width="9" height="9" rx="2.4" fill="#fff"/><rect x="21.4" y="9.6" width="9" height="9" rx="2.4" fill="#fff"/><rect x="9.6" y="21.4" width="9" height="9" rx="2.4" fill="#fff"/><rect x="21.4" y="21.4" width="9" height="9" rx="2.4" fill="#fff"/>
G
;;
    id) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M15.2 18.6a6.4 6.4 0 1 1 0 12.8 6.4 6.4 0 0 1 0-12.8Zm0 3.8a2.6 2.6 0 1 0 0 5.2 2.6 2.6 0 0 0 0-5.2Z"/><path fill="#fff" d="m19 21.2 9.9-9.9 2.4 2.4-2 2 1.9 1.9-2.4 2.4-1.9-1.9-1.6 1.6 1.9 1.9-2.4 2.4-1.9-1.9-1.5 1.5Z"/>
G
;;
    drive) cat <<'G'
<path fill="#fff" d="M10.4 12.4h5.9a2 2 0 0 1 1.56.75l1.42 1.75h10.32a2 2 0 0 1 2 2v10.7a2 2 0 0 1-2 2h-19.2a2 2 0 0 1-2-2V14.4a2 2 0 0 1 2-2Z"/>
G
;;
    docs) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M13 9h8.6v5.4a2 2 0 0 0 2 2H29V29a2 2 0 0 1-2 2H13a2 2 0 0 1-2-2V11a2 2 0 0 1 2-2Zm2 11h10v2H15Zm0 4.6h7v2h-7Z"/><path fill="#fff" d="M22.6 9.4 28.6 15.4h-4.4a1.6 1.6 0 0 1-1.6-1.6Z"/>
G
;;
    sheets) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M13 9h8.6v5.4a2 2 0 0 0 2 2H29V29a2 2 0 0 1-2 2H13a2 2 0 0 1-2-2V11a2 2 0 0 1 2-2Zm2 17.4h2.4v-4.6H15Zm4 0h2.4v-7.4H19Zm4 0h2.4v-5.4H23Z"/><path fill="#fff" d="M22.6 9.4 28.6 15.4h-4.4a1.6 1.6 0 0 1-1.6-1.6Z"/>
G
;;
    slides) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M13 9h8.6v5.4a2 2 0 0 0 2 2H29V29a2 2 0 0 1-2 2H13a2 2 0 0 1-2-2V11a2 2 0 0 1 2-2Zm4.4 10.8v7.2l6-3.6Z"/><path fill="#fff" d="M22.6 9.4 28.6 15.4h-4.4a1.6 1.6 0 0 1-1.6-1.6Z"/>
G
;;
    pdf) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M13 9h8.6v5.4a2 2 0 0 0 2 2H29V29a2 2 0 0 1-2 2H13a2 2 0 0 1-2-2V11a2 2 0 0 1 2-2Zm2 10.6h9.4v2H15Zm5 8.4a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z"/><path fill="#fff" d="M22.6 9.4 28.6 15.4h-4.4a1.6 1.6 0 0 1-1.6-1.6Z"/>
G
;;
    mail) cat <<'G'
<path fill="#fff" d="M11.4 11.8h17.2a2.4 2.4 0 0 1 2.4 2.4v.6L20 22.2 9 14.8v-.6a2.4 2.4 0 0 1 2.4-2.4Z"/><path fill="#fff" d="M9 17.4 20 24.8 31 17.4v8.4a2.4 2.4 0 0 1-2.4 2.4H11.4A2.4 2.4 0 0 1 9 25.8Z"/>
G
;;
    calendar) cat <<'G'
<rect x="12.6" y="8.6" width="3.2" height="5.4" rx="1.6" fill="#fff"/><rect x="24.2" y="8.6" width="3.2" height="5.4" rx="1.6" fill="#fff"/><path fill="#fff" d="M10 10.8h20a2 2 0 0 1 2 2v3.4H8v-3.4a2 2 0 0 1 2-2Z"/><path fill-rule="evenodd" fill="#fff" d="M8 18.6h24v10.4a2 2 0 0 1-2 2H10a2 2 0 0 1-2-2Zm4 3v2.6h2.6v-2.6Zm6.7 0v2.6h2.6v-2.6Zm6.7 0v2.6h2.6v-2.6Z"/>
G
;;
    contacts) cat <<'G'
<path fill="#fff" d="M20 9.2a5.4 5.4 0 1 1 0 10.8 5.4 5.4 0 0 1 0-10.8Z"/><path fill="#fff" d="M20 21.6c5.7 0 10.4 3.6 10.4 8a1.8 1.8 0 0 1-1.8 1.8H11.4a1.8 1.8 0 0 1-1.8-1.8c0-4.4 4.7-8 10.4-8Z"/>
G
;;
    # CÁMARA, no una montaña con sol: el paisaje dentro de un marco es el icono
    # de «una imagen», y aquí lo que se nombra es la APLICACIÓN que las guarda y
    # las edita. La cámara lo dice sin depender de que se distinga un sol de una
    # luna a 24 px.
    #
    # El objetivo es un HUECO (fill-rule evenodd) y no un disco de otro color:
    # así deja ver el color del producto y el glifo sigue siendo una sola
    # silueta, que es la regla de esta familia.
    photos) cat <<'G'
<path fill="#fff" fill-rule="evenodd" d="M16.4 9.4h7.2c.7 0 1.4.4 1.8 1l1.3 2.2H29a3.2 3.2 0 0 1 3.2 3.2v11.6A3.2 3.2 0 0 1 29 30.6H11a3.2 3.2 0 0 1-3.2-3.2V15.8A3.2 3.2 0 0 1 11 12.6h2.3l1.3-2.2c.4-.6 1.1-1 1.8-1Zm3.6 6.9a5.9 5.9 0 1 0 0 11.8 5.9 5.9 0 0 0 0-11.8Zm0 2.6a3.3 3.3 0 1 1 0 6.6 3.3 3.3 0 0 1 0-6.6Z"/>
G
;;
    notes) cat <<'G'
<path fill="#fff" d="M12 8.6h14a2 2 0 0 1 2 2v10.4h-4.6a2.4 2.4 0 0 0-2.4 2.4V28H12a2 2 0 0 1-2-2V10.6a2 2 0 0 1 2-2Z"/><path fill="#fff" d="m27.1 22.6-4.5 4.5v-3.1a1.4 1.4 0 0 1 1.4-1.4Z"/>
G
;;
    # Bocadillo de chat con el OBJETIVO dentro: mensajeria y videollamada son un
    # solo producto y el glifo lo dice de una vez. El aro es un hueco del cuerpo y
    # la lente un disco encima; con el aro grueso y la lente pequena parecia un
    # boton de grabar, no un objetivo.
    rooms) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M11.6 9.2h16.8a3.2 3.2 0 0 1 3.2 3.2v10a3.2 3.2 0 0 1-3.2 3.2h-9.9l-6.5 4.9v-4.9h-.4a3.2 3.2 0 0 1-3.2-3.2v-10a3.2 3.2 0 0 1 3.2-3.2ZM20 11.6a5.8 5.8 0 1 0 0 11.6 5.8 5.8 0 0 0 0-11.6Z"/><circle cx="20" cy="17.4" r="3.6" fill="#fff"/>
G
;;
    # SyERP: un edificio. Es lo único de la familia que dice «la empresa» sin
    # chocar con otro producto —la carpeta es Drive, el documento es Docus, la
    # persona es Contacts—. Un gráfico de barras, el primer intento, se lee
    # «analítica».
    #
    # LAS VENTANAS SON GRANDES PORQUE UN FAVICON MIDE 16 PX. La primera versión
    # llevaba seis de 3,2 y una puerta, y el comentario de aquí decía que «a 16 px
    # las ventanas siguen leyéndose como ventanas». Medido, no: a ese tamaño cada
    # hueco no llega al píxel y medio, se emborronan en un moteado y lo que queda
    # es una textura gris. Era el único glifo de la familia con ese problema —los
    # demás cargan el significado en una o dos formas grandes: el sobre, la
    # carpeta, la esquina doblada del notepad—.
    #
    # Ahora son cuatro de 5x4 y una puerta que llega al suelo. La puerta no es
    # decoración: le abre a la silueta una muesca en la base, y por esa muesca se
    # reconoce el edificio cuando los huecos ya no se ven.
    erp) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M11 11h18a2 2 0 0 1 2 2v18H9V13a2 2 0 0 1 2-2Zm1 4h5v4h-5Zm11 0h5v4h-5Zm-11 6h5v4h-5Zm11 0h5v4h-5Zm-5.5 6h5v4h-5Z"/>
G
;;
    # Sytue Cast: el triángulo de reproducción SÓLIDO —lo que se ve— con dos ondas de
    # emisión saliendo de su vértice superior izquierdo —lo que se emite—. Vídeo y
    # directo en una sola figura, y ninguna choca con otra de la familia: Diapos lleva
    # su triángulo DENTRO de un documento, y aquí va suelto y grande.
    #
    # Las ondas son sectores de corona con las puntas redondas, dibujados como
    # RELLENO (arco exterior, tapa, arco interior, tapa): un `stroke` con
    # `stroke-linecap` sería más corto, pero el rasterizador de la casa lo ignora
    # (ver arriba). Anchas (2,6) y con 2,4 de aire entre las dos para que a 16 px
    # sigan siendo dos trazos y no una mancha.
    cast) cat <<'G'
<path fill="#fff" d="M17 16.5V35.5L34.5 26Z"/><path fill="#fff" d="M11.2 14.95A6 6 0 0 1 15.45 10.7A1.3 1.3 0 0 1 16.12 13.22A3.4 3.4 0 0 0 13.72 15.62A1.3 1.3 0 0 1 11.2 14.95Z"/><path fill="#fff" d="M6.37 13.65A11 11 0 0 1 14.15 5.87A1.3 1.3 0 0 1 14.83 8.39A8.4 8.4 0 0 0 8.89 14.33A1.3 1.3 0 0 1 6.37 13.65Z"/>
G
;;
    # Sytue Web: una VENTANA DE NAVEGADOR —marco redondeado con la barra de arriba y
    # sus tres puntos— con una página que se maqueta dentro: una cabecera y dos bloques.
    # Es la web que se diseña, y no choca con nadie: Docus y compañía son una hoja con
    # la esquina doblada, SyERP es un edificio macizo con ventanas y Calendar lleva la
    # barra SEPARADA del cuerpo; aquí la barra y el marco son una sola pieza y el
    # interior es color, con los bloques blancos dentro.
    #
    # Barra, marco y puntos van en un solo `path` con `evenodd` (el hueco del interior
    # y los tres puntos son agujeros); la cabecera y los bloques, en otro, macizos. Los
    # huecos miden 2,2 y los bloques 6,7 × 4,2: a 16 px lo que se lee es la barra ancha
    # y el marco, que es lo que dice «navegador».
    pages) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M11 9.7h18a3 3 0 0 1 3 3v14.6a3 3 0 0 1-3 3H11a3 3 0 0 1-3-3V12.7a3 3 0 0 1 3-3Zm-.8 5.2v13.2h19.6V14.9ZM10.75 12.3a1.15 1.15 0 1 0 2.3 0a1.15 1.15 0 1 0-2.3 0Zm3.1 0a1.15 1.15 0 1 0 2.3 0a1.15 1.15 0 1 0-2.3 0Zm3.1 0a1.15 1.15 0 1 0 2.3 0a1.15 1.15 0 1 0-2.3 0Z"/><path fill="#fff" d="M12.4 17.1h15.2v2.4H12.4Zm0 4.6h6.7v4.2h-6.7Zm8.5 0h6.7v4.2h-6.7Z"/>
G
;;
    # Sytue Bases: una TABLA —marco redondeado con la banda de cabecera maciza y tres
    # filas de dos celdas— es lo que se rellena y se relaciona. Marco y celdas van en un
    # solo `path` con `evenodd` (cada celda es un agujero): a 16 px se leen la banda y
    # las filas, que es lo que dice «tabla» y no choca con la ventana de Pages ni con la
    # hoja con esquina doblada de Docus.
    bases) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M11 9h18a3 3 0 0 1 3 3v16a3 3 0 0 1-3 3H11a3 3 0 0 1-3-3V12a3 3 0 0 1 3-3Zm-1.6 6.8v3.6h6.2v-3.6Zm7.6 0v3.6h13.6v-3.6ZM9.4 20.8v3.6h6.2v-3.6Zm7.6 0v3.6h13.6v-3.6ZM9.4 25.8v2.4a1.6 1.6 0 0 0 1.6 1.6h4.6v-4Zm7.6 0v4h12a1.6 1.6 0 0 0 1.6-1.6v-2.4Z"/>
G
;;
    cloud) cat <<'G'
<path fill="#fff" d="M14.4 29.6a5.2 5.2 0 0 1-2.2-9.9 7.1 7.1 0 0 1 13.95-1 4.55 4.55 0 0 1-1.15 10.9Z"/>
G
;;
    console) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M11.9 11.4h16.2a2.9 2.9 0 0 1 0 5.8H11.9a2.9 2.9 0 0 1 0-5.8Zm3.9 4.8a1.9 1.9 0 1 0 0-3.8 1.9 1.9 0 0 0 0 3.8Z"/><path fill-rule="evenodd" fill="#fff" d="M11.9 22.8h16.2a2.9 2.9 0 0 1 0 5.8H11.9a2.9 2.9 0 0 1 0-5.8Zm12.3 4.8a1.9 1.9 0 1 0 0-3.8 1.9 1.9 0 0 0 0 3.8Z"/>
G
;;
    hub) cat <<'G'
<path fill-rule="evenodd" fill="#fff" d="M18.4 8.6a8.6 8.6 0 1 1 0 17.2 8.6 8.6 0 0 1 0-17.2Zm0 3.7a4.9 4.9 0 1 0 0 9.8 4.9 4.9 0 0 0 0-9.8Z"/><path fill="#fff" d="m24.6 24.9 2.3-2.3 4.8 4.8a1.63 1.63 0 0 1-2.3 2.3Z"/>
G
;;
    *) return 1 ;;
  esac
}

# Caja MEDIDA de cada glifo: centro real (cx cy) y escala para que su lado largo
# mida 22 unidades. Se rasteriza el glifo solo y se recorta —los numeros salen de
# ahi, no de la vista—. `--medir` los vuelve a calcular y avisa si han cambiado.
encuadre() {
  case "$1" in
    one) echo "20.000 20.000 1.0526" ;;
    id) echo "20.050 21.350 0.9692" ;;
    drive) echo "20.025 21.000 0.9422" ;;
    docs) echo "20.025 20.000 0.9910" ;;
    sheets) echo "20.025 20.000 0.9910" ;;
    slides) echo "20.025 20.000 0.9910" ;;
    pdf) echo "20.025 20.000 0.9910" ;;
    mail) echo "20.000 20.000 0.9910" ;;
    calendar) echo "20.000 19.825 0.9091" ;;
    contacts) echo "20.025 20.325 0.9843" ;;
    photos) echo "20.020 19.935 0.8961" ;;
    notes) echo "19.000 18.325 1.1253" ;;
    rooms) echo "20.025 19.875 0.9422" ;;
    cloud) echo "20.150 21.450 1.0046" ;;
    console) echo "20.025 20.025 0.9932" ;;
    hub) echo "20.950 19.350 0.9821" ;;
    erp) echo "20.000 21.000 0.9955" ;;
    # Caja calculada sobre la geometría (triángulo, arcos y puntas): 6,33–34,50 × 5,83–35,50.
    cast) echo "20.415 20.665 0.7415" ;;
    # Caja de la geometría (rectángulos y arcos, sin trazos): 8–32 × 9,7–30,3.
    pages) echo "20.000 20.000 0.9167" ;;
    # Caja de la geometría (marco y celdas, sin trazos): 8–32 × 9–31.
    bases) echo "20.000 20.000 0.9167" ;;
    *) return 1 ;;
  esac
}

# Nombre visible por clave: el `name` de CATALOGO_ORDENADO en @sytue/ui (lib/apps.ts),
# que es de donde lo toman el conmutador, el Hub y el sitio público. Aquí había una
# lista propia y se quedó atrás en el renombrado v4: al regenerar, «Sytue Docus» volvía
# a «Sytue Docs» y «Sytue DevX» a «Sytue Cloud». Una clave que no está en el catálogo
# (`hub`) se resuelve por su SOURCE_ALIAS, como `appForKey`; sin nombre, se falla en vez
# de inventarlo.
CATALOGO="$RAIZ/apps/frontend/packages/ui/src/lib/apps.ts"
# Se parte cada línea por las comillas simples: «key: 'x'», «name: 'y'» y «hub: 'one'»
# dejan la clave en el trozo anterior y el valor en el siguiente, en una línea o en varias.
NOMBRES="$(awk -F"'" '
  /^const CATALOGO_ORDENADO/ { cat = 1; next }
  cat && /^\];/ { cat = 0 }
  cat { for (i = 1; i < NF; i += 2) {
          if ($i ~ /key: $/) k = $(i + 1)
          else if ($i ~ /name: $/) print "n", k, $(i + 1)
        } }
  /^const SOURCE_ALIAS/ { for (i = 1; i < NF; i += 2) if (match($i, /[a-z]+: $/))
          print "a", substr($i, RSTART, RLENGTH - 2), $(i + 1) }
' OFS='\t' "$CATALOGO")"
nombre() {
  local k="$1" a
  a="$(awk -F'\t' -v k="$k" '$1 == "a" && $2 == k { print $3 }' <<<"$NOMBRES")"
  awk -F'\t' -v k="${a:-$k}" '$1 == "n" && $2 == k { print $3; f = 1 } END { exit !f }' <<<"$NOMBRES" ||
    { echo "«$k» no tiene nombre en $CATALOGO" >&2; return 1; }
}

# --- Comprobacion del encuadre -------------------------------------------------
#
# Existe porque esto se fue de sitio sin que nadie lo viera: el sobre de Mail medía
# 23,4 x 13,4 (una banda, no un icono), Notes estaba 1,7 unidades arriba y los anchos
# de la familia iban de 18,2 a 26,6. A ojo se nota tarde; medido, no se cuela.
#
# Se rasteriza el glifo SOLO —sin badge ni cuña— y se recorta: la caja que sale es la
# que se ve, no la que dice el `viewBox`.
comprobar() {
  command -v magick >/dev/null || { echo "hace falta ImageMagick para comprobar" >&2; exit 1; }
  local tmp fallos=0
  tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' RETURN
  printf '%-10s %6s %6s %7s %7s\n' clave ancho alto dx dy
  for f in "$DESTINO"/*.svg; do
    case "$f" in *-maskable.svg) continue ;; esac
    local k; k="$(basename "$f" .svg)"
    # El glifo es lo que va detras de la cuña; el badge y la cuña se descartan.
    # Se aplana a UNA linea antes de recortar: el fichero tiene saltos y un `sed`
    # por lineas dejaba el documento a medias.
    {
      printf '%s' '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40" width="800" height="800">'
      tr -d '\n' <"$f" | sed -e 's|.*fill-opacity="0.22"/>||' -e 's|</svg>.*||'
      printf '%s' '</svg>'
    } >"$tmp/$k.svg"
    magick -background none -density 1024 "$tmp/$k.svg" -resize 800x800 "$tmp/$k.png"
    local geo; geo="$(magick identify -format '%@' "$tmp/$k.png")"
    awk -v k="$k" -v geo="$geo" 'BEGIN {
      split(geo, a, /[x+]/); w=a[1]/20; h=a[2]/20; cx=a[3]/20+w/2; cy=a[4]/20+h/2
      dx=cx-20; dy=cy-20; larg=(w>h?w:h); cort=(w<h?w:h)
      printf "%-10s %6.2f %6.2f %+7.2f %+7.2f\n", k, w, h, dx, dy
      mal=0
      if (dx<-0.3 || dx>0.3 || dy<-0.3 || dy>0.3) { printf "  ✗ %s descentrado\n", k > "/dev/stderr"; mal=1 }
      if (larg<21.4 || larg>22.6) { printf "  ✗ %s: lado largo %.2f, fuera de 22±0,6\n", k, larg > "/dev/stderr"; mal=1 }
      if (larg/cort>1.42) { printf "  ✗ %s: proporción %.2f > 1,42 (se lee como banda)\n", k, larg/cort > "/dev/stderr"; mal=1 }
      exit mal
    }' || fallos=$((fallos + 1))
  done
  [ "$fallos" -eq 0 ] || { echo; echo "$fallos glifo(s) fuera de encuadre. Remide con la tabla de encuadre()." >&2; exit 1; }
  echo
  echo "encuadre: los quince glifos centrados y con el mismo tamaño óptico"
}

if [ "${1:-}" = "--comprobar" ]; then comprobar; exit 0; fi

n=0
while IFS=$'\t' read -r clave color; do
  [ -n "$clave" ] || continue
  titulo="$(nombre "$clave")" || exit 1
  g="$(glifo "$clave")" || { echo "sin glifo para «$clave»: añádelo a glifo()" >&2; exit 1; }
  read -r gcx gcy gs <<<"$(encuadre "$clave")" || { echo "sin encuadre para «$clave»" >&2; exit 1; }
  # Encuadre comun: centrado en (20,20) y con el lado largo a 22 unidades.
  g="<g transform=\"translate(20 20) scale($gs) translate(-$gcx -$gcy)\">$g</g>"

  # --- Marca ------------------------------------------------------------------
  # SIN `clipPath`, a propósito, aunque el logotipo de la casa lo use: el rasterizador
  # lo ignora en silencio y las esquinas salían cuadradas sin que nada avisara. La cuña
  # sigue el radio por GEOMETRÍA —baja recta hasta y=31 y cierra con el arco r=9 del
  # badge—, y eso lo dibuja igual cualquier rasterizador.
  cat >"$DESTINO/$clave.svg" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 40" width="40" height="40" role="img" aria-label="$titulo">
  <!-- GENERADO por scripts/gen-marks.sh: cambia la paleta o el script, no este fichero. -->
  <rect width="40" height="40" rx="9" fill="$color"/>
  <path d="M40 14V31a9 9 0 0 1-9 9H14Z" fill="#000000" fill-opacity="0.22"/>
  $g
</svg>
SVG

  # --- Variante maskable ------------------------------------------------------
  # La que piden los manifiestos PWA y el icono de móvil: color a SANGRE (sin esquinas
  # redondeadas, que las pone la máscara del sistema) y el glifo dentro de la zona
  # segura. Android recorta al ~66 % central y iOS aplica su escudo.
  #
  # scale 10.9: el glifo mide 23 unidades de lado, así que llega a 251 px sobre 512 y
  # su media diagonal (177) cabe en el círculo seguro (r=205). Con la escala natural
  # (12.8) se saldría por las esquinas.
  cat >"$DESTINO/$clave-maskable.svg" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512" role="img" aria-label="$titulo">
  <!-- GENERADO por scripts/gen-marks.sh: cambia la paleta o el script, no este fichero. -->
  <rect width="512" height="512" fill="$color"/>
  <path d="M512 179V512H179Z" fill="#000000" fill-opacity="0.22"/>
  <g transform="translate(256 256) scale(10.9) translate(-20 -20)">$g</g>
</svg>
SVG

  n=$((n + 1))
  echo "  $clave  $color"
# chat y meet son claves LEGADAS de la paleta (resuelven el color del `source` del Hub) y
# no tienen marca: su pestaña es la de Rooms. Sin esta exclusión el script se paraba en «chat».
done < <(jq -r '.product | to_entries[] | select(.key | startswith("$") | not) | select(.key | IN("chat", "meet") | not) | "\(.key)\t\(.value)"' "$PALETA")

# --- Los PNG de la PWA de Sytue One -------------------------------------------
#
# El manifiesto del portal declara PNG y no SVG (es el único que lo hace), así que hay
# que rasterizarlos. Se hacen aquí y no en sync-branding porque sync-branding lo
# ejecuta `make fe-install` en cualquier máquina y no puede exigir ImageMagick; esto se
# ejecuta a mano al tocar la marca. Sin ImageMagick, se dice y no se rompe nada.
#
# `-strip` y los `png:exclude-chunk`: ImageMagick escribe la FECHA dentro del PNG, así
# que dos ejecuciones seguidas daban bytes distintos y ensuciaban el árbol sin que
# hubiera cambiado nada.
PWA="$RAIZ/apps/frontend/apps/portal/public/icons"
# shellcheck disable=SC2054  # la coma es de ImageMagick («date,time»), no separa elementos
SIN_FECHA=(-strip -define png:exclude-chunk=date,time)
if [ -d "$PWA" ]; then
  if command -v magick >/dev/null; then
    magick -background none -density 2048 "$DESTINO/one.svg" -resize 192x192 "${SIN_FECHA[@]}" "$PWA/icon-192.png"
    magick -background none -density 2048 "$DESTINO/one.svg" -resize 512x512 "${SIN_FECHA[@]}" "$PWA/icon-512.png"
    # apple-touch-icon: iOS NO admite transparencia y no redondea el que le des, así
    # que va la maskable (color a sangre) y no la marca con sus esquinas.
    magick -background none -density 2048 "$DESTINO/one-maskable.svg" -resize 180x180 -alpha off \
      "${SIN_FECHA[@]}" "$PWA/apple-touch-icon-180.png"
    magick -background none -density 2048 "$DESTINO/one-maskable.svg" -resize 512x512 -alpha off \
      "${SIN_FECHA[@]}" "$PWA/icon-maskable-512.png"
    echo "  PWA de Sytue One: 4 PNG regenerados"
  else
    echo "  (sin ImageMagick: los PNG de la PWA del portal NO se han regenerado)"
  fi
fi

echo "gen-marks: $n marcas (+ maskable) en branding/marks/"
