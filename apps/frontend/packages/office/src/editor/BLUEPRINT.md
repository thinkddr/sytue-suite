# BLUEPRINT — Editor Sytue Docs (v1)

Especificación normativa para 14 constructores en paralelo (A1–A3, B1–B11).
Este documento ES el contrato: las interfaces de §2–§5 se copian tal cual a
`src/editor/types.ts` y ficheros indicados. Ante duda, gana el CONTRATO TÉCNICO
del auditor (2026-07-03) y después este blueprint. No es código importable.

---

## 0. Reglas de oro

1. **Clean-room estricto.** OnlyOffice es AGPL: PROHIBIDO copiar su código, CSS,
   nombres de clases, iconos o assets. Aquí se replica la _experiencia_ descrita
   en §6 con marca Sytue. Nada de mirar su repositorio.
2. **Cada fichero tiene UN dueño** (§10). No toques ficheros de otro constructor.
   PROHIBIDO tocar `src/components/{DocList,Shell,DocsEditor}.tsx`,
   `src/pages/api/{documents,create,collab}.ts`, `src/lib/{auth,token}.ts`,
   `packages/ui` y cualquier cosa fuera de `apps/frontend/packages/office/`
   (excepción: A1 edita `src/pages/edit.astro`; A2 crea rutas nuevas bajo
   `src/pages/api/` y `src/lib/docsProxy.ts`; A1 puede añadir un spec en
   `test/e2e/tests/`). Si un fichero ajeno cambia bajo tus pies, reléelo y
   fusiona; JAMÁS reviertas trabajo ajeno. NO `git commit/push`.
3. **Nada de fetch directo del navegador a `docs-api.sytue.localhost`** (no hay
   CORS) y **nada de WebSocket** (el handshake del navegador recibe 401 —
   verificado). Todo pasa por los proxies BFF same-origin de §4 y la
   colaboración es _polling_ REST.
4. **Sin dependencias nuevas.** Preact + `@sytue/ui` + Tailwind v4 ya instalados.
   Iconos del editor: locales en `src/editor/icons.tsx` (§9).
5. **Colores:** utilidades de token (`bg-surface`, `text-text`, `border-border`,
   `bg-bg-subtle`, `bg-selected`, `text-text-muted`, `text-text-subtle`,
   `--sytue-focus-ring`…). Hex nuevos SOLO los declarados en `editor.css` (§8)
   y las paletas de CONTENIDO de §5 (colores que el usuario aplica a su texto).
6. **Accesibilidad AA:** todo icon-button con `aria-label` + `title`; toggles con
   `aria-pressed`; pestañas con roles `tablist/tab/tabpanel` y flechas; estado
   de guardado con `aria-live="polite"`; foco visible siempre
   (el aro global o una variante `sytue-focus-ring*` de `@sytue/ui`; nunca
   `outline:none` sin reemplazo, ni `focus-visible:shadow-[…]`, que no pinta).
7. **Español correcto con tildes** en toda cadena visible y comentario.
8. Cabecera SPDX en cada fichero nuevo, como el resto del repo:
   ```
   /*
    * SPDX-License-Identifier: Apache-2.0
    * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
    */
   ```
9. Typecheck: `export PATH="/opt/homebrew/bin:/Users/ddr/.npm-global/bin:$PATH"`
   y `(cd apps/frontend && bunx astro check --root packages/office)`. El check global
   solo estará verde cuando A1, A3 y B5 hayan aterrizado; cada constructor
   responde de que SUS ficheros no aporten errores propios.
10. Atajos deliberados se marcan con comentario `// ponytail: …` indicando techo
    y camino de mejora.

---

## 1. Estructura de ficheros (todo bajo `apps/frontend/packages/office/`)

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

`src/components/DocsEditor.tsx` queda sin montar; NO se borra (limpieza en otra
fase). Orden de aterrizaje recomendado: A1 sube `types.ts` + `editor.css` de
inmediato; A3 y B5 después; A1 integra `EditorApp.tsx` + `edit.astro` al final.
Mientras tanto, todos codifican contra las interfaces de este documento.

---

## 2. Contratos compartidos — `src/editor/types.ts` (íntegro, A1 lo pega tal cual)

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

**Firma universal de zona y pestaña** (todas idénticas, sin excepción):

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

`EditorLayout` es la única con props propias (slots):

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

`EditorApp` (raíz de isla): `export function EditorApp({ documentId }: { documentId: string }): JSX.Element`.

---

## 3. API de comandos — `src/editor/canvas/exec.ts` (B5)

El HTML del lienzo ES el modelo. Implementación: `contenteditable` +
`document.execCommand`, con fallbacks de `Selection`/`Range` donde execCommand
no llega. Módulo sin componentes; API pública exacta:

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

Detalles internos obligatorios:

- **Memoria de selección:** listener módulo-global de `selectionchange`; si el
  `anchorNode` está dentro del root, clona y guarda el `Range`. `exec()` lo
  restaura cuando `document.activeElement` no está dentro del root. Así las
  primitivas no gestionan selección.
- Al registrar el root: `document.execCommand('styleWithCSS', false, 'true')`
  (envuelto en try/catch), y `defaultParagraphSeparator` = `'p'`.

### 3.1 Mapa de comandos

