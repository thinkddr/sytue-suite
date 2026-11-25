<a id="blueprint--editor-sytue-docs-v1"></a>

# BLUEPRINT — Sytue Docs editor (v1)

Normative specification for 14 builders working in parallel (A1–A3, B1–B11).
This document IS the contract: the interfaces in §2–§5 are copied verbatim into
`src/editor/types.ts` and the files indicated. When in doubt, the auditor's TECHNICAL CONTRACT
(2026-07-03) wins, then this blueprint. It is not importable code.

---

<a id="0-reglas-de-oro"></a>

## 0. Golden rules

1. **Strict clean-room.** OnlyOffice is AGPL: it is FORBIDDEN to copy its code, CSS,
   class names, icons or assets. Here we replicate the _experience_ described
   in §6 with Sytue branding. Do not look at its repository at all.
2. **Each file has ONE owner** (§10). Do not touch another builder's files.
   It is FORBIDDEN to touch `src/components/{DocList,Shell,DocsEditor}.tsx`,
   `src/pages/api/{documents,create,collab}.ts`, `src/lib/{auth,token}.ts`,
   `packages/ui` and anything outside `apps/frontend/packages/office/`
   (exception: A1 edits `src/pages/edit.astro`; A2 creates new routes under
   `src/pages/api/` and `src/lib/docsProxy.ts`; A1 may add a spec in
   `test/e2e/tests/`). If someone else's file changes under your feet, re-read it and
   merge; NEVER revert someone else's work. NO `git commit/push`.
3. **No direct fetch from the browser to `docs-api.sytue.localhost`** (there is no
   CORS) and **no WebSocket** (the browser handshake gets a 401 —
   verified). Everything goes through the same-origin BFF proxies in §4 and
   collaboration is REST _polling_.
4. **No new dependencies.** Preact + `@sytue/ui` + Tailwind v4 are already installed.
   Editor icons: local, in `src/editor/icons.tsx` (§9).
5. **Colors:** token utilities (`bg-surface`, `text-text`, `border-border`,
   `bg-bg-subtle`, `bg-selected`, `text-text-muted`, `text-text-subtle`,
   `--sytue-focus-ring`…). New hex values ONLY those declared in `editor.css` (§8)
   and the CONTENT palettes in §5 (colors the user applies to their text).
6. **AA accessibility:** every icon-button with `aria-label` + `title`; toggles with
   `aria-pressed`; tabs with `tablist/tab/tabpanel` roles and arrow keys; save
   status with `aria-live="polite"`; focus always visible
   (the global ring or a `sytue-focus-ring*` variant from `@sytue/ui`; never
   `outline:none` without a replacement, nor `focus-visible:shadow-[…]`, which does not paint).
7. **Correct Spanish with accents** in every visible string and comment.
8. SPDX header in every new file, like the rest of the repo:
   ```
   /*
    * SPDX-License-Identifier: Apache-2.0
    * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
    */
   ```
9. Typecheck: `export PATH="/opt/homebrew/bin:/Users/ddr/.npm-global/bin:$PATH"`
   and `(cd apps/frontend && bunx astro check --root packages/office)`. The global check
   will only be green once A1, A3 and B5 have landed; each builder
   is responsible for ensuring that THEIR files do not contribute errors of their own.
10. Deliberate shortcuts are marked with a `// ponytail: …` comment stating the ceiling
    and the path to improvement.

---

<a id="1-estructura-de-ficheros-todo-bajo-appsfrontendpackagesoffice"></a>

## 1. File structure (everything under `apps/frontend/packages/office/`)

```
src/editor/
├── BLUEPRINT.md                  (este documento)
├── types.ts                      A1  contratos compartidos (§2, copy-paste)
├── EditorApp.tsx                 A1  isla raíz: compone useDocument + zonas
├── editor.css                    A1  vars locales + estilos de página (§8)
├── icons.tsx                     A3  EditorIcon + 46 trazados propios (§9)
├── layout/
│   ├── EditorLayout.tsx          B1  rejilla de zonas (slots)
│   ├── StatusBar.tsx             B1  barra inferior 28px
│   ├── EditorHeader.tsx          B2  cabecera 48px (título, guardado, compartir…)
│   ├── LeftRail.tsx              B3  franja 40px + paneles buscar/esquema/comentarios
│   └── SettingsPanel.tsx         B4  panel derecho 288px (párrafo/tabla)
├── ribbon/
│   ├── primitives.tsx            A3  RibbonButton/Group/Select/ColorSwatch/Separator (§5)
│   ├── Ribbon.tsx                B7  tira de pestañas 32px + menú Archivo + panel activo 68px
│   └── tabs/
│       ├── Inicio.tsx            B8
│       ├── Insertar.tsx          B9
│       ├── Referencias.tsx       B9
│       ├── Disposicion.tsx       B10
│       ├── Vista.tsx             B10
│       └── Colaboracion.tsx      B10
├── canvas/
│   ├── exec.ts                   B5  comandos de edición sobre contenteditable (§3)
│   ├── PageCanvas.tsx            B6  página A4 (kind document)
│   └── Ruler.tsx                 B6  regla horizontal 20px
├── sheets/
│   └── SheetCanvas.tsx           B11 hoja de cálculo (kind spreadsheet)
├── slides/
│   └── SlidesCanvas.tsx          B11 presentación (kind presentation)
└── state/
    └── useDocument.ts            A2  datos: carga/autosave/título/polling/comentarios/versiones/export (§4)

src/lib/docsProxy.ts              A2  helper de proxy BFF (§4.1)
src/pages/api/state.ts            A2  ┐
src/pages/api/ops.ts              A2  │ proxies same-origin al servicio Go (§4.2)
src/pages/api/rename.ts           A2  │
src/pages/api/comments.ts         A2  │
src/pages/api/versions.ts         A2  │
src/pages/api/export.ts           A2  ┘
src/pages/edit.astro              A1  monta EditorApp en vez de DocsEditor
test/e2e/tests/docs-editor.spec.ts A1 humo Playwright (§11)
```

`src/components/DocsEditor.tsx` is left unmounted; it is NOT deleted (cleanup in a later
phase). Recommended landing order: A1 pushes `types.ts` + `editor.css`
immediately; A3 and B5 next; A1 integrates `EditorApp.tsx` + `edit.astro` at the end.
In the meantime, everyone codes against the interfaces in this document.

---

<a id="2-contratos-compartidos--srceditortypests-íntegro-a1-lo-pega-tal-cual"></a>

## 2. Shared contracts — `src/editor/types.ts` (in full, A1 pastes it verbatim)

