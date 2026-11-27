<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos -->

<a id="diseño--dirección-de-arte-y-sistema-de-diseño"></a>

# Design — art direction and design system

The suite's single design document:

1. **[Art direction](#1-direccion-de-arte)** — the _why_: brand concept, visual
   principles and framework decisions.
2. **[Design system](#2-sistema-de-diseno)** — the _spec_: tokens, AppShell, `@sytue/ui`
   components, per-product layout, accessibility and mandatory rules.
3. **[Assets and vendoring](#3-assets-y-vendorizado)** — where each asset comes from and how it
   reaches the frontend without drift.

> Hierarchy: `branding/tokens/*` = the **number** (source of truth) · this §1 = the **judgment**
> · this §2 = the **spec** · `branding/brand-guidelines.md` = the **brand** (logos, icons,
> illustration). No color value is written by hand in the apps.

---

<a id="1-direccion-de-arte"></a>

<a id="1-dirección-de-arte"></a>

## 1. Art direction

> **The visual bible.** This document sets the cutting-edge DIRECTION for the whole
> suite and aligns all the other specialists (color, typography, iconography,
> illustration, motion, components). It is the "why" and "where to" layer; the final
> values live in `branding/tokens/`, the implementation in `@sytue/ui`, the usage
> rules in `docs/design.md#2-sistema-de-diseno` and the brand in `branding/brand-guidelines.md`.
>
> **Golden rule:** when facing a visual decision, this document overrides individual
> taste. If something here and a token conflict, either the token is fixed or this is
> fixed — nothing is ever improvised in an app.

---

<a id="10-en-una-frase"></a>

### 1.0 In one sentence

**A European professional tool: sober, dense and clear, with a quiet
confidence.** The bar is Linear / Vercel / Raycast / Mattermost in craft and polish —
not as a reference to copy, but as the level of finish we take for granted. No
template look, no "AI-generated" aesthetic (diffuse purple gradients,
gratuitous glassmorphism, blobs, stock isometric illustrations). If it looks like a demo,
it is wrong.

<a id="101-tinta-y-papel"></a>
<a id="101-esmeralda"></a>
<a id="101-color"></a>

<a id="101-lenguaje-visual-dos-colores-elegibles-2026-10-04"></a>

### 1.0.1 Visual language: two selectable colors (2026-10-04)

> **Status:** in force ([ADR 0071](decisions.md#0071)). It replaces “Tinta y papel” (Ink and paper) and the
> single emerald brand, which lasted one day. The principles of §1.1 and §1.3 do not change. The
> anchors `101-tinta-y-papel` and `101-esmeralda` are kept because there are links to them.

**In one sentence:** the suite offers TWO colors, **petróleo** (petrol — the one for whoever has not chosen
anything) and **azul cobalto** (cobalt blue). Color drives action, selection, link and focus and slightly tints
the neutrals; it is an axis SEPARATE from the mode: two colors times three modes (light, translucent,
dark), plus the system dark mode. The states (success, warning, danger) do not change with the
color and “info” is still blue.

In the stylesheet the color is `data-color` on the root, next to `data-theme`: no attribute,
petrol; `data-color="cobalto"`, cobalt. Their blocks go at the end of `colors.css` and only
carry what changes.

**Who chooses.** Each person, in their account menu, next to the mode: swatch and name of
each color. Whoever has not chosen sees their organization's, which the administrator sets in
Sytue Admin («Color de la organización», Organization color); and with neither of the two, petrol. The first
option of the picker is the organization itself: choosing it means forgetting the choice and going back
to following it, and it works the same for color and for shape (a personal account does not have one:
there is nobody to set anything for it). The choice
lives where the mode's does —a cosmetic cookie on the apex (`sytue-color`) and local
storage— and is applied by the same first-paint script, so it does not flicker. The swatch of
each color is a fixed token of the stylesheet, `--sytue-swatch-<color>`, and not the primary currently
in force: the picker also has to show the color that is not painted.

**“Each app's own”.** The third option of the picker ([ADR 0072](decisions.md#0072)): the
rail and the action take the product color of the app in use; the Hub and My
account stay in petrol. Each app contributes only its primary ramp —generated from
`--sytue-product-<app>`— and the per-mode distribution is common: 600 with white label, 700 for
the rail and the link, 400 for the action in dark and 300 for the link in dark. The
rest of the palette (neutrals, states) does not change. Docus, Calcus, Diapos and
CertiPDF share `/docs` and still each paints its own: its layout states the product
in `<html data-app>` from the server.

Five apps —Mail, PDF, Cast, Diapos and ERP— **only tint the rail**: button, link,
selection and focus stay on the house primary. They are the apps whose color gets confused
with danger in badges, links and focus, and it is a list decided by looking at each one,
not a formula: Bases, which by hue sits between Diapos and ERP, is not confused and gets
rail and action. What is measured is who goes through that decision: an app whose color falls
less than 26° from the hue (OKLCH) of `--sytue-danger` has to be listed in one of the
guard's two lists, or it fails. The rail and the frame paint with their own ramp,
`--sytue-rail-*`, which is the primary's except in those five.

| Role                  | Petrol light | Petrol dark | Cobalt light | Cobalt dark |
| --------------------- | ------------ | ----------- | ------------ | ----------- |
| Action (`primary`)    | `#0f7680`    | `#3cc4cc`   | `#3b5fe0`    | `#4169e8`   |
| Label on the action   | `#ffffff`    | `#04161a`   | `#ffffff`    | `#ffffff`   |
| Link (`primary-text`) | `#0c636b`    | `#7fd3d8`   | `#2f4fc4`    | `#a3b9ff`   |
| Focus (`focus-ring`)  | `#0f7680`    | `#7fd3d8`   | `#3b5fe0`    | `#a3b9ff`   |
| Canvas (`bg`)         | `#f1f6f6`    | `#0a1011`   | `#f3f5fa`    | `#0e111a`   |
| Surface (`surface`)   | `#ffffff`    | `#131c1e`   | `#ffffff`    | `#161b28`   |
| Text (`text`)         | `#0f1d20`    | `#e7efef`   | `#141a2b`    | `#e9ecf5`   |
| Rail (web)            | `#0c5f68`    | `#0d2a2e`   | `#3355d6`    | `#141d3d`   |

**Rules**

1. **`on-primary` is not “white”.** It is the label of the PRIMARY fill, and it depends on the
   color and the mode: in petrol dark it is almost black (the primary is a light turquoise);
   in cobalt it is white in dark as well. On the primary,
   `bg-primary text-on-primary`; on any other fill —danger, success, a person's or
   product's color, the fixed 600 step—, `text-white`.
2. **“Info” is blue.** It is not an alias of the primary: with cobalt it matches in hue and with
   petrol it does not, and an informational notice cannot change meaning with the color.
3. **What lives on the chrome uses the chrome tokens:** states with
   `bg-chrome-hover`/`bg-chrome-active`, borders with `border-chrome-border`, text with
   `text-header-fg`/`text-sidebar-fg` and focus with `sytue-focus-ring-chrome`. Literal white
   veils (`bg-white/10`) only on surfaces that are dark in every
   theme: log viewer, terminal, video room, attachment viewer.
4. **Typography:** Figtree (SIL OFL) for the 24 Latin-alphabet languages and Inter
   (OFL) for Greek and Cyrillic, self-hosted from `@fontsource-variable` and imported
   only once in `@sytue/ui/styles.css`. Small caps (`sytue-overline`) at `0.06em`.
5. **No filler decoration:** no emojis in the interface or in the copy, no
   gradients, glows or blobs beyond the ambient background of the translucent mode.
6. **A new color is one more block, not one more stylesheet.** It declares what changes relative
   to petrol and, in each mode, repeats what its light block declares (at equal selector
   weight the light one would win, since it comes later). `contraste-paleta.test.ts` guards it.

<a id="forma-marco"></a>

**Shape is an axis separate from mode and color.** It lives in `data-forma`, on the root,
next to `data-theme` and `data-color`: **with no attribute you see “Marco”** (frame), which is the default
shape of the web, and `data-forma="carril"` (rail) is the one each person can choose. It follows the
same path as color (`@sytue/ui`, `theme/init.ts`, `theme/preference.ts` and
`ThemeProvider`): `THEME_FORMAS` is the list, `readThemeForma()` reads it with the common
reader (`readThemePreference`) from the suite domain cookie and from `localStorage`
(key `sytue-forma`), `applyThemeForma()` sets or removes the attribute, and the first-paint
script (`themeInitScript`) applies the three axes before painting. `useTheme()` provides `forma`
and `setForma`, and the picker is in the account menu. **Whoever has not chosen sees their
organization's shape**, which the administrator sets in Sytue Admin: it is a column of
`organizations` in Sytue ID (`forma`, migration 0108), it travels in the id_token as
`org_forma` only if it is not the house one, and the BFF puts it in the `sytue-forma-org` cookie,
which is what the first paint reads. It is the same path as color, with shared
plumbing: a single handler (`handleAdminPutApariencia`) and a single save
(`GuardarApariencia`) in Sytue ID, a single route (`api/admin/org/[eje]`) and a single
card (`OrgAppearanceCard`) in the console.

<!-- Marker read by apps/frontend/packages/config/src/tokens-coherentes.test.ts (do not remove): No hay dos juegos de componentes. -->

There are no two sets of components. `Button`, `Card`, `Sidebar`, `Table`, `List`, `TopBar` and
the shells (`AppShell`, `AdminShell`) read **shape tokens**, which the attribute
reassigns; the measurements are in `theme.css` («Tokens de forma», Shape tokens) and what paints, in the
shape block of `appearance.css`, always with role tokens.

| Token                                              | Marco         | Carril      | What it governs                           |
| -------------------------------------------------- | ------------- | ----------- | ----------------------------------------- |
| `--radius-control` (`rounded-control`)             | 4 px          | 4 px        | Buttons, bar buttons                      |
| `--radius-nav` (`rounded-nav`)                     | 999 px        | 9 px        | Side menu items                           |
| `--radius-card` (`rounded-card`)                   | 16 px         | 14 px       | Cards                                     |
| `--radius-row`                                     | 10 px         | 0           | Selected or hovered list row              |
| `--radius-sheet`                                   | 16 px         | 0           | The content sheet                         |
| `--sytue-sheet-gap`                                | 10 px         | 0           | Sheet spacing (0 below 768 px)            |
| `--sytue-nav-pad`                                  | 14 px         | 10 px       | Horizontal padding of the menu item       |
| `--sytue-nav-indicator`                            | 0             | 3 px        | “You are here” bar                        |
| `--sytue-nav-active-weight`                        | 650           | 500         | Weight of the active item                 |
| `--sytue-rail-indicator`                           | 3 px          | 0           | Active bar of the icon rail               |
| `--sytue-rail-item-bg` / `-fill`                   | no background | veil, amber | Idle icon of the icon rail                |
| `--sytue-rail-active-bg` / `-fg`                   | veil          | solid       | Active icon of the icon rail              |
| `--sytue-nav-active-bg` / `-fg`                    | veil          | veil        | Active item (see below)                   |
| `--sytue-row-grow`                                 | 1 px          | 0           | Extra row padding, top and bottom         |
| `--sytue-row-inset`                                | 6 px          | 0           | Side inset of the list row                |
| `--sytue-row-bar`                                  | 0             | 2 px        | Bar of the selected list row              |
| `--sytue-title-weight` / `--sytue-subtitle-weight` | 800 / 700     | 700 / 600   | `sytue-h1`–`h2` / `h3`–`h4`, in the sheet |
| `--sytue-title-scale`                              | 1.1           | 1           | Title font size, in the sheet             |

<!-- End marker for tokens-coherentes.test.ts: **Forma «Marco»** -->

**“Marco” shape** (the one you see without choosing anything):

1. **The top bar and the side menu are a continuous color frame** (`--sytue-frame-*`:
   the side menu color; in translucent mode, a horizontal gradient of the primary, from
   its 900 step to 700). The whole bar is chrome at any width:
   `TopBar rail` carries `sytue-topbar-carril` —the class that below 768 px already makes it a rail
   in both shapes—, which enters the same token reassignment as the rail, with no
   bottom line. The search box is the neutral veil `--sytue-sidebar-field` with the chrome
   text.
2. **The content is a sheet** (`sytue-sheet`) with radius 16 that floats inside with an `md`
   shadow, 10 px away from the right and bottom edges (and from the left one if there is no menu).
   In translucent mode it is glass: `surface-raised` at 82 % over the frame. The root of the
   shell (`sytue-frame`) paints the frame. Pages that scroll the document (the
   Hub) use two wrappers, `sytue-frame-page` and `sytue-sheet-page`, which in Carril are
   `display: contents`.
   **The sheet does not clip** (decided on 2026-10-05; `contraste-paleta.test.ts` guards it).
   It used to have `overflow: clip` for the corners, and a box that clips leaves out of reach whatever
   it does not contain: in v3.11.1 the Docs home page lost the document list on a
   phone. The corners are rounded by **what is inside**: the sheet publishes its four radii
   in `--sytue-sheet-{tl,tr,br,bl}` and each piece that touches a corner carries
   `sytue-sheet-corners`; one that only touches some sets the others to `0px`
   (`[--sytue-sheet-tl:0px]`), and whatever it contains inherits it. In a scrolling panel,
   the radius also clips its content. The common shells (`AppShell`, `AdminShell`)
   already do it for all their apps; a screen with its own shell that paints a background
   up to the sheet's corner has to mark that piece.
3. **Pill-shaped buttons and navigation.** The active menu item is a light veil pill
   with white text (`--sytue-nav-active-*`, the same values as in Carril:
   `--sytue-sidebar-active-bg`/`-fg`), with no indicator bar. The solid pill —white with
   the label in the color; in dark, the light primary with a dark label— belongs only to the
   menu's main button («Redactar», «Nuevo» — Compose, New): with the two identical, one above the other,
   the button could not be told apart (decided on 2026-10-05). And it is **bold**
   (`--sytue-nav-active-weight`, 650): the veil alone does not reach the 3:1 of an indicator, and
   without an indicator bar the weight is what says “you are here”.
4. **Taller rows** (1 px more at the top and bottom in `Table` and `List`); in `List`, the
   selected or hovered row is a radius-10 block 6 px away from the edges, with no
   bar on the left. **Titles** 10 % larger and one step bolder, only
   inside the sheet: the public site and the signed-out pages share those classes.
5. **Full-color resource cards:** a `Card` with `tone` —or any element
   with `cardTone(tono)`, which spreads the same class and the two variables between its `class`
   and its `style`— is filled with the group color and its `CardIcon` becomes a veil of the
   fill's ink. Inside a `sytue-tiles` container those elements are
   **tiles**: a grid, with the icon on top and the label below. Today: the Hub's quick
   links (rows with an icon in Carril, tiles in Marco), the Drive
   folders and the Cloud project cards. In Drive, the grid view
   paints them in place and the **list** view pulls them up top, as a strip of low
   tiles (icon, name and type) above the file list: two pieces on screen and
   a single series of positions for range selection (`apps/drive`,
   `lib/teselas.ts`); in Carril, in the grid and in a search there is no strip. A `Card` without `tone`
   is a panel and stays white. The label on each fill is measured by the guard
   («Tarjeta: rótulo sobre su color», Card: label on its color, ≥ 4.5 in all nine tones).
6. **Where the sheet goes full-bleed** (measured with Playwright, 2026-10-04):
   - **Below 768 px, always.** Floating, it took 16 of the 390 px of width
     (4.1 %) and 8 of height. Full-bleed it takes nothing and keeps the two top corners,
     which is what still reads as a sheet under the color bar.
   - **In the editors** (Docus, Calcus, Diapos: the whole Sytue Docs shell, with its
     home page). Floating, it took 20 px of width and 10 of height from the canvas at 1280×800. It uses
     `sytue-sheet-bleed`: no gap and only the top corners.
   - The Sytue Web editor is a full-screen editor, without the suite bar:
     it has neither frame nor sheet. Its list and settings screens do (they mount `AppShell`).
   - The ERP has its own shell and carries a sheet like the others: its root is `sytue-frame`
     and its `main` goes inside `sytue-sheet`. Its **icon rail** is part of the frame
     and follows the `--sytue-rail-*` tokens: in Marco, a white icon with no background when idle and,
     on the active one, a veil and a 3 px **amber bar** at full opacity on the edge of the
     rail (`--sytue-rail-indicator`), which is what says “you are here” where there is no
     label to make bold: the icon fill is painted at 34 % and does not reach 3:1; in Carril, a veil when idle and a solid
     square on the active one, as always. The module drawer uses the menu tokens.
7. **In print there is no shape:** everything that paints the frame and the sheet goes in `@media screen`.
8. **The native apps (Fyne) carry the same shape, read from the same stylesheets:** they embed
   a copy of `theme.css` and of `appearance.css` and ask for each measurement by its token, so
   a change in the table above reaches them without touching Go. How they paint the sheet without
   corner clipping, and what goes full-bleed, in
   [CONVENTIONS §13.6](CONVENTIONS.md#13-6-la-interfaz-nativa-es-fyne-con-todo-propio--normativo-2026-09-21).
9. **The Hub, “Welcome and dashboard”** (proposal B, 2026-10-05): a greeting by local time
   («Buenos días / Buenas tardes / Buenas noches, <nombre>» — Good morning / Good afternoon / Good evening, <name>; keys
   `launcher.greeting.*` of the portal), the day's actions as pills —four of the
   quick links, the first one as primary, and a «Más» (More) with the rest; search is already
   in the bar—, the apps as compact tiles (42 px product
   icon on the left, 62 px tall, three columns from 1024 px) on
   two thirds of the width and, beside them, «Hoy» (Today) and «Reciente» (Recent) (its last three entries);
   with six apps or fewer, «Reciente» moves down to the apps column. In Carril, the usual
   home page. It is a single markup: what one shape shows and the other does not goes with the
   `marco:` and `carril:` variants of `theme.css`, not with a branch in the component, which
   would flicker on hydration because the shape is not known on the server.

**“Carril” shape** (the alternative you choose; where the color lives and what relief the content has):

1. **The brand color lives in the side menu**, in the chosen color from top to bottom. White text,
   idle items dimmed, the active one with a white veil and an **amber**
   (`#ffd45e`) “you are here” bar. Outside the rail the indicator is the primary. The rail
   values live in `appearance.css`, which is loaded by the web (after `colors.css`) and
   by the native apps (`apps/nucleo/tokens` reads it on top, in the same order): both
   paint the same rail. In the native apps there is no cascade, so each menu piece
   asks for the rail token by name (`docs/CONVENTIONS.md` §13.6).
2. **The top bar is light.** It runs from side to side above the menu, so the
   shape is achieved without moving the DOM: its brand area (`TopBar rail`, the app
   switcher) is painted in the rail color and, from 768 px, with the width of the
   side menu, and reads as its header. In an app without a side menu it stays as a
   brand corner, fitted to the brand and the app switcher: it does not paint the width
   of a menu that does not exist, but its box still reserves it and the search box does not move
   when going from one app to another (`.sytue-esquina-marca`). **Below 768 px** the side menu
   goes into a drawer and is not visible: the WHOLE bar is the rail (`.sytue-topbar-carril`), with
   white text and the search box in a neutral dark veil (`--sytue-sidebar-field`), which
   works with any rail color.
3. **Inside the rail the chrome tokens are different.** `bg-sidebar` and `bg-rail`
   reassign in their subtree the `chrome-*` family, the indicator and the glyph
   fill (amber), the focus (white) and the main button (white with a label in the color:
   one in the same color would not be visible). Whatever opens inside with its own surface —the
   app switcher, a menu, a dialog, a white card— goes back to the
   theme values. The rule and its rationale are in `branding/tokens/appearance.css`.
4. **Clean content:** a greenish-gray canvas and white surfaces that lift off with a
   soft shadow. Radius 9 for controls and 14 for cards, panels, menus and dialogs.
5. **Resource cards:** a white surface with the group color in an ICON on a
   solid fill (`CardIcon`), instead of a pastel background.
6. **What rests on the rail with its own color is separated from it.** A color
   swatch —a label's dot, a calendar's checkbox, a presence dot—
   carries a ring in the rail's text color (`ring-sidebar-fg ring-[1.5px]`): it may
   be the same hue as the rail, and the ring is the only thing that distinguishes it. The
   organization logo ALWAYS goes on a rounded white plate, like the house brand. And
   in translucent mode, the menu as a drawer (below 768 px, and the ERP module drawer) is opaque:
   underneath it there is not the ambient background but content, and its text bled through. The account
   avatar and the bell's notification dot carry the same ring when the top
   bar is colored (`.sytue-aro-carril`: in “Marco” always; in “Carril”, below
   768 px): danger red does not reach 3:1 on the rail. With “reduce transparency”, the
   translucent rail is opaque on desktop too (in “Marco” the frame already is). And whatever is
   **dimmed** inside the rail —the muted Rooms channel— is dimmed with the
   “idle item” tone and one weight lighter, never with opacity: there is no margin below.

**Measured contrast** (WCAG 2.x) on the real stylesheet, not on a mock-up: it is computed by
`apps/frontend/packages/config/src/contraste-paleta.test.ts`, which reads `colors.css` and
`appearance.css`, applies the blocks like the cascade, flattens veils and glass over what
is underneath them and fails if a measurement drops below its minimum, in both colors times the
four modes. In translucent mode each measurement is the WORST among the three gradient
stops and the warm light. It is reproduced with `MEDIDAS=1 bun test contraste-paleta` from
`packages/config`.

| Measurement                                   | Min. | Petrol light | Petrol translucent | Petrol dark | Cobalt light | Cobalt translucent | Cobalt dark |
| --------------------------------------------- | ---- | ------------ | ------------------ | ----------- | ------------ | ------------------ | ----------- |
| Text on surface                               | 4.5  | 17.26        | 14.98              | 14.84       | 17.32        | 14.63              | 14.56       |
| Dimmed text on surface                        | 4.5  | 7.82         | 8.58               | 8.40        | 8.08         | 8.23               | 8.17        |
| Subtle text on surface                        | 4.5  | 5.67         | 7.00               | 6.17        | 5.76         | 6.82               | 5.94        |
| Subtle text on the canvas                     | 4.5  | 5.20         | 5.20               | 6.84        | 5.28         | 4.76               | 6.51        |
| Text on selected row                          | 4.5  | 14.62        | 12.14              | 10.92       | 14.88        | 12.04              | 11.53       |
| Link on surface                               | 4.5  | 6.97         | 8.64               | 10.07       | 6.92         | 7.26               | 8.96        |
| Main button label                             | 4.5  | 5.35         | 5.35               | 8.78        | 5.39         | 5.39               | 4.76        |
| Main button against the surface               | 3    | 5.35         | 4.65               | 8.21        | 5.39         | 4.55               | 3.61        |
| Rail: active item                             | 4.5  | 5.28         | 5.16               | 10.46       | 4.81         | 4.77               | 11.67       |
| Rail: idle item                               | 4.5  | 5.72         | 6.02               | 8.06        | 5.00         | 4.99               | 8.94        |
| Rail: main button against the rail            | 3    | 7.36         | 7.23               | 7.18        | 6.16         | 6.13               | 3.47        |
| Rail: main button label                       | 4.5  | 7.36         | 7.36               | 8.78        | 6.16         | 6.16               | 4.76        |
| Rail: focus ring                              | 3    | 7.36         | 7.23               | 15.13       | 6.16         | 6.13               | 16.50       |
| Rail: “you are here” indicator                | 3    | 3.73         | 3.65               | 7.39        | 3.40         | 3.37               | 8.24        |
| Rail: search text on mobile                   | 4.5  | 10.77        | 10.63              | 14.72       | 9.47         | 9.43               | 15.30       |
| Rail: search placeholder (mobile)             | 4.5  | 8.10         | 8.63               | 8.92        | 7.46         | 7.44               | 9.60        |
| Rail: ring of color swatches                  | 3    | 7.36         | 7.23               | 12.96       | 6.16         | 6.13               | 13.97       |
| Rail: counter of a muted channel              | 4.5  | 6.24         | 6.10               | 10.87       | 5.45         | 5.41               | 11.89       |
| Rail: ring of the notification dot and avatar | 3    | 7.36         | 7.23               | 12.96       | 6.16         | 6.13               | 13.97       |
| Top bar: text                                 | 4.5  | 17.26        | 13.97              | 15.95       | 17.32        | 13.47              | 15.28       |
| Top bar: search placeholder                   | 4.5  | 5.67         | 7.52               | 6.17        | 5.76         | 7.43               | 5.94        |
| Card: label on its color                      | 4.5  | 4.96         | 4.96               | 4.56        | 4.68         | 4.68               | 4.56        |
| Control outline                               | 3    | 4.55         | 4.62               | 3.65        | 4.57         | 4.59               | 3.61        |
| Focus ring                                    | 3    | 5.35         | 6.05               | 10.07       | 5.39         | 5.84               | 8.96        |
| “You are here” indicator outside the rail     | 3    | 5.35         | 6.05               | 8.21        | 5.39         | 5.84               | 5.67        |

**Outside this step:** the brand —the logotype and the per-organization color wedge stay
as they are—, the product and person colors, and the document tables and
the canvas of sites created with Sytue Web, which keep their own styles.

---

<a id="superficies-claras-multicolor"></a>

<a id="102-tres-apariencias-compartidas-2026-10-04"></a>

### 1.0.2 Three shared appearances (2026-10-04)

The suite offers light, translucent and dark, as well as following the system appearance.
Light uses a greenish-gray canvas with white surfaces; translucent puts white
veils over a broad background in the color with a warm light; dark uses an almost
black with a hint of green and stepped surfaces. States are still expressed
with text and icons. Actions and quantity controls use the chosen color in all
three appearances, with the label set by `on-primary`: white in light and translucent,
almost black in dark (§1.0.1).

Cards have nine shared families: blue, violet, green, pink,
yellow, cyan, coral, lime and graphite. Each family has a solid fill and a
tinted one adapted to the theme: light, translucent or dark. The text of the solid cards
switches between white and ink according to its contrast. Color identifies resources and
groups content; it does not replace status labels or the primary of actions.

The [composition reference](https://cdn.dribbble.com/userupload/15998211/file/original-b94dc14825242b117649e09e0ab66543.png)
guides the layers, the hierarchy and the spacing between groups. Figtree, already self-hosted in the
suite, accompanies those geometric shapes without bringing in Lufga or any other commercial font.
The notices and the SIL OFL license of the distributed fonts are preserved.

Cards have radii of 16–20 px and the side menu 4–8 px. Separators
keep space before and after. The POS uses compact navigation and keeps all
available models in a drawer with focus management. Its images fill the background
of the card and the editing controls appear only when Images is enabled.

Large forms can use the right side panel of `Modal`, with the same
focus management, Escape and return to the trigger. Popovers use a blur of
24–28 px, with a solid surface when it is not available or reduced transparency
is preferred. Openings use 150–180 ms motions and respect reduced
motion. Applying blur repeatedly to every card in the catalog is avoided.

The palette originates in `branding/tokens/colors.css` and `colors.json`, vendored for the
web, Sytue ID and the native applications. The radii come from
`branding/tokens/spacing.json`. The web root is 16 px: `text-base` measures 14 px and rem
matches the native geometry. The current brand values are in §1.0.1. The
document tables and the canvas of sites created with Sytue Web keep their
own styles, as do each organization's logos.

<a id="11-concepto-y-personalidad-de-marca"></a>

### 1.1 Brand concept and personality

Sytue is the **European, sovereign and free** productivity suite. The visual direction
exists to convey three things, in this order:

1. **Trust.** This holds your email, your documents, your company. It must look
   stable, precise and incorruptible. Trust is communicated through **sobriety and
   consistency**, not decoration. Zero elements that shout.
2. **Clarity.** The user always knows where they are, what they can do and what just
   happened. Hierarchy without ambiguity. The design disappears; the work remains.
3. **European sovereignty.** European origin as quiet pride, not as a waving
   flag. It is expressed through **rigor** (Central European precision, honest typography,
   nothing superfluous) rather than flag icons. When the flag appears (footer,
   login), it is a sober detail, never an ornament.

**Personality (operational adjectives):** precise · serene · dense · honest · timeless.
**Anti-personality (what we NEVER are):** playful, noisy, "friendly startup",
maximalist, trendy, generic.

**Archetype:** the precision instrument. A good mechanical keyboard, a Braun by Rams,
a Linear panel. Invisible while you work, satisfying when you notice it.

---

<a id="12-moodboard-en-palabras"></a>

### 1.2 Moodboard in words

- **Surface:** warm paper. Slightly creamy ivory (`#faf9f6`), not cold or bluish
  white. Warm grays (greige) that feel like cotton paper, not metallic.
- **Depth:** almost flat. Elevation is a whisper — a 1px border and a shadow
  you can barely perceive. Things float a millimeter, not a centimeter.
- **Color:** severe chromatic discipline. 95% of the screen is neutral (ivory +
  graphite); **brass** appears only where there is action or brand. Color is **expensive**: it
  is spent with intent, like a brass detail on fine wood.
- **Density:** a control panel, not a landing page. Lots of legible information in little
  space, breathing just enough. A Bloomberg-terminal-made-human style.
- **Rhythm:** a strict 4px grid. Everything aligns. The feeling of quality comes from
  nothing being 3px off from where it should be.
- **Motion:** curtains that open quickly and firmly. 120–200ms, never bounces or
  sways. Motion confirms, it does not entertain.
- **Dark mode:** it is not "light mode inverted". It is a study paneled in graphite —
  deep warm graphites (`#131619`), surfaces distinguished by 1–2 steps of
  gray, the brass shining just enough to guide the eye.

**Level references (a bar, not a mold):**

- **Linear** — density, purposeful motion, hierarchy through weight and color, premium
  darkness.
- **Vercel / Geist** — monochrome sobriety, surgical use of the accent, typography as
  structure.
- **Raycast** — command and speed, minimal elevated surfaces, focus on content.
- **Mattermost** — the density of a real work tool, a brand sidebar, endless legible
  lists.

From each we take the **principle**, not the style. We do not copy their blue, their radius or their
illustration. The Sytue identity is our own and today it is that of §1.0.1: neutrals with a hint of
its hue, a selectable accent (petrol or cobalt), rounded-tile geometry and Figtree. The
“Graphite & Brass” values remaining in this moodboard (ivory, graphite, brass) are
history: read them as “canvas”, “neutral” and “primary”.

---

<a id="13-principios-visuales"></a>

### 1.3 Visual principles

<a id="31-jerarquía"></a>

#### 3.1 Hierarchy

Hierarchy is built, in this order of preference:
**1) font weight → 2) text color (text / muted / subtle) → 3) spacing → 4) size → 5) (last resort) elevation/shadow.**
A large shadow or a colored background is never used to make something "stand out" if a
`font-weight: 600` and a little air solve it. A well-structured view reads
in gray; color only marks the action.

<a id="32-densidad"></a>

#### 3.2 Density

**Medium-high** density, on purpose. 14px typographic base. Goal: fit more without
cramming. Compact list rows (~32–40px), 12px inner padding, 16px container,
24px section. Density is a brand value — we are a work tool, not
a brochure. But density ≠ cramped: it always breathes enough to scan.

<a id="33-profundidad-y-elevación"></a>

#### 3.3 Depth and elevation

**Minimal and semantic** elevation. Almost everything lives on the base plane with a 1px border.
Only what truly and ephemerally floats is elevated:

- base (content, cards at rest): `1px` border, no shadow or `xs`.
- dropdown / popover: `md`.
- modal / dialog: `lg`.
- page overlay: `xl`.
  Rule: **if it does not open on top of something else, it has no shadow.** The border is the
  primary means of separation; the shadow is only for "this is on top right
  now". In dark mode the `shadows.dark` variants are used (more contrast, less diffusion).

<a id="34-uso-del-color-de-acento"></a>

#### 3.4 Use of the accent color

- **Primary (petrol or cobalt):** main action, brand, links, focus, selected/active
  state. It is the only color that triggers "you can act here". As a fill
  with white text it uses 600; as text on white, 700; in dark the fill is 400 or
  500 depending on the color, with the label set by `on-primary`, and text, 300. It is not used as decoration for large backgrounds.
- **Marine ink (secondary):** the `accent` family is reserved for charts and data. It is not
  an action accent.
- **Violet `#7c5cff` (tertiary):** exclusively for data/product differentiation
  (charts, Calendar, Meet, labels). Never as a UI action color.
- **Semantic** (success/warning/danger/info): only for state meaning, never
  decorative. And **never by color alone**: always accompanied by an icon or text.
- **Product colors** (`colors.json → product`): they identify each app **only in the
  app-switcher tile** (orientation between apps). The chrome is the same in all of them; the
  product color does not tint the header or the work UI.

Reference color budget: on a typical screen, ≤ ~10% of pixels in brand/accent
color. If there is more, there is too much color.

<a id="35-tipografía"></a>

#### 3.5 Typography

**Inter** for the whole product (open font, SIL OFL — consistent with the free-software value);
**JetBrains Mono** for code, PaaS logs and SQL. A single text family across the whole
suite: typographic consistency is the biggest multiplier of "this is a single
product".

- **Weight as the main hierarchy tool:** 400 body, 500 soft emphasis,
  600 titles/labels/buttons, 700 only display/H1–H2.
- **Negative tracking (`-0.01em`) on titles** (display, H1, H2) so they feel
  compact and modern; normal on body; `+0.04em` on uppercase (overline).
- **Numbers:** use tabular figures in tables, data lists and metrics so that
  columns line up.
- Modular scale ~1.2 over 14px. Apps consume **roles** (`display`, `h1`–`h4`,
  `body`, `label`, `caption`, `overline`, `button`, `code`), never loose sizes.
- Generous line height on body (~1.5) for dense legibility; tight on titles.

<a id="36-ritmo-y-espaciado"></a>

#### 3.6 Rhythm and spacing

**4px** base grid, no exceptions. Every padding, gap and margin is a multiple of the
scale. Perceived quality is born from millimetric alignment. Content at max.
**1280px** with a 24px gutter. The layout system is fixed and recognizable in all
apps: **TopBar 56px + Sidebar 260px (or 64px app rail) + content**, with a secondary
panel for inboxes (Mail, Chat). The layout is not reinvented per app.

#### 3.7 Motion

Motion **with purpose, fast and sober.**

- Duration: `120ms` (micro: hover, focus), `150ms` (base: most things), `240ms` (overlay/modal
  entrances). Nothing above 240ms in UI.
- Easing: `cubic-bezier(0.2, 0, 0, 1)` (firm and decisive entrance) for almost everything;
  `cubic-bezier(0.4, 0, 1, 1)` for exits.
- **Forbidden:** bounce, sway, exaggerated spring, decorative parallax,
  distracting auto-animations. Motion **confirms a state change** (something
  opened, was selected, arrived), it never decorates.
- **Always** respect `prefers-reduced-motion`: no non-essential transitions.

<a id="38-guía-no-formulario"></a>

#### 3.8 Guide, not form

Configuration is **product**, not paperwork. Reference: **Apple** (it takes you by the hand,
one step and one idea at a time), **Vercel** and **DigitalOcean** (clean surfaces that explain
the consequence of each option). Explicit anti-reference: the console density of **Google
Cloud** and **AWS**, and the 20-field “form wall” with no context.

- A **basic** user must be able to configure anything **without reading documentation**.
- Plain language oriented to the consequence (“Who does this rule affect?”), never the
  database field name.
- Every piece of data that comes from another system says **where to find it**; whatever can be tested
  is tested **before** saving; whatever is going to be applied is **summarized** before applying it.
- It applies to the **whole** suite —Console, each product's settings, the organization's and the
  account's—, to new things as well as existing ones.

It is a **permanent** requirement; the rules and the mandatory primitives are in
`docs/design.md#2-sistema-de-diseno` §8.

---

<a id="14-decisiones-marco-para-los-especialistas"></a>

### 1.4 Framework decisions (for the specialists)

Directions, not re-derivation of values. The final numbers already live in
`branding/tokens/`; here we set the judgment that governs them.

<a id="41-paleta--dirección"></a>

#### 4.1 Palette — direction

- **Neutral base with a hint of green:** canvas `#f1f5f3` and white surfaces in
  light, `#0a0f0d`–`#1a2520` in dark. Grays lean toward the brand hue, never toward
  cold blue.
- **One accent, selectable between two:** petrol (`#0f7680`) or cobalt (`#3b5fe0`), the only action soloist.
- **Full 50→950 scales** for primary, accent (marine ink) and
  neutral, calibrated for **WCAG AA** contrast on light and dark surfaces.
  Any new text/background pair is validated at ≥ 4.5:1 (text) / ≥ 3:1 (UI and large
  text), and the canonical pairs are guarded by `contraste-paleta.test.ts` (§1.0.1).
- **Color specialist:** works within these scales; if a shade is missing, it is
  added to the token, a HEX is not invented in an app. Dark mode is **not** an inversion: it is
  a reassignment of roles designed separately (see `colors.json → theme.dark`).

<a id="42-tipografía--dirección"></a>

#### 4.2 Typography — direction

- **Single family Inter** across the whole product; **JetBrains Mono** only for technical content. No
  other family is introduced on product surfaces.
- **Modular scale ~1.2 over a 14px base** (medium-high density). Hierarchy by weight
  before size.
- **Canonical roles** as the usage interface (apps ask for `h3`, not "18px semibold").
- Negative tracking on display/titles; tabular figures in data.

<a id="43-radios-sombras-y-elevación--dirección"></a>

#### 4.3 Radii, shadows and elevation — direction

- **Tight, restrained radii:** `5/7/9` for 90% of the UI (inputs, buttons,
  cards, modals), `12/16` only for large surfaces, `pill` for chips/badges. The
  radius family echoes the `rx: 9` of the brand tile — formal consistency between icon
  and component. **No fully square corners nor exaggeratedly rounded ones**;
  the professional middle ground, a touch tighter (Graphite refinement).
- **Very subtle shadows**, `xs→xl` scale, reserved for what floats (see 3.3). Low
  diffusion, low opacity in light; more contrast and less blur in dark.
- **Semantic elevation**, not decorative: each level corresponds to a role
  (base/dropdown/modal/overlay), not to "making it pretty".
- **Focus:** **a single** 2 px ring in the `--sytue-focus-ring` color (the primary), which
  contrasts ≥ 3:1 on its own over white, canvas and sunken. **Always visible** via
  keyboard; never `outline: none` without a replacement. Focus is a first-class
  design element, not a leftover.

<a id="44-iconografía--dirección"></a>

#### 4.4 Iconography — direction

Two systems, same geometric DNA, do not mix:

- **Product icons** (`branding/icons/*.svg`): the "app logo". Rounded tile
  **40×40, rx 9**, white glyph on the product color (`colors.json → product`).
  White stroke **~2.2–3px**, `stroke-linecap="round"`, `stroke-linejoin="round"`.
  Clean geometry recognizable at 24px. They live in the app switcher, app avatars and
  large favicons. They share grid and family with the brand symbol (same tile, same
  S-swoosh as formal root). A new product icon is drawn on this same
  mold: same tile, same weight, same air.
- **UI icons** (inside components: chevrons, trash, search, etc.):
  **geometric duotone glyphs with accent**, **24×24** grid. A **1.5 px on-screen stroke
  at any size**, slightly rounded corners (linecap/linejoin
  round), and an **accent fill** under the stroke in the closed shapes of the drawing;
  pure line ones (chevrons, cross, checkmark) have no fill. **Consistent
  framing:** each glyph centered at (12,12) with a uniform optical box (~2–3u of
  margin), filling its square equally — none floats small or off-center. The
  stroke inherits `currentColor`, never its own color except for semantics. Lucide/Feather style
  in craft, but our own and consistent with the Sytue geometry. Details in §3.20.

**Iconography rules:** geometric before illustrative; a single weight per system;
consistent `viewBox`; clean SVGs (no editor metadata); optimized. A single style
per surface: duotone applies to ALL interface icons, not to some.

<a id="45-ilustración--dirección"></a>

#### 4.5 Illustration — direction

Illustration is **scarce and in service**, never decorative. It appears almost only in
`EmptyState`, onboarding and error/brand pages.

- **Style:** geometric and linear, built with the same system primitives
  (rounded tiles, strokes of product-icon weight, the S-swoosh as a recurring
  motif). Fine line art on a neutral surface, with **a single touch** of brand
  color (blue or teal) as the focal point. Flat — no stock isometric, long shadows,
  diffuse gradients or "corporate memphis" characters.
- **Illustration palette:** system neutrals + one accent. Works in light and
  dark (uses roles, not HEX).
- **Tone:** serene and useful. An empty state illustrates the _empty concept_ (an inbox, a
  folder), not a joke. It always accompanies title + description + CTA.
- **When NOT to illustrate:** in dense work UI. There, a system icon and good
  spacing are enough. Illustration that adds nothing is removed.

---

<a id="15-aplicacion-a-la-suite-una-sola-app"></a>

<a id="15-aplicación-a-la-suite-una-sola-app"></a>

### 1.5 Application to the suite (a single app)

Mail, Drive, Calendar, Contacts, Docs, Chat/Meet, Cloud, Console and www **look and
behave like a single application**: same shell, same `@sytue/ui` components,
same palette, same typography, same motion patterns. The only differentiation
allowed between apps is the **product color in the app-switcher tile** — never a
header in its own color, a different aesthetic, a different layout or its own
components. If an app "needs to look different", the answer is no.

Brand surfaces (Sytue ID login, sidebar/rail, www, footer) carry the graphite
chrome with the brass accent and may show European sovereignty with sobriety.
Work surfaces are ivory and dense, with color reserved for action.

---

<a id="16-checklist-de-direccion-antes-de-dar-por-bueno-cualquier-diseno"></a>

<a id="16-checklist-de-dirección-antes-de-dar-por-bueno-cualquier-diseño"></a>

### 1.6 Direction checklist (before approving any design)

- [ ] Does the hierarchy read **in grayscale**? (if it depends on color, it is wrong)
- [ ] Does the brand color take up ≤ ~10% and only mark action/brand/state?
- [ ] Is everything on the 4px grid and aligned?
- [ ] Is elevation semantic (only what opens on top floats)?
- [ ] Visible focus on everything focusable? AA contrast verified?
- [ ] Does it work identically in light and dark with only roles/tokens (zero loose HEX)?
- [ ] Is motion ≤ 240ms, with no bounces, and does it respect `prefers-reduced-motion`?
- [ ] Does it look like Sytue and like **a single app**, not like a template or like AI?
- [ ] If it **configures** something: is it guided step by step, in plain language, and would
      someone understand it without documentation? (§3.8 · `design.md#2-sistema-de-diseno` §8)
- [ ] Is anything superfluous? (remove it) Is density or clarity lacking? (adjust it)

---

<a id="17-fuentes-de-verdad-y-jerarquia-documental"></a>

<a id="17-fuentes-de-verdad-y-jerarquía-documental"></a>

### 1.7 Sources of truth and document hierarchy

| Layer              | Document / folder                           | What it sets                                       |
| ------------------ | ------------------------------------------- | -------------------------------------------------- |
| **Why / where to** | `docs/design.md#1-direccion-de-arte` (this) | Direction, principles, framework decisions         |
| **Brand**          | `branding/brand-guidelines.md`              | Logos, tone, brand usage, sovereignty              |
| **How (rules)**    | `docs/design.md#2-sistema-de-diseno`        | Layout, components, usage rules, A11y              |
| **Final values**   | `branding/tokens/*`                         | Color, typography, spacing, radii, shadows, motion |
| **Assets**         | `branding/logo/*`, `branding/icons/*`       | Logo and icon SVGs                                 |
| **Implementation** | `@sytue/ui`, `@sytue/shell`                 | Preact components + Tailwind v4 theme              |

In case of conflict: the direction (this doc) sets the judgment; the tokens set the number. If
they diverge, they are aligned — it is never resolved by improvising in an app.

---

<a id="2-sistema-de-diseno"></a>

<a id="2-sistema-de-diseño"></a>

## 2. Design system

> **The bridge to code.** This document translates the art direction
> (`docs/design.md#1-direccion-de-arte`) into **specs implementable without ambiguity** on the
> existing `@sytue/ui` components. Each value maps to a token in
> `branding/tokens/*` (color, typography, spacing/radii/shadows/motion); a HEX, loose px or
> invented duration is **never** written. If a value is missing, it is added
> to the token — nothing is improvised in an app.
>
> **Document hierarchy:** `design.md#1-direccion-de-arte` sets the _why_; **this doc** sets
> the _how_ (usage rules, specs, layout, A11y); `branding/tokens/*` sets the
> _number_; `@sytue/ui` is the implementation. In case of conflict: the token is aligned,
> the app is not patched.
>
> **Scope of the system.** A single aesthetic, typography, palette and component
> library for **all** surfaces (portal/Hub, Mail, Drive, Docs,
> Sheets, Slides, Calendar, Contacts, Chat, Meet, Cloud, Console, www). Apps
> **consume** components from `@sytue/ui` and the shell from `@sytue/shell`; **they do not create
> variants or ad-hoc UI**.

---

<a id="20-como-leer-una-spec"></a>

<a id="20-cómo-leer-una-spec"></a>

### 2.0 How to read a spec

- **All values are tokens.** A value in backticks such as `neutral-150`,
  `radii.md`, `motion.duration.base`, `shadows.md`, `--sytue-surface` references an
  entry in `branding/tokens/*`. The literal is never hard-coded.
- **Roles before HEX.** Components use **role variables**
  (`--sytue-surface`, `--sytue-text`, `--sytue-border`, …) that reassign light/dark
  automatically. Only when the role is not enough is a scale cited (`primary-500`).
- **Dark mode = reassignment, not inversion.** Since components use roles,
  dark support is automatic. Where a component needs a specific adjustment
  (shadows, state opacity, semantic background tint) it is stated in its
  **Dark** block.
- **4px scale, no exceptions.** Every padding/gap/margin is a multiple of
  `spacing.*` (`1`=4px, `2`=8px, `3`=12px, `4`=16px, `6`=24px…).
- **Mandatory states.** Every interactive component declares:
  `default · hover · active · focus-visible · disabled` and, where applicable,
  `loading · selected · error · empty`. None is omitted.

---

<a id="21-fundamentos-resumen-de-tokens"></a>

### 2.1 Foundations (token summary)

Canonical values in `branding/tokens/`. Here, only the quick reference used by the
specs below.

<a id="11-escala-tipográfica-y-roles"></a>

#### 1.1 Type scale and roles

`branding/tokens/typography.json`. Base **14px**, ratio ~1.2, family **Inter**
(`fontFamily.sans`); `JetBrains Mono` (`fontFamily.mono`) only for code/logs/SQL.

| Size   | px  | line-height |     | Weight     | value |
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

**Roles** (usage interface — apps ask for the role, not the size):

| Role       | size / weight / tracking         | Usage                          |
| ---------- | -------------------------------- | ------------------------------ |
| `display`  | 4xl / 700 / tight                | www covers, brand hero         |
| `h1`       | 3xl / 700 / tight                | Page title                     |
| `h2`       | 2xl / 700 / tight                | Major section                  |
| `h3`       | xl / 600 / normal                | Section                        |
| `h4`       | lg / 600 / normal                | Subsection, card title         |
| `body`     | base / 400 / normal              | Default body (14px)            |
| `body-md`  | md / 400 / normal                | Long-read body (Docs, www)     |
| `label`    | sm / 600 / normal                | Field labels, column headers   |
| `caption`  | xs / 400 / normal                | Meta, timestamps, help         |
| `overline` | xs / 600 / caps (+0.04em, UPPER) | Group headers in sidebar/menus |
| `button`   | base / 600 / normal              | Button text                    |
| `code`     | sm / 400 / mono                  | Code, tokens, SQL              |

**Tabular numbers** (`font-variant-numeric: tabular-nums`) are mandatory in Table,
data lists, Console/Cloud metrics and date/size columns.

<a id="12-radios-spacingjson--radii"></a>

#### 1.2 Radii (`spacing.json → radii`)

`sm 4` · `md 9` · `lg 14` · `xl 20` · `2xl 24` · `pill 999` · `full 50%`.
Usage rule: **inputs/buttons/menu items → `md`**, except chips/circular avatars
(`pill`/`full`); **cards/panels/menus → `lg`**; **modals and large surfaces →
`xl`/`2xl`**; `sm` for small things (labels, a link's ring).

<a id="13-elevación-spacingjson--shadows--semántica-no-decorativa"></a>

#### 1.3 Elevation (`spacing.json → shadows`) — semantic, not decorative

| Level          | Light token           | Dark token        | Role                                    |
| -------------- | --------------------- | ----------------- | --------------------------------------- |
| **0 base**     | `none` + `1px border` | same              | Rows, inputs, content on a surface      |
| **1 subtle**   | `shadows.sm`          | `shadows.dark.sm` | Card and panel: the white surface that  |
|                |                       |                   | lifts off the canvas                    |
| **2 dropdown** | `shadows.md`          | `shadows.dark.md` | Dropdown, open Select, Popover, Tooltip |
| **3 modal**    | `shadows.lg`          | `shadows.dark.lg` | Modal/Dialog, Toast                     |
| **4 overlay**  | `shadows.xl`          | `shadows.dark.xl` | Command palette, full-screen sheets     |

**Rule:** within a surface the `1px` border (`--sytue-border`) separates, not the
shadow; the `sm` shadow belongs to the whole surface (card, panel), which lifts off the
canvas, and from `md` upward, to what opens _on top_. Shadows are tinted with the text ink
(cold, almost black ink), not with black; `sm` and `md` have two layers, a short one that
draws the edge and a wide one. In dark mode the `shadows.dark.*` variants are used (black,
more alpha).

<a id="14-foco"></a>

#### 1.4 Focus

**A single ring:** `0 0 0 2px` of `--sytue-focus-ring` —primary 600 in light, 700 in
translucent and 300 in dark—, which meets the 3:1 of WCAG 1.4.11 on its own. **Always
visible via keyboard** (`:focus-visible`); never `outline: none` without a replacement. On a
fill (solid button) `sytue-focus-ring-inset` is used, which opens 1 px of the surface
color before the ring; on the chrome, `sytue-focus-ring-chrome`, which changes the
color and not the shadow.

<a id="15-motion-spacingjson--motion--tabla-canónica"></a>

#### 1.5 Motion (`spacing.json → motion`) — canonical table

| Interaction                                        | Duration     | Easing     | Properties                                      |
| -------------------------------------------------- | ------------ | ---------- | ----------------------------------------------- |
| Control hover (button, list-item, tab)             | `fast` 120ms | `standard` | `background-color`, `color`, `border-color`     |
| Active/pressed (feedback)                          | `fast` 120ms | `standard` | `background-color`, `transform` (≤1px)          |
| Focus (ring appearance)                            | `fast` 120ms | `standard` | `box-shadow`                                    |
| In-place state change (selected, checkbox, switch) | `base` 150ms | `standard` | `background-color`, `transform`, `border-color` |
| Dropdown / Popover / Select (enter)                | `base` 150ms | `standard` | `opacity` 0→1, `transform` translateY(-4px)→0   |
| Dropdown / Popover (exit)                          | `fast` 120ms | `exit`     | `opacity` 1→0                                   |
| Tooltip (enter, with 300ms delay)                  | `fast` 120ms | `standard` | `opacity`, `transform`                          |
| Modal — overlay (enter)                            | `base` 150ms | `standard` | `opacity` 0→1                                   |
| Modal — card (enter)                               | `slow` 240ms | `standard` | `opacity` 0→1, `transform` scale(0.98)→1        |
| Modal (exit)                                       | `base` 150ms | `exit`     | `opacity`, `transform`                          |
| Toast (enter)                                      | `base` 150ms | `standard` | `opacity`, `transform` translateX/Y             |
| Toast (exit)                                       | `base` 150ms | `exit`     | `opacity`, `transform`                          |
| Sidebar collapse/expand                            | `base` 150ms | `standard` | `width`                                         |
| Tab underline (indicator)                          | `base` 150ms | `standard` | `transform` / `left`+`width`                    |
| Skeleton shimmer                                   | 1200ms loop  | `linear`   | `background-position`                           |
| Spinner                                            | 700ms loop   | `linear`   | `rotate`                                        |

**Forbidden:** bounce, exaggerated spring, sway, parallax, decorative animation. Nothing

> 240ms in UI (loading loops aside). **Always respect `prefers-reduced-motion`:**
> all non-essential transitions to `0ms`; shimmer/spinner degrade to a state
> static (fixed opacity) or a minimal crossfade.

<a id="16-color-y-contraste-colorsjson--colorscss--wcag-aa"></a>

#### 1.6 Color and contrast (`colors.json` / `colors.css`) — WCAG AA

The palette is calibrated so that **every canonical text/background pair meets WCAG AA**
(normal text ≥ 4.5:1; UI and large text ≥ 3:1), in light and dark. A HEX is not invented
in an app: a role is consumed. Dark is **not** light inverted — it reassigns roles
(see `colors.json → theme.dark`).

**Key distinction — `base` vs `-text`.** A saturated brand/semantic color that
serves to **fill** (badge, presence dot, banner with white text on top) does not
always pass as **text/icon on a light surface**. That is why each accent and each
semantic color has two roles:

- **`*base`** — fill. The text on top uses `on-*` (white or dark ink), not the base.
- **`*-text`** (`--sytue-{accent,primary,violet,success,warning,danger,info}-text`) —
  the color **as text or icon**, calibrated ≥ 4.5:1 on a light surface _and_ on its
  own `*-bg`. In dark these roles point to a light step on a dark
  surface. **Rule:** colored text/icon → use `-text`; fill → use `base`.

The ratios of the canonical pairs, in the three appearances, are in the table in §1.0.1
and are not copied here: they come from `contraste-paleta.test.ts`, which measures them on the stylesheet and
fails if any drops below its minimum.

**Calibration notes:**

- The **primary** as a fill with white text is 600; one step
  lighter (500) barely passes 3:1 and is only valid for components, never for
  text. As text on white, 700.
- In **dark** the petrol primary fill is 400 (`#3cc4cc`) with a `#04161a` label;
  the cobalt one, 500 (`#4169e8`) with a white label. That is why `on-primary` is not used on
  other fills (§1.0.1, rule 1).
- `warning`/`danger` base pass as a **fill** (white text on top, ≥ 5:1); as
  **text** they use the `-text` role (dark step).
- **Control outline** (`border-strong`) does reach 3:1 (WCAG 1.4.11). Decorative
  lines (`border`, `border-subtle`) do not need it and deliberately do not
  reach it.
- **Color is never the only carrier of meaning:** state and presence always
  come with an icon/shape/text (see §6).

---

<a id="22-layout-base-appshell"></a>

### 2.2 Base layout (AppShell)

`AppShell` is the root frame that composes **TopBar + (App rail?) + Sidebar + secondary
panel? + content**. All apps are mounted inside an `AppShell`; the only
variation allowed is _which_ zones are enabled and the context's **product color**.

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

**Canonical measurements** (`spacing.json → layout`):
`topbarHeight 56px` · `sidebarWidth 260px` · `sidebarWidthCollapsed 64px` ·
`appRailWidth 64px` · `contentMaxWidth 1280px` · `contentGutter 24px`.

**Zones and color roles:**

- **TopBar** (`--sytue-topbar-bg` / `--sytue-topbar-fg`): **graphite chrome**, fixed, white text
  at reduced opacity, bottom border `1px --sytue-border`, `z-index.topbar` (1040). No
  shadow at rest; `shadows.xs` when content scrolls under it.
- **App rail** (optional, `appRailWidth 64px`): **graphite** background `--sytue-sidebar-bg`,
  product icons (`branding/icons/*.svg`) centered 40×40 (the tile keeps the product
  color), active with `--sytue-sidebar-active-bg` + brass accent. It appears when the
  app wants quick jumping between products without opening the switcher.
- **Sidebar** (`sidebarWidth 260px`, collapsible to 64px): **graphite** background (rail)
  `--sytue-sidebar-bg`, text `--sytue-sidebar-fg` (white at reduced opacity), groups with
  `overline` in `--sytue-sidebar-fg-muted`. `z-index.sidebar` (1030). Width transition
  `motion.base`.
- **Secondary panel** (inboxes): ~320–360px, `--sytue-surface` surface, separated from the
  content by `1px --sytue-border`. It contains a dense `List`.
- **Content**: `--sytue-bg` background (ivory), cards/tables on `--sytue-surface`, max
  width `contentMaxWidth` centered with `contentGutter`.

**Global density:** medium-high. List rows 32–40px, card inner padding 16px
(`spacing.4`), section spacing 24px (`spacing.6`). Base 14px (`body`).

**Responsive (reference breakpoints):**
`< 768px` the sidebar and secondary panel collapse into _drawers_ over an overlay
(`z-index.overlay`); the TopBar keeps logo + search + account, the rest goes into a menu.
`768–1024px` sidebar collapsed to a 64px rail by default. `≥ 1280px` full layout.

---

<a id="23-specs-de-componentes-sytueui"></a>

### 2.3 Component specs (`@sytue/ui`)

The 20 existing components. Each spec is **implementable as is** on the
current component.

#### 3.1 AppShell

Root layout (§2). It has no interactive states of its own; it orchestrates slots.

- **Slots:** `topbar` (always), `rail?`, `sidebar?`, `aside?` (secondary panel),
  `children` (content).
- **Structure:** CSS grid — rows `[topbar 56px][1fr]`; body columns
  `[rail 64px?][sidebar 260/64px?][aside auto?][content 1fr]`.
- **A11y:** TopBar in `role="banner"`; Sidebar/rail in `<nav>`; content in `<main>`;
  secondary panel in `<aside>` or `<nav>` depending on whether it contains navigation or a list.
- **Dark:** all via roles; no specific adjustment.
- **Skip link** "Saltar al contenido" (Skip to content) as the first tabbable element, visible on focus.

#### 3.2 Sidebar

Navigation of the current app. It is the **rail**: background in the brand color in all three
themes (§1.0.1, “Carril” shape).

- **Container:** width `260px` (collapsed `64px`), background `--sytue-sidebar-bg`
  (the chosen color), no right-hand rule, vertical padding `spacing.2`, `z-index.sidebar`.
  What lives inside takes the rail tokens (`appearance.css`).
- **Group:** header with the `overline` role in `--sytue-sidebar-fg-muted`, top margin
  `spacing.4`. Group actions (`SidebarSection.actions`) to the right of the header
  as 24px icon buttons (`SidebarAction`: «gestionar carpetas», «nuevo
  cuaderno», «suscribirse» — manage folders, new notebook, subscribe); never as one more item in the list. A group can be
  collapsible (`collapsible`, chevron and `aria-expanded`); settings start collapsed.
- **Main action on mobile (<768px):** `MobileFab`, a 56px round primary button
  pinned at the bottom right (respects `safe-area-inset-bottom`), outside the `AppShell`
  because on mobile the content and the list take turns. It does not exist on desktop: there it is
  carried by the Sidebar's primary button.
- **Item:** height **36px**, radius `md`, horizontal padding `spacing.3`, gap `spacing.2`
  between icon (20px, UI icon) and label (`body`, `--sytue-sidebar-fg`). Count badge on
  the right (neutral `Badge` on brand).
- **States:**
  - `default`: text `--sytue-sidebar-fg-muted` (dimmed white), transparent background.
  - `hover`: background `--sytue-sidebar-hover-bg` (white veil), text `--sytue-sidebar-fg`
    (transition `motion.fast`).
  - `active/selected` (`aria-current="page"`): background `--sytue-sidebar-active-bg` (white
    veil at 14 %), text `--sytue-sidebar-active-fg` (#fff), weight `medium`. A 3px
    bar on the left in **amber** (`--sytue-indicator`, which inside the rail is
    `--sytue-sidebar-indicator`).
  - `focus-visible`: white ring (`sytue-focus-ring-chrome`).
  - `disabled`: opacity 0.5, no pointer.
- **Collapsed (64px):** centered icon only; the label moves to a `Tooltip` on the right. Toggle
  with a `width` transition `motion.base`.
- **Dark:** `--sytue-sidebar-bg` becomes a very dark green (`#0e2a1f`); the rest via role.
- **A11y:** `<nav aria-label>`, a list of links, `aria-current` on the active one, navigable with
  up/down arrows, `Home/End`.

#### 3.3 TopBar

Global shell (`@sytue/shell`), mounted in every app.

- **Container:** height `56px`, background `--sytue-topbar-bg` (light), bottom border `1px
--sytue-border`, horizontal padding `spacing.4`, `z-index.topbar`, fixed.
- **Brand area (`rail`):** in the suite shell, the logo area is painted in the
  rail color and, from 768 px, with the width of the side menu: it is the rail's
  header. Inside it the rail tokens apply; the switcher that opens from it is
  a surface and goes back to the theme ones. Below 768 px the whole bar is the rail
  (§1.0.1, “Carril” shape, point 2).
- **Content (tab order):** suite logo (`branding/logo/sytue-one.svg`) →
  global search → app switcher (`AppGrid`) → notifications → presence → account
  menu (`Avatar` + `Dropdown`).
- **Search:** `sm` input, flexible width (max ~480px), magnifier icon on the left,
  placeholder `--sytue-text-subtle`; `⌘K`/`Ctrl-K` opens the command palette (level 4 overlay).
- **Action icons:** `ghost` `icon` 32px buttons; unread badge on notifications
  (`Badge` `danger` pill, top-right).
- **States:** inherited from Button (ghost) and Dropdown.
- **Dark:** `--sytue-topbar-bg` → `#0d1310`; the border, `chrome-border`.
- **A11y:** `role="banner"`, `aria-label` per action, logical order, search with
  `role="searchbox"`.

#### 3.4 AppGrid (App switcher / Launcher)

Grid of products with brand icons.

- **Trigger:** `ghost icon` button (9 dots) in the TopBar.
- **Panel:** **level 2** elevation popover (`shadows.md`), radius `lg`, background
  `--sytue-surface`, padding `spacing.4`, width for a 3–4 column grid.
- **App cell:** 40×40 product icon (`branding/icons/<app>.svg`, `rx 9` tile
  on `product.<app>`), `caption` label below, centered; cell with radius `md`,
  padding `spacing.2`.
- **Cell states:** `hover` background `--sytue-bg-subtle`; `focus-visible` ring
  `shadows.focus`; `active` background `--sytue-neutral-100` (`neutral-850` dark).
- **Motion:** panel enter `motion.base` (opacity + translateY-4px→0), exit
  `motion.fast` easing `exit`.
- **A11y:** `role="menu"`/`grid`; 2D arrow navigation; `Esc` closes and returns focus
  to the trigger; each cell with the visible name as accessible name and the description in `title`.

#### 3.5 Button

- **Heights / padding (hit area ≥ 32px):**
  - `sm`: height 28px, padding-x `spacing.3` (12px), text `sm/600`.
  - `md` (default): height 32px, padding-x `spacing.4` (16px), text `button` (base/600).
  - `lg`: height 40px, padding-x `spacing.5` (20px), text `md/600`.
  - `icon`: square (28/32/40 depending on size), no padding-x, 20px icon.
- **Radius:** `md`. **Gap** icon↔text: `spacing.2`.
- **Variants × states** (background / text / border):

| Variant     | default                                                          | hover                          | active                                        | disabled                |
| ----------- | ---------------------------------------------------------------- | ------------------------------ | --------------------------------------------- | ----------------------- |
| `primary`   | `primary-500` / `on-primary` / —                                 | `primary-600`                  | `primary-700`                                 | opacity 0.5, no pointer |
| `secondary` | `--sytue-surface` / `--sytue-text` / `1px --sytue-border-strong` | background `--sytue-bg-subtle` | background `neutral-100` (`neutral-800` dark) | opacity 0.5             |
| `ghost`     | transparent / `--sytue-text` / —                                 | background `--sytue-bg-subtle` | background `neutral-100`                      | opacity 0.5             |
| `danger`    | `danger` (#b0413a) / #fff / —                                    | `danger-hover`                 | darkens 1 step                                | opacity 0.5             |

- **focus-visible:** `shadows.focus` ring on all variants (in addition to the background).
- **loading:** a 16px `Spinner` replaces the leading icon, dimmed text, `aria-busy`,
  fixed width (does not jump), not clickable.
- **Motion:** hover/active `motion.fast`; press applies `transform: translateY(0.5px)`.
- **Dark:** via roles; `primary` keeps `primary-500` (AA contrast on dark OK).
- **A11y:** a real `<button>`, `aria-busy` while loading, `aria-label` mandatory on `icon`,
  never color alone for meaning (danger comes with clear text/icon).

#### 3.6 Card

- **Surface:** background `--sytue-surface` (white), border `1px --sytue-border-subtle`,
  radius `lg` and **level 1** elevation (`shadows.sm`): it lifts off the canvas. With
  `raised={false}`, flat.
- **Group color:** it does not tint the card. It goes in `CardIcon`, an icon on a
  solid fill of the tone (`tone`); `fill="solid"` fills the whole card when it is a
  highlighted resource.
- **Padding:** `spacing.4` (16px) content; header/footer separated by `1px
--sytue-border`, same padding.
- **Regions:** `header` (`h4` title + actions on the right), `body`, `footer`.
- **Interactive (card as link/action):** adds `hover` states (border
  `--sytue-border-strong` + `shadows.xs`), `focus-visible` (ring), `active`. Pointer
  cursor only if the whole card is actionable.
- **Dark:** `--sytue-surface` → `neutral-900`; border `neutral-800`.
- **A11y:** `<section>` with `aria-labelledby` pointing to the title when it groups content; an
  actionable card as a wrapping `<a>`/`<button>`, not `div onclick`.

#### 3.7 List

Dense lists (Mail, Chat, Drive, Contacts, notifications).

- **Row:** height **36px** (compact) / **48px** (with subtitle, e.g. Mail), padding-x
  `spacing.3`, gap `spacing.3`. Structure: `[icon/avatar 20–32px] [title `body`+
subtitle`caption` `--sytue-text-muted`] [tabular `caption` meta] [actions on hover]`.
- **Separation:** `1px --sytue-border` between rows **or** no border with `hover` as the only
  separation (choose one per view; Mail uses a border, Chat does not).
- **Row states:**
  - `hover`: background `--sytue-bg-subtle`; reveals quick actions (`ghost` icons).
  - `active/press`: background `neutral-100` (`neutral-850` dark).
  - `selected` (`aria-selected`): background `primary-50` (`primary-950` dark) + a 2px
    left bar `primary-500`; text `--sytue-text`.
  - `focus-visible`: inner `shadows.focus` ring.
  - `unread` (Mail/Chat): title weight `semibold`, a 6px `primary-500` dot on the left.
  - `disabled`: opacity 0.5.
- **empty:** delegates to `EmptyState`.
- **loading:** N `Skeleton` rows with the same height.
- **Motion:** hover `motion.fast`; selection `motion.base`.
- **A11y:** `role="list"`/`listbox` depending on selection; arrow + `Home/End` navigation;
  `aria-selected`; hover actions also reachable by keyboard (visible on focus).

#### 3.8 Table

Dense table with sorting, selection and pagination.

- **Density:** **40px** row (default) / **32px** (compact). 40px header, `sticky`,
  background `--sytue-surface`, `shadows.xs` when scrolling under it.
- **Cell:** padding `spacing.2 spacing.3`, text `body`, **tabular numbers** in
  numeric/date columns; right alignment for numbers.
- **Header:** `label` (sm/600) `--sytue-text-muted`; a sortable column shows a chevron
  on hover and state (`aria-sort`).
- **Row states:** `hover` background `--sytue-bg-subtle`; `selected` background `primary-50`
  (`primary-950` dark) + checked checkbox; `focus-visible` ring on the row.
- **Zebra:** disabled by default (`1px --sytue-border` bottom border per row);
  can be enabled on very wide tables with `--sytue-bg-subtle` on odd rows.
- **empty/loading/error:** `EmptyState` in the body / `Skeleton` rows / error banner
  (inline `danger` `Toast` with retry).
- **Pagination:** footer with `caption` "1–25 de 240" (1–25 of 240, tabular) + `ghost` prev/next buttons +
  size selector (`Select`).
- **Dark:** header `neutral-900`, borders `neutral-800`.
- **A11y:** semantic `<table>`, `<th scope>`, `aria-sort`, announced sort order; selection with
  `aria-selected`/checkbox; a table caption or `aria-label`.

#### 3.9 Modal / Dialog

- **Overlay:** full-screen `--sytue-overlay`, `z-index.overlay` (1070).
- **Card (from 768 px):** centered —or the right-hand panel of `placement="right"`—, background
  `--sytue-panel-surface` (opaque in all three modes: in “Translucent” it is `surface-raised`,
  like the menus, because a semi-transparent dialog could not be read), card radius of the shape (`rounded-card`: 14 in “Carril”,
  16 in “Marco”; formerly 20, the roundest surface in the suite), `shadow-xl`,
  `z-index.modal` (1080). Widths: `sm 400px` · `md 560px` · `lg 720px` · `xl 1120px`; max
  height `90 %` of the visual viewport, fitted to the content, with a scrollable body.
- **Sheet (below 768 px), the one used by ALL dialogs:** full width, resting on the
  bottom edge of the visual viewport (above the keyboard), top corners with the card
  radius, a 36×4 px handle that drags the sheet down with the finger (more than 80 px closes it; only if the
  dialog is `dismissable`), bottom safe area in the last band and air at the top
  (`max(2,5rem, zona segura + 1rem)`, i.e. max(2.5rem, safe area + 1rem)). Header and footer fixed and the body scrolls; only
  if even then a 2.75rem body does not fit does the whole panel scroll.
- **Regions:** header (`sytue-h4` title + `ghost icon` close cross), body and footer (actions on
  the right, gap `spacing.2`; primary at the far right), all at `px-6`. The same
  air, 21 px, at the top, between title and body, between body and actions and at the bottom (header
  `pt-5 pb-2`, body `py-3`, footer `pt-2 pb-5`, plus the 1 px rule of each band).
  **No rules** between bands except when the body scrolls: then both appear
  (`border-subtle`) to say “there is more”.
- **Focus with the finger:** opening does not focus the first field (it would bring up the keyboard over the sheet
  on Android, and on iOS it would not): focus goes to the panel and the keyboard comes up when the field is tapped.
- **Motion:** the overlay enters `motion.base` (opacity); the card enters in 180 ms (opacity + scale
  0.98→1 from 10 px); the sheet slides up from the bottom in 220 ms; no exit animation.
- **States:** `loading` (footer with the primary button loading + `aria-busy` on the dialog);
  `error` (inline `danger` banner above the body).
- **Dark:** `shadows.dark.lg`; overlay `rgba(0,0,0,0.6)`.
- **A11y:** `role="dialog"` `aria-modal="true"`, `aria-labelledby` pointing to the title, **focus
  trap**, `Esc` closes, initial focus on the first control (or on the title), **focus return**
  to the trigger on close. Clicking the overlay closes only if it is not destructive.

#### 3.10 Dropdown / Menu

Action and account menus.

- **Trigger:** button/avatar; reflects `aria-expanded`.
- **Panel:** **level 2** popover (`shadows.md`), radius `lg`, background `--sytue-surface`,
  border `1px --sytue-border`, padding `spacing.1`, min width 180px, `z-index.popover`
  (1060). It is positioned anchored to the trigger (flipping if it does not fit) and **bounded by the window**:
  `max-width: 100vw - 16px` plus a horizontal nudge (`useFitInViewport`) so that it does not
  start at a negative coordinate. Flipping is not enough: with the trigger far from both
  edges, a wide panel anchored to the right starts off-screen on the
  **left**, and that does not create scroll —it gets clipped and there is no way to reach it—.
  Vertically **it does not cover its trigger**: it stays in the space on one side —below, unless
  it does not fit and there is more room above— and whatever does not fit scrolls inside the
  panel (`encajarPanel`). A long menu, like the account one, used to sit on top of its
  own button as soon as it was taller than that space.
  And when there is content below what is visible, **it shows it**: the bottom edge
  fades into the surface and carries an arrow, until the end is reached. The scroll
  bar is not enough, because on macOS and on mobile it is not painted until it
  moves.
- **Item:** height **32px**, radius `md`, padding-x `spacing.3`, gap `spacing.2` (16px icon
  - `body` label); keyboard shortcut on the right in `caption` `--sytue-text-subtle`.
- **Item states:** `hover`/`focus` (unified) background `--sytue-bg-subtle`; `active`
  `neutral-100`; `disabled` opacity 0.5; `danger` item with `danger` text, hover background
  `--sytue-danger-bg`.
- **Separator:** `1px --sytue-border`, margin `spacing.1`. Group header `overline`.
- **Motion:** enter `motion.base` (opacity + translateY-4px→0), exit `motion.fast`
  easing `exit`.
- **A11y:** `role="menu"`/`menuitem`, arrow navigation, `Home/End`, type-ahead to jump,
  `Esc` closes and returns focus, `aria-expanded` on the trigger.

#### 3.11 Toast

Ephemeral notification.

- **Container:** stacks bottom-right (configurable), gap `spacing.2`, `z-index.toast` (1090).
- **Toast:** background `--sytue-surface`, border `1px --sytue-border`, radius `lg`, **level 3**
  elevation (`shadows.lg`), padding `spacing.3 spacing.4`, width 320–400px. A 2px bar/icon
  on the left according to the variant.
- **Variants** (mandatory icon + color, **never color alone**):
  `success` (check icon, `success`) · `info` (`info`) · `warning` (`warning`) ·
  `danger` (`danger`). The background stays neutral (`--sytue-surface`); the color goes in the
  icon + side bar.
- **Structure:** 20px icon + `label` title + `caption` description `--sytue-text-muted`
  - optional action (`Button ghost sm`) + close `ghost icon`.
- **Behavior:** auto-dismiss after 5s (paused on hover/focus); `danger` does not auto-dismiss.
- **Motion:** enter `motion.base` (translateX/Y + opacity), exit `motion.base` easing
  `exit`.
- **A11y:** `role="status"` (info/success) or `role="alert"` (warning/danger),
  the corresponding `aria-live`; it does not trap focus; closable by keyboard.

#### 3.12 Tooltip

- **Panel:** background `neutral-900` (`neutral-0`/`neutral-800` in dark → contrast),
  text `--sytue-neutral-0` `caption`, radius `md`, padding `spacing.1 spacing.2`,
  **level 2** elevation (`shadows.md`), `z-index.tooltip` (1100), max ~240px.
- **Behavior:** appears after **300ms** of hover/focus; disappears immediately on leaving.
  Optional 6px arrow.
- **Motion:** enter `motion.fast` (opacity + translate 4px), no long animated exit.
- **A11y:** `role="tooltip"`, linked via `aria-describedby`; reachable by keyboard focus;
  **never** essential content only in a tooltip; nothing interactive inside.

#### 3.13 Tabs

Segmented navigation within a view.

- **Tablist:** a row with bottom border `1px --sytue-border`. Tab: padding
  `spacing.2 spacing.3`, text `label` (sm/600), height 36px, gap `spacing.1`.
- **States:**
  - `default`: text `--sytue-text-muted`.
  - `hover`: text `--sytue-text`, background `--sytue-bg-subtle`.
  - `active/selected` (`aria-selected`): text `--sytue-text`, a 2px **indicator**
    `primary-500` on the bottom border aligned with the tab.
  - `focus-visible`: `shadows.focus` ring.
  - `disabled`: opacity 0.5.
- **Motion:** the indicator slides between tabs `motion.base` (`transform`/`left+width`).
- **Segmented variant** (pill): container `--sytue-bg-subtle` radius `md`; active tab
  `--sytue-surface` + `shadows.xs`.
- **A11y:** `role="tablist"/"tab"/"tabpanel"`, arrows to move, `aria-selected`,
  `aria-controls`, activation with Enter/Space (manual) or on focus (automatic).

#### 3.14 Form (Input, Textarea, Select, Checkbox, Radio, Switch, FieldGroup)

**Common field rules**

- **Label:** `label` role (sm/600) `--sytue-text`, above the control, gap `spacing.1`.
- **Help:** `caption` `--sytue-text-muted` below the control.
- **Error:** `caption` `--sytue-danger` + icon, `aria-invalid`, `aria-describedby` points
  to the message. **Never color alone:** icon + text always.
- **Required:** textual mark/`*`, `aria-required`.

**Input / Textarea**

- Height `md` **36px** (Textarea min 72px, vertically resizable), radius `md`, border
  `1px --sytue-border-strong`, background `--sytue-surface`, padding-x `spacing.3`, text
  `body`, placeholder `--sytue-text-subtle`.
- **States:** `hover` border `--sytue-neutral-300`; `focus-visible` border `primary-500` +
  `shadows.focus` ring; `disabled` background `--sytue-bg-subtle`, text `--sytue-text-subtle`,
  no pointer; `error` border `danger` + ring `rgba(danger)`; `readonly` without a strong focus
  border.
- Optional icon/affix inside (`--sytue-text-subtle`, 16px), adjusted padding.

**Select**

- Same box as Input; 16px chevron on the right. On open → a Dropdown-style panel
  (**level 2**), 32px items, chosen item with a check + `selected` (`primary-50`).
- States inherit from Input (box) + Dropdown (panel).

**Checkbox / Radio**

- **16px** box (checkbox radius `sm`; circular radio `full`), border `1px
--sytue-border-strong`.
- `checked`: background `primary-500`, #fff glyph (check / dot). `hover` border
  `--sytue-neutral-300`. `focus-visible` ring. `disabled` opacity 0.5. `error` border
  `danger`. `indeterminate` (checkbox) center bar.
- Motion: the check appears `motion.base`.

**Switch**

- Track **36×20px** radius `pill`; 16px knob. `off` track `--sytue-neutral-300`;
  `on` track `primary-500`, the knob slides `motion.base`. `focus-visible` ring;
  `disabled` opacity 0.5. `role="switch"` `aria-checked`.

**FieldGroup / Form**

- Vertical spacing between fields `spacing.4`; sections `spacing.6`. Button bar at the bottom
  aligned to the right (primary + `ghost` secondary).
- **Dark:** boxes `--sytue-surface` → `neutral-900`, border `neutral-700` (strong).
- **A11y:** every control with an associated `<label for>`; radio groups in `<fieldset><legend>`;
  errors linked via `aria-describedby`; focus always visible.

#### 3.15 Avatar

- **Sizes:** `xs 20` · `sm 24` · `md 32` (default) · `lg 40` · `xl 64`. Radius `full`
  (users) or `md` (groups/entities).
- **Content:** image (`object-cover`) or initials (1–2) on a background derived
  deterministically from the name (**earthy palette** `--sytue-avatar-1..6`: teal/plum/slate/clay/
  moss/indigo, white on top ≥ 6:1), text `label`/weight 600 in `--sytue-neutral-0`.
- **Presence:** a `max(10, round(lado × 0,34))` px dot (i.e. max(10, round(side × 0.34))) at the bottom right (at 8 px the disc
  ended up at 4 and the shape could not be seen), with the
  `2px --sytue-surface` ring INSIDE that measurement, color from `presence.*` (online/away/dnd/offline).
  Never color alone: each state also carries its SHAPE in `surface` —online the full disc,
  away a crescent, dnd a horizontal bar, offline a hollow ring— and its translated
  `aria-label`/title (`presence.*` from the common catalog). The fractions are the
  same on the web (`FORMA` in Avatar.tsx) and in Fyne (`ui.FormaPresencia`).
- **Group/stack:** -8px overlap, border `2px --sytue-surface`; "+N" as the last one.
- **A11y:** `<img alt>` with the name, or `aria-label` if initials; presence with text.

#### 3.16 Badge / Tag

- **Badge (count/state):** height 20px, radius `pill`, padding-x `spacing.2`, text
  `caption`/600, **tabular numbers**. Neutral (`--sytue-bg-subtle` + `--sytue-text-muted`)
  by default; semantic variants use `*-bg` + `*` (e.g. `--sytue-success-bg` +
  `success`), `primary`/`accent` accent for brand. A 6–8px dot badge for unread.
- **Tag (label):** radius `md`, same height, optionally with a color dot on the left and a
  close button (`ghost icon` 14px) if removable.
- **States (removable tag):** `hover` on the close button; `focus-visible` ring.
- **Dark:** semantic backgrounds use the dark theme `*-bg` (defined in `colors.css`).
- **A11y:** AA contrast; meaning through text + color, not color alone; count with a
  readable `aria-label` ("3 no leídos", 3 unread).

#### 3.17 EmptyState

- **Composition:** illustration/brand (system line art, §art-direction 4.5) 96–120px
  → `h4` title → `body` description `--sytue-text-muted` → CTA (`Button primary`).
  Centered, `max-width` ~360px, vertical gap `spacing.3`, padding `spacing.8`.
- **Variants:** initial empty (with a create CTA), no search results (no
  heavy illustration, text + clear-filter action), error (`danger` icon + retry).
- **Dark:** illustration via roles (works in both themes).
- **A11y:** title in a real heading; CTA as a button/link; the message does not depend on the image.

#### 3.18 Skeleton

- **Block:** background `--sytue-bg-subtle`, radius of the element it replaces (`md` text,
  `full` avatar, `lg` card). Text lines: height 12–14px, varied widths (100/80/60%).
- **Shimmer:** a subtle gradient `--sytue-neutral-100`→`--sytue-bg-subtle` moving,
  1200ms `linear` loop. Respects `prefers-reduced-motion` → static.
- **Usage:** replicates the real geometry (same row/card height) to avoid layout
  shift on load.
- **A11y:** container `aria-busy="true"`; skeletons `aria-hidden="true"`; announce
  "Cargando" (Loading) with `aria-live` once, not per block.

#### 3.19 Spinner

- **Sizes:** `sm 16` · `md 20` · `lg 24`. 2px stroke, 270° arc in `currentColor` (inherits:
  `primary` in a primary button, `--sytue-text-muted` in content). 700ms `linear` loop.
- **Usage:** inline in Button loading, in one-off loading overlays. For loading lists/
  tables prefer `Skeleton`.
- **A11y:** `role="status"` + `aria-label="Cargando"`; `prefers-reduced-motion` → a soft
  pulsing dot or static.

#### 3.20 Icon

UI icons (interface glyphs, not product ones).

- **Grid:** 24×24 (`viewBox`), usual render **20px** (16px in dense views, 24px in TopBar/
  empty states). `stroke-linecap/linejoin="round"`, `stroke="currentColor"` (inherits the
  context color; never its own color except for semantics).
- **Framing:** each glyph centered at (12,12) with a uniform optical box (~2–3u of margin);
  none floats small or off-center (chevrons fill their square like the rest).
- **Style: duotone with accent.** It is painted by ONE component, `Glifo` from `@sytue/ui`
  (`packages/ui/src/components/Glifo.tsx`); `Icon` and each app's own catalogs
  only store the drawing. Three rules:
  1. **A 1.5 px on-screen stroke at any size**: `stroke-width = 1,5 × 24 / px`.
     With the old fixed `stroke-width={2}` the same glyph weighed 1.3 px at 16 and 2.7 px
     at 32.
  2. **Accent fill under the stroke**, only in closed subpaths (those ending
     in `Z`) and closed shapes (`rect`, `circle`, `polygon`), at 34 % and even-odd:
     a shape inside another stays as a hole (the pupil of the eye). With its own `fill`
     —the marked star or heart— the shape is filled and has no layer.
  3. **The fill color is `--sytue-glyph-fill`**, falling back to `--sytue-primary`: the
     brand color on the content. Inside the color rail it is set by the rail
     rule (amber). On a color fill —main, secondary and danger buttons,
     solid badges and cards, product tiles— `currentColor` applies: this is decided by
     a rule in `packages/ui/src/styles.css` that recognizes the ink of whoever writes
     on top of a color (`text-white`, `text-[var(--sytue-on-primary)]`,
     `text-on-card-solid-*`).
- **Rules:** a single weight per system; optical alignment to the center of the text line;
  clean SVG (no metadata). No hand-written icon `<svg>` outside `Glifo`:
  guarded by `packages/ui/src/components/glifos-al-comun.test.ts`, with its exceptions
  compared by equality. Content canvases (charts, slides,
  annotations) are not icons and do not use this style.
- **Native (Fyne):** the same weight and the same layer, painted by `apps/fyne/iconos`
  with the numbers from `Glifo.tsx` (a test reads them from there). The fill is the theme
  primary; on a color fill it is the ink, recognized the same way as on the web (white
  ink or `on-primary`), and whoever needs another —the rail— sets it with `Dibujo.Acento`.
- **Product vs UI:** product icons (`branding/icons/*.svg`, 40×40 `rx 9` tile) only
  in AppGrid, rail, app avatars and favicons — **not** inside controls.
- **A11y:** decorative → `aria-hidden="true"`; with its own meaning → `role="img"` +
  `aria-label`.

---

<a id="24-guia-de-layout-por-producto"></a>

<a id="24-guía-de-layout-por-producto"></a>

### 2.4 Layout guide per product

All share AppShell, components and palette. Allowed difference: **product color**
(`colors.json → product`) **only in the rail/switcher icon tile** (orientation between
apps) — each app's chrome is monochrome graphite; **never** a colored header, a different aesthetic
or layout.

| Product          | Tile               | Active zones                                                                                  | Main content                                                                                                                                                                                       |
| ---------------- | ------------------ | --------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Hub / Portal** | `hub` #0f766e      | TopBar + Sidebar                                                                              | Large AppGrid + summary cards (unread mail, upcoming events, recent files) in `Card`s                                                                                                              |
| **Mail**         | `mail` #be123c     | TopBar + Sidebar (folders) + **secondary panel** (list) + content                             | Panel = `List` of conversations (48px row, unread bold + dot); content = thread (messages in `Card`s), compose in a `Modal`/side view                                                              |
| **Drive**        | `drive` #146a47    | TopBar + Sidebar (Mi unidad, Compartido, Papelera — My drive, Shared, Trash) + content        | `List`/card grid toggle; breadcrumb (`Tabs`/nav); actions in a contextual `Dropdown`; `Table` in list view (name/owner/modified/size, tabular)                                                     |
| **Docs**         | `docs` #1c58d9     | TopBar + (optional outline Sidebar) + reading content (`body-md`)                             | Editor at reading width (~720px), formatting `Toolbar` (`ghost` buttons), comments in a right-hand secondary panel                                                                                 |
| **Sheets**       | `sheets` #0f7052   | TopBar + content (dense grid)                                                                 | Cell grid (tabular numbers); `Toolbar`; the grid is its own dense surface on tokens, not a standard `Table`                                                                                        |
| **Slides**       | `slides` #94590f   | TopBar + secondary panel (slide strip) + canvas                                               | Panel = vertical `List` of thumbnails; central canvas                                                                                                                                              |
| **Calendar**     | `calendar` #7e22ce | TopBar + Sidebar (mini-month, calendars) + content                                            | Week/month grid; events as blocks color-coded by calendar (`violet` and product colors, AA text); day/week/month `Tabs`                                                                            |
| **Contacts**     | `contacts` #0e7490 | TopBar + Sidebar (groups) + **secondary panel** (list) + detail                               | Panel = A–Z `List` with `Avatar`; detail = field `Card`s                                                                                                                                           |
| **Rooms**        | `rooms` #4c1d95    | TopBar + Sidebar (channels/DMs with presence) + content; the call takes over the content area | Message timeline (dense, no border between rows, grouped by author); composer at the bottom; `Avatar` with presence. In a call: video grid, controls at the bottom and the channel as a side panel |
| **Cloud (PaaS)** | `cloud` #2464c9    | TopBar + Sidebar (projects/resources) + content                                               | Resource `Table`s (status with a semantic `Badge`), logs in `code`/mono, deployment forms                                                                                                          |
| **Console**      | `console` #3a4a5e  | TopBar + Sidebar (admin: users, domains, billing) + content                                   | Dense `Table`s, configuration `Form`s, metrics (tabular numbers), destructive confirmation `Modal`s                                                                                                |
| **www (brand)**  | `one` #c2410c      | Public header + content + footer                                                              | **Brand** surface: `display`/`h1`, graphite chrome with brass accent, sober European sovereignty; uses the same tokens and typography, lower density (marketing)                                   |

**Constant across all of them:** 56px TopBar (graphite chrome) with the app switcher; graphite sidebar/rail;
dense ivory content; product color only in the switcher tile; brass accent; light
and dark; the motion of §1.5.

---

<a id="25-patrones-de-composicion"></a>

<a id="25-patrones-de-composición"></a>

### 2.5 Composition patterns

<a id="51-listas"></a>

#### 5.1 Lists

Dense by default (§3.7). Choose border-between-rows (Mail, data lists) **or**
hover-as-separator (Chat) per view, not both. Quick actions on hover **and** on focus.
Empty state → `EmptyState`; loading → `Skeleton` with the same row height.

<a id="52-tablas"></a>

#### 5.2 Tables

For structured data with sorting/selection/pagination (Drive list, Cloud, Console).
Sticky header, tabular numbers, right alignment for numerics. Avoid zebra except on
very wide tables. Multiple selection with checkbox + a contextual action bar that
appears on selection (`motion.base`).

<a id="53-formularios"></a>

#### 5.3 Forms

Label above, help below, error with icon + text (never color alone). One field per row
except logical pairs (city/postal code). Button bar at the bottom aligned to the right: primary on the
right, `ghost` cancel to its left. Validation on `blur`/submit, not on every keystroke.
Complete field states (§3.14).

> **Configuration ≠ form.** If the form **configures** something (an integration, a
> policy, a provider, an organization setting), it is not solved with this section: it uses
> the primitives of **§8** (`Wizard` / `SettingsShell` + `FieldHint`). This section covers the
> ordinary data form (creating a contact, editing a profile).

<a id="54-modales"></a>

#### 5.4 Modals

Only for short tasks and confirmations. The smallest size that fits (`sm/md/lg`). An `h3`
title that names the action, not "Are you sure?". Destructive confirmation → `danger` button on
the right, describing the consequence. Focus trap + focus return (§3.9). Long tasks →
page/panel, not a modal.

#### 5.5 Toasts

Feedback for a completed action or a system error, not for form validation (that goes
inline). `success`/`info` auto-dismiss after 5s; `warning`/`danger` persist until closed.
Icon + text always. Max ~3 stacked; the rest queued.

<a id="56-navegación"></a>

#### 5.6 Navigation

Hierarchy: **TopBar** (global: switch app, search, account) → **Sidebar** (sections of
the current app) → **Tabs** (views within a section) → **breadcrumb** (depth in
Drive/Console). `aria-current` on the active level of each. Never duplicate the same
navigation in two zones.

<a id="57-elevación-en-la-práctica"></a>

#### 5.7 Elevation in practice

At rest = flat + border. Only the ephemeral things that open on top are elevated (dropdown/modal/
toast/tooltip), with the level from §1.3. If in doubt between shadow or border to separate two
fixed zones → border.

<a id="58-fotos-y-mandos-sobre-foto"></a>

<a id="58-fotos-filas-justificadas-y-mandos-sobre-foto"></a>

#### 5.8 Photos: justified rows and controls on top of a photo

A photo grid (Sytue Photos) uses **justified rows**: each photo keeps its
aspect ratio, the rows fill the width and the last one is not stretched. It is CSS —the `.filas` class
in `apps/photos/src/styles/app.css`, with each tile's aspect ratio in `--r`—, with no
JavaScript layout. Only extreme aspect ratios are cropped, bounded to
9:20–5:2 (`lib/filas.ts`). The row is at least 9, 11 and 13 rem depending on the breakpoint, and on
a phone whatever lets two 16:9 photos fit side by side.

What goes **on top of a photo** does not use surface tokens: the photo does not change with the theme
and `surface` is translucent in translucent mode. It is black at 60 % with white ink
—5.7:1 over a white photo, the worst case—: `IconButton tone="media"` for a control
(the viewer arrows) and the same pair for a mark (the heart on a tile). A
label that must read as part of the chrome uses `bg-surface-raised`, which is opaque in
all three themes.

Hierarchy of an action bar: **one** filled action per screen; the others
`ghost` with their icon; the destructive one apart, after a rule, as an `IconButton
tone="danger"`. The controls in the same bar come in a single size.

---

<a id="26-accesibilidad-wcag-aa--checklist-de-merge"></a>

### 2.6 Accessibility (WCAG AA) — merge checklist

- **Contrast:** text ≥ 4.5:1, UI/large text ≥ 3:1. Palette calibrated in light and dark
  (ratios verified in §1.6); use the `-text` role for colored text/icons, `base` only
  for fills. Any new pair is validated before merge.
- **Visible focus:** `shadows.focus` on everything focusable; never `outline:none` without a replacement.
- **Keyboard:** every interaction reachable; logical order; `Esc` closes overlays; focus trap
  in modals; focus return on close; arrows in list/menu/tabs/grid.
- **Semantics/ARIA:** landmarks (`banner`/`nav`/`main`/`aside`), correct roles,
  `aria-current/selected/expanded/invalid/live/sort/busy`.
- **Not color alone:** state, error and presence carry an icon/text in addition to color.
- **Motion:** `prefers-reduced-motion` respected in every non-essential transition.
- **Touch:** targets ≥ 32px (ideally 44px on mobile).
- **Language:** `lang="es"` in the product.

---

<a id="261-auditoria-de-accesibilidad-de-office"></a>

<a id="261-auditoría-de-accesibilidad-de-office-docus-calcus-diapos-y-certipdf"></a>

#### 2.6.1 Office accessibility audit (Docus, Calcus, Diapos and CertiPDF)

Measured in a browser, not declared. `test/e2e/accesibilidad-office.mjs` opens the `/dev/editor`
and `/dev/pdf` test benches of `apps/docs` (development only; the API is answered with
`page.route`, without a session or service) and checks, per product, at 1280, 390 and 320 px, in light
and in dark:

- **axe-core** (WCAG 2.0/2.1/2.2 A and AA) on the page and with each ribbon tab open.
  `serious` and `critical` FAIL; the rest is listed as minor and does not block.
- **200 % zoom** (640 px window): the document does not overflow and the ribbon stays whole.
- **Keyboard** (1280 px): Tab reaches the ribbon, the arrows move tabs and band, F6 jumps
  between ribbon and canvas, Escape closes a popover and returns focus, and everything focusable
  shows visible focus.
- **Touch** (390 and 320 px, `pointer: coarse`): every header and ribbon control measures at least
  44 × 44 px.
- **Screen reader**: named `tablist`/`tabpanel`, unnamed regions, controls without an
  accessible name, language, and that a save failure (POST `/api/ops` → 500) is announced in
  a live region.

It is launched by `test/e2e/correr-accesibilidad.sh` (also `cinta.mjs` and `paridad-cinta-web.mjs`) and
by the `accesibilidad-office` job in `ci-frontend.yml`: when a release is published and when
`ci-frontend` is dispatched manually (it does not run on push, on PR, or inside `ci`).

**The two rules that came out of measuring.**

1. **Touch floor per zone, not class by class.** `Button` and `BOTON_DE_BARRA` already had
   `pointer-coarse:`, but the ribbon paints dozens of buttons and fields with copied class
   strings (`h-7 w-7`, `h-6`) and each new copy was born at 25 px. `data-suelo-tactil` on the root
   of a zone (`@sytue/ui`, `styles.css`) sets the 44 px minimum on everything inside it with
   a touch pointer; `data-sin-tactil` leaves out what the canvas draws with its own controls
   (grid, stage, page). The fixed-height strips grow through their variables
   (`--editor-tabs-h`, `--editor-status-h`, `--editor-rail-w`). Criterion: `pointer: coarse`,
   not the window width.
2. **A `tablist` is fulfilled, not written.** `useTablist` (`@sytue/ui`) provides the roving tabindex,
   the arrows and the tab-panel link. CertiPDF's three strips (bar, side panel and
   signature dialog) had `role="tab"` and nothing else.

**Documented exception.** The three indent markers on the Docus ruler (14 × 9 px) do not
pass the 24 px of 2.5.8, and cannot grow without overlapping (the ruler is 20 px tall). It is a pointer
shortcut: the three indents can also be set with numeric fields in the settings panel. It is the
“Equivalent” exception of 2.5.8, and the audit applies it only while that equivalent exists.

**Pending (minor or belonging to another piece).**

- The Name Manager does not exist in native Calcus (see the command parity).
- Fyne: the native ribbon is measured by size (`LadoBotonCinta`, 24.5 px) with the fine pointer and its
  accessibility depends on the driver bridge (`AccessibilityLabel`/`AccessibilityRole`, which all
  ribbon controls already carry); it still needs to be checked with VoiceOver and Narrator in a real
  window, and the 44 px touch floor on Android and iOS.
- The editor ribbon (`Ribbon.tsx`) and `Tabs` from `@sytue/ui` carry their own copy of the arrow
  handling; migrating them to `useTablist` is mechanical and is left for when the ribbon is not under
  aesthetic construction.

<a id="27-reglas-de-uso-obligatorias"></a>

### 2.7 Usage rules (mandatory)

1. **A single source.** Components only from `@sytue/ui`; shell only from
   `@sytue/shell`. Reimplementing any of the 20 components in an app is forbidden.
2. **No ad-hoc UI.** A component/variant is missing → it is added to `@sytue/ui` (PR to the design
   system), not in the app.
3. **Only tokens.** Role variables/tokens from `branding/tokens/`; never HEX, loose px or
   local fonts. The values in this guide **are** tokens.
4. **Shell on every surface.** Every app mounts `@sytue/shell` (TopBar + app switcher +
   search + notifications + presence + account).
5. **Brand from `branding/`.** Logos and icons are consumed from `branding/logo/` and
   `branding/icons/`; they are not redesigned per app.
6. **100% Sytue-native.** No third-party engines to theme. All surfaces are
   original Sytue work branded directly with `@sytue/ui` + `branding/`.
7. **Light and dark.** Every component and view works in both themes via roles.
8. **AA always.** No merge goes below the accessibility standard.
9. **Brand density.** 14px base, 32–40px rows, 4px grid. Medium-high density is
   part of the identity, not an optional setting.
10. **Guided configuration.** Every configuration surface —of any product, of the
    organization or of the account— uses the primitives of §8. No “form wall”,
    no setting that requires reading documentation to understand it.

---

<a id="28-configuracion-guiada-obligatoria-en-toda-la-suite"></a>

<a id="28-configuración-guiada-obligatoria-en-toda-la-suite"></a>

### 2.8 Guided configuration (mandatory across the whole suite)

<a id="81-la-directiva"></a>

#### 8.1 The directive

> **Every Sytue feature —new and existing— is configured with an intuitive UI in the
> Apple + Vercel + DigitalOcean style: clean, guided step by step, in plain language, so
> that a basic user can configure it without documentation. NEVER the dense complexity of
> Google Cloud/AWS nor a form wall.**

It is not an aesthetic preference: it is a **permanent** product requirement and it applies to
**all** configuration surfaces of the suite —Console, each product's settings,
organization settings, user settings—, not only to the interoperability ones. A
new configuration surface that does not comply is a regression, even if it works.

What it means in practice:

| Yes                                                               | No                                               |
| ----------------------------------------------------------------- | ------------------------------------------------ |
| One idea per step, with its reason                                | 20 fields on one screen                          |
| Plain language: “Who does this rule affect?”                      | Implementation jargon: `include_groups[]`        |
| Saying **where to find** each piece of data in the source product | A `label` and you are on your own                |
| Testing the connection **before** saving                          | Saving and discovering the failure in production |
| A final summary: “this is what you are going to apply”            | A “Save” button and faith                        |
| Describing the **consequence** of each option                     | Listing technical identifiers                    |
| Saving per section, with its own action                           | A single giant “Save” at the bottom              |
| Being able to cancel without leaving half-created leftovers       | Orphaned instances after an `Esc`                |

<a id="82-las-primitivas-sytueui"></a>

#### 8.2 The primitives (`@sytue/ui`)

**Mandatory** use; they exist for this and are not reimplemented per app (rule §7.1–7.2).

| Primitive             | What it is                                                                                                                                                                                                                                                                                     | When                                                                                              |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| **`Wizard`**          | The canonical frame for “connect/configure something”: steps, back/next, per-step validation, terminal step. `WizardStep` carries `heading` (plain), `description`, `content`, `canContinue` (validation gate), `nextLabel`, `onNext` (run the step's action instead of advancing) and `done`. | Creating or changing something with **consequences**, or that needs data from an external system. |
| **`Stepper`**         | Wizard progress (`<ol>` with `aria-current="step"`). Without `onStepClick` it is a pure indicator.                                                                                                                                                                                             | Inside the `Wizard`; also in custom multi-step flows.                                             |
| **`SettingsShell`**   | Frame of a **settings** page: title + description in plain language, main action, `banner` for permanent notices, bounded reading width (`max-w-5xl`). `level` avoids duplicating the `h1`.                                                                                                    | The ongoing administration page of a feature.                                                     |
| **`SettingsSection`** | A self-explanatory block within the page, with its **own** actions. `plain` for free content without a `Card`.                                                                                                                                                                                 | Each group of settings. **Saved per section**, not at the end of the page.                        |
| **`FieldHint`**       | A line under a field: what it is and **where to find it**. It switches to ok/error state in the same place and size (only the icon and color change); `tone="danger"` emits `role="alert"`. `id` to link with `aria-describedby`.                                                              | Every field whose value comes from another system or can be wrong.                                |
| **`InlineHelp`**      | A context block within the flow: “what to have at hand”, the result of a test, a warning. Sober: border + tinted background, no gradients or shadows.                                                                                                                                          | Before asking for data, and to return the result of a check.                                      |
| **`ReviewSummary`**   | A key→value list of “this is what you are going to apply”. `mono` for technical values (URL, id); `onEdit` per row jumps to the step where that data was filled in.                                                                                                                            | The second-to-last step of every wizard that applies changes.                                     |
| **`ChoiceCard`**      | An option card of a radio group: title + **the consequence** of choosing it, in plain language. `media` for catalogs with a logo. Implemented on a native `<input type="radio">` (arrow navigation for free).                                                                                  | “Choose one of these” — never a `select` of identifiers.                                          |
| **`EmptyState`**      | Empty state with brand, title, description and CTA (§3.17).                                                                                                                                                                                                                                    | Before the first configuration exists: the empty state is the front door, not an error.           |

> **Actual status (2026-07-27):** `ChoiceCard` exists in `@sytue/ui` but **is not exported yet**
> from `index.ts` and has no consumers; `AccessPolicyWizard` still uses a local copy.
> The migration is in progress. Until it is closed, it is not true that “it is already used everywhere”.

<a id="83-asistente-o-panel-de-ajustes"></a>

#### 8.3 Wizard or settings panel

Short rule: **the wizard creates, the panel administers.** In practice they are combined — the panel
is a `SettingsShell` whose main action opens the `Wizard`.

| Use **`Wizard`** when…                                                 | Use **`SettingsShell` + `SettingsSection`** when… |
| ---------------------------------------------------------------------- | ------------------------------------------------- |
| It is a creation, and there is nothing useful until it is finished     | The object already exists and is being fine-tuned |
| There is an **order**: one step depends on the previous one            | The settings are independent of each other        |
| It must be **tested** against an external system before applying       | The change is local and instantly reversible      |
| The result has consequences for other people (access, policies)        | The change affects a single preference            |
| The admin needs data from another product and must be told where it is | The admin already knows the field                 |

And always, in either of the two:

- **Test before saving.** If there is a third party on the other side, there is a test step, and its
  result is shown with `InlineHelp` — not with a `toast` that goes away.
- **Cancelling leaves no leftovers.** If the flow had to create a draft to be able to test, it is
  deleted on cancel (a pattern already applied in `ConnectAppWizard` and `ConnectIdpWizard`).
- **Secrets are _write-only_.** They are asked for once and never returned.
- **What happens with the data, before granting anything.** Everything that connects Sytue One with a
  third party shows `DataFlow` (`@sytue/ui`): whether the data only comes in, goes out to the third party or goes
  both ways, what kind of data and which policy covers it (what comes in, Sytue One's;
  what goes out, the third party's). The direction is deduced from the `incoming`/`outgoing` lists, it is not
  declared separately. In a wizard the full version with the third party's brand is used; in a one-click
  block or a tab where the other side can be anything (a webhook, a bot), `compact`.
- **The summary speaks in Spanish** (plain human language), not in JSON. `AccessPolicyWizard` translates the whole rule into
  a sentence and warns about whom it would leave out: that is the bar.
- **§6 A11y without exceptions:** focus goes to the step heading on change, progress is
  an `<ol>` with `aria-current="step"`, and every field with help links it via `aria-describedby`.

<a id="84-inventario-de-superficies"></a>

#### 8.4 Inventory of surfaces

**Already upgraded** (`apps/frontend/apps/console/src/components/`):

| Surface                                          | Components                                         | Flow                                                                                                                                  |
| ------------------------------------------------ | -------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| **Connect an application** (Integration Gateway) | `ConnectAppWizard.tsx` + `AppsPanel.tsx`           | choose → what you will enable → configure → **test** → review → done                                                                  |
| **Connect an identity provider (SSO)**           | `ConnectIdpWizard.tsx` + `SsoPanel.tsx`            | type (OIDC/SAML) → what will change for your team → configure → **test** → review → done (with the values to paste into the provider) |
| **Conditional Access policy builder**            | `AccessPolicyWizard.tsx` + `AccessPolicyPanel.tsx` | whom it affects → in which situations → what must happen → review (rule in one sentence) → done                                       |

**Pending** (still `SectionHeader` + `Modal` + a flat form):

- Console: `GroupsPanel` (incl. dynamic group rules), `RolesPanel`, `PrivilegedAccessPanel`
  (PIM), `CompliancePanel`, `GovernancePanel` (DLP), `DevicesPanel`, `UsersPanel`,
  `TenantsPanel` / `TenantDetailPanel`, `UnifiedAuditPanel`, `PasswordPolicyCard`, `ProfileCard`.
- **Outside Console there is no settings surface at all** that uses these primitives: Mail,
  Drive, Calendar, Contacts, Portal and Cloud Dashboard do not yet have their own settings screen
  built with `SettingsShell`. It is the biggest gap against the directive of §8.1, which applies to
  the **whole** suite and not only to administration.

> When upgrading a surface: first `SettingsShell` + `SettingsSection` (break up the wall), then
> `Wizard` for creation, and `FieldHint`/`InlineHelp` so that each piece of data says where it comes from.

---

<a id="3-assets-y-vendorizado"></a>

## 3. Assets and vendoring

Source of truth = `branding/`. The frontend **vendors** a copy via
`scripts/sync-branding.sh` (`make sync-branding`, runs in `fe-install`) because the frontend's Docker
images are built with the `apps/frontend/` context, without access to `branding/`.

| Asset                   | Source of truth                                     | Vendored copy                           | Brought in by the sync? |
| ----------------------- | --------------------------------------------------- | --------------------------------------- | ----------------------- |
| Color tokens (CSS)      | `branding/tokens/colors.css`                        | `packages/config/src/tokens/colors.css` | **Yes**                 |
| Tokens (reference JSON) | `branding/tokens/*.json`                            | — (not imported)                        | No (deliberate)         |
| Tokens (TS mirror)      | —                                                   | `packages/config/src/tokens.ts`         | No — **by hand**        |
| Product icons           | `branding/icons/*.svg`                              | `packages/ui/src/assets/icons/*.svg`    | **Yes**                 |
| Logos                   | `branding/logo/*.svg`                               | `packages/ui/src/assets/logos/*.svg`    | **Yes**                 |
| Illustrations + og      | `branding/illustrations/*.svg`, `branding/og/*.svg` | pending                                 | **No**                  |

**Traps (each one costs an afternoon):**

- **Do not hand-edit the vendored copies.** They are the output of `sync-branding.sh`: a manual
  edit is lost on the next sync and produces light/dark drift. Edit `branding/` and
  re-sync.
- **`tokens.ts` is a manual mirror** (the sync does not touch it): it is the easiest source of drift.
- **The `*-text` roles do not work without the `@theme` mapping.** Re-syncing the CSS without mapping leaves
  the `text-*-text` utilities non-existent and the change fails **silently**.
- **`border`/`border-strong` do not reach 3:1 on purpose** (decorative separators): focus or
  state never rely on the border alone — there is a 3 px focus ring in `theme.css`.
- **Dark mode is reassignment, not inversion.** It is already in `colors.css` with
  `[data-theme="dark"]` and `prefers-color-scheme`; do not invert manually in the apps.
- **Never rename `--sytue-*` tokens or change the `@sytue/ui` APIs**: consumers
  depend on both.

<a id="32-puerta-automática-appsfrontendscriptslint-i18nsh"></a>

### 3.2 Automatic gate: `apps/frontend/scripts/lint-i18n.sh`

It counts the interface labels written in Spanish in the code and **fails if the
number goes up**. It is a ratchet, not a closed gate: there is enough debt left that
demanding zero would block any commit, and a gate that cannot be met ends up being
disabled. When the count goes down, the script itself says which number to set.

Before creating a key, **search for the literal in the app's `es.ts`**. The most
repeated pattern is not untranslated text but unused translation: `app.*.description`,
`quick.*`, `slides.shape.*`, the 191 `extdata.*` keys… all written in the five
languages while the component painted the Spanish literal next to them.

**The two i18n gates are complementary and neither is enough on its own:**

|                            | Sees                                            | Does not see                                            |
| -------------------------- | ----------------------------------------------- | ------------------------------------------------------- |
| `lint-i18n.sh`             | `label=`, `title:`, `placeholder=`… in any file | Loose text in JSX; whatever comes from shared constants |
| `i18n-sin-espanol.spec.ts` | What is RENDERED, loading each app in English   | What is not painted without opening a popover or a menu |

That second gap is real and took effort to discover: the spec walks through the ribbon tabs
and considered them clean, but the strings of the drop-down menus —data validation,
sparklines, charts— are not rendered until they are opened, and there was still Spanish there.

<a id="31-puerta-automática-appsfrontendscriptslint-designsh"></a>

### 3.1 Automatic gate: `apps/frontend/scripts/lint-design.sh`

It runs inside `bun run lint`. It covers the three design defects that neither TypeScript nor
ESLint can see, because they live **inside class strings**. All three were detected
while fixing real defects, not in the abstract:

| Rule                                   | What it breaks                                                               | Why it is not seen in review                                                                      |
| -------------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Focus ring without a neutral outline   | WCAG 1.4.11                                                                  | Brass gives 2.81:1 on ivory but **does pass in dark theme**, which is where people usually review |
| Raw `primary-*` / `accent-*` step      | Dark theme does not reassign those families                                  | In light it looks perfect                                                                         |
| Concatenated class without a separator | The resulting class **does not exist** and takes the neighboring one with it | The browser does not warn: it simply applies nothing                                              |

The third is the most treacherous: ``class={`w-max${fz ? 'sticky' : ''}`}`` produces
`w-maxsticky`. It broke **Sheets' «Congelar paneles» (Freeze panes)** in four places at once —the
feature froze nothing— and it only showed up in the conditional branch, which is the
exceptional one. The check reads each file whole (`perl -0777`) and not line by line: the two
worst cases had the `${` and the ternary on different lines, precisely because they were the longest
templates.

> **The separator goes outside the interpolation.** `` `base ${c ? 'x' : ''}` ``, never
> `` `base${c ? ' x' : ''}` ``. Both produce the same HTML, but
> `prettier-plugin-tailwindcss` normalizes class strings and **deletes the space in
> the second one**: the fix lasts until the next `bun run format`. That was the origin
> of all this — nobody wrote the defect, the formatter reintroduced it, and that is
> why the Sytue Admin capacity bar kept breaking every time it was fixed.

Whatever a `grep` cannot check is left to e2e: `test/e2e/tests/sheets-freeze.spec.ts`
verifies that the frozen row, besides staying fixed, **covers what is underneath**. The
selection highlight is brass at 9 % —translucent on purpose—, and on a row that
floats over others it let two row numbers be read at once.

Since the mobile audit of 2026-08-08 there is a fourth rule: **no layout height
in `vh`**. In Chrome Android `100vh` is the height **with the address bar
retracted** —more than the visible screen on load—, so a frame that does not scroll leaves
out whatever is left over and there is no way to reach it. The correct unit is `dvh`, the one
`AdminShell` already used. It had slipped into four places (the two Docs shells, the
Rooms link landing page and `AppShell` itself) and no review caught it, because
in a desktop browser `vh` and `dvh` **measure the same**: the defect only exists on
the phone.

<a id="32-desbordamiento-en-movil"></a>

<a id="33-desbordamiento-en-móvil-las-tres-capas"></a>

### 3.3 Mobile overflow: the three layers

Keeping the interface from spilling out of a phone is not sustained by reviewing; it is sustained by
gates. There are three, from cheapest to most expensive:

1. **`lint-design.sh` (in `bun run lint`, i.e. in every CI run).** It statically catches what
   can be caught that way: today, height in `vh`. It is the only layer that prevents the
   regression from **getting in**.
2. **`test/e2e/mobile-audit.mjs`.** It truly measures, on 18 real device profiles
   (iPhone SE/12/14 Pro Max and landscape, iPad Mini; Galaxy S9+/S24/A55/Tab S4/Tab S9/Z Flip
   Cover/Z Fold 6, Moto G4, Pixel 5/7/9, LG Optimus L70). It measures **each container**, not
   just the document: the suite shells trap scrolling inside `main`, so
   `document.scrollWidth` gives false greens. **It exits with code 1 if anything overflows**, so that
   it can be used as a gate and not only as a report.
3. **The scheduled daily audit**, which runs it in full and fixes whatever it finds.

**The `data-scroll-x` convention.** A deliberate horizontal scroll —a wide table,
a spreadsheet grid, the editor's style gallery, the ribbon's tab
strip— is marked with that attribute on the scrolling container. Without it, the
audit counts it as a defect every day and the report becomes noise nobody
reads. Tables and grids (`table`, `role="grid"`) are already recognized on their own.

**The three patterns that cause this over and over**, in case they serve as a checklist when
writing: a grid or flex child without `min-w-0` (it does not shrink below its minimum content
width, and the whole column stretches); a fixed width (`w-80`, `w-52`) without a
responsive variant; and an action row with `shrink-0` that never wraps.

---
