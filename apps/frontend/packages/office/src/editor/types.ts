/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Contratos del editor Sytue Docs. Fuente única de tipos compartidos entre
 * zonas (BLUEPRINT.md §2). No añadir lógica: solo tipos y constantes.
 */
import type { SheetPivot } from './sheets/pivot';
import { OFFICE_EXPORT_FORMATS } from '@sytue/config';
import type { OfficeExportFormat } from '@sytue/config';
import type { DocKind, DocumentMeta } from '@sytue/api-clients';
import type { Locale } from '@sytue/i18n';
import type { ConnectorAnchor, ConnectorKind } from '@sytue/escena';
import type { DocsT } from '../i18n';

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
  /** Nombre del autor en el directorio; lo resuelve el servicio. */
  author_name?: string;
  anchor: unknown;
  body: string;
  resolved: boolean;
  resolved_by?: string | null;
  created_at: string;
  updated_at: string;
}

/** Una entrada de la auditoría (domain.EntradaAuditoria): un grupo de operaciones de una
 *  persona, del mismo tipo y clave, en el mismo minuto. Sin el contenido de lo escrito. */
export interface AuditEntry {
  seq_desde: number;
  seq_hasta: number;
  author_sub: string;
  kind: string;
  key?: string;
  count: number;
  at: string;
  /** Dónde tocaron los parches del grupo (hoja/celda, diapositiva/caja), hasta cinco. */
  paths?: string[];
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

export type PanelId = 'buscar' | 'esquema' | 'comentarios' | 'revision' | 'actividad' | 'ia';

/**
 * Formatos de descarga, en el orden del menú. PDF primero: es lo que más se busca.
 *
 * Es la ÚNICA lista: el menú del editor la recorre y el BFF (`pages/api/export.ts`)
 * construye su verja con ella. Cuando eran dos, el menú ofrecía PDF y el BFF lo
 * rechazaba con `bad_format`, así que la entrada más usada devolvía un error.
 */
export const EXPORT_FORMATS = OFFICE_EXPORT_FORMATS;

export type ExportFormat = OfficeExportFormat;

export interface EditorUi {
  /** Panel izquierdo abierto (null = solo la franja de iconos). */
  panel: PanelId | null;
  /** Panel derecho de ajustes visible. */
  settingsOpen: boolean;
  /** Regla horizontal visible (solo kind document). */
  ruler: boolean;
  /** Zoom en %: 50..200, paso 10. */
  zoom: number;
  /** Modo «Rellenar formulario» (solo document): cuerpo en solo lectura, campos interactivos. */
  formFill: boolean;
  /** Modo «Control de cambios» (solo document): la edición se MARCA con autoría
   *  (<ins>/<del> data-track) en vez de aplicarse directa. Tanto las marcas como el
   *  modo persisten en el HTML del folio (el modo, como sentinela al frente del
   *  contenido); reabrir el doc restaura el estado (persistencia por-documento). */
  track: boolean;
}

/** Configuración de página (solo kind document). Se guarda en el documento como la capa
 *  `data-sytue-page` (pagedecor.ts: paginaHtml/leerPagina), igual que en rico. */
export interface PageSetup {
  /** Margen interior de la página en px @100%: 48 | 96 | 128. */
  margin: number;
  /** true = apaisado (intercambia ancho/alto). */
  landscape: boolean;
  /** Tamaño del papel; sin él, A4. */
  size?: 'a4' | 'carta' | 'legal' | undefined;
  /** Color del papel, `#RRGGBB`; sin él, blanco. */
  color?: string | undefined;
  /** Los de un documento con su propia página (un .docx importado): mandan sobre size,
   *  landscape y margin hasta que se elija otro en la cinta. width/height ya orientados;
   *  margins: arriba, derecha, abajo, izquierda. */
  width?: number | undefined;
  height?: number | undefined;
  margins?: [number, number, number, number] | undefined;
}

export interface EditorStats {
  /** Palabras (solo document; 0 en el resto). Recalculado en cada autosave. */
  words: number;
  /** document: páginas · spreadsheet: hojas · presentation: diapositivas. */
  pages: number;
  /** Unidad actual (página/hoja/diapositiva), 1-indexada. */
  page: number;
}

/** Colaborador presente, del heartbeat de presencia (PUT /api/presence, §4.4). */
/** Punto de la secuencia RGA (state/rga.ts) tal como viaja en un cursor. */
export interface CursorDot {
  a: string;
  c: number;
}

export interface CursorDots {
  aPuntos: (
    longitudFolio: number,
    off: { anchor: number; focus: number },
  ) => { anchorDot: CursorDot; focusDot: CursorDot } | null;
  aOffsets: (
    longitudFolio: number,
    dots: { anchorDot: CursorDot; focusDot: CursorDot },
  ) => { anchor: number; focus: number } | null;
}

export interface Peer {
  sub: string;
  /** Date.now() del último heartbeat en que se vio a este par. */
  lastSeen: number;
}

/* ---------- Cursores remotos en vivo (efímeros; §4.4) ----------
 * Estado que vive SOLO en la conexión/UI: no entra en el modelo LWW del
 * documento (nunca por onContentInput). Viaja por el WS de colaboración como
 * carga opaca del mensaje "cursor" (protocolo en
 * services/docs/internal/collab/protocol.go) y se difunde a los peers. */

/**
 * Posición efímera del cursor/selección de un participante, por tipo de
 * documento (unión discriminada por `kind`). Se serializa tal cual a JSON.
 *  - sheet: hoja activa + rango de celdas (ancla c1/r1 → extremo c2/r2, 0-based).
 *  - doc:   offsets de texto (ancla/foco) dentro del folio contenteditable.
 *  - slide: diapositiva activa (0-based) + ids de las cajas seleccionadas.
 */
export type CursorPos =
  | {
      kind: 'sheet';
      sheet: number;
      c1: number;
      r1: number;
      c2: number;
      r2: number;
      /** Identidad de la hoja y de las filas y columnas de los extremos (sheets/anclas.ts):
       *  sobreviven a que otra persona inserte, borre o reordene. Ausentes en clientes
       *  anteriores: entonces mandan los índices. */
      sheetId?: string;
      rows?: [string, string];
      cols?: [string, string];
    }
  | {
      kind: 'doc';
      anchor: number;
      focus: number;
      /** Los mismos extremos anclados a la identidad (dot RGA) del carácter que tienen detrás:
       *  sobreviven a las ediciones ajenas. Ausentes si quien emite no los pudo calcular. */
      anchorDot?: CursorDot;
      focusDot?: CursorDot;
    }
  | { kind: 'slide'; slide: number; boxes: string[] };

/** Cursor remoto de un peer, ya resuelto para el render. */
export interface PeerCursor {
  /** sub del participante (clave estable; puede repetirse entre pestañas del mismo usuario). */
  id: string;
  /** Nombre visible (se escapa como texto al pintar); puede venir vacío. */
  name: string;
  /** Color de marca estable por peer (var CSS --sytue-avatar-*). */
  color: string;
  pos: CursorPos;
}

/* ---------- Comandos de edición (canvas/exec.ts, §3) ---------- */

export type EditorCommand =
  | 'bold'
  | 'italic'
  | 'underline'
  | 'strike'
  | 'superscript'
  | 'subscript'
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
  | 'heading' // value: 'p' | 'h1' | 'h2' | 'h3' | 'blockquote'
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
  | 'tableMergeCells'
  | 'tableSplitCell'
  | 'tableCellBg' // value: hex de HIGHLIGHT_PALETTE o 'transparent'
  | 'tableCellValign' // value: 'top' | 'middle' | 'bottom'
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
  heading: 'p' | 'h1' | 'h2' | 'h3' | 'blockquote';
  /** La selección está dentro de una tabla (activa el panel Tabla). */
  inTable: boolean;
  /** Hay una imagen seleccionada (activa el grupo Imagen del ribbon). */
  imageSelected: boolean;
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
  imageSelected: false,
  canUndo: false,
  canRedo: false,
};

/* ---------- Modelos de sheets/slides (§7) ---------- */

/**
 * Protección a nivel LIBRO (paridad Excel «Proteger libro → Estructura»). Con el
 * libro protegido, la ESTRUCTURA queda bloqueada: no se puede añadir, borrar,
 * renombrar, duplicar ni reordenar hojas (cambiar de hoja o editar celdas SÍ se
 * permite). La PRESENCIA del objeto en `SheetModel.protect` = libro protegido
 * (igual criterio que `SheetData.protect` en la hoja). Campo nuevo OPCIONAL y
 * retrocompatible: un doc viejo sin `protect` conserva el comportamiento actual.
 */
