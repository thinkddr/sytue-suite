#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Daniel Delgado Ramos
#
# Dos reglas de diseño que ESLint no puede ver, porque viven dentro de cadenas
# de clases. Las dos cazan el mismo tipo de fallo: alguien reescribe a mano lo
# que el design system ya tiene canonizado, y nadie se entera porque el defecto
# solo se nota en un tema o con el teclado.
#
# No es una regla de estilo: son los dos defectos MÁS EXTENDIDOS que encontró el
# comité de diseño, y el propio `packages/ui/src/styles.css` dice que llevaban
# «rondas» perdiendo el segundo anillo de foco. Arreglarlos sin esta puerta es
# trabajo que se vuelve a perder en la siguiente tanda.
set -uo pipefail

AQUI="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$AQUI" || exit 1
fallos=0

# Solo código de interfaz: la documentación puede citar el patrón malo para
# explicarlo, y el CSS de tokens es justo donde los escalones SÍ se declaran.
BUSCAR=(apps packages --include=*.tsx --include=*.ts --include=*.astro
        --exclude-dir=dist --exclude-dir=node_modules --exclude-dir=.astro)

# --- 1) Anillo de foco con el contorno neutro que ya no existe -------------
# Este gate exigía lo contrario: que todo anillo llevara `--sytue-focus-contrast`
# junto al latón, porque el latón viejo (#b98f4f) se quedaba en 2,81:1 sobre
# marfil y no llegaba al 3:1 de WCAG 1.4.11. El token nuevo llega solo —≥3,17:1
# sobre las ocho superficies de la suite, claras y grafito—, así que el contorno
# se retiró: era la línea negra que se veía alrededor de cada campo.
#
# Ahora el defecto es el inverso, y es SILENCIOSO: una variable CSS que no existe
# no da error, se resuelve a nada y deja un anillo transparente. Quien copie un
# anillo viejo de otro fichero se lleva eso sin enterarse.
# `-e` y no `--` para proteger un patrón que empieza por guion: `--` termina el
# procesamiento de opciones, así que los `--include`/`--exclude-dir` de BUSCAR
# pasarían a leerse como nombres de fichero y el gate se pondría a escanear
# `dist/`, donde vive el build viejo de todo lo que acabas de arreglar.
# Los tests se excluyen porque los que nombran el token son justo los que
# comprueban que YA NO está: sin esto, la red de seguridad se delata a sí misma.
MAL_FOCO="$(grep -rn -e "--sytue-focus-contrast" "${BUSCAR[@]}" 2>/dev/null |
  grep -v -e "\.test\." || true)"
if [ -n "$MAL_FOCO" ]; then
  echo "✗ '--sytue-focus-contrast' ya no existe: el anillo saldría transparente, sin avisar."
  echo "  El foco es UN anillo de latón. Usa 'sytue-focus-ring' o 'sytue-focus-ring-inset':"
  printf '%s\n' "$MAL_FOCO" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

# --- 2) Escalones crudos de primary/accent --------------------------------
# El bloque oscuro NO reasigna estas familias: lo que se pinte con ellas sale
# como una mancha clara sobre grafito. Los tokens semánticos sí conmutan.
MAL_TOKEN="$(grep -rnE "(bg|text|border)-(primary|accent)-(50|100|700|800|900)\b" "${BUSCAR[@]}" 2>/dev/null || true)"
if [ -n "$MAL_TOKEN" ]; then
  echo "✗ Escalón crudo de primary/accent: esa familia no se reasigna en tema oscuro."
  echo "  Usa los tokens semánticos (-bg / -text / bg-selected), que sí conmutan:"
  printf '%s\n' "$MAL_TOKEN" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

# --- 3) Token de TINTA usado como RELLENO ---------------------------------
# `*-text` es el color con el que se ESCRIBE sobre una superficie, y el bloque
# oscuro lo reasigna a un escalón CLARO para que se lea sobre grafito. Usado como
# fondo, ese mismo token da una mancha clara con el texto blanco encima: el
# contador del Badge llevaba `bg-primary-text` y en tema oscuro medía 2,11:1,
# menos de la mitad del 4,5:1 que pide WCAG 1.4.3 (2026-09-21). Para rellenar con
# texto blanco está el escalón 600, que vale lo mismo en los dos temas.
MAL_RELLENO="$(grep -rnE "\bbg-(primary|accent|danger|success|warning|info|violet)-text\b" "${BUSCAR[@]}" 2>/dev/null || true)"
if [ -n "$MAL_RELLENO" ]; then
  echo "✗ Token de tinta como relleno: '*-text' es para ESCRIBIR, no para el fondo."
  echo "  En tema oscuro conmuta a un escalón claro y el texto de encima se pierde."
  echo "  Usa 'bg-primary-600' (o el '-bg' semántico del tono):"
  printf '%s\n' "$MAL_RELLENO" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

