# Sytue — Guía de marca (brand guidelines)

> Marca padre: **Sytue**. Suite: **Sytue One**. Identidad visual moderna y
> **100% Sytue-native**. Esta guía rige el uso de logos, color, tipografía y
> tono. La implementación de UI vive en el sistema de diseño (`docs/design.md#2-sistema-de-diseno`)
> y la marca en `branding/` (tokens, logos, iconos).

---

## 1. La marca

**Sytue** es una suite de productividad **europea, soberana y libre**: alternativa a
Google Workspace / Microsoft 365 construida 100% sobre infraestructura UE (Scaleway)
y software libre (FOSS). **Sytue One** es el conjunto integrado (portal único, login
único, shell común). La soberanía europea y el código abierto son **valores centrales
de marca**, no notas al pie.

**Atributos de marca:** confiable · clara · soberana · moderna · libre.

---

## 2. Logos

Activos en `branding/logo/`:

| Activo | Archivo | Uso |
|---|---|---|
| Wordmark marca padre | `sytue.svg` | marca general, documentos, footers |
| Wordmark suite | `sytue-one.svg` | producto/portal, materiales de la suite |
| Wordmark suite (blanco) | `sytue-one-white.svg` | sobre fondos de marca / sidebar oscuro |
| Símbolo (marca aislada) | `sytue-mark.svg` | avatar de app, espacios cuadrados, favicons grandes |
| Favicon | `favicon.svg` | pestañas, PWA |

El símbolo de marca es un **bloque redondeado** (azul Sytue `#2b6cff`) con la
**"S-swoosh"** blanca. Es la base geométrica de todos los iconos de producto.

### Área de seguridad y tamaños

- **Área de seguridad:** margen libre alrededor del logo ≥ **la mitad de la altura
  del símbolo** (≈ el ancho del bloque "S"). Nada (texto, bordes, otros logos) invade
  esa zona.
- **Tamaño mínimo:** wordmark **≥ 96px** de ancho; símbolo **≥ 24px**. Por debajo,
  usar el favicon.
- Mantener siempre la **proporción** original; escalar de forma uniforme.

### Do / Don't

**Sí (do):**
- Usar los SVG provistos, sin alterar.
- Sobre fondo claro: logo a color. Sobre fondo de marca/oscuro: versión blanca.
- Respetar el área de seguridad y el tamaño mínimo.

