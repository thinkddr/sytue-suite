/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Nombres definidos del libro (`SheetModel.names`): las reglas PURAS del
 * Administrador de nombres, sin DOM ni estado de UI.
 *
 * El motor ya sabía resolver nombres (`Engine.defineName`), pero el producto no
 * los guardaba: un XLSX con `=SUMA(Ventas)` llegaba con la fórmula y sin
 * `Ventas`, y la hoja se llenaba de #NAME?. Aquí vive lo que hace falta para que
 * un nombre sobreviva a la vida normal de un libro:
 *
 *   · validarlo como Excel (sin espacios, sin forma de referencia A1 ni R1C1,
 *     255 caracteres, único por ámbito y distinto de cualquier tabla);
 *   · cargarlo en el motor con su ámbito (libro, u hoja: el de la hoja tapa al
 *     del libro en las fórmulas de esa hoja);
 *   · renombrarlo reescribiendo las fórmulas que lo usan, como Excel;
 *   · y que su definición siga a sus celdas al insertar o borrar filas y
 *     columnas, al renombrar la hoja o al borrarla (mismas reglas que refshift.ts).
 *
 * Self-test: packages/office/test/sheets-nombres.test.ts (bun test).
 */
import { isValidDefinedName, numToCol, parseFormula } from '@sytue/formula';
import type { Engine } from '@sytue/formula';
import type { DefinedName, SheetModel } from '../types';
import { extractRefs, tokenizeFx } from './formula-edit';
import { parseRangeRef, sheetPrefix } from './format';
import { moverReferencia, reescribirCelda, type Desplazamiento } from './refshift';

/** Tope de Excel para la longitud de un nombre. */
const MAX_NAME = 255;
/** Forma de referencia R1C1 («RC», «R2C3», «R4», «C7»): Excel no la admite como nombre. */
const R1C1_RE = /^(R\d*C\d*|R\d+|C\d+)$/i;

/** Por qué no vale un nombre; la clave de su texto es `names.error.<código>`. */
export type NameError = 'empty' | 'invalid' | 'tooLong' | 'taken' | 'badRef' | 'noSheet';

/** Lo que hay que saber del libro para validar un nombre. */
export interface NameBook {
  names: readonly DefinedName[];
  sheets: readonly string[];
  /** Nombres de tabla: un nombre definido no puede llamarse como una tabla. */
  tables?: readonly string[];
}

/** El mismo nombre puede existir una vez por ámbito (libro, o cada hoja). */
const scopeKey = (n: { name: string; sheet?: string | undefined }): string =>
  `${n.sheet ?? ''}\u0000${n.name.trim().toUpperCase()}`;

/** Por qué `draft` no se puede guardar (editando `prev`, si se da); null si vale. */
export function nameError(
  draft: DefinedName,
  book: NameBook,
  prev?: DefinedName,
): NameError | null {
  const name = draft.name.trim();
  if (name === '') return 'empty';
  if (name.length > MAX_NAME) return 'tooLong';
  if (!isValidDefinedName(name) || R1C1_RE.test(name)) return 'invalid';
  if (draft.sheet !== undefined && !book.sheets.includes(draft.sheet)) return 'noSheet';
  const key = scopeKey(draft);
  const own = prev ? scopeKey(prev) : null;
  if (book.names.some((n) => scopeKey(n) === key && scopeKey(n) !== own)) return 'taken';
  if (book.tables?.some((t) => t.toUpperCase() === name.toUpperCase())) return 'taken';
  const ref = draft.ref.trim().replace(/^=/, '');
  if (ref === '') return 'badRef';
  try {
    parseFormula(ref);
  } catch {
    return 'badRef';
  }
  return null;
}

/**
 * La definición tal y como se guarda: sin «=» y, si es un rango suelto («B2:B9»
 * o «Hoja1!B2:B9»), cualificado con su hoja y en absoluto («Hoja1!$B$2:$B$9»).
 * Es como lo escriben Excel y Calc, y lo único que significa lo mismo en los
 * tres: en Excel una referencia RELATIVA dentro de un nombre se mueve con la
 * celda que lo usa, y aquí el motor la ancla en A1.
 * ponytail: una fórmula («B2*2») se guarda tal cual; si se lleva a Excel con
 * refs relativas, allí se leen relativas. Absolutizarlas al guardar si se pide.
 */
export function normalizeRef(ref: string, sheet: string): string {
  const text = ref.trim().replace(/^=/, '');
  const refs = extractRefs(`=${text}`);
  const r = refs[0];
  if (refs.length !== 1 || !r || r.start !== 1 || r.end !== text.length + 1) return text;
  if (!parseRangeRef(r.text.slice(r.text.lastIndexOf('!') + 1))) return text; // A:A, 1:1
  const { c1, r1, c2, r2 } = r.rect;
  const abs = (c: number, f: number): string => `$${numToCol(c)}$${f + 1}`;
  const a1 = c1 === c2 && r1 === r2 ? abs(c1, r1) : `${abs(c1, r1)}:${abs(c2, r2)}`;
  return sheetPrefix(r.sheet ?? sheet) + a1;
}

