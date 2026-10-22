/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Macros — grabar y reproducir secuencias de comandos del editor (paridad con el
 * grabador de macros de Excel/Word; MVP grabar/reproducir, SIN editor VBA).
 *
 * Costuras de captura: los TRES puntos de despacho del editor —exec (docs,
 * canvas/exec.ts), dispatchSheetCommand (hojas) y dispatchSlidesCommand
 * (diapositivas)— llaman a `recordStep()`. Ese hook es NO intrusivo: si no hay
 * grabación activa es un único `if` que retorna, sin tocar el despacho normal.
 *
 * Decisiones (ponytail):
 * - Persistencia a NIVEL USUARIO en localStorage (una clave por `sub`), no en el
 *   modelo del documento: es lo más simple que persiste y encaja con la idea de
 *   macro como herramienta transversal del usuario (como el Libro de macros
 *   personal de Excel / Normal.dotm de Word), no como dato de UN documento. El
 *   modelo-por-documento exigiría tocar 3 modelos (HTML/Sheet/Slides) y sus 3
 *   caminos de serialización; no compensa para el MVP.
 * - Se capturan COMANDOS (acciones de cinta/menú que pasan por los 3 dispatch),
 *   no pulsaciones sueltas: teclear texto en el folio o un valor en una celda no
 *   es un comando y NO se graba (igual criterio que un grabador de comandos).
 * - Reproducir aplica cada paso al CONTEXTO ACTUAL (la selección/celda/caja de
 *   ahora), como el grabador de Excel: «negrita» se aplica a lo seleccionado hoy.
 * - Reproducir NO se fía de lo guardado: una macro es texto en localStorage, y lo que
 *   hay ahí pudo escribirlo otra pestaña, una extensión o una versión anterior. Antes
 *   de despachar NADA, `validarMacro` comprueba cada paso contra el registro de
 *   comandos (`comandos.json`): que exista, que valga para este tipo de documento,
 *   que el permiso de ahora permita editar y que su valor sea de los que la cinta
 *   puede producir (fuente de la lista, color válido, URL de imagen de la casa…). Si
 *   un solo paso falla no se reproduce ninguno: una macro a medias deja el documento
 *   en un estado que nadie pidió. Después se re-despacha por los MISMOS dispatch, que
 *   además vetan lo de contenido cuando la hoja/libro está protegido
 *   (PROTECT_CONTENT_CMDS en SheetCanvas).
 * - Sin sesión no se guarda nada: la clave de localStorage llevaba «anon» y las macros
 *   de dos personas sin cuenta en un mismo perfil se mezclaban. Sin `sub`, la macro
 *   vive en memoria hasta cerrar la pestaña.
 * - Límites: 200 pasos por macro, 50 macros, 64 KiB por valor. Ninguno es técnico; son
 *   los de una macro de verdad y los que evitan que un blob rebose la cuota.
 *
 * Módulo sin componentes: seguro en SSR (sus únicos imports son el registro y los
 * valores admitidos, también puros). La lógica no trivial (normalizar/serializar/reproducir) es PURA y está
 * cubierta por test/macros.test.ts.
 */
import type { DocKind } from '@sytue/api-clients';
import { COMANDOS, esComando } from './comandos';
import {
  ALINEACION_VERTICAL,
  BLOQUES,
  FUENTES,
  INTERLINEADOS,
  TALLAS,
  esColor,
  urlPermitida,
} from './valores';
import type { OrigenesPermitidos } from './valores';

/* ---------- Límites ---------- */

/** Pasos de una macro. */
export const MAX_PASOS = 200;
/** Macros por usuario. */
export const MAX_MACROS = 50;
/** Caracteres de un valor (el JSON de una regla, una URL `data:`…). */
export const MAX_VALOR = 64 * 1024;
/** Caracteres del nombre de una macro. */
export const MAX_NOMBRE = 80;

/** Un paso grabado: comando despachado + valor opcional, y por qué editor salió. */
export interface MacroStep {
  editor: DocKind;
  cmd: string;
  value?: string;
}

/** Una macro guardada: nombre + secuencia. Nunca vacía (no se guardan sin pasos). */
export interface Macro {
  id: string;
  name: string;
  steps: MacroStep[];
}

