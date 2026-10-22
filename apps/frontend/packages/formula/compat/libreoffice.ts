/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Calcula el corpus (corpus.ts) con LibreOffice, una fuente INDEPENDIENTE de nuestro motor,
 * y guarda lo que contestó en `esperado-libreoffice.json`. Es lo que contrastan después
 * `compat.test.ts` (motor TypeScript) y `apps/nucleo/formula/compat_test.go` (motor de Go).
 *
 *   cd apps/frontend/packages/formula && bun compat/libreoffice.ts
 *
 * Escribe un .xlsx con los datos y una fórmula por fila en la columna P, lo convierte a CSV
 * con `soffice --headless` (que calcula las fórmulas sin valor guardado) y lee la columna P.
 * Hace falta `soffice` en el PATH (el contenedor sytue-agentes lo trae); sin él, falla.
 *
 * Lo que sale de aquí son resultados de LibreOffice, NO de Excel: donde LibreOffice y Excel
 * difieren (hay casos documentados), una coincidencia con LibreOffice no prueba que sea
 * Excel. Lo que sí prueba es que dos implementaciones sin código en común dicen lo mismo.
 */
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { catalogo } from '../src/catalogo';
import { DATOS, FORMULAS } from './corpus';
import { zip } from './zip';

/** Lo que contesta una hoja: número, texto, booleano o un error. */
export type Resultado = number | string | boolean | { error: string };

export interface Esperado {
  libreoffice: string;
  fecha: string;
  /** Los datos de la Hoja1 (corpus.ts): los lee también el test de Go, que no ve el corpus. */
  datos: Record<string, number | string | boolean>;
  /** Las funciones de Excel 365 del catálogo: LibreOffice 7.4 no las tiene y no se contrastan. */
  solo365: string[];
  casos: { f: string; lo: Resultado }[];
}

/** Funciones posteriores a Excel 2007: en un .xlsx se escriben con el prefijo `_xlfn.`. */
const POSTERIORES = new Set(
  (
    'IFS SWITCH TEXTJOIN CONCAT MAXIFS MINIFS XOR DAYS ISOWEEKNUM NUMBERVALUE UNICHAR UNICODE ' +
    'BITAND BITOR BITXOR BITLSHIFT BITRSHIFT ARABIC BASE DECIMAL COMBINA CSC CSCH SEC SECH COT COTH ' +
    'ACOT ACOTH GAMMA GAUSS PHI PERMUTATIONA RRI PDURATION ISFORMULA FORMULATEXT SHEET SHEETS ' +
    'ENCODEURL IFNA FILTER SORT SORTBY UNIQUE SEQUENCE XLOOKUP XMATCH LET LAMBDA MAP REDUCE SCAN ' +
    'MAKEARRAY BYROW BYCOL TAKE DROP CHOOSEROWS CHOOSECOLS HSTACK VSTACK TOROW TOCOL WRAPROWS ' +
    'WRAPCOLS EXPAND TEXTSPLIT TEXTBEFORE TEXTAFTER ARRAYTOTEXT VALUETOTEXT GROUPBY PIVOTBY ' +
    'PERCENTOF TRIMRANGE REGEXTEST REGEXEXTRACT REGEXREPLACE RANDARRAY ' +
    'BETA.DIST BETA.INV BINOM.DIST BINOM.DIST.RANGE BINOM.INV CHISQ.DIST CHISQ.DIST.RT CHISQ.INV ' +
    'CHISQ.INV.RT CHISQ.TEST CONFIDENCE.NORM CONFIDENCE.T COVARIANCE.P COVARIANCE.S EXPON.DIST ' +
    'F.DIST F.DIST.RT F.INV F.INV.RT F.TEST GAMMA.DIST GAMMA.INV GAMMALN.PRECISE HYPGEOM.DIST ' +
    'LOGNORM.DIST LOGNORM.INV MODE.MULT MODE.SNGL NEGBINOM.DIST NORM.DIST NORM.INV NORM.S.DIST ' +
    'NORM.S.INV PERCENTILE.EXC PERCENTILE.INC PERCENTRANK.EXC PERCENTRANK.INC POISSON.DIST ' +
    'QUARTILE.EXC QUARTILE.INC RANK.AVG RANK.EQ STDEV.P STDEV.S T.DIST T.DIST.2T T.DIST.RT ' +
    'T.INV T.INV.2T T.TEST VAR.P VAR.S WEIBULL.DIST Z.TEST SKEW.P FORECAST.LINEAR ' +
    'FORECAST.ETS.SEASONALITY FORECAST.ETS FORECAST.ETS.CONFINT FORECAST.ETS.STAT ' +
    'CEILING.MATH FLOOR.MATH CEILING.PRECISE FLOOR.PRECISE ERF.PRECISE ERFC.PRECISE AGGREGATE MUNIT'
  ).split(' '),
);

