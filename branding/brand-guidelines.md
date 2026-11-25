<a id="sytue--guía-de-marca-brand-guidelines"></a>
# Sytue — Brand guidelines

> Parent brand: **Sytue**. Suite: **Sytue One**. A modern and **100% Sytue-native**
> visual identity. This guide governs the use of logos, color, typography and
> tone. The UI implementation lives in the design system (`docs/design.md#2-sistema-de-diseno`)
> and the brand in `branding/` (tokens, logos, icons).

---

<a id="1-la-marca"></a>
## 1. The brand

**Sytue** is a **European, sovereign and free** productivity suite: an alternative to
Google Workspace / Microsoft 365 built 100% on EU infrastructure (Scaleway)
and free software (FOSS). **Sytue One** is the integrated whole (single portal, single
login, common shell). European sovereignty and open source are **core brand
values**, not footnotes.

**Brand attributes:** trustworthy · clear · sovereign · modern · free.

---

## 2. Logos

Assets in `branding/logo/`:

| Asset | File | Use |
|---|---|---|
| Parent brand wordmark | `sytue.svg` | general brand, documents, footers |
| Suite wordmark | `sytue-one.svg` | product/portal, suite materials |
| Suite wordmark (white) | `sytue-one-white.svg` | on brand backgrounds / dark sidebar |
| Symbol (standalone mark) | `sytue-mark.svg` | app avatar, square spaces, large favicons |
| Favicon | `favicon.svg` | tabs, PWA |

The brand symbol is a **rounded block** (Sytue blue `#2b6cff`) with the white
**"S-swoosh"**. It is the geometric basis of all product icons.

<a id="área-de-seguridad-y-tamaños"></a>
### Clear space and sizes

- **Clear space:** free margin around the logo ≥ **half the height of the
  symbol** (≈ the width of the "S" block). Nothing (text, borders, other logos) intrudes
  on that zone.
- **Minimum size:** wordmark **≥ 96px** wide; symbol **≥ 24px**. Below that,
  use the favicon.
- Always keep the original **proportions**; scale uniformly.

### Do / Don't

**Do:**
- Use the provided SVGs, unaltered.
- On a light background: color logo. On a brand/dark background: white version.
- Respect the clear space and the minimum size.

**Don't:**
- Do not recolor the logo outside the palette (`branding/tokens/colors.json`).
- Do not distort, rotate, or apply shadows, gradients or outlines.
- Do not rebuild the wordmark with another typeface.
- Do not place the color logo on low-contrast backgrounds.
- Do not enclose the symbol in another shape or add uneven padding.

---

## 3. Color

Canonical palette in `branding/tokens/colors.json` / `colors.css`.

- **Primary — Sytue Blue `#2b6cff`:** action, brand, links, focus.
- **Accent — Sytue Teal `#00c2a8`:** brand details, presence, "One".
- **Neutrals:** surfaces and text (`#ffffff` … `#0f1115`).
- **Semantic:** success `#1f9d57`, warning `#d98b0a`, danger `#e0395e`, info `#2b6cff`.

Rules: use **role tokens/variables**, never loose HEX values in product; guarantee
**WCAG AA** contrast; respect light and dark mode.

---

<a id="4-tipografía"></a>
## 4. Typography

A single family, **Inter** (free, SIL OFL), for the whole suite; **JetBrains Mono** for
code. Scale and roles in `branding/tokens/typography.json`. Do not introduce other
families on product surfaces.

---

<a id="5-iconografía--sistema-de-iconos-de-producto"></a>
## 5. Iconography — product icon system

`branding/icons/` holds the 14 **product icons** (`one, id, drive, docs, sheets,
slides, mail, calendar, contacts, chat, meet, cloud, console, hub`) — the "app logo"
that appears in the Sytue Shell **app switcher**, in app avatars and in large
favicons. They are a **single, coherent set**: same grid, same stroke, same formal family.
They come from the same mold as the brand symbol (the S-swoosh). Not to be confused with
the **UI icons** (chevrons, trash, search…), which are neutral 24×24 line icons inside the
components (see `docs/design.md#1-direccion-de-arte §4.4`); the two systems are not mixed.

<a id="51-grid-y-construcción"></a>
### 5.1 Grid and construction

| Parameter | Value | Note |
|---|---|---|
| **viewBox** | `0 0 40 40` | canonical; scales to any size |
| **Tile** | `rect x4 y4 w32 h32 rx9` | rounded block; the `rx 9` echoes the radius of the brand symbol |
| **Glyph safe area** | 24×24 (≈ `x8→x32`, `y8→y32`) | the drawing breathes inside the tile; nothing touches the edge |
| **Stroke** | **uniform** `stroke-width="2.4"` | a single weight across the whole set (incl. inner lines) |
| **Line ends** | `stroke-linecap="round"` + `stroke-linejoin="round"` | always both, on every stroke |
| **Glyph** | white `#ffffff`, outline (no fill except dots/occasional accent) | guaranteed contrast on the colored tile |
| **Grid** | coordinates on a ~1px mesh, optically centered shapes | quality comes from alignment |