export interface WorkbookProtection {
  /**
   * El servidor tiene una contraseña (solo su hash) para desproteger. Lo FIJA el
   * servidor: el modelo nunca lleva la contraseña, ni en claro ni en hash. Se
   * teclea al proteger y viaja aparte (state/proteccion.ts); la verifica el servicio
   * Go, que también rechaza los cambios de estructura de quien no la tiene.
   */
  hasPassword?: boolean;
}

export interface SheetModel {
  active: number;
  sheets: SheetData[];
  /** Protección de estructura del libro (añadir/borrar/renombrar/reordenar hojas); ausente = sin proteger. */
  protect?: WorkbookProtection;
  /** Nombres definidos del libro (Administrador de nombres); ausente = ninguno. */
  names?: DefinedName[];
}

/**
 * Nombre definido (rango con nombre): «Ventas» en vez de «Hoja1!$B$2:$B$40». Lo
 * gestiona sheets/nombres.ts; viaja a XLSX como `definedName` y a ODS como
 * `table:named-range`/`table:named-expression`.
 */
export interface DefinedName {
  /** Identificador: letra o «_» y luego letras, dígitos, «_» o «.»; sin forma de referencia. */
  name: string;
  /** Definición sin «=»: un rango («Hoja1!$A$1:$B$5»), un valor («0.21») o una fórmula. */
  ref: string;
  /** Ámbito: el nombre de la hoja a la que se limita; ausente = todo el libro. */
  sheet?: string;
  /** Comentario libre del Administrador de nombres. */
  comment?: string;
}

/* Campos opcionales de SheetData (fase Núcleo de sheets): TODOS retrocompatibles
 * — un doc viejo {name, cells} carga igual, y Go trata doc:sheet como blob opaco. */

/** Formato de número de una celda. Solo afecta al RENDER: el texto crudo manda.
 *  `custom` usa una máscara estilo Excel (campo `mask`); el resto son presets. */
export type NumFmtId =
  'general' | 'number' | 'thousands' | 'currency' | 'percent' | 'date' | 'custom';
export interface NumFmt {
  id: NumFmtId;
  /** Decimales explícitos (aumentar/disminuir); ausente = los del preset. */
  decimals?: number;
  /** Solo `id: 'custom'`: máscara estilo Excel (secciones «pos;neg;cero», tokens
   *  0 # ? , . %, [Color], comillas y fecha/hora). La aplica sheets/numfmt-mask.ts. */
  mask?: string;
}

/** Operador de una regla de formato condicional de tipo «celda». */
export type CondOp = 'gt' | 'ge' | 'lt' | 'le' | 'eq' | 'ne' | 'contains';

/**
 * Tipo de regla de formato condicional (paridad OnlyOffice). El campo `kind`
 * es OPCIONAL: una regla sin `kind` es de tipo «cell» (retrocompatible con los
 * docs viejos {range, op, value, fill}). El orden del array = PRIORIDAD (la
 * primera que produce cada canal —relleno, barra, icono— gana).
 */
export type CondKind =
  | 'cell' // op + value + fill (comparación de valor)
  | 'formula' // formula + fill (=expresión evaluada por celda, refs relativas)
  | 'top' // rank (+percent) + fill (N superiores)
  | 'bottom' // rank (+percent) + fill (N inferiores)
  | 'dup' // fill (valores repetidos)
  | 'unique' // fill (valores únicos)
  | 'dataBar' // color (barra de datos proporcional en la celda)
  | 'colorScale' // scale 2/3 colores (relleno interpolado por valor)
  | 'iconSet'; // icons (juego de iconos por tercios)

/** Juego de iconos de una regla `iconSet` (3 niveles: bajo/medio/alto). */
export type CondIconSet = 'arrows' | 'traffic' | 'symbols';

/**
 * Regla de formato condicional. Los campos son un UNION plano por `kind`
 * (todos opcionales para retrocompatibilidad y saneado tolerante); cada tipo
 * usa los suyos. `sanitizeCondRule` (sheets/format.ts) valida por `kind`.
 */
export interface CondRule {
  /** Rango A1 al que aplica: «B2» o «A1:C10». */
  range: string;
  /** Tipo; ausente = 'cell'. */
  kind?: CondKind;
  /** cell: operador. */
  op?: CondOp;
  /** cell: umbral/texto a comparar, tal cual lo tecleó el usuario (es-ES). */
  value?: string;
  /** cell/formula/top/bottom/dup/unique: fondo #RRGGBB (de HIGHLIGHT_PALETTE). */
  fill?: string;
  /** formula: «=expresión» booleana; refs relativas se desplazan por celda. */
  formula?: string;
  /** top/bottom: N (o porcentaje si `percent`). */
  rank?: number;
  /** top/bottom: `rank` es un porcentaje 0..100 en vez de un recuento. */
  percent?: boolean;
  /** dataBar: color #RRGGBB de la barra. */
  color?: string;
  /**
   * cell/formula/top/bottom/dup/unique: color de letra #RRGGBB, con el relleno o sin él (una
   * regla puede pintar solo la letra: «en rojo si es negativo»). Llega de Excel y de Calc.
   */
  textColor?: string;
  /** colorScale: 2 o 3 colores #RRGGBB [mín,(medio),máx]. */
  scale?: string[];
  /** iconSet: juego de iconos. */
  icons?: CondIconSet;
}

/**
 * Mini-gráfico en una celda (paridad OnlyOffice/Excel). Se dibuja sobre el
 * rango de datos, se persiste en el modelo y se redibuja en vivo al cambiar los
 * valores de origen (usa engine.getValue como el resto del render).
 */
export interface Sparkline {
  /** Celda A1 anfitriona donde se pinta el mini-gráfico. */
  cell: string;
  /** Rango A1 de los datos (fila o columna). */
  range: string;
  type: 'line' | 'column' | 'winloss';
  /** Color principal #RRGGBB (de CONTENT_PALETTE); ausente = azul de serie. */
  color?: string;
}

/** Dónde va la leyenda de un gráfico; ausente = debajo. */
export type ChartLegend = 'bottom' | 'top' | 'right' | 'none';

/** Gráfico incrustado sobre la rejilla (render en sheets/Chart.tsx). Todos los campos
 *  nuevos son OPCIONALES y retrocompatibles: un gráfico viejo se pinta igual. */
export interface ChartSpec {
  id: string;
  /** `bar` son COLUMNAS (verticales) y `hbar` barras horizontales; `scatter`
   *  interpreta la 1.ª columna como X numérica (pares XY); `combo` mezcla columnas +
   *  línea; `area`/`donut`/`radar` son variantes visuales. */
  type: 'bar' | 'hbar' | 'line' | 'pie' | 'scatter' | 'area' | 'donut' | 'radar' | 'combo';
  /** Rango A1 de los datos. */
  range: string;
  title?: string;
  /** Posición de la tarjeta sobre la rejilla en px de maqueta (zoom 100%). */
  x?: number;
  y?: number;
  /** Combo: índices 0-based de serie (sobre las columnas de datos) que se
   *  dibujan como LÍNEA; el resto van como columnas. Ausente = la última serie. */
  lineSeries?: number[];
  /** Índices 0-based de serie mapeados al EJE SECUNDARIO (derecha). Solo
   *  cartesianos (bar/line/area/combo). Ausente/vacío = todo en el eje principal. */
  secondaryAxis?: number[];
  /** Tamaño del ÁREA del gráfico en px de maqueta; ausente = 278 × 156,375. */
  w?: number;
  h?: number;
  /** Las series van por FILAS (el «Cambiar filas/columnas» de Excel). */
  byRow?: boolean;
  /** Columnas, barras o áreas apiladas. */
  stacked?: boolean;
  /** Leyenda: posición; ausente = debajo (solo con ≥2 entradas). */
  legend?: ChartLegend;
  /** Etiquetas de datos (el valor en cada marca; el % en el circular). */
  labels?: boolean;
  /** `false` quita la retícula; ausente = con retícula. */
  grid?: boolean;
  /** Títulos de los ejes de categorías (X) y de valores (Y). */
  xTitle?: string;
  yTitle?: string;
  /** Color por serie (o porción) #RRGGBB de la paleta; '' = el de su puesto. */
  colors?: string[];
}

/** Criterio de una validación de datos (paridad Excel). Ausente = 'list'
 *  (retrocompatible con la validación por lista {range, values, strict}). */