# --- 4) Clase concatenada sin separador -----------------------------------
# `class={`base${cond ? 'modificador' : ''}`}` produce «basemodificador»: una
# clase que no existe, y el elemento pierde TODOS los estilos de la base. No lo
# ve TypeScript, ni ESLint, ni el navegador —que se limita a no aplicar nada—, y
# solo se nota en la rama condicional, que suele ser la excepcional. Pasó en la
# barra de aforo de Sytue Admin: desaparecía justo al superarse el aforo, que es
# cuando hay que verla.
# Con grep no basta: los dos casos de la hoja de cálculo tenían el `${` y el
# ternario en LÍNEAS distintas —justo por ser los templates más largos, que es
# donde el fallo es más fácil de colar—. Perl con -0777 lee el fichero entero.
#
# ⚠️ El separador va SIEMPRE fuera de la interpolación —`base ${c ? 'x' : ''}`—,
# nunca dentro del literal (`base${c ? ' x' : ''}`). Las dos formas producen el
# mismo HTML, pero `prettier-plugin-tailwindcss` normaliza las cadenas de clases
# y BORRA el espacio de la segunda, así que el arreglo dura hasta el siguiente
# `bun run format`. Es lo que había pasado aquí: el defecto no lo escribió nadie,
# lo introducía el formateador, y por eso reaparecía después de arreglarlo.
# shellcheck disable=SC2016  # es un programa perl: el `$` es suyo, no del shell.
PEGADAS="$(find apps packages \( -name '*.tsx' -o -name '*.astro' \) \
  -not -path '*/node_modules/*' -not -path '*/dist/*' -not -path '*/.astro/*' -print0 2>/dev/null |
  xargs -0 perl -0777 -ne 'while (m{class=\{`((?:[^`\\]|\\.)*)`}gs) {
    $t = $1; $p = $-[0];
    while ($t =~ m{[a-zA-Z0-9_\-\)\]\}]\$\{\s*[^{}]*?\?}gs) {
      $ln = (substr($_, 0, $p) =~ tr/\n//) + 1;
      print "$ARGV:$ln\n";
    }
  }' 2>/dev/null | sort -u || true)"
if [ -n "$PEGADAS" ]; then
  echo "✗ Clase pegada a \${...}: sale una clase que no existe."
  echo "  El espacio va ANTES del \${, no dentro del literal (Prettier lo borra ahí):"
  printf '%s\n' "$PEGADAS" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

# --- 3) Encabezado sin jerarquía explícita ---------------------------------
# Solo AVISA, no rompe. Desde que `@layer base` da a h1..h6 la escala del
# sistema, un encabezado desnudo ya no cae a los 16px del navegador; pero
# conviene mirarlo al escribirlo, porque el nivel semántico correcto y el tamaño
# que se quiere no siempre coinciden.
DESNUDOS="$(grep -rnE "<h[1-6][^>]*>" "${BUSCAR[@]}" 2>/dev/null |
  grep -v "sytue-h" | grep -v "sytue-display" |
  grep -vE "text-(xs|sm|base|lg|xl|[0-9]xl)" || true)"
if [ -n "$DESNUDOS" ]; then
  echo "! $(printf '%s\n' "$DESNUDOS" | wc -l | tr -d ' ') encabezado(s) sin clase de tamaño:"
  echo "  heredan la escala de @layer base; comprueba que el nivel es el que quieres."
  echo
fi