Legible and recognizable at **20px**, beautiful at **64px**. A single geometric idea per icon;
geometric rather than illustrative.

<a id="52-color-y-tematización"></a>
### 5.2 Color and theming

Each icon is filled with its **product color** via a CSS variable with a HEX *fallback*:

```svg
<rect x="4" y="4" width="32" height="32" rx="9" fill="var(--sytue-product-drive, #1f9d57)"/>
```

- The `--sytue-product-*` variables live in `branding/tokens/colors.css` (single source)
  and match `colors.json → product`. The HEX *fallback* makes the SVG look
  right **also on its own** (with no CSS loaded: email, README, static favicon).
- The **glyph is fixed white**, not `currentColor`: what gets themed is the tile, not the
  stroke. The product color **does not spill into** the working UI — it only tints the icon and
  occasional contextual accents (see `art-direction.md §3.4`).

Current product colors:

| App | Color | App | Color |
|---|---|---|---|
| one | blue `#2b6cff` | contacts | teal `#00c2a8` |
| id | teal `#00c2a8` | chat | blue `#2b6cff` |
| drive | green `#1f9d57` | meet | violet `#7c5cff` |
| docs | blue `#2b6cff` | cloud | deep blue `#1f57d6` |
| sheets | green `#1f9d57` | console | gray `#5c6270` |
| slides | amber `#d98b0a` | hub | teal `#00c2a8` |
| mail | red `#e0395e` | calendar | violet `#7c5cff` |

<a id="53-símbolo-de-marca-y-logos-coherencia-formal"></a>
### 5.3 Brand symbol and logos (formal coherence)

The logos share the same DNA as the icons: the same rounded tile and the **S-swoosh**
as the single formal root, with `stroke-linejoin="round"` and radii from the same family. The
symbol's tile is themed with `var(--sytue-primary, #2b6cff)`; the wordmark uses
`var(--sytue-text, …)` to adapt to light/dark. The `sytue-one-white.svg` variant
is for brand backgrounds/dark sidebar. A single S-swoosh, scaled proportionally,
in the symbol, wordmark and favicon — never three different curves.

<a id="54-reglas"></a>
### 5.4 Rules

- **Do not** create icons outside this mold (same tile, stroke, breathing room).
- **Do not** mix fills and outline in the same glyph except for accent dots (e.g. the
  day dot in `calendar`).
- **Do not** put a loose product-color HEX in an app: use the variable.
- **Clean** SVGs: no editor metadata, consistent `viewBox`, minimal shapes.
- A new icon (future app) is drawn on this same grid and its
  `--sytue-product-<app>` is added to the token.

---

<a id="6-tono-de-voz"></a>
## 6. Tone of voice

- **Product language:** **Spanish** (es_ES). Code identifiers in English.
- **Clear and direct.** Short sentences, no unnecessary cloud jargon (especially in
  Sytue DevX: plain, action-oriented language).
- **Friendly and professional.** Informal "tú/tu" address in product; respectful and sober.
- **Honest about sovereignty and freedom.** We communicate the European origin and
  free software with pride, without exaggerating or alarming.
- **Accessible.** Useful error messages that explain what happened and what to do.

Examples (Spanish product copy):
- Button: "Crear evento" ("Create event"; not "Creación de evento", "Event creation").
- Empty: "Aún no tienes archivos. Sube el primero para empezar." ("You have no files yet. Upload the first one to get started.")
- Error: "No pudimos guardar los cambios. Revisa tu conexión e inténtalo de nuevo." ("We couldn't save the changes. Check your connection and try again.")

---

<a id="7-soberanía-europea-como-valor-de-marca"></a>
## 7. European sovereignty as a brand value

- **Core message:** *"Tu suite europea, soberana y libre."* ("Your European, sovereign and free suite.")
- Data 100% in the EU (Scaleway, EU regions only); no US providers for personal
  data; no transfers outside the EU.
- The whole stack is **FOSS with a free license**; our own code is **AGPL-3.0**.
- Where relevant (footer, login, marketing), reinforce the European origin with
  restraint (e.g. a 🇪🇺 indicator), never as mere decoration.

---

<a id="8-ilustración-e-imagen-de-marca"></a>
## 8. Illustration and brand imagery