export type DataValidationType = 'list' | 'number' | 'date' | 'textLength' | 'formula';

/** Operador de comparación de number/date/textLength. `between`/`notBetween`
 *  usan dos operandos (a, b); el resto solo `a`. */
export type DataValidationOp = 'between' | 'notBetween' | 'eq' | 'ne' | 'gt' | 'lt' | 'ge' | 'le';

/**
 * Validación de datos de un rango (pestaña Datos). Ausente `type` = 'list'
 * (desplegable de valores permitidos); RETROCOMPATIBLE con los docs viejos
 * {range, values, strict}. Los criterios number/date/textLength comparan con
 * `op` + operandos `a` (+ `b` si es entre/no-entre); `formula` exige que una
 * expresión evalúe a VERDADERO en la celda. Campos por criterio, todos
 * opcionales (saneado tolerante en sanitizeDataValidation).
 */
export interface DataValidation {
  /** Rango A1 al que aplica: «B2» o «A1:C10». */
  range: string;
  /** Criterio; ausente = 'list' (retrocompatible). */
  type?: DataValidationType;
  /** list: valores permitidos (desplegable), no vacía. */
  values?: string[];
  /** number/date/textLength: operador de comparación. */
  op?: DataValidationOp;
  /** number/date/textLength: 1.er operando (texto es-ES: número, fecha dd/mm/aaaa o longitud). */
  a?: string;
  /** number/date/textLength: 2.º operando (solo `between`/`notBetween`). */
  b?: string;
  /** number: exige entero (rechaza decimales). Ausente = admite decimales. */
  integer?: boolean;
  /** formula: «=expresión» booleana que debe cumplirse en la celda (refs relativas al inicio del rango). */
  formula?: string;
  /** Mensaje mostrado al rechazar un valor (ausente = mensaje genérico por criterio). */
  error?: string;
  /** true = rechaza lo que no cumple; ausente/false = solo lo marca (avisa). */
  strict?: boolean;
}

/** Trazo de un lado del borde además de la línea fina (de un .xlsx o un .ods importado). */
export type BorderStroke = 'medium' | 'thick' | 'dashed' | 'dotted' | 'double';

/**
 * Lados de la caja de una celda con borde manual dibujado (BORDES de InicioSheets). De cada
 * lado, su color (`tc`, `rc`, `bc`, `lc`; ausente = el de serie) y su trazo (`ts`…; ausente =
 * fino): los trae un libro importado, y uno de antes no los lleva.
 */
export interface CellBorder {
  t?: boolean;
  r?: boolean;
  b?: boolean;
  l?: boolean;
  tc?: string;
  rc?: string;
  bc?: string;
  lc?: string;
  ts?: BorderStroke;
  rs?: BorderStroke;
  bs?: BorderStroke;
  ls?: BorderStroke;
}

/**
 * Formato VISUAL de una celda (InicioSheets: fuente, tamaño, color, relleno,
 * bordes, subrayado/tachado, alineación vertical, ajuste de texto). Va en un
 * mapa aparte de `cells` — el texto crudo sigue siendo la fuente de verdad y el
 * estilo solo se aplica al pintar (como `formats`). Negrita/cursiva/alineación
 * horizontal viven en el HTML de la celda (envoltorio canónico), no aquí.
 */
export interface CellStyle {
  /** Familia tipográfica (una de las ofrecidas por el ribbon). */
  font?: string;
  /** Tamaño en pt. */
  size?: number;
  /** Color de texto #RRGGBB. */
  color?: string;
  /** Color de relleno manual #RRGGBB (el formato condicional, si cumple, gana). */
  fill?: string;
  /** Subrayado. */
  u?: boolean;
  /** Tachado. */
  s?: boolean;
  /** Alineación vertical (por defecto: la de la rejilla). */
  valign?: 'top' | 'middle' | 'bottom';
  /** Ajuste de texto (wrap). */
  wrap?: boolean;
  /** Bordes manuales por lado. */
  border?: CellBorder;
  /**
   * Protección: la celda está BLOQUEADA (solo relevante si la hoja está
   * protegida). Ausente = true (bloqueada, criterio Excel); `false` = editable
   * aunque la hoja esté protegida. Solo se persiste el `false` (JSON mínimo).
   */
  locked?: boolean;
  /**
   * Protección: OCULTAR la fórmula de la celda en la barra de fórmulas cuando la
   * hoja está protegida. Ausente/false = se muestra. Solo se persiste el `true`.
   */
  hidden?: boolean;
}

/**
 * Autofiltro PERSISTENTE de la hoja (pestaña Datos): fila de encabezados con
 * desplegables por columna que ocultan las filas cuyo valor está excluido.
 * Sustituye al filtro efímero anterior; se guarda en el documento.
 */
export interface SheetAutoFilter {
  /** Fila de los encabezados (0-based) donde aparecen los desplegables. */
  row: number;
  /** Columnas cubiertas [c1, c2] (0-based). */
  c1: number;
  c2: number;
  /** Valores EXCLUIDOS por columna: clave = índice de columna en texto. */
  excl?: Record<string, string[]>;
  /** Condición por columna (mayor que, contiene, N superiores, color…); mismas claves que `excl`. */
  cond?: Record<string, FilterCondition>;
}

/** Operadores del filtro por condición (sheets/filtro.ts). */
export type FilterOp =
  | 'gt'
  | 'ge'
  | 'lt'
  | 'le'
  | 'eq'
  | 'ne'
  | 'between'
  | 'notBetween'
  | 'contains'
  | 'notContains'
  | 'begins'
  | 'ends'
  | 'blank'
  | 'notBlank'
  | 'top'
  | 'bottom'
  | 'aboveAvg'
  | 'belowAvg'
  | 'fill'
  | 'fontColor';

/** Una condición de filtro. Los operandos `a`/`b` van como los tecleó la persona. */
export interface FilterCondition {
  op: FilterOp;
  a?: string;
  b?: string;
  /** N de «N superiores/inferiores» (o su porcentaje con `percent`). */
  n?: number;
  percent?: boolean;
  /** #rrggbb de `fill` (relleno manual) y `fontColor`. */
  color?: string;
}

/**
 * Segmentación de datos (slicer, paridad Excel): panel flotante que filtra una
 * COLUMNA del autofiltro de la hoja mostrando sus valores únicos como botones
 * (los seleccionados = visibles). No guarda su propia selección ni su rango: la
 * selección VIVE en `SheetAutoFilter.excl` (única fuente de verdad, sin desincronía
 * con el desplegable de filtro nativo) y el «rango/campo objetivo» es la columna
 * `col` del autofiltro de la hoja. Campos nuevos opcionales y retrocompatibles
 * (Go trata doc:sheet como blob opaco). Render en sheets/Slicer.tsx.
 */
export interface Slicer {
  id: string;
  /** Columna (0-based ABSOLUTA) que segmenta; cae dentro del autofiltro de la hoja.
   *  Con `pivot`, el campo (0-based) del ORIGEN de esa tabla dinámica. */
  col: number;
  /** Nombre de una tabla dinámica de la hoja: la segmentación filtra sus elementos (el
   *  `hidden` del campo en la tabla, que comparte con su lista de elementos) en vez del
   *  autofiltro. Ausente = segmenta el autofiltro. */
  pivot?: string;
  /** Posición de la tarjeta sobre la rejilla en px de maqueta (zoom 100%). */
  x?: number;
  y?: number;
}

/** Función de agregación de la fila de totales de una tabla (paridad Excel, por
 *  columna). `counta` = celdas no vacías; el resto solo cuentan números. */
export type TableTotalFn = 'none' | 'sum' | 'average' | 'count' | 'counta' | 'min' | 'max';

/**
 * Tabla con formato (ListObject de Excel): un rango rectangular con nombre único
 * en el libro, cabecera con nombres de columna, filas con bandas según un estilo
 * sobrio y fila de totales opcional. Los DATOS siguen viviendo en `cells` (el
 * texto crudo manda); la tabla solo aporta rango + formato + comportamiento
 * (auto-expandir al escribir al lado, referencias estructuradas Tabla[Columna]).
 * Campos nuevos TODOS opcionales y retrocompatibles (Go trata doc:sheet como blob).
 */