/** Editor en el que se grabó la macro (todos sus pasos comparten editor). */
export const macroEditor = (m: Macro): DocKind => m.steps[0]?.editor ?? 'document';

/* ---------- Estado de grabación (módulo) ---------- */

// `null` = sin grabar; un array = grabación en curso acumulando pasos.
let recording: MacroStep[] | null = null;
// Pasos que no cupieron en la grabación (pasó el techo de MAX_PASOS): se cuentan para avisar.
let overflow = 0;
const listeners = new Set<() => void>();

const notify = (): void => {
  for (const fn of listeners) fn();
};

/** Suscribe a los cambios de estado de grabación (para reflejar el toggle). */
export const subscribeMacros = (fn: () => void): (() => void) => {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
};

/** ¿Hay una grabación en curso? */
export const isRecording = (): boolean => recording !== null;

/** Abre una grabación nueva (descarta cualquier resto anterior). */
export const startRecording = (): void => {
  recording = [];
  overflow = 0;
  notify();
};

/**
 * Hook NO intrusivo que llaman los 3 dispatch. Fuera de grabación es un `if` que
 * retorna: no altera en nada el despacho normal.
 */
export const recordStep = (editor: DocKind, cmd: string, value?: string): void => {
  if (recording === null) return;
  if (recording.length >= MAX_PASOS) {
    overflow += 1;
    return;
  }
  recording.push(value === undefined ? { editor, cmd } : { editor, cmd, value });
};

/**
 * Cierra la grabación y devuelve la secuencia NORMALIZADA (puede ir vacía) y cuántos
 * pasos se dejaron fuera —no reproducibles, con un valor demasiado grande o pasado el
 * techo—, para que quien grabó sepa que su macro no lleva todo lo que hizo.
 */
export const stopRecording = (): { steps: MacroStep[]; descartados: number } => {
  const raw = recording ?? [];
  const pasados = overflow;
  recording = null;
  overflow = 0;
  notify();
  const steps = normalizeSteps(raw);
  return { steps, descartados: raw.length - steps.length + pasados };
};

/* ---------- Normalización PURA ---------- */

/**
 * Comandos que NO deben reproducirse.
 *  - Deshacer/rehacer corromperían la reproducción y «presentar» (entrar en modo
 *    presentación) no es una edición.
 *  - Los de protección llevan la CONTRASEÑA tecleada en su valor: guardarla en el
 *    navegador en claro es peor que no tener macro.
 *  - `extdata` lleva el id de una conexión de datos y el resultado de una consulta: lo
 *    que contiene es de esa conexión, no de la macro.
 */
export const NON_REPLAYABLE: ReadonlySet<string> = new Set([
  'undo',
  'redo',
  'presentar',
  'protect',
  'unprotect',
  'protectbook',
  'unprotectbook',
  'extdata',
]);

/**
 * Depura una secuencia grabada para guardarla: descarta los comandos no
 * reproducibles, los que el registro no conoce y los de valor desmesurado, y devuelve
 * copias limpias de cada paso (sin aliasing). PURA.
 */
export const normalizeSteps = (raw: readonly MacroStep[]): MacroStep[] =>
  raw
    .filter(
      (s) =>
        !NON_REPLAYABLE.has(s.cmd) &&
        esComando(s.cmd) &&
        (s.value === undefined || s.value.length <= MAX_VALOR),
    )
    .map((s) =>
      s.value === undefined
        ? { editor: s.editor, cmd: s.cmd }
        : { editor: s.editor, cmd: s.cmd, value: s.value },
    );

/* ---------- Validación (frontera de confianza) ---------- */

/** Dónde se reproduce: el tipo de documento abierto, el permiso de ahora y el origen. */
export interface EntornoMacro {
  kind: DocKind;
  soloLectura: boolean;
  origenes: OrigenesPermitidos;
}

/** Por qué un paso no se reproduce. */
export type Rechazo =
  | 'excluido' // de los que nunca se reproducen (contraseñas, deshacer…)
  | 'comando' // no está en el registro
  | 'tipo' // no existe en este tipo de documento
  | 'soloLectura' // edita y ahora no se puede editar
  | 'valor' // su valor no es de los que la cinta produce
  | 'limite'; // más pasos de los admitidos