| Comando                            | Implementación                                                                                                                                                                                                                                                           |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| bold / italic / underline / strike | `execCommand('bold'\|'italic'\|'underline'\|'strikeThrough')`                                                                                                                                                                                                            |
| font                               | `execCommand('fontName', false, value)`                                                                                                                                                                                                                                  |
| size                               | `execCommand('fontSize', false, value)` con value `'1'..'7'` (§3.2)                                                                                                                                                                                                      |
| color                              | `execCommand('foreColor', false, value)`                                                                                                                                                                                                                                 |
| highlight                          | `execCommand('hiliteColor', …)`; si lanza, fallback `backColor`; `'transparent'` quita el resaltado                                                                                                                                                                      |
| clearFormat                        | `execCommand('removeFormat')`                                                                                                                                                                                                                                            |
| alignLeft/Center/Right/Justify     | `execCommand('justifyLeft'\|'justifyCenter'\|'justifyRight'\|'justifyFull')`                                                                                                                                                                                             |
| listBullet / listNumber            | `execCommand('insertUnorderedList'\|'insertOrderedList')`                                                                                                                                                                                                                |
| indent / outdent                   | `execCommand('indent'\|'outdent')`                                                                                                                                                                                                                                       |
| lineHeight                         | Range: para cada bloque de `blocksInSelection()` (p, h1–h3, li, td), `style.lineHeight = value`                                                                                                                                                                          |
| heading                            | `execCommand('formatBlock', false, value === 'p' ? '<P>' : '<H1>'…)`                                                                                                                                                                                                     |
| insertTable                        | `execCommand('insertHTML')` con `<table><tbody>` de F filas × C columnas, cada celda `<td><br></td>`, seguido de `<p><br></p>`                                                                                                                                           |
| insertImage                        | `execCommand('insertImage', false, dataUrl)`                                                                                                                                                                                                                             |
| insertLink                         | `execCommand('createLink', false, url)` (solo con selección no colapsada; si está colapsada, insertHTML `<a href>url</a>`)                                                                                                                                               |
| insertHr                           | `execCommand('insertHorizontalRule')`                                                                                                                                                                                                                                    |
| insertToc                          | Escanea `h1,h2,h3` del root, asigna `id="h-<n>"` a los que no tengan, elimina `nav[data-toc]` previo e inserta en el cursor `<nav data-toc><p><b>Tabla de contenidos</b></p><ol><li data-level="…"><a href="#id">texto</a></li>…</ol></nav>`                             |
| tableRow*/tableCol*/tableDel*      | Range: desde `anchorNode` → `closest('td')`/`closest('tr')`/`closest('table')` dentro del root; clona/inserta/elimina fila o columna por índice de celda; borrar la última fila/columna elimina la tabla; `tableDelete` elimina la tabla. Sin selección en tabla → no-op |
| undo / redo                        | `execCommand('undo'\|'redo')` (historial nativo del contenteditable)                                                                                                                                                                                                     |

`// ponytail: execCommand está deprecado pero es interoperable y sin dependencias;
// migrar a un modelo propio de Range solo si un navegador soportado lo retira.`

### 3.2 Escala de tallas (UI ↔ execCommand)

| UI (pt) | 8   | 10  | 12  | 14  | 18  | 24  | 36  |
| ------- | --- | --- | --- | --- | --- | --- | --- |
| value   | '1' | '2' | '3' | '4' | '5' | '6' | '7' |

El select de talla del ribbon muestra los pt y envía `'1'..'7'`;
`getFormatState().size` devuelve `'1'..'7'` (de `queryCommandValue('fontSize')`).

### 3.3 getFormatState

`queryCommandState` para bold/italic/underline/strikeThrough/insertUnorderedList/
insertOrderedList/justify*; `queryCommandValue` para fontName (quitar comillas),
fontSize y formatBlock (→ heading, por defecto 'p'); `queryCommandEnabled('undo'|'redo')`;
`inTable` = ancestro `table` del anchorNode dentro del root. Todo en try/catch;
sin selección dentro del root → devolver `EMPTY_FORMAT`.

---

## 4. Datos — `state/useDocument.ts` + proxies BFF (A2)

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

Implementación: `bff.getSession(toBffContext(ctx))` → si falta,
`{"error":"unauthenticated"}` 401; `fetch(docsBaseUrl() + '/api/v1' + path, …)`
con `authorization: Bearer <session.tokens.accessToken>`; devuelve `Response`
con el MISMO status y el cuerpo textual (204 → cuerpo null),
`content-type: application/json; charset=utf-8`, `cache-control: no-store`.
**Todo parámetro interpolado en `path` pasa por `encodeURIComponent`** (frontera
de confianza). `since`/`limit` se validan numéricos. `export const prerender = false;`
en cada ruta.

### 4.2 Rutas BFF (mapeo exacto)