# --- 4) Altura medida en vh -------------------------------------------------
# En Chrome Android `100vh` es la altura CON la barra de direcciones retraída, o
# sea MÁS que la pantalla visible al cargar. Un marco que no desplaza —los shells
# de la suite no lo hacen— deja fuera lo que sobra y no hay forma de alcanzarlo.
# `dvh` es la altura real de ese momento. Se coló en cuatro sitios (los dos
# shells de Docs, el aterrizaje de Rooms y el AppShell) hasta que la auditoría de
# móvil de agosto de 2026 los midió en un Moto G4.
#
# `[0-9]vh]` además de `100vh`: el mismo razonamiento vale para CUALQUIER
# fracción, y ahí es donde seguía vivo. El Modal canónico de @sytue/ui llevaba
# `max-h-[90vh]` —o sea el 90% de una altura MAYOR que la pantalla— y con él
# todos los diálogos de la suite: la fila de botones podía quedar por debajo del
# borde visible nada más abrirlo. La regla solo miraba `100vh` y no lo vio, ni a
# los cuatro `max-h-[Nvh]` de Rooms, Cloud y Drive.
# No caza `dvh`/`svh`/`lvh`: el carácter anterior a `vh]` tiene que ser un DÍGITO.
#
# El grep -v deja fuera las líneas de comentario: este mismo fichero y el propio
# AppShell explican el porqué citando el patrón malo, y el gate no puede
# saltar por la explicación de por qué existe.
#
# Y TAMBIÉN el HTML que sirve Go. El frontend no es solo Astro: Sytue ID se pinta
# con su propio CSS embebido (services/id/internal/http/login.go) y es la única
# pantalla que ve el 100% de los usuarios. Ahí seguía vivo `min-height:100vh` en
# el <body> del login mientras este gate salía en verde, porque su BUSCAR es
# `apps packages` relativo a apps/frontend y services/ no existe para él.
# El patrón del segundo grep es `[0-9]vh` seguido de algo que no sea letra: así
# caza `100vh;` dentro de la cadena CSS y no `100dvh`, que es el arreglo.
VH="$( { grep -rnE "h-screen|min-h-screen|100vh|[0-9]vh\]" "${BUSCAR[@]}" 2>/dev/null
         grep -rnE "[0-9]vh([^a-z]|$)" ../../services --include='*.go' 2>/dev/null; } |
  grep -vE ":[[:space:]]*(//|\*|/\*)" || true)"
if [ -n "$VH" ]; then
  echo "✗ Altura en vh: en Chrome Android mide más que la pantalla visible y corta el contenido."
  echo "  Usa dvh —h-[100dvh], max-h-[90dvh], h-[calc(100dvh-var(--spacing-topbar))]— como AdminShell:"
  printf '%s\n' "$VH" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

# --- 5) Panel anclado al borde derecho con ancho fijo grande ---------------
# Un panel con `absolute right-0` se ancla al borde derecho de su DISPARADOR, no
# al de la pantalla. Si el disparador no está pegado al borde —la campana de la
# barra superior no lo está— el panel arranca en `disparador.right − ancho`, que
# en un teléfono estrecho es NEGATIVO. Y un desbordamiento a la izquierda no
# genera scroll: se RECORTA, así que a lo que queda fuera no se llega ni
# desplazando. Es peor que el desbordamiento clásico y encima `scrollWidth` no
# lo ve, que es por lo que la auditoría de agosto de 2026 lo declaró «ok» 18
# veces mientras el centro de notificaciones se salía 86 px en las diez apps.
#
# Dos salidas legítimas, las dos excluidas aquí:
#   – El CAJÓN pegado a un lado (`inset-y-0` o `top-0`+`bottom-0`): ese sí se
#     ancla al contenedor completo, no a un disparador pequeño.
#   – El fichero que llama a `useFitInViewport`, que es el acotado canónico.
CANDIDATOS="$(grep -rn "absolute" "${BUSCAR[@]}" 2>/dev/null |
  grep -E "(^|[^-])right-0([^0-9]|$)" |
  grep -E "(^|[^-])w-(6[4-9]|7[0-9]|8[0-9]|9[0-6])([^0-9]|$)" |
  grep -vE "inset-y-0|top-0.*bottom-0|bottom-0.*top-0" || true)"
DERECHA=""
while IFS= read -r linea; do
  [ -z "$linea" ] && continue
  grep -q "useFitInViewport" "${linea%%:*}" 2>/dev/null && continue
  DERECHA="$DERECHA$linea"$'\n'
done <<<"$CANDIDATOS"
if [ -n "${DERECHA%$'\n'}" ]; then
  echo "✗ Panel anclado a la derecha con ancho fijo: se sale por la IZQUIERDA en un teléfono,"
  echo "  y a eso NO se llega desplazando (el recorte a la izquierda no genera scroll)."
  echo "  Usa el Dropdown/Popover de @sytue/ui —o useFitInViewport si el panel no puede migrar—:"
  printf '%s' "$DERECHA" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