export type Veredicto = { ok: true } | { ok: false; motivo: Rechazo; paso: number };

type Valor = (v: string | undefined, e: EntornoMacro) => boolean;

const unoDe =
  (lista: readonly string[]): Valor =>
  (v) =>
    v !== undefined && lista.includes(v);
const opcional =
  (f: Valor): Valor =>
  (v, e) =>
    v === undefined || v === '' || f(v, e);
const conforme =
  (re: RegExp): Valor =>
  (v) =>
    v !== undefined && re.test(v);
/** Texto corto y no vacío: un nombre, una fórmula, un delimitador. */
const texto =
  (max: number, vacio = false): Valor =>
  (v) =>
    v !== undefined && v.length <= max && (vacio || v !== '');
const color: Valor = (v) => v !== undefined && esColor(v);
const colorOTransparente: Valor = (v) => v !== undefined && esColor(v, true);

/** Claves de un JSON cuyo valor es un color: se comprueban todas, a cualquier profundidad. */
const CLAVE_COLOR = /colou?r|fill|background|^bg/i;

function jsonSano(v: unknown, hondo = 0): boolean {
  if (hondo > 8) return false;
  if (Array.isArray(v)) return v.every((x) => jsonSano(x, hondo + 1));
  if (typeof v !== 'object' || v === null) return true;
  return Object.entries(v).every(([k, x]) =>
    typeof x === 'string' && CLAVE_COLOR.test(k) ? esColor(x, true) : jsonSano(x, hondo + 1),
  );
}

const leerJson = (v: string | undefined): unknown => {
  if (v === undefined || v === '' || v.length > MAX_VALOR) return undefined;
  try {
    return JSON.parse(v) as unknown;
  } catch {
    return undefined;
  }
};

/** El JSON de una regla, un gráfico, una tabla…: objeto, de poca hondura y con colores válidos. */
const json: Valor = (v) => {
  const d = leerJson(v);
  return typeof d === 'object' && d !== null && jsonSano(d);
};

/** Un enlace: solo http(s), mailto y ancla, o un dominio pelado (que `exec` pasa a https). */
const enlace: Valor = (v) => {
  if (v === undefined || v === '' || v.length > 2048 || /\s/.test(v)) return false;
  const esquema = /^([a-z][a-z0-9+.-]*):/i.exec(v)?.[1]?.toLowerCase();
  return esquema === undefined || ['http', 'https', 'mailto'].includes(esquema);
};

const imagen: Valor = (v, e) => v !== undefined && urlPermitida(v, e.origenes);

/** El medio de una diapositiva viaja como JSON `{ src, mediaType… }`. */
const medio: Valor = (v, e) => {
  const d = leerJson(v);
  if (typeof d !== 'object' || d === null || !jsonSano(d)) return false;
  const src = (d as { src?: unknown }).src;
  return typeof src === 'string' && urlPermitida(src, e.origenes, true);
};

const DISPOSICIONES = ['titulo', 'titulo-cuerpo', 'dos-columnas'];
const ENTERO = /^\d{1,6}$/;

/**
 * Lo que cada comando admite como valor. Los que no figuran aceptan texto corto o nada:
 * la lista es de los que hacen algo con él que una cadena cualquiera no debería poder
 * hacer. La mantiene a la vista de las uniones de `types.ts`, `SheetCanvas.tsx` y
 * `SlidesCanvas.tsx`, cuyos comentarios describen cada valor.
 */