/** Los nombres del suavizado exponencial en un .xlsx de LibreOffice: las variantes aditivas son las
 *  de Excel (`FORECAST.ETS`, `.CONFINT`, `.STAT`); las multiplicativas solo existen con su prefijo. */
const ETS_XLSX: Record<string, string> = {
  'FORECAST.ETS.ADD': '_xlfn.FORECAST.ETS',
  'FORECAST.ETS.PI.ADD': '_xlfn.FORECAST.ETS.CONFINT',
  'FORECAST.ETS.STAT.ADD': '_xlfn.FORECAST.ETS.STAT',
  'FORECAST.ETS.MULT': '_xlfn.ORG.LIBREOFFICE.FORECAST.ETS.MULT',
  'FORECAST.ETS.PI.MULT': '_xlfn.ORG.LIBREOFFICE.FORECAST.ETS.PI.MULT',
  'FORECAST.ETS.STAT.MULT': '_xlfn.ORG.LIBREOFFICE.FORECAST.ETS.STAT.MULT',
};

const prefijar = (f: string): string =>
  f.replace(/(?<![A-Za-z0-9_."])([A-Z][A-Z0-9.]*)\(/g, (m, n: string) =>
    ETS_XLSX[n] ? `${ETS_XLSX[n]}(` : POSTERIORES.has(n) ? `_xlfn.${n}(` : m,
  );

const esc = (s: string): string =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

/** El libro .xlsx: los datos, y la fórmula i-ésima del corpus en P{i}. */
export function libro(
  formulas: readonly string[],
  datos: Record<string, number | string | boolean> = DATOS,
): Uint8Array {
  const filas = new Map<number, [string, string][]>();
  const celda = (ref: string, xml: string): void => {
    const fila = Number(/\d+/.exec(ref)![0]);
    const l = filas.get(fila) ?? [];
    l.push([ref, xml]);
    filas.set(fila, l);
  };
  const orden = (a: string, b: string): number => {
    const ra = /^([A-Z]+)(\d+)$/.exec(a)!;
    const rb = /^([A-Z]+)(\d+)$/.exec(b)!;
    return ra[1]!.length - rb[1]!.length || ra[1]!.localeCompare(rb[1]!);
  };
  for (const ref of Object.keys(datos).sort(orden)) {
    const v = datos[ref]!;
    if (typeof v === 'number') celda(ref, `<c r="${ref}"><v>${v}</v></c>`);
    else if (typeof v === 'boolean') celda(ref, `<c r="${ref}" t="b"><v>${v ? 1 : 0}</v></c>`);
    else
      celda(ref, `<c r="${ref}" t="inlineStr"><is><t xml:space="preserve">${esc(v)}</t></is></c>`);
  }
  formulas.forEach((f, i) => {
    celda(`P${i + 1}`, `<c r="P${i + 1}"><f>${esc(prefijar(f.slice(1)))}</f></c>`);
    // El tipo de lo que salió (1 número, 2 texto, 4 lógico, 16 error): el CSV no lo dice, y
    // TYPE de LibreOffice da 8 para toda celda con fórmula.
    // Y el número en notación científica: una fecha o un porcentaje salen del CSV con su
    // formato («2024-03-01», «5.35%») aunque se pida el valor sin formato.
    celda(
      `R${i + 1}`,
      `<c r="R${i + 1}"><f>IF(ISNUMBER(P${i + 1}),TEXT(P${i + 1},"0.00000000000000E+00"),"")</f></c>`,
    );
    celda(
      `Q${i + 1}`,
      `<c r="Q${i + 1}"><f>IF(ISERROR(P${i + 1}),16,IF(ISTEXT(P${i + 1}),2,IF(ISLOGICAL(P${i + 1}),4,1)))</f></c>`,
    );
  });
  const sheet = [...filas.keys()]
    .sort((a, b) => a - b)
    .map((r) => {
      // Las celdas de una fila van por orden de columna (los datos del corpus llegan a la AC).
      const col = (ref: string): number =>
        Array.from(ref.replace(/\d+/, '')).reduce((n, c) => n * 26 + c.charCodeAt(0) - 64, 0);
      const l = filas.get(r)!.sort((a, b) => col(a[0]) - col(b[0]));
      return `<row r="${r}">${l.map((c) => c[1]).join('')}</row>`;
    })
    .join('');
  const x = (s: string): string => `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>${s}`;
  const R = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships';
  return zip({
    '[Content_Types].xml': x(
      '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>',
    ),
    '_rels/.rels': x(
      `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="${R}/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
    ),
    'xl/workbook.xml': x(
      `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="${R}"><sheets><sheet name="Hoja1" sheetId="1" r:id="rId1"/></sheets></workbook>`,
    ),
    'xl/_rels/workbook.xml.rels': x(
      `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="${R}/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
    ),
    'xl/worksheets/sheet1.xml': x(
      `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>${sheet}</sheetData></worksheet>`,
    ),
  });
}

/** Un CSV (comillas dobles, coma) a filas de celdas; `texto` marca las que venían entrecomilladas. */
export function leerCsv(csv: string): { v: string; texto: boolean }[][] {
  const filas: { v: string; texto: boolean }[][] = [];
  let fila: { v: string; texto: boolean }[] = [];
  let v = '';
  let texto = false;
  let dentro = false;
  for (let i = 0; i < csv.length; i++) {
    const c = csv[i]!;
    if (dentro) {
      if (c === '"' && csv[i + 1] === '"') {
        v += '"';
        i++;
      } else if (c === '"') dentro = false;
      else v += c;
    } else if (c === '"') {
      dentro = true;
      texto = true;
    } else if (c === ',') {
      fila.push({ v, texto });
      v = '';
      texto = false;
    } else if (c === '\n') {
      fila.push({ v, texto });
      filas.push(fila);
      fila = [];
      v = '';
      texto = false;
    } else if (c !== '\r') v += c;
  }
  if (v !== '' || fila.length > 0) {
    fila.push({ v, texto });
    filas.push(fila);
  }
  return filas;
}

/** Los códigos de error propios de LibreOffice que tienen equivalente en Excel; el resto
 *  (Err:508 paréntesis, Err:509 operador…) se queda tal cual y no se contrasta. */
const ERR_LO: Record<string, string> = {
  'Err:502': '#NUM!',
  'Err:503': '#NUM!',
  'Err:523': '#NUM!',
  'Err:504': '#VALUE!',
  'Err:511': '#VALUE!',
  'Err:519': '#VALUE!',
  'Err:532': '#DIV/0!',
  'Err:524': '#REF!',
  'Err:525': '#NAME?',
};

/** Una celda del CSV (columna P), su tipo (Q) y su valor en notación científica (R). */
export function interpretar(v: string, tipo: string, cientifico: string): Resultado {
  switch (tipo) {
    case '1':
      return Number(cientifico);
    case '4':
      return v === 'TRUE' || v === '1';
    case '16':
      return { error: ERR_LO[v] ?? v };
    default:
      return v;
  }
}

export async function calcular(
  formulas: readonly string[] = FORMULAS,
  datos: Record<string, number | string | boolean> = DATOS,
): Promise<Esperado> {
  const dir = mkdtempSync(join(tmpdir(), 'calcus-lo-'));
  try {
    writeFileSync(join(dir, 'a.xlsx'), libro(formulas, datos));
    const r = Bun.spawnSync(
      [
        'soffice',
        '--headless',
        '--convert-to',
        'csv:Text - txt - csv (StarCalc):44,34,76,1,,0,false,false,false,false,false,1',
        '--outdir',
        dir,
        join(dir, 'a.xlsx'),
      ],
      { env: { ...process.env, HOME: dir } },
    );
    if (r.exitCode !== 0) throw new Error(`soffice falló: ${r.stderr.toString()}`);
    const csv = await Bun.file(join(dir, 'a-Hoja1.csv')).text();
    const filas = leerCsv(csv);
    const ver = Bun.spawnSync(['soffice', '--version']).stdout.toString().trim();
    return {
      libreoffice: ver,
      fecha: new Date().toISOString().slice(0, 10),
      datos,
      solo365: catalogo()
        .filter((f) => f.compat === 'excel365')
        .map((f) => f.nombre),
      casos: formulas.map((f, i) => ({
        f,
        lo: interpretar(filas[i]?.[15]?.v ?? '', filas[i]?.[16]?.v ?? '2', filas[i]?.[17]?.v ?? ''),
      })),
    };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

if (import.meta.main) {
  const e = await calcular();
  const ruta = new URL('./esperado-libreoffice.json', import.meta.url);
  writeFileSync(ruta, JSON.stringify(e, null, 1) + '\n');
  // El gate de formato del frontend mira también los JSON: se deja como lo deja prettier.
  Bun.spawnSync(['bunx', 'prettier@3.9.9', '--write', fileURLToPath(ruta)]);
  process.stdout.write(`${e.casos.length} fórmulas calculadas con ${e.libreoffice}\n`);
}