export interface SheetTable {
  /** Nombre único en el libro (referencias estructuradas: Tabla1[Columna]). */
  name: string;
  /** Rango A1 que cubre, incluidas la cabecera y la fila de totales si las hay. */
  range: string;
  /** Id de un estilo de TABLE_STYLES (sheets/table.ts); ausente = el primero. */
  style?: string;
  /** La 1.ª fila del rango son cabeceras con los nombres de columna. Ausente = true. */
  headers?: boolean;
  /** Muestra una fila de totales al final del rango. Ausente = false. */
  totalsRow?: boolean;
  /** Función de total por índice de columna 0-based (clave = índice en texto); ausente = 'none'. */
  totals?: Record<string, TableTotalFn>;
}

/**
 * Consulta a un origen externo, EXACTAMENTE en la forma (snake_case) que el
 * backend espera como campo `query` de `POST /api/v1/connectors/execute`: solo
 * se rellena `rest` XOR `postgres` según el tipo de la conexión. Se guarda en el
 * documento para poder REFRESCAR (se reenvía tal cual) y el backend la revalida y
 * sanea en cada ejecución (frontera de confianza real: guard SELECT único en
 * PostgreSQL, barrera SSRF en REST). NUNCA lleva credenciales.
 */
export interface ExtRestQuery {
  /** 'GET' (por defecto en el backend) | 'POST'. */
  method?: string;
  /** Ruta RELATIVA a la URL base de la conexión (nunca absoluta). */
  path?: string;
  /** Parámetros de query añadidos a la URL. */
  query?: Record<string, string>;
  /** Cabeceras NO secretas de esta consulta. */
  headers?: Record<string, string>;
  /** Cuerpo JSON para POST (opcional). */
  body?: unknown;
  /** json-path SIMPLE (dot / [idx]) al array dentro del JSON de respuesta. */
  root_path?: string;
}
export interface ExtPostgresQuery {
  /** Un ÚNICO SELECT (lo valida el backend con guardSelect). */
  sql: string;
  /** Parámetros posicionales para $1..$n (jamás se concatenan al SQL). */
  params?: unknown[];
}
export interface ExtGatewayQuery {
  /** Spec OPACA que el gateway reenvía al driver de la app conectada (colección,
   *  JQL, SOQL…). Su forma la define ese driver; docs solo la transporta. */
  spec?: unknown;
}
export interface ExtQuerySpec {
  rest?: ExtRestQuery;
  postgres?: ExtPostgresQuery;
  gateway?: ExtGatewayQuery;
  /** Origen Sytue Bases: sin consulta. El token de la conexión (cifrado en el backend) ya
   *  dice qué tabla o qué vista se lee; aquí solo se marca el tipo. */
  bases?: Record<string, never>;
}

/**
 * PROCEDENCIA de una tabla traída de Sytue Bases: de qué base, tabla y vista sale, quién
 * concedió la lectura, cuándo y con qué versión de la conexión. La declara Sytue Bases en
 * cada lectura; el documento la guarda para enseñarla junto a la tabla. Sin secretos.
 */
export interface ExtProvenance {
  source: 'sytue-bases';
  baseName: string;
  tableName: string;
  viewName?: string;
  grantedBy: string;
  /** ISO-8601 de la concesión. */
  grantedAt: string;
  version: number;
}

/* ---------- Transformaciones tabulares (Power Query / «pasos aplicados») ---------- */

/** Tipo lógico al que convertir una columna (paso changeType, con parseo). */
export type QueryColType = 'text' | 'number' | 'date' | 'boolean';

/** Operador de una condición de filtrado de filas. */
export type QueryFilterOp =
  | 'eq' // igual a
  | 'neq' // distinto de
  | 'contains' // contiene (subcadena, sin distinguir mayúsculas)
  | 'gt' // mayor que
  | 'lt' // menor que
  | 'between' // entre `value` y `value2` (ambos inclusive)
  | 'empty' // vacío (nulo o cadena vacía)
  | 'nonempty'; // no vacío

/** Función de agregación de un group-by / pivot. `first` = primer valor del grupo
 *  (conserva el valor de celda original, como min/max). */
export type QueryAggFn = 'sum' | 'count' | 'avg' | 'min' | 'max' | 'first';

/** Tipo de combinación (join) de un paso MERGE. `antiLeft` = filas de la izquierda
 *  SIN coincidencia en la derecha (no trae columnas). */
export type QueryJoinType = 'inner' | 'left' | 'right' | 'full' | 'antiLeft';

/** Dirección de ordenación. */
export type QuerySortDir = 'asc' | 'desc';

/** Extremo desde el que contar filas (keepRows/removeRows). */
export type QueryRowEnd = 'first' | 'last';

/** Filtrar filas por una condición sobre una columna. */
export interface QueryFilterStep {
  kind: 'filter';
  column: string;
  op: QueryFilterOp;
  /** Operando (eq/neq/contains/gt/lt y límite inferior de between); no usado por empty/nonempty. */
  value?: string;
  /** Límite superior de between. */
  value2?: string;
}
/** Conservar solo estas columnas, en este orden. */
export interface QuerySelectStep {
  kind: 'select';
  columns: string[];
}
/** Quitar estas columnas. */
export interface QueryRemoveColumnsStep {
  kind: 'removeColumns';
  columns: string[];
}
/** Renombrar una columna. */
export interface QueryRenameStep {
  kind: 'rename';
  column: string;
  to: string;
}
/** Reordenar: estas columnas pasan al frente (en este orden); el resto conserva el suyo. */
export interface QueryReorderStep {
  kind: 'reorder';
  columns: string[];
}
/** Cambiar el tipo de una columna (parsea el texto a número/fecha/booleano). */
export interface QueryChangeTypeStep {
  kind: 'changeType';
  column: string;
  type: QueryColType;
}
/** Ordenar por una o varias columnas (multi-nivel, en orden de prioridad). */
export interface QuerySortStep {
  kind: 'sort';
  by: { column: string; dir: QuerySortDir }[];
}
/** Agrupar por columnas + agregaciones (una columna de salida por agregación). */
export interface QueryGroupByStep {
  kind: 'groupBy';
  columns: string[];
  aggs: { column: string; fn: QueryAggFn; as: string }[];
}
/** Añadir una columna calculada (constante, interpolación `{col}` o aritmética simple). */
export interface QueryAddColumnStep {
  kind: 'addColumn';
  name: string;
  expr: string;
}
/** Reemplazar texto dentro de una columna (todas las apariciones). */
export interface QueryReplaceStep {
  kind: 'replace';
  column: string;
  find: string;
  replace: string;
}
/** Quitar filas duplicadas (por todas las columnas o por un subconjunto). */
export interface QueryDedupeStep {
  kind: 'dedupe';
  columns?: string[];
}
/** Conservar solo las primeras/últimas N filas. */
export interface QueryKeepRowsStep {
  kind: 'keepRows';
  end: QueryRowEnd;
  count: number;
}
/** Quitar las primeras/últimas N filas. */
export interface QueryRemoveRowsStep {
  kind: 'removeRows';
  end: QueryRowEnd;
  count: number;
}
/**
 * DESPIVOTAR (columnas→filas): colapsa las columnas «atributo» (`columns`) en dos
 * columnas —una con el NOMBRE de la columna colapsada (`attribute`) y otra con su
 * VALOR (`value`)—, repitiendo las columnas ancla (el resto) en cada fila nueva.
 * No descarta valores nulos/vacíos (round-trip exacto con `pivot`).
 */
export interface QueryUnpivotStep {
  kind: 'unpivot';
  /** Columnas a despivotar (las «atributo»); el resto son ancla y se repiten. */
  columns: string[];
  /** Nombre de la columna que recibe el NOMBRE de la columna despivotada. */
  attribute: string;
  /** Nombre de la columna que recibe el VALOR de la celda despivotada. */
  value: string;
}
/**
 * PIVOTAR (filas→columnas): los valores DISTINTOS de la columna `key` (en orden de
 * aparición) pasan a ser columnas nuevas; cada celda agrega (`fn`) los valores de
 * `value` de las filas que comparten esa clave. Las filas se agrupan por las
 * columnas RESTANTES. Combinación clave×grupo ausente → celda vacía.
 */
export interface QueryPivotStep {
  kind: 'pivot';
  /** Columna clave: sus valores distintos se convierten en columnas nuevas. */
  key: string;
  /** Columna cuyos valores se agregan en cada celda del resultado. */
  value: string;
  /** Agregación al esparcir (colisiones de múltiples filas por celda). */
  fn: QueryAggFn;
}
/**
 * ANEXAR (unión de filas): apila debajo las filas de OTRA consulta/tabla (`from`,
 * el nombre único de otra ExternalTable), ALINEANDO por NOMBRE de columna. La
 * unión de columnas = las de esta tabla + las de la otra que no aparezcan; las
 * ausentes en una u otra se rellenan con null.
 */