```ts
/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Contratos del editor Sytue Docs. Fuente única de tipos compartidos entre
 * zonas (BLUEPRINT.md §2). No añadir lógica: solo tipos y constantes.
 */
import type { DocKind, DocumentMeta } from '@sytue/api-clients';

/* ---------- Persistencia (contrato del auditor §5): UN registro LWW por kind ---------- */

/** Clave del registro LWW donde vive TODO el contenido, según el tipo de documento. */
export const CONTENT_KEY: Record<DocKind, string> = {
  document: 'doc:html', // HTML del lienzo (el HTML ES el modelo)
  spreadsheet: 'doc:sheet', // JSON.stringify(SheetModel)
  presentation: 'doc:slides', // JSON.stringify(SlidesModel)
};

/** Registro LWW tal y como lo emite el servicio Go (snake_case, sin mapear). */
export interface LwwRegister {
  value: unknown;
  dot: { a: string; c: number };
  lamport: number;
}

export interface DocStateResponse {
  seq: number;
  state: {
    seq: number;
    lamport: number;
    elements: unknown[] | null;
    registers: Record<string, LwwRegister>;
  };
}

/** Única op que emite el editor v1: set LWW. */
export interface CrdtSetOp {
  kind: 'set';
  dot: { a: string; c: number };
  key: string;
  value: unknown;
  lamport: number;
}

export interface SequencedOp {
  document_id: string;
  seq: number;
  actor: string;
  author_sub: string;
  lamport: number;
  op: CrdtSetOp & { kind: string };
}

/* ---------- Comentarios y versiones (shape Go, snake_case, sin mapear) ---------- */

export interface CommentItem {
  id: string;
  document_id: string;
  parent_id?: string | null;
  author_sub: string;
  anchor: unknown;
  body: string;
  resolved: boolean;
  resolved_by?: string | null;
  created_at: string;
  updated_at: string;
}

export interface VersionItem {
  id: string;
  document_id: string;
  snapshot_id: string;
  label?: string | null;
  author_sub: string;
  seq: number;
  auto: boolean;
  created_at: string;
}

/* ---------- Estado de la interfaz ---------- */

export type SaveState = 'saved' | 'saving' | 'dirty' | 'error';

export type PanelId = 'buscar' | 'esquema' | 'comentarios';

export type ExportFormat = 'pdf' | 'html' | 'markdown' | 'odf' | 'ooxml';

export interface EditorUi {
  /** Panel izquierdo abierto (null = solo la franja de iconos). */
  panel: PanelId | null;
  /** Panel derecho de ajustes visible. */
  settingsOpen: boolean;
  /** Regla horizontal visible (solo kind document). */
  ruler: boolean;
  /** Zoom en %: 50..200, paso 10. */
  zoom: number;
}

/** Configuración de página (solo kind document; NO se persiste en v1). */
export interface PageSetup {
  /** Margen interior de la página en px @100%: 48 | 96 | 128. */
  margin: number;
  /** true = apaisado (intercambia ancho/alto A4). */
  landscape: boolean;
}

export interface EditorStats {
  /** Palabras (solo document; 0 en el resto). Recalculado en cada autosave. */
  words: number;
  /** document: páginas · spreadsheet: hojas · presentation: diapositivas. */
  pages: number;
  /** Unidad actual (página/hoja/diapositiva), 1-indexada. */
  page: number;
}

/** Colaborador visto por polling (sin WS: contrato §4). */
export interface Peer {
  sub: string;
  /** Date.now() de la última op ajena vista; se expira a los 60 s. */
  lastSeen: number;
}

/* ---------- Comandos de edición (canvas/exec.ts, §3) ---------- */

export type EditorCommand =
  | 'bold'
  | 'italic'
  | 'underline'
  | 'strike'
  | 'font' // value: nombre de fuente, p. ej. 'Georgia'
  | 'size' // value: '1'..'7' (escala execCommand, §3.2)
  | 'color' // value: hex de CONTENT_PALETTE
  | 'highlight' // value: hex de HIGHLIGHT_PALETTE o 'transparent'
  | 'clearFormat'
  | 'alignLeft'
  | 'alignCenter'
  | 'alignRight'
  | 'alignJustify'
  | 'listBullet'
  | 'listNumber'
  | 'indent'
  | 'outdent'
  | 'lineHeight' // value: '1' | '1.15' | '1.5' | '2'
  | 'heading' // value: 'p' | 'h1' | 'h2' | 'h3'
  | 'insertTable' // value: 'FxC', p. ej. '3x4'
  | 'insertImage' // value: dataURL
  | 'insertLink' // value: URL absoluta
  | 'insertHr'
  | 'insertToc'
  | 'tableRowAbove'
  | 'tableRowBelow'
  | 'tableColLeft'
  | 'tableColRight'
  | 'tableDelRow'
  | 'tableDelCol'
  | 'tableDelete'
  | 'undo'
  | 'redo';

/** Estado de formato bajo el cursor (queryCommandState/Value; §3.3). */
export interface FormatState {
  bold: boolean;
  italic: boolean;
  underline: boolean;
  strike: boolean;
  /** Nombre de fuente normalizado (sin comillas); '' si indeterminado. */
  font: string;
  /** Talla '1'..'7' de execCommand; '' si indeterminada. */
  size: string;
  align: 'left' | 'center' | 'right' | 'justify';
  bullet: boolean;
  number: boolean;
  heading: 'p' | 'h1' | 'h2' | 'h3';
  /** La selección está dentro de una tabla (activa el panel Tabla). */
  inTable: boolean;
  canUndo: boolean;
  canRedo: boolean;
}

export const EMPTY_FORMAT: FormatState = {
  bold: false,
  italic: false,
  underline: false,
  strike: false,
  font: 'Inter',
  size: '3',
  align: 'left',
  bullet: false,
  number: false,
  heading: 'p',
  inTable: false,
  canUndo: false,
  canRedo: false,
};

/* ---------- Modelos de sheets/slides (§7) ---------- */

export interface SheetModel {
  active: number;
  sheets: SheetData[];
}
export interface SheetData {
  name: string;
  /** 'A1'..'Z100' → HTML de la celda (solo celdas no vacías). */
  cells: Record<string, string>;
}

export interface SlidesModel {
  slides: Slide[];
}
export interface Slide {
  id: string;
  boxes: SlideBox[];
}
/** Caja de texto; x/y/w/h en % del lienzo 16:9 (0..100). */
export interface SlideBox {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
  html: string;
}

/* ---------- EL contrato: contexto que recibe toda zona ---------- */

export interface EditorContext {
  doc: DocumentMeta; // camelCase (BFF /api/documents existente)
  kind: DocKind;

  title: string;
  setTitle(next: string): void; // optimista + POST /api/rename con debounce 800 ms

  save: SaveState;

  /** Último contenido cargado/adoptado. NO cambia con cada tecla (§4.4). */
  content: string;
  /** Incrementa cuando `content` debe re-aplicarse al DOM (carga, adopción remota, restauración). */
  contentEpoch: number;
  /** El lienzo informa de cada edición; programa el autosave. No re-renderiza el lienzo. */
  onContentInput(next: string): void;

  fmt: FormatState;
  /** exec de canvas/exec.ts, tal cual (§3). */
  exec(command: EditorCommand, value?: string): void;

  ui: EditorUi;
  setUi(patch: Partial<EditorUi>): void;

  pageSetup: PageSetup;
  setPageSetup(patch: Partial<PageSetup>): void;

  stats: EditorStats;
  /** Los lienzos informan pages/page (words lo pone useDocument). */
  reportStats(patch: Partial<EditorStats>): void;

  peers: Peer[];

  comments: CommentItem[];
  addComment(body: string): Promise<void>;
  resolveComment(id: string, resolved: boolean): Promise<void>;
  removeComment(id: string): Promise<void>;

  versions: VersionItem[];
  loadVersions(): Promise<void>;
  createVersion(label?: string): Promise<void>;
  /** Restaura y recarga estado (content + epoch++). */
  restoreVersion(versionId: string): Promise<void>;

  /** POST /api/export → { url } → window.open(url, '_blank'). Errores → toast. */
  exportAs(format: ExportFormat): Promise<void>;
}
```

**Universal zone and tab signature** (all identical, no exceptions):

```ts
import type { JSX } from 'preact';
import type { EditorContext } from '../types';

export function InicioTab({ ctx }: { ctx: EditorContext }): JSX.Element {
  /* … */
}
// Igual: InsertarTab, DisposicionTab, ReferenciasTab, ColaboracionTab, VistaTab,
// Ribbon, EditorHeader, StatusBar, LeftRail, SettingsPanel, PageCanvas, Ruler,
// SheetCanvas, SlidesCanvas — todas reciben `{ ctx: EditorContext }`.
```

`EditorLayout` is the only one with its own props (slots):

```ts
import type { ComponentChildren, JSX } from 'preact';

export interface EditorLayoutProps {
  header: ComponentChildren;
  ribbon: ComponentChildren;
  rail: ComponentChildren; // franja izquierda (incluye su panel desplegable)
  main: ComponentChildren; // zona de lienzo con scroll propio
  settings: ComponentChildren | null; // null = panel derecho oculto
  status: ComponentChildren;
}
export function EditorLayout(props: EditorLayoutProps): JSX.Element;
```

`EditorApp` (island root): `export function EditorApp({ documentId }: { documentId: string }): JSX.Element`.

---

<a id="3-api-de-comandos--srceditorcanvasexects-b5"></a>

## 3. Command API — `src/editor/canvas/exec.ts` (B5)

The canvas HTML IS the model. Implementation: `contenteditable` +
`document.execCommand`, with `Selection`/`Range` fallbacks where execCommand
falls short. Module without components; exact public API:

```ts
import type { EditorCommand, FormatState } from '../types';

/** Registra el root editable activo. PageCanvas lo llama al montar (y con null al desmontar).
 *  SheetCanvas/SlidesCanvas registran la celda/caja enfocada en su focusin. */
export function setEditorRoot(el: HTMLElement | null): void;
export function getEditorRoot(): HTMLElement | null;

/** Ejecuta un comando sobre la selección. Si el foco salió del root (p. ej. un
 *  <select> del ribbon), restaura primero el último Range conocido. Termina
 *  despachando `new Event('input', { bubbles: true })` sobre el root cuando el
 *  camino fue manual (Range); los caminos execCommand ya emiten input nativo. */
export function exec(command: EditorCommand, value?: string): void;

/** Estado de formato bajo el cursor; EMPTY_FORMAT si no hay selección en el root. */
export function getFormatState(): FormatState;
```