/** Saneado de carga: conserva todo nombre con forma mínima, aunque el motor no lo
 *  entienda (uno con tilde que viene de Excel se guarda y vuelve a salir igual). */
export function sanitizeNames(raw: unknown): DefinedName[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const out: DefinedName[] = [];
  for (const x of raw as unknown[]) {
    if (!x || typeof x !== 'object') continue;
    const { name, ref, sheet, comment } = x as Record<string, unknown>;
    if (typeof name !== 'string' || name === '' || typeof ref !== 'string') continue;
    const n: DefinedName = { name, ref };
    if (typeof sheet === 'string' && sheet !== '') n.sheet = sheet;
    if (typeof comment === 'string' && comment !== '') n.comment = comment;
    out.push(n);
  }
  return out.length > 0 ? out : undefined;
}

/** Carga los nombres en el motor. Va ANTES que las celdas: cada `defineName`
 *  recalcula las dependencias de todas, y con el motor vacío eso no cuesta nada. */
export function defineNames(engine: Engine, names: readonly DefinedName[] | undefined): void {
  const sheets = engine.sheetNames();
  for (const n of names ?? []) {
    if (n.sheet !== undefined && !sheets.includes(n.sheet)) continue; // su hoja ya no está
    engine.defineName(n.name, n.ref, n.sheet, n.sheet !== undefined);
  }
}

/** Los nombres que ve una fórmula de `sheet` (los suyos y los del libro), sin repetir. */
export function namesVisibleFrom(
  names: readonly DefinedName[] | undefined,
  sheet: string,
): string[] {
  const seen = new Map<string, string>();
  for (const n of names ?? []) {
    if (n.sheet === undefined || n.sheet === sheet) seen.set(n.name.toUpperCase(), n.name);
  }
  return [...seen.values()];
}

/** ¿Una fórmula de `sheet` resuelve `n`? Uno de hoja solo allí; uno del libro, donde
 *  ningún nombre de la hoja se llame igual. */
function sees(n: DefinedName, sheet: string, names: readonly DefinedName[]): boolean {
  if (n.sheet !== undefined) return n.sheet === sheet;
  const up = n.name.toUpperCase();
  return !names.some((x) => x.sheet === sheet && x.name.toUpperCase() === up);
}

/**
 * Cambia el nombre `viejo` por `nuevo` en el texto de una fórmula («=…»). Solo
 * donde la palabra ES el nombre: no un nombre de función («Tasa(»), ni una hoja
 * («Tasa!A1»), ni una columna de tabla («Ventas[Tasa]», «[@Tasa]»), ni lo que va
 * entre comillas. ponytail: una columna de tres palabras con la del medio igual
 * al nombre se tocaría; resolver con los tramos de corchete si llega a pasar.
 */
export function renombrarEnFormula(texto: string, viejo: string, nuevo: string): string {
  if (!texto.startsWith('=')) return texto;
  const up = viejo.toUpperCase();
  const toks = tokenizeFx(texto);
  let out = texto;
  for (let k = toks.length - 1; k >= 0; k--) {
    const t = toks[k]!;
    if (t.t !== 'word' || t.v.toUpperCase() !== up) continue;
    const next = toks[k + 1]?.t;
    if (next === 'lparen' || next === 'bang' || toks[k - 1]?.t === 'bang') continue;
    if ('[@'.includes(texto[t.start - 1] ?? ' ') || '[]'.includes(texto[t.end] ?? ' ')) continue;
    out = out.slice(0, t.start) + nuevo + out.slice(t.end);
  }
  return out;
}

/** Reescribe las referencias de una definición que apuntan a `sheet` (las sin hoja
 *  cuentan como de `base`); `to` decide el texto nuevo de cada una. */
function rewriteRefs(
  ref: string,
  sheet: string,
  base: string,
  to: (refText: string) => string,
): string {
  const texto = `=${ref}`;
  let out = texto;
  for (const r of extractRefs(texto).sort((x, y) => y.start - x.start)) {
    if ((r.sheet ?? base) !== sheet) continue;
    const nuevo = to(r.text);
    if (nuevo !== r.text) out = out.slice(0, r.start) + nuevo + out.slice(r.end);
  }
  return out.slice(1);
}

/** Las definiciones siguen a sus celdas al insertar o borrar filas o columnas en
 *  `sheet` (`first` = hoja base de los nombres del libro). Mismas reglas que las
 *  fórmulas: lo borrado entero pasa a #¡REF!. */
export function moverNombres(
  names: readonly DefinedName[] | undefined,
  sheet: string,
  first: string,
  d: Desplazamiento,
): DefinedName[] | undefined {
  return names?.map((n) => {
    const ref = rewriteRefs(n.ref, sheet, n.sheet ?? first, (t) => moverReferencia(t, d));
    return ref === n.ref ? n : { ...n, ref };
  });
}