| Ruta BFF (same-origin)                                    | Upstream Go (`/api/v1`)                                                                                                                              |
| --------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET /api/state?id=`                                      | `GET /documents/{id}/state`                                                                                                                          |
| `POST /api/ops?id=` (body tal cual)                       | `POST /documents/{id}/ops`                                                                                                                           |
| `GET /api/ops?id=&since=&limit=`                          | `GET /documents/{id}/ops?since=&limit=`                                                                                                              |
| `POST /api/rename` body `{id,title}`                      | `PATCH /documents/{id}` body `{title}` → 204                                                                                                         |
| `GET /api/comments?id=&resolved=`                         | `GET /documents/{id}/comments[?resolved=true]`                                                                                                       |
| `POST /api/comments?id=` body `{body,parent_id?,anchor?}` | `POST /documents/{id}/comments` → 201                                                                                                                |
| `PUT /api/comments?id=&commentId=` body `{resolved}`      | `PUT /documents/{id}/comments/{commentId}/resolve` → 204                                                                                             |
| `DELETE /api/comments?id=&commentId=`                     | `DELETE /documents/{id}/comments/{commentId}` → 204                                                                                                  |
| `GET /api/versions?id=&limit=`                            | `GET /documents/{id}/versions?limit=`                                                                                                                |
| `POST /api/versions?id=` body `{label?}`                  | `POST /documents/{id}/versions` → 201                                                                                                                |
| `POST /api/versions?id=&restore=<versionId>`              | `POST /documents/{id}/versions/{versionId}/restore`                                                                                                  |
| `POST /api/export?id=` body `{format}`                    | `POST /documents/{id}/exports` → 201 `{export:{id}}`, luego `GET /documents/{id}/exports/{blobId}` → responde `{url,content_type,size_bytes,format}` |

Falta `id` u otro parámetro obligatorio → `{"error":"missing_id"}` 400. Los
cuerpos Go se reenvían VERBATIM (snake_case llega al cliente; los tipos de §2 lo
reflejan). `/api/documents` (camelCase) y `/api/create` existentes NO se tocan.

### 4.3 Hook `useDocument` — firma

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

### 4.4 Algoritmo (contrato del auditor §5, obligatorio)

- **Carga:** en paralelo `GET /api/documents?id=` (meta camelCase) y
  `GET /api/state?id=`. `content = String(state.registers[CONTENT_KEY[kind]]?.value ?? '')`,
  `lamport = state.state.lamport`, `lastSeq = seq`, `actor = crypto.randomUUID()`,
  `counter = 0`, `contentEpoch = 1`. También `GET /api/comments?id=` (no bloquea
  el `ready`). Cualquier fallo de meta/state → `status: 'error'`.
- **Autosave:** `onContentInput(html)` guarda el HTML en un **ref** (¡no en
  estado que re-renderice el lienzo!), pone `save='dirty'` y re-arma un debounce
  de **1500 ms**. Al disparar: `save='saving'` y
  `POST /api/ops?id=` body
  `{"ops":[{"kind":"set","dot":{"a":actor,"c":++counter},"key":CONTENT_KEY[kind],"value":html,"lamport":++lamport}]}`.
  200 → `lastSeq = seq`, `save='saved'`, recalcular `words`
  (`html.replace(/<[^>]+>/g,' ').trim().split(/\s+/).filter(Boolean).length`,
  solo kind document; resto 0). Red/5xx → `save='error'` y reintento a los 5 s
  (mismo cuerpo, MISMO counter/lamport no: recalcula `++counter`/`++lamport` en
  cada intento para monotonicidad). 401 → `save='error'` sin reintento (chip
  "Sesión caducada — recarga la página"). 4xx restantes → re-GET state una vez,
  adoptar lamport/lastSeq y reintentar.
- **Título:** `setTitle` optimista + debounce 800 ms → `POST /api/rename`
  `{id,title}`; error → revertir + toast. Efecto aparte:
  `document.title = \`${title} — Sytue Docs\``.
- **Contenido (colaboración, sin WS):** cada **4000 ms**, solo con
  `document.visibilityState === 'visible'`: `GET /api/ops?id=&since=${lastSeq}`.
  Por cada `SequencedOp`: `lastSeq = max(lastSeq, so.seq)`; si `so.actor !== actor`
  y además `so.op.kind==='set' && so.op.key===CONTENT_KEY[kind] &&
so.op.lamport > lamport` → **adoptar**: `lamport = so.op.lamport`,
  `content = String(so.op.value)`, `contentEpoch++` — SOLO si `save==='saved'`
  (si hay edición local en vuelo, gana el próximo autosave: LWW).
- **Presencia REAL por conexión:** cada **10000 ms** `PUT /api/presence?id=`
  `{conn: actor}`; el servicio Go registra la pestaña (TTL 30 s, en memoria) y
  devuelve `{members:[{conn,sub}]}`. `peers` = miembros con `conn !== actor`,
  deduplicados por `sub`. Cuenta al LECTOR PASIVO (abrir basta; no exige ops).
  `// ponytail: adoptar recoloca el caret al principio; cursores finos llegarán con la migración a RGA.`
- **flush():** si hay autosave pendiente,
  `navigator.sendBeacon('/api/ops?id='+id, new Blob([body], {type:'application/json'}))`
  (cookies same-origin viajan). Registrar en `pagehide`.
- **Comentarios:** `addComment` → `POST /api/comments?id=` `{body, anchor: null}`
  (`// ponytail: anclas por carácter cuando haya RGA`); mutaciones re-consultan
  la lista. **Versiones:** `loadVersions` bajo demanda; `createVersion` →
  POST; `restoreVersion` → POST restore y después re-GET `/api/state` para
  adoptar contenido/lamport/lastSeq con `contentEpoch++`. **exportAs** → POST
  `/api/export` → `window.open(url, '_blank')`; error → toast "No se pudo exportar".

`EditorApp` (A1) compone: `useDocument` + estado local `ui`, `pageSetup`,
`fmt` (listener `selectionchange` con throttle 100 ms → `getFormatState()` si la
selección está en el root), `stats` (merge de `words` del hook con
`reportStats` de los lienzos) → construye `ctx: EditorContext` y lo pasa a las
zonas. Envuelve todo en `<ToastProvider>` de `@sytue/ui`. Estados `loading` /
`error` como el DocsEditor actual (Spinner / EmptyState + botón a `/`).

---