Mandatory internal details:

- **Selection memory:** module-global `selectionchange` listener; if the
  `anchorNode` is inside the root, it clones and stores the `Range`. `exec()`
  restores it when `document.activeElement` is not inside the root. That way the
  primitives do not manage selection.
- When registering the root: `document.execCommand('styleWithCSS', false, 'true')`
  (wrapped in try/catch), and `defaultParagraphSeparator` = `'p'`.

<a id="31-mapa-de-comandos"></a>

### 3.1 Command map

| Command                            | Implementation                                                                                                                                                                                                                                                        |
| ---------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| bold / italic / underline / strike | `execCommand('bold'\|'italic'\|'underline'\|'strikeThrough')`                                                                                                                                                                                                         |
| font                               | `execCommand('fontName', false, value)`                                                                                                                                                                                                                               |
| size                               | `execCommand('fontSize', false, value)` with value `'1'..'7'` (§3.2)                                                                                                                                                                                                  |
| color                              | `execCommand('foreColor', false, value)`                                                                                                                                                                                                                              |
| highlight                          | `execCommand('hiliteColor', …)`; if it throws, fall back to `backColor`; `'transparent'` removes the highlight                                                                                                                                                        |
| clearFormat                        | `execCommand('removeFormat')`                                                                                                                                                                                                                                         |
| alignLeft/Center/Right/Justify     | `execCommand('justifyLeft'\|'justifyCenter'\|'justifyRight'\|'justifyFull')`                                                                                                                                                                                          |
| listBullet / listNumber            | `execCommand('insertUnorderedList'\|'insertOrderedList')`                                                                                                                                                                                                             |
| indent / outdent                   | `execCommand('indent'\|'outdent')`                                                                                                                                                                                                                                    |
| lineHeight                         | Range: for each block in `blocksInSelection()` (p, h1–h3, li, td), `style.lineHeight = value`                                                                                                                                                                         |
| heading                            | `execCommand('formatBlock', false, value === 'p' ? '<P>' : '<H1>'…)`                                                                                                                                                                                                  |
| insertTable                        | `execCommand('insertHTML')` with a `<table><tbody>` of R rows × C columns, each cell `<td><br></td>`, followed by `<p><br></p>`                                                                                                                                       |
| insertImage                        | `execCommand('insertImage', false, dataUrl)`                                                                                                                                                                                                                          |
| insertLink                         | `execCommand('createLink', false, url)` (only with a non-collapsed selection; if collapsed, insertHTML `<a href>url</a>`)                                                                                                                                             |
| insertHr                           | `execCommand('insertHorizontalRule')`                                                                                                                                                                                                                                 |
| insertToc                          | Scans the root's `h1,h2,h3`, assigns `id="h-<n>"` to those without one, removes any previous `nav[data-toc]` and inserts at the cursor `<nav data-toc><p><b>Tabla de contenidos</b></p><ol><li data-level="…"><a href="#id">texto</a></li>…</ol></nav>`               |
| tableRow*/tableCol*/tableDel*      | Range: from `anchorNode` → `closest('td')`/`closest('tr')`/`closest('table')` within the root; clones/inserts/deletes a row or column by cell index; deleting the last row/column removes the table; `tableDelete` removes the table. No selection in a table → no-op |
| undo / redo                        | `execCommand('undo'\|'redo')` (the contenteditable's native history)                                                                                                                                                                                                  |

`// ponytail: execCommand está deprecado pero es interoperable y sin dependencias;
// migrar a un modelo propio de Range solo si un navegador soportado lo retira.`

<a id="32-escala-de-tallas-ui--execcommand"></a>

### 3.2 Size scale (UI ↔ execCommand)

| UI (pt) | 8   | 10  | 12  | 14  | 18  | 24  | 36  |
| ------- | --- | --- | --- | --- | --- | --- | --- |
| value   | '1' | '2' | '3' | '4' | '5' | '6' | '7' |

The ribbon's size select shows the pt values and sends `'1'..'7'`;
`getFormatState().size` returns `'1'..'7'` (from `queryCommandValue('fontSize')`).

### 3.3 getFormatState

`queryCommandState` for bold/italic/underline/strikeThrough/insertUnorderedList/
insertOrderedList/justify*; `queryCommandValue` for fontName (strip quotes),
fontSize and formatBlock (→ heading, default 'p'); `queryCommandEnabled('undo'|'redo')`;
`inTable` = `table` ancestor of the anchorNode within the root. Everything in try/catch;
no selection inside the root → return `EMPTY_FORMAT`.

---

<a id="4-datos--stateusedocumentts--proxies-bff-a2"></a>

## 4. Data — `state/useDocument.ts` + BFF proxies (A2)

### 4.1 Helper `src/lib/docsProxy.ts`

```ts
import type { APIContext } from 'astro';

/** Proxy same-origin al servicio Go de Docs (sin CORS ni WS de navegador: contrato §1/§4).
 *  Resuelve la sesión httpOnly, añade Authorization: Bearer y reenvía. */
export async function proxyDocs(
  ctx: APIContext,
  path: string,
  init?: RequestInit,
): Promise<Response>;
```

Implementation: `bff.getSession(toBffContext(ctx))` → if missing,
`{"error":"unauthenticated"}` 401; `fetch(docsBaseUrl() + '/api/v1' + path, …)`
with `authorization: Bearer <session.tokens.accessToken>`; returns a `Response`
with the SAME status and the text body (204 → null body),
`content-type: application/json; charset=utf-8`, `cache-control: no-store`.
**Every parameter interpolated into `path` goes through `encodeURIComponent`** (trust
boundary). `since`/`limit` are validated as numeric. `export const prerender = false;`
in every route.

<a id="42-rutas-bff-mapeo-exacto"></a>

### 4.2 BFF routes (exact mapping)

| BFF route (same-origin)                                   | Go upstream (`/api/v1`)                                                                                                                             |
| --------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/state?id=`                                      | `GET /documents/{id}/state`                                                                                                                         |
| `POST /api/ops?id=` (body as-is)                          | `POST /documents/{id}/ops`                                                                                                                          |
| `GET /api/ops?id=&since=&limit=`                          | `GET /documents/{id}/ops?since=&limit=`                                                                                                             |
| `POST /api/rename` body `{id,title}`                      | `PATCH /documents/{id}` body `{title}` → 204                                                                                                        |
| `GET /api/comments?id=&resolved=`                         | `GET /documents/{id}/comments[?resolved=true]`                                                                                                      |
| `POST /api/comments?id=` body `{body,parent_id?,anchor?}` | `POST /documents/{id}/comments` → 201                                                                                                               |
| `PUT /api/comments?id=&commentId=` body `{resolved}`      | `PUT /documents/{id}/comments/{commentId}/resolve` → 204                                                                                            |
| `DELETE /api/comments?id=&commentId=`                     | `DELETE /documents/{id}/comments/{commentId}` → 204                                                                                                 |
| `GET /api/versions?id=&limit=`                            | `GET /documents/{id}/versions?limit=`                                                                                                               |
| `POST /api/versions?id=` body `{label?}`                  | `POST /documents/{id}/versions` → 201                                                                                                               |
| `POST /api/versions?id=&restore=<versionId>`              | `POST /documents/{id}/versions/{versionId}/restore`                                                                                                 |
| `POST /api/export?id=` body `{format}`                    | `POST /documents/{id}/exports` → 201 `{export:{id}}`, then `GET /documents/{id}/exports/{blobId}` → responds `{url,content_type,size_bytes,format}` |

Missing `id` or another mandatory parameter → `{"error":"missing_id"}` 400. The
Go bodies are forwarded VERBATIM (snake_case reaches the client; the types in §2
reflect that). The existing `/api/documents` (camelCase) and `/api/create` are NOT touched.

<a id="43-hook-usedocument--firma"></a>

### 4.3 Hook `useDocument` — signature

```ts
import type {
  CommentItem,
  EditorStats,
  ExportFormat,
  Peer,
  SaveState,
  VersionItem,
} from '../types';
import type { DocumentMeta } from '@sytue/api-clients';

export interface UseDocumentResult {
  status: 'loading' | 'ready' | 'error';
  doc: DocumentMeta | null;
  title: string;
  setTitle(next: string): void;
  save: SaveState;
  content: string;
  contentEpoch: number;
  onContentInput(next: string): void;
  /** Envía ya el autosave pendiente (pagehide → sendBeacon). */
  flush(): void;
  words: number;
  peers: Peer[];
  comments: CommentItem[];
  addComment(body: string): Promise<void>;
  resolveComment(id: string, resolved: boolean): Promise<void>;
  removeComment(id: string): Promise<void>;
  versions: VersionItem[];
  loadVersions(): Promise<void>;
  createVersion(label?: string): Promise<void>;
  restoreVersion(versionId: string): Promise<void>;
  exportAs(format: ExportFormat): Promise<void>;
}

export function useDocument(documentId: string): UseDocumentResult;
```

<a id="44-algoritmo-contrato-del-auditor-5-obligatorio"></a>

### 4.4 Algorithm (auditor's contract §5, mandatory)

- **Load:** in parallel `GET /api/documents?id=` (camelCase meta) and
  `GET /api/state?id=`. `content = String(state.registers[CONTENT_KEY[kind]]?.value ?? '')`,
  `lamport = state.state.lamport`, `lastSeq = seq`, `actor = crypto.randomUUID()`,
  `counter = 0`, `contentEpoch = 1`. Also `GET /api/comments?id=` (does not block
  `ready`). Any meta/state failure → `status: 'error'`.
- **Autosave:** `onContentInput(html)` stores the HTML in a **ref** (not in
  state that re-renders the canvas!), sets `save='dirty'` and re-arms a debounce
  of **1500 ms**. When it fires: `save='saving'` and
  `POST /api/ops?id=` body
  `{"ops":[{"kind":"set","dot":{"a":actor,"c":++counter},"key":CONTENT_KEY[kind],"value":html,"lamport":++lamport}]}`.
  200 → `lastSeq = seq`, `save='saved'`, recompute `words`
  (`html.replace(/<[^>]+>/g,' ').trim().split(/\s+/).filter(Boolean).length`,
  kind document only; otherwise 0). Network/5xx → `save='error'` and retry after 5 s
  (same body; SAME counter/lamport, no: recompute `++counter`/`++lamport` on
  each attempt for monotonicity). 401 → `save='error'` with no retry (chip
  "Sesión caducada — recarga la página" (Session expired — reload the page)). Remaining 4xx → re-GET state once,
  adopt lamport/lastSeq and retry.
- **Title:** optimistic `setTitle` + 800 ms debounce → `POST /api/rename`
  `{id,title}`; error → revert + toast. Separate effect:
  `document.title = \`${title} — Sytue Docs\``.
- **Content (collaboration, no WS):** every **4000 ms**, only when
  `document.visibilityState === 'visible'`: `GET /api/ops?id=&since=${lastSeq}`.
  For each `SequencedOp`: `lastSeq = max(lastSeq, so.seq)`; if `so.actor !== actor`
  and also `so.op.kind==='set' && so.op.key===CONTENT_KEY[kind] &&
so.op.lamport > lamport` → **adopt**: `lamport = so.op.lamport`,
  `content = String(so.op.value)`, `contentEpoch++` — ONLY if `save==='saved'`
  (if there is a local edit in flight, the next autosave wins: LWW).
- **REAL per-connection presence:** every **10000 ms** `PUT /api/presence?id=`
  `{conn: actor}`; the Go service registers the tab (TTL 30 s, in memory) and
  returns `{members:[{conn,sub}]}`. `peers` = members with `conn !== actor`,
  deduplicated by `sub`. It counts the PASSIVE READER (opening is enough; no ops required).
  `// ponytail: adoptar recoloca el caret al principio; cursores finos llegarán con la migración a RGA.`
- **flush():** if there is a pending autosave,
  `navigator.sendBeacon('/api/ops?id='+id, new Blob([body], {type:'application/json'}))`
  (same-origin cookies are sent). Register it on `pagehide`.
- **Comments:** `addComment` → `POST /api/comments?id=` `{body, anchor: null}`
  (`// ponytail: anclas por carácter cuando haya RGA`); mutations re-query
  the list. **Versions:** `loadVersions` on demand; `createVersion` →
  POST; `restoreVersion` → POST restore and then re-GET `/api/state` to
  adopt content/lamport/lastSeq with `contentEpoch++`. **exportAs** → POST
  `/api/export` → `window.open(url, '_blank')`; error → toast "No se pudo exportar" (Could not export).

`EditorApp` (A1) composes: `useDocument` + local state `ui`, `pageSetup`,
`fmt` (`selectionchange` listener throttled to 100 ms → `getFormatState()` if the
selection is in the root), `stats` (merge of the hook's `words` with
the canvases' `reportStats`) → builds `ctx: EditorContext` and passes it to the
zones. It wraps everything in `@sytue/ui`'s `<ToastProvider>`. `loading` /
`error` states like the current DocsEditor (Spinner / EmptyState + button to `/`).

---

<a id="5-primitivas-del-ribbon--srceditorribbonprimitivestsx-a3"></a>

## 5. Ribbon primitives — `src/editor/ribbon/primitives.tsx` (A3)

```ts
import type { ComponentChildren, JSX } from 'preact';
import type { EditorIconName } from '../icons';

/** Botón-icono compacto 28×28. onMouseDown SIEMPRE hace preventDefault para no
 *  robar el foco (y la selección) del lienzo. */
export interface RibbonButtonProps {
  icon: EditorIconName;
  label: string; // aria-label; title = label + (shortcut ? ` (${shortcut})` : '')
  shortcut?: string; // p. ej. 'Ctrl+B' (mostrar ⌘ en Mac: navigator.platform)
  active?: boolean; // aria-pressed
  disabled?: boolean;
  onAction(): void;
}
export function RibbonButton(props: RibbonButtonProps): JSX.Element;

/** Grupo del ribbon: fila de controles (44px) + rótulo inferior (text-[10px]). */
export interface RibbonGroupProps {
  label: string; // visible bajo el grupo y aria-label del contenedor (role="group")
  children: ComponentChildren;
}
export function RibbonGroup(props: RibbonGroupProps): JSX.Element;

/** Select nativo compacto (h-7). En onMouseDown NO se hace preventDefault (el
 *  select necesita foco); exec() restaura la selección del lienzo él solo (§3). */
export interface RibbonSelectProps {
  label: string; // aria-label
  value: string;
  options: { value: string; label: string }[];
  width?: number; // px; por defecto 120
  onSelect(value: string): void;
}
export function RibbonSelect(props: RibbonSelectProps): JSX.Element;

/** Botón con barrita de color + popover de paleta (grid 6×2 de muestras 20×20).
 *  Todos los mousedown del popover hacen preventDefault (el foco no sale del lienzo). */
export interface RibbonColorSwatchProps {
  icon: EditorIconName; // 'textColor' | 'highlight'
  label: string;
  color: string; // color actual (barrita inferior de 3px)
  palette?: readonly string[]; // por defecto CONTENT_PALETTE
  onPick(color: string): void;
}
export function RibbonColorSwatch(props: RibbonColorSwatchProps): JSX.Element;

/** Separador vertical entre grupos: w-px h-10 bg-border mx-1.5. */
export function RibbonSeparator(): JSX.Element;

/** Paletas de CONTENIDO (colores que el usuario aplica a su documento; no son
 *  tokens de UI — únicos hex permitidos fuera de editor.css, fijados aquí). */
export const CONTENT_PALETTE = [
  '#0A0A0A',
  '#475569',
  '#94A3B8',
  '#FFFFFF',
  '#F97316',
  '#DC2626',
  '#D97706',
  '#16A34A',
  '#2563EB',
  '#7C3AED',
  '#DB2777',
  '#0D9488',
] as const;

export const HIGHLIGHT_PALETTE = [
  '#FEF08A',
  '#FED7AA',
  '#BBF7D0',
  '#BFDBFE',
  '#E9D5FF',
  '#FBCFE8',
  '#E2E8F0',
  'transparent', // transparent = quitar resaltado
] as const;
```

Class recipe for `RibbonButton` (for uniformity; the rest of the primitives use the
same language):

```
inline-flex h-7 w-7 items-center justify-center rounded-sm text-text-muted
hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text
disabled:pointer-events-none disabled:opacity-40
sytue-focus-ring
```

Each ColorSwatch sample is a 20×20 `<button aria-label="Color #F97316">`
with `border border-border` (and `check` over the current color). Popover:
`absolute top-full mt-1 z-[var(--z-popover)] rounded-md border border-border
bg-surface p-2 shadow-[var(--shadow-lg)]`; it closes on Escape and on an outside click.

---

<a id="6-blueprint-visual-por-zona"></a>

## 6. Visual blueprint by zone

Root container (`EditorLayout`): `class="sytue-editor flex h-full min-h-0
flex-col bg-surface text-text"`. The editor lives under the Shell's topbar
(56px): `edit.astro` already reserves `h-[calc(100vh-var(--spacing-topbar))]`.
Vertical structure: header 48 · tabs 32 · groups 68 · [rail 40 |
left panel 288? | canvas flex-1 | settings 288?] · status 28. Heights/widths
come from the vars in §8 (`h-[var(--editor-header-h)]`, etc.).

### 6.1 EditorHeader (B2) — 48px, `bg-surface border-b border-border px-3 gap-2`

From left to right:

1. Back button (`chevronLeft`, aria-label "Volver a tus documentos" (Back to your documents), href `/`).
2. **Editable title**: `<input id="editor-title-input" aria-label="Título del
documento">` without a border (`bg-transparent text-sm font-semibold text-text
hover:bg-bg-subtle focus:bg-bg-subtle rounded-sm px-1.5 h-7 max-w-[360px]`),
   `value={ctx.title}`, onInput → `ctx.setTitle`. Enter/blur confirm.
3. **Save chip** `aria-live="polite"` (`text-xs text-text-muted`):
   saved → "Guardado" (Saved; `bg-success` dot) · saving → "Guardando…" (Saving…)
   (`bg-warning`) · dirty → "Cambios sin guardar" (Unsaved changes; `bg-warning`) · error →
   "Error al guardar" (Error saving; `bg-danger`).
4. `flex-1`.
5. **Collaborators**: overlapping 24px circles (max. 4, `-ml-1.5`,
   `rounded-full bg-bg-subtle border border-border text-[10px]`) with
   `peer.sub.slice(0,2).toUpperCase()` and `title="Colaborador conectado"`.
   `// ponytail: nombres reales cuando exista lookup de identidad.`
6. **Share**: `Button variant="primary" size="sm"` → its own popover:
   text "Solo pueden abrirlo usuarios con permiso." (Only users with permission can open it.) + button "Copiar enlace" (Copy link)
   (`navigator.clipboard.writeText(location.href)` + toast "Enlace copiado" (Link copied)).
   `// ponytail: permisos finos y enlaces de invitado en fase 2 (API ya existe).`
7. **Download**: `@sytue/ui` `Dropdown` (`Button variant="secondary"
size="sm"` + `download`): MenuItems "PDF", "HTML", "Markdown", "ODF (.odt)",
   "OOXML (.docx)" → `ctx.exportAs('pdf'|'html'|'markdown'|'odf'|'ooxml')`.

<a id="62-ribbon-b7--pestañas-32px--grupos-68px"></a>

### 6.2 Ribbon (B7) — tabs 32px + groups 68px

**Tab strip** (`h-[var(--editor-tabs-h)] bg-surface border-b
border-border px-2 flex items-end gap-1`, `role="tablist"`
aria-label "Cinta de opciones" (Ribbon)): the first “position” is **Archivo** (File; it is NOT a tab:
a button that opens a `Dropdown` with "Descargar como PDF/HTML/Markdown" (Download as PDF/HTML/Markdown), "Guardar
versión" (Save version), "Renombrar" (Rename; focuses `#editor-title-input`), "Volver a Documentos" (Back to Documents)
(href `/`)). Then the real tabs, filtered by kind.

> **In force since cinta-4 (2026-10-02):** the list of tabs and the kinds in which they
> are shown live in `pestanas.json` (a copy of `apps/nucleo/office/pestanas.json`); Inicio (Home) and
> Revisar (Review) always, Insertar (Insert) and Diseño (Design) in document and presentation, Datos (Data) and Fórmulas (Formulas) in the
> spreadsheet, Presentar (Present) in slides. The block below is the one from the first version
> and is kept as history.

```ts
const TABS = [
  {
    id: 'inicio',
    label: 'Inicio',
    kinds: ['document', 'spreadsheet', 'presentation'],
    Body: InicioTab,
  },
  { id: 'insertar', label: 'Insertar', kinds: ['document', 'presentation'], Body: InsertarTab },
  { id: 'disposicion', label: 'Disposición', kinds: ['document'], Body: DisposicionTab },
  { id: 'referencias', label: 'Referencias', kinds: ['document'], Body: ReferenciasTab },
  {
    id: 'colaboracion',
    label: 'Colaboración',
    kinds: ['document', 'spreadsheet', 'presentation'],
    Body: ColaboracionTab,
  },
  {
    id: 'vista',
    label: 'Vista',
    kinds: ['document', 'spreadsheet', 'presentation'],
    Body: VistaTab,
  },
] as const;
```

Tab: `role="tab" aria-selected` · `text-sm px-3 h-full text-text-muted
hover:text-text` · active: `text-text font-medium` + 2px underline
`bg-accent` (brand orange) attached to the bottom edge. ←/→ arrows move
the active tab (roving focus), Home/End to the ends. Local `useState` state
(initially `'inicio'`).

**Groups row** (`h-[var(--editor-groups-h)] bg-surface border-b
border-border px-2 flex items-stretch overflow-x-auto`, `role="tabpanel"`):
renders the active tab's `<Body ctx={ctx}/>`. The tabs compose
`RibbonGroup` + `RibbonSeparator` between groups.

<a id="63-contenido-de-cada-pestaña"></a>

### 6.3 Contents of each tab

**Inicio (Home) (B8):**

1. Group "Deshacer" (Undo): `undo` (Ctrl+Z, disabled `!fmt.canUndo`), `redo` (Ctrl+Y,
   disabled `!fmt.canRedo`).
2. Group "Fuente" (Font): font RibbonSelect (Inter, Arial, Georgia, Times New Roman,
   Courier New, JetBrains Mono; width 132; value `fmt.font` if it matches) ·
   size RibbonSelect (§3.2, width 64) · `bold` (Ctrl+B, active `fmt.bold`) ·
   `italic` (Ctrl+I) · `underline` (Ctrl+U) · `strike` (Ctrl+Mayús+X, i.e. Ctrl+Shift+X) ·
   text ColorSwatch (`exec('color', c)`) · highlight ColorSwatch
   (HIGHLIGHT_PALETTE, `exec('highlight', c)`) · `clearFormat`.
3. Group "Párrafo" (Paragraph): `alignLeft/Center/Right/Justify` (active according to `fmt.align`)
   · `listBullet`/`listNumber` (active) · `outdent`/`indent` · line spacing
   RibbonSelect (1 / 1,15 / 1,5 / 2 → `exec('lineHeight', v)`, width 64).
4. Group "Estilos" (Styles; hidden if `kind==='spreadsheet'`): RibbonSelect (Texto
   normal / Título 1 / Título 2 / Título 3 (Normal text / Heading 1 / Heading 2 / Heading 3) → `exec('heading', v)`, value
   `fmt.heading`, width 132).

**Insertar (Insert) (B9):**

1. "Tabla" (Table): `table` button that opens its own popover with a 6×6 grid of
   16×16 cells (hover marks R×C and a "3 × 4" label; click → `exec('insertTable','3x4')`;
   mousedown preventDefault on the whole popover; Escape closes).
2. "Imagen" (Image): `image` button + `<input type="file" accept="image/*" hidden>` →
   FileReader → dataURL; if `dataUrl.length > 1_500_000` → toast "Imagen
   demasiado grande (máx. ~1 MB)" (Image too large (max. ~1 MB)) and do not insert (the whole document travels on
   every autosave); if ok → `exec('insertImage', dataUrl)`.
3. "Enlace" (Link): `link` button → `window.prompt('URL del enlace:')` → if not empty,
   `exec('insertLink', url)`.
4. "Separador" (Separator): `hr` button → `exec('insertHr')`.
5. "Comentario" (Comment): `comment` button → `ctx.setUi({ panel: 'comentarios' })`.

**Disposición (Layout) (B10):** "Márgenes" (Margins) RibbonSelect (Estrecho 48 / Normal 96 /
Ancho 128 (Narrow / Normal / Wide) → `setPageSetup({margin})`) · "Orientación" (Orientation) buttons `pagePortrait`/
`pageLandscape` (active according to `pageSetup.landscape`) · "Sangría" (Indent)
`outdent`/`indent` · "Interlineado" (Line spacing) select as in Inicio.

**Referencias (References) (B9):** "Tabla de contenidos" (Table of contents) button `toc` → `exec('insertToc')`
(running it again regenerates it) · "Vínculos" (Links) button `link` (as in Insertar) ·
"Comentario" button `comment`.

**Colaboración (Collaboration) (B10):**

1. "Comentarios" (Comments): `comment` "Añadir comentario" (Add comment; opens the panel) · toggle
   "Mostrar comentarios" (Show comments; active `ui.panel==='comentarios'`).
2. "Versiones" (Versions): `history` button "Guardar versión" (Save version) →
   `window.prompt('Etiqueta de la versión (opcional):')`; null → cancel; →
   `ctx.createVersion(label || undefined)` + toast "Versión guardada" (Version saved) ·
   `Dropdown` "Historial" (History) which on opening calls `ctx.loadVersions()` and lists
   MenuItems "`label ?? 'Versión'` — local date" → `confirm('¿Restaurar esta
versión? El estado actual se conserva en el historial.')` →
   `ctx.restoreVersion(id)` + toast.
3. "Compartir" (Share): `share` button → copies `location.href` + toast (as in the header).

**Vista (View) (B10):** "Zoom": `minus` (−10) · RibbonSelect with presets 50 / 75 /
100 / 125 / 150 / 200 % (if the current zoom is not a preset, it shows the
nearest one) · `plus` (+10), clamp 50..200 → `setUi({zoom})` · "Mostrar" (Show): toggle `ruler`
"Regla" (Ruler; kind document only; active `ui.ruler`) · toggle `panelRight` "Panel de
ajustes" (Settings panel; active `ui.settingsOpen`).

<a id="64-leftrail-b3--franja-40px--panel-288px"></a>

### 6.4 LeftRail (B3) — 40px rail + 288px panel

Rail: `w-[var(--editor-rail-w)] bg-surface border-r border-border flex
flex-col items-center gap-1 py-2`. Three 32×32 buttons (`search` "Buscar" (Search),
`outline` "Esquema del documento" (Document outline), `comment` "Comentarios"; active
`bg-selected` + `aria-pressed`): a click toggles `setUi({ panel: x | null })`.

Panel (if `ui.panel`): `w-[var(--editor-panel-w)] bg-surface border-r
border-border flex flex-col min-h-0`, `role="complementary"` with aria-label;
h-10 header (`text-sm font-semibold px-3` + `close` button); body
`overflow-y-auto`:

- **Search:** input (`@sytue/ui` `Input`, aria-label "Buscar en el
  documento" (Search in the document), 300 ms debounce) + counter "3 de 7" (3 of 7) + previous/next
  buttons (Enter = next). Implementation: TreeWalker over the root's text nodes
  (`getEditorRoot()`), case-insensitive `indexOf` → list of Ranges;
  select the active one (`selection.addRange`) + `scrollIntoView({block:'center'})`.
  No results → "Sin coincidencias" (No matches).
- **Outline:** list of the root's `h1,h2,h3` (indentation by level:
  `pl-2/4/6`; `text-sm`, hover `bg-bg-subtle`); click → `scrollIntoView`.
  It refreshes on opening and on every transition of `ctx.save` to `'saved'`. Empty →
  "Añade títulos para ver el esquema" (Add headings to see the outline).
- **Comments:** cards (`rounded-md border border-border p-2`): author
  (`author_sub.slice(0,8)`), local date, body, "Resolver"/"Reabrir" (Resolve/Reopen) button
  (`check`) and `trash` "Eliminar" (Delete) (403 → toast). Resolved ones with `opacity-60`.
  At the bottom a textarea + `Button size="sm"` "Comentar" (Comment; disabled when empty) →
  `ctx.addComment`.

<a id="65-lienzo--regla-b6--kind-document"></a>

### 6.5 Canvas + ruler (B6) — kind `document`

Central zone: `flex-1 min-w-0 overflow-auto` with
`background: var(--editor-canvas-bg)`. Inside, a centered wrapper with
`style="zoom: ctx.ui.zoom / 100"` that contains the ruler and the page (so they scale
together).

- **Page**: `class="sytue-page"` (styles in §8), `contentEditable`,
  `role="textbox" aria-multiline="true"` `aria-label={ctx.title}` `lang="es"`,
  `spellcheck`. Dimensions via style: width `794px` (landscape `1123px`),
  `min-height 1123px` (landscape `794px`), `padding: pageSetup.margin px`,
  `margin: 32px auto`. Background ALWAYS white (paper), shadow
  `var(--editor-page-shadow)`, no border.
- **Data cycle**: on mount → `setEditorRoot(el)`;
  `useEffect([ctx.contentEpoch])` → `el.innerHTML = ctx.content ||
'<p><br></p>'`; `onInput` → `ctx.onContentInput(el.innerHTML)`. On unmount →
  `setEditorRoot(null)`. Content does NOT go through Preact state on every keystroke
  (the caret survives).
- **Shortcuts** (`onKeyDown`, `metaKey||ctrlKey`): B/I/U → `exec('bold'|…)` +
  preventDefault; Mayús+X (Shift+X) → strike. Z/Y are left to the native history.
- **Statistics**: on input (500 ms throttle) and scroll:
  `pages = max(1, ceil(el.offsetHeight / altoPágina))`,
  `page = clamp(floor(scrollTop / (altoPágina · zoom/100)) + 1, 1, pages)` →
  `ctx.reportStats({pages, page})`.
  `// ponytail: folio continuo que crece; paginación visual real (cortes) en fase 2.`
- **Ruler (B6, same owner)**: visible if `ui.ruler && kind==='document'`;
  `height: var(--editor-ruler-h)`, same width as the page, `sticky top-0`
  inside the wrapper. An `<svg>` with ticks: major every cm (number `8px
text-text-subtle`), medium every 0.5 cm (px/cm = page width/21 — landscape /29.7).
  Shaded margin zones (`--editor-ruler-margin`), two small static
  triangles marking the indent on the margins.
  `// ponytail: marcadores no arrastrables; drag de sangría en fase 2.`

<a id="66-settingspanel-b4--288px-derecha"></a>

### 6.6 SettingsPanel (B4) — 288px right

Visible if `ui.settingsOpen`. `w-[var(--editor-panel-w)] bg-surface border-l
border-border overflow-y-auto`, `role="complementary"`
aria-label "Ajustes" (Settings). h-10 header: "Ajustes de tabla" (Table settings) if `fmt.inTable`,
otherwise "Ajustes de párrafo" (Paragraph settings); `close` button → `setUi({settingsOpen:false})`.

- **Paragraph section** (always): "Interlineado" (Line spacing; `@sytue/ui` `Select` →
  `exec('lineHeight', v)`) · "Alineación" (Alignment; 4 RibbonButtons, active according to
  `fmt.align`) · "Sangría" (Indent; outdent/indent).
- **Table section** (only `fmt.inTable`, above Paragraph): full-width button
  rows (h-8, icon + text, `hover:bg-bg-subtle rounded-sm`):
  "Insertar fila encima/debajo" (Insert row above/below; `rowAbove/rowBelow`), "Insertar columna a la
  izquierda/derecha" (Insert column left/right; `colLeft/colRight`), "Eliminar fila" / "Eliminar
  columna" / "Eliminar tabla" (Delete row / column / table; `trash`, the last one `text-danger`) → the
  `table*` commands in §3.
  `// ponytail: sin espaciado antes/después ni anchos de columna en v1.`

### 6.7 StatusBar (B1) — 28px

`h-[var(--editor-status-h)] bg-surface border-t border-border px-3 flex
items-center gap-4 text-xs text-text-muted`. Left side depending on kind:
document → "Página {page} de {pages}" (Page {page} of {pages}) · "Palabras: {words}" (Words) · "Español
(España)" (Spanish (Spain)); spreadsheet → "Hoja {page} de {pages}" (Sheet … of …); presentation →
"Diapositiva {page} de {pages}" (Slide … of …). Right side (`ml-auto flex items-center gap-1`):
`minus` button "Reducir" (Zoom out; −10) · `<input type="range" min="50" max="200"
step="10" aria-label="Zoom">` (w-24, `accent-color: var(--sytue-accent)`) ·
text "{zoom} %" (w-10, `tabular-nums`) · `plus` button "Ampliar" (Zoom in; +10).

<a id="68-atajos-globales"></a>

### 6.8 Global shortcuts

| Shortcut           | Action                    | Where                                |
| ------------------ | ------------------------- | ------------------------------------ |
| Ctrl/Cmd+B · I · U | bold · italic · underline | canvas onKeyDown (§6.5)              |
| Ctrl/Cmd+Mayús+X   | strikethrough             | same                                 |
| Ctrl/Cmd+Z · Y     | undo · redo               | the contenteditable's native history |

RibbonButton `shortcut`s are documentation only (title); they do not register listeners.

---

<a id="7-variantes-sheets-y-slides-b11--mismo-chrome-mismo-guardado"></a>

## 7. Sheets and slides variants (B11) — SAME chrome, same saving

Both use the same header/ribbon/rail/status; only the canvas that
`EditorApp` mounts according to `ctx.kind` changes. They persist through the SAME path: they serialize their
model with `JSON.stringify` and call `ctx.onContentInput(json)`; they load with
`JSON.parse(ctx.content || 'null') ?? modeloVacío` on every `contentEpoch`.
Invalid parse → empty model + toast. They register the focused cell/box with
`setEditorRoot` on `focusin` (so Inicio/exec format text inside the
cell or box) and `setEditorRoot(null)` on unmount.

<a id="71-sheetcanvas--kindspreadsheet-clave-docsheet"></a>

### 7.1 SheetCanvas — `kind==='spreadsheet'` (key `doc:sheet`)

Empty model: `{ active: 0, sheets: [{ name: 'Hoja 1', cells: {} }] }`.

- **Formula bar** (h-7, under the ribbon, `bg-surface border-b
border-border px-2 flex items-center gap-2 text-sm`): reference box
  (w-16, `text-text-muted`, e.g. "B4") + `<input aria-label="Barra de
fórmulas">` flex-1 with the TEXT of the selected cell; Enter writes the
  text (escaping HTML) into the cell and moves down one row.
  `// ponytail: sin fórmulas (=SUMA…) en v1; solo valores.`
- **A..Z × 1..100 grid** in the scrolling area: corner + column headers
  (sticky top) and row headers (sticky left), `bg-bg-subtle text-xs
text-text-muted border-border`, with the selected cell's headers highlighted.
  Cells: `contenteditable` div 96×24, `border-r border-b border-border
bg-white` (ALWAYS white: paper), `text-sm px-1`, content =
  `cells[ref] ?? ''` (innerHTML).
- **Selection/editing**: states `selected {c,r}` and `editing: boolean`. Click =
  select (`2px var(--sytue-accent)` inset ring); double click, Enter or
  typing = edit (focus + caret); Escape = back to selection; arrows (when not
  editing) move; Tab/Enter confirm and advance (right/down). Commit (blur or
  confirmation): if `innerHTML` changed → update `cells[ref]` (empty →
  `delete`) → `onContentInput(JSON.stringify(model))`.
- **Sheet tabs** (bottom strip h-7, `bg-surface border-t
border-border px-2 flex gap-1 text-xs`): one tab per sheet (active
  `font-medium text-text` + `bg-accent` underline; inactive
  `text-text-muted`), double click renames (prompt), `plus` button "Añadir
  hoja" (Add sheet; name "Hoja N"). Switching sheet → `model.active` + commit.
- `reportStats({ pages: sheets.length, page: active + 1 })`.
- The grid is `role="grid"` with `aria-rowindex/aria-colindex` on cells;
  headers `role="columnheader"/"rowheader"`.
  `// ponytail: 26×100 celdas fijas renderizadas de golpe (~2600 divs, va sobrado); virtualizar solo si se amplía la rejilla.`

<a id="72-slidescanvas--kindpresentation-clave-docslides"></a>

### 7.2 SlidesCanvas — `kind==='presentation'` (key `doc:slides`)

Empty model: one slide with a title box
`{ id, boxes: [{ id, x: 10, y: 35, w: 80, h: 20, html: '<h1>Haz clic para añadir título</h1>' }] }`
(ids with `crypto.randomUUID()`).

- **Thumbnail rail** (left, w-40 = 160px, `bg-surface border-r
border-border overflow-y-auto p-2 flex flex-col gap-2`): 16:9 thumbnail
  (128×72 box: 960×540 interior with `transform: scale(128/960)` origin
  top-left, `pointer-events-none`, boxes painted read-only). Active: ring
  `2px var(--sytue-accent)`; order number in the corner. Buttons: `plus`
  "Nueva diapositiva" (New slide) and `trash` "Eliminar diapositiva" (Delete slide; min. 1, `confirm`).
- **Canvas**: centered on `var(--editor-canvas-bg)`; 16:9 stage, base
  960×540, scaled by `min(anchoDisponible/960, altoDisponible/540) ·
zoom/100`, white background, shadow `var(--editor-page-shadow)`.
- **Text boxes**: absolute div (x/y/w/h in %), `contenteditable` interior.
  Click = select (`1px dashed var(--sytue-accent)` border + top move
  handle + SE resize handle); double click = edit text; dragging the
  handles with Pointer Events (`setPointerCapture`) updates x/y or w/h (clamp
  0..100); Supr (Delete) with a box selected (not editing) removes it. A "Cuadro
  de texto" (Text box) button (`textBox`) in a mini-bar above the stage adds a
  centered box. Every change → model commit → `onContentInput`.
- `reportStats({ pages: slides.length, page: activa + 1 })`.
  `// ponytail: sin imágenes ni formas en slides v1; solo cajas de texto.`

---

<a id="8-srceditoreditorcss-a1-lo-pega-tal-cual-único-fichero-con-hex-de-ui"></a>

## 8. `src/editor/editor.css` (A1 pastes it verbatim; the ONLY file with UI hex values)

```css
/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tokens locales del editor Sytue Docs (BLUEPRINT.md §8) + estilos de contenido
 * de la página. Únicos hex nuevos permitidos: el cromo usa tokens --sytue-*.
 */
.sytue-editor {
  /* Lienzo gris neutro sobre el que flota la página blanca (estructura OnlyOffice, piel Sytue). */
  --editor-canvas-bg: #e7eaf0;
  --editor-page-bg: #ffffff; /* papel: blanco también en modo oscuro */
  --editor-page-shadow: 0 1px 2px rgba(15, 23, 42, 0.1), 0 6px 20px rgba(15, 23, 42, 0.12);
  --editor-ruler-margin: rgba(15, 23, 42, 0.08);
  /* Tinta de la regla: FIJA, nunca tokens de tema — se pinta sobre papel blanco. */
  --editor-ruler-ink: rgba(40, 36, 30, 0.8);
  --editor-ruler-ink-hover: rgb(40, 36, 30);
  --editor-ruler-border: rgba(40, 36, 30, 0.16);

  /* Métricas del cromo (§6). */
  --editor-header-h: 48px;
  --editor-tabs-h: 32px;
  --editor-groups-h: 68px;
  --editor-status-h: 28px;
  --editor-ruler-h: 20px;
  --editor-rail-w: 40px;
  --editor-panel-w: 288px;

  /* Página A4 a zoom 100 (96 dpi). */
  --editor-page-w: 794px;
  --editor-page-h: 1123px;
  --editor-page-margin: 96px;
}

[data-theme='dark'] .sytue-editor {
  --editor-canvas-bg: #16181d;
  --editor-page-shadow: 0 1px 2px rgba(0, 0, 0, 0.5), 0 8px 24px rgba(0, 0, 0, 0.45);
}
@media (prefers-color-scheme: dark) {
  :root:not([data-theme='light']) .sytue-editor {
    --editor-canvas-bg: #16181d;
    --editor-page-shadow: 0 1px 2px rgba(0, 0, 0, 0.5), 0 8px 24px rgba(0, 0, 0, 0.45);
  }
}

/* ---- La página (folio): tipografía de CONTENIDO sobre papel blanco. ---- */
.sytue-page {
  background: var(--editor-page-bg);
  box-shadow: var(--editor-page-shadow);
  color: #0a0a0a; /* texto de contenido: negro de marca, fijo (el papel es blanco siempre) */
  font-family: var(--font-sans);
  font-size: 16px;
  line-height: 1.5;
  caret-color: var(--sytue-accent);
  outline: none; /* el caret es el indicador de foco del folio */
  word-wrap: break-word;
}
.sytue-page ::selection {
  background: rgba(249, 115, 22, 0.25);
}

.sytue-page h1 {
  font-size: 2em;
  font-weight: 700;
  margin: 0.67em 0 0.35em;
}
.sytue-page h2 {
  font-size: 1.5em;
  font-weight: 600;
  margin: 0.83em 0 0.35em;
}
.sytue-page h3 {
  font-size: 1.17em;
  font-weight: 600;
  margin: 1em 0 0.35em;
}
.sytue-page p {
  margin: 0 0 0.5em;
}
.sytue-page ul,
.sytue-page ol {
  margin: 0 0 0.5em;
  padding-left: 1.5em;
  list-style: revert;
}
.sytue-page a {
  color: #2563eb;
  text-decoration: underline;
}
.sytue-page hr {
  border: none;
  border-top: 1px solid #cbd5e1;
  margin: 1em 0;
}
.sytue-page img {
  max-width: 100%;
  height: auto;
}
.sytue-page table {
  border-collapse: collapse;
  width: 100%;
  table-layout: fixed;
  margin: 0 0 0.5em;
}
.sytue-page td,
.sytue-page th {
  border: 1px solid #cbd5e1;
  padding: 4px 8px;
  vertical-align: top;
}
.sytue-page nav[data-toc] {
  border: 1px dashed #cbd5e1;
  padding: 8px 12px;
  margin: 0 0 0.5em;
}
```

`EditorApp.tsx` does `import './editor.css';`. Tailwind detects the classes in
`src/editor/*.tsx` automatically (the Vite plugin's source detection).

---

<a id="9-iconos--srceditoriconstsx-a3"></a>

## 9. Icons — `src/editor/icons.tsx` (A3)

Same pattern as `@sytue/ui`'s `Icon` (do NOT touch packages/ui; OWN paths,
drawn from scratch): `viewBox="0 0 24 24"`, `fill="none"`,
`stroke="currentColor"`, `stroke-width={1.75}`, `stroke-linecap/linejoin="round"`,
decorative unless `label`.

```ts
export type EditorIconName =
  | 'undo'
  | 'redo'
  | 'bold'
  | 'italic'
  | 'underline'
  | 'strike'
  | 'textColor'
  | 'highlight'
  | 'clearFormat'
  | 'alignLeft'
  | 'alignCenter'
  | 'alignRight'
  | 'alignJustify'
  | 'listBullet'
  | 'listNumber'
  | 'indent'
  | 'outdent'
  | 'lineHeight'
  | 'table'
  | 'image'
  | 'link'
  | 'hr'
  | 'toc'
  | 'comment'
  | 'history'
  | 'share'
  | 'download'
  | 'search'
  | 'outline'
  | 'ruler'
  | 'panelRight'
  | 'plus'
  | 'minus'
  | 'close'
  | 'check'
  | 'chevronDown'
  | 'chevronLeft'
  | 'trash'
  | 'rowAbove'
  | 'rowBelow'
  | 'colLeft'
  | 'colRight'
  | 'pagePortrait'
  | 'pageLandscape'
  | 'slideAdd'
  | 'textBox';

export interface EditorIconProps {
  name: EditorIconName;
  size?: number; // por defecto 18
  label?: string; // ausente = aria-hidden
}
export function EditorIcon(props: EditorIconProps): JSX.Element;
```

Multi-path allowed (array of `d` per name). Aesthetics: simple line
geometry, rounded corners, consistent with the design system.

---

<a id="10-reparto-por-constructor-ficheros-disjuntos"></a>

## 10. Allocation by builder (DISJOINT files)

| Who     | Files (single owner)                                                                                           | Depends on (contracts)                                         | Notes                                                                                                                                                                                              |
| ------- | -------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A1**  | `types.ts`, `EditorApp.tsx`, `editor.css`, `src/pages/edit.astro`, `test/e2e/tests/docs-editor.spec.ts`        | §2 (publishes it), A2 (hook), B5 (getFormatState)              | Pushes `types.ts`+`editor.css` FIRST (pasting them from §2/§8); integrates `EditorApp` and the `edit.astro` swap at the end; writes the e2e smoke test (§11) following `test/e2e/tests/helpers.ts` |
| **A2**  | `state/useDocument.ts`, `src/lib/docsProxy.ts`, `src/pages/api/{state,ops,rename,comments,versions,export}.ts` | §2, §4                                                         | The complete data layer; verifiable with curl (contract §5)                                                                                                                                        |
| **A3**  | `icons.tsx`, `ribbon/primitives.tsx`                                                                           | §2, §5, §9                                                     | Blocks B4 and B7–B10: land early                                                                                                                                                                   |
| **B1**  | `layout/EditorLayout.tsx`, `layout/StatusBar.tsx`                                                              | §2, §6.7, §8                                                   | Pure layout + status bar                                                                                                                                                                           |
| **B2**  | `layout/EditorHeader.tsx`                                                                                      | §2, §6.1, A3 (icons), `@sytue/ui` (Button, Dropdown, useToast) | Title, chip, peers, Share, Download                                                                                                                                                                |
| **B3**  | `layout/LeftRail.tsx`                                                                                          | §2, §6.4, A3, B5 (`getEditorRoot`)                             | Rail + search/outline/comments                                                                                                                                                                     |
| **B4**  | `layout/SettingsPanel.tsx`                                                                                     | §2, §6.6, A3                                                   | Paragraph + contextual Table                                                                                                                                                                       |
| **B5**  | `canvas/exec.ts`                                                                                               | §2, §3                                                         | The editing brain; no components                                                                                                                                                                   |
| **B6**  | `canvas/PageCanvas.tsx`, `canvas/Ruler.tsx`                                                                    | §2, §6.5, §8, B5                                               | A4 page + ruler                                                                                                                                                                                    |
| **B7**  | `ribbon/Ribbon.tsx`                                                                                            | §2, §6.2, A3, tabs (import)                                    | Tab strip + Archivo (File) menu                                                                                                                                                                    |
| **B8**  | `ribbon/tabs/Inicio.tsx`                                                                                       | §2, §5, §6.3                                                   | The densest tab                                                                                                                                                                                    |
| **B9**  | `ribbon/tabs/Insertar.tsx`, `ribbon/tabs/Referencias.tsx`                                                      | §2, §5, §6.3                                                   | Includes the 6×6 table popover                                                                                                                                                                     |
| **B10** | `ribbon/tabs/Disposicion.tsx`, `ribbon/tabs/Vista.tsx`, `ribbon/tabs/Colaboracion.tsx`                         | §2, §5, §6.3, `@sytue/ui` (Dropdown)                           | Three lightweight tabs                                                                                                                                                                             |
| **B11** | `sheets/SheetCanvas.tsx`, `slides/SlidesCanvas.tsx`                                                            | §2, §7, B5 (`setEditorRoot`)                                   | The two variants; the heaviest batch                                                                                                                                                               |

Integration rules: cross imports go ONLY towards `types.ts`,
`icons.tsx`, `primitives.tsx` and `canvas/exec.ts` (plus the composition imports
of `EditorApp`/`Ribbon`/`EditorLayout`, which their owners do).
Nobody imports another builder's files outside that list.

---

<a id="11-definición-de-hecho"></a>

## 11. Definition of done

1. `(cd apps/frontend && bunx astro check --root packages/office)` green after
   A1's integration (each builder: zero errors of their own).
2. Playwright smoke test (A1, `test/e2e/tests/docs-editor.spec.ts`, following the pattern of
   `docs.spec.ts`/`helpers.ts`, `ignoreHTTPSErrors: true`, login
   `admin@sytue.localhost` / `sytue-dev-admin`): create a document (POST
   `/api/create` or UI) → open `/edit?id=` → type "Hola mundo" on the page
   → click the Negrita (Bold) button and type → wait for the "Guardado" (Saved) chip → `reload()`
   → the text persists (and the bold fragment is still in `<b>`/`<span>` with
   `font-weight`). This is THE verification of the persistence contract.
3. AA: keyboard navigable (ribbon tabs with arrow keys, all
   controls focusable with visible focus), `aria-label` on every icon-button,
   `aria-pressed` on toggles, `aria-live` on the save chip.
4. No console errors on load and normal editing; no requests from the
   browser to `docs-api.sytue.localhost` (everything same-origin).
5. No new dependencies in `package.json`; no files outside the allocation.

Out of scope for v1 (explicit, do not attempt it): WebSocket/live cursors,
real visual pagination, spreadsheet formulas, fine-grained permissions/sharing,
per-character comment anchors, dragging indents on the ruler.