# --- 6) Clase de color con un token que el tema no declara -----------------
# Una utilidad de Tailwind cuyo token no existe NO ES UN ERROR EN NINGÚN SITIO:
# no rompe el build, no la marca el linter y no avisa el navegador. Simplemente
# no se genera, y el elemento sale sin ese estilo.
#
# Sytue Photos entera estaba escrita con `bg-bg-base`, `text-fg-base`, `text-fg-muted`
# y `border-border-base` —cincuenta y dos veces en diez ficheros— y ninguno de
# esos tokens existe: el vocabulario de la suite es `bg`, `text`, `text-muted` y
# `border`. El visor de fotos se veía como HTML sin hoja de estilos, con el
# `<select>` nativo del sistema y los enlaces subrayados en azul. Comprobado
# contra el CSS construido, que es la única forma de saberlo: `bg-bg-base` no
# aparece en la hoja y `bg-bg` sí.
#
# La lista de válidos SALE DEL TEMA, no de aquí: se leen los `--color-*` de
# `packages/config/src/theme.css`, así que un token nuevo funciona sin tocar este
# guion y uno retirado empieza a fallar donde se siga usando.
#
# Solo se miran los prefijos inequívocos de color y solo cuando lo que va detrás
# EMPIEZA por el nombre de una familia del tema. Sin eso, `text-sm`, `border-2` o
# `bg-cover` —utilidades de Tailwind que no son colores— saldrían como fallo.
#
# La DEUDA que se listaba aquí —trece tokens muertos en seis apps— se pagó entera
# (la última, 2026-09-22: `bg-selected`, `success-subtle` y `primary-bg`). Ya no
# hay excepciones: un token que no existe falla, sea nuevo o viejo.
TOKENS="$(grep -oE '^[[:space:]]*--color-[a-z0-9-]+' \
  "$(dirname "$0")/../packages/config/src/theme.css" 2>/dev/null |
  sed 's/^[[:space:]]*--color-//' | sort -u)"
FAMILIAS="$(printf '%s\n' "$TOKENS" | cut -d- -f1 | sort -u | paste -sd'|' -)"
COLOR_HUERFANO=""
if [ -n "$TOKENS" ] && [ -n "$FAMILIAS" ]; then
  USOS="$(grep -rhoE "(bg|text|border|ring|fill|stroke|divide)-($FAMILIAS)(-[a-z0-9]+)*" \
    "${BUSCAR[@]}" 2>/dev/null | sed -E 's/^(bg|text|border|ring|fill|stroke|divide)-//' | sort -u)"
  for uso in $USOS; do
    # Here-string y no `printf | grep -q`: grep sale al primer acierto y el printf,
    # a medio escribir, recibe SIGPIPE; con pipefail el token SÍ declarado salía
    # como huérfano (quality v3.6.2, issue 861).
    grep -qxF -- "$uso" <<<"$TOKENS" && continue
    COLOR_HUERFANO="$COLOR_HUERFANO$uso\n"
  done
fi
if [ -n "$COLOR_HUERFANO" ]; then
  echo "✗ Clase de color con un token que el tema NO declara: no se genera nada."
  echo "  El elemento sale sin ese estilo y nadie avisa. Tokens válidos en"
  echo "  packages/config/src/theme.css; dónde se usan:"
  printf "$COLOR_HUERFANO" | while read -r huerfano; do
    [ -z "$huerfano" ] && continue
    grep -rnE "(bg|text|border|ring|fill|stroke|divide)-$huerfano\\b" "${BUSCAR[@]}" 2>/dev/null |
      head -3 | sed 's/^/    /'
  done
  fallos=$((fallos + 1))
  echo
fi

