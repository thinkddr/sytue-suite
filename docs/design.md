<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos -->

# Diseño — dirección de arte y sistema de diseño

Documento único de diseño de la suite:

1. **[Dirección de arte](#1-direccion-de-arte)** — el _porqué_: concepto de marca, principios
   visuales y decisiones marco.
2. **[Sistema de diseño](#2-sistema-de-diseno)** — la _spec_: tokens, AppShell, componentes de
   `@sytue/ui`, layout por producto, accesibilidad y reglas obligatorias.
3. **[Assets y vendorizado](#3-assets-y-vendorizado)** — de dónde sale cada asset y cómo llega
   al frontend sin deriva.

> Jerarquía: `branding/tokens/*` = el **número** (fuente de verdad) · esta §1 = el **criterio**
> · esta §2 = la **spec** · `branding/brand-guidelines.md` = la **marca** (logos, iconos,
> ilustración). Ningún valor de color se escribe a mano en las apps.

---

<a id="1-direccion-de-arte"></a>

## 1. Dirección de arte

> **La biblia visual.** Este documento fija la DIRECCIÓN de vanguardia para toda la
> suite y unifica al resto de especialistas (color, tipografía, iconografía,
> ilustración, motion, componentes). Es la capa "por qué" y "hacia dónde"; los valores
> finales viven en `branding/tokens/`, la implementación en `@sytue/ui`, las reglas de
> uso en `docs/design.md#2-sistema-de-diseno` y la marca en `branding/brand-guidelines.md`.
>
> **Regla de oro:** ante una decisión visual, este documento manda sobre el gusto
> individual. Si algo aquí y un token entran en conflicto, se corrige el token o se
> corrige aquí — nunca se improvisa en una app.

---

<a id="10-en-una-frase"></a>

### 1.0 En una frase

**Una herramienta profesional europea: sobria, densa y clara, con una confianza
tranquila.** El listón es Linear / Vercel / Raycast / Mattermost en oficio y pulido —
no como referencia a copiar, sino como el nivel de acabado que damos por sentado. Nada
de look de plantilla, nada de estética "generada por IA" (degradados morados difusos,
glassmorphism gratuito, blobs, ilustraciones isométricas de stock). Si parece una demo,
está mal.

<a id="101-tinta-y-papel"></a>
<a id="101-esmeralda"></a>
<a id="101-color"></a>

### 1.0.1 Lenguaje visual: dos colores elegibles (2026-10-04)

> **Estado:** vigente ([ADR 0071](decisions.md#0071)). Sustituye a «Tinta y papel» y a la
> marca única esmeralda, que duró un día. Los principios de §1.1 y §1.3 no cambian. Las
> anclas `101-tinta-y-papel` y `101-esmeralda` se conservan porque hay enlaces a ellas.

**En una frase:** la suite ofrece DOS colores, **petróleo** (el de quien no ha elegido
nada) y **azul cobalto**. El color pone acción, selección, enlace y foco y tiñe un punto
los neutros; es un eje APARTE del modo: dos colores por tres modos (claro, traslúcido,
oscuro), más el oscuro del sistema. Los estados (éxito, aviso, peligro) no cambian con el
color e «info» sigue siendo azul.

En la hoja el color es `data-color` en la raíz, junto a `data-theme`: sin atributo,
petróleo; `data-color="cobalto"`, cobalto. Sus bloques van al final de `colors.css` y solo
llevan lo que cambia.

**Quién elige.** Cada persona, en su menú de cuenta, junto al modo: muestra y nombre de
cada color. Quien no ha elegido ve el de su organización, que el administrador fija en
Sytue Admin («Color de la organización»); y sin ninguno de los dos, petróleo. La primera
opción del selector es la propia organización: elegirla es olvidar la elección y volver
a seguirla, y vale igual para el color y para la forma (una cuenta personal no la tiene:
no hay quien le fije nada). La elección
vive donde la del modo —cookie cosmética en el apex (`sytue-color`) y almacenamiento
local— y la aplica el mismo script del primer pintado, así que no parpadea. La muestra de
cada color es un token fijo de la hoja, `--sytue-swatch-<color>`, y no el primario en
vigor: el selector tiene que enseñar también el color que no está pintado.

**«El de cada app».** La tercera opción del selector ([ADR 0072](decisions.md#0072)): el
carril y la acción toman el color de producto de la app que se está usando; el Hub y Mi
cuenta se quedan en petróleo. Cada app aporta solo su rampa de primario —generada desde
`--sytue-product-<app>`— y el reparto por modo es común: 600 con rótulo blanco, 700 para
el carril y el enlace, 400 para la acción en oscuro y 300 para el enlace en oscuro. El
resto de la paleta (neutros, estados) no cambia. Docus, Calcus, Diapos y
CertiPDF comparten `/docs` y aun así cada uno pinta el suyo: su layout dice el producto
en `<html data-app>` desde el servidor.

Cinco apps —Mail, PDF, Cast, Diapos y ERP— **solo tiñen el carril**: botón, enlace,
selección y foco siguen en el primario de la casa. Son las apps cuyo color se confunde
con el de peligro en insignias, enlaces y foco, y es una lista decidida mirando cada una,
no una fórmula: Bases, que por tono queda entre Diapos y ERP, no se confunde y lleva
carril y acción. Lo que se mide es quién pasa por esa decisión: una app cuyo color cae a
menos de 26° del tono (OKLCH) de `--sytue-danger` tiene que estar apuntada en una de las
dos listas del guardián, o falla. El carril y el marco pintan con su propia rampa,
`--sytue-rail-*`, que es la del primario salvo en esas cinco.

| Rol                     | Petróleo claro | Petróleo oscuro | Cobalto claro | Cobalto oscuro |
| ----------------------- | -------------- | --------------- | ------------- | -------------- |
| Acción (`primary`)      | `#0f7680`      | `#3cc4cc`       | `#3b5fe0`     | `#4169e8`      |
| Rótulo sobre la acción  | `#ffffff`      | `#04161a`       | `#ffffff`     | `#ffffff`      |
| Enlace (`primary-text`) | `#0c636b`      | `#7fd3d8`       | `#2f4fc4`     | `#a3b9ff`      |
| Foco (`focus-ring`)     | `#0f7680`      | `#7fd3d8`       | `#3b5fe0`     | `#a3b9ff`      |
| Lienzo (`bg`)           | `#f1f6f6`      | `#0a1011`       | `#f3f5fa`     | `#0e111a`      |
| Superficie (`surface`)  | `#ffffff`      | `#131c1e`       | `#ffffff`     | `#161b28`      |
| Texto (`text`)          | `#0f1d20`      | `#e7efef`       | `#141a2b`     | `#e9ecf5`      |
| Carril (web)            | `#0c5f68`      | `#0d2a2e`       | `#3355d6`     | `#141d3d`      |

**Reglas**

1. **`on-primary` no es «blanco».** Es el rótulo del relleno PRIMARIO, y depende del
   color y del modo: en petróleo oscuro es casi negro (el primario es un turquesa claro);
   en cobalto es blanco también en oscuro. Sobre el primario,
   `bg-primary text-on-primary`; sobre cualquier otro relleno —peligro, éxito, un color de
   persona o de producto, el escalón 600 fijo—, `text-white`.
2. **«Info» es azul.** No es un alias del primario: con cobalto coincide de tono y con
   petróleo no, y un aviso informativo no puede cambiar de significado con el color.
3. **Lo que vive sobre el cromo usa los tokens del cromo:** estados con
   `bg-chrome-hover`/`bg-chrome-active`, bordes con `border-chrome-border`, texto con
   `text-header-fg`/`text-sidebar-fg` y foco con `sytue-focus-ring-chrome`. Velos
   blancos literales (`bg-white/10`) solo en superficies que son oscuras en todos los
   temas: visor de logs, terminal, sala de vídeo, visor de adjuntos.
4. **Tipografía:** Figtree (SIL OFL) para los 24 idiomas de alfabeto latino e Inter
   (OFL) para griego y cirílico, autoalojadas desde `@fontsource-variable` e importadas
   una sola vez en `@sytue/ui/styles.css`. Versalitas (`sytue-overline`) a `0.06em`.
5. **Nada de decoración de relleno:** sin emojis en la interfaz ni en los textos, sin
   degradados, brillos ni blobs más allá del fondo ambiental del traslúcido.
6. **Un color nuevo es un bloque más, no una hoja más.** Declara lo que cambia respecto
   al petróleo y, en cada modo, repite lo que su bloque claro declara (a igual peso de
   selector ganaría el claro, que va después). Lo vigila `contraste-paleta.test.ts`.

<a id="forma-marco"></a>

**La forma es un eje aparte del modo y del color.** Vive en `data-forma`, en la raíz,
junto a `data-theme` y `data-color`: **sin atributo se ve «Marco»**, que es la forma por
defecto de la web, y `data-forma="carril"` es la que cada persona puede elegir. Sigue el
mismo camino que el color (`@sytue/ui`, `theme/init.ts`, `theme/preference.ts` y
`ThemeProvider`): `THEME_FORMAS` es la lista, `readThemeForma()` la lee con el lector
común (`readThemePreference`) de la cookie del dominio de la suite y de `localStorage`
(clave `sytue-forma`), `applyThemeForma()` pone o quita el atributo, y el guion de primer
pintado (`themeInitScript`) aplica los tres ejes antes de pintar. `useTheme()` da `forma`
y `setForma`, y el selector está en el menú de la cuenta. **Quien no ha elegido ve la
forma de su organización**, que el administrador fija en Sytue Admin: es una columna de
`organizations` en Sytue ID (`forma`, migración 0108), viaja en el id_token como
`org_forma` solo si no es la de la casa, y el BFF la deja en la cookie `sytue-forma-org`,
que es lo que lee el primer pintado. Es el mismo camino que el color, con la plomería
compartida: un solo manejador (`handleAdminPutApariencia`) y un solo guardado
(`GuardarApariencia`) en Sytue ID, una sola ruta (`api/admin/org/[eje]`) y una sola
tarjeta (`OrgAppearanceCard`) en la consola.

No hay dos juegos de componentes. `Button`, `Card`, `Sidebar`, `Table`, `List`, `TopBar` y
las carcasas (`AppShell`, `AdminShell`) leen **tokens de forma**, que el atributo
reasigna; las medidas están en `theme.css` («Tokens de forma») y lo que pinta, en el
bloque de la forma de `appearance.css`, siempre con tokens de rol.

| Token                                              | Marco     | Carril      | Qué gobierna                            |
| -------------------------------------------------- | --------- | ----------- | --------------------------------------- |
| `--radius-control` (`rounded-control`)             | 999 px    | 9 px        | Botones, botones de la barra            |
| `--radius-nav` (`rounded-nav`)                     | 999 px    | 9 px        | Ítems del menú lateral                  |
| `--radius-card` (`rounded-card`)                   | 16 px     | 14 px       | Tarjetas                                |
| `--radius-row`                                     | 10 px     | 0           | Fila de lista elegida o bajo el cursor  |
| `--radius-sheet`                                   | 16 px     | 0           | La hoja del contenido                   |
| `--sytue-sheet-gap`                                | 10 px     | 0           | Separación de la hoja (0 bajo 768 px)   |
| `--sytue-nav-pad`                                  | 14 px     | 10 px       | Relleno horizontal del ítem del menú    |
| `--sytue-nav-indicator`                            | 0         | 3 px        | Barra de «estás aquí»                   |
| `--sytue-nav-active-weight`                        | 650       | 500         | Peso del ítem activo                    |
| `--sytue-rail-item-bg` / `-fill`                   | sin fondo | velo, ámbar | Icono en reposo del carril de iconos    |
| `--sytue-rail-active-bg` / `-fg`                   | velo      | macizo      | Icono activo del carril de iconos       |
| `--sytue-nav-active-bg` / `-fg`                    | velo      | velo        | Ítem activo (ver abajo)                 |
| `--sytue-row-grow`                                 | 1 px      | 0           | Relleno extra de fila, arriba y abajo   |
| `--sytue-row-inset`                                | 6 px      | 0           | Sangría lateral de la fila de lista     |
| `--sytue-row-bar`                                  | 0         | 2 px        | Barra de la fila de lista elegida       |
| `--sytue-title-weight` / `--sytue-subtitle-weight` | 800 / 700 | 700 / 600   | `sytue-h1`–`h2` / `h3`–`h4`, en la hoja |
| `--sytue-title-scale`                              | 1,1       | 1           | Cuerpo de los títulos, en la hoja       |

**Forma «Marco»** (la que se ve sin elegir nada):

1. **La barra superior y el menú lateral son un marco de color continuo** (`--sytue-frame-*`:
   el color del menú lateral; en traslúcido, un degradado horizontal del primario, de
   su escalón 900 al 700). La barra entera es cromo a cualquier ancho:
   `TopBar rail` lleva `sytue-topbar-carril` —la clase que en <768 px ya la hace carril
   en las dos formas—, que entra en la misma reasignación de tokens que el carril, sin
   línea inferior. El buscador es el velo neutro `--sytue-sidebar-field` con el texto
   del cromo.
2. **El contenido es una hoja** (`sytue-sheet`) de radio 16 que flota dentro con sombra
   `md`, separada 10 px del borde derecho e inferior (y del izquierdo si no hay menú).
   En traslúcido es un cristal: `surface-raised` al 82 % sobre el marco. La raíz de la
   carcasa (`sytue-frame`) pinta el marco. Las páginas que desplazan el documento (el
   Hub) usan dos envoltorios, `sytue-frame-page` y `sytue-sheet-page`, que en Carril son
   `display: contents`.
   **La hoja no recorta** (decidido el 2026-10-05; lo vigila `contraste-paleta.test.ts`).
   Llevó `overflow: clip` para las esquinas, y una caja que recorta deja sin alcance lo
   que no contiene: en la v3.11.1 la portada de Docs perdía la lista de documentos en un
   móvil. Las esquinas las redondea **lo de dentro**: la hoja publica sus cuatro radios
   en `--sytue-sheet-{tl,tr,br,bl}` y cada pieza que toca una esquina lleva
   `sytue-sheet-corners`; la que solo toca algunas pone a `0px` las demás
   (`[--sytue-sheet-tl:0px]`), y lo hereda lo que lleve dentro. En un panel que desplaza,
   el radio recorta además su contenido. Los armazones comunes (`AppShell`, `AdminShell`)
   ya lo hacen por todas sus apps; una pantalla con armazón propio que pinte un fondo
   hasta la esquina de la hoja tiene que marcar esa pieza.
3. **Botones y navegación en píldora.** El ítem activo del menú es una píldora de velo
   claro con el texto blanco (`--sytue-nav-active-*`, los mismos valores que en Carril:
   `--sytue-sidebar-active-bg`/`-fg`), sin barra de señal. La píldora maciza —blanca con
   el rótulo del color; en oscuro, el primario claro con rótulo oscuro— es solo del
   botón principal del menú («Redactar», «Nuevo»): con las dos iguales, una sobre otra,
   no se distinguía el botón (decidido el 2026-10-05). Y va en **negrita**
   (`--sytue-nav-active-weight`, 650): el velo solo no llega al 3:1 de un indicador, y
   sin barra de señal «estás aquí» lo dice el peso.
4. **Filas más altas** (1 px más arriba y abajo en `Table` y `List`); en `List`, la fila
   elegida o bajo el cursor es un bloque de radio 10 separado 6 px de los bordes, sin
   barra a la izquierda. **Títulos** un 10 % más grandes y un escalón más gruesos, solo
   dentro de la hoja: el sitio público y las páginas sin sesión comparten esas clases.
5. **Tarjetas de recurso de color pleno:** un `Card` con `tone` —o cualquier elemento
   con `cardTone(tono)`, que reparte la misma clase y las dos variables entre su `class`
   y su `style`— se rellena con el color de grupo y su `CardIcon` pasa a un velo de la
   tinta del relleno. Dentro de un contenedor `sytue-tiles` esos elementos son
   **teselas**: una rejilla, con el icono arriba y el rótulo debajo. Hoy: los accesos
   rápidos del Hub (filas con icono en Carril, teselas en Marco), las carpetas de
   Drive y las tarjetas de proyecto de Cloud. En Drive, la vista de cuadrícula las
   pinta en su sitio y la de **lista** las saca arriba, como una franja de teselas
   bajas (icono, nombre y tipo) sobre la lista de ficheros: dos piezas en pantalla y
   una sola serie de posiciones para la selección por rango (`apps/drive`,
   `lib/teselas.ts`); en Carril, en la cuadrícula y en una búsqueda no hay franja. Un `Card` sin `tone`
   es un panel y sigue blanco. El rótulo sobre cada relleno lo mide el guardián
   («Tarjeta: rótulo sobre su color», ≥ 4,5 en los nueve tonos).
6. **Dónde la hoja va a sangre** (medido con Playwright, 2026-10-04):
   - **Por debajo de 768 px, siempre.** Flotando quitaba 16 de los 390 px de ancho
     (4,1 %) y 8 de alto. A sangre no quita nada y conserva las dos esquinas de arriba,
     que es lo que la sigue leyendo como hoja bajo la barra de color.
   - **En los editores** (Docus, Calcus, Diapos: la carcasa de Sytue Docs entera, con su
     portada). Flotando quitaba 20 px de ancho y 10 de alto al lienzo a 1280×800. Va con
     `sytue-sheet-bleed`: sin separación y con solo las esquinas de arriba.
   - El editor de Sytue Web es un editor a pantalla completa, sin la barra de la suite:
     no lleva marco ni hoja. Sus pantallas de lista y de ajustes sí (montan `AppShell`).
   - El ERP tiene carcasa propia y lleva hoja como los demás: su raíz es `sytue-frame`
     y su `main` va dentro de `sytue-sheet`. Su **carril de iconos** es parte del marco
     y sigue los tokens `--sytue-rail-*`: en Marco, icono blanco sin fondo en reposo y,
     en el activo, un velo y una **barra ámbar** de 3 px a plena opacidad en el borde del
     carril (`--sytue-rail-indicator`), que es lo que dice «estás aquí» donde no hay
     rótulo que poner en negrita: el relleno del icono se pinta al 34 % y no llega a 3:1; en Carril, velo en reposo y cuadro
     macizo en el activo, como siempre. El cajón de módulos usa los tokens del menú.
7. **Impreso no hay forma:** todo lo que pinta el marco y la hoja va en `@media screen`.
8. **Las apps nativas (Fyne) llevan la misma forma, leída de las mismas hojas:** embeben
   copia de `theme.css` y de `appearance.css` y piden cada medida por su token, así que
   un cambio en la tabla de arriba les llega sin tocar Go. Cómo pintan la hoja sin
   recorte de esquinas, y qué va a sangre, en
   [CONVENTIONS §13.6](CONVENTIONS.md#13-6-la-interfaz-nativa-es-fyne-con-todo-propio--normativo-2026-09-21).
9. **El Hub, «Bienvenida y tablero»** (propuesta B, 2026-10-05): saludo por la hora local
   («Buenos días / Buenas tardes / Buenas noches, <nombre>», claves
   `launcher.greeting.*` del portal), las acciones del día en píldoras —cuatro de los
   accesos rápidos, la primera como principal, y un «Más» con el resto; buscar ya está
   en la barra—, las apps en teselas compactas (icono
   de producto de 42 px a la izquierda, 62 px de alto, tres columnas desde 1024 px) a
   dos tercios del ancho y, al lado, «Hoy» y «Reciente» (sus tres últimas entradas);
   con seis apps o menos, «Reciente» baja a la columna de las apps. En Carril, la portada de
   siempre. Es un solo marcado: lo que una forma enseña y la otra no va con las
   variantes `marco:` y `carril:` de `theme.css`, no con una rama en el componente, que
   parpadearía al hidratar porque la forma no se conoce en el servidor.

**Forma «Carril»** (la alternativa que se elige; dónde vive el color y qué relieve tiene el contenido):

1. **El color de marca vive en el menú lateral**, del color elegido de arriba abajo. Texto blanco,
   ítems en reposo atenuados, el activo con velo blanco y una barra **ámbar**
   (`#ffd45e`) de «estás aquí». Fuera del carril el indicador es el primario. Los valores
   del carril viven en `appearance.css`, que cargan la web (después de `colors.css`) y
   las apps nativas (`apps/nucleo/tokens` la lee encima, en el mismo orden): las dos
   pintan el mismo carril. En las nativas no hay cascada, así que cada pieza del menú
   pide el token del carril por su nombre (`docs/CONVENTIONS.md` §13.6).
2. **La barra superior es clara.** Va de lado a lado por encima del menú, así que la
   forma se consigue sin mover el DOM: su zona de marca (`TopBar rail`, el conmutador
   de aplicaciones) se pinta del color del carril y, desde 768 px, con el ancho del
   menú lateral, y se lee como su cabecera. En una app sin menú lateral queda como
   esquina de marca, ceñida a la marca y al conmutador de aplicaciones: no pinta el ancho
   de un menú que no existe, pero su caja lo sigue reservando y el buscador no se mueve
   al pasar de una app a otra (`.sytue-esquina-marca`). **En <768 px** el menú lateral
   va en un cajón y no se ve: la barra ENTERA es el carril (`.sytue-topbar-carril`), con
   texto blanco y el buscador en un velo oscuro neutro (`--sytue-sidebar-field`), que
   vale con cualquier color de carril.
3. **Dentro del carril los tokens del cromo son otros.** `bg-sidebar` y `bg-rail`
   reasignan en su subárbol la familia `chrome-*`, el indicador y el relleno de los
   glifos (ámbar), el foco (blanco) y el botón principal (blanco con rótulo del color:
   uno del mismo color no se vería). Lo que se abre dentro con superficie propia —el
   conmutador de aplicaciones, un menú, un diálogo, una tarjeta blanca— vuelve a los
   valores del tema. La regla y su porqué están en `branding/tokens/appearance.css`.
4. **Contenido limpio:** lienzo gris verdoso y superficies blancas que se despegan con
   sombra suave. Radio 9 para controles y 14 para tarjetas, paneles, menús y diálogos.
5. **Tarjetas de recurso:** superficie blanca con el color de grupo en un ICONO sobre
   relleno sólido (`CardIcon`), en vez de fondo pastel.
6. **Lo que se apoya en el carril con color propio se separa de él.** Una muestra de
   color —el punto de una etiqueta, la casilla de un calendario, un punto de presencia—
   lleva un aro del color del texto del carril (`ring-sidebar-fg ring-[1.5px]`): puede
   ser del mismo tono que el carril, y el aro es lo único que la distingue. El logo de la
   organización va SIEMPRE sobre placa blanca redondeada, como la marca de la casa. Y
   en el traslúcido, el menú como cajón (en <768 px, y el de módulos del ERP) es opaco:
   debajo no tiene el fondo ambiental sino contenido, y su texto se colaba. El avatar de
   la cuenta y el punto de aviso de la campana llevan el mismo aro cuando la barra
   superior es de color (`.sytue-aro-carril`: en «Marco» siempre; en «Carril», en
   <768 px): el rojo de peligro no llega a 3:1 sobre el carril. Con «reducir transparencia», el carril
   del traslúcido es opaco también en escritorio (en «Marco» ya lo es el marco). Y lo
   **atenuado** dentro del carril —el canal silenciado de Rooms— se atenúa con el tono
   de «ítem en reposo» y un peso menos, nunca con opacidad: no hay margen por debajo.

**Contraste medido** (WCAG 2.x) sobre la hoja real, no sobre una maqueta: lo calcula
`apps/frontend/packages/config/src/contraste-paleta.test.ts`, que lee `colors.css` y
`appearance.css`, aplica los bloques como la cascada, aplana velos y cristales sobre lo
que tienen debajo y falla si una medida baja de su mínimo, en los dos colores por los
cuatro modos. En el traslúcido cada medida es la PEOR entre las tres paradas del
degradado y la luz cálida. Se reproduce con `MEDIDAS=1 bun test contraste-paleta` desde
`packages/config`.

| Medida                                     | Mín. | Petróleo claro | Petróleo traslúcido | Petróleo oscuro | Cobalto claro | Cobalto traslúcido | Cobalto oscuro |
| ------------------------------------------ | ---- | -------------- | ------------------- | --------------- | ------------- | ------------------ | -------------- |
| Texto sobre superficie                     | 4,5  | 17,26          | 14,98               | 14,84           | 17,32         | 14,63              | 14,56          |
| Texto atenuado sobre superficie            | 4,5  | 7,82           | 8,58                | 8,40            | 8,08          | 8,23               | 8,17           |
| Texto sutil sobre superficie               | 4,5  | 5,67           | 7,00                | 6,17            | 5,76          | 6,82               | 5,94           |
| Texto sutil sobre el lienzo                | 4,5  | 5,20           | 5,20                | 6,84            | 5,28          | 4,76               | 6,51           |
| Texto sobre fila seleccionada              | 4,5  | 14,62          | 12,14               | 10,92           | 14,88         | 12,04              | 11,53          |
| Enlace sobre superficie                    | 4,5  | 6,97           | 8,64                | 10,07           | 6,92          | 7,26               | 8,96           |
| Rótulo del botón principal                 | 4,5  | 5,35           | 5,35                | 8,78            | 5,39          | 5,39               | 4,76           |
| Botón principal contra la superficie       | 3    | 5,35           | 4,65                | 8,21            | 5,39          | 4,55               | 3,61           |
| Carril: ítem activo                        | 4,5  | 5,28           | 5,16                | 10,46           | 4,81          | 4,77               | 11,67          |
| Carril: ítem en reposo                     | 4,5  | 5,72           | 6,02                | 8,06            | 5,00          | 4,99               | 8,94           |
| Carril: botón principal contra el carril   | 3    | 7,36           | 7,23                | 7,18            | 6,16          | 6,13               | 3,47           |
| Carril: rótulo del botón principal         | 4,5  | 7,36           | 7,36                | 8,78            | 6,16          | 6,16               | 4,76           |
| Carril: anillo de foco                     | 3    | 7,36           | 7,23                | 15,13           | 6,16          | 6,13               | 16,50          |
| Carril: indicador «estás aquí»             | 3    | 3,73           | 3,65                | 7,39            | 3,40          | 3,37               | 8,24           |
| Carril: texto del buscador en móvil        | 4,5  | 10,77          | 10,63               | 14,72           | 9,47          | 9,43               | 15,30          |
| Carril: marca de agua del buscador (móvil) | 4,5  | 8,10           | 8,63                | 8,92            | 7,46          | 7,44               | 9,60           |
| Carril: aro de las muestras de color       | 3    | 7,36           | 7,23                | 12,96           | 6,16          | 6,13               | 13,97          |
| Carril: contador de un canal silenciado    | 4,5  | 6,24           | 6,10                | 10,87           | 5,45          | 5,41               | 11,89          |
| Carril: aro del punto de aviso y el avatar | 3    | 7,36           | 7,23                | 12,96           | 6,16          | 6,13               | 13,97          |
| Barra superior: texto                      | 4,5  | 17,26          | 13,97               | 15,95           | 17,32         | 13,47              | 15,28          |
| Barra superior: marca de agua del buscador | 4,5  | 5,67           | 7,52                | 6,17            | 5,76          | 7,43               | 5,94           |
| Tarjeta: rótulo sobre su color             | 4,5  | 4,96           | 4,96                | 4,56            | 4,68          | 4,68               | 4,56           |
| Contorno de control                        | 3    | 4,55           | 4,62                | 3,65            | 4,57          | 4,59               | 3,61           |
| Anillo de foco                             | 3    | 5,35           | 6,05                | 10,07           | 5,39          | 5,84               | 8,96           |
| Indicador «estás aquí» fuera del carril    | 3    | 5,35           | 6,05                | 8,21            | 5,39          | 5,84               | 5,67           |

**Fuera de este paso:** la marca —el logotipo y la cuña de color por organización se
quedan como están—, los colores de producto y de persona, y las tablas de documentos y
el lienzo de los sitios creados con Sytue Web, que conservan sus propios estilos.

---

<a id="superficies-claras-multicolor"></a>

### 1.0.2 Tres apariencias compartidas (2026-10-04)

La suite ofrece claro, traslúcido y oscuro, además de seguir la apariencia del sistema.
El claro usa un lienzo gris verdoso con superficies blancas; el traslúcido pone velos
blancos sobre un fondo amplio del color con una luz cálida; el oscuro usa un casi
negro con un punto de verde y superficies escalonadas. Los estados siguen expresándose
con texto e iconos. Las acciones y los controles de cantidades van en el color elegido en las
tres apariencias, con el rótulo que marca `on-primary`: blanco en claro y en traslúcido,
casi negro en oscuro (§1.0.1).

Las tarjetas disponen de nueve familias compartidas: azul, violeta, verde, rosa,
amarillo, cian, coral, lima y grafito. Cada familia tiene un relleno sólido y uno
teñido adaptado al tema: claro, traslúcido u oscuro. El texto de las tarjetas sólidas
cambia entre blanco y tinta según su contraste. El color identifica recursos y
agrupa contenido; no reemplaza las etiquetas de estado ni el primario de las acciones.

La [referencia de composición](https://cdn.dribbble.com/userupload/15998211/file/original-b94dc14825242b117649e09e0ab66543.png)
orienta las capas, la jerarquía y el espacio entre grupos. Figtree, ya autoalojada en la
suite, acompaña esas formas geométricas sin incorporar Lufga ni otra fuente comercial.
Se conservan los avisos y la licencia SIL OFL de las fuentes distribuidas.

Las tarjetas tienen radios de 16–20 px y el menú lateral de 4–8 px. Los separadores
conservan espacio antes y después. El TPV usa una navegación compacta y mantiene todos
los modelos disponibles en un cajón con control de foco. Sus imágenes ocupan el fondo
de la tarjeta y los controles de edición aparecen solo al activar Imágenes.

Los formularios amplios pueden usar el panel lateral derecho de `Modal`, con el mismo
control de foco, Escape y retorno al disparador. Los pop-over usan un desenfoque de
24–28 px, con superficie sólida cuando no está disponible o se prefiere reducir
transparencia. Las aperturas usan movimientos de 150–180 ms y respetan movimiento
reducido. Se evita aplicar desenfoque repetidamente a todas las tarjetas del catálogo.

La paleta nace en `branding/tokens/colors.css` y `colors.json`, vendorizadas para la
web, Sytue ID y las aplicaciones nativas. Los radios salen de
`branding/tokens/spacing.json`. La raíz web es de 16 px: `text-base` mide 14 px y rem
coincide con la geometría nativa. Los valores vigentes de marca están en §1.0.1. Las
tablas de documentos y el lienzo de los sitios creados con Sytue Web conservan sus
propios estilos, igual que los logotipos de cada organización.

<a id="11-concepto-y-personalidad-de-marca"></a>

### 1.1 Concepto y personalidad de marca

Sytue es la suite de productividad **europea, soberana y libre**. La dirección visual
existe para transmitir tres cosas, en este orden:

1. **Confianza.** Esto guarda tu correo, tus documentos, tu empresa. Debe verse
   estable, preciso e incorruptible. La confianza se comunica con **sobriedad y
   consistencia**, no con decoración. Cero elementos que griten.
2. **Claridad.** El usuario siempre sabe dónde está, qué puede hacer y qué acaba de
   pasar. Jerarquía sin ambigüedad. El diseño desaparece; queda el trabajo.
3. **Soberanía europea.** Origen europeo como orgullo sereno, no como bandera
   ondeando. Se expresa en el **rigor** (precisión centroeuropea, tipografía honesta,
   nada superfluo) más que en iconos de banderas. Cuando la bandera aparece (footer,
   login), es un detalle sobrio, nunca un adorno.

**Personalidad (adjetivos operativos):** precisa · serena · densa · honesta · atemporal.
**Anti-personalidad (lo que NUNCA somos):** juguetona, ruidosa, "friendly startup",
maximalista, tendenciosa, genérica.

**Arquetipo:** el instrumento de precisión. Un buen teclado mecánico, un Braun de Rams,
un panel de Linear. Invisible cuando trabajas, satisfactorio cuando lo notas.

---

<a id="12-moodboard-en-palabras"></a>

### 1.2 Moodboard en palabras

- **Superficie:** papel cálido. Marfil ligeramente cremoso (`#faf9f6`), no blanco frío
  ni azulado. Grises cálidos (greige) que se sienten como papel de algodón, no metálicos.
- **Profundidad:** casi plano. La elevación es un susurro — 1px de borde y una sombra
  que apenas se percibe. Las cosas flotan un milímetro, no un centímetro.
- **Color:** disciplina cromática severa. El 95% de la pantalla es neutro (marfil +
  grafito); el **latón** aparece solo donde hay acción o marca. El color es **caro**: se
  gasta con intención, como un detalle de latón sobre madera noble.
- **Densidad:** un panel de control, no una landing. Mucha información legible en poco
  espacio, respirando lo justo. Estilo Bloomberg-terminal-hecho-humano.
- **Ritmo:** rejilla estricta de 4px. Todo se alinea. La sensación de calidad viene de
  que nada está a 3px de donde debería.
- **Motion:** cortinas que se abren rápido y con firmeza. 120–200ms, nunca rebotes ni
  balanceos. El movimiento confirma, no entretiene.
- **Modo oscuro:** no es "el claro invertido". Es un despacho revestido en grafito —
  grafitos cálidos profundos (`#131619`), superficies que se distinguen por 1–2 pasos de
  gris, el latón brillando lo justo para guiar el ojo.

**Referencias de nivel (listón, no molde):**

- **Linear** — densidad, motion con propósito, jerarquía por peso y color, oscuridad
  premium.
- **Vercel / Geist** — sobriedad monocroma, uso quirúrgico del acento, tipografía como
  estructura.
- **Raycast** — comando y velocidad, superficies elevadas mínimas, foco en contenido.
- **Mattermost** — densidad de herramienta de trabajo real, sidebar de marca, listas
  interminables legibles.

De cada uno tomamos el **principio**, no el estilo. No copiamos su azul, su radio ni su
ilustración. La identidad Sytue es propia y hoy es la de §1.0.1: neutros con un punto de
su tono, un acento elegible (petróleo o cobalto), geometría de tile redondeado y Figtree. Los
valores de «Graphite & Brass» que quedan en este moodboard (marfil, grafito, latón) son
historia: léase «lienzo», «neutro» y «primario».

---

<a id="13-principios-visuales"></a>

### 1.3 Principios visuales

#### 3.1 Jerarquía

La jerarquía se construye, en este orden de preferencia:
**1) peso tipográfico → 2) color de texto (text / muted / subtle) → 3) espaciado → 4) tamaño → 5) (último recurso) elevación/sombra.**
Nunca se usa una sombra grande o un fondo de color para hacer que algo "destaque" si un
`font-weight: 600` y un poco de aire lo resuelven. Una vista bien jerarquizada se lee
en gris; el color solo marca la acción.

#### 3.2 Densidad

Densidad **media-alta**, deliberada. Base tipográfica 14px. Objetivo: caber más sin
apretar. Filas de lista compactas (~32–40px), padding interno de 12px, contenedor 16px,
sección 24px. La densidad es un valor de marca — somos una herramienta de trabajo, no
un folleto. Pero densidad ≠ apretado: siempre respira lo suficiente para escanear.

#### 3.3 Profundidad y elevación

Elevación **mínima y semántica**. Casi todo vive en el plano base con un borde de 1px.
Se eleva solo lo que flota de verdad y de forma efímera:

- base (contenido, cards en reposo): borde `1px`, sin sombra o `xs`.
- dropdown / popover: `md`.
- modal / dialog: `lg`.
- overlay de página: `xl`.
  Regla: **si no se abre por encima de otra cosa, no lleva sombra.** El borde es el
  recurso primario de separación; la sombra es solo para "esto está por encima ahora
  mismo". En oscuro se usan las variantes `shadows.dark` (más contraste, menos difusión).

#### 3.4 Uso del color de acento

- **Primario (petróleo o cobalto):** acción principal, marca, enlaces, foco, estado
  seleccionado/activo. Es el único color que dispara "aquí puedes actuar". Como relleno
  con texto blanco usa el 600; como texto sobre blanco, el 700; en oscuro el relleno es el 400 o el
  500 según el color, con el rótulo que marca `on-primary`, y el texto, el 300. No se usa como decoración de fondos amplios.
- **Tinta marina (secundario):** la familia `accent` queda para gráficos y datos. No es
  un acento de acción.
- **Violeta `#7c5cff` (terciario):** exclusivamente diferenciación de datos/producto
  (gráficos, Calendar, Meet, etiquetas). Nunca como color de acción de UI.
- **Semánticos** (success/warning/danger/info): solo para significado de estado, nunca
  decorativos. Y **nunca solo por color**: siempre acompañados de icono o texto.
- **Colores de producto** (`colors.json → product`): identifican cada app **solo en el
  tile del app-switcher** (orientación entre apps). El cromo es el mismo en todas; el
  color de producto no tiñe la cabecera ni la UI de trabajo.

Presupuesto cromático de referencia: en una pantalla típica, ≤ ~10% de píxeles de color
de marca/acento. Si hay más, sobra color.

#### 3.5 Tipografía

**Inter** para todo el producto (open font, SIL OFL — coherente con el valor libre);
**JetBrains Mono** para código, logs del PaaS y SQL. Una sola familia de texto en toda
la suite: la coherencia tipográfica es el mayor multiplicador de "esto es un solo
producto".

- **Peso como herramienta principal de jerarquía:** 400 cuerpo, 500 énfasis suave,
  600 títulos/labels/botones, 700 solo display/H1–H2.
- **Tracking negativo (`-0.01em`) en títulos** (display, H1, H2) para que se sientan
  compactos y modernos; normal en cuerpo; `+0.04em` en mayúsculas (overline).
- **Números:** usar cifras tabulares en tablas, listas de datos y métricas para que las
  columnas se alineen.
- Escala modular ~1.2 sobre 14px. Las apps consumen **roles** (`display`, `h1`–`h4`,
  `body`, `label`, `caption`, `overline`, `button`, `code`), nunca tamaños sueltos.
- Line-height generoso en cuerpo (~1.5) para legibilidad densa; ajustado en títulos.

#### 3.6 Ritmo y espaciado

Rejilla base **4px**, sin excepciones. Todo padding, gap y margen es múltiplo de la
escala. La calidad percibida nace de la alineación milimétrica. Contenido a máx.
**1280px** con gutter de 24px. El sistema de layout es fijo y reconocible en todas las
apps: **TopBar 56px + Sidebar 260px (o app rail 64px) + contenido**, con panel
secundario para bandejas (Mail, Chat). No se reinventa el layout por app.

#### 3.7 Motion

Movimiento **con propósito, rápido y sobrio.**

- Duración: `120ms` (micro: hover, foco), `150ms` (base: la mayoría), `240ms` (entradas
  de overlay/modal). Nada por encima de 240ms en UI.
- Easing: `cubic-bezier(0.2, 0, 0, 1)` (entrada firme y decidida) para casi todo;
  `cubic-bezier(0.4, 0, 1, 1)` para salidas.
- **Prohibido:** rebote (bounce), balanceo, spring exagerado, parallax decorativo,
  auto-animaciones que distraen. El motion **confirma un cambio de estado** (algo se
  abrió, se seleccionó, llegó), nunca decora.
- Respetar **siempre** `prefers-reduced-motion`: sin transiciones no esenciales.

#### 3.8 Guía, no formulario

La configuración es **producto**, no un trámite. Referencia: **Apple** (te lleva de la mano,
un paso y una idea a la vez), **Vercel** y **DigitalOcean** (superficies limpias que explican
la consecuencia de cada opción). Antirreferencia explícita: la densidad de consola de **Google
Cloud** y **AWS**, y el «muro de formulario» de 20 campos sin contexto.

- Un usuario **básico** debe poder configurar cualquier cosa **sin leer documentación**.
- Lenguaje llano y orientado a la consecuencia («¿A quién afecta esta regla?»), nunca el
  nombre del campo de la base de datos.
- Cada dato que venga de otro sistema dice **dónde encontrarlo**; lo que se pueda probar, se
  prueba **antes** de guardar; lo que se vaya a aplicar se **resume** antes de aplicarlo.
- Aplica a **toda** la suite —Console, ajustes de cada producto, de la organización y de la
  cuenta—, tanto a lo nuevo como a lo ya existente.

Es un requisito **permanente**; las reglas y las primitivas obligatorias están en
`docs/design.md#2-sistema-de-diseno` §8.

---

<a id="14-decisiones-marco-para-los-especialistas"></a>

### 1.4 Decisiones marco (para los especialistas)

Direcciones, no re-derivación de valores. Los números finales ya viven en
`branding/tokens/`; aquí se fija el criterio que los gobierna.

#### 4.1 Paleta — dirección

- **Base neutra con un punto de verde:** lienzo `#f1f5f3` y superficies blancas en
  claro, `#0a0f0d`–`#1a2520` en oscuro. Los grises tienden al matiz de la marca, nunca a
  azul frío.
- **Un acento, elegible entre dos:** petróleo (`#0f7680`) o cobalto (`#3b5fe0`), el único solista de acción.
- **Escalas completas 50→950** para primario, acento (tinta marina) y
  neutro, calibradas para contraste **WCAG AA** sobre superficies claras y oscuras.
  Cualquier par texto/fondo nuevo se valida a ≥ 4.5:1 (texto) / ≥ 3:1 (UI y texto
  grande), y los pares canónicos los vigila `contraste-paleta.test.ts` (§1.0.1).
- **Especialista de color:** trabaja dentro de estas escalas; si falta un tono, se
  añade al token, no se inventa un HEX en una app. Modo oscuro **no** es inversión: es
  una re-asignación de roles pensada aparte (ver `colors.json → theme.dark`).

#### 4.2 Tipografía — dirección

- **Familia única Inter** en todo el producto; **JetBrains Mono** solo técnico. No se
  introduce ninguna otra familia en superficies de producto.
- **Escala modular ~1.2 sobre base 14px** (densidad media-alta). Jerarquía por peso
  antes que por tamaño.
- **Roles canónicos** como interfaz de uso (las apps piden `h3`, no "18px semibold").
- Tracking negativo en display/títulos; cifras tabulares en datos.

#### 4.3 Radios, sombras y elevación — dirección

- **Radios ajustados y contenidos:** `5/7/9` para el 90% de la UI (inputs, botones,
  cards, modales), `12/16` solo para superficies grandes, `pill` para chips/badges. La
  familia de radios ecoa el `rx: 9` del tile de marca — coherencia formal entre icono
  y componente. **Nada de esquinas totalmente rectas ni exageradamente redondeadas**;
  el punto medio profesional, un pelín más ajustado (refinado Graphite).
- **Sombras muy sutiles**, escala `xs→xl`, reservadas a lo que flota (ver 3.3). Difusión
  baja, opacidad baja en claro; más contraste y menos blur en oscuro.
- **Elevación semántica**, no decorativa: cada nivel corresponde a un rol
  (base/dropdown/modal/overlay), no a "hacerlo bonito".
- **Foco:** **un anillo** de 2 px del color `--sytue-focus-ring` (el primario), que
  contrasta ≥ 3:1 por su cuenta sobre blanco, lienzo y hundido. **Siempre visible** por
  teclado; jamás `outline: none` sin reemplazo. El foco es un elemento de diseño de
  primera clase, no un residuo.

#### 4.4 Iconografía — dirección

Dos sistemas, mismo ADN geométrico, no mezclar:

- **Iconos de producto** (`branding/icons/*.svg`): el "logo de app". Tile redondeado
  **40×40, rx 9**, glyph blanco sobre el color de producto (`colors.json → product`).
  Trazo blanco **~2.2–3px**, `stroke-linecap="round"`, `stroke-linejoin="round"`.
  Geometría limpia y reconocible a 24px. Viven en el app switcher, avatares de app y
  favicons grandes. Comparten grid y familia con el símbolo de marca (mismo tile, misma
  S-swoosh como raíz formal). Un icono de producto nuevo se dibuja sobre este mismo
  molde: mismo tile, mismo grosor, mismo aire.
- **Iconos de UI** (dentro de los componentes: chevrons, papelera, buscar, etc.):
  **glifos geométricos en duotono con acento**, grid **24×24**. Trazo de **1,5 px en
  pantalla a cualquier tamaño**, esquinas ligeramente redondeadas (linecap/linejoin
  round), y un **relleno de acento** bajo el trazo en las formas cerradas del dibujo;
  los de línea pura (chevrones, aspa, visto) no llevan relleno. **Encuadre
  consistente:** cada glifo centrado en (12,12) con caja óptica uniforme (~2–3u de
  margen), llenando su cuadro por igual — ninguno flota pequeño ni descentrado. El
  trazo hereda `currentColor`, nunca color propio salvo semántica. Estilo Lucide/Feather
  en oficio, pero propios y consistentes con la geometría Sytue. El detalle, en §3.20.

**Reglas de iconografía:** geométrico antes que ilustrativo; un solo grosor por sistema;
`viewBox` coherente; SVG limpios (sin metadatos de editor); optimizados. Un solo estilo
por superficie: el duotono es de TODOS los iconos de interfaz, no de algunos.

#### 4.5 Ilustración — dirección

La ilustración es **escasa y de servicio**, nunca decorativa. Aparece casi solo en
`EmptyState`, onboarding y páginas de error/marca.

- **Estilo:** geométrico y lineal, construido con las mismas primitivas del sistema
  (tiles redondeados, trazos del grosor de icono de producto, la S-swoosh como motivo
  recurrente). Line-art fino sobre superficie neutra, con **un solo toque** de color de
  marca (azul o teal) como foco. Plano — nada de isométrico de stock, sombras largas,
  degradados difusos ni personajes "corporate memphis".
- **Paleta de ilustración:** neutros del sistema + un acento. Funciona en claro y
  oscuro (usa roles, no HEX).
- **Tono:** sereno y útil. Un empty state ilustra el _concepto vacío_ (una bandeja, una
  carpeta), no un chiste. Acompaña siempre a título + descripción + CTA.
- **Cuándo NO ilustrar:** en UI de trabajo densa. Ahí, un icono de sistema y buen
  espaciado bastan. La ilustración que no aporta se elimina.

---

<a id="15-aplicacion-a-la-suite-una-sola-app"></a>

### 1.5 Aplicación a la suite (una sola app)

Mail, Drive, Calendar, Contacts, Docs, Chat/Meet, Cloud, Console y www **se ven y se
comportan como una sola aplicación**: misma shell, mismos componentes de `@sytue/ui`,
misma paleta, misma tipografía, mismos patrones de motion. La única diferenciación
permitida entre apps es el **color de producto en el tile del app-switcher** — nunca una
cabecera de color propio, una estética distinta, un layout distinto ni componentes
propios. Si una app "necesita verse diferente", la respuesta es no.

Superficies de marca (login de Sytue ID, sidebar/rail, www, footer) llevan el cromo
grafito con el acento latón y pueden mostrar la soberanía europea con sobriedad.
Superficies de trabajo son marfil y densas, con el color reservado a la acción.

---

<a id="16-checklist-de-direccion-antes-de-dar-por-bueno-cualquier-diseno"></a>

### 1.6 Checklist de dirección (antes de dar por bueno cualquier diseño)

- [ ] ¿Se lee la jerarquía **en escala de grises**? (si depende del color, está mal)
- [ ] ¿El color de marca ocupa ≤ ~10% y solo marca acción/marca/estado?
- [ ] ¿Todo está en la rejilla de 4px y alineado?
- [ ] ¿La elevación es semántica (solo flota lo que se abre por encima)?
- [ ] ¿Foco visible en todo lo enfocable? ¿Contraste AA verificado?
- [ ] ¿Funciona idéntico en claro y oscuro con solo roles/tokens (cero HEX suelto)?
- [ ] ¿El motion es ≤ 240ms, sin rebotes, y respeta `prefers-reduced-motion`?
- [ ] ¿Se ve como Sytue y como **una sola app**, no como una plantilla ni como IA?
- [ ] Si **configura** algo: ¿está guiado paso a paso, en lenguaje llano, y lo entendería
      alguien sin documentación? (§3.8 · `design.md#2-sistema-de-diseno` §8)
- [ ] ¿Sobra algo? (quítalo) ¿Falta densidad o claridad? (ajústalo)

---

<a id="17-fuentes-de-verdad-y-jerarquia-documental"></a>

### 1.7 Fuentes de verdad y jerarquía documental

| Capa                      | Documento / carpeta                         | Qué fija                                            |
| ------------------------- | ------------------------------------------- | --------------------------------------------------- |
| **Por qué / hacia dónde** | `docs/design.md#1-direccion-de-arte` (este) | Dirección, principios, decisiones marco             |
| **Marca**                 | `branding/brand-guidelines.md`              | Logos, tono, uso de marca, soberanía                |
| **Cómo (reglas)**         | `docs/design.md#2-sistema-de-diseno`        | Layout, componentes, reglas de uso, A11y            |
| **Valores finales**       | `branding/tokens/*`                         | Color, tipografía, spacing, radios, sombras, motion |
| **Assets**                | `branding/logo/*`, `branding/icons/*`       | SVG de logos e iconos                               |
| **Implementación**        | `@sytue/ui`, `@sytue/shell`                 | Componentes Preact + tema Tailwind v4               |

Ante conflicto: la dirección (este doc) fija el criterio; los tokens fijan el número. Si
divergen, se alinean — nunca se resuelve improvisando en una app.

---

<a id="2-sistema-de-diseno"></a>

## 2. Sistema de diseño

> **El puente al código.** Este documento traduce la dirección de arte
> (`docs/design.md#1-direccion-de-arte`) a **specs implementables sin ambigüedad** sobre los
> componentes existentes de `@sytue/ui`. Cada valor mapea a un token de
> `branding/tokens/*` (color, tipografía, spacing/radii/shadows/motion); **nunca**
> se escribe un HEX, px suelto ni duración inventada. Si falta un valor, se añade
> al token — no se improvisa en una app.
>
> **Jerarquía documental:** `design.md#1-direccion-de-arte` fija el _por qué_; **este doc** fija
> el _cómo_ (reglas de uso, specs, layout, A11y); `branding/tokens/*` fija el
> _número_; `@sytue/ui` es la implementación. Ante conflicto: se alinea el token,
> no se parchea la app.
>
> **Alcance del sistema.** Una sola estética, tipografía, paleta y biblioteca de
> componentes para **todas** las superficies (portal/Hub, Mail, Drive, Docs,
> Sheets, Slides, Calendar, Contacts, Chat, Meet, Cloud, Console, www). Las apps
> **consumen** componentes de `@sytue/ui` y la shell de `@sytue/shell`; **no crean
> variantes ni UI ad-hoc**.

---

<a id="20-como-leer-una-spec"></a>

### 2.0 Cómo leer una spec

- **Todos los valores son tokens.** Un valor entre backticks tipo `neutral-150`,
  `radii.md`, `motion.duration.base`, `shadows.md`, `--sytue-surface` referencia una
  entrada de `branding/tokens/*`. Nunca se codifica el literal.
- **Roles antes que HEX.** En componentes se usan **variables de rol**
  (`--sytue-surface`, `--sytue-text`, `--sytue-border`, …) que reasignan claro/oscuro
  automáticamente. Solo cuando el rol no basta se cita una escala (`primary-500`).
- **Dark mode = re-asignación, no inversión.** Como los componentes usan roles, el
  soporte oscuro es automático. Donde un componente necesita un ajuste específico
  (sombras, opacidad de estado, tinte de fondo semántico) se indica en su bloque
  **Dark**.
- **Escala 4px, sin excepciones.** Todo padding/gap/margen es un múltiplo de
  `spacing.*` (`1`=4px, `2`=8px, `3`=12px, `4`=16px, `6`=24px…).
- **Estados obligatorios.** Todo componente interactivo declara:
  `default · hover · active · focus-visible · disabled` y, donde aplique,
  `loading · selected · error · empty`. Ninguno se omite.

---

<a id="21-fundamentos-resumen-de-tokens"></a>

### 2.1 Fundamentos (resumen de tokens)

Valores canónicos en `branding/tokens/`. Aquí, solo la referencia rápida que usan las
specs de abajo.

#### 1.1 Escala tipográfica y roles

`branding/tokens/typography.json`. Base **14px**, ratio ~1.2, familia **Inter**
(`fontFamily.sans`); `JetBrains Mono` (`fontFamily.mono`) solo código/logs/SQL.

| Tamaño | px  | line-height |     | Peso       | valor |
| ------ | --- | ----------- | --- | ---------- | ----- |
| `xs`   | 12  | 16          |     | `regular`  | 400   |
| `sm`   | 13  | 18          |     | `medium`   | 500   |
| `base` | 14  | 22          |     | `semibold` | 600   |
| `md`   | 16  | 24          |     | `bold`     | 700   |
| `lg`   | 18  | 26          |     |            |       |
| `xl`   | 22  | 30          |     |            |       |
| `2xl`  | 28  | 36          |     |            |       |
| `3xl`  | 36  | 42          |     |            |       |
| `4xl`  | 48  | 52          |     |            |       |

**Roles** (interfaz de uso — las apps piden el rol, no el tamaño):

| Rol        | size / weight / tracking         | Uso                                     |
| ---------- | -------------------------------- | --------------------------------------- |
| `display`  | 4xl / 700 / tight                | Portadas www, hero de marca             |
| `h1`       | 3xl / 700 / tight                | Título de página                        |
| `h2`       | 2xl / 700 / tight                | Sección mayor                           |
| `h3`       | xl / 600 / normal                | Sección                                 |
| `h4`       | lg / 600 / normal                | Subsección, título de card              |
| `body`     | base / 400 / normal              | Cuerpo por defecto (14px)               |
| `body-md`  | md / 400 / normal                | Cuerpo de lectura larga (Docs, www)     |
| `label`    | sm / 600 / normal                | Labels de campo, encabezados de columna |
| `caption`  | xs / 400 / normal                | Meta, timestamps, ayuda                 |
| `overline` | xs / 600 / caps (+0.04em, UPPER) | Cabeceras de grupo en sidebar/menús     |
| `button`   | base / 600 / normal              | Texto de botón                          |
| `code`     | sm / 400 / mono                  | Código, tokens, SQL                     |

**Números tabulares** (`font-variant-numeric: tabular-nums`) obligatorios en Table,
listas de datos, métricas de Console/Cloud y columnas de fecha/tamaño.

#### 1.2 Radios (`spacing.json → radii`)

`sm 4` · `md 9` · `lg 14` · `xl 20` · `2xl 24` · `pill 999` · `full 50%`.
Regla de uso: **inputs/botones/ítems de menú → `md`**, salvo chips/avatares circulares
(`pill`/`full`); **cards/paneles/menús → `lg`**; **modales y superficies grandes →
`xl`/`2xl`**; `sm` para lo menudo (etiquetas, el aro de un enlace).

#### 1.3 Elevación (`spacing.json → shadows`) — semántica, no decorativa

| Nivel          | Token claro           | Token oscuro      | Rol                                           |
| -------------- | --------------------- | ----------------- | --------------------------------------------- |
| **0 base**     | `none` + `1px border` | igual             | Filas, inputs, contenido sobre una superficie |
| **1 sutil**    | `shadows.sm`          | `shadows.dark.sm` | Card y panel: la superficie blanca que se     |
|                |                       |                   | despega del lienzo                            |
| **2 dropdown** | `shadows.md`          | `shadows.dark.md` | Dropdown, Select abierto, Popover, Tooltip    |
| **3 modal**    | `shadows.lg`          | `shadows.dark.lg` | Modal/Dialog, Toast                           |
| **4 overlay**  | `shadows.xl`          | `shadows.dark.xl` | Command palette, sheets de pantalla completa  |

**Regla:** dentro de una superficie separa el borde de `1px` (`--sytue-border`), no la
sombra; la sombra `sm` es de la superficie entera (tarjeta, panel), que se despega del
lienzo, y de `md` en adelante, de lo que se abre _por encima_. Las sombras van teñidas con la tinta del texto
(tinta fría casi negra), no con negro; `sm` y `md` llevan dos capas, una corta que
dibuja el canto y otra ancha. En oscuro se usan las variantes `shadows.dark.*` (negro,
más alfa).

#### 1.4 Foco

**Un anillo:** `0 0 0 2px` de `--sytue-focus-ring` —el primario 600 en claro, 700 en
traslúcido y 300 en oscuro—, que cumple el 3:1 de WCAG 1.4.11 por su cuenta. **Siempre
visible por teclado** (`:focus-visible`); jamás `outline: none` sin reemplazo. Sobre un
relleno (botón sólido) se usa `sytue-focus-ring-inset`, que abre 1 px del color de la
superficie antes del anillo; sobre el cromo, `sytue-focus-ring-chrome`, que cambia el
color y no la sombra.

#### 1.5 Motion (`spacing.json → motion`) — tabla canónica

| Interacción                                            | Duración     | Easing     | Propiedades                                     |
| ------------------------------------------------------ | ------------ | ---------- | ----------------------------------------------- |
| Hover de control (button, list-item, tab)              | `fast` 120ms | `standard` | `background-color`, `color`, `border-color`     |
| Active/pressed (feedback)                              | `fast` 120ms | `standard` | `background-color`, `transform` (≤1px)          |
| Foco (aparición del anillo)                            | `fast` 120ms | `standard` | `box-shadow`                                    |
| Cambio de estado en sitio (selected, checkbox, switch) | `base` 150ms | `standard` | `background-color`, `transform`, `border-color` |
| Dropdown / Popover / Select (entrada)                  | `base` 150ms | `standard` | `opacity` 0→1, `transform` translateY(-4px)→0   |
| Dropdown / Popover (salida)                            | `fast` 120ms | `exit`     | `opacity` 1→0                                   |
| Tooltip (entrada, con delay 300ms)                     | `fast` 120ms | `standard` | `opacity`, `transform`                          |
| Modal — overlay (entrada)                              | `base` 150ms | `standard` | `opacity` 0→1                                   |
| Modal — card (entrada)                                 | `slow` 240ms | `standard` | `opacity` 0→1, `transform` scale(0.98)→1        |
| Modal (salida)                                         | `base` 150ms | `exit`     | `opacity`, `transform`                          |
| Toast (entrada)                                        | `base` 150ms | `standard` | `opacity`, `transform` translateX/Y             |
| Toast (salida)                                         | `base` 150ms | `exit`     | `opacity`, `transform`                          |
| Sidebar colapsar/expandir                              | `base` 150ms | `standard` | `width`                                         |
| Tab underline (indicador)                              | `base` 150ms | `standard` | `transform` / `left`+`width`                    |
| Skeleton shimmer                                       | 1200ms loop  | `linear`   | `background-position`                           |
| Spinner                                                | 700ms loop   | `linear`   | `rotate`                                        |

**Prohibido:** bounce, spring exagerado, balanceo, parallax, animación decorativa. Nada

> 240ms en UI (loops de carga aparte). **Respetar siempre `prefers-reduced-motion`:**
> todas las transiciones no esenciales a `0ms`; shimmer/spinner degradan a estado
> estático (opacidad fija) o crossfade mínimo.

#### 1.6 Color y contraste (`colors.json` / `colors.css`) — WCAG AA

La paleta está calibrada para que **todo par texto/fondo canónico cumpla WCAG AA**
(texto normal ≥ 4.5:1; UI y texto grande ≥ 3:1), en claro y oscuro. No se inventa un
HEX en una app: se consume un rol. Dark **no** es el claro invertido — reasigna roles
(ver `colors.json → theme.dark`).

**Distinción clave — `base` vs `-text`.** Un color de marca/semántico saturado que
sirve para **rellenar** (badge, punto de presencia, banner con texto blanco encima) no
siempre pasa como **texto/icono sobre superficie clara**. Por eso cada acento y cada
semántico tiene dos roles:

- **`*base`** — relleno. El texto encima usa `on-*` (blanco o tinta oscura), no el base.
- **`*-text`** (`--sytue-{accent,primary,violet,success,warning,danger,info}-text`) —
  el color **como texto o icono**, calibrado ≥ 4.5:1 sobre superficie clara _y_ sobre su
  propio `*-bg`. En oscuro estos roles apuntan a un escalón claro sobre superficie
  oscura. **Regla:** texto/icono coloreado → usa `-text`; relleno → usa `base`.

Los ratios de los pares canónicos, en las tres apariencias, están en la tabla de §1.0.1
y no se copian aquí: salen de `contraste-paleta.test.ts`, que los mide sobre la hoja y
falla si alguno baja de su mínimo.

**Notas de calibración:**

- El **primario** como relleno con texto blanco es el 600; un escalón más
  claro (el 500) apenas pasa de 3:1 y solo vale para componentes, nunca para
  texto. Como texto sobre blanco, el 700.
- En **oscuro** el relleno primario de petróleo es el 400 (`#3cc4cc`) con rótulo `#04161a`;
  el de cobalto, el 500 (`#4169e8`) con rótulo blanco. Por eso `on-primary` no se usa sobre
  otros rellenos (§1.0.1, regla 1).
- `warning`/`danger` base pasan como **relleno** (texto blanco encima, ≥ 5:1); como
  **texto** usan el rol `-text` (escalón oscuro).
- **Contorno de control** (`border-strong`) sí llega a 3:1 (WCAG 1.4.11). Las líneas
  decorativas (`border`, `border-subtle`) no lo necesitan y deliberadamente no lo
  alcanzan.
- **El color nunca es el único portador de significado:** estado y presencia siempre
  acompañan icono/forma/texto (ver §6).

---

<a id="22-layout-base-appshell"></a>

### 2.2 Layout base (AppShell)

`AppShell` es el marco raíz que compone **TopBar + (App rail?) + Sidebar + panel
secundario? + contenido**. Todas las apps se montan dentro de un `AppShell`; la única
variación permitida es _qué_ zonas se activan y el **color de producto** del contexto.

```
┌──────────────────────────────────────────────────────────────────────┐
│  TopBar — 56px  · logo · buscador · app switcher · notif · presencia · cuenta │
├────┬─────────────┬───────────────────────┬───────────────────────────┤
│ App│  Sidebar    │  Panel secundario     │  Contenido                │
│rail│  260px      │  (bandejas: Mail,     │  (max 1280px · gutter 24) │
│64px│  nav de app │   Chat, Drive detalle)│                           │
│    │  (marca)    │  ~320–360px           │                           │
└────┴─────────────┴───────────────────────┴───────────────────────────┘
```

**Medidas canónicas** (`spacing.json → layout`):
`topbarHeight 56px` · `sidebarWidth 260px` · `sidebarWidthCollapsed 64px` ·
`appRailWidth 64px` · `contentMaxWidth 1280px` · `contentGutter 24px`.

**Zonas y roles de color:**

- **TopBar** (`--sytue-topbar-bg` / `--sytue-topbar-fg`): **cromo grafito**, fija, texto
  blanco a opacidad, borde inferior `1px --sytue-border`, `z-index.topbar` (1040). Sin
  sombra en reposo; `shadows.xs` al aparecer contenido bajo ella (scroll).
- **App rail** (opcional, `appRailWidth 64px`): fondo **grafito** `--sytue-sidebar-bg`,
  iconos de producto (`branding/icons/*.svg`) centrados 40×40 (el tile conserva el color
  de producto), activo con `--sytue-sidebar-active-bg` + acento latón. Aparece cuando la
  app quiere salto rápido entre productos sin abrir el switcher.
- **Sidebar** (`sidebarWidth 260px`, colapsable a 64px): fondo **grafito** (rail)
  `--sytue-sidebar-bg`, texto `--sytue-sidebar-fg` (blanco a opacidad), grupos con
  `overline` en `--sytue-sidebar-fg-muted`. `z-index.sidebar` (1030). Transición de ancho
  `motion.base`.
- **Panel secundario** (bandejas): ~320–360px, superficie `--sytue-surface`, separado del
  contenido por `1px --sytue-border`. Contiene una `List` densa.
- **Contenido**: fondo `--sytue-bg` (marfil), cards/tablas sobre `--sytue-surface`, ancho
  máx. `contentMaxWidth` centrado con `contentGutter`.

**Densidad global:** media-alta. Filas de lista 32–40px, padding interno de card 16px
(`spacing.4`), separación de secciones 24px (`spacing.6`). Base 14px (`body`).

**Responsive (breakpoints de referencia):**
`< 768px` sidebar y panel secundario colapsan a _drawers_ sobre overlay
(`z-index.overlay`); TopBar mantiene logo + buscador + cuenta, el resto va a un menú.
`768–1024px` sidebar colapsada a rail 64px por defecto. `≥ 1280px` layout completo.

---

<a id="23-specs-de-componentes-sytueui"></a>

### 2.3 Specs de componentes (`@sytue/ui`)

Los 20 componentes existentes. Cada spec es **implementable tal cual** sobre el
componente actual.

#### 3.1 AppShell

Layout raíz (§2). No tiene estados interactivos propios; orquesta slots.

- **Slots:** `topbar` (siempre), `rail?`, `sidebar?`, `aside?` (panel secundario),
  `children` (contenido).
- **Estructura:** CSS grid — filas `[topbar 56px][1fr]`; columnas del cuerpo
  `[rail 64px?][sidebar 260/64px?][aside auto?][content 1fr]`.
- **A11y:** TopBar en `role="banner"`; Sidebar/rail en `<nav>`; contenido en `<main>`;
  panel secundario en `<aside>` o `<nav>` según contenga navegación o lista.
- **Dark:** todo por roles; sin ajuste específico.
- **Skip link** "Saltar al contenido" como primer tabbable, visible al foco.

#### 3.2 Sidebar

Navegación de la app actual. Es el **carril**: fondo del color de marca en los tres
temas (§1.0.1, forma «Carril»).

- **Contenedor:** ancho `260px` (colapsado `64px`), fondo `--sytue-sidebar-bg`
  (el color elegido), sin filete a la derecha, padding vertical `spacing.2`, `z-index.sidebar`.
  Lo que vive dentro toma los tokens del carril (`appearance.css`).
- **Grupo:** cabecera con rol `overline` en `--sytue-sidebar-fg-muted`, margen superior
  `spacing.4`. Acciones del grupo (`SidebarSection.actions`) a la derecha de la cabecera
  como botones de icono de 24px (`SidebarAction`: «gestionar carpetas», «nuevo
  cuaderno», «suscribirse»); nunca como un ítem más de la lista. Un grupo puede ser
  plegable (`collapsible`, chevron y `aria-expanded`); los ajustes arrancan plegados.
- **Acción principal en móvil (<768px):** `MobileFab`, botón redondo de 56px primario
  fijado abajo a la derecha (respeta `safe-area-inset-bottom`), fuera del `AppShell`
  porque en móvil el contenido y la lista se turnan. En escritorio no existe: ahí la
  lleva el botón primario de la Sidebar.
- **Ítem:** alto **36px**, radio `md`, padding horizontal `spacing.3`, gap `spacing.2`
  entre icono (20px, UI icon) y label (`body`, `--sytue-sidebar-fg`). Badge de conteo a
  la derecha (`Badge` neutro sobre marca).
- **Estados:**
  - `default`: texto `--sytue-sidebar-fg-muted` (blanco atenuado), fondo transparente.
  - `hover`: fondo `--sytue-sidebar-hover-bg` (velo blanco), texto `--sytue-sidebar-fg`
    (transición `motion.fast`).
  - `active/selected` (`aria-current="page"`): fondo `--sytue-sidebar-active-bg` (velo
    blanco al 14 %), texto `--sytue-sidebar-active-fg` (#fff), peso `medium`. Barra de
    3px a la izquierda en **ámbar** (`--sytue-indicator`, que dentro del carril vale
    `--sytue-sidebar-indicator`).
  - `focus-visible`: anillo blanco (`sytue-focus-ring-chrome`).
  - `disabled`: opacidad 0.5, sin puntero.
- **Colapsado (64px):** solo icono centrado; label pasa a `Tooltip` a la derecha. Toggle
  con transición `width` `motion.base`.
- **Dark:** `--sytue-sidebar-bg` pasa a un verde muy oscuro (`#0e2a1f`); el resto por rol.
- **A11y:** `<nav aria-label>`, lista de enlaces, `aria-current` en activo, navegable con
  flechas arriba/abajo, `Home/End`.

#### 3.3 TopBar

Shell global (`@sytue/shell`), montada en cada app.

- **Contenedor:** alto `56px`, fondo `--sytue-topbar-bg` (claro), borde inferior `1px
--sytue-border`, padding horizontal `spacing.4`, `z-index.topbar`, fija.
- **Zona de marca (`rail`):** en la shell de la suite, la zona del logo se pinta del
  color del carril y, desde 768 px, con el ancho del menú lateral: es la cabecera del
  carril. Dentro valen los tokens del carril; el conmutador que se abre desde ella es
  una superficie y vuelve a los del tema. En <768 px la barra entera es el carril
  (§1.0.1, forma «Carril», punto 2).
- **Contenido (orden de tabulación):** logo de suite (`branding/logo/sytue-one.svg`) →
  buscador global → app switcher (`AppGrid`) → notificaciones → presencia → menú de
  cuenta (`Avatar` + `Dropdown`).
- **Buscador:** input `sm`, ancho flexible (máx ~480px), icono de lupa a la izquierda,
  placeholder `--sytue-text-subtle`; `⌘K`/`Ctrl-K` abre command palette (overlay nivel 4).
- **Iconos de acción:** botones `ghost` `icon` 32px; badge de no-leídos en notificaciones
  (`Badge` `danger` pill, top-right).
- **Estados:** heredados de Button (ghost) y Dropdown.
- **Dark:** `--sytue-topbar-bg` → `#0d1310`; el borde, `chrome-border`.
- **A11y:** `role="banner"`, `aria-label` por acción, orden lógico, buscador con
  `role="searchbox"`.

#### 3.4 AppGrid (App switcher / Launcher)

Grid de productos con iconos de marca.

- **Trigger:** botón `ghost icon` (9 puntos) en TopBar.
- **Panel:** popover elevación **nivel 2** (`shadows.md`), radio `lg`, fondo
  `--sytue-surface`, padding `spacing.4`, ancho para grid de 3–4 columnas.
- **Celda de app:** icono de producto 40×40 (`branding/icons/<app>.svg`, tile `rx 9`
  sobre `product.<app>`), label `caption` debajo, centrado; celda con radio `md`,
  padding `spacing.2`.
- **Estados de celda:** `hover` fondo `--sytue-bg-subtle`; `focus-visible` anillo
  `shadows.focus`; `active` fondo `--sytue-neutral-100` (`neutral-850` dark).
- **Motion:** entrada del panel `motion.base` (opacity + translateY-4px→0), salida
  `motion.fast` easing `exit`.
- **A11y:** `role="menu"`/`grid`; navegación con flechas 2D; `Esc` cierra y devuelve foco
  al trigger; cada celda con el nombre visible como accesible y la descripción en `title`.

#### 3.5 Button

- **Alturas / padding (hit area ≥ 32px):**
  - `sm`: alto 28px, padding-x `spacing.3` (12px), texto `sm/600`.
  - `md` (default): alto 32px, padding-x `spacing.4` (16px), texto `button` (base/600).
  - `lg`: alto 40px, padding-x `spacing.5` (20px), texto `md/600`.
  - `icon`: cuadrado (28/32/40 según tamaño), sin padding-x, icono 20px.
- **Radio:** `md`. **Gap** icono↔texto: `spacing.2`.
- **Variantes × estados** (fondo / texto / borde):

| Variante    | default                                                          | hover                     | active                                   | disabled                  |
| ----------- | ---------------------------------------------------------------- | ------------------------- | ---------------------------------------- | ------------------------- |
| `primary`   | `primary-500` / `on-primary` / —                                 | `primary-600`             | `primary-700`                            | opacidad 0.5, sin puntero |
| `secondary` | `--sytue-surface` / `--sytue-text` / `1px --sytue-border-strong` | fondo `--sytue-bg-subtle` | fondo `neutral-100` (`neutral-800` dark) | opacidad 0.5              |
| `ghost`     | transparente / `--sytue-text` / —                                | fondo `--sytue-bg-subtle` | fondo `neutral-100`                      | opacidad 0.5              |
| `danger`    | `danger` (#b0413a) / #fff / —                                    | `danger-hover`            | oscurece 1 paso                          | opacidad 0.5              |

- **focus-visible:** anillo `shadows.focus` en todas las variantes (además del fondo).
- **loading:** `Spinner` 16px reemplaza el icono líder, texto atenuado, `aria-busy`,
  ancho fijo (no salta), no clicable.
- **Motion:** hover/active `motion.fast`; press aplica `transform: translateY(0.5px)`.
- **Dark:** por roles; `primary` mantiene `primary-500` (contraste AA sobre oscuro OK).
- **A11y:** `<button>` real, `aria-busy` en loading, `aria-label` obligatorio en `icon`,
  nunca solo color para significado (danger acompaña texto/icono claro).

#### 3.6 Card

- **Superficie:** fondo `--sytue-surface` (blanco), borde `1px --sytue-border-subtle`,
  radio `lg` y elevación **nivel 1** (`shadows.sm`): se despega del lienzo. Con
  `raised={false}`, plana.
- **Color de grupo:** no tiñe la tarjeta. Va en `CardIcon`, un icono sobre relleno
  sólido del tono (`tone`); `fill="solid"` rellena la tarjeta entera cuando es un
  recurso destacado.
- **Padding:** `spacing.4` (16px) contenido; cabecera/pie separados por `1px
--sytue-border`, mismo padding.
- **Regiones:** `header` (título `h4` + acciones a la derecha), `body`, `footer`.
- **Interactiva (card como enlace/acción):** añade estados `hover` (borde
  `--sytue-border-strong` + `shadows.xs`), `focus-visible` (anillo), `active`. Cursor
  pointer solo si toda la card es accionable.
- **Dark:** `--sytue-surface` → `neutral-900`; borde `neutral-800`.
- **A11y:** `<section>` con `aria-labelledby` al título cuando agrupa contenido; card
  accionable como `<a>`/`<button>` envolvente, no `div onclick`.

#### 3.7 List

Listas densas (Mail, Chat, Drive, Contacts, notificaciones).

- **Fila:** alto **36px** (compacta) / **48px** (con subtítulo, p.ej. Mail), padding-x
  `spacing.3`, gap `spacing.3`. Estructura: `[icono/avatar 20–32px] [título `body`+
subtítulo`caption` `--sytue-text-muted`] [meta `caption` tabular] [acciones al hover]`.
- **Separación:** `1px --sytue-border` entre filas **o** sin borde con `hover` como única
  separación (elegir uno por vista; Mail usa borde, Chat no).
- **Estados de fila:**
  - `hover`: fondo `--sytue-bg-subtle`; revela acciones rápidas (iconos `ghost`).
  - `active/press`: fondo `neutral-100` (`neutral-850` dark).
  - `selected` (`aria-selected`): fondo `primary-50` (`primary-950` dark) + barra
    izquierda 2px `primary-500`; texto `--sytue-text`.
  - `focus-visible`: anillo `shadows.focus` interior.
  - `unread` (Mail/Chat): título peso `semibold`, punto `primary-500` 6px a la izquierda.
  - `disabled`: opacidad 0.5.
- **empty:** delega en `EmptyState`.
- **loading:** N filas `Skeleton` con la misma altura.
- **Motion:** hover `motion.fast`; selección `motion.base`.
- **A11y:** `role="list"`/`listbox` según selección; navegación flechas + `Home/End`;
  `aria-selected`; acciones al hover también alcanzables por teclado (visibles al foco).

#### 3.8 Table

Tabla densa con orden, selección y paginación.

- **Densidad:** fila **40px** (default) / **32px** (compacta). Header 40px, `sticky`,
  fondo `--sytue-surface`, `shadows.xs` al hacer scroll bajo él.
- **Celda:** padding `spacing.2 spacing.3`, texto `body`, **números tabulares** en
  columnas numéricas/fecha; alineación derecha para números.
- **Header:** `label` (sm/600) `--sytue-text-muted`; columna ordenable muestra chevron
  al hover y estado (`aria-sort`).
- **Estados de fila:** `hover` fondo `--sytue-bg-subtle`; `selected` fondo `primary-50`
  (`primary-950` dark) + checkbox marcado; `focus-visible` anillo en la fila.
- **Zebra:** desactivada por defecto (borde `1px --sytue-border` inferior por fila);
  activable en tablas muy anchas con `--sytue-bg-subtle` en filas impares.
- **empty/loading/error:** `EmptyState` en el cuerpo / filas `Skeleton` / banner de error
  (`Toast` inline `danger` con reintento).
- **Paginación:** pie con `caption` "1–25 de 240" (tabular) + botones `ghost` prev/next +
  selector de tamaño (`Select`).
- **Dark:** header `neutral-900`, bordes `neutral-800`.
- **A11y:** `<table>` semántica, `<th scope>`, `aria-sort`, orden anunciado; selección con
  `aria-selected`/checkbox; caption o `aria-label` de tabla.

#### 3.9 Modal / Dialog

- **Overlay:** `--sytue-overlay` a pantalla completa, `z-index.overlay` (1070).
- **Card (desde 768 px):** centrada —o el panel derecho de `placement="right"`—, fondo
  `--sytue-panel-surface` (opaco en los tres modos: en «Traslúcido» es `surface-raised`,
  como los menús, porque un diálogo semitransparente no se leía), radio de tarjeta de la forma (`rounded-card`: 14 en «Carril»,
  16 en «Marco»; antes 20, la superficie más redonda de la suite), `shadow-xl`,
  `z-index.modal` (1080). Anchos: `sm 400px` · `md 560px` · `lg 720px` · `xl 1120px`; alto
  máx `90 %` del viewport visual, ajustado al contenido, con el cuerpo desplazable.
- **Hoja (por debajo de 768 px), la de TODOS los diálogos:** a todo el ancho, apoyada en el
  borde inferior del viewport visual (encima del teclado), esquinas de arriba con el radio
  de tarjeta, asa de 36×4 px que baja la hoja con el dedo (más de 80 px cierra; solo si el
  diálogo es `dismissable`), zona segura de abajo en la última banda y aire arriba
  (`max(2,5rem, zona segura + 1rem)`). Cabecera y pie fijos y el cuerpo se desplaza; solo
  si ni así cabe un cuerpo de 2,75rem se desplaza el panel entero.
- **Regiones:** header (título `sytue-h4` + aspa `ghost icon`), body y footer (acciones a
  la derecha, gap `spacing.2`; primaria a la derecha del todo), todos a `px-6`. El mismo
  aire, 21 px, arriba, entre título y cuerpo, entre cuerpo y acciones y abajo (cabecera
  `pt-5 pb-2`, cuerpo `py-3`, pie `pt-2 pb-5`, más el filete de 1 px de cada banda).
  **Sin filetes** entre bandas salvo cuando el cuerpo se desplaza: entonces aparecen los
  dos (`border-subtle`) para decir «hay más».
- **Foco con el dedo:** abrir no enfoca el primer campo (sacaría el teclado sobre la hoja
  en Android, y en iOS no): el foco va al panel y el teclado sale al tocar el campo.
- **Motion:** overlay entra `motion.base` (opacity); card entra 180 ms (opacity + scale
  0.98→1 desde 10 px); la hoja sube desde abajo en 220 ms; sin animación de salida.
- **Estados:** `loading` (footer con botón primario en loading + `aria-busy` en dialog);
  `error` (banner inline `danger` sobre el body).
- **Dark:** `shadows.dark.lg`; overlay `rgba(0,0,0,0.6)`.
- **A11y:** `role="dialog"` `aria-modal="true"`, `aria-labelledby` al título, **focus
  trap**, `Esc` cierra, foco inicial al primer control (o al título), **retorno de foco**
  al disparador al cerrar. Clic en overlay cierra solo si no es destructivo.

#### 3.10 Dropdown / Menu

Menús de acción y de cuenta.

- **Trigger:** botón/avatar; refleja `aria-expanded`.
- **Panel:** popover **nivel 2** (`shadows.md`), radio `lg`, fondo `--sytue-surface`,
  borde `1px --sytue-border`, padding `spacing.1`, ancho mín 180px, `z-index.popover`
  (1060). Se posiciona anclado al trigger (flip si no cabe) y **acotado a la ventana**:
  `max-width: 100vw - 16px` más un empujón horizontal (`useFitInViewport`) para que no
  arranque en coordenada negativa. El flip no basta: con el trigger lejos de los dos
  bordes, un panel ancho anclado a la derecha empieza fuera de pantalla por la
  **izquierda**, y eso no genera scroll —se recorta y no hay forma de alcanzarlo—.
  En vertical **no tapa su disparador**: se queda en el hueco de un lado —abajo, salvo
  que no quepa y arriba haya más sitio— y lo que no entra se desplaza por dentro del
  panel (`encajarPanel`). Un menú largo, como el de cuenta, se plantaba encima de su
  propio botón en cuanto medía más que ese hueco.
  Y cuando queda contenido por debajo de lo que se ve, **lo enseña**: el borde inferior
  se funde con la superficie y lleva una flecha, hasta que se llega al final. La barra
  de desplazamiento no basta, porque en macOS y en el móvil no se pinta hasta que se
  mueve.
- **Ítem:** alto **32px**, radio `md`, padding-x `spacing.3`, gap `spacing.2` (icono 16px
  - label `body`); atajo de teclado a la derecha en `caption` `--sytue-text-subtle`.
- **Estados de ítem:** `hover`/`focus` (unificados) fondo `--sytue-bg-subtle`; `active`
  `neutral-100`; `disabled` opacidad 0.5; ítem `danger` texto `danger`, hover fondo
  `--sytue-danger-bg`.
- **Separador:** `1px --sytue-border`, margen `spacing.1`. Cabecera de grupo `overline`.
- **Motion:** entrada `motion.base` (opacity + translateY-4px→0), salida `motion.fast`
  easing `exit`.
- **A11y:** `role="menu"`/`menuitem`, navegación flechas, `Home/End`, tipeo para saltar,
  `Esc` cierra y retorna foco, `aria-expanded` en el trigger.

#### 3.11 Toast

Notificación efímera.

- **Contenedor:** apila abajo-derecha (config.), gap `spacing.2`, `z-index.toast` (1090).
- **Toast:** fondo `--sytue-surface`, borde `1px --sytue-border`, radio `lg`, elevación
  **nivel 3** (`shadows.lg`), padding `spacing.3 spacing.4`, ancho 320–400px. Barra/icono
  de 2px a la izquierda según variante.
- **Variantes** (icono obligatorio + color, **nunca solo color**):
  `success` (icono check, `success`) · `info` (`info`) · `warning` (`warning`) ·
  `danger` (`danger`). Fondo se mantiene neutro (`--sytue-surface`); el color va en
  icono + barra lateral.
- **Estructura:** icono 20px + título `label` + descripción `caption` `--sytue-text-muted`
  - acción opcional (`Button ghost sm`) + cerrar `ghost icon`.
- **Comportamiento:** auto-dismiss 5s (pausable al hover/foco); `danger` no auto-dismiss.
- **Motion:** entrada `motion.base` (translateX/Y + opacity), salida `motion.base` easing
  `exit`.
- **A11y:** `role="status"` (info/success) o `role="alert"` (warning/danger),
  `aria-live` correspondiente; no atrapa foco; cerrable por teclado.

#### 3.12 Tooltip

- **Panel:** fondo `neutral-900` (`neutral-0`/`neutral-800` en dark → contraste),
  texto `--sytue-neutral-0` `caption`, radio `md`, padding `spacing.1 spacing.2`,
  elevación **nivel 2** (`shadows.md`), `z-index.tooltip` (1100), máx ~240px.
- **Comportamiento:** aparece tras **300ms** de hover/foco; desaparece inmediato al salir.
  Flecha opcional 6px.
- **Motion:** entrada `motion.fast` (opacity + translate 4px), sin salida animada larga.
- **A11y:** `role="tooltip"`, ligado por `aria-describedby`; accesible por foco de teclado;
  **nunca** contenido esencial solo en tooltip; no interactivo dentro.

#### 3.13 Tabs

Navegación segmentada dentro de una vista.

- **Tablist:** fila con borde inferior `1px --sytue-border`. Tab: padding
  `spacing.2 spacing.3`, texto `label` (sm/600), alto 36px, gap `spacing.1`.
- **Estados:**
  - `default`: texto `--sytue-text-muted`.
  - `hover`: texto `--sytue-text`, fondo `--sytue-bg-subtle`.
  - `active/selected` (`aria-selected`): texto `--sytue-text`, **indicador** 2px
    `primary-500` en el borde inferior alineado al tab.
  - `focus-visible`: anillo `shadows.focus`.
  - `disabled`: opacidad 0.5.
- **Motion:** indicador se desliza entre tabs `motion.base` (`transform`/`left+width`).
- **Variante segmented** (pill): contenedor `--sytue-bg-subtle` radio `md`; tab activo
  `--sytue-surface` + `shadows.xs`.
- **A11y:** `role="tablist"/"tab"/"tabpanel"`, flechas para moverse, `aria-selected`,
  `aria-controls`, activación con Enter/Espacio (manual) o al foco (automática).

#### 3.14 Form (Input, Textarea, Select, Checkbox, Radio, Switch, FieldGroup)

**Reglas comunes de campo**

- **Label:** rol `label` (sm/600) `--sytue-text`, encima del control, gap `spacing.1`.
- **Ayuda:** `caption` `--sytue-text-muted` bajo el control.
- **Error:** `caption` `--sytue-danger` + icono, `aria-invalid`, `aria-describedby` apunta
  al mensaje. **Nunca solo color:** icono + texto siempre.
- **Requerido:** marca textual/`*`, `aria-required`.

**Input / Textarea**

- Alto `md` **36px** (Textarea min 72px, redimensionable vertical), radio `md`, borde
  `1px --sytue-border-strong`, fondo `--sytue-surface`, padding-x `spacing.3`, texto
  `body`, placeholder `--sytue-text-subtle`.
- **Estados:** `hover` borde `--sytue-neutral-300`; `focus-visible` borde `primary-500` +
  anillo `shadows.focus`; `disabled` fondo `--sytue-bg-subtle`, texto `--sytue-text-subtle`,
  sin puntero; `error` borde `danger` + anillo `rgba(danger)`; `readonly` sin borde de foco
  fuerte.
- Icono/afijo opcional dentro (`--sytue-text-subtle`, 16px), padding ajustado.

**Select**

- Igual caja que Input; chevron 16px a la derecha. Al abrir → panel tipo Dropdown
  (**nivel 2**), ítems 32px, ítem elegido con check + `selected` (`primary-50`).
- Estados heredan de Input (caja) + Dropdown (panel).

**Checkbox / Radio**

- Caja **16px** (checkbox radio `sm`; radio circular `full`), borde `1px
--sytue-border-strong`.
- `checked`: fondo `primary-500`, glyph #fff (check / punto). `hover` borde
  `--sytue-neutral-300`. `focus-visible` anillo. `disabled` opacidad 0.5. `error` borde
  `danger`. `indeterminate` (checkbox) barra central.
- Motion: check aparece `motion.base`.

**Switch**

- Pista **36×20px** radio `pill`; pomo 16px. `off` pista `--sytue-neutral-300`;
  `on` pista `primary-500`, pomo se desliza `motion.base`. `focus-visible` anillo;
  `disabled` opacidad 0.5. `role="switch"` `aria-checked`.

**FieldGroup / Form**

- Separación vertical entre campos `spacing.4`; secciones `spacing.6`. Botonera al pie
  alineada a la derecha (primaria + secundaria `ghost`).
- **Dark:** cajas `--sytue-surface` → `neutral-900`, borde `neutral-700` (strong).
- **A11y:** todo control con `<label for>` asociado; grupos radio en `<fieldset><legend>`;
  errores enlazados por `aria-describedby`; foco visible siempre.

#### 3.15 Avatar

- **Tamaños:** `xs 20` · `sm 24` · `md 32` (default) · `lg 40` · `xl 64`. Radio `full`
  (usuarios) o `md` (grupos/entidades).
- **Contenido:** imagen (`object-cover`) o iniciales (1–2) sobre fondo derivado
  determinista del nombre (**paleta terrosa** `--sytue-avatar-1..6`: teal/plum/slate/clay/
  moss/indigo, blanco encima ≥ 6:1), texto `label`/peso 600 en `--sytue-neutral-0`.
- **Presencia:** punto de `max(10, round(lado × 0,34))` px abajo-derecha (a 8 px el disco
  se quedaba en 4 y la forma no se veía), con el anillo
  `2px --sytue-surface` DENTRO de esa medida, color de `presence.*` (online/away/dnd/offline).
  Nunca solo color: cada estado lleva además su FORMA en `surface` —online el disco lleno,
  away una media luna, dnd una raya horizontal, offline un aro hueco— y su
  `aria-label`/title traducido (`presence.*` del catálogo común). Las fracciones son las
  mismas en la web (`FORMA` de Avatar.tsx) y en Fyne (`ui.FormaPresencia`).
- **Grupo/stack:** superposición -8px, borde `2px --sytue-surface`; "+N" como último.
- **A11y:** `<img alt>` con nombre, o `aria-label` si iniciales; presencia con texto.

#### 3.16 Badge / Tag

- **Badge (conteo/estado):** alto 20px, radio `pill`, padding-x `spacing.2`, texto
  `caption`/600, **números tabulares**. Neutro (`--sytue-bg-subtle` + `--sytue-text-muted`)
  por defecto; variantes semánticas usan `*-bg` + `*` (p.ej. `--sytue-success-bg` +
  `success`), acento `primary`/`accent` para marca. Dot-badge 6–8px para no-leídos.
- **Tag (etiqueta):** radio `md`, mismo alto, opcional con punto de color a la izquierda y
  botón cerrar (`ghost icon` 14px) si es removible.
- **Estados (tag removible):** `hover` en el cerrar; `focus-visible` anillo.
- **Dark:** fondos semánticos usan `*-bg` de tema oscuro (definidos en `colors.css`).
- **A11y:** contraste AA; significado por texto + color, no solo color; conteo con
  `aria-label` legible ("3 no leídos").

#### 3.17 EmptyState

- **Composición:** ilustración/marca (line-art del sistema, §art-direction 4.5) 96–120px
  → título `h4` → descripción `body` `--sytue-text-muted` → CTA (`Button primary`).
  Centrado, `max-width` ~360px, gap vertical `spacing.3`, padding `spacing.8`.
- **Variantes:** vacío inicial (con CTA de creación), sin resultados de búsqueda (sin
  ilustración pesada, texto + acción de limpiar filtro), error (icono `danger` + reintento).
- **Dark:** ilustración por roles (funciona en ambos temas).
- **A11y:** título en heading real; CTA como botón/enlace; no depende de la imagen para el
  mensaje.

#### 3.18 Skeleton

- **Bloque:** fondo `--sytue-bg-subtle`, radio del elemento que sustituye (`md` texto,
  `full` avatar, `lg` card). Líneas de texto: alto 12–14px, anchos variados (100/80/60%).
- **Shimmer:** gradiente sutil `--sytue-neutral-100`→`--sytue-bg-subtle` desplazándose,
  loop 1200ms `linear`. Respeta `prefers-reduced-motion` → estático.
- **Uso:** replica la geometría real (misma altura de fila/card) para evitar salto de
  layout al cargar.
- **A11y:** contenedor `aria-busy="true"`; skeletons `aria-hidden="true"`; anunciar
  "Cargando" con `aria-live` una vez, no por bloque.

#### 3.19 Spinner

- **Tamaños:** `sm 16` · `md 20` · `lg 24`. Trazo 2px, arco 270° en `currentColor` (hereda:
  `primary` en botón primario, `--sytue-text-muted` en contenido). Loop 700ms `linear`.
- **Uso:** inline en Button loading, en overlays de carga puntual. Para carga de listas/
  tablas preferir `Skeleton`.
- **A11y:** `role="status"` + `aria-label="Cargando"`; `prefers-reduced-motion` → punto
  pulsante suave o estático.

#### 3.20 Icon

Iconos de UI (glifos de interfaz, no los de producto).

- **Grid:** 24×24 (`viewBox`), render habitual **20px** (16px en denso, 24px en TopBar/
  vacíos). `stroke-linecap/linejoin="round"`, `stroke="currentColor"` (hereda color del
  contexto; nunca color propio salvo semántica).
- **Encuadre:** cada glifo centrado en (12,12) con caja óptica uniforme (~2–3u de margen);
  ninguno flota pequeño ni descentrado (los chevrons llenan su cuadro como el resto).
- **Estilo: duotono con acento.** Lo pinta UN componente, `Glifo` de `@sytue/ui`
  (`packages/ui/src/components/Glifo.tsx`); `Icon` y los catálogos propios de cada app
  solo guardan el dibujo. Tres reglas:
  1. **Trazo de 1,5 px en pantalla a cualquier tamaño**: `stroke-width = 1,5 × 24 / px`.
     Con el `stroke-width={2}` fijo de antes el mismo glifo pesaba 1,3 px a 16 y 2,7 px
     a 32.
  2. **Relleno de acento bajo el trazo**, solo en los subtrazos cerrados (los que acaban
     en `Z`) y las figuras cerradas (`rect`, `circle`, `polygon`), al 34 % y en par-impar:
     una forma dentro de otra queda como hueco (la pupila del ojo). Con `fill` propio
     —la estrella o el corazón marcados— la forma va llena y no lleva capa.
  3. **El color del relleno es `--sytue-glyph-fill`**, con caída a `--sytue-primary`: el
     color de marca sobre el contenido. Dentro del carril de color lo fija la regla del
     carril (ámbar). Sobre un relleno de color —botón principal, secundario y de peligro,
     insignias y tarjetas sólidas, teselas de producto— vale `currentColor`: lo decide
     una regla de `packages/ui/src/styles.css` que reconoce la tinta de quien escribe
     encima de un color (`text-white`, `text-[var(--sytue-on-primary)]`,
     `text-on-card-solid-*`).
- **Reglas:** un solo grosor por sistema; alineación óptica al centro de línea de texto;
  SVG limpio (sin metadatos). Ningún `<svg>` de icono escrito a mano fuera de `Glifo`:
  lo vigila `packages/ui/src/components/glifos-al-comun.test.ts`, con sus excepciones
  comparadas por igualdad. Los lienzos de contenido (gráficos, diapositivas,
  anotaciones) no son iconos y no llevan este estilo.
- **Nativas (Fyne):** el mismo grosor y la misma capa, pintados por `apps/fyne/iconos`
  con los números de `Glifo.tsx` (un test los lee de ahí). El relleno es el primario del
  tema; sobre un relleno de color es la tinta, reconocida igual que en la web (tinta
  blanca o `on-primary`), y quien necesite otro —el carril— lo fija con `Dibujo.Acento`.
- **Producto vs UI:** iconos de producto (`branding/icons/*.svg`, tile 40×40 `rx 9`) solo
  en AppGrid, rail, avatares de app y favicons — **no** dentro de controles.
- **A11y:** decorativo → `aria-hidden="true"`; con significado propio → `role="img"` +
  `aria-label`.

---

<a id="24-guia-de-layout-por-producto"></a>

### 2.4 Guía de layout por producto

Todos comparten AppShell, componentes y paleta. Diferencia permitida: **color de producto**
(`colors.json → product`) **solo en el tile del icono del rail/switcher** (orientación entre
apps) — el cromo de cada app es grafito monocromo; **nunca** una cabecera de color, estética
o layout distinto.

| Producto         | Tile               | Zonas activas                                                                                  | Contenido principal                                                                                                                                                                          |
| ---------------- | ------------------ | ---------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Hub / Portal** | `hub` #0f766e      | TopBar + Sidebar                                                                               | AppGrid grande + tarjetas de resumen (correo no leído, próximos eventos, archivos recientes) en `Card`s                                                                                      |
| **Mail**         | `mail` #be123c     | TopBar + Sidebar (carpetas) + **panel secundario** (lista) + contenido                         | Panel = `List` de conversaciones (fila 48px, unread bold + dot); contenido = hilo (mensajes en `Card`s), redactar en `Modal`/vista lateral                                                   |
| **Drive**        | `drive` #146a47    | TopBar + Sidebar (Mi unidad, Compartido, Papelera) + contenido                                 | Toggle `List`/grid de tarjetas; breadcrumb (`Tabs`/nav); acciones en `Dropdown` contextual; `Table` en vista lista (nombre/propietario/modificado/tamaño, tabular)                           |
| **Docs**         | `docs` #1c58d9     | TopBar + (Sidebar de esquema opcional) + contenido de lectura (`body-md`)                      | Editor a ancho de lectura (~720px), `Toolbar` de formato (botones `ghost`), comentarios en panel secundario derecho                                                                          |
| **Sheets**       | `sheets` #0f7052   | TopBar + contenido (grid denso)                                                                | Rejilla de celdas (números tabulares); `Toolbar`; el grid es una superficie densa propia sobre tokens, no una `Table` estándar                                                               |
| **Slides**       | `slides` #94590f   | TopBar + panel secundario (tiras de diapositivas) + lienzo                                     | Panel = `List` vertical de miniaturas; lienzo central                                                                                                                                        |
| **Calendar**     | `calendar` #7e22ce | TopBar + Sidebar (mini-mes, calendarios) + contenido                                           | Rejilla de semana/mes; eventos como bloques color-codificados por calendario (`violet` y product colors, texto AA); `Tabs` día/semana/mes                                                    |
| **Contacts**     | `contacts` #0e7490 | TopBar + Sidebar (grupos) + **panel secundario** (lista) + detalle                             | Panel = `List` A–Z con `Avatar`; detalle = `Card`s de campos                                                                                                                                 |
| **Rooms**        | `rooms` #4c1d95    | TopBar + Sidebar (canales/DMs con presencia) + contenido; la llamada toma el área de contenido | Timeline de mensajes (densa, sin borde entre filas, agrupada por autor); composer abajo; `Avatar` con presencia. En llamada: rejilla de vídeo, controles abajo y el canal como panel lateral |
| **Cloud (PaaS)** | `cloud` #2464c9    | TopBar + Sidebar (proyectos/recursos) + contenido                                              | `Table`s de recursos (estado con `Badge` semántico), logs en `code`/mono, formularios de despliegue                                                                                          |
| **Console**      | `console` #3a4a5e  | TopBar + Sidebar (admin: usuarios, dominios, facturación) + contenido                          | `Table`s densas, `Form`s de configuración, métricas (números tabulares), `Modal`s de confirmación destructiva                                                                                |
| **www (marca)**  | `one` #c2410c      | Header público + contenido + footer                                                            | Superficie de **marca**: `display`/`h1`, cromo grafito con acento latón, soberanía europea sobria; usa los mismos tokens y tipografía, densidad menor (marketing)                            |

**Constante en todos:** TopBar 56px (cromo grafito) con app switcher; sidebar/rail grafito;
contenido marfil denso; color de producto solo en el tile del switcher; acento latón; claro
y oscuro; motion de la §1.5.

---

<a id="25-patrones-de-composicion"></a>

### 2.5 Patrones de composición

#### 5.1 Listas

Densas por defecto (§3.7). Elegir borde-entre-filas (Mail, listas de datos) **o**
hover-como-separador (Chat) por vista, no ambos. Acciones rápidas al hover **y** al foco.
Estado vacío → `EmptyState`; carga → `Skeleton` con la misma altura de fila.

#### 5.2 Tablas

Para datos estructurados con orden/selección/paginación (Drive lista, Cloud, Console).
Header sticky, números tabulares, alineación derecha en numéricos. Evitar zebra salvo
tablas muy anchas. Selección múltiple con checkbox + barra de acciones contextual que
aparece al seleccionar (`motion.base`).

#### 5.3 Formularios

Label encima, ayuda debajo, error con icono + texto (nunca solo color). Un campo por fila
salvo pares lógicos (ciudad/CP). Botonera al pie alineada a la derecha: primaria a la
derecha, cancelar `ghost` a su izquierda. Validación al `blur`/submit, no por cada tecla.
Estados de campo completos (§3.14).

> **Configuración ≠ formulario.** Si el formulario **configura** algo (una integración, una
> política, un proveedor, un ajuste de organización), no se resuelve con esta sección: va con
> las primitivas de la **§8** (`Wizard` / `SettingsShell` + `FieldHint`). Esta sección cubre el
> formulario de datos corriente (crear un contacto, editar un perfil).

#### 5.4 Modales

Solo para tareas cortas y confirmaciones. Tamaño mínimo que quepa (`sm/md/lg`). Título
`h3` que nombra la acción, no "¿Estás seguro?". Confirmación destructiva → botón `danger` a
la derecha, describe la consecuencia. Focus trap + retorno de foco (§3.9). Tareas largas →
página/panel, no modal.

#### 5.5 Toasts

Feedback de acción completada o error de sistema, no para validación de formulario (esa va
inline). `success`/`info` auto-dismiss 5s; `warning`/`danger` persisten hasta cerrar.
Icono + texto siempre. Máx ~3 apilados; el resto en cola.

#### 5.6 Navegación

Jerarquía: **TopBar** (global: cambiar de app, buscar, cuenta) → **Sidebar** (secciones de
la app actual) → **Tabs** (vistas dentro de una sección) → **breadcrumb** (profundidad en
Drive/Console). `aria-current` en el nivel activo de cada uno. Nunca duplicar la misma
navegación en dos zonas.

#### 5.7 Elevación en la práctica

Reposo = plano + borde. Solo se eleva lo efímero que se abre por encima (dropdown/modal/
toast/tooltip), con el nivel de la §1.3. Si dudas entre sombra o borde para separar dos
zonas fijas → borde.

<a id="58-fotos-y-mandos-sobre-foto"></a>

#### 5.8 Fotos: filas justificadas y mandos sobre foto

Una rejilla de fotos (Sytue Photos) va en **filas justificadas**: cada foto conserva su
proporción, las filas llenan el ancho y la última no se estira. Es CSS —la clase `.filas`
de `apps/photos/src/styles/app.css`, con la proporción de cada tesela en `--r`—, sin
maquetación en JavaScript. Solo se recortan las proporciones extremas, acotadas a
9:20–5:2 (`lib/filas.ts`). La fila mide como mínimo 9, 11 y 13 rem según el corte, y en
un teléfono lo que deja caber dos 16:9 a la par.

Lo que va **sobre una foto** no usa tokens de superficie: la foto no cambia con el tema
y `surface` es translúcida en el traslúcido. Es negro al 60 % con la tinta en blanco
—5,7:1 sobre una foto blanca, el peor caso—: `IconButton tone="media"` para un mando
(las flechas del visor) y el mismo par para una marca (el corazón de una tesela). Una
etiqueta que deba leerse como parte del cromo usa `bg-surface-raised`, que es opaca en
los tres temas.

Jerarquía de una barra de acciones: **una** acción rellena por pantalla; las demás de
`ghost` con su icono; lo destructivo aparte, tras un filete, como `IconButton
tone="danger"`. Los mandos de una misma barra van en una sola talla.

---

<a id="26-accesibilidad-wcag-aa--checklist-de-merge"></a>

### 2.6 Accesibilidad (WCAG AA) — checklist de merge

- **Contraste:** texto ≥ 4.5:1, UI/texto grande ≥ 3:1. Paleta calibrada en claro y oscuro
  (ratios verificados en §1.6); usar el rol `-text` para texto/icono coloreado, `base` solo
  para relleno. Cualquier par nuevo se valida antes de merge.
- **Foco visible:** `shadows.focus` en todo enfocable; nunca `outline:none` sin reemplazo.
- **Teclado:** toda interacción alcanzable; orden lógico; `Esc` cierra overlays; focus trap
  en modales; retorno de foco al cerrar; flechas en list/menu/tabs/grid.
- **Semántica/ARIA:** landmarks (`banner`/`nav`/`main`/`aside`), roles correctos,
  `aria-current/selected/expanded/invalid/live/sort/busy`.
- **No solo color:** estado, error y presencia llevan icono/texto además de color.
- **Movimiento:** `prefers-reduced-motion` respetado en toda transición no esencial.
- **Táctil:** objetivos ≥ 32px (ideal 44px en móvil).
- **Idioma:** `lang="es"` en producto.

---

<a id="261-auditoria-de-accesibilidad-de-office"></a>

#### 2.6.1 Auditoría de accesibilidad de Office (Docus, Calcus, Diapos y CertiPDF)

Medida en un navegador, no declarada. `test/e2e/accesibilidad-office.mjs` abre los bancos de
pruebas `/dev/editor` y `/dev/pdf` de `apps/docs` (solo en desarrollo; la API se contesta con
`page.route`, sin sesión ni servicio) y comprueba, por producto, a 1280, 390 y 320 px, en claro
y en oscuro:

- **axe-core** (WCAG 2.0/2.1/2.2 A y AA) sobre la página y con cada pestaña de la cinta abierta.
  `serious` y `critical` FALLAN; lo demás se lista como menor y no bloquea.
- **Zoom al 200 %** (ventana de 640 px): el documento no desborda y la cinta sigue entera.
- **Teclado** (1280 px): Tab llega a la cinta, las flechas mueven pestañas y banda, F6 salta
  entre cinta y lienzo, Escape cierra un popover y devuelve el foco, y todo lo enfocable
  muestra foco visible.
- **Táctil** (390 y 320 px, `pointer: coarse`): todo control de cabecera y cinta mide al menos
  44 × 44 px.
- **Lector de pantalla**: `tablist`/`tabpanel` con nombre, regiones sin nombre, controles sin
  nombre accesible, idioma, y que un fallo de guardado (POST `/api/ops` → 500) se anuncia en
  una región viva.

Lo lanza `test/e2e/correr-accesibilidad.sh` (también `cinta.mjs` y `paridad-cinta-web.mjs`) y
el job `accesibilidad-office` de `ci-frontend.yml`: al publicar una release, por programación
y a mano, como el resto del flujo (no corre en push ni en PR).

**Las dos reglas que salieron de medir.**

1. **Suelo táctil por zona, no clase a clase.** `Button` y `BOTON_DE_BARRA` ya traían
   `pointer-coarse:`, pero la cinta pinta decenas de botones y campos con cadenas de clases
   copiadas (`h-7 w-7`, `h-6`) y cada copia nueva nacía de 25 px. `data-suelo-tactil` en la raíz
   de una zona (`@sytue/ui`, `styles.css`) pone el mínimo de 44 px a todo lo que lleve dentro con
   puntero táctil; `data-sin-tactil` deja fuera lo que el lienzo dibuja con sus controles
   (rejilla, escenario, folio). Las franjas de altura fija suben con sus variables
   (`--editor-tabs-h`, `--editor-status-h`, `--editor-rail-w`). Criterio: `pointer: coarse`,
   no el ancho de la ventana.
2. **Un `tablist` se cumple, no se escribe.** `useTablist` (`@sytue/ui`) da el roving tabindex,
   las flechas y el enlace pestaña-panel. Las tres tiras de CertiPDF (barra, panel lateral y
   diálogo de firma) llevaban `role="tab"` sin nada más.

**Excepción documentada.** Los tres marcadores de sangría de la regla de Docus (14 × 9 px) no
pasan el 24 px de 2.5.8, y no pueden crecer sin pisarse (la regla mide 20 px de alto). Es un atajo
de puntero: las tres sangrías se fijan también con campos numéricos del panel de ajustes. Es la
excepción «Equivalente» de 2.5.8, y la auditoría la aplica solo mientras ese equivalente exista.

**Pendiente (menor o de otra pieza).**

- El Administrador de nombres no existe en Calcus nativo (ver la paridad de comandos).
- Fyne: la cinta nativa se mide en tamaño (`LadoBotonCinta`, 24,5 px) con el puntero fino y su
  accesibilidad depende del puente del driver (`AccessibilityLabel`/`AccessibilityRole`, que ya
  llevan todos los controles de la cinta); falta verlo con VoiceOver y Narrador en una ventana
  real, y el suelo táctil de 44 px en Android e iOS.
- La cinta del editor (`Ribbon.tsx`) y `Tabs` de `@sytue/ui` llevan su propia copia del manejo de
  flechas; migrarlas a `useTablist` es mecánico y queda para cuando la cinta no esté en obras
  de estética.

<a id="27-reglas-de-uso-obligatorias"></a>

### 2.7 Reglas de uso (obligatorias)

1. **Una sola fuente.** Componentes solo desde `@sytue/ui`; shell solo desde
   `@sytue/shell`. Prohibido reimplementar cualquiera de los 20 componentes en una app.
2. **Sin UI ad-hoc.** Falta un componente/variante → se añade a `@sytue/ui` (PR al design
   system), no en la app.
3. **Solo tokens.** Variables de rol/tokens de `branding/tokens/`; nunca HEX, px sueltos ni
   fuentes locales. Los valores de esta guía **son** tokens.
4. **Shell en cada superficie.** Toda app monta `@sytue/shell` (TopBar + app switcher +
   búsqueda + notificaciones + presencia + cuenta).
5. **Marca desde `branding/`.** Logos e iconos se consumen de `branding/logo/` y
   `branding/icons/`; no se rediseñan por app.
6. **100% Sytue-native.** Sin motores de terceros que tematizar. Todas las superficies son
   obra original de Sytue con marca directa de `@sytue/ui` + `branding/`.
7. **Claro y oscuro.** Todo componente y vista funcionan en ambos temas vía roles.
8. **AA siempre.** Ningún merge baja del estándar de accesibilidad.
9. **Densidad de marca.** Base 14px, filas 32–40px, rejilla 4px. Densidad media-alta es
   parte de la identidad, no un ajuste opcional.
10. **Configuración guiada.** Toda superficie de configuración —de cualquier producto, de la
    organización o de la cuenta— usa las primitivas de la §8. Ningún «muro de formulario»,
    ningún ajuste que exija leer documentación para entenderlo.

---

<a id="28-configuracion-guiada-obligatoria-en-toda-la-suite"></a>

### 2.8 Configuración guiada (obligatoria en toda la suite)

#### 8.1 La directiva

> **Toda funcionalidad de Sytue —nueva y existente— se configura con una UI intuitiva de
> estilo Apple + Vercel + DigitalOcean: limpia, guiada paso a paso, en lenguaje llano, para
> que un usuario básico la configure sin documentación. NUNCA la complejidad densa de
> Google Cloud/AWS ni un muro de formulario.**

No es una preferencia estética: es un requisito de producto **permanente** y aplica a
**todas** las superficies de configuración de la suite —Console, ajustes de cada producto,
ajustes de organización, ajustes de usuario—, no solo a las de interoperabilidad. Una
superficie de configuración nueva que no lo cumpla es una regresión, aunque sea funcional.

Qué significa en concreto:

| Sí                                                           | No                                          |
| ------------------------------------------------------------ | ------------------------------------------- |
| Una idea por paso, con su porqué                             | 20 campos en una pantalla                   |
| Lenguaje llano: «¿A quién afecta esta regla?»                | Jerga de implementación: `include_groups[]` |
| Decir **dónde encontrar** cada dato en el producto de origen | Un `label` y a buscarse la vida             |
| Probar la conexión **antes** de guardar                      | Guardar y descubrir el fallo en producción  |
| Resumen final: «esto es lo que vas a aplicar»                | Un botón «Guardar» y fe                     |
| Describir la **consecuencia** de cada opción                 | Enumerar identificadores técnicos           |
| Guardar por sección, con su propia acción                    | Un único «Guardar» gigante al fondo         |
| Poder cancelar sin dejar restos a medio crear                | Instancias huérfanas tras un `Esc`          |

#### 8.2 Las primitivas (`@sytue/ui`)

Uso **obligatorio**; están para esto y no se reimplementan por app (regla §7.1–7.2).

| Primitiva             | Qué es                                                                                                                                                                                                                                                                                         | Cuándo                                                                                       |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| **`Wizard`**          | Marco canónico de «conectar/configurar algo»: pasos, atrás/siguiente, validación por paso, paso terminal. `WizardStep` lleva `heading` (llano), `description`, `content`, `canContinue` (verja de validación), `nextLabel`, `onNext` (ejecutar la acción del paso en vez de avanzar) y `done`. | Alta o cambio de algo con **consecuencias**, o que necesita datos de un sistema externo.     |
| **`Stepper`**         | Progreso del asistente (`<ol>` con `aria-current="step"`). Sin `onStepClick` es indicador puro.                                                                                                                                                                                                | Dentro del `Wizard`; también en flujos multi-paso propios.                                   |
| **`SettingsShell`**   | Marco de una página de **ajustes**: título + descripción en lenguaje llano, acción principal, `banner` para avisos permanentes, ancho de lectura acotado (`max-w-5xl`). `level` evita duplicar el `h1`.                                                                                        | La página de administración continua de una función.                                         |
| **`SettingsSection`** | Bloque autoexplicativo dentro de la página, con sus **propias** acciones. `plain` para contenido libre sin `Card`.                                                                                                                                                                             | Cada grupo de ajustes. **Se guarda por sección**, no al final de la página.                  |
| **`FieldHint`**       | Una línea bajo un campo: qué es y **dónde encontrarlo**. Cambia a estado ok/error en el mismo sitio y tamaño (solo cambian icono y color); `tone="danger"` emite `role="alert"`. `id` para enlazar con `aria-describedby`.                                                                     | Todo campo cuyo valor viene de otro sistema o admite error.                                  |
| **`InlineHelp`**      | Bloque de contexto dentro del flujo: «qué tener a mano», el resultado de una prueba, una advertencia. Sobrio: borde + fondo teñido, sin degradados ni sombras.                                                                                                                                 | Antes de pedir datos, y para devolver el resultado de una comprobación.                      |
| **`ReviewSummary`**   | Lista clave→valor de «esto es lo que vas a aplicar». `mono` para valores técnicos (URL, id); `onEdit` por fila salta al paso donde se rellenó ese dato.                                                                                                                                        | Penúltimo paso de todo asistente que aplique cambios.                                        |
| **`ChoiceCard`**      | Tarjeta-opción de un grupo de radios: título + **la consecuencia** de elegirla, en lenguaje llano. `media` para catálogos con logo. Implementado sobre `<input type="radio">` nativo (navegación con flechas gratis).                                                                          | «Elige una de estas» — nunca un `select` de identificadores.                                 |
| **`EmptyState`**      | Estado vacío con marca, título, descripción y CTA (§3.17).                                                                                                                                                                                                                                     | Antes de que exista la primera configuración: el vacío es la puerta de entrada, no un error. |

> **Estado real (2026-07-27):** `ChoiceCard` existe en `@sytue/ui` pero **aún no se exporta**
> desde `index.ts` y no tiene consumidores; `AccessPolicyWizard` usa todavía una copia local.
> La migración está en curso. Hasta que cierre, no es cierto que «ya se use en todas partes».

#### 8.3 Asistente o panel de ajustes

Regla corta: **el asistente crea, el panel administra.** En la práctica se combinan — el panel
es un `SettingsShell` cuya acción principal abre el `Wizard`.

| Usa **`Wizard`** cuando…                                                 | Usa **`SettingsShell` + `SettingsSection`** cuando… |
| ------------------------------------------------------------------------ | --------------------------------------------------- |
| Es un alta, y hasta terminar no hay nada útil                            | El objeto ya existe y se está afinando              |
| Hay **orden**: un paso depende del anterior                              | Los ajustes son independientes entre sí             |
| Hay que **probar** contra un sistema externo antes de aplicar            | El cambio es local y reversible al instante         |
| El resultado tiene consecuencias para otras personas (acceso, políticas) | El cambio afecta a una sola preferencia             |
| El admin necesita datos de otro producto y hay que decirle dónde están   | El admin ya conoce el campo                         |

Y siempre, en cualquiera de los dos:

- **Probar antes de guardar.** Si hay un tercero al otro lado, hay paso de prueba, y su
  resultado se muestra con `InlineHelp` — no con un `toast` que se va.
- **Cancelar no deja restos.** Si el flujo tuvo que crear un borrador para poder probar, se
  borra al cancelar (patrón ya aplicado en `ConnectAppWizard` y `ConnectIdpWizard`).
- **Los secretos son _write-only_.** Se piden una vez, no se devuelven nunca.
- **Qué pasa con los datos, antes de conceder nada.** Todo lo que conecta Sytue One con un
  tercero enseña `DataFlow` (`@sytue/ui`): si los datos solo entran, salen hacia el tercero o van
  en los dos sentidos, qué tipo de datos y qué política los cubre (lo que entra, la de Sytue One;
  lo que sale, la del tercero). La dirección se deduce de las listas `incoming`/`outgoing`, no se
  declara aparte. En un asistente va la versión completa con la marca del tercero; en un bloque
  de un clic o una pestaña donde el otro lado es cualquiera (un webhook, un bot), `compact`.
- **El resumen habla en español**, no en JSON. `AccessPolicyWizard` traduce la regla entera a
  una frase y advierte a quién dejaría fuera: ese es el listón.
- **A11y de §6 sin excepciones:** el foco va al encabezado del paso al cambiar, el progreso es
  un `<ol>` con `aria-current="step"`, y todo campo con ayuda la enlaza por `aria-describedby`.

#### 8.4 Inventario de superficies

**Ya elevadas** (`apps/frontend/apps/console/src/components/`):

| Superficie                                         | Componentes                                        | Flujo                                                                                                                                |
| -------------------------------------------------- | -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| **Conectar una aplicación** (Integration Gateway)  | `ConnectAppWizard.tsx` + `AppsPanel.tsx`           | elegir → qué activarás → configurar → **probar** → revisar → hecho                                                                   |
| **Conectar proveedor de identidad (SSO)**          | `ConnectIdpWizard.tsx` + `SsoPanel.tsx`            | tipo (OIDC/SAML) → qué cambiará para tu equipo → configurar → **probar** → revisar → hecho (con los valores a pegar en el proveedor) |
| **Constructor de políticas de Acceso Condicional** | `AccessPolicyWizard.tsx` + `AccessPolicyPanel.tsx` | a quién afecta → en qué situaciones → qué debe ocurrir → revisar (regla en una frase) → hecho                                        |

**Pendientes** (siguen siendo `SectionHeader` + `Modal` + formulario plano):

- Console: `GroupsPanel` (incl. reglas de grupo dinámico), `RolesPanel`, `PrivilegedAccessPanel`
  (PIM), `CompliancePanel`, `GovernancePanel` (DLP), `DevicesPanel`, `UsersPanel`,
  `TenantsPanel` / `TenantDetailPanel`, `UnifiedAuditPanel`, `PasswordPolicyCard`, `ProfileCard`.
- **Fuera de Console no hay ninguna superficie de ajustes** que use estas primitivas: Mail,
  Drive, Calendar, Contacts, Portal y Cloud Dashboard no tienen aún pantalla de ajustes propia
  construida con `SettingsShell`. Es el mayor hueco frente a la directiva de §8.1, que aplica a
  **toda** la suite y no solo a la administración.

> Al elevar una superficie: primero `SettingsShell` + `SettingsSection` (partir el muro), luego
> `Wizard` para el alta, y `FieldHint`/`InlineHelp` para que cada dato diga de dónde sale.

---

<a id="3-assets-y-vendorizado"></a>

## 3. Assets y vendorizado

Fuente de verdad = `branding/`. El frontend **vendoriza** una copia vía
`scripts/sync-branding.sh` (`make sync-branding`, corre en `fe-install`) porque las imágenes
Docker del frontend se construyen con contexto `apps/frontend/`, sin acceso a `branding/`.

| Asset                       | Fuente de verdad                                    | Copia vendorizada                       | ¿Lo trae el sync? |
| --------------------------- | --------------------------------------------------- | --------------------------------------- | ----------------- |
| Tokens de color (CSS)       | `branding/tokens/colors.css`                        | `packages/config/src/tokens/colors.css` | **Sí**            |
| Tokens (JSON de referencia) | `branding/tokens/*.json`                            | — (no se importan)                      | No (deliberado)   |
| Tokens (espejo TS)          | —                                                   | `packages/config/src/tokens.ts`         | No — **a mano**   |
| Iconos de producto          | `branding/icons/*.svg`                              | `packages/ui/src/assets/icons/*.svg`    | **Sí**            |
| Logos                       | `branding/logo/*.svg`                               | `packages/ui/src/assets/logos/*.svg`    | **Sí**            |
| Ilustraciones + og          | `branding/illustrations/*.svg`, `branding/og/*.svg` | pendiente                               | **No**            |

**Trampas (cuestan una tarde cada una):**

- **No editar a mano las copias vendorizadas.** Son salida de `sync-branding.sh`: una edición
  manual se pierde en el siguiente sync y produce deriva claro/oscuro. Editar `branding/` y
  re-sincronizar.
- **`tokens.ts` es un espejo manual** (el sync no lo toca): es la fuente de deriva más fácil.
- **Los roles `*-text` no sirven sin el mapeo `@theme`.** Re-sincronizar el CSS sin mapear deja
  las utilidades `text-*-text` inexistentes y el cambio falla **en silencio**.
- **`border`/`border-strong` no llegan a 3:1 a propósito** (separadores decorativos): el foco o
  el estado nunca descansan solo en el borde — hay anillo de foco de 3 px en `theme.css`.
- **El modo oscuro es reasignación, no inversión.** Ya está en `colors.css` con
  `[data-theme="dark"]` y `prefers-color-scheme`; no invertir manualmente en las apps.
- **Nunca renombrar tokens `--sytue-*` ni cambiar las APIs de `@sytue/ui`**: los consumidores
  dependen de ambos.

### 3.2 Puerta automática: `apps/frontend/scripts/lint-i18n.sh`

Cuenta los rótulos de interfaz escritos en castellano en el código y **falla si el
número sube**. Es un trinquete, no una puerta cerrada: queda deuda de sobra como para
que exigir cero bloquease cualquier commit, y un gate que no se puede cumplir se acaba
desactivando. Al bajar la cuenta, el propio script dice qué número poner.

Antes de crear una clave, **busca el literal en el `es.ts` de la app**. El patrón que
más se repite no es texto sin traducir sino traducción sin usar: `app.*.description`,
`quick.*`, `slides.shape.*`, las 191 claves `extdata.*`… todas escritas en los cinco
idiomas mientras el componente pintaba el literal castellano al lado.

**Los dos gates de i18n son complementarios y ninguno basta solo:**

|                            | Ve                                                       | No ve                                                      |
| -------------------------- | -------------------------------------------------------- | ---------------------------------------------------------- |
| `lint-i18n.sh`             | `label=`, `title:`, `placeholder=`… en cualquier fichero | Texto suelto en JSX; lo que sale de constantes compartidas |
| `i18n-sin-espanol.spec.ts` | Lo RENDERIZADO, cargando cada app en inglés              | Lo que no se pinta sin abrir un popover o un menú          |

Ese segundo hueco es real y costó descubrirlo: el spec recorre las pestañas de la cinta
y las daba por limpias, pero las cadenas de los menús desplegables —validación de datos,
sparklines, gráficos— no se renderizan hasta abrirlos, y ahí seguía habiendo castellano.

### 3.1 Puerta automática: `apps/frontend/scripts/lint-design.sh`

Corre dentro de `bun run lint`. Cubre los tres defectos de diseño que ni TypeScript ni
ESLint pueden ver, porque viven **dentro de cadenas de clases**. Los tres se detectaron
arreglando defectos reales, no en abstracto:

| Regla                                  | Qué rompe                                                              | Por qué no se ve en revisión                                                                   |
| -------------------------------------- | ---------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Anillo de foco sin contorno neutro     | WCAG 1.4.11                                                            | El latón da 2,81:1 sobre marfil pero **sí pasa en tema oscuro**, que es donde se suele revisar |
| Escalón crudo `primary-*` / `accent-*` | El tema oscuro no reasigna esas familias                               | En claro se ve perfecto                                                                        |
| Clase concatenada sin separador        | La clase resultante **no existe** y se lleva por delante la de al lado | El navegador no avisa: simplemente no aplica nada                                              |

La tercera es la más traicionera: ``class={`w-max${fz ? 'sticky' : ''}`}`` produce
`w-maxsticky`. Rompió **«Congelar paneles» de Sheets** en cuatro sitios a la vez —la
función no congelaba nada— y solo se manifestaba en la rama condicional, que es la
excepcional. El chequeo lee cada fichero entero (`perl -0777`) y no por líneas: los dos
casos peores tenían el `${` y el ternario en líneas distintas, justo por ser los templates
más largos.

> **El separador va fuera de la interpolación.** `` `base ${c ? 'x' : ''}` ``, nunca
> `` `base${c ? ' x' : ''}` ``. Las dos producen el mismo HTML, pero
> `prettier-plugin-tailwindcss` normaliza las cadenas de clases y **borra el espacio de
> la segunda**: el arreglo dura hasta el siguiente `bun run format`. Ahí estaba el origen
> de todo esto — el defecto no lo escribió nadie, lo reintroducía el formateador, y por
> eso la barra de aforo de Sytue Admin volvía a romperse cada vez que se arreglaba.

Lo que un `grep` no puede comprobar queda en e2e: `test/e2e/tests/sheets-freeze.spec.ts`
verifica que la fila congelada, además de quedarse fija, **tapa lo de debajo**. El
resaltado de selección es latón al 9 % —translúcido a propósito—, y sobre una fila que
flota encima de otras dejaba leer dos números de fila a la vez.

Desde la auditoría móvil del 2026-08-08 hay una cuarta regla: **ninguna altura de layout
en `vh`**. En Chrome Android `100vh` es la altura **con la barra de direcciones
retraída** —más que la pantalla visible al cargar—, así que un marco que no desplaza deja
fuera lo que sobra y no hay forma de alcanzarlo. La unidad correcta es `dvh`, la que ya
usaba `AdminShell`. Se había colado en cuatro sitios (los dos shells de Docs, el
aterrizaje de enlaces de Rooms y el propio `AppShell`) y ninguna revisión lo vio, porque
en un navegador de escritorio `vh` y `dvh` **miden lo mismo**: el defecto solo existe en
el teléfono.

<a id="32-desbordamiento-en-movil"></a>

### 3.3 Desbordamiento en móvil: las tres capas

Que la interfaz no se salga de un teléfono no se sostiene revisando; se sostiene con
puertas. Hay tres, de la más barata a la más cara:

1. **`lint-design.sh` (en `bun run lint`, o sea en cada CI).** Caza estáticamente lo que
   se puede cazar así: hoy, la altura en `vh`. Es la única capa que impide que la
   regresión **entre**.
2. **`test/e2e/mobile-audit.mjs`.** Mide de verdad, en 18 perfiles reales de dispositivo
   (iPhone SE/12/14 Pro Max y apaisado, iPad Mini; Galaxy S9+/S24/A55/Tab S4/Tab S9/Z Flip
   Cover/Z Fold 6, Moto G4, Pixel 5/7/9, LG Optimus L70). Mide **cada contenedor**, no
   solo el documento: los shells de la suite atrapan el scroll dentro de `main`, así que
   `document.scrollWidth` da falsos verdes. **Sale con código 1 si algo desborda**, para
   poder usarlo como puerta y no solo como informe.
3. **La auditoría diaria programada**, que lo corre entero y resuelve lo que encuentre.

**Convención `data-scroll-x`.** Un desplazamiento horizontal deliberado —una tabla ancha,
la rejilla de una hoja de cálculo, la galería de estilos del editor, la tira de pestañas
de la cinta— se marca con ese atributo en el contenedor que desplaza. Sin él, la
auditoría lo cuenta como defecto todos los días y el informe se vuelve ruido que nadie
lee. Las tablas y las rejillas (`table`, `role="grid"`) ya se reconocen solas.

**Los tres patrones que causan esto una y otra vez**, por si sirven de checklist al
escribir: un hijo de rejilla o de flex sin `min-w-0` (no baja de su ancho mínimo de
contenido, y la columna entera se estira); un ancho fijo (`w-80`, `w-52`) sin variante
responsive; y una fila de acciones con `shrink-0` que nunca envuelve.

---