SVG illustration system in `branding/illustrations/` (+ social images in
`branding/og/`). It is **illustration that serves, not decoration**: it appears almost only in
`EmptyState`, onboarding, error pages and brand surfaces (www). It follows the
direction of `docs/design.md#1-direccion-de-arte §4.5`.

<a id="81-estilo"></a>
### 8.1 Style

- **Geometric, flat line art.** Built with the system's own primitives:
  rounded tiles (the same `rx` mold as the brand symbol), the **S-swoosh** as a
  recurring motif, a stroke as thick as the product icon's (`2.4px` at 200×160 scale;
  `linecap`/`linejoin` `round`). No isometric, long shadows, diffuse gradients
  or "corporate memphis" characters.
- **A single touch of color per piece.** The rest is neutral. The accent marks the focus:
  blue (action/brand), teal (presence/success), violet (data/Calendar) or `danger`
  (error, with a recognizable shape in addition to the color).
- **Concept, not joke.** An empty state illustrates the *emptiness* (a tray, a folder),
  calm and useful. It **always** accompanies a title + description + CTA; it never stands alone.

<a id="82-color-y-tema-clarooscuro"></a>
### 8.2 Color and theme (light/dark)

Illustrations use **token roles** (`stroke="var(--sytue-neutral-400)"`,
`var(--sytue-primary-500)`, `var(--sytue-accent-500)`, `var(--sytue-border)`…), **never
loose HEX values**. That way they respect light/dark automatically by inheriting the variables from
`branding/tokens/colors.css`. Recommended consumption: inline `<img>`/`<object>` or SVG
embedded in the DOM so that the custom properties cascade.

> **Exception — fixed brand surfaces.** The hero (`hero-suite.svg`) and the images
> in `branding/og/` live on the brand graphite **independently of the theme** (just
> like `sytue-one-white.svg`), so they embed the brand hexes on purpose: field
> `#1a1d21`, the symbol from `logo/sytue-mark.svg` as is and brass as the only accent.
> `arte-de-marca.test.ts` (in `@sytue/ui`) fails if any of them strays from that palette. They are not
> the general pattern; every product illustration uses roles.

<a id="83-catálogo"></a>
### 8.3 Catalog

| Piece | File | Use |
|---|---|---|
| Empty inbox | `illustrations/empty-mail.svg` | Mail with no emails |
| No files | `illustrations/empty-drive.svg` | Empty Drive/folder |
| No events | `illustrations/empty-calendar.svg` | Empty Calendar |
| Empty channel | `illustrations/empty-chat.svg` | Chat/Hub with no messages |
| No contacts | `illustrations/empty-contacts.svg` | Empty Contacts |
| No results | `illustrations/empty-search.svg` | Search with no matches (generic) |
| Error / retry | `illustrations/error-retry.svg` | Load or network failure |
| All caught up | `illustrations/all-done.svg` | Completed inbox/list (positive state) |
| Suite hero | `illustrations/hero-suite.svg` | www: brand image (640×480) |
| Sovereignty spot | `illustrations/spot-sovereignty.svg` | Data in the EU (120×120) |
| Upload spot | `illustrations/spot-upload.svg` | "Sube tu primer archivo" ("Upload your first file") (120×120) |
| OG / social card | `og/og-image.svg` | Open Graph / Twitter (1200×630) |
| Maskable favicon | `og/favicon-maskable.svg` | PWA maskable (512×512, safe zone) |

Empty states share a **200×160** `viewBox`; spots **120×120**. For the tab
favicon, `logo/favicon.svg` (32×32) is still used; the maskable one is its PWA variant with
a full-bleed background and the symbol inside the safe zone.

<a id="84-reglas"></a>
### 8.4 Rules

- **Do:** use the provided SVGs; embed them so that they inherit the theme
  variables; always accompany them with text and an action; keep the `viewBox` when scaling.
- **Don't:** recolor outside the palette; add a second accent; use illustration
  in dense working UI (a system icon is enough there); create new illustration that does not
  follow the geometric mold (line art, a single accent, token roles). Illustration that does not
  add anything gets removed.

---

<a id="9-diseño-100-sytue-native"></a>
## 9. 100% Sytue-native design

> **Retirement note (v4):** there are **no** longer third-party engines to theme. The
> AGPL engines (Mattermost, Nextcloud, Keycloak, OnlyOffice, Stalwart) were **retired** and
> replaced by **Sytue-native Go** services, so `branding/themes/` no longer
> exists.

The whole suite is **Sytue's original work** with a single visual identity. The brand, the
palette and the typography are defined directly by `@sytue/ui` from `branding/`
(tokens, logos, icons) and must match this guide and `docs/design.md#2-sistema-de-diseno`.