**No (don't):**
- No recolorear el logo fuera de la paleta (`branding/tokens/colors.json`).
- No deformar, rotar, aplicar sombras, degradados ni contornos.
- No reconstruir el wordmark con otra tipografía.
- No colocar el logo a color sobre fondos de bajo contraste.
- No encerrar el símbolo en otra forma ni añadir relleno desigual.

---

## 3. Color

Paleta canónica en `branding/tokens/colors.json` / `colors.css`.

- **Primario — Azul Sytue `#2b6cff`:** acción, marca, enlaces, foco.
- **Acento — Teal Sytue `#00c2a8`:** detalles de marca, presencia, "One".
- **Neutros:** superficies y texto (`#ffffff` … `#0f1115`).
- **Semánticos:** success `#1f9d57`, warning `#d98b0a`, danger `#e0395e`, info `#2b6cff`.

Reglas: usar **tokens/variables de rol**, nunca HEX sueltos en producto; garantizar
contraste **WCAG AA**; respetar modo claro y oscuro.

---

## 4. Tipografía

Familia única **Inter** (libre, SIL OFL) para toda la suite; **JetBrains Mono** para
código. Escala y roles en `branding/tokens/typography.json`. No introducir otras
familias en superficies de producto.

---

## 5. Iconografía — sistema de iconos de producto

En `branding/icons/` viven los 14 **iconos de producto** (`one, id, drive, docs, sheets,
slides, mail, calendar, contacts, chat, meet, cloud, console, hub`) — el "logo de app"
que aparece en el **app switcher** de la Sytue Shell, en avatares de app y en favicons
grandes. Son un **set único y coherente**: mismo grid, mismo trazo, misma familia formal.
Nacen del mismo molde que el símbolo de marca (la S-swoosh). No confundir con los **iconos
de UI** (chevrons, papelera, buscar…), que son line-icons neutros 24×24 dentro de los
componentes (ver `docs/design.md#1-direccion-de-arte §4.4`); no se mezclan los dos sistemas.

### 5.1 Grid y construcción

| Parámetro | Valor | Nota |
|---|---|---|
| **viewBox** | `0 0 40 40` | canónico; escala a cualquier tamaño |
| **Tile** | `rect x4 y4 w32 h32 rx9` | bloque redondeado; el `rx 9` ecoa el radio del símbolo de marca |
| **Área segura del glyph** | 24×24 (≈ `x8→x32`, `y8→y32`) | el dibujo respira dentro del tile; nada toca el borde |
| **Trazo** | `stroke-width="2.4"` **uniforme** | un solo grosor en todo el set (incl. líneas interiores) |
| **Terminaciones** | `stroke-linecap="round"` + `stroke-linejoin="round"` | siempre las dos, en todo trazo |
| **Glyph** | blanco `#ffffff`, outline (sin relleno salvo puntos/acento puntual) | contraste garantizado sobre el tile de color |
| **Rejilla** | coordenadas sobre malla de ~1px, formas centradas ópticamente | la calidad nace de la alineación |

Legible y reconocible a **20px**, bonito a **64px**. Una sola idea geométrica por icono;
geométrico antes que ilustrativo.

### 5.2 Color y tematización

Cada icono se rellena con su **color de producto** vía variable CSS con *fallback* HEX:

```svg
<rect x="4" y="4" width="32" height="32" rx="9" fill="var(--sytue-product-drive, #1f9d57)"/>
```

- Las variables `--sytue-product-*` viven en `branding/tokens/colors.css` (fuente única)
  y coinciden con `colors.json → product`. El *fallback* HEX hace que el SVG se vea
  correcto **también aislado** (sin CSS cargado: e-mail, README, favicon estático).
- El **glyph es blanco fijo**, no `currentColor`: lo que se tematiza es el tile, no el
  trazo. El color de producto **no invade** la UI de trabajo — solo tiñe el icono y
  acentos contextuales puntuales (ver `art-direction.md §3.4`).

Colores de producto vigentes:

| App | Color | App | Color |
|---|---|---|---|
| one | azul `#2b6cff` | contacts | teal `#00c2a8` |
| id | teal `#00c2a8` | chat | azul `#2b6cff` |
| drive | verde `#1f9d57` | meet | violeta `#7c5cff` |
| docs | azul `#2b6cff` | cloud | azul profundo `#1f57d6` |
| sheets | verde `#1f9d57` | console | gris `#5c6270` |
| slides | ámbar `#d98b0a` | hub | teal `#00c2a8` |
| mail | rojo `#e0395e` | calendar | violeta `#7c5cff` |

### 5.3 Símbolo de marca y logos (coherencia formal)

Los logos comparten el mismo ADN que los iconos: mismo tile redondeado y la **S-swoosh**
como raíz formal única, con `stroke-linejoin="round"` y radios de la misma familia. El
tile del símbolo se tematiza con `var(--sytue-primary, #2b6cff)`; el wordmark usa
`var(--sytue-text, …)` para adaptarse a claro/oscuro. La variante `sytue-one-white.svg`
es para fondos de marca/sidebar oscuro. Una sola S-swoosh, escalada proporcionalmente,
en símbolo, wordmark y favicon — nunca tres curvas distintas.

### 5.4 Reglas

- **No** crear iconos fuera de este molde (mismo tile, trazo, aire).
- **No** mezclar rellenos e outline en un mismo glyph salvo puntos de acento (p. ej. el
  punto del día en `calendar`).
- **No** meter HEX de color de producto suelto en una app: usar la variable.
- SVG **limpios**: sin metadatos de editor, `viewBox` coherente, formas mínimas.
- Un icono nuevo (app futura) se dibuja sobre este mismo grid y se añade su
  `--sytue-product-<app>` al token.

---

## 6. Tono de voz

- **Idioma de producto:** **español** (es_ES). Identificadores de código en inglés.
- **Claro y directo.** Frases cortas, sin jerga cloud innecesaria (especialmente en
  Sytue DevX: lenguaje sencillo, orientado a la acción).
- **Cercano y profesional.** Trato de "tú/tu" en producto; respetuoso y sobrio.
- **Honesto sobre soberanía y libertad.** Comunicamos con orgullo el origen europeo y
  el software libre, sin exagerar ni alarmar.
- **Accesible.** Mensajes de error útiles que explican qué pasó y qué hacer.

Ejemplos:
- Botón: "Crear evento" (no "Creación de evento").
- Vacío: "Aún no tienes archivos. Sube el primero para empezar."
- Error: "No pudimos guardar los cambios. Revisa tu conexión e inténtalo de nuevo."

---

## 7. Soberanía europea como valor de marca

- **Mensaje central:** *"Tu suite europea, soberana y libre."*
- Datos 100% en la UE (Scaleway, solo regiones UE); sin proveedores US para datos
  personales; sin transferencias fuera de la UE.
- Todo el stack es **FOSS con licencia libre**; el código propio es **AGPL-3.0**.
- Donde sea pertinente (footer, login, marketing), reforzar el origen europeo con
  sobriedad (p.ej. indicador 🇪🇺), nunca como mero adorno.

---

## 8. Ilustración e imagen de marca

Sistema de ilustración SVG en `branding/illustrations/` (+ imágenes sociales en
`branding/og/`). Es **ilustración de servicio, no decoración**: aparece casi solo en
`EmptyState`, onboarding, páginas de error y superficies de marca (www). Sigue la
dirección de `docs/design.md#1-direccion-de-arte §4.5`.

### 8.1 Estilo

- **Line-art geométrico y plano.** Construido con las mismas primitivas del sistema:
  tiles redondeados (mismo molde `rx` que el símbolo de marca), la **S-swoosh** como
  motivo recurrente, trazo del grosor del icono de producto (`2.4px` a escala 200×160;
  `linecap`/`linejoin` `round`). Nada de isométrico, sombras largas, degradados difusos
  ni personajes "corporate memphis".
- **Un solo toque de color por pieza.** El resto es neutro. El acento marca el foco:
  azul (acción/marca), teal (presencia/éxito), violeta (dato/Calendar) o `danger`
  (error, con forma reconocible además del color).
- **Concepto, no chiste.** Un empty state ilustra el *vacío* (una bandeja, una carpeta),
  sereno y útil. Acompaña **siempre** a título + descripción + CTA; nunca va solo.

### 8.2 Color y tema (claro/oscuro)

Las ilustraciones usan **roles de token** (`stroke="var(--sytue-neutral-400)"`,
`var(--sytue-primary-500)`, `var(--sytue-accent-500)`, `var(--sytue-border)`…), **nunca
HEX sueltos**. Así respetan light/dark automáticamente al heredar las variables de
`branding/tokens/colors.css`. Consumo recomendado: `<img>`/`<object>` inline o SVG
embebido en el DOM para que las custom properties cascadeen.

> **Excepción — superficies de marca fijas.** El hero (`hero-suite.svg`) y las imágenes
> de `branding/og/` viven sobre azul profundo de marca **independiente del tema** (igual
> que `sytue-one-white.svg`), por lo que embeben los hexes de marca a propósito. No son
> el patrón general; toda ilustración de producto usa roles.

### 8.3 Catálogo

| Pieza | Archivo | Uso |
|---|---|---|
| Bandeja vacía | `illustrations/empty-mail.svg` | Mail sin correos |
| Sin archivos | `illustrations/empty-drive.svg` | Drive/carpeta vacía |
| Sin eventos | `illustrations/empty-calendar.svg` | Calendar vacío |
| Canal vacío | `illustrations/empty-chat.svg` | Chat/Hub sin mensajes |
| Sin contactos | `illustrations/empty-contacts.svg` | Contacts vacío |
| Sin resultados | `illustrations/empty-search.svg` | Búsqueda sin coincidencias (genérico) |
| Error / reintentar | `illustrations/error-retry.svg` | Fallo de carga o red |
| Todo al día | `illustrations/all-done.svg` | Bandeja/lista completada (estado positivo) |
| Hero de suite | `illustrations/hero-suite.svg` | www: imagen de marca (640×480) |
| Spot soberanía | `illustrations/spot-sovereignty.svg` | Dato en la UE (120×120) |
| Spot subir | `illustrations/spot-upload.svg` | "Sube tu primer archivo" (120×120) |
| OG / social card | `og/og-image.svg` | Open Graph / Twitter (1200×630) |
| Favicon maskable | `og/favicon-maskable.svg` | PWA maskable (512×512, safe-zone) |

Los empty states comparten `viewBox` **200×160**; los spots **120×120**. Para el favicon
de pestaña sigue usándose `logo/favicon.svg` (32×32); el maskable es su variante PWA con
fondo a sangre y símbolo dentro de la safe-zone.

### 8.4 Reglas

- **Sí:** usar los SVG provistos; incrustarlos de forma que hereden las variables de
  tema; acompañar siempre de texto y acción; mantener el `viewBox` al escalar.
- **No:** recolorear fuera de la paleta; añadir un segundo acento; usar la ilustración
  en UI de trabajo densa (ahí basta un icono de sistema); crear ilustración nueva que no
  siga el molde geométrico (line-art, un solo acento, roles de token). Ilustración que no
  aporta, se elimina.

---

## 9. Diseño 100% Sytue-native

> **Nota de retiro (v4):** ya **no** hay motores de terceros que tematizar. Los motores
> AGPL (Mattermost, Nextcloud, Keycloak, OnlyOffice, Stalwart) fueron **retirados** y
> sustituidos por servicios **Sytue-native en Go**, por lo que `branding/themes/` ya no
> existe.

Toda la suite es **obra original de Sytue** con una sola identidad visual. La marca, la
paleta y la tipografía las define directamente `@sytue/ui` a partir de `branding/`
(tokens, logos, iconos) y deben coincidir con esta guía y con `docs/design.md#2-sistema-de-diseno`.