## 5. Primitivas del ribbon — `src/editor/ribbon/primitives.tsx` (A3)

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

Receta de clases del `RibbonButton` (uniformidad; el resto de primitivas en el
mismo lenguaje):

```
inline-flex h-7 w-7 items-center justify-center rounded-sm text-text-muted
hover:bg-bg-subtle hover:text-text aria-pressed:bg-selected aria-pressed:text-text
disabled:pointer-events-none disabled:opacity-40
sytue-focus-ring
```

Cada muestra del ColorSwatch es un `<button aria-label="Color #F97316">` de
20×20 con `border border-border` (y `check` sobre el color actual). Popover:
`absolute top-full mt-1 z-[var(--z-popover)] rounded-md border border-border
bg-surface p-2 shadow-[var(--shadow-lg)]`; se cierra con Escape y clic fuera.

---

## 6. Blueprint visual por zona

Contenedor raíz (`EditorLayout`): `class="sytue-editor flex h-full min-h-0
flex-col bg-surface text-text"`. El editor vive bajo la topbar de la Shell
(56px): `edit.astro` ya reserva `h-[calc(100vh-var(--spacing-topbar))]`.
Estructura vertical: cabecera 48 · pestañas 32 · grupos 68 · [franja 40 |
panel-izq 288? | lienzo flex-1 | ajustes 288?] · estado 28. Alturas/anchos
salen de las vars de §8 (`h-[var(--editor-header-h)]`, etc.).

### 6.1 EditorHeader (B2) — 48px, `bg-surface border-b border-border px-3 gap-2`

De izquierda a derecha:

1. Botón volver (`chevronLeft`, aria-label "Volver a tus documentos", href `/`).
2. **Título editable**: `<input id="editor-title-input" aria-label="Título del
documento">` sin borde (`bg-transparent text-sm font-semibold text-text
hover:bg-bg-subtle focus:bg-bg-subtle rounded-sm px-1.5 h-7 max-w-[360px]`),
   `value={ctx.title}`, onInput → `ctx.setTitle`. Enter/blur confirman.
3. **Chip de guardado** `aria-live="polite"` (`text-xs text-text-muted`):
   saved → "Guardado" (punto `bg-success`) · saving → "Guardando…"
   (`bg-warning`) · dirty → "Cambios sin guardar" (`bg-warning`) · error →
   "Error al guardar" (`bg-danger`).
4. `flex-1`.
5. **Colaboradores**: círculos 24px solapados (máx. 4, `-ml-1.5`,
   `rounded-full bg-bg-subtle border border-border text-[10px]`) con
   `peer.sub.slice(0,2).toUpperCase()` y `title="Colaborador conectado"`.
   `// ponytail: nombres reales cuando exista lookup de identidad.`
6. **Compartir**: `Button variant="primary" size="sm"` → popover propio:
   texto "Solo pueden abrirlo usuarios con permiso." + botón "Copiar enlace"
   (`navigator.clipboard.writeText(location.href)` + toast "Enlace copiado").
   `// ponytail: permisos finos y enlaces de invitado en fase 2 (API ya existe).`
7. **Descargar**: `Dropdown` de `@sytue/ui` (`Button variant="secondary"
size="sm"` + `download`): MenuItems "PDF", "HTML", "Markdown", "ODF (.odt)",
   "OOXML (.docx)" → `ctx.exportAs('pdf'|'html'|'markdown'|'odf'|'ooxml')`.

### 6.2 Ribbon (B7) — pestañas 32px + grupos 68px

**Tira de pestañas** (`h-[var(--editor-tabs-h)] bg-surface border-b
border-border px-2 flex items-end gap-1`, `role="tablist"`
aria-label "Cinta de opciones"): primera "posición" es **Archivo** (NO es tab:
botón que abre `Dropdown` con "Descargar como PDF/HTML/Markdown", "Guardar
versión", "Renombrar" (foco a `#editor-title-input`), "Volver a Documentos"
(href `/`)). Después las pestañas reales, filtradas por kind.