/** Renombrar una hoja: su prefijo en las definiciones y el ámbito de sus nombres. */
export function renombrarHojaEnNombres(
  names: readonly DefinedName[] | undefined,
  viejo: string,
  nuevo: string,
): DefinedName[] | undefined {
  return names?.map((n) => {
    // `base` imposible: solo se tocan las refs que nombran la hoja explícitamente.
    const ref = rewriteRefs(n.ref, viejo, '\u0000', (t) => {
      return sheetPrefix(nuevo) + t.slice(t.lastIndexOf('!') + 1);
    });
    if (ref === n.ref && n.sheet !== viejo) return n;
    const m: DefinedName = { ...n, ref };
    if (n.sheet === viejo) m.sheet = nuevo;
    return m;
  });
}

/** Borrar una hoja se lleva sus nombres de hoja (como Excel). Los del libro que la
 *  citaban se quedan, y dan #¡REF! al usarse. */
export function quitarHojaDeNombres(
  names: readonly DefinedName[] | undefined,
  sheet: string,
): DefinedName[] | undefined {
  const out = names?.filter((n) => n.sheet !== sheet);
  return out && out.length > 0 ? out : undefined;
}

/** El modelo con esa lista de nombres (sin la clave si queda vacía). */
export function withNames(
  model: SheetModel,
  names: readonly DefinedName[] | undefined,
): SheetModel {
  const next: SheetModel = { ...model };
  if (names && names.length > 0) next.names = [...names];
  else delete next.names;
  return next;
}

/** Lo que pide el Administrador de nombres (valor JSON de `nameset`/`namedel`). */
export interface NameRequest {
  name: string;
  ref?: string;
  /** Ámbito; ausente o '' = libro. */
  sheet?: string;
  comment?: string;
  /** Al editar: el nombre que se cambia (nombre + ámbito de antes). */
  prev?: { name: string; sheet?: string };
}

/** Resultado de un comando de nombres: el modelo nuevo, o por qué no. */
export type NameResult = { model: SheetModel } | { error: NameError | 'protected' };

/**
 * Crea, edita o borra un nombre. Renombrar reescribe las fórmulas que lo usaban
 * (las celdas y las otras definiciones), igual que Excel; si eso tocara una hoja
 * protegida, no se hace nada. Borrar NO toca las fórmulas: pasan a #NAME?, que es
 * lo honesto. null si la petición no se entiende o no cambia nada.
 */
export function applyNameCommand(
  model: SheetModel,
  cmd: 'nameset' | 'namedel',
  value: string | undefined,
): NameResult | null {
  let req: NameRequest;
  try {
    req = JSON.parse(value ?? '') as NameRequest;
  } catch {
    return null;
  }
  if (!req || typeof req.name !== 'string') return null;
  const names = model.names ?? [];
  const scopeOf = (s: unknown): string | undefined =>
    typeof s === 'string' && s !== '' ? s : undefined;

  if (cmd === 'namedel') {
    const key = scopeKey({ name: req.name, sheet: scopeOf(req.sheet) });
    const next = names.filter((n) => scopeKey(n) !== key);
    return next.length === names.length ? null : { model: withNames(model, next) };
  }

  const sheet = scopeOf(req.sheet);
  const active = model.sheets[model.active]?.name ?? '';
  const draft: DefinedName = {
    name: req.name.trim(),
    ref: normalizeRef(typeof req.ref === 'string' ? req.ref : '', sheet ?? active),
  };
  if (sheet !== undefined) draft.sheet = sheet;
  if (typeof req.comment === 'string' && req.comment.trim() !== '')
    draft.comment = req.comment.trim();
  const prevKey = req.prev
    ? scopeKey({ name: req.prev.name, sheet: scopeOf(req.prev.sheet) })
    : null;
  const prev = prevKey ? names.find((n) => scopeKey(n) === prevKey) : undefined;
  const sheets = model.sheets.map((s) => s.name);
  const err = nameError(
    draft,
    { names, sheets, tables: model.sheets.flatMap((s) => (s.tables ?? []).map((t) => t.name)) },
    prev,
  );
  if (err) return { error: err };

  let list = prev ? names.map((n) => (n === prev ? draft : n)) : [...names, draft];
  let next = model;
  if (prev && prev.name.toUpperCase() !== draft.name.toUpperCase()) {
    const ren = (f: string): string => renombrarEnFormula(f, prev.name, draft.name);
    let blocked = false;
    next = {
      ...model,
      sheets: model.sheets.map((s) => {
        if (!sees(prev, s.name, names)) return s;
        let cells: typeof s.cells | null = null;
        for (const ref in s.cells) {
          const html = s.cells[ref]!;
          const nuevo = reescribirCelda(html, ren);
          if (nuevo === html) continue;
          cells ??= { ...s.cells };
          cells[ref] = nuevo;
        }
        if (!cells) return s;
        if (s.protect) blocked = true;
        return { ...s, cells };
      }),
    };
    if (blocked) return { error: 'protected' };
    const first = sheets[0] ?? '';
    list = list.map((n) => {
      if (n === draft || !sees(prev, n.sheet ?? first, names)) return n;
      const ref = ren(`=${n.ref}`).slice(1);
      return ref === n.ref ? n : { ...n, ref };
    });
  }
  return { model: withNames(next, list) };
}