# --- 7) Ancho fijo mayor que el teléfono más estrecho, sin cota -------------
# La regla 5 exige `absolute` Y `right-0` para disparar. El defecto que llegó a
# PRODUCCIÓN no llevaba ninguno de los dos: el centro de notificaciones era
# `<div class="w-80">` a secas, porque el posicionamiento lo ponía el Dropdown
# padre. 320 px dentro de un popover de 304 px = 16 px fuera, y eso metió scroll
# lateral en TODA la página de las diez apps a la vez. El gate pasó con el
# defecto vivo: la regla escrita para prevenirlo no lo veía.
#
# Aquí no se mira dónde está el panel, solo cuánto mide: un ancho FIJO de 304 px
# o más no cabe en un teléfono de 320 px, esté anclado como esté. El umbral es
# `w-76` = 304 px = `calc(100vw-16px)` a 320 px, que es el hueco real del popover
# canónico; por debajo (`w-72` = 288) cabe y no es defecto.
#
# Salidas legítimas, todas excluidas: cualquier cota en la misma línea
# (`max-w-80`, `max-w-[calc(100vw-16px)]` como ReminderCenter) y el ancho puesto
# tras un breakpoint (`md:w-80`, `max-md:w-full md:w-80` como AppShell), que por
# definición no se aplica en el móvil estrecho.
ANCHOS="$(grep -rnE "(^|[\"'\` ])w-(7[6-9]|8[0-9]|9[0-6])([\"'\` ]|$)" "${BUSCAR[@]}" 2>/dev/null |
  grep -v "max-w-" |
  grep -vE "(max-md:|max-sm:|sm:|md:|lg:|xl:)w-" |
  grep -vE ":[[:space:]]*(//|\*|/\*|\{/\*)" || true)"
if [ -n "$ANCHOS" ]; then
  echo "✗ Ancho fijo de 304 px o más sin cota: no cabe en un teléfono de 320 px y desborda la página."
  echo "  Añade la cota del popover canónico —w-80 max-w-[calc(100vw-16px)]— o usa w-full max-w-80:"
  printf '%s\n' "$ANCHOS" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

# --- 7) Hijo de rejilla sin `min-w-0` --------------------------------------
# Un elemento con `col-span-*` es hijo de una rejilla, y ahí `min-width` vale
# `auto`: NO baja de su ancho mínimo de contenido por mucho que la columna se
# estreche. En un teléfono eso no encoge, empuja — y como el desbordamiento nace
# por encima, arrastra el DOCUMENTO entero a scroll lateral.
#
# Medido en producción el 2026-08-11: las dos secciones del lanzador del portal
# («Accesos rápidos» y «Reciente en toda la suite») medían 487px en una pantalla
# de 320 y sacaban la home 191px fuera, en 7 perfiles de teléfono. La `Card` de
# dentro YA llevaba `min-w-0` y no bastaba: el ancho lo imponía el `<section>` de
# fuera, que es quien cuelga de la rejilla.
#
# `<Card` queda excluido porque el componente ya trae `min-w-0` de serie
# (packages/ui/src/components/Card.tsx): marcarlo sería un falso positivo.
#
# TECHO DE ESTA REGLA, para que su verde no se lea como cobertura: solo dispara
# con `col-span-*`, que aparece 4 veces, mientras la suite tiene 84 contenedores
# `grid-cols-N`. O sea que cubre en torno al 5% de los hijos de rejilla; el resto
# son hijos normales de una rejilla, con el mismo `min-width: auto`. Y no puede
# crecer por ahí: con grep no se empareja un hijo con su padre.
#
# NO se amplía a base de más grep. Se midieron los huecos que quedaban y dan
# CERO: `flex-1|grow|flex-auto` + `whitespace-nowrap` sin `min-w-0` ni `truncate`,
# `w-[NNNpx]` ≥ 304, `min-w-[NNNpx]` ≥ 304 y `min-w-` de la escala 76-96.
#
# La forma de eliminar la CLASE de defecto —y no el 5%— es una línea en
# packages/ui/src/styles.css: `@layer base { :where(.grid) > * { min-width: 0 } }`
# (especificidad cero, cualquier utilidad la pisa). Es un cambio de comportamiento
# global, así que entra con el barrido de las 18 combinaciones de
# test/e2e/mobile-audit.mjs como validación, nunca a ciegas: hoy no hay ningún
# desbordamiento vivo que lo justifique.
REJILLA="$(grep -rnE "col-span-[0-9]" "${BUSCAR[@]}" 2>/dev/null |
  grep -v "min-w-0" | grep -v "<Card" |
  grep -vE ":[[:space:]]*(//|\*|/\*)" || true)"
if [ -n "$REJILLA" ]; then
  echo "✗ Hijo de rejilla sin 'min-w-0': no encoge, empuja, y saca la página entera de la pantalla."
  printf '%s\n' "$REJILLA" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

# EL MÍNIMO TÁCTIL SE MIDE EN PÍXELES, no en la escala de espaciado.
#
# `min-h-11` son 2.75rem, y eso solo da los 44 px que exige la guía táctil si la
# raíz del navegador está en 16. Con la base en 14 —un ajuste de accesibilidad
# corriente— el objetivo quedaba en 38,5: por debajo del mínimo, y justo para
# quien reduce el tamaño de letra, que suele ser quien más precisión necesita.
#
# La forma correcta es `min-h-[max(44px,2.75rem)]`: 44 como SUELO, respetando a
# quien sube la base (con 20, `2.75rem` son 55 y se quedan en 55).
TACTIL="$(grep -rnE "pointer-coarse:min-[hw]-(11|[0-9]+)([^[]|$)" "${BUSCAR[@]}" 2>/dev/null |
  grep -vE ":[[:space:]]*(//|\*|/\*)" || true)"