export interface QueryAppendStep {
  kind: 'append';
  /** Nombre de la otra ExternalTable cuyas filas se apilan. */
  from: string;
}
/**
 * COMBINAR (merge/join): empareja con OTRA tabla (`from`) por pares de columnas
 * clave (`on`), del tipo `join`, y trae las columnas seleccionadas de la derecha
 * (`bring`; vacío = todas menos las clave) al estilo «expandir» de Power Query.
 * Ante colisión de nombres antepone `prefix` (y un sufijo « (2)» si persiste).
 * Múltiples coincidencias → PRODUCTO (una fila por par).
 */
export interface QueryMergeStep {
  kind: 'merge';
  /** Nombre de la otra ExternalTable con la que combinar. */
  from: string;
  /** Tipo de combinación. */
  join: QueryJoinType;
  /** Pares de columnas clave [izquierda, derecha] (≥1). */
  on: { left: string; right: string }[];
  /** Columnas de la derecha a traer (vacío = todas menos las de la clave). */
  bring: string[];
  /** Prefijo para las columnas traídas ante colisión de nombres; ausente = sin prefijo. */
  prefix?: string;
}

/**
 * Un PASO del pipeline de transformación (estilo Power Query «M»): un objeto
 * discriminado por `kind`. El motor (sheets/querytransform.ts) aplica la lista en
 * orden a una tabla { columns, rows } y devuelve la tabla transformada (puro,
 * determinista, sin DOM). Persistido en `ExternalTable.transform`.
 */
export type QueryStep =
  | QueryFilterStep
  | QuerySelectStep
  | QueryRemoveColumnsStep
  | QueryRenameStep
  | QueryReorderStep
  | QueryChangeTypeStep
  | QuerySortStep
  | QueryGroupByStep
  | QueryAddColumnStep
  | QueryReplaceStep
  | QueryDedupeStep
  | QueryKeepRowsStep
  | QueryRemoveRowsStep
  | QueryUnpivotStep
  | QueryPivotStep
  | QueryAppendStep
  | QueryMergeStep;

/**
 * Tabla de DATOS EXTERNOS materializada (conector estilo Power Query): enlaza una
 * SheetTable de la hoja (por su `table` = nombre único) con la conexión del
 * BACKEND (`connectionId`) y la consulta (`spec`) que la generó, para poder
 * REFRESCARLA (re-ejecuta y reescribe las celdas). Las CREDENCIALES viven SOLO en
 * el backend (cifradas at-rest); el documento jamás las guarda. Los DATOS siguen
 * viviendo en `cells` (el texto crudo manda); esto solo añade la referencia para
 * refrescar. Campos nuevos opcionales y retrocompatibles (Go trata doc:sheet como
 * blob opaco). Saneado en dataquery.ts (sanitizeExternalTable).
 */
export interface ExternalTable {
  /** Nombre de la SheetTable materializada (une la definición con la tabla en la hoja). */
  table: string;
  /** Id de la conexión en el backend (las credenciales viven allí, nunca en el doc). */
  connectionId: string;
  /** Celda ancla A1 de la esquina superior izquierda del volcado. */
  anchor: string;
  /** Consulta a re-ejecutar para refrescar (forma del backend; sin secretos). */
  spec: ExtQuerySpec;
  /**
   * Pipeline de transformación (pasos aplicados) que se aplica al resultado crudo
   * del backend { columns, rows } antes de sembrar la SheetTable. Ausente o vacío =
   * datos crudos (retrocompatible). Motor puro en sheets/querytransform.ts.
   */
  transform?: QueryStep[];
  /** ISO-8601 del último refresco correcto (informativo). */
  refreshedAt?: string;
  /** De dónde vienen los datos, cuando el origen lo declara (Sytue Bases). */
  provenance?: ExtProvenance;
}

/**
 * Protección de HOJA (paridad Excel). Con la hoja protegida, las celdas
 * BLOQUEADAS (CellStyle.locked, por defecto true) pasan a solo lectura y las
 * `hidden` no revelan su fórmula. La PRESENCIA del objeto en `SheetData.protect`
 * = hoja protegida (como el autofiltro). Campos nuevos opcionales y
 * retrocompatibles (Go trata doc:sheet como blob opaco).
 */
/**
 * Rango de una hoja protegida que SOLO pueden editar ciertas personas o grupos, aunque
 * sus celdas estén bloqueadas. Vive dentro de `protect` (cambiarlo es cambiar la
 * protección: propietario o contraseña) y lo cumple el servidor. Las personas se guardan
 * por su `sub`, los grupos por su identificador.
 */
export interface RangoEditable {
  /** «B2», «B2:D10». El servidor admite además columnas «B:D» y filas «3:5». */
  ref: string;
  users?: string[];
  groups?: string[];
}

export interface SheetProtection {
  /**
   * El servidor tiene una contraseña (solo su hash) para desproteger. Lo FIJA el
   * servidor: el modelo nunca lleva la contraseña, ni en claro ni en hash. Se
   * teclea al proteger y viaja aparte (state/proteccion.ts); la verifica el servicio
   * Go, que además cumple la protección (celdas bloqueadas, opciones) frente a
   * escrituras directas a la API.
   */
  hasPassword?: boolean;
  /** Rangos editables por persona o grupo (ver `RangoEditable`). Ausente = ninguno. */
  ranges?: RangoEditable[];
  /** Permitir SELECCIONAR celdas bloqueadas. Ausente = true (criterio Excel). */
  selectCells?: boolean;
  /** Permitir aplicar FORMATO a las celdas con la hoja protegida. Ausente = false. */
  formatCells?: boolean;
  /* Permisos granulares del cuadro «Proteger hoja» de Excel. TODOS ausentes = false
   * (Excel los deja desmarcados): con la hoja protegida la acción queda vetada salvo
   * que se conceda explícitamente. Retrocompatibles: una hoja protegida de un doc
   * viejo simplemente no concede ninguno. */
  /** Insertar filas. */
  insertRows?: boolean;
  /** Eliminar filas. */
  deleteRows?: boolean;
  /** Insertar columnas. */
  insertCols?: boolean;
  /** Eliminar columnas. */
  deleteCols?: boolean;
  /** Ordenar rangos. */
  sort?: boolean;
  /** Usar el autofiltro (conmutarlo y cambiar sus criterios). */
  autoFilter?: boolean;
  /** Editar OBJETOS/anotaciones: formato condicional, validación, gráficos,
   *  sparklines y segmentaciones (el «Modificar objetos» de Excel). */
  objects?: boolean;
  /** Crear, actualizar o borrar ESCENARIOS del Análisis Y-si. */
  scenarios?: boolean;
}

/**
 * Escenario del Análisis Y-si (Administrador de escenarios, paridad Excel): un
 * nombre + un conjunto de celdas de ENTRADA con el valor (texto crudo) que toma
 * cada una. Aplicar un escenario escribe esos valores en las celdas y la hoja
 * recalcula; el informe resumen compara varios escenarios contra unas celdas de
 * resultado. Campos nuevos opcionales y retrocompatibles (Go trata doc:sheet
 * como blob opaco). Helpers puros en sheets/scenario.ts.
 */
export interface Scenario {
  /** Nombre (único dentro de la hoja) del escenario. */
  name: string;
  /** Celda de entrada (ref A1 en MAYÚSCULAS) → valor a escribir (texto crudo). */
  values: Record<string, string>;
}

/**
 * Grupo de ESQUEMA (outline) de filas o de columnas (paridad Excel). Cubre las
 * unidades de DETALLE [start,end] (0-based, inclusive) a un nivel de anidamiento
 * (1 = el más externo); colapsarlo oculta ese detalle. El resumen (p. ej. la fila
 * de SUBTOTAL) vive FUERA del grupo (en end+1), así que nunca se oculta. Campos
 * nuevos opcionales y retrocompatibles (Go trata doc:sheet como blob opaco).
 */
export interface SheetGroup {
  /** Primera unidad (fila/columna) 0-based del detalle. */
  start: number;
  /** Última unidad 0-based del detalle (inclusive). */
  end: number;
  /** Nivel de esquema 1-based (1 = el más externo). */
  level: number;
  /** Colapsado: su detalle [start,end] se oculta. Ausente = expandido (solo se persiste el true). */
  collapsed?: boolean;
}