const VALORES: Readonly<Record<string, Valor>> = {
  // Documento y diapositiva (exec)
  font: unoDe(FUENTES),
  size: unoDe(TALLAS),
  color,
  highlight: colorOTransparente,
  lineHeight: unoDe(INTERLINEADOS),
  heading: unoDe(BLOQUES),
  insertTable: conforme(/^\d{1,2}\s*[x×]\s*\d{1,2}$/i),
  insertImage: imagen,
  insertLink: enlace,
  tableCellBg: colorOTransparente,
  tableCellValign: unoDe(ALINEACION_VERTICAL),
  // Hoja
  align: unoDe(['left', 'center', 'right']),
  numfmt: conforme(/^(?:custom:.{1,128}|[a-z0-9_-]{1,32})$/i),
  decimals: unoDe(['+1', '-1']),
  border: unoDe(['all', 'outer', 'bottom', 'none']),
  style: json,
  condadd: json,
  conddel: conforme(ENTERO),
  condmove: conforme(/^\d{1,6},(?:-1|1)$/),
  validadd: json,
  validdel: conforme(ENTERO),
  copy: opcional(unoDe(['cut'])),
  paste: opcional(unoDe(['values'])),
  freeze: conforme(/^\d{1,3},\d{1,6}$/),
  sort: unoDe(['asc', 'desc']),
  filterset: json,
  filtercond: json,
  filtersort: json,
  text2cols: texto(8),
  insrow: unoDe(['above', 'below']),
  inscol: unoDe(['left', 'right']),
  chartadd: json,
  sparkadd: json,
  sparkdel: conforme(/^[A-Z]{1,3}\d{1,7}$/i),
  pivot: json,
  pivotrefresh: texto(255, true),
  pivotdel: texto(255),
  sliceradd: json,
  aifill: texto(MAX_VALOR),
  goalseek: json,
  solverapply: json,
  fillcells: json,
  scenadd: json,
  scenupdate: texto(255),
  scenapply: texto(255),
  scendel: texto(255),
  scenreport: json,
  datatable: json,
  tableadd: json,
  tablestyle: conforme(/^[\w-]{1,64}$/),
  tabletotalfn: json,
  outline: json,
  subtotal: json,
  // Diapositivas
  nueva: opcional(unoDe(DISPOSICIONES)),
  disposicion: opcional(unoDe(DISPOSICIONES)),
  imagen,
  media: medio,
  forma: texto(64),
  smartart: texto(64),
  organizar: texto(64),
  conector: opcional(unoDe(['straight', 'elbow', 'curved'])),
  tamano: opcional(json),
  iafill: texto(MAX_VALOR),
};

/** Valor de un comando sin entrada propia: nada, o un texto que no desborde. */
const porDefecto: Valor = (v) => v === undefined || v.length <= MAX_VALOR;

/** Por qué NO se reproduce este paso aquí y ahora, o `null` si se puede. PURA. */
export function validarPaso(step: MacroStep, entorno: EntornoMacro): Rechazo | null {
  if (NON_REPLAYABLE.has(step.cmd)) return 'excluido';
  if (!esComando(step.cmd)) return 'comando';
  const def = COMANDOS[step.cmd];
  if (step.editor !== entorno.kind || !def.kinds.includes(step.editor)) return 'tipo';
  // «Cortar» es `copy` con valor: el registro lo da por lectura y mueve celdas.
  const edita = def.edits || (step.cmd === 'copy' && step.value === 'cut');
  if (edita && entorno.soloLectura) return 'soloLectura';
  const valido = Object.hasOwn(VALORES, step.cmd) ? VALORES[step.cmd]! : porDefecto;
  return valido(step.value, entorno) ? null : 'valor';
}

/**
 * Valida la macro ENTERA antes de despachar nada: el primer paso que falla (con su
 * posición, desde 0) o `ok`. PURA.
 */
export function validarMacro(steps: readonly MacroStep[], entorno: EntornoMacro): Veredicto {
  if (steps.length > MAX_PASOS) return { ok: false, motivo: 'limite', paso: MAX_PASOS };
  for (let paso = 0; paso < steps.length; paso++) {
    const motivo = validarPaso(steps[paso]!, entorno);
    if (motivo) return { ok: false, motivo, paso };
  }
  return { ok: true };
}

/* ---------- Reproducción ---------- */

/**
 * Reproduce los pasos EN ORDEN despachando cada uno con `dispatch`. La función de
 * despacho se INYECTA (la UI enruta a exec/dispatchSheetCommand/
 * dispatchSlidesCommand), así el orden y el enrutado quedan testeables sin lienzo.
 * PURA respecto a este módulo (el efecto vive en el `dispatch` inyectado).
 */
export const replaySteps = (
  steps: readonly MacroStep[],
  dispatch: (step: MacroStep) => void,
): void => {
  for (const step of steps) dispatch(step);
};