if [ -n "$TACTIL" ]; then
  echo "✗ Objetivo táctil con la escala de espaciado (rem): mide menos de 44px si la raíz no está en 16."
  echo "  Usa 'pointer-coarse:min-h-[max(44px,2.75rem)]' (y su min-w)."
  printf '%s\n' "$TACTIL" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

# LAS UTILIDADES DE @sytue/ui LLEGAN A LA HOJA DE CADA APP.
#
# Tailwind v4 genera SOLO las utilidades que ve escritas y NO mira dentro de
# node_modules, así que las clases que viven en los componentes compartidos
# dependen de un `@source` que las alcance. Cuando falta, no hay error: la app
# arranca, sirve HTML y va perdiendo clases una a una.
#
# «Mi cuenta» salió así, y no parecía un fallo de estilos: el selector de
# aplicaciones se desplegaba en UNA COLUMNA que se salía de la pantalla —le
# faltaba `grid-cols-3` de AppGrid— y los botones eran texto suelto —les faltaba
# `bg-primary` de Button—. 174 clases generadas contra las 365 que tocaban.
#
# Se comprueba el RESULTADO y no cómo esté escrito el `@source`: hay tres formas
# en uso en el repo y las tres valen. Lo que no vale es que la clase no salga.
CENTINELAS="grid-cols-3 bg-primary"
sin_ui=""
# La hoja de la ofimática vive en su paquete (`packages/office`) y la montan las apps
# de Docus, Calcus, Diapos y CertiPDF, que por eso no tienen `src/styles` propio.
for css in apps/*/src/styles/*.css packages/office/src/styles/*.css; do
  app="${css%/src/styles/*}"
  out="$(mktemp)"
  (cd "$app" && bunx @tailwindcss/cli -i "${css#"$app"/}" -o "$out" >/dev/null 2>&1) || continue
  for clase in $CENTINELAS; do
    grep -q "\.$clase" "$out" || sin_ui="$sin_ui$app: falta .$clase\n"
  done
  rm -f "$out"
done
if [ -n "$sin_ui" ]; then
  echo "✗ La hoja de esta app no trae las utilidades de @sytue/ui: sus componentes saldrán sin estilo."
  printf "$sin_ui" | sed 's/^/    /'
  echo "  El paquete ya declara sus fuentes; revisa que la app importe '@sytue/ui/styles.css'."
  fallos=$((fallos + 1))
  echo
fi

# --- 9) `min-h-6` / `min-w-6` para el mínimo de 24 px -----------------------
# La raíz de la suite está en 14 px (`theme.css`), así que `min-h-6` son 1,5 rem =
# 21 px, no los 24 de WCAG 2.5.8. Se escribía así con el comentario «(24px)» al
# lado y medía 21 en producción (2026-09-21): el disparador del canal de Rooms, el
# enlace de Apple de Account y los «×» del compositor. Un suelo de objetivo va en
# px: `min-h-[24px]`, como en `IconButton` y `SIDEBAR_ACTION_CLASS`.
# Fuera: los comentarios, que citan el patrón para explicarlo, y la tecla dibujada
# de `ShortcutsModal`, que no se pulsa: su `min-w-6` la hace cuadrada con su `h-6`.
MIN6="$(grep -rnE "(^|[\"'\` :])min-[hw]-6([\"'\` ]|$)" "${BUSCAR[@]}" 2>/dev/null |
  grep -vE -e "\.test\." -e "^[^:]+:[0-9]+:[[:space:]]*(//|\*|\{/\*)" -e "^packages/ui/src/components/ShortcutsModal\.tsx:" || true)"
if [ -n "$MIN6" ]; then
  echo "✗ 'min-h-6'/'min-w-6' son 21 px con la raíz a 14, no 24. Usa 'min-h-[24px]'/'min-w-[24px]':"
  printf '%s\n' "$MIN6" | sed 's/^/    /'
  fallos=$((fallos + 1))
  echo
fi

if [ "$fallos" -gt 0 ]; then
  echo "El design system ya resuelve estas cosas; ver packages/ui/src/styles.css."
  exit 1
fi
echo "✓ diseño: anillos de foco, tokens de tema, alturas de layout y escaneo de Tailwind correctos"