export interface SheetData {
  /** Identidad estable de la hoja: no cambia al renombrarla ni al moverla. La co-edición
   *  (state/fusion.ts) y las operaciones de hoja se anclan en ella y no en el nombre.
   *  Ausente en documentos anteriores: se deduce del nombre al cargar (sheets/idhoja.ts). */
  id?: string;
  /** Identidad de las filas y de las columnas (sheets/anclas.ts); ausente = todas implícitas. */
  rowIds?: { id: string }[];
  rowBase?: number;
  colIds?: { id: string }[];
  colBase?: number;
  name: string;
  /** Color de la pestaña #RRGGBB (el `tabColor` de Excel); ausente = sin color. */
  tabColor?: string;
  /** 'A1'..'Z100' → HTML de la celda (solo celdas no vacías). */
  cells: Record<string, string>;
  /** ref → formato de número (solo celdas con formato). */
  formats?: Record<string, NumFmt>;
  /** ref → formato visual de celda (fuente, color, relleno, bordes…). */
  styles?: Record<string, CellStyle>;
  /** Reglas de formato condicional de la hoja, en orden de prioridad. */
  condFormats?: CondRule[];
  /** Mini-gráficos de celda (sparklines) de la hoja. */
  sparklines?: Sparkline[];
  /** Validaciones de datos de la hoja (lista de valores por rango); la última que cubre una celda gana. */
  validations?: DataValidation[];
  /** Paneles congelados: nº de columnas/filas fijas desde el origen. */
  freeze?: { cols: number; rows: number };
  /** Autofiltro persistente (desplegables por encabezado). */
  autofilter?: SheetAutoFilter;
  /** Tablas con formato (ListObject) de la hoja. */
  tables?: SheetTable[];
  /** Tablas de datos externos materializadas (conector Power Query); ausente = ninguna. */
  externalTables?: ExternalTable[];
  charts?: ChartSpec[];
  /** Segmentaciones de datos (slicers): filtran columnas del autofiltro en vivo. */
  slicers?: Slicer[];
  /** Protección de hoja (celdas bloqueadas en solo lectura); ausente = sin proteger. */
  protect?: SheetProtection;
  /** Grupos de esquema (agrupar/subtotales) de FILAS; ausente = sin esquema de filas. */
  rowGroups?: SheetGroup[];
  /** Grupos de esquema de COLUMNAS; ausente = sin esquema de columnas. */
  colGroups?: SheetGroup[];
  /** Escenarios del Análisis Y-si (Administrador de escenarios); ausente = ninguno. */
  scenarios?: Scenario[];
  /** Tablas dinámicas de la hoja (su definición; lo que escriben son celdas normales). */
  pivots?: SheetPivot[];
  /** Anchos de columna personalizados en px de maqueta (zoom 100 %): clave = índice de
   *  columna 0-based en texto. Ausente (o columna sin clave) = el ancho de serie. */
  colWidths?: Record<string, number>;
  /** Altos de fila personalizados en px de maqueta: clave = índice de fila 0-based en
   *  texto. Ausente (o fila sin clave) = el alto de serie. */
  rowHeights?: Record<string, number>;
  /** Celdas combinadas: rangos A1 de más de una celda que no se solapan (la de arriba a la
   *  izquierda es el ancla; las demás quedan cubiertas). Ausente = ninguna. */
  merges?: string[];
}

export interface SlidesModel {
  slides: Slide[];
  /** Tema (fondo + tinta + fuente) de TODA la presentación: el id de un preset
   *  (SlideThemeId) o de un tema propio (`themes`); ausente = Papel (fondo blanco,
   *  tinta negra, fuente de la app). Retrocompatible. */
  theme?: string;
  /** Temas propios de la presentación (colores y fuente que fija quien la hace);
   *  ausente = ninguno (slides/temas.ts). */
  themes?: SlideCustomTheme[];
  /** Patrón de diapositiva (slide master): cajas compartidas que se pintan
   *  DETRÁS de las de cada diapositiva (logo, pie, número de página…). Ausente =
   *  sin patrón (comportamiento clásico). Retrocompatible. */
  master?: SlideMaster;
  /** Tamaño/proporción del lienzo; ausente = 16:9 (retrocompatible). */
  size?: SlideSize;
  /** Protección de la presentación (diapositivas y cajas bloqueadas, patrón y tema); la
   *  cumple el servidor. Ausente = sin proteger (retrocompatible). */
  protect?: SlidesProtection;
}
/**
 * Protección propia de Diapos (la cumple el servicio Go: domain/proteccion_diapos.go).
 * La PRESENCIA del objeto = presentación protegida. Con ella, lo bloqueado queda fijo para
 * todos salvo la persona propietaria y quien presente la contraseña, y cambiar la propia
 * protección es lo mismo. Sin contraseña solo la salta la persona propietaria.
 */
export interface SlidesProtection {
  /** El servidor tiene una contraseña (solo su hash). Lo FIJA el servidor: el modelo nunca
   *  lleva la contraseña (se teclea y viaja aparte, state/proteccion.ts). */
  hasPassword?: boolean;
  /** Diapositivas bloqueadas (ids): contenido fijo y no se pueden borrar. */
  slides?: string[];
  /** Cajas bloqueadas (ids): quedan como están y en su diapositiva. */
  boxes?: string[];
  /** Patrón (con sus diseños), tema, temas propios y tamaño del lienzo fijos. */
  master?: boolean;
}
/** Tamaño (proporción) del lienzo de la presentación (fase Slides-Tamaño).
 *  Ausente en el modelo = 16:9. Como las cajas van en % del lienzo, el render
 *  solo usa la PROPORCIÓN (se reescalan solas); el ancho×alto de 'custom' se
 *  guarda para el round-trip y para derivar esa proporción. Retrocompatible. */
export type SlideSizePreset = '16:9' | '4:3' | 'custom';
export interface SlideSize {
  preset: SlideSizePreset;
  /** Solo 'custom': ancho y alto (> 0) en `unit`; su cociente da la proporción. */
  w?: number;
  h?: number;
  /** Solo 'custom': unidad de w/h (cosmética; la proporción es adimensional).
   *  Ausente = 'px'. */
  unit?: 'px' | 'cm';
}
/** Patrón de diapositiva: elementos comunes a TODAS las diapositivas. Sus cajas
 *  de texto admiten los campos {n} (número de diapositiva, 1-based) y {total}
 *  (total), que se sustituyen POR diapositiva al pintar (substituteMasterFields). */
export interface SlideMaster {
  boxes: SlideBox[];
  /** Diseños de la presentación (slides/disenos.ts); ausente = los tres de serie. */
  layouts?: SlideLayout[];
}
/** Diseño: un nombre y los marcadores de posición (cajas de texto con su muestra) de los
 *  que parte una diapositiva. */
export interface SlideLayout {
  id: string;
  name: string;
  boxes: SlideBox[];
  /**
   * Formas del diseño que no son marcadores (un logo, una franja, un texto fijo) y sus
   * líneas: guardadas UNA vez aquí, se pintan debajo de cada diapositiva que lleva el diseño
   * y encima del patrón, como en PowerPoint. Las trae un .pptx o un .odp importado; un diseño
   * sin ellas se pinta como siempre.
   */
  decor?: SlideBox[];
  connectors?: SlideConnector[];
  /** El diseño oculta las cajas del patrón en sus diapositivas (`showMasterSp="0"`). */
  hideMaster?: boolean;
}
/** Tema propio: fondo y tinta (#RRGGBB) y, si la fija, la familia tipográfica. */
export interface SlideCustomTheme {
  id: string;
  name: string;
  bg: string;
  ink: string;
  font?: string;
}
/** Transición de ENTRADA de una diapositiva en el modo presentación. 'morph'
 *  interpola las cajas emparejadas con la diapositiva anterior (slides/anim.ts). */
export type SlideTransition = 'none' | 'fade' | 'slide' | 'morph';
/** Tema de presentación (fase Slides-Temas): id de un preset de SLIDE_THEMES
 *  (fondo/tinta/fuente sólidos, anti-slop). Ausente/desconocido = Papel. */