/**
 * Reproducir con garantías: valida la macro entera contra el registro y el permiso de
 * ahora (`validarMacro`) y SOLO si todo pasa despacha los pasos. Devuelve el veredicto.
 */
export function reproducir(
  steps: readonly MacroStep[],
  entorno: EntornoMacro,
  dispatch: (step: MacroStep) => void,
): Veredicto {
  const veredicto = validarMacro(steps, entorno);
  if (veredicto.ok) replaySteps(steps, dispatch);
  return veredicto;
}

/* ---------- Persistencia (localStorage, clave por usuario) ---------- */

const KEY_PREFIX = 'sytue:docs:macros:';

/**
 * Clave de almacenamiento: una por `sub`, para no mezclar cuentas en un mismo perfil.
 * Sin sesión NO hay clave (`null`): una clave «anon» común juntaba las macros de todo
 * el que abriese el editor sin cuenta en ese navegador.
 */
export const macrosKey = (userSub?: string): string | null =>
  userSub ? `${KEY_PREFIX}${userSub}` : null;

const isDocKind = (v: unknown): v is DocKind =>
  v === 'document' || v === 'spreadsheet' || v === 'presentation';

const coerceStep = (s: unknown): MacroStep | null => {
  if (typeof s !== 'object' || s === null) return null;
  const r = s as Record<string, unknown>;
  if (!isDocKind(r.editor) || typeof r.cmd !== 'string') return null;
  if (!esComando(r.cmd) || NON_REPLAYABLE.has(r.cmd)) return null; // lo que el registro no conoce no se guarda
  if (typeof r.value === 'string' && r.value.length > MAX_VALOR) return null;
  return typeof r.value === 'string'
    ? { editor: r.editor, cmd: r.cmd, value: r.value }
    : { editor: r.editor, cmd: r.cmd };
};

const coerceMacro = (m: unknown): Macro | null => {
  if (typeof m !== 'object' || m === null) return null;
  const r = m as Record<string, unknown>;
  if (typeof r.id !== 'string' || typeof r.name !== 'string' || !Array.isArray(r.steps))
    return null;
  const steps: MacroStep[] = [];
  for (const s of r.steps.slice(0, MAX_PASOS)) {
    const step = coerceStep(s);
    if (step) steps.push(step);
  }
  if (steps.length === 0) return null; // una macro sin pasos válidos se descarta
  return { id: r.id, name: r.name.slice(0, MAX_NOMBRE), steps };
};

/**
 * Parseo TOLERANTE del blob persistido (frontera de confianza): nunca lanza y
 * descarta entradas mal formadas. PURA (no toca localStorage). Exportada para test.
 */
export const parseMacros = (json: string | null): Macro[] => {
  if (!json) return [];
  let data: unknown;
  try {
    data = JSON.parse(json);
  } catch {
    return [];
  }
  if (!Array.isArray(data)) return [];
  const out: Macro[] = [];
  for (const m of data) {
    if (out.length >= MAX_MACROS) break;
    const macro = coerceMacro(m);
    if (macro) out.push(macro);
  }
  return out;
};

/** Serializa las macros a JSON. PURA. */
export const serializeMacros = (macros: readonly Macro[]): string => JSON.stringify(macros);

/** Lee las macros del usuario (o [] sin sesión / sin localStorage / blob corrupto). */
export const loadMacros = (userSub?: string): Macro[] => {
  const key = macrosKey(userSub);
  if (key === null || typeof localStorage === 'undefined') return [];
  try {
    return parseMacros(localStorage.getItem(key));
  } catch {
    return []; // almacenamiento bloqueado: el acceso mismo puede lanzar
  }
};

/** Guarda las macros del usuario (best-effort: sin sesión, cuota llena o modo privado = no persiste). */
export const saveMacros = (macros: readonly Macro[], userSub?: string): void => {
  const key = macrosKey(userSub);
  if (key === null || typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(key, serializeMacros(macros));
  } catch {
    /* sin cuota o almacenamiento bloqueado: la macro no persiste */
  }
};

/** Id nuevo para una macro (UUID del navegador; reserva simple sin crypto). */
export const newMacroId = (): string =>
  typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `m-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
