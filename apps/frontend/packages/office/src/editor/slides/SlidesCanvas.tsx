/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SlidesCanvas — lienzo de presentación (kind `presentation`, BLUEPRINT.md §7.2).
 * Carril de miniaturas (añadir/duplicar/eliminar), escenario 16:9 de 960×540
 * escalado al hueco disponible × zoom, cajas de texto contenteditable
 * seleccionables, editables y arrastrables (asa de mover superior + asa de
 * redimensionar SE, con Pointer Events). Persiste serializando SlidesModel por
 * el mismo camino que el folio: `ctx.onContentInput(JSON.stringify(model))`.
 *
 * Decisiones fuera del blueprint (tareas B11 y Slides-Elementos):
 * - La tarea añade «duplicar diapositiva» al carril del §7.2 (icono slideAdd).
 * - La pestaña InicioSlides necesita «nueva diapositiva» y «disposición (3
 *   plantillas)»: este fichero expone un mini-registro de comandos
 *   (dispatchSlidesCommand) análogo al de SheetCanvas; la fase Slides-Elementos
 *   añade 'imagen' (dataURL ya reescalado por Insertar.tsx) y 'forma'.
 * - El acento de edición del lienzo es el latón de suite (--sytue-primary, como
 *   Docs/Datos); el color de producto (ocre) se reserva al tile del launcher.
 * - Tailwind resetea h1/h2: la tipografía del CONTENIDO de las cajas se emite
 *   aquí (<style> .sytue-slidebox), porque editor.css (§8) es de A1.
 * - Accesibilidad: con la caja seleccionada, flechas mueven (1 %) y
 *   Mayús+flechas redimensionan; Supr elimina; Enter/F2 editan (solo texto).
 *   Imagen/forma son focalizables (tabIndex) para operar por teclado.
 * - renderBox(t, ) es EL render compartido del contenido de una caja: escenario,
 *   miniaturas y el modo presentación pintan exactamente lo mismo
 *   (sin contenteditable ni tiradores fuera del escenario).
 * - Techo del modelo serializado: el op-body del colab Go corta en 1 MiB
 *   (services/docs/internal/collab/client.go, maxMessageBytes) y por encima el
 *   autosave SE PIERDE; insertar imagen valida el tamaño total antes de aplicar.
 * - Modo presentación (fase Slides-Presentación): overlay fijo a viewport (ese
 *   ES el fallback) sobre el que se intenta la Fullscreen API; UNA diapositiva
 *   escalada al viewport (16:9 conservado) con el MISMO renderBox(t, ) estático.
 *   La transición de entrada por diapositiva (ninguna/fundido/deslizar) se
 *   elige en la mini-barra del escenario, no en el ribbon: ahí el select puede
 *   mostrar el valor REAL de la diapositiva activa (el ribbon no ve el modelo).
 * - El carril de miniaturas reordena con drag and drop nativo (paridad con las
 *   pestañas multi-hoja de Sheets); cada movimiento es un commit (apply).
 */
import { render } from 'preact';
import type { CSSProperties, JSX } from 'preact';
import type { DocsKey, DocsT } from '../../i18n/index';
import { EditorI18nCtx, useT } from '../context';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  useToast,
  FOCUSABLE,
  useUltimo,
  useMobileBack,
  calcularArrastre,
  calcularRedimension,
  calcularLimitesArrastre,
} from '@sytue/ui';
import { saneaHtmlDeDocumento } from '@sytue/ui';
import { bloquesDe } from '@sytue/ai';
import { diapositivasDe, htmlDeCajas } from './iaslides.js';
import { escapeHtml } from '../alphaindex.js';
import { confirmarEnEditor } from '@sytue/ui';
import { orgActual } from '@sytue/config';
import type {
  ConnectorEnd,
  ConnectorKind,
  EditorCommand,
  EditorContext,
  MotionPoint,
  Slide,
  SlideAnimCategory,
  SlideAnimType,
  SlideAnimTrigger,
  SlideBox,
  SlideChart,
  SlideChartType,
  SlideConnector,
  SlideCustomTheme,
  SlideLayout,
  SlideLinkTarget,
  SlideMaster,
  SlideShape,
  SlidesModel,
  SlidesProtection,
  SlideSize,
  SlideSmartArt,
  SlideThemeId,
  SlideTrack,
  SlideTransition,
  SmartArtItem,
  SmartArtLayout,
} from '../types';
import { setEditorRoot } from '../canvas/exec';
import { recordStep } from '../macros';
import { cursorPosKey, peerName, slidePeerBoxes } from '../state/cursors';
import { EditorIcon } from '../icons';
import type { EditorIconName } from '../icons';
import { smartArtLayout } from './smartart';
import { SubtitulosPanel } from './SubtitulosPanel';
import { CastDialog } from './CastDialog';
import { avisador, posicionDe, urlDeEmision } from './cast';
import { sanitizeTracks, trackLabel, trackLangs, trackSrc } from './subtitulos';
import { TemaPanel } from './TemaPanel';
import { ProteccionPanel } from './ProteccionPanel';
import {
  alternarId,
  cambioProtegido,
  cajaBloqueada,
  conBloqueadoOriginal,
  diapoBloqueada,
  sanitizeSlidesProtection,
} from './proteger';
import { AMBITO_DIAPOS, esPropietario, presentarClave, presentarNueva } from '../state/proteccion';
import { DisenosBarra } from './DisenosBarra';
import {
  MAX_CUSTOM_THEMES,
  addCustomTheme,
  patchCustomTheme,
  removeCustomTheme,
  sanitizeCustomThemes,
} from './temas';
import {
  MAX_LAYOUTS,
  addLayout,
  applyLayoutBoxes,
  boxesFromLayout,
  defaultNewLayoutId,
  duplicateLayout,
  layoutEditBoxes,
  layoutsOf,
  mergeIntoLayout,
  patchLayout,
  removeLayout,
  sanitizeLayouts,
} from './disenos';
import { diapositivaDeBloque } from './bloque';
import {
  ANIM_CATEGORY,
  ANIM_INSTANT,
  animStepCount,
  boxAnims,
  boxFiringAtStep,
  boxPresentAtStep,
  migrateSlide,
  morphDelta,
  morphPairs,
  motionEnd,
  motionPathD,
  motionPoints,
  motionRenderOrigin,
  motionRestOffset,
  normalizeAnimOrders,
  planByBox,
  presentAdvance,
  reorderAnimOrder,
  slidePlan,
  withAnims,
} from './anim';
import {
  alignBoxes,
  boundingBox,
  distributeBoxes,
  expandGroups,
  groupBoxes,
  reorderZ,
  ungroupBoxes,
} from './organize';
import type { AlignEdge, AlignRef, ZOrderOp } from './organize';
import {
  arrowHead,
  connectorRoute,
  pruneConnectors,
  remapConnectorIds,
  sanitizeConnectors,
} from './connector';
import type { ConnEnd, CRect } from './connector';
import {
  cylinderBodyPath,
  cylinderDims,
  cylinderSeamPath,
  polygonAttr,
  polygonPoints,
} from './shapes';
import {
  CONTENT_PALETTE,
  HIGHLIGHT_PALETTE,
  RibbonButton,
  RibbonColorSwatch,
  RibbonSelect,
} from '../ribbon/primitives';
// Reutiliza la interpretación de datos y la escala de eje de los gráficos de la
// hoja (clean-room, ya validadas): un gráfico de diapositiva NO reimplementa
// esa lógica, solo dibuja marcas que LLENAN su caja (a diferencia de la tarjeta
// fija de 320 px de sheets/Chart.tsx, inservible dentro de una caja redimensionable).
import { extractChartData, niceTicks } from '../sheets/Chart';
import type { ChartData } from '../sheets/Chart';
import { editorT } from '../../i18n';
import { comandoDelAtajo } from '../comandos';
import '../canvas/superficies.css';

/* ---------- Comandos de la pestaña InicioSlides / Insertar / Vista ---------- */

export type SlidesCommand =
  | 'nueva'
  | 'disposicion'
  | 'imagen'
  | 'forma'
  | 'grafico'
  | 'smartart'
  | 'media'
  | 'patron'
  | 'organizar' // value: op de organización (ver ORGANIZE_OPS)
  | 'conector' // value: ConnectorKind (straight/elbow/curved)
  | 'tamano' // value: JSON de SlideSize (o vacío = 16:9)
  | 'iafill' // value: la respuesta del asistente — la reparte en diapositivas
  | 'bloque' // value: JSON de la diapositiva de un bloque reutilizable — va detrás de la actual
  | 'presentar';
export type SlideLayoutId = 'titulo' | 'titulo-cuerpo' | 'dos-columnas';

let slidesCommandHandler: ((cmd: SlidesCommand, value?: string) => void) | null = null;

/** Envía un comando al SlidesCanvas montado (no-op si no hay ninguno). Si hay
 *  grabación de macro activa, registra el comando antes de despacharlo
 *  (recordStep es no-op fuera de grabación: no altera el despacho normal). */
export function dispatchSlidesCommand(cmd: SlidesCommand, value?: string): void {
  if (!slidesCommandHandler) return;
  recordStep('presentation', cmd, value);
  slidesCommandHandler(cmd, value);
}

/* ---------- Organizar objetos: op → helper puro (fuente única) ----------
 * El `value` del comando 'organizar' es una de estas claves. Mapear aquí (y no
 * en el ribbon) evita que la UI conozca la lógica pura de organize.ts. */
const Z_OPS: Record<string, ZOrderOp> = {
  front: 'front',
  forward: 'forward',
  backward: 'backward',
  back: 'back',
};
const ALIGN_OPS: Record<string, AlignEdge> = {
  'align-left': 'left',
  'align-hcenter': 'hcenter',
  'align-right': 'right',
  'align-top': 'top',
  'align-vmiddle': 'vmiddle',
  'align-bottom': 'bottom',
};

/** Resumen de la selección del lienzo montado para habilitar los comandos
 *  «Organizar» del ribbon (que no ve el estado de React del lienzo). */
export interface SlidesSelectionSnapshot {
  /** Nº de cajas en la selección efectiva (con los grupos ya expandidos). */
  count: number;
  /** Alguna caja seleccionada pertenece a un grupo (habilita «Desagrupar»). */
  hasGroup: boolean;
}

let slidesSelSnapshot: SlidesSelectionSnapshot = { count: 0, hasGroup: false };

/** Última instantánea publicada por el SlidesCanvas montado. ponytail: variable
 *  de módulo (un único lienzo montado por documento, igual que
 *  slidesCommandHandler); si hicieran falta varios, un store con subscripción. */
export function getSlidesSelection(): SlidesSelectionSnapshot {
  return slidesSelSnapshot;
}
/** La publica el lienzo desde sus efectos, nunca desde el render. */
function publicarSeleccion(s: SlidesSelectionSnapshot): void {
  slidesSelSnapshot = s;
}

/** Último tamaño del lienzo publicado por el SlidesCanvas montado (para el
 *  control «Tamaño de diapositiva» del ribbon, que no ve el modelo). ponytail:
 *  variable de módulo como slidesSelSnapshot (un lienzo montado por documento). */
let slidesSizeSnapshot: SlideSize | undefined;
export function getSlidesSize(): SlideSize | undefined {
  return slidesSizeSnapshot;
}

/** Los diseños vigentes (id y nombre) para el selector «Disposición» del ribbon, que no ve
 *  el modelo. Vacío si no hay lienzo montado. ponytail: variable de módulo como las otras. */
let slidesLayoutsSnapshot: readonly { id: string; name: string }[] = [];
const slidesLayoutsListeners = new Set<() => void>();
export function getSlidesLayouts(): readonly { id: string; name: string }[] {
  return slidesLayoutsSnapshot;
}
/** Avisa cuando cambian los diseños (el ribbon se repinta con la lista nueva). */
export function subscribeSlidesLayouts(fn: () => void): () => void {
  slidesLayoutsListeners.add(fn);
  return () => {
    slidesLayoutsListeners.delete(fn);
  };
}
function publicarDisenos(layouts: readonly { id: string; name: string }[]): void {
  slidesLayoutsSnapshot = layouts;
  slidesLayoutsListeners.forEach((fn) => fn());
}

/** Las tres plantillas de disposición (fuente única para la pestaña Inicio). */
export const SLIDE_LAYOUTS: { id: SlideLayoutId; labelKey: DocsKey }[] = [
  { id: 'titulo', labelKey: 'slides.layout.title' },
  { id: 'titulo-cuerpo', labelKey: 'slides.layout.titleBody' },
  { id: 'dos-columnas', labelKey: 'slides.layout.twoColumns' },
];

/** Las formas (fuente única para Insertar; el id es también el nombre del icono).
 *  Núcleo + ampliación (fase Slides-Conectores + más formas). */
export const SLIDE_SHAPES: readonly { id: SlideShape; labelKey: DocsKey }[] = [
  { id: 'rect', labelKey: 'slides.shape.rect' },
  { id: 'ellipse', labelKey: 'slides.shape.ellipse' },
  { id: 'line', labelKey: 'slides.shape.line' },
  { id: 'arrow', labelKey: 'slides.shape.arrow' },
  { id: 'triangle', labelKey: 'slides.shape.triangle' },
  { id: 'diamond', labelKey: 'slides.shape.diamond' },
  { id: 'pentagon', labelKey: 'slides.shape.pentagon' },
  { id: 'hexagon', labelKey: 'slides.shape.hexagon' },
  { id: 'star', labelKey: 'slides.shape.star' },
  { id: 'cylinder', labelKey: 'slides.shape.cylinder' },
  { id: 'callout', labelKey: 'slides.shape.callout' },
];

/** Los tres tipos de conector (fuente única para Insertar y la mini-barra). */
export const CONNECTOR_KINDS_UI: readonly {
  kind: ConnectorKind;
  labelKey: DocsKey;
  icon: EditorIconName;
}[] = [
  { kind: 'straight', labelKey: 'slides.connector.straight', icon: 'connStraight' },
  { kind: 'elbow', labelKey: 'slides.connector.elbow', icon: 'connElbow' },
  { kind: 'curved', labelKey: 'slides.connector.curved', icon: 'connCurved' },
];
const CONNECTOR_KIND_SET = new Set<ConnectorKind>(['straight', 'elbow', 'curved']);
const connectorTypeOptions = (t: DocsT): { value: string; label: string }[] =>
  CONNECTOR_KINDS_UI.map((c) => ({ value: c.kind, label: t(c.labelKey) }));

/** Las cuatro disposiciones de SmartArt (fuente única para Insertar y la
 *  mini-barra: id + rótulo + icono). */
export const SMARTART_LAYOUTS: readonly {
  id: SmartArtLayout;
  labelKey: DocsKey;
  icon: EditorIconName;
}[] = [
  { id: 'list', labelKey: 'smartart.list', icon: 'smartList' },
  { id: 'process', labelKey: 'smartart.process', icon: 'smartProcess' },
  { id: 'cycle', labelKey: 'smartart.cycle', icon: 'smartCycle' },
  { id: 'hierarchy', labelKey: 'smartart.hierarchy', icon: 'smartHierarchy' },
];
const SMARTART_IDS = new Set<SmartArtLayout>(['list', 'process', 'cycle', 'hierarchy']);
/** Opciones del select de disposición del diagrama (mini-barra). */
const smartArtLayoutOptions = (t: DocsT): { value: string; label: string }[] =>
  SMARTART_LAYOUTS.map((l) => ({ value: l.id, label: t(l.labelKey) }));
/** Elementos de arranque de un diagrama nuevo, por disposición (la jerarquía
 *  nace con una raíz y dos hijos para que se vea el árbol de inmediato). Texto de
 *  ejemplo en el idioma de la sesión: se inserta para reemplazarlo, pero llegaba
 *  en español a todo el mundo. */
const defaultSmartArtItems = (layout: SmartArtLayout, t: DocsT): SmartArtItem[] => {
  const n = (key: DocsKey, count: number): SmartArtItem[] =>
    Array.from({ length: count }, (_, i) => ({ text: t(key, { n: i + 1 }) }));
  switch (layout) {
    case 'process':
      return n('slides.smartart.sample.step', 3);
    case 'cycle':
      return n('slides.smartart.sample.phase', 4);
    case 'hierarchy':
      return [
        { text: t('slides.smartart.sample.root') },
        { text: t('slides.smartart.sample.team', { n: 1 }), level: 1 },
        { text: t('slides.smartart.sample.team', { n: 2 }), level: 1 },
      ];
    default:
      return n('slides.smartart.sample.item', 3);
  }
};

/** Transiciones de entrada (select de la mini-barra; ausente en el modelo = 'none'). */
const TRANSITION_OPTIONS: { value: SlideTransition; labelKey: DocsKey }[] = [
  { value: 'none', labelKey: 'slides.trans.none' },
  { value: 'fade', labelKey: 'slides.trans.fade' },
  { value: 'slide', labelKey: 'slides.trans.slide' },
  { value: 'morph', labelKey: 'slides.trans.morph' },
];

/* ---------- Animaciones de caja (fase Slides-Animación avanzada) ----------
 * Rótulos por efecto/categoría/disparador para la UI. La categoría de un efecto
 * se deriva de ANIM_CATEGORY (slides/anim.ts): fuente única, sin desincronía. */

const ANIM_EFFECT_LABEL: Record<SlideAnimType, DocsKey> = {
  appear: 'slides.effect.appear',
  fade: 'slides.effect.fade',
  fly: 'slides.effect.fly',
  zoom: 'slides.effect.zoom',
  pulse: 'slides.effect.pulse',
  spin: 'slides.effect.spin',
  'grow-shrink': 'slides.effect.growShrink',
  disappear: 'slides.effect.disappear',
  'fade-out': 'slides.effect.fadeOut',
  'fly-out': 'slides.effect.flyOut',
  'zoom-out': 'slides.effect.zoomOut',
  'motion-right': 'slides.effect.motionRight',
  'motion-left': 'slides.effect.motionLeft',
  'motion-up': 'slides.effect.motionUp',
  'motion-down': 'slides.effect.motionDown',
  'motion-up-right': 'slides.effect.motionUpRight',
  'motion-up-left': 'slides.effect.motionUpLeft',
  'motion-down-right': 'slides.effect.motionDownRight',
  'motion-down-left': 'slides.effect.motionDownLeft',
  'motion-loop': 'slides.effect.motionLoop',
  'motion-arc': 'slides.effect.motionArc',
  'motion-custom': 'slides.effect.motionCustom',
};

/** Rótulo de cada categoría; el select de categorías se DERIVA de este mapa (era
 *  una segunda lista con los mismos cuatro nombres escritos otra vez). */
const ANIM_CATEGORY_LABEL: Record<SlideAnimCategory, DocsKey> = {
  entrance: 'slides.category.entrance',
  emphasis: 'slides.category.emphasis',
  exit: 'slides.category.exit',
  motion: 'slides.category.motion',
};

const categoryOptions = (t: DocsT): { value: SlideAnimCategory; label: string }[] =>
  (Object.keys(ANIM_CATEGORY_LABEL) as SlideAnimCategory[]).map((value) => ({
    value,
    label: t(ANIM_CATEGORY_LABEL[value]),
  }));

const TRIGGER_OPTIONS: { value: SlideAnimTrigger; labelKey: DocsKey }[] = [
  { value: 'click', labelKey: 'slides.anim.onClickTrigger' },
  { value: 'with', labelKey: 'slides.anim.withPrev' },
  { value: 'after', labelKey: 'slides.anim.afterPrev' },
];

/** Todos los efectos (orden de declaración: entrada, énfasis, salida). */
const ANIM_TYPES = Object.keys(ANIM_CATEGORY) as SlideAnimType[];
/** Opciones del select de efecto de una categoría (derivadas de ANIM_CATEGORY). */
const effectOptionsOf = (cat: SlideAnimCategory, t: DocsT): { value: string; label: string }[] =>
  ANIM_TYPES.filter((k) => ANIM_CATEGORY[k] === cat).map((k) => ({
    value: k,
    label: t(ANIM_EFFECT_LABEL[k]),
  }));

/** Tipos de gráfico (los tres de sheets/Chart.tsx), con los MISMOS rótulos que
 *  el gráfico de la hoja: un gráfico de barras se llama igual en toda la suite. */
const CHART_TYPE_LABEL: Record<SlideChartType, DocsKey> = {
  bar: 'chart.kind.bar',
  line: 'chart.kind.line',
  pie: 'chart.kind.pie',
};
const CHART_TYPES = new Set<SlideChartType>(['bar', 'line', 'pie']);
const chartTypeOptions = (t: DocsT): { value: string; label: string }[] =>
  [...CHART_TYPES].map((k) => ({ value: k, label: t(CHART_TYPE_LABEL[k]) }));

/** Datos de arranque de un gráfico nuevo: 1.ª fila cabecera, 1.ª columna rótulos. */
const DEFAULT_CHART_GRID: string[][] = [
  ['', 'Serie 1'],
  ['Ene', '10'],
  ['Feb', '15'],
  ['Mar', '7'],
  ['Abr', '12'],
];

/* Colores del gráfico: mismo subconjunto AA de CONTENT_PALETTE que sheets/Chart.tsx
 * (≥3:1 sobre blanco, orden fijo), replicado aquí porque sheets no lo exporta. */
const CHART_SERIES: readonly string[] = [
  CONTENT_PALETTE[8], // azul
  CONTENT_PALETTE[5], // rojo
  CONTENT_PALETTE[7], // verde
  CONTENT_PALETTE[9], // violeta
  CONTENT_PALETTE[6], // ámbar
  CONTENT_PALETTE[10], // rosa
  CONTENT_PALETTE[11], // verde azulado
];
const CHART_MUTED = CONTENT_PALETTE[1];
const CHART_AXIS = CONTENT_PALETTE[2];
const CHART_GRID = HIGHLIGHT_PALETTE[6]; // #E2E8F0 hairlines
const CHART_NUM = new Intl.NumberFormat('es-ES', { maximumFractionDigits: 2 });

/* ---------- Constantes y utilidades ---------- */

/** Escenario base: ancho INTERNO fijo (las cajas van en % → solo la proporción
 *  cambia el alto). STAGE_H es el alto por defecto (16:9); el efectivo lo da
 *  stageHeight(model.size). */
const STAGE_W = 960;
const STAGE_H = 540;
/** Ancho de la miniatura del carril (§7.2: caja 128×72 en 16:9). */
const THUMB_W = 128;
/** Aire alrededor del escenario al auto-encajar. */
const FIT_GAP = 64;

/* ---------- Tamaño del lienzo (fase Slides-Tamaño) ----------
 * El lienzo era 16:9 fijo (960×540). Ahora el modelo puede fijar su PROPORCIÓN
 * (16:9, 4:3 o personalizada); como las cajas van en % del lienzo, solo importa
 * la proporción: el ancho interno se mantiene en STAGE_W y la ALTURA se deriva
 * (stageHeight). El escenario se escala SIEMPRE de forma uniforme, así que nada
 * se deforma; al cambiar de proporción las cajas conservan su % (una a ancho
 * completo lo sigue estando, una centrada sigue centrada) y el contenido REFLUYE
 * al nuevo alto (4:3 = más alto) en vez de estirarse. Helpers puros y exportados
 * para el self-test (bun test). */

/** Proporción (ancho/alto) de los presets con proporción fija. */
const PRESET_ASPECT: Record<'16:9' | '4:3', number> = { '16:9': 16 / 9, '4:3': 4 / 3 };
/** Topes de proporción de un tamaño personalizado (evita lienzos degenerados: un
 *  panorámico extremo o una tira vertical siguen siendo usables y escalables). */
const MIN_ASPECT = 0.2;
const MAX_ASPECT = 5;

/** Sanea el tamaño del lienzo (JSON ya parseado, frontera de confianza): preset
 *  válido y, en 'custom', ancho/alto finitos > 0 con unidad px/cm. Devuelve
 *  `undefined` para 16:9 y para lo inválido (por defecto 16:9; JSON limpio: «sin
 *  tamaño = 16:9»). Pura y exportada para bun test. */
export function sanitizeSlideSize(raw: unknown): SlideSize | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const o = raw as Record<string, unknown>;
  if (o['preset'] === '4:3') return { preset: '4:3' };
  if (o['preset'] === 'custom') {
    const num = (v: unknown): number | null =>
      typeof v === 'number' && Number.isFinite(v) && v > 0 ? v : null;
    const w = num(o['w']);
    const h = num(o['h']);
    if (w === null || h === null) return undefined; // custom sin dimensiones = 16:9
    return { preset: 'custom', w, h, unit: o['unit'] === 'cm' ? 'cm' : 'px' };
  }
  return undefined; // '16:9' o desconocido = por defecto (sin campo)
}

/** Proporción efectiva (ancho/alto) del lienzo, topada a un rango sano. Ausente
 *  = 16:9. Pura y exportada para bun test. */
export function slideAspect(size: SlideSize | undefined): number {
  if (size?.preset === '4:3') return PRESET_ASPECT['4:3'];
  if (size?.preset === 'custom' && size.w && size.h)
    return clamp(size.w / size.h, MIN_ASPECT, MAX_ASPECT);
  return PRESET_ASPECT['16:9'];
}

/** Alto efectivo del escenario en px del sistema interno (ancho fijo STAGE_W):
 *  STAGE_W / proporción. 16:9 → 540, 4:3 → 720. Pura y exportada para bun test. */
export function stageHeight(size: SlideSize | undefined): number {
  return STAGE_W / slideAspect(size);
}

/** Acento de edición del lienzo: latón de suite (como Docs/Datos). El color de
 *  producto (ocre) se reserva al tile del launcher, no al cromo de edición. */
const ACCENT = 'var(--editor-select, var(--sytue-primary-500))';

/** Techo del modelo serializado ANTES de aceptar una imagen: el op-body del
 * colab Go corta en 1 MiB (client.go, maxMessageBytes) y el mensaje que lo
 * envuelve añade poco más; por encima, el autosave se pierde entero. */
const MODEL_MAX_CHARS = 900_000;

/** Relleno/borde por defecto de las formas (slate y tinta de CONTENT_PALETTE:
 * sobrios y con contraste AA sobre el folio blanco). */
const SHAPE_FILL = CONTENT_PALETTE[1];
const SHAPE_STROKE = CONTENT_PALETTE[0];
/** Paleta de relleno: la de contenido con «sin relleno» en vez de blanco puro. */
const SHAPE_FILL_PALETTE = CONTENT_PALETTE.map((c) => (c === '#FFFFFF' ? 'transparent' : c));
/** Tinta del contenido sobre papel blanco (negro de marca de CONTENT_PALETTE, §5). */
const INK = CONTENT_PALETTE[0];

/* ---------- Temas de presentación (fase Slides-Temas) ----------
 * Set corto de presets SÓLIDOS (fondo + tinta + fuente), anti-slop: sin
 * degradados; hex reutilizados de CONTENT/HIGHLIGHT_PALETTE (§5), sin hex
 * nuevos. Ausente = Papel (fondo blanco, tinta negra, fuente de la app). */

/** Fondo por defecto (sin tema): el papel blanco del editor (token, cae a white). */
const DEFAULT_BG = 'var(--editor-page-bg, white)';

export interface SlideVisuals {
  /** Color de fondo resuelto (CSS). */
  bg: string;
  /** Color de tinta/texto resuelto (CSS). */
  ink: string;
  /** Familia tipográfica del tema; undefined = la de la app (--font-sans). */
  font?: string;
}

export const SLIDE_THEMES: {
  id: SlideThemeId;
  labelKey: DocsKey;
  bg: string;
  ink: string;
  font?: string;
}[] = [
  {
    id: 'carbon',
    labelKey: 'slides.theme.charcoal',
    bg: CONTENT_PALETTE[0],
    ink: CONTENT_PALETTE[3],
  },
  {
    id: 'pizarra',
    labelKey: 'slides.theme.slate',
    bg: CONTENT_PALETTE[1],
    ink: CONTENT_PALETTE[3],
  },
  {
    id: 'arena',
    labelKey: 'slides.theme.sand',
    bg: HIGHLIGHT_PALETTE[1],
    ink: CONTENT_PALETTE[0],
    font: 'Georgia',
  },
  { id: 'menta', labelKey: 'slides.theme.mint', bg: HIGHLIGHT_PALETTE[2], ink: CONTENT_PALETTE[0] },
  { id: 'cielo', labelKey: 'slides.theme.sky', bg: HIGHLIGHT_PALETTE[3], ink: CONTENT_PALETTE[0] },
];

/** Fondo/tinta/fuente efectivos de una diapositiva: el fondo propio manda sobre
 *  el del tema; tinta y fuente vienen del tema (o de la app). Pura y exportada
 *  para el self-test (bun test). */
export function slideVisuals(model: SlidesModel, slide: Slide): SlideVisuals {
  const theme: { bg: string; ink: string; font?: string } | undefined =
    SLIDE_THEMES.find((t) => t.id === model.theme) ??
    sanitizeCustomThemes(model.themes).find((t) => t.id === model.theme);
  const bg = slide.bg && slide.bg !== 'transparent' ? slide.bg : (theme?.bg ?? DEFAULT_BG);
  const ink = theme?.ink ?? INK;
  // font solo si el tema la fija (no emitir `undefined` explícito).
  return theme?.font ? { bg, ink, font: theme.font } : { bg, ink };
}