export type SlideThemeId = 'carbon' | 'pizarra' | 'arena' | 'menta' | 'cielo';
export interface Slide {
  id: string;
  boxes: SlideBox[];
  /** Conectores (líneas que unen formas o puntos) de la diapositiva; ausente =
   *  ninguno. Retrocompatible (un doc previo sin este campo carga igual). */
  connectors?: SlideConnector[];
  /** Ausente = 'none' (retrocompatible; Go trata doc:slides como blob opaco). */
  transition?: SlideTransition;
  /** Notas del ponente (texto plano); ausente = sin notas (retrocompatible). */
  notes?: string;
  /** Id del diseño (de `master.layouts`) con el que está hecha; ausente = ninguno. */
  layout?: string;
  /** Color de fondo propio de la diapositiva (#RRGGBB); ausente = el del tema. */
  bg?: string;
  /** Oculta los elementos del patrón (slide master) en ESTA diapositiva (p. ej.
   *  la portada). Ausente/false = se muestran. Retrocompatible. */
  hideMaster?: boolean;
}
/** Destino de un enlace de acción de una caja al hacer clic en presentación
 *  (fase Slides-Enlaces): salto relativo o a una diapositiva concreta (1-based). */
export type SlideLinkTarget = 'first' | 'last' | 'next' | 'prev' | number;
/** Forma de una caja `kind: 'shape'`. Núcleo (fase Slides-Elementos):
 *  rect/ellipse/line/arrow. Ampliación (fase Slides-Conectores + más formas):
 *  polígonos y cilindro/bocadillo, saneados con reserva a 'rect'. Retrocompat. */
export type SlideShape =
  | 'rect'
  | 'ellipse'
  | 'line'
  | 'arrow'
  | 'triangle'
  | 'diamond'
  | 'pentagon'
  | 'hexagon'
  | 'star'
  | 'cylinder'
  | 'callout';

/* ---------- Conectores (fase Slides-Conectores) ----------
 * Una línea que une dos formas (por id, se recalcula al moverlas) o dos puntos
 * libres. Vive en `Slide.connectors` (colección aparte, NO en `boxes`) para no
 * tocar el resto del pipeline de cajas (anim/morph/organizar/miniaturas). */

/** Lado de anclaje de un extremo al borde de una forma ('auto' = el borde que
 *  mira al otro extremo) y trazado de un conector (recto, codo o curvo): son los
 *  de la escena común (`@sytue/escena`). */
export type { ConnectorAnchor, ConnectorKind };
/** Extremo de un conector: anclado a una caja (por id) o punto libre en % del
 *  lienzo. Campos opcionales y saneados al cargar (sanitizeConnectors). */
export interface ConnectorEnd {
  /** Id de la caja a la que se ancla; ausente = punto libre (usa x/y). */
  boxId?: string;
  /** Lado de anclaje; ausente = 'auto' (borde más cercano al otro extremo). */
  anchor?: ConnectorAnchor;
  /** Punto libre en % del lienzo (0..100); solo si no hay boxId. */
  x?: number;
  y?: number;
}
/** Conector de una diapositiva (geometría calculada al pintar según la posición
 *  actual de las formas ancladas: slides/connector.ts). */
export interface SlideConnector {
  id: string;
  kind: ConnectorKind;
  from: ConnectorEnd;
  to: ConnectorEnd;
  /** Punta de flecha en el extremo inicial/final. Ausente = sin punta. */
  startArrow?: boolean;
  endArrow?: boolean;
  /** Color del trazo #RRGGBB; ausente = la tinta del tema (currentColor). */
  stroke?: string;
}

/** Categoría de un efecto de animación (paridad PowerPoint). NO se persiste: se
 *  DERIVA del tipo (animCategory en slides/anim.ts) para no poder desincronizarse.
 *  - entrance: revela la caja (aparece).
 *  - emphasis: llama la atención sin cambiar la visibilidad (vuelve al reposo).
 *  - exit: oculta la caja (desaparece).
 *  - motion: la caja se DESPLAZA siguiendo una trayectoria (no cambia su
 *    visibilidad; geometría en slides/anim.ts, puntos en SlideAnim.points). */
export type SlideAnimCategory = 'entrance' | 'emphasis' | 'exit' | 'motion';
/** Disparador de una anim en la secuencia: 'click' abre un PASO nuevo (avanza con
 *  un clic), 'with' se reproduce A LA VEZ que la anterior y 'after' se encadena
 *  tras ella (escalonada). Ausente = 'click' (retrocompat). */
export type SlideAnimTrigger = 'click' | 'with' | 'after';
/** Efecto de animación de una caja. Entrada (retrocompat): appear/fade/fly/zoom.
 *  Énfasis: pulse/spin/grow-shrink. Salida: disappear/fade-out/fly-out/zoom-out.
 *  Trayectorias (motion): líneas en 8 direcciones, rectángulo (circuito), arco y
 *  personalizada (usa SlideAnim.points). La categoría se deriva del tipo
 *  (SlideAnimCategory), no se guarda. */
export type SlideAnimType =
  | 'appear'
  | 'fade'
  | 'fly'
  | 'zoom'
  | 'pulse'
  | 'spin'
  | 'grow-shrink'
  | 'disappear'
  | 'fade-out'
  | 'fly-out'
  | 'zoom-out'
  | 'motion-right'
  | 'motion-left'
  | 'motion-up'
  | 'motion-down'
  | 'motion-up-right'
  | 'motion-up-left'
  | 'motion-down-right'
  | 'motion-down-left'
  | 'motion-loop'
  | 'motion-arc'
  | 'motion-custom';
/** Punto de una trayectoria de movimiento: DESFASE respecto al inicio, en puntos
 *  porcentuales del lienzo (dx = % del ancho, dy = % del alto). El primer punto de
 *  una trayectoria es siempre {0,0} (la posición actual de la caja). */
export interface MotionPoint {
  dx: number;
  dy: number;
}
/** Una animación de la secuencia de una caja. `order` = posición GLOBAL 1..N en
 *  la diapositiva (contigua tras normalizar); el disparador agrupa las anims en
 *  PASOS (clics). Retrocompat: el viejo {type, order} de ENTRADA sigue siendo
 *  válido (sin `trigger` = 'click', categoría 'entrance'). */
export interface SlideAnim {
  type: SlideAnimType;
  order: number;
  /** Ausente = 'click' (retrocompat). */
  trigger?: SlideAnimTrigger;
  /** Solo `type: 'motion-custom'`: vértices de la trayectoria personalizada (% del
   *  lienzo, relativos al inicio {0,0}). Los tipos de trayectoria PREDEFINIDOS
   *  derivan su geometría (motionPoints en slides/anim.ts) y no persisten puntos.
   *  Ausente = trayectoria por defecto. Se sanea al leer (coerceAnim). */
  points?: MotionPoint[];
}

/** Disposición de un diagrama SmartArt de una caja `kind: 'smartart'` (fase
 *  Slides-SmartArt): lista en tarjetas, proceso con flechas, ciclo en anillo u
 *  organigrama jerárquico. */
export type SmartArtLayout = 'list' | 'process' | 'cycle' | 'hierarchy';
/** Elemento (nodo) de un diagrama SmartArt. `level` es la sangría 0..N que solo
 *  interpreta el layout 'hierarchy' (0 = raíz, 1 = hijo…); el resto de layouts
 *  la ignora. Ausente = 0. */
export interface SmartArtItem {
  text: string;
  level?: number;
}
/** Diagrama SmartArt incrustado en una diapositiva: una disposición + la lista
 *  de elementos. El auto-layout (posiciones/tamaños de nodos y conectores) se
 *  calcula al pintar según el tamaño de la caja (slides/smartart.ts) y NO se
 *  persiste; solo se guardan `layout` e `items`. */
export interface SlideSmartArt {
  layout: SmartArtLayout;
  items: SmartArtItem[];
}

/** Tipo de gráfico de una caja `kind: 'chart'` (fase Slides-Gráfico). */
export type SlideChartType = 'bar' | 'line' | 'pie';
/** Gráfico incrustado en una diapositiva: datos editables en rejilla que se
 *  interpretan igual que un rango de sheets (extractChartData de sheets/Chart.tsx):
 *  1.ª columna = rótulos de categoría, 1.ª fila = cabecera que nombra las series. */
export interface SlideChart {
  type: SlideChartType;
  /** Rejilla por filas de texto crudo (número en es-ES o texto). Rectangular. */
  grid: string[][];
}

/** Pista de subtítulos de un vídeo o un audio: el `.vtt` entero, su idioma (BCP 47) y si es
 *  la que se muestra por defecto (a lo sumo una por medio). */
export interface SlideTrack {
  lang: string;
  /** Nombre que ve quien reproduce; ausente = el idioma en su propia lengua. */
  label?: string;
  vtt: string;
  default?: boolean;
}