> **Vigente desde cinta-4 (2026-10-02):** la lista de pestañas y los tipos en que se
> ven viven en `pestanas.json` (copia de `apps/nucleo/office/pestanas.json`); Inicio y
> Revisar siempre, Insertar y Diseño en documento y presentación, Datos y Fórmulas en la
> hoja, Presentar en las diapositivas. El bloque de abajo es el de la primera versión
> y se conserva como historia.

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
hover:text-text` · activa: `text-text font-medium` + subrayado 2px
`bg-accent` (naranja de marca) pegado al borde inferior. Flechas ←/→ mueven
la pestaña activa (foco roving), Home/End extremos. Estado `useState` local
(inicial `'inicio'`).

**Fila de grupos** (`h-[var(--editor-groups-h)] bg-surface border-b
border-border px-2 flex items-stretch overflow-x-auto`, `role="tabpanel"`):
renderiza `<Body ctx={ctx}/>` de la pestaña activa. Los tabs componen
`RibbonGroup` + `RibbonSeparator` entre grupos.

### 6.3 Contenido de cada pestaña

**Inicio (B8):**

1. Grupo "Deshacer": `undo` (Ctrl+Z, disabled `!fmt.canUndo`), `redo` (Ctrl+Y,
   disabled `!fmt.canRedo`).
2. Grupo "Fuente": RibbonSelect fuente (Inter, Arial, Georgia, Times New Roman,
   Courier New, JetBrains Mono; width 132; value `fmt.font` si coincide) ·
   RibbonSelect talla (§3.2, width 64) · `bold` (Ctrl+B, active `fmt.bold`) ·
   `italic` (Ctrl+I) · `underline` (Ctrl+U) · `strike` (Ctrl+Mayús+X) ·
   ColorSwatch texto (`exec('color', c)`) · ColorSwatch resaltado
   (HIGHLIGHT_PALETTE, `exec('highlight', c)`) · `clearFormat`.
3. Grupo "Párrafo": `alignLeft/Center/Right/Justify` (active según `fmt.align`)
   · `listBullet`/`listNumber` (active) · `outdent`/`indent` · RibbonSelect
   interlineado (1 / 1,15 / 1,5 / 2 → `exec('lineHeight', v)`, width 64).
4. Grupo "Estilos" (oculto si `kind==='spreadsheet'`): RibbonSelect (Texto
   normal / Título 1 / Título 2 / Título 3 → `exec('heading', v)`, value
   `fmt.heading`, width 132).

**Insertar (B9):**

1. "Tabla": botón `table` que abre popover propio con rejilla 6×6 de celdas
   16×16 (hover marca F×C y rótulo "3 × 4"; clic → `exec('insertTable','3x4')`;
   mousedown preventDefault en todo el popover; Escape cierra).
2. "Imagen": botón `image` + `<input type="file" accept="image/*" hidden>` →
   FileReader → dataURL; si `dataUrl.length > 1_500_000` → toast "Imagen
   demasiado grande (máx. ~1 MB)" y no insertar (el documento entero viaja en
   cada autosave); si ok → `exec('insertImage', dataUrl)`.
3. "Enlace": botón `link` → `window.prompt('URL del enlace:')` → si no vacío,
   `exec('insertLink', url)`.
4. "Separador": botón `hr` → `exec('insertHr')`.
5. "Comentario": botón `comment` → `ctx.setUi({ panel: 'comentarios' })`.

**Disposición (B10):** "Márgenes" RibbonSelect (Estrecho 48 / Normal 96 /
Ancho 128 → `setPageSetup({margin})`) · "Orientación" botones `pagePortrait`/
`pageLandscape` (active según `pageSetup.landscape`) · "Sangría"
`outdent`/`indent` · "Interlineado" select como Inicio.

**Referencias (B9):** "Tabla de contenidos" botón `toc` → `exec('insertToc')`
(re-ejecutar la regenera) · "Vínculos" botón `link` (como Insertar) ·
"Comentario" botón `comment`.

**Colaboración (B10):**

1. "Comentarios": `comment` "Añadir comentario" (abre panel) · toggle
   "Mostrar comentarios" (active `ui.panel==='comentarios'`).
2. "Versiones": botón `history` "Guardar versión" →
   `window.prompt('Etiqueta de la versión (opcional):')`; null → cancelar; →
   `ctx.createVersion(label || undefined)` + toast "Versión guardada" ·
   `Dropdown` "Historial" que al abrir llama `ctx.loadVersions()` y lista
   MenuItems "`label ?? 'Versión'` — fecha local" → `confirm('¿Restaurar esta
versión? El estado actual se conserva en el historial.')` →
   `ctx.restoreVersion(id)` + toast.
3. "Compartir": botón `share` → copia `location.href` + toast (como cabecera).

**Vista (B10):** "Zoom": `minus` (−10) · RibbonSelect con presets 50 / 75 /
100 / 125 / 150 / 200 % (si el zoom actual no es un preset, muestra el más
cercano) · `plus` (+10), clamp 50..200 → `setUi({zoom})` · "Mostrar": toggle `ruler`
"Regla" (solo kind document; active `ui.ruler`) · toggle `panelRight` "Panel de
ajustes" (active `ui.settingsOpen`).

### 6.4 LeftRail (B3) — franja 40px + panel 288px

Franja: `w-[var(--editor-rail-w)] bg-surface border-r border-border flex
flex-col items-center gap-1 py-2`. Tres botones 32×32 (`search` "Buscar",
`outline` "Esquema del documento", `comment` "Comentarios"; active
`bg-selected` + `aria-pressed`): clic alterna `setUi({ panel: x | null })`.

Panel (si `ui.panel`): `w-[var(--editor-panel-w)] bg-surface border-r
border-border flex flex-col min-h-0`, `role="complementary"` con aria-label;
cabecera h-10 (`text-sm font-semibold px-3` + botón `close`); cuerpo
`overflow-y-auto`:

- **Buscar:** input (`Input` de `@sytue/ui`, aria-label "Buscar en el
  documento", debounce 300 ms) + contador "3 de 7" + botones anterior/
  siguiente (Enter = siguiente). Implementación: TreeWalker de nodos de texto
  del root (`getEditorRoot()`), `indexOf` case-insensitive → lista de Ranges;
  seleccionar el activo (`selection.addRange`) + `scrollIntoView({block:'center'})`.
  Sin resultados → "Sin coincidencias".
- **Esquema:** lista de `h1,h2,h3` del root (indentación por nivel:
  `pl-2/4/6`; `text-sm`, hover `bg-bg-subtle`); clic → `scrollIntoView`.
  Se refresca al abrir y en cada transición de `ctx.save` a `'saved'`. Vacío →
  "Añade títulos para ver el esquema".
- **Comentarios:** tarjetas (`rounded-md border border-border p-2`): autor
  (`author_sub.slice(0,8)`), fecha local, cuerpo, botón "Resolver"/"Reabrir"
  (`check`) y `trash` "Eliminar" (403 → toast). Resueltos con `opacity-60`.
  Abajo textarea + `Button size="sm"` "Comentar" (disabled vacío) →
  `ctx.addComment`.

### 6.5 Lienzo + regla (B6) — kind `document`

Zona central: `flex-1 min-w-0 overflow-auto` con
`background: var(--editor-canvas-bg)`. Dentro, un wrapper centrado con
`style="zoom: ctx.ui.zoom / 100"` que contiene regla y página (así escalan
juntas).

- **Página**: `class="sytue-page"` (estilos en §8), `contentEditable`,
  `role="textbox" aria-multiline="true"` `aria-label={ctx.title}` `lang="es"`,
  `spellcheck`. Dimensiones vía style: ancho `794px` (apaisado `1123px`),
  `min-height 1123px` (apaisado `794px`), `padding: pageSetup.margin px`,
  `margin: 32px auto`. Fondo SIEMPRE blanco (papel), sombra
  `var(--editor-page-shadow)`, sin borde.
- **Ciclo de datos**: al montar → `setEditorRoot(el)`;
  `useEffect([ctx.contentEpoch])` → `el.innerHTML = ctx.content ||
'<p><br></p>'`; `onInput` → `ctx.onContentInput(el.innerHTML)`. Desmontar →
  `setEditorRoot(null)`. El contenido NO pasa por estado Preact por tecla
  (el caret sobrevive).
- **Atajos** (`onKeyDown`, `metaKey||ctrlKey`): B/I/U → `exec('bold'|…)` +
  preventDefault; Mayús+X → strike. Z/Y se dejan al historial nativo.
- **Estadísticas**: en input (throttle 500 ms) y scroll:
  `pages = max(1, ceil(el.offsetHeight / altoPágina))`,
  `page = clamp(floor(scrollTop / (altoPágina · zoom/100)) + 1, 1, pages)` →
  `ctx.reportStats({pages, page})`.
  `// ponytail: folio continuo que crece; paginación visual real (cortes) en fase 2.`
- **Ruler (B6, mismo dueño)**: visible si `ui.ruler && kind==='document'`;
  `height: var(--editor-ruler-h)`, mismo ancho que la página, `sticky top-0`
  dentro del wrapper. Un `<svg>` con ticks: mayor cada cm (número `8px
text-text-subtle`), medio cada 0,5 cm (px/cm = anchoPágina/21 — apaisado /29,7).
  Zonas de margen sombreadas (`--editor-ruler-margin`), dos triangulitos
  estáticos marcando la sangría en los márgenes.
  `// ponytail: marcadores no arrastrables; drag de sangría en fase 2.`

### 6.6 SettingsPanel (B4) — 288px derecha

Visible si `ui.settingsOpen`. `w-[var(--editor-panel-w)] bg-surface border-l
border-border overflow-y-auto`, `role="complementary"`
aria-label "Ajustes". Cabecera h-10: "Ajustes de tabla" si `fmt.inTable`,
si no "Ajustes de párrafo"; botón `close` → `setUi({settingsOpen:false})`.

- **Sección Párrafo** (siempre): "Interlineado" (`Select` de `@sytue/ui` →
  `exec('lineHeight', v)`) · "Alineación" (4 RibbonButtons, active por
  `fmt.align`) · "Sangría" (outdent/indent).
- **Sección Tabla** (solo `fmt.inTable`, encima de Párrafo): filas de botón
  ancho completo (h-8, icono + texto, `hover:bg-bg-subtle rounded-sm`):
  "Insertar fila encima/debajo" (`rowAbove/rowBelow`), "Insertar columna a la
  izquierda/derecha" (`colLeft/colRight`), "Eliminar fila" / "Eliminar
  columna" / "Eliminar tabla" (`trash`, esta última `text-danger`) → comandos
  `table*` de §3.
  `// ponytail: sin espaciado antes/después ni anchos de columna en v1.`

### 6.7 StatusBar (B1) — 28px

`h-[var(--editor-status-h)] bg-surface border-t border-border px-3 flex
items-center gap-4 text-xs text-text-muted`. Izquierda según kind:
document → "Página {page} de {pages}" · "Palabras: {words}" · "Español
(España)"; spreadsheet → "Hoja {page} de {pages}"; presentation →
"Diapositiva {page} de {pages}". Derecha (`ml-auto flex items-center gap-1`):
botón `minus` "Reducir" (−10) · `<input type="range" min="50" max="200"
step="10" aria-label="Zoom">` (w-24, `accent-color: var(--sytue-accent)`) ·
texto "{zoom} %" (w-10, `tabular-nums`) · botón `plus` "Ampliar" (+10).

### 6.8 Atajos globales

| Atajo              | Acción                        | Dónde                                |
| ------------------ | ----------------------------- | ------------------------------------ |
| Ctrl/Cmd+B · I · U | negrita · cursiva · subrayado | onKeyDown del lienzo (§6.5)          |
| Ctrl/Cmd+Mayús+X   | tachado                       | ídem                                 |
| Ctrl/Cmd+Z · Y     | deshacer · rehacer            | historial nativo del contenteditable |

Los `shortcut` de RibbonButton solo documentan (title); no registran listeners.

---

## 7. Variantes sheets y slides (B11) — MISMO chrome, mismo guardado

Ambas usan el mismo header/ribbon/rail/status; solo cambia el lienzo que
`EditorApp` monta según `ctx.kind`. Persisten por el MISMO camino: serializan su
modelo con `JSON.stringify` y llaman `ctx.onContentInput(json)`; cargan con
`JSON.parse(ctx.content || 'null') ?? modeloVacío` en cada `contentEpoch`.
Parse inválido → modelo vacío + toast. Registran la celda/caja enfocada con
`setEditorRoot` en `focusin` (así Inicio/exec formatean texto dentro de la
celda o caja) y `setEditorRoot(null)` al desmontar.

### 7.1 SheetCanvas — `kind==='spreadsheet'` (clave `doc:sheet`)

Modelo vacío: `{ active: 0, sheets: [{ name: 'Hoja 1', cells: {} }] }`.

- **Barra de fórmulas** (h-7, bajo el ribbon, `bg-surface border-b
border-border px-2 flex items-center gap-2 text-sm`): caja de referencia
  (w-16, `text-text-muted`, p. ej. "B4") + `<input aria-label="Barra de
fórmulas">` flex-1 con el TEXTO de la celda seleccionada; Enter escribe el
  texto (escapando HTML) en la celda y baja una fila.
  `// ponytail: sin fórmulas (=SUMA…) en v1; solo valores.`
- **Rejilla A..Z × 1..100** en el área con scroll: esquina + cabeceras de
  columna (sticky top) y de fila (sticky left), `bg-bg-subtle text-xs
text-text-muted border-border`, cabecera de la celda seleccionada resaltada.
  Celdas: div `contenteditable` 96×24, `border-r border-b border-border
bg-white` (blanco SIEMPRE: papel), `text-sm px-1`, contenido =
  `cells[ref] ?? ''` (innerHTML).
- **Selección/edición**: estados `selected {c,r}` y `editing: boolean`. Clic =
  seleccionar (anillo `2px var(--sytue-accent)` inset); doble clic, Enter o
  teclear = editar (foco + caret); Escape = volver a selección; flechas (no
  editando) mueven; Tab/Enter confirman y avanzan (dcha/abajo). Commit (blur o
  confirmación): si `innerHTML` cambió → actualizar `cells[ref]` (vacío →
  `delete`) → `onContentInput(JSON.stringify(model))`.
- **Pestañas de hoja** (strip inferior h-7, `bg-surface border-t
border-border px-2 flex gap-1 text-xs`): una pestaña por hoja (activa
  `font-medium text-text` + subrayado `bg-accent`; inactivas
  `text-text-muted`), doble clic renombra (prompt), botón `plus` "Añadir
  hoja" (nombre "Hoja N"). Cambio de hoja → `model.active` + commit.
- `reportStats({ pages: sheets.length, page: active + 1 })`.
- La rejilla es `role="grid"` con `aria-rowindex/aria-colindex` en celdas;
  cabeceras `role="columnheader"/"rowheader"`.
  `// ponytail: 26×100 celdas fijas renderizadas de golpe (~2600 divs, va sobrado); virtualizar solo si se amplía la rejilla.`

### 7.2 SlidesCanvas — `kind==='presentation'` (clave `doc:slides`)

Modelo vacío: una diapositiva con caja título
`{ id, boxes: [{ id, x: 10, y: 35, w: 80, h: 20, html: '<h1>Haz clic para añadir título</h1>' }] }`
(ids con `crypto.randomUUID()`).

- **Carril de miniaturas** (izquierda, w-40 = 160px, `bg-surface border-r
border-border overflow-y-auto p-2 flex flex-col gap-2`): miniatura 16:9
  (caja 128×72: interior 960×540 con `transform: scale(128/960)` origin
  top-left, `pointer-events-none`, cajas pintadas readonly). Activa: anillo
  `2px var(--sytue-accent)`; número de orden en la esquina. Botones: `plus`
  "Nueva diapositiva" y `trash` "Eliminar diapositiva" (mín. 1, `confirm`).
- **Lienzo**: centrado sobre `var(--editor-canvas-bg)`; escenario 16:9 base
  960×540 escalado por `min(anchoDisponible/960, altoDisponible/540) ·
zoom/100`, fondo blanco, sombra `var(--editor-page-shadow)`.
- **Cajas de texto**: div absoluto (x/y/w/h en %), interior `contenteditable`.
  Clic = seleccionar (borde `1px dashed var(--sytue-accent)` + asa de mover
  superior + asa de redimensionar SE); doble clic = editar texto; arrastrar
  asas con Pointer Events (`setPointerCapture`) actualiza x/y o w/h (clamp
  0..100); Supr con caja seleccionada (no editando) la elimina. Botón "Cuadro
  de texto" (`textBox`) en una mini-barra sobre el escenario añade una caja
  centrada. Todo cambio → commit del modelo → `onContentInput`.
- `reportStats({ pages: slides.length, page: activa + 1 })`.
  `// ponytail: sin imágenes ni formas en slides v1; solo cajas de texto.`

---

## 8. `src/editor/editor.css` (A1 lo pega tal cual; ÚNICO fichero con hex de UI)

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

`EditorApp.tsx` hace `import './editor.css';`. Tailwind detecta las clases de
`src/editor/*.tsx` automáticamente (detección de fuentes del plugin Vite).

---

## 9. Iconos — `src/editor/icons.tsx` (A3)

Mismo patrón que `Icon` de `@sytue/ui` (NO tocar packages/ui; trazados PROPIOS,
dibujados de cero): `viewBox="0 0 24 24"`, `fill="none"`,
`stroke="currentColor"`, `stroke-width={1.75}`, `stroke-linecap/linejoin="round"`,
decorativo salvo `label`.

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

Multi-path permitido (array de `d` por nombre). Estética: geometría simple de
línea, esquinas redondeadas, coherente con el design system.

---

## 10. Reparto por constructor (ficheros DISJUNTOS)

| Quién   | Ficheros (dueño único)                                                                                         | Depende de (contratos)                                          | Notas                                                                                                                                                                             |
| ------- | -------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A1**  | `types.ts`, `EditorApp.tsx`, `editor.css`, `src/pages/edit.astro`, `test/e2e/tests/docs-editor.spec.ts`        | §2 (lo publica), A2 (hook), B5 (getFormatState)                 | Sube `types.ts`+`editor.css` PRIMERO (los pega de §2/§8); integra `EditorApp` y el swap de `edit.astro` al final; escribe el humo e2e (§11) siguiendo `test/e2e/tests/helpers.ts` |
| **A2**  | `state/useDocument.ts`, `src/lib/docsProxy.ts`, `src/pages/api/{state,ops,rename,comments,versions,export}.ts` | §2, §4                                                          | La capa de datos completa; verificable con curl (contrato §5)                                                                                                                     |
| **A3**  | `icons.tsx`, `ribbon/primitives.tsx`                                                                           | §2, §5, §9                                                      | Bloquea a B4 y B7–B10: aterrizar pronto                                                                                                                                           |
| **B1**  | `layout/EditorLayout.tsx`, `layout/StatusBar.tsx`                                                              | §2, §6.7, §8                                                    | Layout puro + barra de estado                                                                                                                                                     |
| **B2**  | `layout/EditorHeader.tsx`                                                                                      | §2, §6.1, A3 (iconos), `@sytue/ui` (Button, Dropdown, useToast) | Título, chip, peers, Compartir, Descargar                                                                                                                                         |
| **B3**  | `layout/LeftRail.tsx`                                                                                          | §2, §6.4, A3, B5 (`getEditorRoot`)                              | Franja + buscar/esquema/comentarios                                                                                                                                               |
| **B4**  | `layout/SettingsPanel.tsx`                                                                                     | §2, §6.6, A3                                                    | Párrafo + Tabla contextual                                                                                                                                                        |
| **B5**  | `canvas/exec.ts`                                                                                               | §2, §3                                                          | El cerebro de edición; sin componentes                                                                                                                                            |
| **B6**  | `canvas/PageCanvas.tsx`, `canvas/Ruler.tsx`                                                                    | §2, §6.5, §8, B5                                                | Página A4 + regla                                                                                                                                                                 |
| **B7**  | `ribbon/Ribbon.tsx`                                                                                            | §2, §6.2, A3, tabs (import)                                     | Tira de pestañas + menú Archivo                                                                                                                                                   |
| **B8**  | `ribbon/tabs/Inicio.tsx`                                                                                       | §2, §5, §6.3                                                    | La pestaña más densa                                                                                                                                                              |
| **B9**  | `ribbon/tabs/Insertar.tsx`, `ribbon/tabs/Referencias.tsx`                                                      | §2, §5, §6.3                                                    | Incluye el popover 6×6 de tabla                                                                                                                                                   |
| **B10** | `ribbon/tabs/Disposicion.tsx`, `ribbon/tabs/Vista.tsx`, `ribbon/tabs/Colaboracion.tsx`                         | §2, §5, §6.3, `@sytue/ui` (Dropdown)                            | Tres pestañas ligeras                                                                                                                                                             |
| **B11** | `sheets/SheetCanvas.tsx`, `slides/SlidesCanvas.tsx`                                                            | §2, §7, B5 (`setEditorRoot`)                                    | Las dos variantes; el lote más pesado                                                                                                                                             |

Reglas de integración: los imports cruzados van SOLO hacia `types.ts`,
`icons.tsx`, `primitives.tsx` y `canvas/exec.ts` (más los imports de
composición de `EditorApp`/`Ribbon`/`EditorLayout`, que hacen sus dueños).
Nadie importa ficheros de otro constructor fuera de esa lista.

---

## 11. Definición de hecho

1. `(cd apps/frontend && bunx astro check --root packages/office)` en verde tras la
   integración de A1 (cada constructor: cero errores propios).
2. Humo Playwright (A1, `test/e2e/tests/docs-editor.spec.ts`, patrón de
   `docs.spec.ts`/`helpers.ts`, `ignoreHTTPSErrors: true`, login
   `admin@sytue.localhost` / `sytue-dev-admin`): crear documento (POST
   `/api/create` o UI) → abrir `/edit?id=` → escribir "Hola mundo" en la página
   → pulsar el botón Negrita y escribir → esperar chip "Guardado" → `reload()`
   → el texto persiste (y el fragmento en negrita sigue en `<b>`/`<span>` con
   `font-weight`). Es LA verificación del contrato de persistencia.
3. AA: navegable con teclado (pestañas del ribbon con flechas, todos los
   controles enfocables con foco visible), `aria-label` en cada icon-button,
   `aria-pressed` en toggles, `aria-live` en el chip de guardado.
4. Sin errores de consola en carga y edición normal; sin peticiones del
   navegador a `docs-api.sytue.localhost` (todo same-origin).
5. Sin dependencias nuevas en `package.json`; sin ficheros fuera del reparto.

Fuera de alcance v1 (explícito, no lo intentes): WebSocket/cursores en vivo,
paginación visual real, fórmulas de hoja de cálculo, permisos/uso compartido
fino, anclas de comentario por carácter, arrastre de sangrías en la regla.