/** Style con el fondo/tinta del tema y, si lo hay, la fuente (override local de
 *  --font-sans que .sytue-slidebox lee, sin tocar el token global). */
function visualsStyle(v: SlideVisuals): CSSProperties {
  return v.font
    ? { background: v.bg, color: v.ink, ['--font-sans' as string]: v.font }
    : { background: v.bg, color: v.ink };
}

/** Valor de la opción «Personalizar tema…» del select de tema: no es un tema, abre el panel. */
const CUSTOM_THEME_VALUE = '__custom__';

/** Opciones del select de tema (1.ª = Papel/predeterminado, value ''). */
const THEME_OPTIONS: { value: string; labelKey: DocsKey }[] = [
  { value: '', labelKey: 'slides.theme.paper' },
  ...SLIDE_THEMES.map((tema) => ({ value: tema.id, labelKey: tema.labelKey })),
];

/** Paleta del fondo POR diapositiva: 'transparent' = usar el del tema, más los
 *  sólidos de los temas (sin hex nuevos). */
const SLIDE_BG_PALETTE: readonly string[] = [
  'transparent',
  CONTENT_PALETTE[3], // blanco
  CONTENT_PALETTE[0], // carbón
  CONTENT_PALETTE[1], // pizarra
  HIGHLIGHT_PALETTE[1], // arena
  HIGHLIGHT_PALETTE[2], // menta
  HIGHLIGHT_PALETTE[3], // cielo
];

// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/* Tipografía del CONTENIDO de las cajas (los tamaños van en px del escenario
 * 960×540 y escalan con él). Sin hex: tokens y color-mix sobre tokens. */
const BOX_CSS = `
.sytue-slidebox { font-family: var(--font-sans, Inter, sans-serif); line-height: 1.25; word-wrap: break-word; caret-color: ${ACCENT}; }
.sytue-slidebox ::selection { background: color-mix(in srgb, ${ACCENT} 25%, transparent); }
.sytue-slidebox h1 { font-size: 44px; font-weight: 700; margin: 0; }
.sytue-slidebox h2 { font-size: 30px; font-weight: 600; margin: 0; }
.sytue-slidebox h3 { font-size: 24px; font-weight: 600; margin: 0; }
.sytue-slidebox p { font-size: 20px; margin: 0 0 0.3em; }
.sytue-slidebox ul, .sytue-slidebox ol { font-size: 20px; margin: 0 0 0.3em; padding-left: 1.2em; }

/* Tiradores de mover/redimensionar. Sin touch-action:none el navegador se queda
 * el gesto del dedo como scroll del lienzo y el pointermove no llega nunca: con
 * ratón se arrastraba y con el dedo no. En pantalla táctil el ::after agranda
 * SOLO la zona de toque (10 px no los acierta nadie). */
.sytue-slide-handle { touch-action: none; }
@media (pointer: coarse) {
  .sytue-slide-handle::after { content: ''; position: absolute; inset: -16px; }
}

/* Transiciones del modo presentación (por diapositiva) y animaciones de ENTRADA
 * de las cajas al revelarlas. Solo si el usuario no pide movimiento reducido;
 * con reduced-motion el cambio es instantáneo (la caja aparece ya visible: sin
 * regla de animación, no queda atrapada en el fotograma inicial). */
@media (prefers-reduced-motion: no-preference) {
  .sytue-present-fade { animation: sytue-present-fade 300ms ease-out both; }
  .sytue-present-slide-fwd { animation: sytue-present-slide-fwd 300ms ease-out both; }
  .sytue-present-slide-back { animation: sytue-present-slide-back 300ms ease-out both; }
  /* Entrada. */
  .sytue-anim-fade { animation: sytue-anim-fade 320ms ease-out both; }
  .sytue-anim-fly { animation: sytue-anim-fly 380ms cubic-bezier(0.22, 1, 0.36, 1) both; }
  .sytue-anim-zoom { animation: sytue-anim-zoom 300ms ease-out both; }
  /* Énfasis (vuelven al reposo: fill both es seguro). */
  .sytue-anim-pulse { animation: sytue-anim-pulse 620ms ease-in-out both; }
  .sytue-anim-spin { animation: sytue-anim-spin 700ms ease-in-out both; }
  .sytue-anim-grow-shrink { animation: sytue-anim-grow-shrink 560ms ease-in-out both; }
  /* Salida (terminan ocultas: fill both mantiene el fotograma final). */
  .sytue-anim-fade-out { animation: sytue-anim-fade-out 300ms ease-in both; }
  .sytue-anim-fly-out { animation: sytue-anim-fly-out 380ms cubic-bezier(0.4, 0, 1, 0.6) both; }
  .sytue-anim-zoom-out { animation: sytue-anim-zoom-out 300ms ease-in both; }
  /* Morph: FLIP desde la caja emparejada de la diapositiva anterior (variables
   * --mdx/--mdy/--msx/--msy en línea; transform-origin en la esquina superior izq.). */
  .sytue-morph { animation: sytue-morph 520ms cubic-bezier(0.4, 0, 0.2, 1) both; }
  /* Trayectoria de movimiento: la caja recorre su offset-path (autoría en línea,
   * en px) de 0 % a 100 %. offset-rotate:0 evita el giro automático; con reduced
   * motion no hay regla y la caja salta directa a offset-distance:100 % (el final). */
  .sytue-motion { animation: sytue-motion 640ms cubic-bezier(0.4, 0, 0.2, 1) both; }
}
@keyframes sytue-present-fade { from { opacity: 0; } }
@keyframes sytue-present-slide-fwd { from { opacity: 0; transform: translateX(4%); } }
@keyframes sytue-present-slide-back { from { opacity: 0; transform: translateX(-4%); } }
@keyframes sytue-anim-fade { from { opacity: 0; } }
@keyframes sytue-anim-fly { from { opacity: 0; transform: translateY(26px); } }
@keyframes sytue-anim-zoom { from { opacity: 0; transform: scale(0.82); } }
@keyframes sytue-anim-pulse { 0%, 100% { transform: scale(1); } 50% { transform: scale(1.06); } }
@keyframes sytue-anim-spin { from { transform: rotate(0); } to { transform: rotate(360deg); } }
@keyframes sytue-anim-grow-shrink { 0%, 100% { transform: scale(1); } 50% { transform: scale(1.22); } }
@keyframes sytue-anim-fade-out { to { opacity: 0; } }
@keyframes sytue-anim-fly-out { to { opacity: 0; transform: translateY(26px); } }
@keyframes sytue-anim-zoom-out { to { opacity: 0; transform: scale(0.82); } }
@keyframes sytue-morph { from { transform: translate(var(--mdx, 0), var(--mdy, 0)) scale(var(--msx, 1), var(--msy, 1)); } }
/* La caja declara offset-distance:100 % en línea (su reposo = final de la
 * trayectoria); el keyframe solo la arranca en 0 %. */
@keyframes sytue-motion { from { offset-distance: 0%; } }
`;

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n));
}

function newBox(x: number, y: number, w: number, h: number, html: string): SlideBox {
  return { id: crypto.randomUUID(), x, y, w, h, html };
}

const titleHtml = (t: DocsT): string => `<h1>${t('slides.placeholder.title')}</h1>`;
const bodyHtml = (t: DocsT): string => `<p>${t('slides.placeholder.body')}</p>`;

function layoutBoxes(layout: SlideLayoutId, t: DocsT): SlideBox[] {
  switch (layout) {
    case 'titulo':
      return [newBox(10, 35, 80, 20, titleHtml(t))]; // igual que el modelo vacío (§7.2)
    case 'titulo-cuerpo':
      return [newBox(8, 8, 84, 16, titleHtml(t)), newBox(8, 30, 84, 60, bodyHtml(t))];
    case 'dos-columnas':
      return [
        newBox(8, 8, 84, 16, titleHtml(t)),
        newBox(8, 30, 40, 60, bodyHtml(t)),
        newBox(52, 30, 40, 60, bodyHtml(t)),
      ];
  }
}

/** Los tres diseños de serie del editor, con su nombre en el idioma de la sesión. */
function builtinLayouts(t: DocsT): SlideLayout[] {
  return SLIDE_LAYOUTS.map((l) => ({ id: l.id, name: t(l.labelKey), boxes: layoutBoxes(l.id, t) }));
}

/** Diapositiva nueva con un diseño. Con `layouts` (los del modelo) la diapositiva guarda el
 *  id del diseño; sin ellos, es la del modelo vacío y no lleva ninguno. */
function newSlide(layout: string, t: DocsT, layouts?: readonly SlideLayout[]): Slide {
  const def = (layouts ?? builtinLayouts(t)).find((l) => l.id === layout);
  const boxes = def ? boxesFromLayout(def, () => crypto.randomUUID()) : [];
  return { id: crypto.randomUUID(), boxes, ...(layouts && def ? { layout: def.id } : {}) };
}

function emptySlidesModel(t: DocsT): SlidesModel {
  return { slides: [newSlide('titulo', t)] };
}

/** Reflujo de plantilla: el TEXTO entra por orden en las cajas de la plantilla;
 * imágenes y formas se conservan tal cual (cambiar la disposición no puede
 * destruirlas). Pura y exportada para el self-test (bun test). */
export function mergeLayoutBoxes(layout: SlideLayoutId, boxes: SlideBox[], t: DocsT): SlideBox[] {
  // Todo lo que no es texto (imagen, forma, gráfico, smartart) sobrevive intacto.
  const def = builtinLayouts(t).find((l) => l.id === layout);
  return def ? mergeIntoLayout(def, boxes, () => crypto.randomUUID()) : boxes;
}

/** Encuadre inicial de una imagen en % del escenario: 40 % de ancho (70 % de
 * alto máx.), centrado y con el aspecto conservado (el % vertical corrige la
 * proporción del lienzo, `sh` = stageHeight). Pura y exportada para el self-test. */
export function imageBoxRect(
  nw: number,
  nh: number,
  sh: number = STAGE_H,
): Pick<SlideBox, 'x' | 'y' | 'w' | 'h'> {
  let w = 40;
  let h = (w * (nh / Math.max(1, nw)) * STAGE_W) / sh;
  if (h > 70) {
    w = (w * 70) / h;
    h = 70;
  }
  w = Math.max(4, w);
  h = Math.max(4, h);
  return { x: (100 - w) / 2, y: (100 - h) / 2, w, h };
}

/* ---------- Medios incrustados (audio/vídeo) ----------
 * Detección de tipo por MIME/extensión y saneado del origen (frontera de
 * confianza: la URL la teclea el usuario). Puro + test (slides-media.test.ts). */

const VIDEO_EXT = new Set(['mp4', 'webm', 'ogv', 'ogm', 'mov', 'm4v', 'mkv']);
const AUDIO_EXT = new Set(['mp3', 'wav', 'ogg', 'oga', 'm4a', 'aac', 'flac', 'opus', 'weba']);

/** Tipo de medio a partir de un MIME («video/mp4») o de un nombre/URL por su
 *  extensión. `null` si no se reconoce. Pura y exportada para bun test. */