/** Caja de diapositiva; x/y/w/h en % del lienzo 16:9 (0..100).
 * Campos nuevos TODOS opcionales: un doc previo {id,x,y,w,h,html} carga igual
 * (Go trata doc:slides como blob opaco). */
export interface SlideBox {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
  /** Ausente o 'text' = caja de texto (retrocompatible). */
  kind?: 'text' | 'image' | 'shape' | 'chart' | 'smartart' | 'media';
  /** HTML del texto (kind text); '' en imagen/forma/gráfico/smartart/medio. */
  html: string;
  /** Origen del medio: dataURL reescalado en cliente (kind image; < ~700 KiB, ver
   *  Insertar.tsx) o, en kind 'media', la URL o el dataURL del audio/vídeo. */
  src?: string;
  /** Solo kind 'media': tipo de medio incrustado. */
  mediaType?: 'video' | 'audio';
  /** Solo kind 'media' (vídeo): imagen de portada (URL o dataURL) para el marcador
   *  del escenario y el atributo `poster` del <video>. */
  poster?: string;
  /** Solo kind 'media': reproducir al entrar en la diapositiva (el vídeo se silencia
   *  para que el navegador lo permita). Ausente = false (solo se persiste el `true`). */
  autoplay?: boolean;
  /** Solo kind 'media': reproducir en bucle. Ausente = false. */
  loop?: boolean;
  /** Solo kind 'media': mostrar los controles del reproductor. Ausente = con
   *  controles (solo se persiste el `false`). */
  controls?: boolean;
  /** Solo kind 'media': subtítulos WebVTT (slides/subtitulos.ts). Ausente = sin ellos. */
  tracks?: SlideTrack[];
  /** Solo kind 'shape'. */
  shape?: SlideShape;
  /** Relleno #RRGGBB de CONTENT_PALETTE ('transparent' = sin relleno). */
  fill?: string;
  /** Borde/trazo #RRGGBB de CONTENT_PALETTE. */
  stroke?: string;
  /** Solo kind 'chart': tipo + datos editables. */
  chart?: SlideChart;
  /** Solo kind 'smartart': disposición + elementos del diagrama. */
  smartart?: SlideSmartArt;
  /** Secuencia de animaciones en presentación (entrada/énfasis/salida por
   *  disparador). Ausente = sin animación. Se sanea/renumera al cargar y editar
   *  (normalizeAnimOrders de slides/anim.ts). */
  anims?: SlideAnim[];
  /** Modelo ANTIGUO (compat de lectura): UNA animación de entrada. Se migra a
   *  `anims` al leer (boxAnims de slides/anim.ts) y no se emite en documentos
   *  nuevos. Se mantiene declarado solo para leer presentaciones viejas. */
  anim?: SlideAnim;
  /** Enlace de acción: al hacer clic en presentación salta a esta diapositiva
   *  (o relativo). Ausente = la caja no navega (el clic avanza como siempre). */
  action?: SlideLinkTarget;
  /** Grupo al que pertenece la caja (fase Slides-Organizar): las cajas con el
   *  mismo `groupId` se mueven/alinean/animan juntas. Ausente = suelta.
   *  Retrocompatible (un doc previo sin este campo carga igual). */
  groupId?: string;
}

/* ---------- EL contrato: contexto que recibe toda zona ---------- */

export interface EditorContext {
  doc: DocumentMeta; // camelCase (BFF /api/documents existente)
  kind: DocKind;

  /* El idioma negociado en el servidor viaja por el contexto y no por props zona a
   * zona: la cinta, los paneles y los lienzos ya reciben `ctx`, y duplicar el
   * cableado solo abriría la puerta a que una zona hable otro idioma (§13.1). */
  locale: Locale;
  /** Traduce una clave del catálogo de la app (`src/i18n`) al idioma de `locale`. */
  t: DocsT;

  /** ¿Está desplegado el servicio de IA (SYTUE_AI_URL en el BFF)? Lo decide el
   *  servidor. Si no lo está, la cinta y el carril NO pintan la puerta: el panel
   *  solo podría devolver 503 y una función que no ocurre no se enseña. */
  aiEnabled: boolean;

  title: string;
  setTitle: (next: string) => void; // optimista + POST /api/rename con debounce 800 ms

  save: SaveState;
  /** Conectividad efectiva del documento (incluye fallback desde caché del shell nativo). */
  connection: 'online' | 'offline';

  /** Último contenido cargado/adoptado. NO cambia con cada tecla (§4.4). */
  content: string;
  /** Incrementa cuando `content` debe re-aplicarse al DOM (carga, adopción remota, restauración). */
  contentEpoch: number;
  /** El lienzo informa de cada edición; programa el autosave. No re-renderiza el lienzo. */
  onContentInput: (next: string) => void;

  fmt: FormatState;
  /** exec de canvas/exec.ts, tal cual (§3). */
  exec: (command: EditorCommand, value?: string) => void;

  ui: EditorUi;
  setUi: (patch: Partial<EditorUi>) => void;

  pageSetup: PageSetup;
  setPageSetup: (patch: Partial<PageSetup>) => void;

  stats: EditorStats;
  /** Los lienzos informan pages/page (words lo pone useDocument). */
  reportStats: (patch: Partial<EditorStats>) => void;

  peers: Peer[];
  /** Cursores remotos en vivo (efímeros, por el WS de colaboración). */
  peerCursors: PeerCursor[];
  /** Publica la posición local de cursor/selección a los peers (throttled; no-op sin WS). */
  sendCursor: (pos: CursorPos) => void;

  comments: CommentItem[];
  /** `anchor` = ancla libre (jsonb) del backend; el editor guarda `{ mark, text }`. */
  addComment: (body: string, anchor?: unknown) => Promise<void>;
  resolveComment: (id: string, resolved: boolean) => Promise<void>;
  removeComment: (id: string) => Promise<void>;

  versions: VersionItem[];
  loadVersions: () => Promise<void>;
  createVersion: (label?: string) => Promise<void>;
  /** Restaura y recarga estado (content + epoch++). */
  restoreVersion: (versionId: string) => Promise<void>;
  /** Aplica un tema del catálogo común o la marca de la organización; el servicio solo
   *  cambia la presentación y deja un punto de restauración. */
  applyTheme: (themeId: string, brandVersion?: number) => Promise<void>;
  /** Qué cambiaría aplicar ese tema (diff legible, sin escribir nada); null si falla. Ausente
   *  en las demos. */
  previewTheme?: (themeId: string, brandVersion?: number) => Promise<TemaPrevia | null>;

  /** POST /api/export → el BFF proxea el blob (attachment) y dispara la descarga. Errores → toast. */
  exportAs: (format: ExportFormat, plantilla?: boolean) => Promise<void>;
  /** Fichero de Drive del que salió el documento, o '' si nació aquí. */
  sourceRef: string;
  /** Cursores anclados a la identidad de los caracteres (state/coedicion.ts). Solo en documentos
   *  de texto con la secuencia en sincronía; ambas devuelven null si no se puede. */
  cursorDots: CursorDots;
  /**
   * ¿Solo lectura? Lo dice el SERVICIO (el rol del documento), y también es `true`
   * mientras la carga no ha contestado.
   *
   * No es solo para pintar la interfaz apagada: `exec` y `onContentInput` lo
   * comprueban, y ahí está la barrera de verdad. Poner el lienzo en
   * `contentEditable=false` no basta — la cinta, los atajos y el pegado seguirían
   * escribiendo, que es cómo un lector acababa perdiendo su trabajo al guardar.
   */
  soloLectura: boolean;
  /** ¿Soy el propietario? Solo él ve la actividad del documento (GET …/audit, 403 al resto). */
  esPropietario: boolean;
  savingToDrive: boolean;
  /** Devuelve lo editado a su fichero de Drive (queda allí como versión nueva). */
  saveToDrive: () => Promise<void>;
  /** Carpeta de Drive de la que vino el documento para volver a ella al salir. */
  volverA?: string | undefined;
}

/** Una línea del diff de «Actualizar a la marca vigente» (POST /api/themes con `preview`). */
export interface CambioTema {
  /** `logo`: el logotipo de la organización se pone o se cambia (`to`) o se quita. */
  scope: 'headings' | 'table_headers' | 'body_font' | 'sheet_header' | 'slide_theme' | 'logo';
  sheet?: string;
  count: number;
  from?: string;
  to?: string;
}

export interface TemaPrevia {
  changed: boolean;
  changes: CambioTema[];
  /** Versión de la marca que se aplicaría (0 si no es la marca). */
  version: number;
}
