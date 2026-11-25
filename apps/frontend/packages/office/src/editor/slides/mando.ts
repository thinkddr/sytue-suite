/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MANDO DE LAS DIAPOSITIVAS: los comandos que la cinta, el asistente y las macros
 * mandan al lienzo de diapositivas montado, lo que leen de él (selección, tamaño,
 * diseños) y las listas que pintan (disposiciones, formas, conectores, SmartArt).
 *
 * Vivía dentro de `SlidesCanvas.tsx`, y la cinta de un DOCUMENTO («Insertar», «Vista»,
 * las macros) importaba el lienzo de diapositivas entero. El lienzo publica aquí lo
 * suyo al montarse; `SlidesCanvas` reexporta todo, para quien ya lo importaba de allí.
 */
import type { DocsKey } from '../../i18n/index';
import type { EditorIconName } from '../icons';
import { recordStep } from '../macros';
import type { ConnectorKind, SlideShape, SlideSize, SmartArtLayout } from '../types';

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

/** Lo registra el lienzo al montarse; con `null` (o con otro), deja de recibir comandos. */
export function registrarComandosDeDiapos(
  run: ((cmd: SlidesCommand, value?: string) => void) | null,
): void {
  slidesCommandHandler = run;
}

/** ¿Es `run` el manejador montado ahora? (al desmontarse, solo se quita el suyo). */
export function esManejadorDeDiapos(run: (cmd: SlidesCommand, value?: string) => void): boolean {
  return slidesCommandHandler === run;
}

/** Envía un comando al SlidesCanvas montado (no-op si no hay ninguno). Si hay
 *  grabación de macro activa, registra el comando antes de despacharlo
 *  (recordStep es no-op fuera de grabación: no altera el despacho normal). */
export function dispatchSlidesCommand(cmd: SlidesCommand, value?: string): void {
  if (!slidesCommandHandler) return;
  recordStep('presentation', cmd, value);
  slidesCommandHandler(cmd, value);
}

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
export function publicarSeleccion(s: SlidesSelectionSnapshot): void {
  slidesSelSnapshot = s;
}

/** Último tamaño del lienzo publicado por el SlidesCanvas montado (para el
 *  control «Tamaño de diapositiva» del ribbon, que no ve el modelo). ponytail:
 *  variable de módulo como slidesSelSnapshot (un lienzo montado por documento). */
let slidesSizeSnapshot: SlideSize | undefined;
export function getSlidesSize(): SlideSize | undefined {
  return slidesSizeSnapshot;
}
/** El lienzo publica su tamaño al montarse y al cambiar; `undefined` al desmontarse. */
export function publicarTamano(size: SlideSize | undefined): void {
  slidesSizeSnapshot = size;
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
export function publicarDisenos(layouts: readonly { id: string; name: string }[]): void {
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
/* ---------- Medios incrustados (audio/vídeo): tipo y origen ---------- */

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