export function mediaKindFor(mimeOrName: string): 'video' | 'audio' | null {
  const s = mimeOrName.trim().toLowerCase();
  if (s === '') return null;
  if (s.startsWith('video/')) return 'video';
  if (s.startsWith('audio/')) return 'audio';
  // Extensión: lo que sigue al último «.» tras recortar query/hash.
  const path = s.split(/[?#]/)[0] ?? '';
  const ext = path.slice(path.lastIndexOf('.') + 1);
  if (VIDEO_EXT.has(ext)) return 'video';
  if (AUDIO_EXT.has(ext)) return 'audio';
  return null;
}

/** Sanea el origen de un medio (o de un póster): admite un dataURL del tipo
 *  esperado y http(s); un dominio sin esquema se normaliza a https; cualquier
 *  otro esquema (javascript:, blob:, file:…) se rechaza. `null` = descartar.
 *  Frontera de confianza (URL tecleada). Pura y exportada para bun test. */
export function safeMediaSrc(
  raw: string,
  dataPrefix: RegExp = /^data:(?:audio|video)\//i,
): string | null {
  const s = raw.trim();
  if (s === '') return null;
  if (dataPrefix.test(s)) return s;
  if (/^https?:\/\//i.test(s)) return s;
  if (/^[a-z][a-z0-9+.-]*:/i.test(s)) return null; // otro esquema explícito → fuera
  return `https://${s}`;
}

/** Descriptor de medio ya validado (lo que persiste el comando 'media'). */
export interface MediaSpec {
  mediaType: 'video' | 'audio';
  src: string;
  poster?: string;
  controls: boolean;
  autoplay: boolean;
  loop: boolean;
  /** Subtítulos WebVTT ya saneados (slides/subtitulos.ts); ausente = sin ellos. */
  tracks?: SlideTrack[];
}

/** Valida/normaliza el descriptor del comando 'media' (JSON ya parseado): tipo
 *  válido y origen seguro son obligatorios; el póster (imagen) y las banderas son
 *  opcionales. `null` = no insertar. Pura y exportada para bun test. */
export function sanitizeMediaSpec(raw: unknown): MediaSpec | null {
  if (!raw || typeof raw !== 'object') return null;
  const o = raw as Record<string, unknown>;
  const mediaType =
    o['mediaType'] === 'audio' ? 'audio' : o['mediaType'] === 'video' ? 'video' : null;
  if (!mediaType) return null;
  const src = typeof o['src'] === 'string' ? safeMediaSrc(o['src']) : null;
  if (!src) return null;
  const poster =
    typeof o['poster'] === 'string' ? safeMediaSrc(o['poster'], /^data:image\//i) : null;
  const spec: MediaSpec = {
    mediaType,
    src,
    controls: o['controls'] !== false, // ausente = con controles
    autoplay: o['autoplay'] === true,
    loop: o['loop'] === true,
  };
  const tracks = sanitizeTracks(o['tracks']);
  return { ...spec, ...(poster ? { poster } : {}), ...(tracks.length > 0 ? { tracks } : {}) };
}

/** Destino de una tecla del modo presentación: índice destino (con topes),
 * 'exit' o null si la tecla no navega. Pura y exportada para bun test. */
export function presentKeyTarget(key: string, idx: number, last: number): number | 'exit' | null {
  switch (key) {
    case 'ArrowRight':
    case ' ':
    case 'PageDown':
      return Math.min(last, idx + 1);
    case 'ArrowLeft':
    case 'PageUp':
      return Math.max(0, idx - 1);
    case 'Home':
      return 0;
    case 'End':
      return last;
    case 'Escape':
      return 'exit';
    default:
      return null;
  }
}

/** Duración transcurrida (ms) → «M:SS» o «H:MM:SS» para el cronómetro del
 *  moderador. Negativa → 0:00. Pura y exportada para bun test. */
export function fmtElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const s = String(total % 60).padStart(2, '0');
  const m = Math.floor(total / 60) % 60;
  const h = Math.floor(total / 3600);
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${s}` : `${m}:${s}`;
}

/** Índice destino (0-based, con topes) de un enlace de acción de caja al hacer
 *  clic en presentación; `null` si el destino no es válido. `idx`/`last` son la
 *  diapositiva actual y la última (0-based). Pura y exportada para bun test. */
export function actionTarget(action: SlideLinkTarget, idx: number, last: number): number | null {
  if (action === 'first') return 0;
  if (action === 'last') return last;
  if (action === 'next') return Math.min(last, idx + 1);
  if (action === 'prev') return Math.max(0, idx - 1);
  // Número 1-based de diapositiva: fuera de rango = sin destino.
  return typeof action === 'number' && action >= 1 && action <= last + 1 ? action - 1 : null;
}

/** Etiqueta legible del destino de un enlace de acción (tooltip/aria). */
function actionLabel(t: DocsT, action: SlideLinkTarget): string {
  switch (action) {
    case 'first':
      return t('slides.link.goFirst');
    case 'last':
      return t('slides.link.goLast');
    case 'next':
      return t('slides.link.goNext');
    case 'prev':
      return t('slides.link.goPrev');
    default:
      return t('slides.link.goTo', { n: action });
  }
}

/** Mueve la diapositiva `from` a la posición `to` sin mutar (misma identidad
 * de objetos: la activa se sigue con findIndex). Con `from` fuera de rango
 * devuelve el array original. Pura y exportada para bun test. */
export function reorderSlides(slides: Slide[], from: number, to: number): Slide[] {
  const out = [...slides];
  const [moved] = out.splice(from, 1);
  if (!moved) return slides;
  out.splice(to, 0, moved);
  return out;
}

/* ---------- Patrón de diapositiva (slide master) ----------
 * Un `master` opcional en el modelo aporta cajas que se pintan DETRÁS de las de
 * cada diapositiva (logo, pie, número de página). Sin master = comportamiento
 * clásico. Las cajas de texto del patrón admiten los campos {n} (número de
 * diapositiva, 1-based) y {total} (total), sustituidos por diapositiva al pintar;
 * en el modo «Editar patrón» se muestran literales (es lo que se edita). Helpers
 * puros y exportados para el self-test (bun test). */

const FIELD_N = /\{n\}/g;
const FIELD_TOTAL = /\{total\}/g;

/** Sustituye los campos {n}/{total} del HTML de una caja del patrón por el número
 *  (1-based) y el total de diapositivas. Pura y exportada para bun test. */
export function substituteMasterFields(html: string, n: number, total: number): string {
  return html.replace(FIELD_TOTAL, String(total)).replace(FIELD_N, String(n));
}

/** El diseño de la diapositiva `index`, si lo lleva y existe. */
function layoutOfSlide(model: SlidesModel, index: number): SlideLayout | undefined {
  const id = model.slides[index]?.layout;
  return id ? model.master?.layouts?.find((l) => l.id === id) : undefined;
}

/** Cajas del patrón resueltas para la diapositiva `index` (0-based), con {n}/{total} ya
 *  sustituidos: las del patrón (salvo que su diseño lo oculte) y, encima, las formas de su
 *  diseño. Vacío si la diapositiva lo oculta (hideMaster), que oculta las dos cosas, como
 *  PowerPoint. Pura y exportada para bun test. */
export function masterBoxesFor(model: SlidesModel, index: number): SlideBox[] {
  const slide = model.slides[index];
  if (!slide || slide.hideMaster) return [];
  const layout = layoutOfSlide(model, index);
  const boxes = [
    ...(layout?.hideMaster ? [] : (model.master?.boxes ?? [])),
    ...(layout?.decor ?? []),
  ];
  const n = index + 1;
  const total = model.slides.length;
  return boxes.map((b) =>
    !b.kind || b.kind === 'text' ? { ...b, html: substituteMasterFields(b.html, n, total) } : b,
  );
}

/** Las líneas de las formas del diseño de la diapositiva `index` (extremos sueltos), que se
 *  pintan con las cajas del patrón, debajo de las de la diapositiva. Pura. */
export function layoutConnectorsFor(model: SlidesModel, index: number): SlideConnector[] {
  const slide = model.slides[index];
  if (!slide || slide.hideMaster) return [];
  return layoutOfSlide(model, index)?.connectors ?? [];
}

/** Cajas EFECTIVAS de una diapositiva = patrón (detrás) + propias (delante). Es lo
 *  que pintan miniaturas, presentación y vista del moderador. Fuera de rango =
 *  vacío. Pura y exportada para bun test. */
export function resolveSlideBoxes(model: SlidesModel, index: number): SlideBox[] {
  const slide = model.slides[index];
  if (!slide) return [];
  return [...masterBoxesFor(model, index), ...slide.boxes];
}

/* ---------- Animaciones de caja (fase Slides-Animación avanzada) ----------
 * La lógica pura (secuencia por disparador, pasos, presencia/disparo, orden y
 * emparejado morph) vive en slides/anim.ts y se testea con bun. Aquí solo se
 * consume para el render y los comandos de edición. */

/* ---------- Gráfico de diapositiva (fase Slides-Gráfico) ----------
 * Reutiliza extractChartData/niceTicks de sheets/Chart.tsx (interpretación de la
 * rejilla + escala «bonita» del eje) y dibuja marcas que LLENAN la caja. */

/** Texto crudo de una celda → número (es-ES: coma decimal) o texto. Puro. */
export function parseChartCell(raw: string): number | string {
  const t = raw.trim();
  if (t === '' || /[a-zA-Z]/.test(t)) return t; // vacío o con letras: texto tal cual
  // Con coma es formato es-ES (coma decimal, punto de millar); si no, punto decimal.
  const s = (t.includes(',') ? t.replace(/\./g, '').replace(',', '.') : t).replace('%', '');
  const n = Number(s);
  return Number.isFinite(n) ? n : t;
}

/** Interpreta la rejilla del gráfico igual que un rango de sheets. Pura + test. */
export function slideChartData(chart: SlideChart): ChartData {
  const rows = chart.grid.length;
  const cols = chart.grid.reduce((m, r) => Math.max(m, r.length), 0);
  if (rows === 0 || cols === 0) return { labels: [], series: [] };
  return extractChartData({ c1: 0, r1: 0, c2: cols - 1, r2: rows - 1 }, (c, r) =>
    parseChartCell(chart.grid[r]?.[c] ?? ''),
  );
}

/** Rótulo legible de una caja para el panel de animaciones. */
function boxLabel(t: DocsT, b: SlideBox): string {
  if (b.kind === 'image') return t('slides.box.image');
  if (b.kind === 'media')
    return t(b.mediaType === 'audio' ? 'slides.box.audio' : 'slides.box.video');
  if (b.kind === 'chart') return t('slides.box.chart');
  if (b.kind === 'smartart') return t('slides.box.smartart');
  if (b.kind === 'shape') {
    const forma = SLIDE_SHAPES.find((s) => s.id === b.shape);
    return forma ? t(forma.labelKey) : t('slides.box.shape');
  }
  const text = b.html
    .replace(/<[^>]*>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  return text === '' ? t('slides.textBox') : text.length > 30 ? `${text.slice(0, 29)}…` : text;
}

/** Redondeo a centésima de px: paths compactos (igual criterio que sheets/Chart). */
const cpx = (n: number): number => Math.round(n * 100) / 100;

/** Gráfico dibujado para LLENAR la caja: viewBox = px intrínsecos de la caja
 * (mismo aspecto que su tamaño CSS → sin distorsión), marcas barras/líneas/
 * circular sobre los datos de la rejilla. */
function SlideChartView({
  chart,
  vw,
  vh,
}: {
  chart: SlideChart;
  vw: number;
  vh: number;
}): JSX.Element {
  const t = useT();
  const data = slideChartData(chart);
  const pieVals = chart.type === 'pie' ? (data.series[0]?.values ?? []) : [];
  const empty =
    data.labels.length === 0 ||
    data.series.length === 0 ||
    (chart.type === 'pie' && !pieVals.some((v) => v > 0));

  const pad = {
    l: chart.type === 'pie' ? 6 : Math.min(38, vw * 0.16),
    r: 6,
    t: 8,
    b: chart.type === 'pie' ? 6 : Math.min(20, vh * 0.14),
  };
  const plotL = pad.l;
  const plotT = pad.t;
  const plotW = Math.max(1, vw - pad.l - pad.r);
  const plotH = Math.max(1, vh - pad.t - pad.b);
  const font = Math.max(7, Math.min(11, vh * 0.06));

  const label = t('slides.chart.aria', { kind: t(CHART_TYPE_LABEL[chart.type]) });

  if (empty) {
    return (
      <div
        class="pointer-events-none grid h-full w-full place-items-center px-2 text-center"
        style={{ color: CHART_MUTED }}
      >
        <span style={{ fontSize: `${font + 1}px` }}>{t('slides.chart.noData')}</span>
      </div>
    );
  }

  const series = data.series.slice(0, CHART_SERIES.length);

  /* --- Barras y líneas comparten eje Y «bonito» (niceTicks reutilizado). --- */
  const bodyBarLine = (): JSX.Element => {
    const all = series.flatMap((s) => s.values);
    const sc =
      chart.type === 'bar'
        ? niceTicks(Math.min(0, ...all), Math.max(0, ...all))
        : niceTicks(all.length ? Math.min(...all) : 0, all.length ? Math.max(...all) : 1);
    const yOf = (v: number): number => plotT + ((sc.max - v) / (sc.max - sc.min || 1)) * plotH;
    const n = Math.max(1, data.labels.length);
    const slot = plotW / n;
    const xLabelStep = Math.ceil(n / Math.max(1, Math.floor(plotW / (font * 4))));
    return (
      <>
        {/* Retícula + rótulos del eje Y. */}
        {sc.ticks.map((t) => (
          <g key={t}>
            <line
              x1={plotL}
              x2={vw - pad.r}
              y1={cpx(yOf(t))}
              y2={cpx(yOf(t))}
              stroke={CHART_GRID}
              stroke-width={1}
              shape-rendering="crispEdges"
            />
            <text
              x={plotL - 3}
              y={cpx(yOf(t))}
              dy="0.32em"
              text-anchor="end"
              font-size={font}
              fill={CHART_MUTED}
            >
              {CHART_NUM.format(t)}
            </text>
          </g>
        ))}
        {chart.type === 'bar'
          ? (() => {
              const group = slot * 0.72;
              const bw = group / Math.max(1, series.length);
              const y0 = yOf(0);
              return series.flatMap((s, k) =>
                s.values.map((v, i) => {
                  const yv = yOf(v);
                  return (
                    <rect
                      key={`${k}-${i}`}
                      x={cpx(plotL + i * slot + (slot - group) / 2 + k * bw + 1)}
                      y={cpx(Math.min(yv, y0))}
                      width={cpx(Math.max(1, bw - 2))}
                      height={cpx(Math.abs(yv - y0))}
                      fill={CHART_SERIES[k] ?? CHART_AXIS}
                      shape-rendering="crispEdges"
                    >
                      <title>{`${data.labels[i] ?? ''} — ${s.name}: ${CHART_NUM.format(v)}`}</title>
                    </rect>
                  );
                }),
              );
            })()
          : series.map((s, k) => {
              const color = CHART_SERIES[k] ?? CHART_AXIS;
              const xAt = (i: number): number => plotL + slot * (i + 0.5);
              const d = s.values
                .map((v, i) => `${i === 0 ? 'M' : 'L'}${cpx(xAt(i))} ${cpx(yOf(v))}`)
                .join('');
              return (
                <path
                  key={k}
                  d={d}
                  fill="none"
                  stroke={color}
                  stroke-width={2}
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              );
            })}
        {/* Línea base del eje X + rótulos de categoría. */}
        <line
          x1={plotL}
          x2={vw - pad.r}
          y1={cpx(chart.type === 'bar' ? yOf(0) : plotT + plotH)}
          y2={cpx(chart.type === 'bar' ? yOf(0) : plotT + plotH)}
          stroke={CHART_AXIS}
          stroke-width={1}
          shape-rendering="crispEdges"
        />
        {data.labels.map((t, i) =>
          i % xLabelStep === 0 && t !== '' ? (
            <text
              key={i}
              x={cpx(plotL + slot * (i + 0.5))}
              y={vh - pad.b / 2 + font * 0.3}
              text-anchor="middle"
              font-size={font}
              fill={CHART_MUTED}
            >
              {t.length > 8 ? `${t.slice(0, 7)}…` : t}
            </text>
          ) : null,
        )}
      </>
    );
  };

  /* --- Circular: sectores de la 1.ª serie (valores > 0). --- */
  const bodyPie = (): JSX.Element => {
    const parts = data.labels
      .map((name, i) => ({
        name,
        value: Math.max(0, pieVals[i] ?? 0),
        color: CHART_SERIES[i % CHART_SERIES.length]!,
      }))
      .filter((p) => p.value > 0);
    const total = parts.reduce((sum, p) => sum + p.value, 0) || 1;
    const cx = plotL + plotW / 2;
    const cy = plotT + plotH / 2;
    const r = Math.max(1, Math.min(plotW, plotH) / 2 - 2);
    let a = -Math.PI / 2;
    return (
      <>
        {parts.map((p, i) => {
          const frac = p.value / total;
          const a1 = a + frac * 2 * Math.PI;
          const tip = `${p.name}: ${CHART_NUM.format(p.value)} (${CHART_NUM.format(frac * 100)} %)`;
          const el =
            frac > 0.9995 ? (
              <circle
                key={i}
                cx={cpx(cx)}
                cy={cpx(cy)}
                r={cpx(r)}
                fill={p.color}
                stroke="white"
                stroke-width={1.5}
              >
                <title>{tip}</title>
              </circle>
            ) : (
              <path
                key={i}
                d={`M${cpx(cx)} ${cpx(cy)}L${cpx(cx + r * Math.cos(a))} ${cpx(cy + r * Math.sin(a))}A${cpx(r)} ${cpx(r)} 0 ${frac > 0.5 ? 1 : 0} 1 ${cpx(cx + r * Math.cos(a1))} ${cpx(cy + r * Math.sin(a1))}Z`}
                fill={p.color}
                stroke="white"
                stroke-width={1.5}
                stroke-linejoin="round"
              >
                <title>{tip}</title>
              </path>
            );
          a = a1;
          return el;
        })}
      </>
    );
  };

  return (
    <svg
      class="pointer-events-none h-full w-full select-none"
      viewBox={`0 0 ${cpx(vw)} ${cpx(vh)}`}
      role="img"
      aria-label={label}
    >
      {chart.type === 'pie' ? bodyPie() : bodyBarLine()}
    </svg>
  );
}

/* ---------- SmartArt de diapositiva (fase Slides-SmartArt) ----------
 * Dibuja el diagrama LLENANDO la caja (viewBox = px del escenario, sin
 * distorsión). El auto-layout es puro (smartArtLayout); el color sale del TEMA
 * activo vía `currentColor` (= la tinta que hereda la caja), con opacidades
 * sobrias: nodos con relleno tenue y borde, texto a plena tinta. Sin degradados. */

/** Rótulo accesible del diagrama según su disposición (las mismas claves que el
 *  SmartArt del folio: un diagrama se llama igual en toda la suite). */
const SMARTART_ARIA: Record<SmartArtLayout, DocsKey> = {
  list: 'smartart.kind.list',
  process: 'smartart.kind.process',
  cycle: 'smartart.kind.cycle',
  hierarchy: 'smartart.kind.hierarchy',
};

function SmartArtView({
  smart,
  vw,
  vh,
}: {
  smart: SlideSmartArt;
  vw: number;
  vh: number;
}): JSX.Element {
  const t = useT();
  // Saneado tolerante: un modelo viejo/corrupto sin items válidos no revienta.
  const items = Array.isArray(smart.items) ? smart.items : [];
  const layout = SMARTART_IDS.has(smart.layout) ? smart.layout : 'list';
  const geo = smartArtLayout(layout, items, vw, vh);
  const bullet = layout === 'list';

  if (geo.nodes.length === 0) {
    return (
      <div
        class="pointer-events-none grid h-full w-full place-items-center px-2 text-center"
        style={{ color: 'currentColor', opacity: 0.6 }}
      >
        <span style={{ fontSize: `${Math.max(8, Math.min(13, vh * 0.06))}px` }}>
          {t('smartart.empty')}
        </span>
      </div>
    );
  }

  const headSize = clamp(Math.min(vw, vh) * 0.03, 4, 12);

  return (
    <svg
      class="pointer-events-none h-full w-full select-none"
      viewBox={`0 0 ${cpx(vw)} ${cpx(vh)}`}
      role="img"
      aria-label={t('smartart.aria', { kind: t(SMARTART_ARIA[layout]) })}
    >
      {/* Conectores primero (por debajo de los nodos). */}
      {geo.connectors.map((c, i) => {
        if (c.kind === 'elbow') {
          // Codo ortogonal padre→hijo (organigrama): baja, cruza, baja. Sin punta.
          const my = cpx((c.y1 + c.y2) / 2);
          return (
            <path
              key={`c${i}`}
              d={`M${cpx(c.x1)} ${cpx(c.y1)}V${my}H${cpx(c.x2)}V${cpx(c.y2)}`}
              fill="none"
              stroke="currentColor"
              stroke-opacity={0.42}
              stroke-width={1.5}
              stroke-linejoin="round"
            />
          );
        }
        // Flecha recta con punta (proceso/ciclo).
        const ang = Math.atan2(c.y2 - c.y1, c.x2 - c.x1);
        const h1x = cpx(c.x2 - headSize * Math.cos(ang - Math.PI / 7));
        const h1y = cpx(c.y2 - headSize * Math.sin(ang - Math.PI / 7));
        const h2x = cpx(c.x2 - headSize * Math.cos(ang + Math.PI / 7));
        const h2y = cpx(c.y2 - headSize * Math.sin(ang + Math.PI / 7));
        return (
          <g key={`c${i}`} stroke="currentColor" stroke-opacity={0.5} stroke-width={2} fill="none">
            <line
              x1={cpx(c.x1)}
              y1={cpx(c.y1)}
              x2={cpx(c.x2)}
              y2={cpx(c.y2)}
              stroke-linecap="round"
            />
            <path
              d={`M${h1x} ${h1y}L${cpx(c.x2)} ${cpx(c.y2)}L${h2x} ${h2y}`}
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          </g>
        );
      })}

      {/* Nodos: forma sobria (relleno tenue + borde) + texto centrado/legible. */}
      {geo.nodes.map((nd) => {
        const fs = clamp(Math.min(nd.h * 0.32, nd.w * 0.15), 7, 20);
        const text = items[nd.i]?.text ?? '';
        return (
          <g key={`n${nd.i}`}>
            {nd.shape === 'ellipse' ? (
              <ellipse
                cx={cpx(nd.x + nd.w / 2)}
                cy={cpx(nd.y + nd.h / 2)}
                rx={cpx(nd.w / 2)}
                ry={cpx(nd.h / 2)}
                fill="currentColor"
                fill-opacity={0.08}
                stroke="currentColor"
                stroke-opacity={0.4}
                stroke-width={1.5}
              />
            ) : (
              <rect
                x={cpx(nd.x)}
                y={cpx(nd.y)}
                width={cpx(nd.w)}
                height={cpx(nd.h)}
                rx={cpx(clamp(Math.min(nd.w, nd.h) * 0.16, 2, 10))}
                fill="currentColor"
                fill-opacity={0.08}
                stroke="currentColor"
                stroke-opacity={0.4}
                stroke-width={1.5}
              />
            )}
            {/* foreignObject: centrado + ajuste de línea nativos (sin recortes por px). */}
            <foreignObject x={cpx(nd.x)} y={cpx(nd.y)} width={cpx(nd.w)} height={cpx(nd.h)}>
              <div
                style={{
                  boxSizing: 'border-box',
                  width: '100%',
                  height: '100%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: bullet ? 'flex-start' : 'center',
                  textAlign: bullet ? 'left' : 'center',
                  gap: '0.4em',
                  padding: `${clamp(nd.h * 0.08, 1, 6)}px ${clamp(nd.w * 0.06, 3, 14)}px`,
                  fontFamily: 'var(--font-sans, Inter, sans-serif)',
                  fontSize: `${fs}px`,
                  fontWeight: 500,
                  lineHeight: 1.2,
                  color: 'currentColor',
                  overflow: 'hidden',
                  wordBreak: 'break-word',
                }}
              >
                {bullet && (
                  <span aria-hidden="true" style={{ opacity: 0.55, flex: '0 0 auto' }}>
                    •
                  </span>
                )}
                <span style={{ minWidth: 0 }}>{text}</span>
              </div>
            </foreignObject>
          </g>
        );
      })}
    </svg>
  );
}

/* ---------- renderBox: EL render compartido del contenido de una caja ----------
 * Escenario, miniaturas y modo presentación pintan una caja SOLO por
 * aquí. `edit` añade el contenteditable y sus manejadores (únicamente cajas de
 * texto del escenario); sin `edit`, el render es estático e idéntico. */

const TEXT_BOX_CLASS = 'sytue-slidebox h-full w-full overflow-hidden px-1 outline-none';

interface TextEdit {
  boxId: string;
  onMouseDown: (e: MouseEvent) => void;
  onDblClick: (e: MouseEvent) => void;
}

/** Marcador sobrio de un medio en el escenario y las miniaturas (NO reproduce):
 *  el póster del vídeo si lo hay y, encima, el glifo del tipo; sin póster, el icono
 *  sobre un tinte tenue de la tinta. pointer-events-none: la caja se arrastra por su
 *  marco (igual que imagen/gráfico). */
function MediaPlaceholder({ box, sh }: { box: SlideBox; sh: number }): JSX.Element {
  const t = useT();
  const isAudio = box.mediaType === 'audio';
  const subs = trackLangs(box.tracks);
  const vw = (box.w / 100) * STAGE_W;
  const vh = (box.h / 100) * sh;
  const icon = Math.round(clamp(Math.min(vw, vh) * 0.3, 12, 72));
  const hasPoster = !isAudio && typeof box.poster === 'string' && box.poster !== '';
  return (
    <div
      class="pointer-events-none relative flex h-full w-full items-center justify-center overflow-hidden"
      style={
        hasPoster ? undefined : { background: 'color-mix(in srgb, currentColor 8%, transparent)' }
      }
    >
      {hasPoster && (
        <img
          src={box.poster}
          alt=""
          draggable={false}
          class="absolute inset-0 h-full w-full object-contain select-none"
        />
      )}
      <span
        aria-hidden="true"
        class="relative grid place-items-center rounded-full"
        style={{
          padding: `${Math.round(icon * 0.24)}px`,
          background: hasPoster ? 'rgba(0,0,0,0.45)' : 'transparent',
          color: hasPoster ? 'white' : 'currentColor',
          opacity: hasPoster ? 1 : 0.72,
        }}
      >
        <EditorIcon name={isAudio ? 'audio' : 'video'} size={icon} />
      </span>
      {/* Con subtítulos: distintivo con sus idiomas. En el escenario, las miniaturas y la
          vista del moderador el medio no se reproduce; los subtítulos salen al presentar. */}
      {subs !== '' && (
        <span
          role="img"
          aria-label={`${t('slides.subs')}: ${subs}`}
          title={`${t('slides.subs')}: ${subs}`}
          class="absolute right-1 bottom-1 grid place-items-center rounded-sm bg-black/55 p-0.5 text-white"
        >
          <EditorIcon name="caption" size="sm" />
        </span>
      )}
    </div>
  );
}

/** Audio con sus subtítulos: un `<audio>` no pinta pistas de texto, así que se leen las de
 *  su `<track>` y la línea activa se escribe debajo del reproductor (como texto, nunca
 *  como HTML). Con varias pistas, un selector (la de por defecto o la primera al empezar). */
function AudioConSubtitulos(props: {
  box: SlideBox;
  controls: boolean;
  autoplay: boolean;
  loop: boolean;
}): JSX.Element {
  const t = useT();
  const tracks = sanitizeTracks(props.box.tracks);
  const [lang, setLang] = useState((tracks.find((x) => x.default) ?? tracks[0])?.lang ?? '');
  const [cue, setCue] = useState('');
  const ref = useRef<HTMLAudioElement>(null);
  useEffect(() => {
    const list = Array.from(ref.current?.textTracks ?? []);
    for (const tt of list) tt.mode = tt.language === lang ? 'hidden' : 'disabled';
    const tt = list.find((x) => x.language === lang);
    if (!tt) {
      setCue('');
      return;
    }
    const on = (): void =>
      setCue(
        Array.from(tt.activeCues ?? [])
          .map((c) => ('text' in c ? String(c.text) : ''))
          .join('\n')
          .replace(/<[^>]*>/g, ''),
      );
    tt.addEventListener('cuechange', on);
    on();
    return () => tt.removeEventListener('cuechange', on);
  }, [lang, tracks.length]);
  return (
    <div class="flex h-full w-full flex-col items-center justify-center gap-1">
      {/* eslint-disable-next-line jsx-a11y/media-has-caption -- las pistas van en los <track> de abajo */}
      <audio
        ref={ref}
        src={props.box.src ?? ''}
        class="w-full max-w-full"
        controls={props.controls}
        autoPlay={props.autoplay}
        loop={props.loop}
      >
        {tracks.map((tr) => (
          <track
            key={tr.lang}
            kind="subtitles"
            srcLang={tr.lang}
            label={trackLabel(tr)}
            src={trackSrc(tr.vtt)}
          />
        ))}
      </audio>
      <div class="flex min-h-5 items-center gap-2">
        {tracks.length > 1 && (
          <select
            class="border-border bg-surface text-text h-6 rounded-sm border px-1 text-xs"
            aria-label={t('slides.subs')}
            value={lang}
            onChange={(e) => setLang(e.currentTarget.value)}
          >
            <option value="">{t('slides.subs.off')}</option>
            {tracks.map((tr) => (
              <option key={tr.lang} value={tr.lang}>
                {trackLabel(tr)}
              </option>
            ))}
          </select>
        )}
        <span aria-live="off" class="text-center text-sm whitespace-pre-line">
          {cue}
        </span>
      </div>
    </div>
  );
}

/** `sh` = alto efectivo del escenario (stageHeight): fija el aspecto del viewBox
 *  de gráfico/diagrama/forma para que no se deformen al cambiar de proporción.
 *  `present`: en el modo presentación los medios se pintan como reproductor real
 *  (<video>/<audio> con controles); en el escenario/miniaturas, como marcador. */
function renderBox(
  t: DocsT,
  b: SlideBox,
  sh: number,
  edit?: TextEdit,
  present = false,
): JSX.Element {
  if (b.kind === 'image') {
    return (
      <img
        src={b.src ?? ''}
        alt=""
        draggable={false}
        class="pointer-events-none h-full w-full object-contain select-none"
      />
    );
  }
  if (b.kind === 'media') {
    const isAudio = b.mediaType === 'audio';
    if (!present) return <MediaPlaceholder box={b} sh={sh} />;
    const controls = b.controls !== false; // ausente = con controles
    const autoplay = b.autoplay === true;
    const loop = b.loop === true;
    const tracks = sanitizeTracks(b.tracks);
    // El envoltorio corta la propagación de click/mousedown para que USAR el
    // reproductor no pase de diapositiva ni el overlay le robe el foco (esos son
    // los eventos que el overlay escucha). El mazo se sigue recorriendo con teclado.
    return (
      // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions -- técnico: aislar el reproductor del overlay
      <div
        class="h-full w-full"
        onMouseDown={(e) => e.stopPropagation()}
        onClick={(e) => e.stopPropagation()}
      >
        {isAudio ? (
          tracks.length > 0 ? (
            <AudioConSubtitulos box={b} controls={controls} autoplay={autoplay} loop={loop} />
          ) : (
            <div class="grid h-full w-full place-items-center">
              {/* eslint-disable-next-line jsx-a11y/media-has-caption -- medio insertado por el usuario sin subtítulos */}
              <audio
                src={b.src ?? ''}
                class="w-full max-w-full"
                controls={controls}
                autoPlay={autoplay}
                loop={loop}
              />
            </div>
          )
        ) : (
          // eslint-disable-next-line jsx-a11y/media-has-caption -- sin subtítulos, o con ellos en los <track>
          <video
            src={b.src ?? ''}
            poster={b.poster}
            class="h-full w-full object-contain"
            controls={controls}
            autoPlay={autoplay}
            muted={autoplay}
            loop={loop}
            playsInline
          >
            {tracks.map((tr) => (
              <track
                key={tr.lang}
                kind="subtitles"
                srcLang={tr.lang}
                label={trackLabel(tr)}
                src={trackSrc(tr.vtt)}
                default={tr.default === true}
              />
            ))}
          </video>
        )}
      </div>
    );
  }
  if (b.kind === 'chart') {
    if (!b.chart) return <div class="h-full w-full" />;
    return <SlideChartView chart={b.chart} vw={(b.w / 100) * STAGE_W} vh={(b.h / 100) * sh} />;
  }
  if (b.kind === 'smartart') {
    if (!b.smartart) return <div class="h-full w-full" />;
    return <SmartArtView smart={b.smartart} vw={(b.w / 100) * STAGE_W} vh={(b.h / 100) * sh} />;
  }
  if (b.kind === 'shape') {
    /* SVG en px del escenario (el viewBox iguala el tamaño real de la caja a
     * escala 1): trazos uniformes sin distorsión, sin degradados ni defs. */
    const w = (b.w / 100) * STAGE_W;
    const h = (b.h / 100) * sh;
    const fill = b.fill ?? SHAPE_FILL;
    const stroke = b.stroke ?? SHAPE_STROKE;
    // Formas poligonales ampliadas (triángulo, rombo, pentágono, hexágono,
    // estrella, bocadillo): un único <polygon> con los vértices calculados.
    const poly = polygonPoints(b.shape ?? 'rect', w, h);
    return (
      <svg
        class="pointer-events-none h-full w-full"
        viewBox={`0 0 ${w} ${h}`}
        preserveAspectRatio="none"
        aria-hidden="true"
      >
        {poly.length > 0 ? (
          <polygon
            points={polygonAttr(poly)}
            fill={fill}
            stroke={stroke}
            stroke-width={2}
            stroke-linejoin="round"
          />
        ) : b.shape === 'cylinder' ? (
          (() => {
            const cyl = cylinderDims(w, h);
            return (
              <>
                <path
                  d={cylinderBodyPath(cyl)}
                  fill={fill}
                  stroke={stroke}
                  stroke-width={2}
                  stroke-linejoin="round"
                />
                <path
                  d={cylinderSeamPath(cyl)}
                  fill="none"
                  stroke={stroke}
                  stroke-width={2}
                  stroke-linecap="round"
                />
              </>
            );
          })()
        ) : b.shape === 'ellipse' ? (
          <ellipse
            cx={w / 2}
            cy={h / 2}
            rx={Math.max(1, w / 2 - 1)}
            ry={Math.max(1, h / 2 - 1)}
            fill={fill}
            stroke={stroke}
            stroke-width={2}
          />
        ) : b.shape === 'line' || b.shape === 'arrow' ? (
          <>
            <line
              x1={2}
              y1={h / 2}
              x2={w - 2}
              y2={h / 2}
              stroke={stroke}
              stroke-width={3}
              stroke-linecap="round"
            />
            {b.shape === 'arrow' && (
              <path
                d={`M${w - 14} ${h / 2 - 7} L${w - 2} ${h / 2} L${w - 14} ${h / 2 + 7}`}
                fill="none"
                stroke={stroke}
                stroke-width={3}
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            )}
          </>
        ) : (
          <rect
            x={1}
            y={1}
            width={Math.max(1, w - 2)}
            height={Math.max(1, h - 2)}
            fill={fill}
            stroke={stroke}
            stroke-width={2}
          />
        )}
      </svg>
    );
  }
  if (!edit) {
    return (
      <div
        class={TEXT_BOX_CLASS}
        dangerouslySetInnerHTML={{ __html: saneaHtmlDeDocumento(b.html) }}
      />
    );
  }
  return (
    <div
      data-slidebox={edit.boxId}
      contentEditable
      role="textbox"
      tabIndex={0}
      aria-multiline="true"
      aria-label={t('slides.textBox')}
      lang="es"
      class={TEXT_BOX_CLASS}
      dangerouslySetInnerHTML={{ __html: saneaHtmlDeDocumento(b.html) }}
      onMouseDown={edit.onMouseDown}
      onDblClick={edit.onDblClick}
    />
  );
}

/** Posición/tamaño de la caja dentro del lienzo 16:9 (mismo estilo en todos los consumidores). */
function boxPos(b: SlideBox): { left: string; top: string; width: string; height: string } {
  return { left: `${b.x}%`, top: `${b.y}%`, width: `${b.w}%`, height: `${b.h}%` };
}

/** Style del morph (FLIP) de una caja emparejada: variables --m* (desfase en px
 *  del escenario + escala) que lee el keyframe `sytue-morph`, con el origen en la
 *  esquina superior izquierda para alinear la escala con left/top. */
function morphVars(from: SlideBox, to: SlideBox, sh: number): CSSProperties {
  const d = morphDelta(from, to);
  return {
    transformOrigin: '0 0',
    ['--mdx' as string]: `${(d.dxPct / 100) * STAGE_W}px`,
    ['--mdy' as string]: `${(d.dyPct / 100) * sh}px`,
    ['--msx' as string]: `${d.sx}`,
    ['--msy' as string]: `${d.sy}`,
  };
}

/* ---------- Conectores (fase Slides-Conectores) ----------
 * Capa SVG que se pinta SOBRE las cajas (escenario, miniaturas y presentación).
 * La geometría se calcula al pintar desde la posición ACTUAL de las cajas
 * ancladas (connectorRoute), así el conector se recalcula solo al moverlas. El
 * svg tiene pointer-events:none (deja pasar el clic a las cajas de debajo); solo
 * en modo edición los trazos «hit» invisibles y los tiradores de punto libre
 * capturan el puntero. Rects por id en un mapa: O(conectores + cajas), sin O(n²). */
function ConnectorLayer({
  connectors,
  boxes,
  sh,
  selId = null,
  onSelect,
  onHandleDown,
  onHandleMove,
  onHandleUp,
}: {
  connectors: SlideConnector[];
  boxes: SlideBox[];
  sh: number;
  selId?: string | null;
  onSelect?: (e: PointerEvent, id: string) => void;
  onHandleDown?: (e: PointerEvent, id: string, which: 'from' | 'to') => void;
  onHandleMove?: (e: PointerEvent) => void;
  onHandleUp?: () => void;
}): JSX.Element | null {
  if (connectors.length === 0) return null;
  const interactive = onSelect !== undefined;
  const rectById = new Map<string, CRect>();
  for (const b of boxes)
    rectById.set(b.id, {
      x: (b.x / 100) * STAGE_W,
      y: (b.y / 100) * sh,
      w: (b.w / 100) * STAGE_W,
      h: (b.h / 100) * sh,
    });
  const endGeom = (end: ConnectorEnd): ConnEnd | null => {
    if (end.boxId !== undefined) {
      const rect = rectById.get(end.boxId);
      return rect ? { rect, anchor: end.anchor ?? 'auto' } : null;
    }
    return { point: { x: ((end.x ?? 0) / 100) * STAGE_W, y: ((end.y ?? 0) / 100) * sh } };
  };
  return (
    <svg
      class="absolute inset-0 overflow-visible"
      width={STAGE_W}
      height={sh}
      viewBox={`0 0 ${STAGE_W} ${sh}`}
      fill="none"
      style={{ pointerEvents: 'none' }}
      aria-hidden="true"
    >
      {connectors.map((c) => {
        const from = endGeom(c.from);
        const to = endGeom(c.to);
        if (!from || !to) return null; // ancla perdida (caja borrada): no se pinta
        const route = connectorRoute(from, to, c.kind);
        const stroke = c.stroke ?? 'currentColor';
        const sel = interactive && selId === c.id;
        return (
          <g key={c.id}>
            {sel && (
              <path
                d={route.d}
                stroke={ACCENT}
                stroke-opacity={0.3}
                stroke-width={7}
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            )}
            <path
              d={route.d}
              stroke={stroke}
              stroke-width={2}
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            {c.startArrow && (
              <path
                d={arrowHead(route.start.x, route.start.y, route.start.angle)}
                stroke={stroke}
                stroke-width={2}
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            )}
            {c.endArrow && (
              <path
                d={arrowHead(route.end.x, route.end.y, route.end.angle)}
                stroke={stroke}
                stroke-width={2}
                stroke-linecap="round"
                stroke-linejoin="round"
              />
            )}
            {interactive && (
              // Trazo ancho invisible: agranda el área de clic para seleccionar.
              <path
                d={route.d}
                stroke="transparent"
                stroke-width={14}
                style={{ pointerEvents: 'stroke', cursor: 'pointer' }}
                onPointerDown={(e) => onSelect?.(e, c.id)}
              />
            )}
            {sel &&
              onHandleDown &&
              (
                [
                  ['from', c.from, route.start],
                  ['to', c.to, route.end],
                ] as const
              ).map(([which, end, pt]) =>
                end.boxId === undefined ? (
                  <circle
                    key={which}
                    cx={pt.x}
                    cy={pt.y}
                    r={6}
                    fill="white"
                    stroke={ACCENT}
                    stroke-width={2}
                    style={{ pointerEvents: 'all', cursor: 'move' }}
                    onPointerDown={(e) => onHandleDown(e, c.id, which)}
                    onPointerMove={onHandleMove}
                    onPointerUp={onHandleUp}
                    onPointerCancel={onHandleUp}
                  />
                ) : null,
              )}
          </g>
        );
      })}
    </svg>
  );
}

/* ---------- Vista del moderador (2.ª ventana) ----------
 * window.open gobernada por la ventana principal (mismo estado React → siempre
 * sincronizadas, sin BroadcastChannel): diapositiva ACTUAL + SIGUIENTE,
 * cronómetro + reloj y notas. El cromo usa estilos en línea (robusto aunque el
 * copiado de hojas falle); el CONTENIDO de las cajas reutiliza renderBox(t, ) y las
 * hojas copiadas (BOX_CSS, Tailwind, tokens --sytue-* y --editor-*). */

/** Prepara el documento de la 2.ª ventana: título, tema, reset y COPIA de todas
 *  las hojas de estilo del documento origen para que renderBox(t, ) se pinte igual. */
function setupPresenterWindow(w: Window, t: DocsT): void {
  const d = w.document;
  d.title = t('slides.presenter.title');
  const theme = document.documentElement.getAttribute('data-theme');
  if (theme) d.documentElement.setAttribute('data-theme', theme);
  // El idioma de la ventana es el de la sesión, no un «es» fijo: de él dependen
  // el corrector, la lectura del lector de pantalla y el formato del reloj.
  d.documentElement.setAttribute('lang', document.documentElement.lang || 'en');
  const reset = d.createElement('style');
  reset.textContent = 'html,body{margin:0;padding:0;height:100%}*{box-sizing:border-box}';
  d.head.appendChild(reset);
  document.querySelectorAll('style, link[rel="stylesheet"]').forEach((node) => {
    d.head.appendChild(node.cloneNode(true));
  });
  d.body.className = 'sytue-editor'; // resuelve --editor-* dentro de la ventana
}

/** Miniatura grande (16:9) de una diapositiva para el moderador: mismo render
 *  estático que el escenario, escalado a `w` px, con el fondo/tinta del tema. */
function PresenterSlide({
  model,
  slide,
  step,
  w,
  index,
}: {
  model: SlidesModel;
  slide: Slide;
  step: number;
  w: number;
  /** Índice 0-based de la diapositiva (resuelve los campos {n}/{total} del patrón). */
  index: number;
}): JSX.Element {
  const t = useT();
  const vis = slideVisuals(model, slide);
  const sh = stageHeight(model.size);
  const h = (w * sh) / STAGE_W;
  // Estado ESTÁTICO en el paso `step` (presencia por entrada/salida; sin animar).
  const byBox = planByBox(slidePlan(slide));
  return (
    <div
      style={{
        ...visualsStyle(vis),
        position: 'relative',
        width: `${w}px`,
        height: `${h}px`,
        overflow: 'hidden',
        borderRadius: '4px',
        boxShadow: 'var(--editor-page-shadow)',
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          width: `${STAGE_W}px`,
          height: `${sh}px`,
          transformOrigin: 'top left',
          transform: `scale(${w / STAGE_W})`,
          color: vis.ink,
        }}
      >
        {/* Patrón (slide master) detrás, siempre visible, con las formas del diseño. */}
        {masterBoxesFor(model, index).map((b) => (
          <div key={`m-${b.id}`} class="absolute" style={boxPos(b)}>
            {renderBox(t, b, sh)}
          </div>
        ))}
        <ConnectorLayer connectors={layoutConnectorsFor(model, index)} boxes={[]} sh={sh} />
        {slide.boxes
          .filter((b) => boxPresentAtStep(byBox.get(b.id) ?? [], step))
          .map((b) => (
            <div key={b.id} class="absolute" style={boxPos(b)}>
              {renderBox(t, b, sh)}
            </div>
          ))}
        <ConnectorLayer connectors={slide.connectors ?? []} boxes={slide.boxes} sh={sh} />
      </div>
    </div>
  );
}

/** Contenido de la ventana del moderador (sin hooks: props puros → la principal
 *  la repinta con render() en cada cambio). */
function PresenterView(props: {
  model: SlidesModel;
  cur: number;
  step: number;
  elapsedMs: number;
  clock: string;
  winW: number;
  onPrev: () => void;
  onNext: () => void;
  onReset: () => void;
  onEnd: () => void;
}): JSX.Element {
  const t = useT();
  const last = props.model.slides.length - 1;
  const slide = props.model.slides[props.cur] ?? { id: '', boxes: [] };
  const next = props.cur < last ? props.model.slides[props.cur + 1] : null;
  const previewW = Math.max(220, Math.min(560, Math.floor((props.winW - 72) / 2)));
  const sh = stageHeight(props.model.size);

  const ink = CONTENT_PALETTE[3]; // blanco
  const muted = CONTENT_PALETTE[2]; // gris
  const hair = 'color-mix(in srgb, white 16%, transparent)';
  /* Los tamaños salen de la escala del sistema (`--text-*`) y no de px sueltos: esta
     ventana clona las hojas de estilo de la principal, así que los tokens resuelven igual.
     Antes eran ocho medidas escritas a mano, dos de ellas fuera de la escala. */
  const caption: CSSProperties = {
    fontSize: 'var(--text-2xs)',
    letterSpacing: '0.08em',
    textTransform: 'uppercase',
    color: muted,
  };
  const btn: CSSProperties = {
    appearance: 'none',
    border: `1px solid ${hair}`,
    background: 'transparent',
    color: ink,
    borderRadius: 'var(--radius-control)',
    padding: '8px 14px',
    fontSize: 'var(--text-base)',
    cursor: 'pointer',
  };
  const btnGhost: CSSProperties = {
    ...btn,
    padding: '4px 10px',
    fontSize: 'var(--text-xs)',
    color: muted,
  };
  const colStyle: CSSProperties = {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    alignItems: 'flex-start',
  };

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        display: 'flex',
        flexDirection: 'column',
        background: CONTENT_PALETTE[0],
        color: ink,
        fontFamily: 'var(--font-sans, Inter, system-ui, sans-serif)',
      }}
    >
      <header
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: '16px',
          padding: '12px 20px',
          borderBottom: `1px solid ${hair}`,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'baseline', gap: '14px' }}>
          <span
            style={{
              fontSize: 'var(--text-2xl)',
              fontWeight: 700,
              fontVariantNumeric: 'tabular-nums',
              letterSpacing: '0.02em',
            }}
          >
            {fmtElapsed(props.elapsedMs)}
          </span>
          <button type="button" style={btnGhost} onClick={props.onReset}>
            {t('slides.presenter.reset')}
          </button>
          <span style={{ fontSize: 'var(--text-base)', color: muted }}>{props.clock}</span>
        </div>
        <span style={{ fontSize: 'var(--text-base)', color: muted }}>
          {t('slides.aria.slideOf', { n: props.cur + 1, total: last + 1 })}
        </span>
      </header>

      <main
        style={{
          display: 'flex',
          gap: '24px',
          flex: '1 1 auto',
          minHeight: 0,
          padding: '18px 20px',
          alignItems: 'flex-start',
          justifyContent: 'center',
        }}
      >
        <section style={colStyle}>
          <span style={caption}>{t('slides.presenter.current')}</span>
          <PresenterSlide
            model={props.model}
            slide={slide}
            step={props.step}
            w={previewW}
            index={props.cur}
          />
        </section>
        <section style={colStyle}>
          <span style={caption}>{t('nav.nextItem')}</span>
          {next ? (
            <PresenterSlide
              model={props.model}
              slide={next}
              step={0}
              w={previewW}
              index={props.cur + 1}
            />
          ) : (
            <div
              style={{
                display: 'grid',
                placeItems: 'center',
                width: `${previewW}px`,
                height: `${(previewW * sh) / STAGE_W}px`,
                borderRadius: '4px',
                border: `1px dashed ${hair}`,
                color: muted,
                fontSize: 'var(--text-base)',
              }}
            >
              {t('slides.presenter.endOfDeck')}
            </div>
          )}
        </section>
      </main>

      <section
        style={{
          flex: '0 0 26%',
          minHeight: '92px',
          overflow: 'auto',
          padding: '12px 20px',
          borderTop: `1px solid ${hair}`,
        }}
      >
        <div style={caption}>{t('slides.notes')}</div>
        {slide.notes && slide.notes.trim() !== '' ? (
          <p
            style={{
              whiteSpace: 'pre-wrap',
              lineHeight: 1.5,
              fontSize: 'var(--text-md)',
              margin: '6px 0 0',
            }}
          >
            {slide.notes}
          </p>
        ) : (
          <p style={{ color: muted, margin: '6px 0 0', fontSize: 'var(--text-base)' }}>
            {t('slides.presenter.noNotes')}
          </p>
        )}
      </section>

      <footer
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: '12px',
          padding: '10px 20px',
          borderTop: `1px solid ${hair}`,
        }}
      >
        <div style={{ display: 'flex', gap: '8px' }}>
          <button type="button" style={btn} onClick={props.onPrev}>
            ← {t('nav.previous')}
          </button>
          <button type="button" style={btn} onClick={props.onNext}>
            {t('nav.nextItem')} →
          </button>
        </div>
        <span style={{ fontSize: 'var(--text-xs)', color: muted }}>{t('slides.present.hint')}</span>
        <button type="button" style={btnGhost} onClick={props.onEnd}>
          {t('slides.presenter.end')}
        </button>
      </footer>
    </div>
  );
}

/* ---------- Modo presentación (fase Slides-Presentación) ----------
 * Overlay fijo a viewport completo (ese ES el fallback) sobre el que se
 * intenta la Fullscreen API; UNA diapositiva escalada al viewport conservando
 * el 16:9 y pintada con el MISMO renderBox(t, ) estático que las miniaturas.
 * Foco atrapado: Tab CICLA por los controles del visor (y por el reproductor de
 * un medio) sin salirse al documento de detrás; en fullscreen, el Esc nativo del
 * navegador sale vía fullscreenchange. Los controles a la vista (anterior /
 * siguiente / notas / salir) no son un adorno: en una pantalla táctil no hay Esc
 * ni flechas, y sin ellos de aquí no se salía. */

/** Botón del cromo del visor: 44 px de zona de toque (la medida mínima de un
 *  dedo) y hairline claro sobre el fondo oscuro del overlay. Frena la
 *  propagación porque un clic en el visor avanza de diapositiva: pulsar
 *  «anterior» y saltar dos no es lo que nadie espera. */
function PresentButton({
  icon,
  label,
  pressed,
  disabled,
  onPress,
}: {
  icon: EditorIconName;
  label: string;
  pressed?: boolean;
  disabled?: boolean;
  onPress: () => void;
}): JSX.Element {
  return (
    <button
      type="button"
      class={`rounded-control inline-flex h-11 w-11 items-center justify-center disabled:opacity-30 ${FOCUS_RING}`}
      style={{
        border: '1px solid color-mix(in srgb, white 22%, transparent)',
        background: pressed
          ? 'color-mix(in srgb, white 24%, transparent)'
          : 'color-mix(in srgb, black 45%, transparent)',
        color: 'white',
      }}
      aria-label={label}
      title={label}
      aria-pressed={pressed}
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation();
        onPress();
      }}
    >
      <EditorIcon name={icon} size="md" />
    </button>
  );
}

function Presentation(props: {
  model: SlidesModel;
  start: number;
  /** Abrir la vista del moderador (2.ª ventana) al entrar. */
  presenter?: boolean;
  onExit: () => void;
  /** Avisa de la diapositiva en la que va (0-based) al entrar y en cada cambio. */
  onSlide?: (index: number) => void;
}): JSX.Element {
  const t = useT();
  const { notify } = useToast();
  const rootRef = useRef<HTMLDivElement>(null);
  const [idx, setIdx] = useState(props.start);
  /** Paso de animación DENTRO de la diapositiva (0 = solo el contenido estático). */
  const [step, setStep] = useState(0);
  const [size, setSize] = useState(() => ({ w: window.innerWidth, h: window.innerHeight }));
  /** Notas del ponente sobre el propio overlay (fallback de una sola pantalla):
   *  se alternan con la tecla N y arrancan ocultas. Con la VISTA DEL MODERADOR
   *  abierta (2.ª ventana) las notas van allí; esta capa sigue disponible. */
  const [showNotes, setShowNotes] = useState(false);
  /** Vista del moderador: 2.ª ventana (window.open) con la diapositiva actual y
   *  la SIGUIENTE, notas y cronómetro, gobernada por ESTA ventana (mismo estado,
   *  sin BroadcastChannel). Se abre/cierra con P o al entrar por «Moderador». */
  const [presenterOn, setPresenterOn] = useState(props.presenter ?? false);
  const presenterWinRef = useRef<Window | null>(null);
  /** Cronómetro del moderador: origen (ms) y «ahora» que refresca cada segundo. El
   *  reloj se lee una vez al montar (inicializador perezoso), no en cada render. */
  const [montadoEn] = useState(Date.now);
  const startedAtRef = useRef(montadoEn);
  const [now, setNow] = useState(montadoEn);
  /** Sentido del último salto: «deslizar»/entrada de caja entran por ese lado y
   *  las animaciones solo se REPRODUCEN al avanzar (dir 1), no al retroceder. */
  const [dir, setDir] = useState<1 | -1>(1);
  /** Diapositiva de la que venimos (para el morph: interpola desde sus cajas). */
  const [prevIdx, setPrevIdx] = useState(props.start);
  const onExitRef = useUltimo(props.onExit);
  const onSlideRef = useUltimo(props.onSlide);

  useMobileBack(true, props.onExit);

  const last = props.model.slides.length - 1;
  const cur = clamp(idx, 0, last); // el modelo puede menguar en caliente (colab)
  useEffect(() => {
    onSlideRef.current?.(cur);
  }, [cur, onSlideRef]);
  const slide: Slide = props.model.slides[cur] ?? { id: '', boxes: [] };
  const steps = animStepCount(slide);
  const curStep = clamp(step, 0, steps);
  /** Alto efectivo del escenario (proporción del modelo); `size` es el viewport. */
  const sh = stageHeight(props.model.size);
  const scale = Math.max(0.05, Math.min(size.w / STAGE_W, size.h / sh));

  /** Fija diapositiva+paso con topes (el paso se recorta a los de su destino). */
  const go = (nextSlide: number, nextStep: number, dir: 1 | -1): void => {
    const t = clamp(nextSlide, 0, last);
    if (t !== cur) setPrevIdx(cur); // de dónde venimos (morph)
    setDir(dir);
    setIdx(t);
    setStep(clamp(nextStep, 0, animStepCount(props.model.slides[t] ?? { id: '', boxes: [] })));
  };

  /** Avanza/retrocede un paso (revelando/ocultando animaciones y saltando de
   *  diapositiva cuando se agotan), respetando el orden de aparición. */
  const advance = (dir: 1 | -1): void => {
    const n = presentAdvance(props.model.slides, cur, curStep, dir);
    go(n.slide, n.step, dir);
  };

  const onKeyDown = (e: KeyboardEvent): void => {
    // Con el foco dentro de un reproductor, las teclas son SUYAS (espacio = pausa,
    // flechas = avanzar el medio); si no, usar el vídeo con teclado pasaba de
    // diapositiva a la vez. Escape sigue saliendo desde cualquier sitio.
    const tag = (e.target as HTMLElement | null)?.tagName;
    if ((tag === 'VIDEO' || tag === 'AUDIO') && e.key !== 'Escape') return;
    if (e.key === 'n' || e.key === 'N') {
      e.preventDefault(); // alterna las notas del ponente sin navegar
      setShowNotes((v) => !v);
      return;
    }
    if (e.key === 'p' || e.key === 'P') {
      e.preventDefault(); // abre/cierra la vista del moderador (2.ª ventana)
      setPresenterOn((v) => !v);
      return;
    }
    // Avance/retroceso granular (revela animaciones): →/Espacio y ←.
    if (e.key === 'ArrowRight' || e.key === ' ') {
      e.preventDefault();
      advance(1);
      return;
    }
    if (e.key === 'ArrowLeft') {
      e.preventDefault();
      advance(-1);
      return;
    }
    // Trap de Tab: mientras se presenta, tabular no debe llevar a la página de detrás.
    // El visor es `aria-modal`, así que prometerlo y no cumplirlo deja a quien navega con
    // teclado paseando por controles que no puede ver. Mismo selector que el `Modal` del
    // design system, para que «enfocable» signifique lo mismo en los dos sitios.
    if (e.key === 'Tab') {
      const raiz = rootRef.current;
      const nodos = raiz ? Array.from(raiz.querySelectorAll<HTMLElement>(FOCUSABLE)) : [];
      // Sin nada enfocable dentro, Tab no lleva a ninguna parte: se queda en el visor.
      if (nodos.length === 0) {
        e.preventDefault();
        return;
      }
      const primero = nodos[0]!;
      const ultimo = nodos[nodos.length - 1]!;
      if (e.shiftKey && document.activeElement === primero) {
        e.preventDefault();
        ultimo.focus();
      } else if (!e.shiftKey && document.activeElement === ultimo) {
        e.preventDefault();
        primero.focus();
      }
      return;
    }
    // Saltos de diapositiva completa (AvPág/RePág/Inicio/Fin) y salida.
    const target = presentKeyTarget(e.key, cur, last);
    if (target === null) return;
    e.preventDefault();
    if (target === 'exit') props.onExit();
    else go(target, 0, target >= cur ? 1 : -1); // un salto aterriza al inicio de la diapositiva
  };

  useEffect(() => {
    const el = rootRef.current;
    if (!el) return;
    // A dónde vuelve el foco al salir. Sin esto, cerrar con Esc lo dejaba en <body> y
    // quien navega con teclado tenía que recorrer la aplicación entera desde el
    // principio para volver a donde estaba. Mismo patrón que `Modal.tsx`.
    const previo = document.activeElement as HTMLElement | null;
    el.focus();
    // Fullscreen API sobre el propio overlay; si el navegador la rechaza, el
    // overlay fijo ya cubre el viewport y todo funciona igual.
    void el.requestFullscreen?.().catch(() => undefined);
    // Salir del fullscreen (Esc nativo, F11…) cierra también la presentación.
    const onFsChange = (): void => {
      if (!document.fullscreenElement) onExitRef.current();
    };
    // Se mide el contenido del overlay: excluye el padding de las zonas seguras
    // y sigue el viewport visible, también al entrar/salir de pantalla completa.
    const ro = new ResizeObserver(([entrada]) => {
      if (entrada) setSize({ w: entrada.contentRect.width, h: entrada.contentRect.height });
    });
    ro.observe(el);
    document.addEventListener('fullscreenchange', onFsChange);
    return () => {
      ro.disconnect();
      document.removeEventListener('fullscreenchange', onFsChange);
      if (document.fullscreenElement) void document.exitFullscreen().catch(() => undefined);
      // `isConnected` porque el elemento de origen puede haberse desmontado mientras se
      // presentaba: enfocar un nodo huérfano no hace nada y el foco caería igualmente
      // en <body>, que es justo lo que se quiere evitar.
      if (previo?.isConnected) previo.focus();
    };
  }, [onExitRef]);

  /* La ventana del moderador escucha el teclado y navega con la MISMA lógica que
   * la principal; un ref mantiene la última clausura (idx/step frescos) sin
   * reenganchar el listener en cada render. */
  const keyRef = useUltimo(onKeyDown);
  const advanceRef = useUltimo(advance);

  /* Reloj del cronómetro: solo tictea con el moderador abierto. */
  useEffect(() => {
    if (!presenterOn) return;
    const iv = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(iv);
  }, [presenterOn]);

  /* Ciclo de vida de la 2.ª ventana: abrir (con gesto: clic/tecla → no la bloquea
   * el navegador), copiar estilos, escuchar teclado y cierre. La ventana la
   * pinta el efecto de contenido de abajo con render() de Preact. */
  useEffect(() => {
    if (!presenterOn) return;
    const w = window.open('', 'sytue-presenter', 'width=1040,height=720');
    if (!w) {
      notify({
        tone: 'warning',
        title: t('slides.msg.presenterFailed'),
        description: t('slides.msg.popupBlocked'),
      });
      setPresenterOn(false);
      return;
    }
    presenterWinRef.current = w;
    setupPresenterWindow(w, t);
    const onKey = (e: KeyboardEvent): void => keyRef.current(e);
    w.document.addEventListener('keydown', onKey);
    const onUnload = (): void => setPresenterOn(false); // cerrar la ventana termina el moderador
    w.addEventListener('beforeunload', onUnload);
    const poll = window.setInterval(() => {
      if (w.closed) setPresenterOn(false);
    }, 500);
    return () => {
      window.clearInterval(poll);
      w.document.removeEventListener('keydown', onKey);
      w.removeEventListener('beforeunload', onUnload);
      try {
        render(null, w.document.body); // desmonta el árbol Preact de la ventana
      } catch {
        /* la ventana pudo cerrarse ya */
      }
      presenterWinRef.current = null;
      if (!w.closed) w.close();
    };
  }, [notify, presenterOn, t, keyRef]);

  /* Contenido de la 2.ª ventana: se repinta al cambiar diapositiva/paso o al
   * tictear el reloj (mismo estado que la principal → siempre sincronizadas). */
  useEffect(() => {
    const w = presenterWinRef.current;
    if (!presenterOn || !w || w.closed) return;
    render(
      // La 2.ª ventana es OTRA raíz de Preact: sin este proveedor `useT()` caía al
      // catálogo por defecto y el moderador se abría en un idioma distinto al del
      // resto de la sesión. El reloj sigue al idioma del documento por lo mismo.
      <EditorI18nCtx.Provider value={t}>
        <PresenterView
          model={props.model}
          cur={cur}
          step={curStep}
          elapsedMs={now - startedAtRef.current}
          clock={new Date(now).toLocaleTimeString(document.documentElement.lang || [], {
            hour: '2-digit',
            minute: '2-digit',
          })}
          winW={w.innerWidth}
          onPrev={() => advanceRef.current(-1)}
          onNext={() => advanceRef.current(1)}
          onReset={() => {
            startedAtRef.current = Date.now();
            setNow(Date.now());
          }}
          onEnd={() => onExitRef.current()}
        />
      </EditorI18nCtx.Provider>,
      w.document.body,
    );
  }, [presenterOn, cur, curStep, now, props.model, t, advanceRef, onExitRef]);

  const anim =
    slide.transition === 'fade'
      ? 'sytue-present-fade'
      : slide.transition === 'slide'
        ? dir < 0
          ? 'sytue-present-slide-back'
          : 'sytue-present-slide-fwd'
        : '';

  const vis = slideVisuals(props.model, slide);
  const hasNotes = slide.notes !== undefined && slide.notes.trim() !== '';

  /* Plan de la diapositiva: presencia y disparo por caja (una pasada, O(n)). */
  const byBox = planByBox(slidePlan(slide));
  /* Morph: al AVANZAR (dir 1) a una diapositiva 'morph', empareja con la anterior
   * para interpolar (FLIP) las cajas presentes de arranque (paso 0). Las nuevas
   * entran con fundido; las que desaparecen no se dibujan. ponytail: sin
   * fantasmas de salida; upgrade: solaparlas y desvanecerlas si se pide. */
  const fromSlide = props.model.slides[prevIdx];
  const morph =
    slide.transition === 'morph' && dir > 0 && fromSlide && fromSlide !== slide
      ? morphPairs(fromSlide, slide)
      : null;
  const morphFrom = new Map<string, SlideBox>();
  if (morph) for (const [f, t] of morph.pairs) morphFrom.set(t.id, f);
  const morphEnter = morph ? new Set(morph.enter.map((b) => b.id)) : null;

  return (
    // Visor de presentación: captura teclado (flechas/Escape en onKeyDown) y clic para
    // navegar; role dialog no es interactivo, de ahí el aviso (el teclado ya está soportado).
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <div
      ref={rootRef}
      role="dialog"
      aria-modal="true"
      aria-label={t('slides.present')}
      tabIndex={-1}
      class="sytue-safe-fullscreen fixed inset-0 flex items-center justify-center overflow-hidden outline-none"
      style={{ background: INK, zIndex: 'var(--z-modal)' }}
      onKeyDown={onKeyDown}
      onMouseDown={(e) => e.preventDefault()} // el clic no roba el foco (teclado vivo)
      onClick={() => advance(1)}
    >
      {/* key = remonta el envoltorio al cambiar de diapositiva → re-dispara la
       * animación CSS; la escala vive en el hijo para no pisar el transform. */}
      <div
        key={slide.id}
        class={anim}
        style={{ width: `${STAGE_W * scale}px`, height: `${sh * scale}px` }}
      >
        <div
          class="relative origin-top-left"
          style={{
            ...visualsStyle(vis),
            width: `${STAGE_W}px`,
            height: `${sh}px`,
            transform: `scale(${scale})`,
          }}
        >
          {/* Patrón (slide master) detrás, siempre visible y sin acción/animación. */}
          {masterBoxesFor(props.model, cur).map((b) => (
            <div key={`m-${b.id}`} class="pointer-events-none absolute" style={boxPos(b)}>
              {renderBox(t, b, sh)}
            </div>
          ))}
          <ConnectorLayer connectors={layoutConnectorsFor(props.model, cur)} boxes={[]} sh={sh} />
          {slide.boxes.map((b) => {
            const mine = byBox.get(b.id) ?? [];
            const present = boxPresentAtStep(mine, curStep);
            // Anim que la caja dispara ESTE paso al avanzar (null al retroceder).
            const firing = boxFiringAtStep(mine, curStep, dir);
            // Una salida ANIMADA se dibuja el paso en que se dispara (termina
            // oculta, fill both); una salida instantánea (disappear) ya no se dibuja.
            const exitAnim =
              firing !== null && firing.category === 'exit' && !ANIM_INSTANT.has(firing.anim.type);
            if (!present && !exitAnim) return null;
            // Trayectoria disparada este paso: la caja recorre un offset-path (no un
            // keyframe sytue-anim-*). El resto de efectos son «reveal» clásico.
            const firingMotion = firing !== null && firing.category === 'motion';
            // reveal = reproducir un keyframe (entrada/énfasis/salida animada). El
            // key cambia al dispararse → remonta y re-lanza la animación CSS.
            const reveal = firing !== null && !firingMotion && !ANIM_INSTANT.has(firing.anim.type);
            // Morph solo al ENTRAR (paso 0); las cajas con entrada aún no están presentes.
            const morphSrc = curStep === 0 ? morphFrom.get(b.id) : undefined;
            const morphIn = curStep === 0 && morphEnter?.has(b.id) === true;
            // Enlace de acción: clic navega y NO avanza el fondo. ponytail: solo
            // ratón; el mazo sigue recorrible con flechas (a11y del teclado).
            const hasAction = b.action !== undefined;
            const cls =
              'absolute' +
              (firingMotion
                ? ' sytue-motion'
                : reveal
                  ? ` sytue-anim-${firing.anim.type}`
                  : morphSrc
                    ? ' sytue-morph'
                    : morphIn
                      ? ' sytue-present-fade'
                      : '');
            // Trayectorias: desfase acumulado (reposo) y, si dispara, la geometría del
            // offset-path (origen = centro de la caja + desfase de las trayectorias
            // previas; recorre hasta +su final). Fuera del disparo, una traslación
            // estática mantiene la caja donde la dejó el último movimiento.
            const rest = motionRestOffset(mine, curStep);
            const mulX = STAGE_W / 100;
            const mulY = sh / 100;
            let motionStyle: CSSProperties = {};
            if (firingMotion) {
              const end = motionEnd(firing.anim.type, firing.anim.points);
              // Origen RELATIVO a la caja (su left/top ya la posiciona): sin (x,y),
              // que la teletransportaría su propia posición. Ver motionRenderOrigin.
              const { ox, oy } = motionRenderOrigin(b.w, b.h, rest, end, mulX, mulY);
              const d = motionPathD(
                motionPoints(firing.anim.type, firing.anim.points),
                mulX,
                mulY,
                ox,
                oy,
              );
              motionStyle = {
                offsetPath: `path('${d}')`,
                offsetRotate: '0deg',
                offsetDistance: '100%',
                ...(firing.delayMs > 0 ? { animationDelay: `${firing.delayMs}ms` } : {}),
              };
            } else if (firing === null && (rest.dx !== 0 || rest.dy !== 0)) {
              motionStyle = { transform: `translate(${rest.dx * mulX}px, ${rest.dy * mulY}px)` };
            }
            const style: CSSProperties = {
              ...boxPos(b),
              ...(reveal && firing.delayMs > 0 ? { animationDelay: `${firing.delayMs}ms` } : {}),
              ...motionStyle,
              ...(morphSrc ? morphVars(morphSrc, b, sh) : {}),
              ...(hasAction ? { cursor: 'pointer' } : {}),
            };
            return (
              // Enlace de acción solo-ratón (ver nota arriba); el mazo se recorre con las
              // flechas del teclado, así que no añadimos un onKeyDown por caja.
              // El `role` es condicional (solo las cajas con acción son enlaces) y
              // oxlint lee la rama sin role como elemento estático con handler.
              // oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
              <div
                key={reveal || firingMotion ? `${b.id}:${curStep}:${firing.index}` : b.id}
                class={cls}
                style={style}
                role={hasAction ? 'link' : undefined}
                aria-label={hasAction ? actionLabel(t, b.action!) : undefined}
                title={hasAction ? actionLabel(t, b.action!) : undefined}
                onClick={
                  hasAction
                    ? (e): void => {
                        e.stopPropagation();
                        const t = actionTarget(b.action!, cur, last);
                        if (t !== null) go(t, 0, t >= cur ? 1 : -1);
                      }
                    : undefined
                }
              >
                {renderBox(t, b, sh, undefined, true)}
              </div>
            );
          })}
          {/* Conectores de la diapositiva (estáticos: sin interacción). */}
          <ConnectorLayer connectors={slide.connectors ?? []} boxes={slide.boxes} sh={sh} />
        </div>
      </div>
      {/* Notas del ponente (tecla N o el botón): fondo papel para leerlas sobre el
       * overlay oscuro; stopPropagation para que interactuar no pase de diapositiva. */}
      {showNotes && hasNotes && (
        // onClick solo frena la propagación (leer las notas no debe pasar de diapositiva);
        // no es un control interactivo.
        // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
        <div
          /* pb-16: los controles del visor viven en esa esquina y taparían la
             última línea de las notas. */
          class="fixed inset-x-0 bottom-0 max-h-[32%] overflow-auto pt-4 pr-[max(2rem,var(--sytue-safe-right,0px))] pb-[calc(4rem+var(--sytue-safe-bottom,0px))] pl-[max(2rem,var(--sytue-safe-left,0px))]"
          style={{ background: 'var(--editor-page-bg, white)', color: INK }}
          onClick={(e) => e.stopPropagation()}
        >
          <p class="mx-auto max-w-4xl text-base leading-relaxed whitespace-pre-wrap">
            {slide.notes}
          </p>
        </div>
      )}
      <p
        aria-hidden="true"
        class="fixed bottom-[max(1rem,var(--sytue-safe-bottom,0px))] left-[max(0.75rem,var(--sytue-safe-left,0px))] text-xs"
        style={{ color: 'color-mix(in srgb, white 45%, transparent)' }}
      >
        {presenterOn ? t('slides.present.presenterOpen') : t('slides.present.keys')}
      </p>
      {/* Controles a la vista: en una pantalla táctil no hay Esc ni flechas, así
       * que sin esto no se podía ni retroceder ni SALIR de la presentación. Se
       * atenúan para no competir con la diapositiva y vuelven enteros al
       * apuntarlos o al llegarles el foco (que es también lo que atrapa Tab). */}
      <div class="fixed right-[max(0.75rem,var(--sytue-safe-right,0px))] bottom-[max(0.75rem,var(--sytue-safe-bottom,0px))] flex items-center gap-1.5 opacity-35 transition-opacity focus-within:opacity-100 hover:opacity-100">
        {hasNotes && (
          <PresentButton
            icon="comment"
            label={t('slides.notes')}
            pressed={showNotes}
            onPress={() => setShowNotes((v) => !v)}
          />
        )}
        <PresentButton
          icon="chevronLeft"
          label={t('nav.previous')}
          disabled={cur === 0 && curStep === 0}
          onPress={() => advance(-1)}
        />
        <PresentButton
          icon="chevronRight"
          label={t('nav.nextItem')}
          disabled={cur === last && curStep === steps}
          onPress={() => advance(1)}
        />
        <PresentButton icon="close" label={t('slides.present.exit')} onPress={props.onExit} />
      </div>
      <p class="sr-only" aria-live="polite">
        {t('slides.aria.slideOf', { n: cur + 1, total: last + 1 })}
        {steps > 0 ? `, ${t('slides.present.step', { n: curStep, total: steps })}` : ''}
      </p>
    </div>
  );
}

/* ---------- Componente ---------- */

/** Un solo aviso de «bloqueado» cada 1,5 s (un arrastre intenta guardar en cada paso). */
let ultimoAvisoProtegido = 0;
function avisoProtegidoPermitido(): boolean {
  const ahora = Date.now();
  if (ahora - ultimoAvisoProtegido <= 1500) return false;
  ultimoAvisoProtegido = ahora;
  return true;
}

export function SlidesCanvas({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = useT();
  const { notify } = useToast();
  const areaRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<HTMLDivElement>(null);
  const [model, setModel] = useState<SlidesModel>(() => emptySlidesModel(t));
  const [active, setActive] = useState(0);
  const [selBox, setSelBox] = useState<string | null>(null);
  /** Cajas AÑADIDAS a la selección con Mayús+clic (multiselección para
   *  organizar/alinear/agrupar); NO incluye a `selBox` (el primario/ancla). La
   *  selección efectiva es `selBox` + estas, expandida por grupos. */
  const [extraSel, setExtraSel] = useState<string[]>([]);
  /** Conector seleccionado (id) de la diapositiva activa; null = ninguno. Es
   *  independiente de la selección de cajas (una u otra, nunca las dos). */
  const [selConn, setSelConn] = useState<string | null>(null);
  const [editBox, setEditBox] = useState<string | null>(null);
  const [area, setArea] = useState({ w: 800, h: 480 });
  /** Índice inicial del modo presentación; null = no se está presentando. */
  const [presenting, setPresenting] = useState<number | null>(null);
  /** ¿Arrancar la presentación con la vista del moderador abierta? */
  const [presentMod, setPresentMod] = useState(false);
  /** «Presentar con Cast»: el diálogo y, mientras se presenta, quién avisa de la diapositiva. */
  const [castOpen, setCastOpen] = useState(false);
  const [castAvisar, setCastAvisar] = useState<((i: number) => void) | null>(null);
  /** Panel inferior bajo el escenario: notas del ponente, orden de animaciones o
   *  datos del gráfico seleccionado (null = ninguno). Notas visible por defecto. */
  const [bottom, setBottom] = useState<
    'notes' | 'anim' | 'chart' | 'smartart' | 'subs' | 'theme' | 'protect' | null
  >('notes');
  /** «Editar patrón»: el lienzo edita el patrón (slide master) en vez de la
   *  diapositiva activa; al cerrar, sus elementos aparecen en todas. */
  const [editingMaster, setEditingMaster] = useState(false);
  /** Dentro de «Editar patrón»: el diseño que se edita (null = el patrón mismo). */
  const [editingLayout, setEditingLayout] = useState<string | null>(null);
  /** Categoría elegida en el panel de animaciones para añadir un efecto nuevo. */
  const [animCat, setAnimCat] = useState<SlideAnimCategory>('entrance');
  /* Protección (proteger.ts): la contraseña presentada en esta sesión (solo memoria, nunca se
   * guarda), el modelo tal cual lo trajo el servidor y el último aviso de «bloqueado». */
  const claveRef = useRef('');
  const originalRef = useRef<SlidesModel | null>(null);
  /* Hay una contraseña presentada (para pintar): su valor vive en claveRef. */
  const [conClave, setConClave] = useState(false);

  /* Alto efectivo del escenario según la proporción del modelo (16:9 por defecto);
   * el ancho interno es siempre STAGE_W y el escenario se escala uniformemente. */
  const sh = stageHeight(model.size);
  /* Escala del escenario: auto-encaje al hueco × zoom (§7.2).
   *
   * El ANCHO manda siempre; el alto, solo si tiene hueco. Antes se tomaba el
   * mínimo de los dos y, si salía ≤ 0, se caía a 0,5 fijo: un escenario de 480px
   * que no mira el ancho. Eso pasa antes de la primera medida (el área mide 0×0) y
   * cuando en un móvil la cinta y los paneles dejan el área sin alto; en un teléfono
   * cuyo hueco mide ~130px la diapositiva se salía +372px (medido por el barrido
   * móvil en iPhone 12). Sin medida no se inventa un tamaño: se pinta al mínimo y
   * el ResizeObserver la encaja en cuanto mide. Con los dos ejes medidos, igual que
   * antes. */
  const fitW = (area.w - FIT_GAP) / STAGE_W;
  const fitH = (area.h - FIT_GAP) / sh;
  const fit = fitW > 0 ? (fitH > 0 ? Math.min(fitW, fitH) : fitW) : 0;
  const scale = Math.max(0.08, fit * (ctx.ui.zoom / 100));

  /* Espejo síncrono para manejadores registrados una sola vez y para el arrastre. */
  const stateRef = useUltimo({
    ctx,
    model,
    active,
    selBox,
    extraSel,
    editBox,
    scale,
    sh,
    editingMaster,
    editingLayout,
  });

  const slide: Slide = model.slides[active] ?? model.slides[0] ?? { id: '', boxes: [] };
  /** Cajas que edita el lienzo: las del patrón (modo «Editar patrón») o las de la
   *  diapositiva activa. Un único origen para selección, gestos y render. */
  const editBoxes = useMemo<SlideBox[]>(() => {
    if (!editingMaster) return slide.boxes;
    if (editingLayout)
      return layoutEditBoxes(model.master?.layouts?.find((l) => l.id === editingLayout));
    return model.master?.boxes ?? [];
  }, [editingMaster, editingLayout, model.master, slide.boxes]);
  /** Los diseños vigentes: los del patrón o, si no los toca, los tres de serie. */
  const layoutDefs = useMemo(
    () => layoutsOf(model.master?.layouts, () => builtinLayouts(t)),
    [model.master?.layouts, t],
  );

  /* ----- multiselección (fase Slides-Organizar) -----
   * `selBox` = primario/ancla; `extraSel` = añadidas con Mayús+clic. La
   * multiselección solo se conserva mientras se construye con Mayús: al cambiar
   * el primario (clic normal/insertar/borrar), de diapositiva o de modo, se
   * descarta (dos efectos), así los demás sitios que hacen setSelBox no tocan
   * `extraSel`. La selección EFECTIVA expande los grupos (mover juntas). */
  useEffect(() => {
    setExtraSel((prev) => (prev.length ? [] : prev)); // cambia el primario ⇒ sin multiselección
  }, [selBox]);
  useEffect(() => {
    setExtraSel((prev) => (prev.length ? [] : prev)); // cambia diapositiva/modo ⇒ sin multiselección
  }, [active, editingMaster]);
  useEffect(() => {
    setSelConn(null); // el conector pertenece a una diapositiva/modo concretos
  }, [active, editingMaster]);

  /* Cursor remoto: publica la diapositiva activa + las cajas seleccionadas a los
   * peers. En «Editar patrón» los ids no son cajas de la diapositiva → se envía
   * sin cajas. El transporte agrupa por tiempo y degrada sin WS; sendCursor es estable. */
  const sendCursor = ctx.sendCursor;
  useEffect(() => {
    const boxes = editingMaster ? [] : selBox ? [selBox, ...extraSel] : [];
    sendCursor({ kind: 'slide', slide: active, boxes });
  }, [active, selBox, extraSel, editingMaster, sendCursor]);

  /** Ids explícitos seleccionados (primario + extra), en el orden del array. */
  const explicitSel = (): string[] => {
    const cur = stateRef.current;
    return cur.selBox ? [cur.selBox, ...cur.extraSel] : [];
  };
  /** Selección EFECTIVA: la explícita con los grupos expandidos (lo que se
   *  mueve/alinea/organiza junto). */
  const effSelIds = (): string[] => expandGroups(editTargetBoxes(), explicitSel());
  const isSelected = (id: string): boolean =>
    stateRef.current.selBox === id || stateRef.current.extraSel.includes(id);

  /** Selección limpia de una sola caja (o ninguna): también borra la
   *  multiselección y la selección de conector (una cosa u otra). */
  const selectOnly = (id: string | null): void => {
    setSelBox(id);
    setExtraSel([]);
    setSelConn(null);
    stateRef.current = { ...stateRef.current, selBox: id, extraSel: [] };
  };
  /** Alterna una caja en la multiselección (Mayús+clic). El primario no se quita
   *  con Mayús (mantiene un ancla estable); si no había primario, pasa a serlo. */
  const toggleSel = (id: string): void => {
    setSelConn(null);
    const cur = stateRef.current;
    if (cur.selBox === null) {
      selectOnly(id);
      return;
    }
    if (id === cur.selBox) return; // el ancla no se deselecciona con Mayús
    const rest = cur.extraSel.includes(id)
      ? cur.extraSel.filter((x) => x !== id)
      : [...cur.extraSel, id];
    setExtraSel(rest);
    stateRef.current = { ...cur, extraSel: rest };
  };

  /* ----- persistencia (contrato §7: mismo camino que el folio) ----- */

  const apply = (
    patch: {
      slides?: Slide[];
      theme?: string | undefined;
      themes?: SlideCustomTheme[] | undefined;
      master?: SlideMaster | undefined;
      size?: SlideSize | undefined;
      protect?: SlidesProtection | undefined;
    },
    persist = true,
  ): void => {
    // Fusiona sobre el modelo actual: conserva los campos de nivel superior
    // (p. ej. `theme`, `master`) aunque el llamante solo cambie `slides`. Para
    // BORRAR el tema, el llamante pasa `theme: undefined` y aquí se elimina.
    const next = { ...stateRef.current.model, ...patch };
    if (next.theme === undefined) delete next.theme;
    if (next.protect === undefined) delete next.protect;
    // Tamaño ausente = 16:9 (JSON limpio; «sin size = comportamiento clásico»).
    if (next.size === undefined) delete next.size;
    // Sin temas propios = sin campo (JSON limpio).
    if (!next.themes || next.themes.length === 0) delete next.themes;
    // Patrón vacío = sin patrón (JSON limpio; «sin master = comportamiento actual»), salvo que
    // lleve su lista de diseños: sin ella los diseños volverían a los tres de serie.
    if (!next.master || (next.master.boxes.length === 0 && next.master.layouts === undefined))
      delete next.master;
    const model = next as SlidesModel;
    // Protección: lo bloqueado no cambia sin permiso (propietario o contraseña). Lo cumple el
    // servidor; esto solo avisa antes de enviar algo que va a rechazar.
    const autorizado = esPropietario() || claveRef.current !== '';
    if (!autorizado && cambioProtegido(stateRef.current.model, model)) {
      if (avisoProtegidoPermitido()) {
        notify({
          tone: 'warning',
          title: t('slides.msg.protected'),
          description: t('sheet.msg.protectionDenied'),
        });
      }
      return;
    }
    stateRef.current.model = model; // coherencia dentro del mismo tick (arrastre)
    setModel(model);
    if (persist) {
      if (claveRef.current !== '') presentarClave(claveRef.current);
      // Quien no está autorizado guarda lo bloqueado tal cual lo trajo el servidor.
      const aGuardar = autorizado ? model : conBloqueadoOriginal(model, originalRef.current);
      stateRef.current.ctx.onContentInput(JSON.stringify(aGuardar));
    }
  };

  /** Cambia la protección (los campos con undefined se quitan). */
  const setProtect = (patch: {
    [K in keyof SlidesProtection]?: SlidesProtection[K] | undefined;
  }): void => {
    // sanitizeSlidesProtection quita lo undefined y las listas vacías.
    apply({ protect: sanitizeSlidesProtection({ ...stateRef.current.model.protect, ...patch }) });
  };

  /** Cajas del OBJETIVO de edición: el patrón si se está editando, si no la
   *  diapositiva activa. */
  const editTargetBoxes = (): SlideBox[] => {
    const cur = stateRef.current;
    if (!cur.editingMaster) return cur.model.slides[cur.active]?.boxes ?? [];
    const m = cur.model.master;
    return cur.editingLayout
      ? layoutEditBoxes(m?.layouts?.find((l) => l.id === cur.editingLayout))
      : (m?.boxes ?? []);
  };

  /** Localiza una caja dentro del objetivo de edición actual. */
  const findEditBox = (boxId: string): SlideBox | undefined =>
    editTargetBoxes().find((b) => b.id === boxId);

  /** Transforma las cajas del objetivo de edición (patrón o diapositiva activa).
   *  Arrastre, texto, insertar y borrar pasan por aquí: el mismo lienzo sirve a
   *  ambos modos sin duplicarse. */
  const applyBoxes = (fn: (boxes: SlideBox[]) => SlideBox[], persist = true): void => {
    const cur = stateRef.current;
    if (cur.editingMaster) {
      const m = cur.model.master;
      if (cur.editingLayout) {
        const layouts = (m?.layouts ?? []).map((l) =>
          l.id === cur.editingLayout ? applyLayoutBoxes(l, fn(layoutEditBoxes(l))) : l,
        );
        apply({ master: { boxes: m?.boxes ?? [], layouts } }, persist);
      } else {
        apply({ master: { ...m, boxes: fn(m?.boxes ?? []) } }, persist);
      }
    } else {
      apply(
        {
          slides: cur.model.slides.map((s, i) =>
            i === cur.active ? { ...s, boxes: fn(s.boxes) } : s,
          ),
        },
        persist,
      );
    }
  };

  const patchBox = (boxId: string, patch: Partial<SlideBox>, persist: boolean): void => {
    applyBoxes((boxes) => boxes.map((b) => (b.id === boxId ? { ...b, ...patch } : b)), persist);
  };

  /** Cambia los subtítulos de un medio (lista vacía = sin ellos). Un `.vtt` puede reventar
   *  el op-body: como al insertar un medio, se comprueba el modelo entero y, si no cabe,
   *  toast honesto y no se toca nada. */
  const setMediaTracks = (boxId: string, tracks: SlideTrack[]): void => {
    const boxes = editTargetBoxes().map((b) => {
      if (b.id !== boxId) return b;
      const { tracks: _antes, ...resto } = b;
      return tracks.length > 0 ? { ...resto, tracks } : resto;
    });
    if (JSON.stringify(modelWithBoxes(boxes)).length > MODEL_MAX_CHARS) {
      notify({
        tone: 'warning',
        title: t('slides.msg.mediaTooBig'),
        description: t('slides.msg.mediaTooBig.detail'),
      });
      return;
    }
    applyBoxes(() => boxes);
  };

  /* ----- conectores (fase Slides-Conectores) -----
   * Viven en `slide.connectors` (no en boxes); solo en diapositivas (no en el
   * patrón). Un array vacío se limpia (JSON: «sin conectores = sin campo»). */

  /** Transforma los conectores de la diapositiva activa. `persist` false durante
   *  el arrastre de un tirador; el commit llega al soltar (como las cajas). */
  const applyConnectors = (
    fn: (cs: SlideConnector[]) => SlideConnector[],
    persist = true,
  ): void => {
    const cur = stateRef.current;
    apply(
      {
        slides: cur.model.slides.map((s, i) => {
          if (i !== cur.active) return s;
          const next = fn(s.connectors ?? []);
          if (next.length === 0) {
            const { connectors: _drop, ...rest } = s;
            return rest;
          }
          return { ...s, connectors: next };
        }),
      },
      persist,
    );
  };

  /** Inserta un conector del tipo pedido: si hay ≥2 cajas seleccionadas, une las
   *  dos primeras por id (se recalcula al moverlas); si no, un conector de dos
   *  puntos libres en el centro del lienzo (tiradores arrastrables). */
  const addConnector = (kindRaw: string): void => {
    const cur = stateRef.current;
    if (cur.editingMaster) return; // conectores solo en diapositivas
    const kind: ConnectorKind = CONNECTOR_KIND_SET.has(kindRaw as ConnectorKind)
      ? (kindRaw as ConnectorKind)
      : 'straight';
    const boxes = editTargetBoxes();
    const chosen = explicitSel()
      .map((id) => boxes.find((b) => b.id === id))
      .filter((b): b is SlideBox => b !== undefined)
      .slice(0, 2);
    const id = crypto.randomUUID();
    const conn: SlideConnector =
      chosen.length === 2
        ? { id, kind, from: { boxId: chosen[0]!.id }, to: { boxId: chosen[1]!.id }, endArrow: true }
        : { id, kind, from: { x: 28, y: 42 }, to: { x: 64, y: 62 }, endArrow: true };
    applyConnectors((cs) => [...cs, conn]);
    selectOnly(null);
    setSelConn(id);
  };

  /** Cambia el estilo de un conector (tipo/puntas/color); poda los defectos para
   *  un JSON limpio. */
  const patchConnector = (id: string, patch: Partial<SlideConnector>): void => {
    applyConnectors((cs) =>
      cs.map((c) => {
        if (c.id !== id) return c;
        const next: SlideConnector = { ...c, ...patch };
        if (next.startArrow !== true) delete next.startArrow;
        if (next.endArrow !== true) delete next.endArrow;
        if (next.stroke === undefined) delete next.stroke;
        return next;
      }),
    );
  };

  const deleteConnector = (id: string): void => {
    applyConnectors((cs) => cs.filter((c) => c.id !== id));
    setSelConn(null);
  };

  const onConnSelect = (e: PointerEvent, id: string): void => {
    e.preventDefault();
    e.stopPropagation();
    selectOnly(null); // deselecciona cajas (y limpia selConn); luego marca este
    setSelConn(id);
    stageRef.current?.focus(); // que Supr/Esc lleguen al onKeyDown del escenario
  };

  /** Arrastre de un tirador de EXTREMO LIBRE (los anclados siguen a su forma):
   *  reposiciona el punto en % del lienzo; el extremo queda como punto libre. */
  const connDragRef = useRef<{ id: string; which: 'from' | 'to' } | null>(null);
  const onConnHandleDown = (e: PointerEvent, id: string, which: 'from' | 'to'): void => {
    e.preventDefault();
    e.stopPropagation();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    connDragRef.current = { id, which };
  };
  const onConnHandleMove = (e: PointerEvent): void => {
    const d = connDragRef.current;
    const rect = stageRef.current?.getBoundingClientRect();
    if (!d || !rect) return;
    const x = Math.round(clamp(((e.clientX - rect.left) / rect.width) * 100, 0, 100) * 10) / 10;
    const y = Math.round(clamp(((e.clientY - rect.top) / rect.height) * 100, 0, 100) * 10) / 10;
    const pt: ConnectorEnd = { x, y };
    applyConnectors(
      (cs) =>
        cs.map((c) =>
          c.id === d.id ? (d.which === 'from' ? { ...c, from: pt } : { ...c, to: pt }) : c,
        ),
      false,
    );
  };
  const onConnHandleUp = (): void => {
    if (!connDragRef.current) return;
    connDragRef.current = null;
    stateRef.current.ctx.onContentInput(JSON.stringify(stateRef.current.model));
  };

  /* ----- operaciones de diapositiva ----- */

  const addSlide = (layout?: string): void => {
    const cur = stateRef.current;
    const slides = [...cur.model.slides];
    const defs = layoutsOf(cur.model.master?.layouts, () => builtinLayouts(t));
    slides.splice(
      cur.active + 1,
      0,
      newSlide(layout ?? defaultNewLayoutId(defs, cur.model.slides[cur.active]?.layout), t, defs),
    );
    apply({ slides });
    setActive(cur.active + 1);
    setSelBox(null);
    setEditBox(null);
  };

  /**
   * Reparte la respuesta del asistente en diapositivas, detrás de la actual.
   *
   * En Diapos «insertar» no es escribir un párrafo. El panel genérico volcaba la
   * respuesta entera en una caja de texto: una lámina con seiscientas palabras y
   * ningún título, ilegible. Aquí el corte lo marcan los títulos del modelo y el
   * tamaño, y cada lámina sale con su título arriba y sus puntos debajo.
   *
   * El HTML de las cajas lo escribe ESTA función con el texto escapado: el
   * modelo aporta palabras, nunca marcado.
   */
  const iaFill = (texto: string): void => {
    const diapos = diapositivasDe(bloquesDe(texto));
    if (diapos.length === 0) return;
    const cur = stateRef.current;
    const nuevas: Slide[] = diapos.map((d) => {
      const base = newSlide(
        d.titulo.length > 0 ? 'titulo-cuerpo' : 'titulo',
        t,
        layoutsOf(cur.model.master?.layouts, () => builtinLayouts(t)),
      );
      const cajas = base.boxes.filter((b) => !b.kind || b.kind === 'text');
      const html = htmlDeCajas(d, cajas.length, escapeHtml);
      const boxes = base.boxes.map((b) => {
        const i = cajas.findIndex((c) => c.id === b.id);
        return i >= 0 && i < html.length ? { ...b, html: html[i]! } : b;
      });
      return { ...base, boxes };
    });
    const slides = [...cur.model.slides];
    slides.splice(cur.active + 1, 0, ...nuevas);
    apply({ slides });
    // Se deja el foco en la PRIMERA de las nuevas y no en la última: quien acaba
    // de generar cinco láminas quiere revisarlas desde el principio.
    setActive(cur.active + 1);
    setSelBox(null);
    setEditBox(null);
  };

  /** Inserta la diapositiva de un bloque reutilizable (slides/bloque.ts) detrás de la actual. */
  const insertarBloque = (raw: string): void => {
    const nueva = diapositivaDeBloque(raw);
    if (!nueva) return;
    const cur = stateRef.current;
    const slides = [...cur.model.slides];
    slides.splice(cur.active + 1, 0, nueva);
    apply({ slides });
    setActive(cur.active + 1);
    setSelBox(null);
    setEditBox(null);
  };

  const duplicateSlide = (): void => {
    const cur = stateRef.current;
    const src = cur.model.slides[cur.active];
    if (!src) return;
    // Las cajas reciben ids nuevos; los conectores se remapean a esos ids (si no,
    // apuntarían a las cajas de la diapositiva original).
    const idMap = new Map<string, string>();
    const boxes = src.boxes.map((b) => {
      const nid = crypto.randomUUID();
      idMap.set(b.id, nid);
      return { ...b, id: nid };
    });
    const copy: Slide = {
      ...src, // conserva la transición (y campos futuros de Slide)
      id: crypto.randomUUID(),
      boxes,
    };
    if (src.connectors && src.connectors.length > 0)
      copy.connectors = remapConnectorIds(src.connectors, idMap);
    const slides = [...cur.model.slides];
    slides.splice(cur.active + 1, 0, copy);
    // Duplicar una diapositiva con imágenes también puede reventar el op-body.
    if (JSON.stringify({ slides }).length > MODEL_MAX_CHARS) {
      notify({
        tone: 'warning',
        title: t('slides.msg.dupeFailed'),
        description: t('slides.msg.dupeFailed.detail'),
      });
      return;
    }
    apply({ slides });
    setActive(cur.active + 1);
    setSelBox(null);
    setEditBox(null);
  };

  const deleteSlide = async (): Promise<void> => {
    const cur = stateRef.current;
    if (cur.model.slides.length <= 1) return; // siempre queda al menos una (§7.2)
    if (
      !(await confirmarEnEditor({
        mensaje: editorT()('dialog.slide.delete', { n: cur.active + 1 }),
        confirmLabel: editorT()('slides.delete'),
        tone: 'danger',
      }))
    )
      return;
    const slides = cur.model.slides.filter((_, i) => i !== cur.active);
    apply({ slides });
    setActive(clamp(cur.active, 0, slides.length - 1));
    setSelBox(null);
    setEditBox(null);
  };

  /** Reordena desde el carril (drag and drop): un commit por movimiento. */
  const moveSlideTo = (from: number, to: number): void => {
    const cur = stateRef.current;
    const act = cur.model.slides[cur.active];
    const slides = reorderSlides(cur.model.slides, from, to);
    if (slides === cur.model.slides) return;
    apply({ slides });
    setActive(
      Math.max(
        0,
        slides.findIndex((s) => s === act),
      ),
    );
  };

  /** Cambia la transición de entrada de la diapositiva activa (mini-barra). */
  const setTransition = (t: SlideTransition): void => {
    const cur = stateRef.current;
    apply({
      slides: cur.model.slides.map((s, i) => (i === cur.active ? { ...s, transition: t } : s)),
    });
  };

  /** Aplica el tema (fondo/tinta/fuente) a TODA la presentación; '' = Papel
   *  (se elimina el campo). */
  const setTheme = (id: string): void => {
    if (id === CUSTOM_THEME_VALUE) {
      customizeTheme();
      return;
    }
    const valid =
      SLIDE_THEMES.some((x) => x.id === id) ||
      sanitizeCustomThemes(stateRef.current.model.themes).some((x) => x.id === id);
    apply({ theme: valid ? id : undefined });
    if (!valid || SLIDE_THEMES.some((x) => x.id === id))
      setBottom((b) => (b === 'theme' ? null : b));
  };

  /** Fija el tamaño/proporción del lienzo de TODA la presentación (comando del
   *  ribbon: JSON de SlideSize). Vacío/'16:9'/inválido = 16:9 (elimina el campo).
   *  Las cajas van en %, así que solo cambia la proporción: el contenido refluye
   *  al nuevo alto sin deformarse. */
  const setSlideSize = (payloadJson?: string): void => {
    let size: SlideSize | undefined;
    try {
      size = payloadJson ? sanitizeSlideSize(JSON.parse(payloadJson)) : undefined;
    } catch {
      size = undefined;
    }
    apply({ size });
  };

  /** Fija/quita el color de fondo propio de la diapositiva activa. 'transparent'
   *  = usar el del tema (se elimina el campo). */
  const setSlideBg = (color: string): void => {
    const cur = stateRef.current;
    apply({
      slides: cur.model.slides.map((s, i) => {
        if (i !== cur.active) return s;
        const ns = { ...s };
        if (color === 'transparent') delete ns.bg;
        else ns.bg = color;
        return ns;
      }),
    });
  };

  /** Actualiza las notas del ponente de la diapositiva activa (persiste; el
   *  autosave ya viene con debounce). Vacío = se borra el campo (JSON limpio). */
  const setNotes = (text: string): void => {
    const cur = stateRef.current;
    apply({
      slides: cur.model.slides.map((s, i) => {
        if (i !== cur.active) return s;
        const ns = { ...s };
        if (text === '') delete ns.notes;
        else ns.notes = text;
        return ns;
      }),
    });
  };

  /* ----- patrón de diapositiva (slide master) ----- */

  const enterMaster = (): void => {
    setEditingMaster(true);
    setEditingLayout(null);
    setSelBox(null);
    setEditBox(null);
    setBottom(null); // notas/animaciones no aplican al patrón
  };
  const exitMaster = (): void => {
    setEditingMaster(false);
    setEditingLayout(null);
    setSelBox(null);
    setEditBox(null);
  };

  /** Añade al patrón una caja de texto con el campo {n} (número de diapositiva),
   *  anclada abajo a la derecha: el clásico número de página del patrón. */
  const addPageNumberBox = (): void => {
    pushBox(newBox(84, 89, 12, 8, '<p style="text-align:right">{n}</p>'));
  };

  /** Muestra/oculta los elementos del patrón en la diapositiva activa (portada…). */
  const toggleHideMaster = (): void => {
    const cur = stateRef.current;
    apply({
      slides: cur.model.slides.map((s, i) => {
        if (i !== cur.active) return s;
        const ns = { ...s };
        if (ns.hideMaster) delete ns.hideMaster;
        else ns.hideMaster = true;
        return ns;
      }),
    });
  };

  /* ----- animaciones de una caja (fase Slides-Animación avanzada) ----- */

  /** Reescribe las cajas de la diapositiva activa (helper de las ops de anim). */
  const mapActiveBoxes = (fn: (boxes: SlideBox[]) => SlideBox[]): void => {
    const cur = stateRef.current;
    apply({
      slides: cur.model.slides.map((s, i) => (i === cur.active ? { ...s, boxes: fn(s.boxes) } : s)),
    });
  };

  /** Añade un efecto a la secuencia de una caja (al final, disparador 'click').
   *  La categoría se deriva del tipo; normaliza el orden global. */
  const addBoxAnim = (boxId: string, type: SlideAnimType): void => {
    mapActiveBoxes((boxes) => {
      const maxOrder = boxes.flatMap(boxAnims).reduce((m, a) => Math.max(m, a.order), 0);
      return normalizeAnimOrders(
        boxes.map((b) =>
          b.id === boxId
            ? withAnims(b, [...boxAnims(b), { type, order: maxOrder + 1, trigger: 'click' }])
            : b,
        ),
      );
    });
  };

  /** Cambia el efecto de la anim `index` de una caja (misma categoría o distinta). */
  const setAnimEffect = (boxId: string, index: number, type: SlideAnimType): void => {
    mapActiveBoxes((boxes) =>
      boxes.map((b) =>
        b.id === boxId
          ? withAnims(
              b,
              boxAnims(b).map((a, i) => (i === index ? { ...a, type } : a)),
            )
          : b,
      ),
    );
  };

  /** Cambia el disparador de la anim `index` de una caja. */
  const setAnimTrigger = (boxId: string, index: number, trigger: SlideAnimTrigger): void => {
    mapActiveBoxes((boxes) =>
      boxes.map((b) =>
        b.id === boxId
          ? withAnims(
              b,
              boxAnims(b).map((a, i) => (i === index ? { ...a, trigger } : a)),
            )
          : b,
      ),
    );
  };

  /** Quita la anim `index` de una caja; renormaliza el orden global. */
  const removeAnimAt = (boxId: string, index: number): void => {
    mapActiveBoxes((boxes) =>
      normalizeAnimOrders(
        boxes.map((b) =>
          b.id === boxId
            ? withAnims(
                b,
                boxAnims(b).filter((_, i) => i !== index),
              )
            : b,
        ),
      ),
    );
  };

  /** Sube/baja en la secuencia global la anim con ese `order` (panel de animaciones). */
  const moveAnim = (order: number, dir: -1 | 1): void => {
    mapActiveBoxes((boxes) => reorderAnimOrder(boxes, order, dir));
  };

  /* ----- edición de la trayectoria: arrastrar el marcador de FIN ----- */

  /** Arrastre en curso del marcador de fin de una trayectoria: `base` = sus vértices
   *  al empezar; el puntero mueve el ÚLTIMO. Al soltar la trayectoria se convierte
   *  en personalizada (el resto de vértices se conservan → una recta/curva/circuito
   *  se puede reorientar sin perder su forma).
   *  ponytail: solo el marcador de fin; editar vértices intermedios queda fuera. */
  const motionDragRef = useRef<{
    boxId: string;
    index: number;
    base: MotionPoint[];
    x0: number;
    y0: number;
  } | null>(null);

  /** Fija la trayectoria de la anim `index` como PERSONALIZADA con estos vértices.
   *  `persist=false` durante el arrastre (un único commit al soltar). */
  const setMotionPoints = (
    boxId: string,
    index: number,
    points: MotionPoint[],
    persist: boolean,
  ): void => {
    applyBoxes(
      (boxes) =>
        boxes.map((b) =>
          b.id === boxId
            ? withAnims(
                b,
                boxAnims(b).map((a, i) =>
                  i === index ? { ...a, type: 'motion-custom', points } : a,
                ),
              )
            : b,
        ),
      persist,
    );
  };

  const onMotionEndDown = (e: PointerEvent, boxId: string, index: number): void => {
    e.preventDefault();
    e.stopPropagation();
    const b = findEditBox(boxId);
    const a = b && boxAnims(b)[index];
    if (!a) return;
    motionDragRef.current = {
      boxId,
      index,
      base: motionPoints(a.type, a.points),
      x0: e.clientX,
      y0: e.clientY,
    };
    (e.currentTarget as Element).setPointerCapture?.(e.pointerId);
  };

  const onMotionEndMove = (e: PointerEvent): void => {
    const drag = motionDragRef.current;
    if (!drag) return;
    const s = stateRef.current.scale;
    const dx = ((e.clientX - drag.x0) / (STAGE_W * s)) * 100;
    const dy = ((e.clientY - drag.y0) / (stateRef.current.sh * s)) * 100;
    const n = drag.base.length - 1;
    const last = drag.base[n]!;
    const pts = drag.base.map((p, i) =>
      i === n
        ? { dx: clamp(last.dx + dx, -100, 100), dy: clamp(last.dy + dy, -100, 100) }
        : { ...p },
    );
    setMotionPoints(drag.boxId, drag.index, pts, false);
  };

  const onMotionEndUp = (): void => {
    if (!motionDragRef.current) return;
    motionDragRef.current = null;
    stateRef.current.ctx.onContentInput(JSON.stringify(stateRef.current.model));
  };

  /** Fija/quita el enlace de acción (clic en presentación → navegar) de una caja.
   *  '' = ninguno (se elimina el campo). */
  const setBoxAction = (boxId: string, action: SlideLinkTarget | ''): void => {
    const cur = stateRef.current;
    apply({
      slides: cur.model.slides.map((s, i) =>
        i === cur.active
          ? {
              ...s,
              boxes: s.boxes.map((b) => {
                if (b.id !== boxId) return b;
                if (action === '') {
                  const { action: _drop, ...rest } = b;
                  return rest;
                }
                return { ...b, action };
              }),
            }
          : s,
      ),
    });
  };

  /* ----- gráfico de la caja seleccionada (fase Slides-Gráfico) ----- */

  /** Fusiona un parche en el `chart` de una caja (tipo o rejilla). */
  const patchChart = (boxId: string, patch: Partial<SlideChart>): void => {
    const box = findEditBox(boxId);
    if (!box?.chart) return;
    patchBox(boxId, { chart: { ...box.chart, ...patch } }, true);
  };

  /** Escribe una celda de la rejilla del gráfico (rellenando huecos). */
  const setChartCell = (
    chart: SlideChart,
    boxId: string,
    r: number,
    c: number,
    value: string,
  ): void => {
    const grid = chart.grid.map((row) => [...row]);
    while (grid.length <= r) grid.push([]);
    const row = grid[r]!;
    while (row.length <= c) row.push('');
    row[c] = value;
    patchChart(boxId, { grid });
  };

  const addChartRow = (chart: SlideChart, boxId: string): void => {
    const cols = Math.max(
      2,
      chart.grid.reduce((m, r) => Math.max(m, r.length), 0),
    );
    patchChart(boxId, { grid: [...chart.grid.map((r) => [...r]), Array<string>(cols).fill('')] });
  };
  const addChartCol = (chart: SlideChart, boxId: string): void => {
    patchChart(boxId, { grid: chart.grid.map((r) => [...r, '']) });
  };
  const delChartRow = (chart: SlideChart, boxId: string): void => {
    if (chart.grid.length > 2)
      patchChart(boxId, { grid: chart.grid.slice(0, -1).map((r) => [...r]) });
  };
  const delChartCol = (chart: SlideChart, boxId: string): void => {
    const cols = chart.grid.reduce((m, r) => Math.max(m, r.length), 0);
    if (cols > 2) patchChart(boxId, { grid: chart.grid.map((r) => r.slice(0, cols - 1)) });
  };

  /* ----- diagrama SmartArt de la caja seleccionada (fase Slides-SmartArt) ----- */

  /** Fusiona un parche en el `smartart` de una caja (disposición o elementos). */
  const patchSmart = (boxId: string, patch: Partial<SlideSmartArt>): void => {
    const box = findEditBox(boxId);
    if (!box?.smartart) return;
    patchBox(boxId, { smartart: { ...box.smartart, ...patch } }, true);
  };

  const setSmartText = (smart: SlideSmartArt, boxId: string, i: number, text: string): void => {
    patchSmart(boxId, { items: smart.items.map((it, k) => (k === i ? { ...it, text } : it)) });
  };

  const addSmartItem = (smart: SlideSmartArt, boxId: string): void => {
    // El elemento nuevo hereda la sangría del último (enumerar hermanos es lo común).
    const level = smart.items[smart.items.length - 1]?.level ?? 0;
    const item: SmartArtItem = level > 0 ? { text: '', level } : { text: '' };
    patchSmart(boxId, { items: [...smart.items, item] });
  };

  const removeSmartItem = (smart: SlideSmartArt, boxId: string, i: number): void => {
    if (smart.items.length <= 1) return; // siempre queda al menos un elemento
    patchSmart(boxId, { items: smart.items.filter((_, k) => k !== i) });
  };

  const moveSmartItem = (smart: SlideSmartArt, boxId: string, i: number, dir: -1 | 1): void => {
    const j = i + dir;
    if (j < 0 || j >= smart.items.length) return;
    const items = [...smart.items];
    const tmp = items[i]!;
    items[i] = items[j]!;
    items[j] = tmp;
    patchSmart(boxId, { items });
  };

  /** Sangra/desangra (delta ±1) un elemento en la jerarquía, respetando la regla
   *  de esquema (no salta más de un nivel bajo el anterior; el 1.º es raíz). */
  const indentSmartItem = (smart: SlideSmartArt, boxId: string, i: number, delta: -1 | 1): void => {
    if (i === 0) return; // el primer elemento es la raíz: sin sangría
    const items = smart.items.map((it) => ({ ...it }));
    const prevLevel = items[i - 1]!.level ?? 0;
    const curLevel = items[i]!.level ?? 0;
    const next = clamp(curLevel + delta, 0, prevLevel + 1);
    if (next === curLevel) return;
    if (next === 0) delete items[i]!.level;
    else items[i]!.level = next;
    patchSmart(boxId, { items });
  };

  const addSmartArtBox = (layout: SmartArtLayout): void => {
    pushBox({
      id: crypto.randomUUID(),
      kind: 'smartart',
      x: 14,
      y: 12,
      w: 72,
      h: 72,
      html: '',
      smartart: { layout, items: defaultSmartArtItems(layout, t) },
    });
    setBottom('smartart');
  };

  /* ----- modo presentación ----- */

  const startPresentation = (presenter = false, cast = false): void => {
    if (cast) {
      setCastOpen(true);
      return;
    }
    setSelBox(null);
    setEditBox(null);
    setPresentMod(presenter);
    setPresenting(stateRef.current.active); // arranca en la diapositiva activa
  };

  /** El directo de Cast ya está programado: se abre su página para emitir y se presenta
   *  con la vista del moderador (las notas son del ponente y se quedan aquí). */
  const startCast = (videoId: string): void => {
    setCastOpen(false);
    const avisar = avisador(videoId, () =>
      notify({ tone: 'warning', title: t('slides.cast.lost') }),
    );
    setCastAvisar(() => (i: number) => {
      const p = posicionDe(stateRef.current.model.slides, i, ctx.doc.id);
      if (p) avisar(p);
    });
    // Sin `noopener` en la llamada: con él `window.open` devuelve siempre null y no se
    // sabría si el navegador la bloqueó.
    const ventana = window.open(
      urlDeEmision(videoId, orgActual(import.meta.env.BASE_URL)),
      '_blank',
    );
    if (ventana) ventana.opener = null;
    else notify({ tone: 'warning', title: t('slides.cast.popup') });
    startPresentation(true);
  };

  const exitPresentation = (): void => {
    setCastAvisar(null);
    setPresenting(null);
    // El overlay se lleva el foco al desmontarse: se devuelve al escenario.
    requestAnimationFrame(() => stageRef.current?.focus());
  };

  /** Aplica un diseño a la diapositiva actual conservando el texto por orden; imágenes y
   * formas sobreviven tal cual (mergeIntoLayout). La diapositiva guarda el id del diseño. */
  const applyLayout = (layoutId: string): void => {
    const cur = stateRef.current;
    if (cur.editingMaster) return; // la disposición aplica a diapositivas, no al patrón
    const src = cur.model.slides[cur.active];
    const def = layoutsOf(cur.model.master?.layouts, () => builtinLayouts(t)).find(
      (l) => l.id === layoutId,
    );
    if (!src || !def) return;
    // Renormaliza: el reflujo puede tirar cajas de texto animadas y dejar huecos.
    const boxes = normalizeAnimOrders(mergeIntoLayout(def, src.boxes, () => crypto.randomUUID()));
    apply({
      slides: cur.model.slides.map((s, i) =>
        i === cur.active ? { ...s, boxes, layout: def.id } : s,
      ),
    });
    setSelBox(null);
    setEditBox(null);
  };

  /* ----- diseños dentro del patrón y temas propios ----- */

  /** Fija la lista de diseños del patrón (y, con `slides`, las diapositivas que cambian). */
  const commitLayouts = (layouts: SlideLayout[], slides?: Slide[]): void => {
    const cur = stateRef.current;
    apply({
      master: { boxes: cur.model.master?.boxes ?? [], layouts },
      ...(slides ? { slides } : {}),
    });
  };

  const selectLayout = (id: string | null): void => {
    const cur = stateRef.current;
    // El primer diseño que se toca materializa la lista: de ahí en adelante es del modelo.
    if (id !== null && cur.model.master?.layouts === undefined) commitLayouts(layoutDefs);
    setEditingLayout(id);
    setSelBox(null);
    setEditBox(null);
  };

  const notifyTooManyLayouts = (): void => {
    notify({ tone: 'warning', title: t('slides.layout.tooMany', { max: MAX_LAYOUTS }) });
  };

  const newLayout = (): void => {
    const r = addLayout(layoutDefs, t('slides.layout.newName', { n: layoutDefs.length + 1 }), [
      newBox(8, 8, 84, 16, titleHtml(t)),
      newBox(8, 30, 84, 60, bodyHtml(t)),
    ]);
    if (!r) return notifyTooManyLayouts();
    commitLayouts(r.layouts);
    setEditingLayout(r.id);
    setSelBox(null);
    setEditBox(null);
  };

  const copyLayout = (): void => {
    const src = layoutDefs.find((l) => l.id === editingLayout);
    if (!src) return;
    const r = duplicateLayout(
      layoutDefs,
      src.id,
      t('slides.layout.copyName', { name: src.name }),
      () => crypto.randomUUID(),
    );
    if (!r) return notifyTooManyLayouts();
    commitLayouts(r.layouts);
    setEditingLayout(r.id);
    setSelBox(null);
    setEditBox(null);
  };

  const renameLayout = (name: string): void => {
    if (editingLayout) commitLayouts(patchLayout(layoutDefs, editingLayout, { name }));
  };

  const deleteLayout = async (): Promise<void> => {
    const cur = stateRef.current;
    const def = layoutDefs.find((l) => l.id === editingLayout);
    if (!def) return;
    if (
      !(await confirmarEnEditor({
        mensaje: editorT()('slides.layout.deleteConfirm', { name: def.name || def.id }),
        confirmLabel: editorT()('slides.layout.delete'),
        tone: 'danger',
      }))
    )
      return;
    // Las diapositivas que lo llevaban se quedan sin diseño (su contenido no se toca).
    const slides = cur.model.slides.map((s) => {
      if (s.layout !== def.id) return s;
      const { layout: _quitado, ...rest } = s;
      return rest;
    });
    commitLayouts(removeLayout(layoutDefs, def.id), slides);
    setEditingLayout(null);
    setSelBox(null);
    setEditBox(null);
  };

  /** Un diseño admite marcadores de texto, formas e imágenes (`decor`); gráficos, diagramas, medios y conectores son de las diapositivas. */
  const bloqueadoPorDiseno = (cmd: SlidesCommand): boolean => {
    const solo: SlidesCommand[] = ['grafico', 'smartart', 'media', 'conector'];
    if (!stateRef.current.editingLayout || !solo.includes(cmd)) return false;
    notify({ tone: 'warning', title: t('slides.layout.onlyText') });
    return true;
  };

  /** Los temas propios vigentes y el que usa la presentación, si es propio. */
  const customThemes = sanitizeCustomThemes(model.themes);
  const curCustomTheme = customThemes.find((x) => x.id === model.theme) ?? null;

  /** «Personalizar tema…»: parte del tema actual (el de serie o Papel) como tema propio nuevo
   *  y abre su panel; si el tema actual ya es propio, solo abre el panel. */
  const customizeTheme = (): void => {
    const cur = stateRef.current;
    const customs = sanitizeCustomThemes(cur.model.themes);
    if (customs.some((x) => x.id === cur.model.theme)) {
      setBottom('theme');
      return;
    }
    const base = SLIDE_THEMES.find((x) => x.id === cur.model.theme);
    const r = addCustomTheme(
      customs,
      {
        bg: base?.bg ?? '#FFFFFF',
        ink: base?.ink ?? INK,
        ...(base?.font ? { font: base.font } : {}),
      },
      t('slides.theme.newName', { n: customs.length + 1 }),
    );
    if (!r) {
      notify({ tone: 'warning', title: t('slides.theme.tooMany', { max: MAX_CUSTOM_THEMES }) });
      return;
    }
    apply({ themes: r.themes, theme: r.id });
    setBottom('theme');
  };

  const patchTheme = (id: string, patch: Partial<Omit<SlideCustomTheme, 'id'>>): void => {
    apply({
      themes: patchCustomTheme(sanitizeCustomThemes(stateRef.current.model.themes), id, patch),
    });
  };

  const deleteTheme = (id: string): void => {
    apply({
      themes: removeCustomTheme(sanitizeCustomThemes(stateRef.current.model.themes), id),
      theme: undefined,
    });
    setBottom(null);
  };

  /** Añade una caja a la diapositiva activa y la deja seleccionada. */
  const pushBox = (b: SlideBox): void => {
    applyBoxes((boxes) => [...boxes, b]);
    setSelBox(b.id);
    setEditBox(null);
  };

  const addTextBox = (): void => {
    pushBox(newBox(30, 40, 40, 14, `<p>${t('slides.textBox.sample')}</p>`));
  };

  /** Modelo resultante de fijar `boxes` en el objetivo de edición (patrón o
   *  diapositiva activa): se usa para el tope de tamaño antes de incrustar medios. */
  const modelWithBoxes = (boxes: SlideBox[]): SlidesModel => {
    const cur = stateRef.current;
    const m = cur.model.master;
    if (cur.editingMaster && cur.editingLayout)
      return {
        ...cur.model,
        master: {
          boxes: m?.boxes ?? [],
          layouts: (m?.layouts ?? []).map((l) =>
            l.id === cur.editingLayout ? applyLayoutBoxes(l, boxes) : l,
          ),
        },
      };
    return cur.editingMaster
      ? { ...cur.model, master: { ...m, boxes } }
      : {
          ...cur.model,
          slides: cur.model.slides.map((s, i) => (i === cur.active ? { ...s, boxes } : s)),
        };
  };

  /** Inserta la imagen (dataURL YA reescalado por Insertar.tsx) si el modelo
   * completo sigue cabiendo en el op-body; si no, toast honesto y no se toca. */
  const addImageBox = (src: string): void => {
    const img = new Image();
    img.onload = () => {
      const b: SlideBox = {
        id: crypto.randomUUID(),
        kind: 'image',
        ...imageBoxRect(img.naturalWidth, img.naturalHeight, stateRef.current.sh),
        html: '',
        src,
      };
      // Simula el modelo resultante (patrón o diapositiva activa) para el tope de tamaño.
      const boxes = [...editTargetBoxes(), b];
      if (JSON.stringify(modelWithBoxes(boxes)).length > MODEL_MAX_CHARS) {
        notify({
          tone: 'warning',
          title: t('slides.msg.imageTooBig'),
          description: t('slides.msg.imageTooBig.detail'),
        });
        return;
      }
      applyBoxes(() => boxes);
      setSelBox(b.id);
      setEditBox(null);
    };
    img.src = src;
  };

  /** Inserta un audio/vídeo (comando 'media': JSON ya saneado por Insertar.tsx y
   * revalidado aquí). Un fichero incrustado (dataURL) puede reventar el op-body:
   * se comprueba el modelo completo y, si no cabe, toast honesto y no se toca. */
  const addMediaBox = (payloadJson: string): void => {
    let spec: MediaSpec | null = null;
    try {
      spec = sanitizeMediaSpec(JSON.parse(payloadJson));
    } catch {
      spec = null;
    }
    if (!spec) {
      notify({
        tone: 'danger',
        title: t('slides.msg.mediaFailed'),
        description: t('slides.msg.mediaFailed.detail'),
      });
      return;
    }
    const isAudio = spec.mediaType === 'audio';
    // Audio: barra ancha abajo; vídeo: caja grande centrada (aspecto libre, el
    // usuario la ajusta). Solo se persisten las banderas que se apartan del defecto.
    const rect = isAudio ? { x: 18, y: 76, w: 64, h: 12 } : { x: 15, y: 12, w: 70, h: 68 };
    const b: SlideBox = {
      id: crypto.randomUUID(),
      kind: 'media',
      ...rect,
      html: '',
      src: spec.src,
      mediaType: spec.mediaType,
      ...(spec.poster ? { poster: spec.poster } : {}),
      ...(spec.autoplay ? { autoplay: true } : {}),
      ...(spec.loop ? { loop: true } : {}),
      ...(spec.controls ? {} : { controls: false }),
      ...(spec.tracks ? { tracks: spec.tracks } : {}),
    };
    const boxes = [...editTargetBoxes(), b];
    if (JSON.stringify(modelWithBoxes(boxes)).length > MODEL_MAX_CHARS) {
      notify({
        tone: 'warning',
        title: t('slides.msg.mediaTooBig'),
        description: t('slides.msg.mediaTooBig.detail'),
      });
      return;
    }
    applyBoxes(() => boxes);
    setSelBox(b.id);
    setEditBox(null);
  };

  const addShapeBox = (shape: SlideShape): void => {
    const linear = shape === 'line' || shape === 'arrow';
    pushBox({
      id: crypto.randomUUID(),
      kind: 'shape',
      shape,
      x: 35,
      y: linear ? 47 : 32,
      w: 30,
      h: linear ? 6 : 36,
      html: '',
      fill: SHAPE_FILL,
      stroke: SHAPE_STROKE,
    });
  };

  /** Inserta un gráfico (caja kind 'chart') con datos de arranque editables y
   *  abre el panel de datos. Su render reutiliza la lógica de sheets/Chart.tsx. */
  const addChartBox = (): void => {
    pushBox({
      id: crypto.randomUUID(),
      kind: 'chart',
      x: 20,
      y: 18,
      w: 60,
      h: 64,
      html: '',
      chart: { type: 'bar', grid: DEFAULT_CHART_GRID.map((r) => [...r]) },
    });
    setBottom('chart');
  };

  /** Borra el conjunto `ids` (una caja, una multiselección o un grupo entero).
   *  Renormaliza el orden de animación: quitar cajas no deja huecos. */
  const removeBoxes = (ids: readonly string[]): void => {
    if (ids.length === 0) return;
    const kill = new Set(ids);
    const cur = stateRef.current;
    if (cur.editingMaster) {
      applyBoxes((boxes) => normalizeAnimOrders(boxes.filter((b) => !kill.has(b.id))));
    } else {
      // Borra las cajas y, con ellas, sus conectores (podados en el mismo commit).
      apply({
        slides: cur.model.slides.map((s, i) => {
          if (i !== cur.active) return s;
          const boxes = normalizeAnimOrders(s.boxes.filter((b) => !kill.has(b.id)));
          const connectors = s.connectors ? pruneConnectors(s.connectors, kill) : [];
          const ns: Slide = { ...s, boxes };
          if (connectors.length > 0) ns.connectors = connectors;
          else delete ns.connectors;
          return ns;
        }),
      });
    }
    selectOnly(null);
    setEditBox(null);
  };

  /* ----- organizar objetos (fase Slides-Organizar) ----- */

  /** Desplaza el conjunto `ids` como bloque (grupo/multiselección) por dx/dy en %,
   *  topando su envolvente al lienzo para que no se salga ni se descuadre. */
  const nudgeSet = (ids: readonly string[], dx: number, dy: number): void => {
    if (ids.length === 0) return;
    const set = new Set(ids);
    const targets = editTargetBoxes().filter((b) => set.has(b.id));
    const bx = boundingBox(targets);
    const limites = calcularLimitesArrastre(bx);
    const {
      dx: cdx,
      dy: cdy,
      nuevasPosiciones,
    } = calcularArrastre(targets, dx, dy, limites.min, limites.max);
    if (cdx === 0 && cdy === 0) return;
    const at = new Map(nuevasPosiciones.map((b) => [b.id, b]));
    applyBoxes(
      (boxes) =>
        boxes.map((b) => (at.has(b.id) ? { ...b, x: at.get(b.id)!.x, y: at.get(b.id)!.y } : b)),
      true,
    );
  };

  /** Ejecuta un comando de organización sobre la selección EFECTIVA (grupos
   *  expandidos). La lógica pura vive en slides/organize.ts. */
  const organize = (op: string): void => {
    const ids = effSelIds();
    if (ids.length === 0) return;
    if (op === 'group') {
      // Agrupa la selección (mín. 2); el groupId nuevo lo genera el cliente.
      applyBoxes((boxes) => groupBoxes(boxes, ids, crypto.randomUUID()));
    } else if (op === 'ungroup') {
      applyBoxes((boxes) => ungroupBoxes(boxes, ids));
    } else if (Z_OPS[op]) {
      applyBoxes((boxes) => reorderZ(boxes, ids, Z_OPS[op]!));
    } else if (ALIGN_OPS[op]) {
      // Con ≥2 cajas, respecto al conjunto; con 1, respecto al lienzo.
      const ref: AlignRef = ids.length >= 2 ? 'selection' : 'canvas';
      applyBoxes((boxes) => alignBoxes(boxes, ids, ALIGN_OPS[op]!, ref));
    } else if (op === 'dist-h') {
      applyBoxes((boxes) => distributeBoxes(boxes, ids, 'h'));
    } else if (op === 'dist-v') {
      applyBoxes((boxes) => distributeBoxes(boxes, ids, 'v'));
    }
  };

  /* Publica el resumen de la selección para los comandos «Organizar» del ribbon. */
  useEffect(() => {
    const idSet = new Set(expandGroups(editBoxes, selBox ? [selBox, ...extraSel] : []));
    publicarSeleccion({
      count: idSet.size,
      hasGroup: editBoxes.some((b) => idSet.has(b.id) && b.groupId !== undefined),
    });
  }, [selBox, extraSel, model, editingMaster, active, editBoxes]);
  useEffect(
    () => () => {
      publicarSeleccion({ count: 0, hasGroup: false });
      slidesSizeSnapshot = undefined;
    },
    [],
  );

  /* Publica el tamaño del lienzo para el control «Tamaño de diapositiva» del ribbon. */
  useEffect(() => {
    slidesSizeSnapshot = model.size;
  }, [model.size]);

  /* Y los diseños vigentes, para el selector «Disposición» del ribbon. */
  useEffect(() => {
    publicarDisenos(layoutDefs.map((l) => ({ id: l.id, name: l.name })));
    return () => publicarDisenos([]);
  }, [layoutDefs]);

  /* ----- registro de comandos de la pestaña InicioSlides ----- */

  // El ribbon conserva un único handler global. La ref evita reinstalarlo en
  // cada render y garantiza que cada comando use las acciones vigentes.
  const commandActionsRef = useUltimo({
    addChartBox,
    addConnector,
    addImageBox,
    addMediaBox,
    addShapeBox,
    addSlide,
    iaFill,
    insertarBloque,
    addSmartArtBox,
    applyLayout,
    bloqueadoPorDiseno,
    enterMaster,
    organize,
    setSlideSize,
    startPresentation,
  });

  useEffect(() => {
    const run = (cmd: SlidesCommand, value?: string): void => {
      const {
        addChartBox,
        addConnector,
        addImageBox,
        addMediaBox,
        addShapeBox,
        addSlide,
        iaFill,
        insertarBloque,
        addSmartArtBox,
        applyLayout,
        bloqueadoPorDiseno,
        enterMaster,
        organize,
        setSlideSize,
        startPresentation,
      } = commandActionsRef.current;
      if (bloqueadoPorDiseno(cmd)) return;
      if (cmd === 'nueva') addSlide();
      else if (cmd === 'iafill' && value) iaFill(value);
      else if (cmd === 'bloque' && value) insertarBloque(value);
      else if (cmd === 'patron') enterMaster();
      else if (cmd === 'tamano') setSlideSize(value);
      else if (cmd === 'presentar') startPresentation(value === 'moderador', value === 'cast');
      else if (cmd === 'grafico') addChartBox();
      else if (cmd === 'imagen' && value) addImageBox(value);
      else if (cmd === 'media' && value) addMediaBox(value);
      else if (cmd === 'forma' && SLIDE_SHAPES.some((s) => s.id === value))
        addShapeBox(value as SlideShape);
      else if (cmd === 'smartart' && value && SMARTART_IDS.has(value as SmartArtLayout))
        addSmartArtBox(value as SmartArtLayout);
      else if (cmd === 'organizar' && value) organize(value);
      else if (cmd === 'conector') addConnector(value ?? 'straight');
      else if (cmd === 'disposicion' && value) applyLayout(value);
    };
    slidesCommandHandler = run;
    return () => {
      if (slidesCommandHandler === run) slidesCommandHandler = null;
    };
  }, [commandActionsRef]);

  /* ----- medición del hueco del escenario ----- */

  useEffect(() => {
    const el = areaRef.current;
    if (!el) return;
    const measure = (): void => setArea({ w: el.clientWidth, h: el.clientHeight });
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  /* ----- carga por época (contrato §7): parse inválido → modelo vacío + toast ----- */

  const loadRef = useUltimo({ content: ctx.content, notify });

  useEffect(() => {
    const latest = loadRef.current;
    let next = emptySlidesModel(t);
    if (latest.content) {
      try {
        const parsed = JSON.parse(latest.content) as SlidesModel | null;
        originalRef.current = parsed && Array.isArray(parsed.slides) ? parsed : null;
        if (
          parsed &&
          Array.isArray(parsed.slides) &&
          parsed.slides.length > 0 &&
          parsed.slides.every((s) => s && Array.isArray(s.boxes))
        ) {
          // Migra/sanea las animaciones (viejo `anim` único → secuencia `anims`,
          // orden global renumerado) y los conectores (tipo/anclas válidas) al
          // cargar; el resto del código solo ve datos ya saneados.
          next = {
            ...parsed,
            slides: parsed.slides.map((s) => {
              const migrated = migrateSlide(s);
              const conns = sanitizeConnectors(
                s.connectors,
                new Set(migrated.boxes.map((b) => b.id)),
              );
              const { connectors: _raw, ...rest } = migrated;
              return conns.length > 0 ? { ...rest, connectors: conns } : rest;
            }),
          };
          const prot = sanitizeSlidesProtection(parsed.protect);
          if (prot) next.protect = prot;
          else delete next.protect;
          // Saneado del patrón: sin cajas válidas se descarta (doc viejo/corrupto).
          if (next.master && !Array.isArray(next.master.boxes)) delete next.master;
          // Temas propios y diseños: solo lo bien formado (frontera de confianza).
          const themes = sanitizeCustomThemes(next.themes);
          if (themes.length > 0) next.themes = themes;
          else delete next.themes;
          if (
            next.theme &&
            next.theme.startsWith('custom-') &&
            !themes.some((x) => x.id === next.theme)
          )
            delete next.theme; // un tema propio que ya no existe
          if (next.master && next.master.layouts !== undefined)
            next.master = { ...next.master, layouts: sanitizeLayouts(next.master.layouts) };
          // Saneado del tamaño: preset válido o custom con dimensiones; si no, 16:9.
          const cleanSize = sanitizeSlideSize(next.size);
          if (cleanSize) next.size = cleanSize;
          else delete next.size;
        } else {
          latest.notify({
            tone: 'warning',
            title: t('slides.msg.readFailed'),
            description: t('slides.msg.readFailed.detail'),
          });
        }
      } catch {
        latest.notify({
          tone: 'warning',
          title: t('slides.msg.readFailed'),
          description: t('slides.msg.readFailed.detail'),
        });
      }
    }
    setModel(next);
    setActive(0);
    setSelBox(null);
    setSelConn(null);
    setEditBox(null);
    setEditingMaster(false);
    setEditingLayout(null);
    // `t` entra por el modelo vacío, que ahora nombra la primera diapositiva en
    // el idioma de la sesión.
  }, [ctx.contentEpoch, t, loadRef]);

  /* ----- estadísticas (§7.2) y limpieza del root de exec ----- */

  const reportStats = ctx.reportStats;
  useEffect(() => {
    reportStats({ pages: model.slides.length, page: active + 1 });
  }, [model.slides.length, active, reportStats]);

  useEffect(() => () => setEditorRoot(null), []);

  /** Miniatura en arrastre (reordenar con drag and drop nativo, como Sheets). */
  const slideDrag = useRef<number | null>(null);

  /* ----- arrastre de asas (Pointer Events con setPointerCapture, §7.2) ----- */

  const dragRef = useRef<{
    boxId: string; // caja bajo el puntero
    mode: 'mover' | 'redimensionar';
    x0: number;
    y0: number;
    /** Cajas que se mueven (grupo/multiselección); al redimensionar, solo `boxId`. */
    origs: { id: string; x: number; y: number; w: number; h: number }[];
    /** Topes del desplazamiento (mover): la envolvente del conjunto no sale del lienzo. */
    min: { x: number; y: number };
    max: { x: number; y: number };
  } | null>(null);

  const snapRect = (b: SlideBox): { id: string; x: number; y: number; w: number; h: number } => ({
    id: b.id,
    x: b.x,
    y: b.y,
    w: b.w,
    h: b.h,
  });

  const onHandleDown = (e: PointerEvent, box: SlideBox, mode: 'mover' | 'redimensionar'): void => {
    e.preventDefault();
    e.stopPropagation();
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    if (mode === 'redimensionar') {
      // Redimensionar afecta solo a la caja bajo el puntero (sin escalado de grupo).
      dragRef.current = {
        boxId: box.id,
        mode,
        x0: e.clientX,
        y0: e.clientY,
        origs: [snapRect(box)],
        min: { x: 0, y: 0 },
        max: { x: 0, y: 0 },
      };
      return;
    }
    // Mover: conjunto efectivo (multiselección/grupo si la caja pertenece), topado por su envolvente.
    const boxesNow = editTargetBoxes();
    const base = isSelected(box.id) ? explicitSel() : [box.id];
    const moving = new Set(expandGroups(boxesNow, base));
    moving.add(box.id); // la caja bajo el puntero siempre se mueve
    const origs = boxesNow.filter((b) => moving.has(b.id)).map(snapRect);
    const bx = boundingBox(origs);
    const limites = calcularLimitesArrastre(bx);
    dragRef.current = {
      boxId: box.id,
      mode,
      x0: e.clientX,
      y0: e.clientY,
      origs,
      min: limites.min,
      max: limites.max,
    };
  };

  const onHandleMove = (e: PointerEvent): void => {
    const drag = dragRef.current;
    if (!drag) return;
    const s = stateRef.current.scale;
    const dxRaw = ((e.clientX - drag.x0) / (STAGE_W * s)) * 100;
    const dyRaw = ((e.clientY - drag.y0) / (stateRef.current.sh * s)) * 100;
    if (drag.mode === 'mover') {
      // Un solo delta topado ⇒ el conjunto conserva sus posiciones relativas.
      const { nuevasPosiciones } = calcularArrastre(drag.origs, dxRaw, dyRaw, drag.min, drag.max);
      const at = new Map(nuevasPosiciones.map((o) => [o.id, { x: o.x, y: o.y }]));
      applyBoxes((boxes) => boxes.map((b) => ({ ...b, ...at.get(b.id) })), false);
    } else {
      const o = drag.origs[0]!;
      const dim = calcularRedimension(o, dxRaw, dyRaw, 4, 100);
      applyBoxes(
        (boxes) => boxes.map((b) => (b.id === drag.boxId ? { ...b, w: dim.w, h: dim.h } : b)),
        false,
      );
    }
  };

  const onHandleUp = (): void => {
    if (!dragRef.current) return;
    dragRef.current = null;
    // Un único commit al soltar (el autosave ya viene con debounce).
    stateRef.current.ctx.onContentInput(JSON.stringify(stateRef.current.model));
  };

  /* ----- edición de texto en cajas ----- */

  const boxEl = (boxId: string): HTMLElement | null =>
    stageRef.current?.querySelector<HTMLElement>(`[data-slidebox="${boxId}"]`) ?? null;

  const startEditBox = (boxId: string): void => {
    setSelBox(boxId);
    setEditBox(boxId);
    const el = boxEl(boxId);
    if (!el) return;
    el.focus();
    const range = document.createRange();
    range.selectNodeContents(el);
    range.collapse(false); // caret al final
    const s = window.getSelection();
    s?.removeAllRanges();
    s?.addRange(range);
  };

  /* ponytail: el texto se consolida al salir de la caja (blur/Escape), no por
   * tecla: reescribir __html por tecla mataría el caret. El autosave llega con
   * el commit y las miniaturas se refrescan entonces. */
  const commitBoxHtml = (boxId: string, el: HTMLElement): void => {
    const b = findEditBox(boxId);
    if (!b || b.html === el.innerHTML) return;
    patchBox(boxId, { html: el.innerHTML }, true);
  };

  const onStageFocusIn = (e: FocusEvent): void => {
    const el = e.target as HTMLElement;
    const id = el.dataset['slidebox'];
    if (!id) return;
    setEditorRoot(el); // Inicio/exec formatean texto dentro de la caja (§7)
    setSelBox(id);
    setEditBox(id);
  };

  const onStageFocusOut = (e: FocusEvent): void => {
    const el = e.target as HTMLElement;
    const id = el.dataset['slidebox'];
    if (!id) return;
    commitBoxHtml(id, el);
    setEditBox((prev) => (prev === id ? null : prev));
  };

  const onStageKeyDown = (e: KeyboardEvent): void => {
    const target = e.target as HTMLElement;
    const boxId = target.dataset['slidebox'];
    if (boxId) {
      // Edición de texto dentro de la caja.
      if (e.key === 'Escape') {
        e.preventDefault();
        commitBoxHtml(boxId, target);
        setEditBox(null);
        stageRef.current?.focus();
      } else {
        // Texto de la caja: sus atajos son los de `exec` (registro de comandos).
        const atajo = comandoDelAtajo(e, 'document');
        if (atajo) {
          e.preventDefault();
          ctx.exec(atajo.id as EditorCommand, atajo.value);
        }
      }
      return;
    }
    // Conector seleccionado: Supr lo borra, Esc lo deselecciona (prioridad sobre
    // las cajas, que no pueden estar seleccionadas a la vez que un conector).
    if (selConn) {
      if (e.key === 'Delete' || e.key === 'Backspace') {
        e.preventDefault();
        deleteConnector(selConn);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setSelConn(null);
        return;
      }
    }
    // Teclado sobre el escenario o sobre el marco focalizable de una imagen/forma.
    if (!selBox || (target !== stageRef.current && target.dataset['boxframe'] === undefined))
      return;
    const b = editBoxes.find((x) => x.id === selBox);
    if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault();
      removeBoxes(effSelIds()); // Supr borra la selección efectiva (caja/grupo/multiselección)
    } else if ((e.key === 'Enter' || e.key === 'F2') && b && (!b.kind || b.kind === 'text')) {
      e.preventDefault();
      startEditBox(selBox); // solo texto: imagen/forma no tienen modo edición
    } else if (e.key === 'Escape') {
      e.preventDefault();
      selectOnly(null);
    } else if (b && e.key.startsWith('Arrow')) {
      e.preventDefault();
      const dx = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0;
      const dy = e.key === 'ArrowUp' ? -1 : e.key === 'ArrowDown' ? 1 : 0;
      if (e.shiftKey) {
        // Redimensiona solo el primario (sin escalado de grupo, como el arrastre).
        const dim = calcularRedimension(b, dx, dy, 4, 100);
        patchBox(selBox, { w: dim.w, h: dim.h }, true);
      } else {
        nudgeSet(effSelIds(), dx, dy); // mueve la selección efectiva como bloque
      }
    }
  };

  /* ----- render ----- */

  const selBoxObj = selBox ? (editBoxes.find((b) => b.id === selBox) ?? null) : null;
  /** Conector seleccionado (mini-barra: tipo/puntas/color/eliminar). */
  const selConnObj = selConn
    ? ((slide.connectors ?? []).find((c) => c.id === selConn) ?? null)
    : null;
  const selChart = selBoxObj && selBoxObj.kind === 'chart' && selBoxObj.chart ? selBoxObj : null;
  const selSmart =
    selBoxObj && selBoxObj.kind === 'smartart' && selBoxObj.smartart ? selBoxObj : null;
  /** Plan de animación de la diapositiva activa: la secuencia global (panel) y el
   *  primer paso de cada caja (distintivo del escenario). Vacío al editar el patrón. */
  const activePlan = editingMaster ? [] : slidePlan(slide);
  const animByBox = planByBox(activePlan);
  /** Caja por id de la diapositiva activa (rótulos del panel de animaciones). */
  const boxById = new Map(slide.boxes.map((b) => [b.id, b] as const));
  /** Ids de la selección EFECTIVA (multiselección + grupos): pintan el marco de
   *  todas las cajas seleccionadas; los tiradores solo van en el primario. */
  const selectedIds = new Set(expandGroups(editBoxes, selBox ? [selBox, ...extraSel] : []));
  /** Clic y doble clic de una caja de TEXTO. Estables y fuera del mapa: el objeto que
   *  recibe `renderBox` se arma en el render, y unas closures que tocan refs pasadas a
   *  una función en mitad del render son justo lo que prohíbe `react(refs)`. Las
   *  acciones van por una ref con su última versión, como `commandActionsRef`. */
  const accionesTextoRef = useUltimo({ toggleSel, selectOnly, startEditBox });
  const alPulsarTexto = useCallback(
    (e: MouseEvent, id: string, editando: boolean): void => {
      if (editando) return; // en edición: caret y selección nativos
      e.preventDefault(); // clic = seleccionar; editar llega con doble clic
      const { toggleSel, selectOnly } = accionesTextoRef.current;
      if (e.shiftKey)
        toggleSel(id); // Mayús+clic: multiselección
      else selectOnly(id);
      stageRef.current?.focus();
    },
    [accionesTextoRef],
  );
  const alDobleClicTexto = useCallback(
    (id: string): void => accionesTextoRef.current.startEditBox(id),
    [accionesTextoRef],
  );
  /** Trayectorias de movimiento a dibujar en el editor: las de las cajas
   *  SELECCIONADAS (evita saturar el lienzo). Cada una lleva su caja + índice de
   *  anim para poder editar el marcador de fin. No aplica al patrón. */
  const motionOverlays = editingMaster
    ? []
    : editBoxes.flatMap((b) =>
        selectedIds.has(b.id)
          ? boxAnims(b).flatMap((a, index) =>
              ANIM_CATEGORY[a.type] === 'motion' ? [{ box: b, index, anim: a }] : [],
            )
          : [],
      );
  /** Fondo/tinta/fuente efectivos de la diapositiva activa (tema + fondo propio). */
  const stageVis = slideVisuals(model, slide);
  /** Opciones del enlace de acción de la caja: relativos + una por diapositiva. */
  const actionOptions = [
    { value: '', label: t('slides.link.none') },
    { value: 'next', label: t('nav.nextItem') },
    { value: 'prev', label: t('nav.previous') },
    { value: 'first', label: t('slides.link.first') },
    { value: 'last', label: t('slides.link.last') },
    ...model.slides.map((_, i) => ({
      value: String(i + 1),
      label: t('slides.number', { n: i + 1 }),
    })),
  ];

  return (
    <div class="flex h-full min-h-0 flex-1 max-sm:flex-col">
      <style>{BOX_CSS}</style>

      {/* Carril de miniaturas (§7.2): 16:9 escaladas 128/960, activa con anillo de acento.

          EN PÍXELES Y NO EN `rem`, igual que la miniatura: lo que va dentro se
          escala a THUMB_W píxeles, así que medir su caja en rem la ata al tamaño
          de letra de la raíz. Con la raíz en 14 —un ajuste de accesibilidad
          corriente, y el que tiene la suite— `w-40` daba 139px y `w-32` 112,
          mientras el contenido seguía midiendo 128: cada miniatura recortaba
          16px del lado derecho de su diapositiva, medido en las tres ventanas de
          escritorio. Con la raíz en 16 esto vale exactamente lo mismo que antes.

          EN UN TELÉFONO, TIRA HORIZONTAL ARRIBA (`max-sm:`). En columna se comía la
          mitad de la pantalla: en un iPhone SE quedaban 80px para el lienzo, la
          diapositiva no pasaba de 77px y aun así se salía por la derecha —el barrido
          móvil lo marcaba—. Ocultarla no valía: es la ÚNICA forma de cambiar de
          diapositiva. Medido en producción con este CSS: iPhone SE, lienzo 80→240px y
          diapositiva 77→176px; iPhone 12, 150→310 y 86→246. El ancho va en una
          variable y no en `style.width` para que la variante estrecha pueda soltarlo:
          un ancho en línea no lo pisa ninguna clase. */}
      <div
        class="border-border bg-surface flex w-[var(--carril-diapos-w)] shrink-0 flex-col border-r max-sm:w-auto max-sm:flex-row max-sm:border-r-0 max-sm:border-b"
        style={{ '--carril-diapos-w': `${THUMB_W + 32}px` }}
      >
        <div
          data-scroll-x
          class="flex min-h-0 min-w-0 flex-1 flex-col items-center gap-2 overflow-y-auto p-2 max-sm:flex-row max-sm:overflow-x-auto max-sm:overflow-y-hidden"
        >
          {model.slides.map((s, i) => {
            const vis = slideVisuals(model, s);
            return (
              <button
                key={s.id}
                type="button"
                class={`relative shrink-0 overflow-hidden rounded-sm ${FOCUS_RING}`}
                style={{
                  ...visualsStyle(vis),
                  // Ancho y alto en las MISMAS unidades que la escala de dentro:
                  // ver el comentario del carril.
                  width: `${THUMB_W}px`,
                  // Alto por proporción del lienzo (16:9 → 72px como el h-18 clásico).
                  height: `${(THUMB_W * sh) / STAGE_W}px`,
                  boxShadow: i === active ? `0 0 0 2px ${ACCENT}` : '0 0 0 1px var(--sytue-border)',
                }}
                aria-label={t('slides.aria.slideOf', { n: i + 1, total: model.slides.length })}
                aria-current={i === active ? 'true' : undefined}
                title={t('slides.aria.dragHint', { n: i + 1 })}
                draggable
                onDragStart={(e) => {
                  slideDrag.current = i;
                  // setData: sin él, Firefox no inicia el arrastre.
                  e.dataTransfer?.setData('text/plain', String(i + 1));
                  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
                }}
                onDragOver={(e) => {
                  if (slideDrag.current !== null) e.preventDefault(); // acepta soltar aquí
                }}
                onDrop={(e) => {
                  e.preventDefault();
                  const from = slideDrag.current;
                  slideDrag.current = null;
                  if (from !== null && from !== i) moveSlideTo(from, i);
                }}
                onDragEnd={() => {
                  slideDrag.current = null;
                }}
                /* Reordenar sin ratón: el arrastre del carril no existe para quien
                 * navega con teclado, y el orden de las diapositivas no es un
                 * adorno. Mismo atajo que PowerPoint y Google Slides. */
                onKeyDown={(e) => {
                  if (!(e.ctrlKey || e.metaKey) || !e.shiftKey) return;
                  const to = e.key === 'ArrowUp' ? i - 1 : e.key === 'ArrowDown' ? i + 1 : -1;
                  if (to < 0 || to >= model.slides.length) return;
                  e.preventDefault();
                  moveSlideTo(i, to);
                }}
                onClick={() => {
                  setActive(i);
                  setSelBox(null);
                  setEditBox(null);
                }}
              >
                <div
                  aria-hidden="true"
                  class="pointer-events-none absolute top-0 left-0 origin-top-left"
                  style={{
                    width: `${STAGE_W}px`,
                    height: `${sh}px`,
                    transform: `scale(${THUMB_W / STAGE_W})`,
                    color: vis.ink,
                  }}
                >
                  {masterBoxesFor(model, i).map((b) => (
                    <div key={`m-${b.id}`} class="absolute" style={boxPos(b)}>
                      {renderBox(t, b, sh)}
                    </div>
                  ))}
                  <ConnectorLayer connectors={layoutConnectorsFor(model, i)} boxes={[]} sh={sh} />
                  {s.boxes.map((b) => (
                    <div key={b.id} class="absolute" style={boxPos(b)}>
                      {renderBox(t, b, sh)}
                    </div>
                  ))}
                  <ConnectorLayer connectors={s.connectors ?? []} boxes={s.boxes} sh={sh} />
                </div>
                <span class="bg-bg-subtle text-text-muted text-2xs absolute right-1 bottom-0.5 rounded-sm px-1 leading-4">
                  {i + 1}
                </span>
              </button>
            );
          })}
        </div>
        <div class="border-border flex shrink-0 items-center justify-center gap-1 border-t p-1 max-sm:border-t-0 max-sm:border-l">
          <RibbonButton icon="plus" label={t('slides.new')} onAction={() => addSlide()} />
          <RibbonButton icon="slideAdd" label={t('slides.duplicate')} onAction={duplicateSlide} />
          <RibbonButton
            icon="trash"
            label={t('slides.delete')}
            disabled={model.slides.length <= 1}
            onAction={() => void deleteSlide()}
          />
        </div>
      </div>

      {/* Zona del escenario. */}
      <div class="flex min-h-0 min-w-0 flex-1 flex-col">
        {/* Mini-barra sobre el escenario (§7.2). Con una forma seleccionada
         * aparecen sus muestras de relleno/borde (paleta de contenido, §5). */}
        {/* `data-scroll-x`: la mini-barra se desplaza de lado A PROPÓSITO. Sus
            controles suman ~483px fijos —tema, fondo, transición, patrón…—, así
            que no caben en ningún teléfono: medido +55px en el iPhone 14 Pro Max
            (428), +75 en el iPhone 12 (390). Encogerlos más los dejaría sin
            rótulo; que se deslice es la decisión, y esto la declara para que el
            barrido no la cuente como defecto de maquetación.
            Para que de verdad se deslice: `*:shrink-0`, porque sin él los botones se
            aplastaban a 18 px de ancho y los desplegables se quedaban en «F» y «Si»
            (medido en producción a 390, 2026-09-21); y `justify-center-safe`, porque
            un `justify-center` que desborda reparte el sobrante a los DOS lados y lo
            de la izquierda queda fuera, donde el desplazamiento no llega. */}
        <div
          data-scroll-x
          class="border-border bg-surface flex h-9 shrink-0 items-center justify-center-safe gap-1 overflow-x-auto border-b px-1 *:shrink-0 pointer-coarse:h-auto pointer-coarse:min-h-[max(44px,2.75rem)]"
        >
          <RibbonButton icon="textBox" label={t('slides.textBox')} onAction={addTextBox} />
          {!editingLayout && (
            <RibbonButton icon="chart" label={t('data.chart.insert')} onAction={addChartBox} />
          )}
          {editingMaster && !editingLayout && (
            <RibbonButton
              icon="pageNumber"
              label={t('slides.insertNumber')}
              onAction={addPageNumberBox}
            />
          )}
          <div aria-hidden="true" class="bg-border mx-1 h-5 w-px" />
          {!editingMaster && (
            <>
              <RibbonButton
                icon="comment"
                label={t('slides.notes')}
                active={bottom === 'notes'}
                onAction={() => setBottom((b) => (b === 'notes' ? null : 'notes'))}
              />
              <RibbonButton
                icon="arrow"
                label={t('slides.animations.hint')}
                active={bottom === 'anim'}
                onAction={() => setBottom((b) => (b === 'anim' ? null : 'anim'))}
              />
              <RibbonButton
                icon="lock"
                label={t('slides.protect')}
                active={bottom === 'protect'}
                onAction={() => setBottom((b) => (b === 'protect' ? null : 'protect'))}
              />
              <div aria-hidden="true" class="bg-border mx-1 h-5 w-px" />
            </>
          )}
          {/* 172 y no 128: con 128, «Papel (predeterminado)» —lo que enseña casi siempre— se
              cortaba en «Papel (predeterm» (medido el 2026-10-16). */}
          <RibbonSelect
            label={t('slides.theme')}
            width={172}
            value={model.theme ?? ''}
            options={[
              ...THEME_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) })),
              ...customThemes.map((x) => ({ value: x.id, label: x.name || x.id })),
              { value: CUSTOM_THEME_VALUE, label: t('slides.theme.custom') },
            ]}
            onSelect={setTheme}
          />
          {!editingMaster && (
            <>
              <RibbonColorSwatch
                icon="image"
                label={t('slides.background')}
                color={slide.bg ?? 'transparent'}
                palette={SLIDE_BG_PALETTE}
                onPick={setSlideBg}
              />
              <RibbonSelect
                label={t('slides.transition')}
                width={118}
                value={slide.transition ?? 'none'}
                options={TRANSITION_OPTIONS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
                onSelect={(v) =>
                  setTransition(v === 'fade' || v === 'slide' || v === 'morph' ? v : 'none')
                }
              />
              {model.master && (
                <RibbonButton
                  icon="slideMaster"
                  label={t('slides.hideMaster')}
                  active={slide.hideMaster === true}
                  onAction={toggleHideMaster}
                />
              )}
            </>
          )}
          {/* Con una forma seleccionada: sus muestras de relleno/borde (§5). */}
          {selBoxObj?.kind === 'shape' && (
            <>
              <div aria-hidden="true" class="bg-border mx-1 h-5 w-px" />
              {selBoxObj.shape !== 'line' && selBoxObj.shape !== 'arrow' && (
                <RibbonColorSwatch
                  icon="rect"
                  label={t('shape.fill')}
                  color={selBoxObj.fill ?? SHAPE_FILL}
                  palette={SHAPE_FILL_PALETTE}
                  onPick={(c) => patchBox(selBoxObj.id, { fill: c }, true)}
                />
              )}
              <RibbonColorSwatch
                icon="line"
                label={t('shape.outline')}
                color={selBoxObj.stroke ?? SHAPE_STROKE}
                onPick={(c) => patchBox(selBoxObj.id, { stroke: c }, true)}
              />
            </>
          )}
          {/* Con un vídeo o un audio seleccionado: sus subtítulos. */}
          {selBoxObj?.kind === 'media' && !editingMaster && (
            <>
              <div aria-hidden="true" class="bg-border mx-1 h-5 w-px" />
              <RibbonButton
                icon="caption"
                label={t('slides.subs')}
                active={bottom === 'subs'}
                onAction={() => setBottom((b) => (b === 'subs' ? null : 'subs'))}
              />
            </>
          )}
          {/* Con un gráfico seleccionado: tipo + abrir el editor de datos. */}
          {selChart && (
            <>
              <div aria-hidden="true" class="bg-border mx-1 h-5 w-px" />
              <RibbonSelect
                label={t('chart.type')}
                width={92}
                value={selChart.chart!.type}
                options={chartTypeOptions(t)}
                onSelect={(v) => {
                  if (CHART_TYPES.has(v as SlideChartType))
                    patchChart(selChart.id, { type: v as SlideChartType });
                }}
              />
              <RibbonButton
                icon="table"
                label={t('chart.editData')}
                active={bottom === 'chart'}
                onAction={() => setBottom((b) => (b === 'chart' ? null : 'chart'))}
              />
            </>
          )}
          {/* Con un diagrama SmartArt seleccionado: disposición + editor de elementos. */}
          {selSmart && (
            <>
              <div aria-hidden="true" class="bg-border mx-1 h-5 w-px" />
              <RibbonSelect
                label={t('smartart.layout')}
                width={104}
                value={selSmart.smartart!.layout}
                options={smartArtLayoutOptions(t)}
                onSelect={(v) => {
                  if (SMARTART_IDS.has(v as SmartArtLayout))
                    patchSmart(selSmart.id, { layout: v as SmartArtLayout });
                }}
              />
              <RibbonButton
                icon="outline"
                label={t('smartart.editItems')}
                active={bottom === 'smartart'}
                onAction={() => setBottom((b) => (b === 'smartart' ? null : 'smartart'))}
              />
            </>
          )}
          {/* Con una caja seleccionada (solo diapositiva): su enlace de acción. La
           *  secuencia de animación (efecto/categoría/orden/disparador) se edita en
           *  el panel «Animaciones». */}
          {selBoxObj && !editingMaster && (
            <>
              <div aria-hidden="true" class="bg-border mx-1 h-5 w-px" />
              {/* El botón de animaciones vive tres iconos a la izquierda y es el
                  MISMO conmutador: repetirlo aquí solo daba dos botones idénticos
                  en la misma barra. */}
              <RibbonSelect
                label={t('slides.onClick')}
                width={148}
                value={
                  selBoxObj.action === undefined
                    ? ''
                    : typeof selBoxObj.action === 'number'
                      ? String(selBoxObj.action)
                      : selBoxObj.action
                }
                options={actionOptions}
                onSelect={(v) => {
                  if (v === '') setBoxAction(selBoxObj.id, '');
                  else if (v === 'first' || v === 'last' || v === 'next' || v === 'prev')
                    setBoxAction(selBoxObj.id, v);
                  else {
                    const n = Number(v);
                    if (Number.isInteger(n) && n >= 1) setBoxAction(selBoxObj.id, n);
                  }
                }}
              />
            </>
          )}
          {/* Con un conector seleccionado: tipo de trazado, puntas de flecha,
           *  color y eliminar. */}
          {selConnObj && !editingMaster && (
            <>
              <div aria-hidden="true" class="bg-border mx-1 h-5 w-px" />
              <RibbonSelect
                label={t('connector.type')}
                width={104}
                value={selConnObj.kind}
                options={connectorTypeOptions(t)}
                onSelect={(v) => {
                  if (CONNECTOR_KIND_SET.has(v as ConnectorKind))
                    patchConnector(selConnObj.id, { kind: v as ConnectorKind });
                }}
              />
              <RibbonButton
                icon="connStraight"
                label={t('connector.arrowStart')}
                active={selConnObj.startArrow === true}
                onAction={() =>
                  patchConnector(selConnObj.id, { startArrow: selConnObj.startArrow !== true })
                }
              />
              <RibbonButton
                icon="arrow"
                label={t('connector.arrowEnd')}
                active={selConnObj.endArrow === true}
                onAction={() =>
                  patchConnector(selConnObj.id, { endArrow: selConnObj.endArrow !== true })
                }
              />
              <RibbonColorSwatch
                icon="line"
                label={t('connector.color')}
                color={selConnObj.stroke ?? INK}
                onPick={(c) => patchConnector(selConnObj.id, { stroke: c })}
              />
              <RibbonButton
                icon="trash"
                label={t('connector.delete')}
                onAction={() => deleteConnector(selConnObj.id)}
              />
            </>
          )}
        </div>
        {editingMaster && (
          <div class="border-border bg-selected flex shrink-0 flex-wrap items-center justify-between gap-x-3 gap-y-1 border-b px-3 py-1">
            <span class="text-text text-xs">
              {/* Los campos van por variable: el interpolador no vuelve a leer lo que
                  sustituye, así que «{n}» llega literal al rótulo traducido. */}
              {editingLayout
                ? t('slides.layout.banner')
                : t('slides.master.banner', { fieldN: '{n}', fieldTotal: '{total}' })}
            </span>
            <DisenosBarra
              layouts={layoutDefs}
              editing={editingLayout}
              canAdd={layoutDefs.length < MAX_LAYOUTS}
              onSelect={selectLayout}
              onAdd={newLayout}
              onDuplicate={copyLayout}
              onDelete={() => void deleteLayout()}
              onRename={renameLayout}
            />
            <button
              type="button"
              class={`border-border bg-surface text-text hover:bg-bg-subtle inline-flex h-6 shrink-0 items-center gap-1 rounded-sm border px-2 text-xs ${FOCUS_RING}`}
              onClick={exitMaster}
            >
              <EditorIcon name="check" size="sm" /> {t('slides.master.close')}
            </button>
          </div>
        )}
        <div
          ref={areaRef}
          class="flex min-h-0 flex-1 overflow-auto p-6"
          style={{ background: 'var(--editor-canvas-bg)' }}
        >
          <div
            // `data-sin-tactil`: los tiradores del escenario miden lo que la caja (editor.css).
            data-sin-tactil
            class="m-auto shrink-0"
            style={{ width: `${STAGE_W * scale}px`, height: `${sh * scale}px` }}
          >
            {/* Escenario de edición: focusable (tabIndex 0) para navegar y mover cajas con el
                teclado (onKeyDown); role group no es interactivo, de ahí los dos avisos. */}
            {/* eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions */}
            <div
              ref={stageRef}
              role="group"
              aria-label={
                editingMaster
                  ? t('slides.master.aria')
                  : t('slides.aria.slideOf', { n: active + 1, total: model.slides.length })
              }
              // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
              tabIndex={0}
              class="sytue-focus-ring relative origin-top-left outline-none"
              style={{
                ...visualsStyle(stageVis),
                width: `${STAGE_W}px`,
                height: `${sh}px`,
                transform: `scale(${scale})`,
                boxShadow: 'var(--editor-page-shadow, none)',
              }}
              onKeyDown={onStageKeyDown}
              onFocusIn={onStageFocusIn}
              onFocusOut={onStageFocusOut}
              onMouseDown={(e) => {
                if (e.target === e.currentTarget) selectOnly(null); // clic en el fondo deselecciona
              }}
            >
              {/* Patrón (slide master) DETRÁS y no editable; en «Editar patrón» sus
               * cajas pasan a ser la capa editable de abajo (editBoxes). */}
              {!editingMaster &&
                masterBoxesFor(model, active).map((b) => (
                  <div key={`m-${b.id}`} class="pointer-events-none absolute" style={boxPos(b)}>
                    {renderBox(t, b, sh)}
                  </div>
                ))}
              {!editingMaster && (
                <ConnectorLayer
                  connectors={layoutConnectorsFor(model, active)}
                  boxes={[]}
                  sh={sh}
                />
              )}
              {/* Editando un diseño: el patrón (si el diseño no lo oculta) y sus líneas, de contexto; las formas del diseño (`decor`) están en `editBoxes` y se editan. */}
              {editingMaster &&
                editingLayout &&
                !model.master?.layouts?.find((l) => l.id === editingLayout)?.hideMaster &&
                (model.master?.boxes ?? []).map((b) => (
                  <div
                    key={`m-${b.id}`}
                    class="pointer-events-none absolute opacity-40"
                    style={boxPos(b)}
                  >
                    {renderBox(t, b, sh)}
                  </div>
                ))}
              {editingMaster && editingLayout && (
                <ConnectorLayer
                  connectors={
                    model.master?.layouts?.find((l) => l.id === editingLayout)?.connectors ?? []
                  }
                  boxes={editBoxes}
                  sh={sh}
                />
              )}
              {editBoxes.map((b) => {
                const isPrimary = selBox === b.id; // ancla: lleva los tiradores
                const inSel = selectedIds.has(b.id); // en la selección efectiva: lleva el marco
                const editingThis = editBox === b.id;
                const isText = !b.kind || b.kind === 'text';
                // Primer paso (clic) en que la caja se anima, para el distintivo.
                const firstAnim = animByBox.get(b.id)?.[0];
                return (
                  <div
                    key={b.id}
                    class={'absolute' + (isText ? '' : ` ${FOCUS_RING}`)}
                    style={{
                      ...boxPos(b),
                      outline: inSel ? `1px solid ${ACCENT}` : '1px solid transparent',
                      // Sin esto el navegador se queda el gesto del dedo como
                      // scroll del lienzo y la caja no se movía en una tableta.
                      ...(isText ? {} : { touchAction: 'none' }),
                    }}
                    /* Imagen/forma: el marco es focalizable (teclado) y su cuerpo
                     * arrastra para mover (los hijos llevan pointer-events:none). */
                    tabIndex={isText ? undefined : 0}
                    role={isText ? undefined : 'img'}
                    /* El mismo nombre que usa el panel de animaciones: una imagen
                     * no puede llamarse de dos maneras según quién la nombre. */
                    aria-label={isText ? undefined : boxLabel(t, b)}
                    data-boxframe={isText ? undefined : ''}
                    /* Enfocar una caja ya seleccionada NO altera la selección (si
                     * no, arrastrar un miembro de una multiselección la colapsaría). */
                    onFocus={
                      isText
                        ? undefined
                        : () => {
                            if (!isSelected(b.id)) selectOnly(b.id);
                          }
                    }
                    onPointerDown={
                      isText
                        ? undefined
                        : (e) => {
                            e.preventDefault();
                            if (e.shiftKey) {
                              toggleSel(b.id); // Mayús+clic: alterna en la multiselección (sin arrastrar)
                              return;
                            }
                            if (!isSelected(b.id)) selectOnly(b.id); // clic normal fuera de la selección: limpia
                            setEditBox(null);
                            (e.currentTarget as HTMLElement).focus();
                            onHandleDown(e, b, 'mover');
                          }
                    }
                    onPointerMove={isText ? undefined : onHandleMove}
                    onPointerUp={isText ? undefined : onHandleUp}
                    onPointerCancel={isText ? undefined : onHandleUp}
                  >
                    {renderBox(
                      t,
                      b,
                      sh,
                      isText
                        ? {
                            boxId: b.id,
                            onMouseDown: (e) => alPulsarTexto(e, b.id, editingThis),
                            onDblClick: () => alDobleClicTexto(b.id),
                          }
                        : undefined,
                    )}
                    {/* Distintivo del paso (clic) en que arranca la caja (como
                     * PowerPoint): visible al abrir el panel o con la caja seleccionada. */}
                    {firstAnim && (bottom === 'anim' || inSel) && (
                      <span
                        aria-hidden="true"
                        class="text-2xs absolute -top-2 -left-2 grid h-4 min-w-4 place-items-center rounded-full px-1 leading-none font-semibold text-white"
                        style={{ background: 'var(--sytue-primary-600)' }}
                        title={t('slides.anim.atStep', { n: firstAnim.step })}
                      >
                        {firstAnim.step}
                      </span>
                    )}
                    {isPrimary && !editingThis && (
                      <>
                        <div
                          class="sytue-slide-handle absolute -top-3 left-1/2 h-2 w-8 -translate-x-1/2 cursor-move rounded-full"
                          style={{ background: ACCENT }}
                          title={t('slides.box.move')}
                          onPointerDown={(e) => onHandleDown(e, b, 'mover')}
                          onPointerMove={onHandleMove}
                          onPointerUp={onHandleUp}
                          onPointerCancel={onHandleUp}
                        />
                        <div
                          class="sytue-slide-handle absolute -right-[5px] -bottom-[5px] h-2.5 w-2.5 cursor-nwse-resize rounded-[2px]"
                          style={{
                            background: 'var(--editor-page-bg)',
                            border: `1.5px solid ${ACCENT}`,
                          }}
                          title={t('slides.box.resize')}
                          onPointerDown={(e) => onHandleDown(e, b, 'redimensionar')}
                          onPointerMove={onHandleMove}
                          onPointerUp={onHandleUp}
                          onPointerCancel={onHandleUp}
                        />
                      </>
                    )}
                  </div>
                );
              })}
              {/* Conectores (solo en diapositivas): capa interactiva sobre las cajas.
               * Se recalculan al pintar según la posición actual de las formas. */}
              {!editingMaster && (
                <ConnectorLayer
                  connectors={slide.connectors ?? []}
                  boxes={slide.boxes}
                  sh={sh}
                  selId={selConn}
                  onSelect={onConnSelect}
                  onHandleDown={onConnHandleDown}
                  onHandleMove={onConnHandleMove}
                  onHandleUp={onConnHandleUp}
                />
              )}
              {/* Trayectorias de movimiento de las cajas seleccionadas: polilínea con
               * marcador de inicio (círculo hueco) y de fin (relleno, arrastrable para
               * reorientarla). Comparte el mismo viewBox px del lienzo. */}
              {motionOverlays.length > 0 && (
                <svg
                  class="pointer-events-none absolute inset-0 overflow-visible"
                  width={STAGE_W}
                  height={sh}
                  viewBox={`0 0 ${STAGE_W} ${sh}`}
                  fill="none"
                  aria-hidden="true"
                >
                  {motionOverlays.map(({ box: b, index, anim: a }) => {
                    const pts = motionPoints(a.type, a.points);
                    const mulX = STAGE_W / 100;
                    const mulY = sh / 100;
                    const cx = (b.x + b.w / 2) * mulX;
                    const cy = (b.y + b.h / 2) * mulY;
                    const last = pts[pts.length - 1]!;
                    const ex = cx + last.dx * mulX;
                    const ey = cy + last.dy * mulY;
                    const isOpen = last.dx !== 0 || last.dy !== 0; // el circuito cierra en el inicio
                    return (
                      <g key={`${b.id}:${index}`}>
                        <path
                          d={motionPathD(pts, mulX, mulY, cx, cy)}
                          stroke={ACCENT}
                          stroke-width={2}
                          stroke-dasharray="5 4"
                        />
                        <circle
                          cx={cx}
                          cy={cy}
                          r={5}
                          fill="white"
                          stroke={ACCENT}
                          stroke-width={2}
                        />
                        {isOpen && (
                          <circle
                            cx={ex}
                            cy={ey}
                            r={7}
                            fill={ACCENT}
                            class="pointer-events-auto cursor-move"
                            onPointerDown={(e) => onMotionEndDown(e, b.id, index)}
                            onPointerMove={onMotionEndMove}
                            onPointerUp={onMotionEndUp}
                            onPointerCancel={onMotionEndUp}
                          />
                        )}
                      </g>
                    );
                  })}
                </svg>
              )}
              {/* Cursores remotos: marco de color alrededor de las cajas que cada
                  peer tiene seleccionadas en esta diapositiva + etiqueta con su
                  nombre (contra-escalada para ser legible a cualquier zoom del
                  escenario; se desvanece al parar). No editar patrón. */}
              {!editingMaster &&
                ctx.peerCursors.map((pc) =>
                  pc.pos.kind !== 'slide' || pc.pos.slide !== active
                    ? null
                    : slidePeerBoxes(editBoxes, pc.pos.boxes).map((sb) => (
                        <div
                          key={`peer-${pc.id}-${sb.id}`}
                          aria-hidden="true"
                          class="pointer-events-none absolute"
                          style={{
                            left: `${sb.x}%`,
                            top: `${sb.y}%`,
                            width: `${sb.w}%`,
                            height: `${sb.h}%`,
                            outline: `2px solid ${pc.color}`,
                            outlineOffset: '1px',
                          }}
                        >
                          <span
                            key={cursorPosKey(pc.pos)}
                            class="sytue-peer-label text-2xs absolute left-0 max-w-[160px] truncate rounded-sm px-1 py-px leading-tight font-medium text-white"
                            style={{
                              background: pc.color,
                              bottom: '100%',
                              transformOrigin: 'left bottom',
                              transform: `scale(${1 / scale})`,
                            }}
                          >
                            {peerName(pc.name)}
                          </span>
                        </div>
                      )),
                )}
            </div>
          </div>
        </div>

        {/* Panel inferior conmutable: notas del ponente · orden de animaciones ·
         * datos del gráfico seleccionado (se guardan por el mismo camino que el modelo). */}
        {/* Las notas ENCOGEN cuando la columna no da para todo (`min-h-0`, y el campo
            también): eran `shrink-0` con un campo de alto fijo, y en un móvil apaisado
            —844×390— la barra, el margen del lienzo y las notas pedían 28 px más de los
            que había. Lo que sobraba se salía del editor, y dentro de la hoja de la
            forma «Marco», que recorta, el pie del campo quedaba fuera sin forma de
            llegar a él. Con sitio de sobra miden lo de siempre: no crecen. */}
        {bottom === 'notes' && (
          <div class="border-border bg-surface flex min-h-0 flex-col border-t">
            <div class="flex shrink-0 items-center justify-between px-3 pt-1">
              <span class="text-text text-xs font-semibold">{t('slides.notes')}</span>
              <span class="sytue-meta-dense">{t('slides.number', { n: active + 1 })}</span>
            </div>
            <textarea
              class={`bg-surface text-text placeholder:text-text-subtle h-20 min-h-0 w-full resize-none px-3 pt-1 pb-2 text-sm ${FOCUS_RING}`}
              aria-label={t('slides.notes.of', { n: active + 1 })}
              placeholder={t('slides.notes.hint')}
              value={slide.notes ?? ''}
              onInput={(e) => setNotes(e.currentTarget.value)}
            />
          </div>
        )}

        {bottom === 'anim' && (
          <div class="border-border bg-surface flex max-h-52 shrink-0 flex-col border-t">
            <div class="flex items-center justify-between px-3 pt-1.5">
              <span class="text-text text-xs font-semibold">{t('slides.animations')}</span>
              <span class="sytue-meta-dense">{t('slides.anim.legend')}</span>
            </div>

            {/* Añadir un efecto a la caja seleccionada: categoría → efecto. */}
            {selBox && boxById.has(selBox) ? (
              <div class="flex shrink-0 flex-wrap items-center gap-1.5 px-3 py-1">
                <span class="text-text-muted text-2xs">
                  {t('slides.anim.addTo', { label: boxLabel(t, boxById.get(selBox)!) })}
                </span>
                <select
                  class={`border-border bg-surface text-text h-6 rounded-sm border px-1 text-xs ${FOCUS_RING}`}
                  aria-label={t('slides.effect.categoryAdd')}
                  value={animCat}
                  onChange={(e) => setAnimCat(e.currentTarget.value as SlideAnimCategory)}
                >
                  {categoryOptions(t).map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
                <select
                  class={`border-border bg-surface text-text h-6 rounded-sm border px-1 text-xs ${FOCUS_RING}`}
                  aria-label={t('slides.effect.toAdd')}
                  value=""
                  onChange={(e) => {
                    const v = e.currentTarget.value;
                    if (ANIM_TYPES.includes(v as SlideAnimType))
                      addBoxAnim(selBox, v as SlideAnimType);
                    e.currentTarget.value = ''; // vuelve al rótulo tras añadir
                  }}
                >
                  <option value="">{t('slides.effect.add')}</option>
                  {effectOptionsOf(animCat, t).map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              </div>
            ) : (
              <p class="sytue-meta-dense px-3 py-1">{t('slides.anim.pickBox')}</p>
            )}

            {/* Secuencia global de la diapositiva (orden de reproducción). */}
            {activePlan.length === 0 ? (
              <p class="sytue-meta px-3 py-2">{t('slides.anim.empty')}</p>
            ) : (
              <ul class="min-h-0 flex-1 overflow-auto px-2 py-1">
                {activePlan.map((p, i) => {
                  const box = boxById.get(p.boxId);
                  const label = box ? boxLabel(t, box) : 'Caja';
                  return (
                    <li
                      key={`${p.boxId}:${p.index}`}
                      class="hover:bg-bg-subtle flex items-center gap-1.5 rounded-sm px-1 py-0.5"
                    >
                      <span
                        aria-hidden="true"
                        class="text-2xs grid h-5 w-5 shrink-0 place-items-center rounded-full font-semibold text-white"
                        style={{ background: 'var(--sytue-primary-600)' }}
                        title={t('slides.step', { n: p.step })}
                      >
                        {p.step}
                      </span>
                      <button
                        type="button"
                        class={`text-text min-w-0 flex-1 truncate text-left text-xs ${FOCUS_RING} rounded-sm`}
                        title={t('slides.box.select')}
                        aria-current={selBox === p.boxId ? 'true' : undefined}
                        onClick={() => {
                          setSelBox(p.boxId);
                          setEditBox(null);
                        }}
                      >
                        <span class={selBox === p.boxId ? 'font-medium' : ''}>{label}</span>
                      </button>
                      <span
                        class="border-border text-text-muted text-2xs shrink-0 rounded-sm border px-1 leading-4"
                        title={t('slides.effect.category')}
                      >
                        {t(ANIM_CATEGORY_LABEL[p.category])}
                      </span>
                      {/* Efecto (dentro de su categoría). */}
                      <select
                        class={`border-border bg-surface text-text h-6 w-28 shrink-0 rounded-sm border px-1 text-xs ${FOCUS_RING}`}
                        aria-label={t('slides.anim.effectOf', { label })}
                        value={p.anim.type}
                        onChange={(e) =>
                          setAnimEffect(p.boxId, p.index, e.currentTarget.value as SlideAnimType)
                        }
                      >
                        {effectOptionsOf(p.category, t).map((o) => (
                          <option key={o.value} value={o.value}>
                            {o.label}
                          </option>
                        ))}
                      </select>
                      {/* Disparador (la 1.ª siempre abre paso; se deja editable igual). */}
                      <select
                        class={`border-border bg-surface text-text h-6 w-32 shrink-0 rounded-sm border px-1 text-xs ${FOCUS_RING}`}
                        aria-label={t('slides.anim.triggerOf', { label })}
                        value={p.anim.trigger ?? 'click'}
                        disabled={i === 0}
                        title={i === 0 ? t('slides.anim.firstIsClick') : undefined}
                        onChange={(e) =>
                          setAnimTrigger(
                            p.boxId,
                            p.index,
                            e.currentTarget.value as SlideAnimTrigger,
                          )
                        }
                      >
                        {TRIGGER_OPTIONS.map((o) => (
                          <option key={o.value} value={o.value}>
                            {t(o.labelKey)}
                          </option>
                        ))}
                      </select>
                      <button
                        type="button"
                        class={`text-text-muted hover:bg-selected inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm disabled:opacity-30 ${FOCUS_RING}`}
                        aria-label={t('slides.anim.moveUp', { label })}
                        title={t('common.moveUp')}
                        disabled={i === 0}
                        onClick={() => moveAnim(p.anim.order, -1)}
                      >
                        <EditorIcon name="chevronUp" size="md" />
                      </button>
                      <button
                        type="button"
                        class={`text-text-muted hover:bg-selected inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm disabled:opacity-30 ${FOCUS_RING}`}
                        aria-label={t('slides.anim.moveDown', { label })}
                        title={t('common.moveDown')}
                        disabled={i === activePlan.length - 1}
                        onClick={() => moveAnim(p.anim.order, 1)}
                      >
                        <EditorIcon name="chevronDown" size="md" />
                      </button>
                      <button
                        type="button"
                        class={`text-text-muted hover:bg-selected inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-sm ${FOCUS_RING}`}
                        aria-label={t('slides.anim.remove', { label })}
                        title={t('slides.effect.remove')}
                        onClick={() => removeAnimAt(p.boxId, p.index)}
                      >
                        <EditorIcon name="close" size="md" />
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        )}

        {bottom === 'chart' &&
          (selChart ? (
            <div class="border-border bg-surface flex max-h-44 shrink-0 flex-col border-t">
              <div class="flex items-center justify-between px-3 pt-1.5">
                <span class="text-text text-xs font-semibold">{t('slides.chartData')}</span>
                <span class="sytue-meta-dense">{t('slides.chart.hint')}</span>
              </div>
              <div class="min-h-0 flex-1 overflow-auto px-3 pt-1 pb-2">
                <table class="border-collapse">
                  <tbody>
                    {(() => {
                      const grid = selChart.chart!.grid;
                      const cols = Math.max(
                        2,
                        grid.reduce((m, r) => Math.max(m, r.length), 0),
                      );
                      return grid.map((row, r) => (
                        <tr key={r}>
                          {Array.from({ length: cols }, (_, c) => (
                            <td key={c} class="p-0.5">
                              <input
                                class={`border-border bg-surface text-text h-6 min-h-[24px] w-20 rounded-sm border px-1 text-xs ${FOCUS_RING} ${r === 0 || c === 0 ? 'font-medium' : ''}`}
                                value={row[c] ?? ''}
                                aria-label={t('table.cellAt', { row: r + 1, col: c + 1 })}
                                onInput={(e) =>
                                  setChartCell(
                                    selChart.chart!,
                                    selChart.id,
                                    r,
                                    c,
                                    e.currentTarget.value,
                                  )
                                }
                              />
                            </td>
                          ))}
                        </tr>
                      ));
                    })()}
                  </tbody>
                </table>
              </div>
              {/* Los cuatro botones se llaman «Fila»/«Columna» en pantalla: sin
                  aria-label, un lector de pantalla anunciaba dos «Fila» iguales y
                  una de ellas borraba. Botones y celdas con suelo de 24 px: `h-6` son
                  21 con la raíz a 14, y así medían en producción (2026-09-21). */}
              <div class="border-border flex shrink-0 items-center gap-1 border-t px-3 py-1">
                <button
                  type="button"
                  class={`text-text-muted hover:bg-bg-subtle inline-flex h-6 min-h-[24px] items-center gap-1 rounded-sm px-2 text-xs ${FOCUS_RING}`}
                  aria-label={t('slides.chart.addRow')}
                  title={t('slides.chart.addRow')}
                  onClick={() => addChartRow(selChart.chart!, selChart.id)}
                >
                  <EditorIcon name="plus" size="sm" /> {t('common.row')}
                </button>
                <button
                  type="button"
                  class={`text-text-muted hover:bg-bg-subtle inline-flex h-6 min-h-[24px] items-center gap-1 rounded-sm px-2 text-xs ${FOCUS_RING}`}
                  aria-label={t('slides.chart.addCol')}
                  title={t('slides.chart.addCol')}
                  onClick={() => addChartCol(selChart.chart!, selChart.id)}
                >
                  <EditorIcon name="plus" size="sm" /> {t('common.column')}
                </button>
                <div aria-hidden="true" class="bg-border mx-1 h-4 w-px" />
                <button
                  type="button"
                  class={`text-text-muted hover:bg-bg-subtle inline-flex h-6 min-h-[24px] items-center gap-1 rounded-sm px-2 text-xs disabled:opacity-30 ${FOCUS_RING}`}
                  aria-label={t('slides.chart.removeRow')}
                  title={t('slides.chart.removeRow')}
                  disabled={selChart.chart!.grid.length <= 2}
                  onClick={() => delChartRow(selChart.chart!, selChart.id)}
                >
                  <EditorIcon name="minus" size="sm" /> {t('common.row')}
                </button>
                <button
                  type="button"
                  class={`text-text-muted hover:bg-bg-subtle inline-flex h-6 min-h-[24px] items-center gap-1 rounded-sm px-2 text-xs disabled:opacity-30 ${FOCUS_RING}`}
                  aria-label={t('slides.chart.removeCol')}
                  title={t('slides.chart.removeCol')}
                  disabled={selChart.chart!.grid.reduce((m, r) => Math.max(m, r.length), 0) <= 2}
                  onClick={() => delChartCol(selChart.chart!, selChart.id)}
                >
                  <EditorIcon name="minus" size="sm" /> {t('common.column')}
                </button>
              </div>
            </div>
          ) : (
            <div class="border-border bg-surface shrink-0 border-t px-3 py-3">
              <p class="sytue-meta">{t('slides.chart.pick')}</p>
            </div>
          ))}

        {bottom === 'subs' &&
          (selBoxObj?.kind === 'media' ? (
            <SubtitulosPanel
              box={selBoxObj}
              onChange={(tracks) => setMediaTracks(selBoxObj.id, tracks)}
            />
          ) : (
            <div class="border-border bg-surface shrink-0 border-t px-3 py-3">
              <p class="sytue-meta">{t('slides.subs.pick')}</p>
            </div>
          ))}

        {bottom === 'theme' && curCustomTheme && (
          <TemaPanel
            theme={curCustomTheme}
            onPatch={(patch) => patchTheme(curCustomTheme.id, patch)}
            onDelete={() => deleteTheme(curCustomTheme.id)}
          />
        )}

        {bottom === 'protect' && (
          <ProteccionPanel
            protect={model.protect}
            slideLocked={diapoBloqueada(model, slide.id)}
            boxLocked={selBox !== null && cajaBloqueada(model, selBox)}
            hayCaja={selBox !== null && !editingMaster}
            puede={!model.protect || esPropietario() || conClave}
            onProtect={(clave) => {
              if (clave !== '') presentarNueva(AMBITO_DIAPOS, clave);
              apply({ protect: {} });
              if (clave !== '') claveRef.current = clave;
              setConClave(claveRef.current !== '');
            }}
            onToggleSlide={() =>
              setProtect({ slides: alternarId(model.protect?.slides, slide.id) })
            }
            onToggleBox={() => {
              if (selBox !== null) setProtect({ boxes: alternarId(model.protect?.boxes, selBox) });
            }}
            onToggleMaster={() =>
              setProtect({ master: model.protect?.master === true ? undefined : true })
            }
            onPassword={(clave) => {
              presentarNueva(AMBITO_DIAPOS, clave);
              setProtect({}); // el guardado lleva la contraseña vigente y la nueva
              claveRef.current = clave;
              setConClave(true);
            }}
            onUnlock={(clave) => {
              claveRef.current = clave;
              presentarClave(clave);
              setConClave(true);
            }}
            onOff={() => {
              apply({ protect: undefined });
              claveRef.current = '';
              setConClave(false);
            }}
          />
        )}

        {bottom === 'smartart' &&
          (selSmart ? (
            <div class="border-border bg-surface flex max-h-52 shrink-0 flex-col border-t">
              <div class="flex items-center justify-between px-3 pt-1.5">
                <span class="text-text text-xs font-semibold">{t('slides.smartart.items')}</span>
                <span class="sytue-meta-dense">
                  {t(
                    selSmart.smartart!.layout === 'hierarchy'
                      ? 'smartart.hint.hierarchy'
                      : 'smartart.hint.items',
                  )}
                </span>
              </div>
              <ul class="min-h-0 flex-1 overflow-y-auto px-2 py-1">
                {selSmart.smartart!.items.map((it, i) => {
                  const smart = selSmart.smartart!;
                  const isHier = smart.layout === 'hierarchy';
                  const level = it.level ?? 0;
                  const maxLevel = (smart.items[i - 1]?.level ?? 0) + 1;
                  return (
                    <li key={i} class="flex items-center gap-1 py-0.5">
                      {isHier && (
                        <>
                          <button
                            type="button"
                            class={`text-text-muted hover:bg-selected inline-flex h-6 w-6 items-center justify-center rounded-sm disabled:opacity-30 ${FOCUS_RING}`}
                            aria-label={t('item.outdent', { n: i + 1 })}
                            title={t('indent.decreaseLabel')}
                            disabled={i === 0 || level === 0}
                            onClick={() => indentSmartItem(smart, selSmart.id, i, -1)}
                          >
                            <EditorIcon name="chevronLeft" size="md" />
                          </button>
                          <button
                            type="button"
                            class={`text-text-muted hover:bg-selected inline-flex h-6 w-6 items-center justify-center rounded-sm disabled:opacity-30 ${FOCUS_RING}`}
                            aria-label={t('item.indent', { n: i + 1 })}
                            title={t('indent.increaseLabel')}
                            disabled={i === 0 || level >= maxLevel}
                            onClick={() => indentSmartItem(smart, selSmart.id, i, 1)}
                          >
                            <EditorIcon name="chevronRight" size="md" />
                          </button>
                        </>
                      )}
                      <input
                        class={`border-border bg-surface text-text placeholder:text-text-subtle h-7 min-w-0 flex-1 rounded-sm border px-1.5 text-xs ${FOCUS_RING}`}
                        style={isHier ? { marginLeft: `${level * 14}px` } : undefined}
                        value={it.text}
                        aria-label={t('item.text', { n: i + 1 })}
                        placeholder={t('slides.smartart.itemText')}
                        onInput={(e) => setSmartText(smart, selSmart.id, i, e.currentTarget.value)}
                      />
                      <button
                        type="button"
                        class={`text-text-muted hover:bg-selected inline-flex h-6 w-6 items-center justify-center rounded-sm disabled:opacity-30 ${FOCUS_RING}`}
                        aria-label={t('item.moveUp', { n: i + 1 })}
                        title={t('common.moveUp')}
                        disabled={i === 0}
                        onClick={() => moveSmartItem(smart, selSmart.id, i, -1)}
                      >
                        <EditorIcon name="chevronUp" size="md" />
                      </button>
                      <button
                        type="button"
                        class={`text-text-muted hover:bg-selected inline-flex h-6 w-6 items-center justify-center rounded-sm disabled:opacity-30 ${FOCUS_RING}`}
                        aria-label={t('item.moveDown', { n: i + 1 })}
                        title={t('common.moveDown')}
                        disabled={i === smart.items.length - 1}
                        onClick={() => moveSmartItem(smart, selSmart.id, i, 1)}
                      >
                        <EditorIcon name="chevronDown" size="md" />
                      </button>
                      <button
                        type="button"
                        class={`text-text-muted hover:bg-selected inline-flex h-6 w-6 items-center justify-center rounded-sm disabled:opacity-30 ${FOCUS_RING}`}
                        aria-label={t('item.remove', { n: i + 1 })}
                        title={t('slides.smartart.removeItem')}
                        disabled={smart.items.length <= 1}
                        onClick={() => removeSmartItem(smart, selSmart.id, i)}
                      >
                        <EditorIcon name="close" size="md" />
                      </button>
                    </li>
                  );
                })}
              </ul>
              <div class="border-border flex shrink-0 items-center gap-1 border-t px-3 py-1">
                <button
                  type="button"
                  class={`text-text-muted hover:bg-bg-subtle inline-flex h-6 min-h-[24px] items-center gap-1 rounded-sm px-2 text-xs ${FOCUS_RING}`}
                  aria-label={t('slides.smartart.addItem')}
                  title={t('slides.smartart.addItem')}
                  onClick={() => addSmartItem(selSmart.smartart!, selSmart.id)}
                >
                  <EditorIcon name="plus" size="sm" /> {t('slides.smartart.item')}
                </button>
              </div>
            </div>
          ) : (
            <div class="border-border bg-surface shrink-0 border-t px-3 py-3">
              <p class="sytue-meta">{t('slides.smartart.pick')}</p>
            </div>
          ))}
      </div>

      {presenting !== null && (
        <Presentation
          model={model}
          start={presenting}
          presenter={presentMod}
          onExit={exitPresentation}
          {...(castAvisar ? { onSlide: castAvisar } : {})}
        />
      )}
      {castOpen && (
        <CastDialog
          open
          titulo={ctx.title}
          docId={ctx.doc.id}
          onClose={() => setCastOpen(false)}
          onStarted={startCast}
        />
      )}
    </div>
  );
}
