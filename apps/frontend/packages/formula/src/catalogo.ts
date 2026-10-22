/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * CATÁLOGO VERSIONADO de las funciones de Calcus: qué funciones hay, desde qué versión
 * del catálogo y con qué compatibilidad. Es lo que recorren los tests (que toda función
 * tenga su ficha y que el número de cada versión no cambie sin avisar) y el benchmark
 * (que ejecuta los ejemplos), y lo que usa el autocompletado del editor para ofrecer
 * también las funciones que despacha el motor y no el registro (LET, MAP, GROUPBY…).
 *
 * Se deriva del registro, así que una función nueva aparece sola; lo que NO se puede
 * deducir —desde qué versión existe y si Excel la tiene— se declara aquí:
 *
 * - `DESDE`: versión del catálogo en que entró una función. Las que no figuran son de la
 *   versión 1. Al añadir una función hay que declararla aquí y subir CATALOGO_VERSION; si
 *   se olvida, el test del número de funciones por versión se pone rojo.
 * - `SOLO_365`: las que no existen en Excel 2019 ni antes (matrices dinámicas y siguientes).
 * - `NOTAS`: dónde Calcus no hace lo mismo que Excel, función a función. Las que se han
 *   CONTRASTADO con LibreOffice y difieren salen de `compat/divergencias.json`: el test las
 *   obliga a tener nota. (Las coincidencias, y cuáles son, están en `compat/` y en
 *   docs/architecture.md#calcus-compatibilidad.)
 */
import { buildRegistry } from './functions/index.js';

/** Versión del catálogo: sube cuando se añaden funciones (o cambia lo que declara). */
export const CATALOGO_VERSION = 4;

/** Las funciones que despacha el motor y no el registro: reciben el AST (un LAMBDA o un
 *  nombre) o necesitan el entorno (los parámetros del LAMBDA en curso). */
export const FUNCIONES_DEL_MOTOR: readonly string[] = [
  'LET',
  'LAMBDA',
  'MAP',
  'REDUCE',
  'SCAN',
  'MAKEARRAY',
  'BYROW',
  'BYCOL',
  'ISOMITTED',
  'GROUPBY',
  'PIVOTBY',
];

/** Compatibilidad con Excel: `clasica` existe en Excel 2019 y anteriores; `excel365`, solo
 *  en Excel 2021 / Microsoft 365. */
export type Compatibilidad = 'clasica' | 'excel365';

/** Un caso de uso completo y comprobable: datos en la Hoja1, una fórmula en P1 y lo que
 *  debe dar P1 (la celda de arriba a la izquierda si derrama). */
export interface Ejemplo {
  datos: Record<string, string | number>;
  formula: string;
  resultado: string | number | boolean;
  /** Nombres definidos que la fórmula necesita (nombre → definición sin «=»). */
  nombres?: Record<string, string>;
}

export interface FuncionCatalogo {
  nombre: string;
  /** Versión del catálogo en que apareció la función. */
  desde: number;
  compat: Compatibilidad;
  /** La despacha el motor, no el registro. */
  motor: boolean;
  ejemplo?: Ejemplo;
  /** Dónde no coincide con Excel (ver NOTAS). */
  nota?: string;
}

/** Versión en que entró cada función que no es de la 1. */
const DESDE: Readonly<Record<string, number>> = {
  GROUPBY: 2,
  PIVOTBY: 2,
  ISOMITTED: 2,
  PERCENTOF: 2,
  TRIMRANGE: 2,
  // Versión 3: las de Excel que faltaban y tienen semántica clara (compatibilidad medida).
  STDEVPA: 3,
  VARPA: 3,
  'BINOM.DIST.RANGE': 3,
  FVSCHEDULE: 3,
  TBILLPRICE: 3,
  TBILLYIELD: 3,
  TBILLEQ: 3,
  PRICEMAT: 3,
  YIELDMAT: 3,
  AMORLINC: 3,
  AMORDEGRC: 3,
  LEFTB: 3,
  RIGHTB: 3,
  MIDB: 3,
  LENB: 3,
  FINDB: 3,
  SEARCHB: 3,
  REPLACEB: 3,
  ASC: 3,
  DBCS: 3,
  JIS: 3,
  PHONETIC: 3,
  ENCODEURL: 3,
  BAHTTEXT: 3,
  AREAS: 3,
  FORMULATEXT: 3,
  SHEET: 3,
  SHEETS: 3,
  CELL: 3,
  // Versión 4: cupones irregulares.
  ODDFPRICE: 4,
  ODDFYIELD: 4,
  ODDLPRICE: 4,
  ODDLYIELD: 4,
  // Suavizado exponencial triple: los nombres de Excel y los de LibreOffice (.ADD, .MULT, .PI.*).
  'FORECAST.ETS': 4,
  'FORECAST.ETS.ADD': 4,
  'FORECAST.ETS.MULT': 4,
  'FORECAST.ETS.CONFINT': 4,
  'FORECAST.ETS.PI.ADD': 4,
  'FORECAST.ETS.PI.MULT': 4,
  'FORECAST.ETS.SEASONALITY': 4,
  'FORECAST.ETS.STAT': 4,
  'FORECAST.ETS.STAT.ADD': 4,
  'FORECAST.ETS.STAT.MULT': 4,
};

/** Dónde no coincide con Excel, o qué le falta. Texto para quien lo lee, no para el motor. */
export const NOTAS: Readonly<Record<string, string>> = {
  LEFTB: 'Como LEFT: cuenta caracteres. Excel solo cuenta bytes en los idiomas de doble byte.',
  RIGHTB: 'Como RIGHT: cuenta caracteres.',
  MIDB: 'Como MID: cuenta caracteres.',
  LENB: 'Como LEN: cuenta caracteres.',
  FINDB: 'Como FIND: cuenta caracteres.',
  SEARCHB: 'Como SEARCH: cuenta caracteres.',
  REPLACEB: 'Como REPLACE: cuenta caracteres.',
  ASC: 'Convierte el bloque ASCII de ancho completo y el espacio ideográfico; no los katakana.',
  DBCS: 'Solo convierte el ASCII a ancho completo.',
  JIS: 'Igual que DBCS.',
  PHONETIC: 'Devuelve el propio texto: Calcus no guarda la lectura fonética.',
  CELL: 'Solo address, col, row, contents, type y filename; el resto da #N/A.',
  AREAS: 'Sin referencias de varias áreas: siempre 1.',
  SHEET: 'Sin referencias 3D.',
  SHEETS: 'Sin referencias 3D.',
  TBILLEQ:
    'Hasta 182 días es la fórmula de Excel; más allá, la ecuación del rendimiento equivalente a un bono (LibreOffice da otro valor).',
  DURATION:
    'Con las bases 1 y 3 difiere de LibreOffice, que usa la fracción de año; coincide con el ejemplo de Excel.',
  MDURATION: 'Como DURATION.',
  BESSELK:
    'Por cuadratura, a ~1e-15. LibreOffice usa una aproximación polinómica y se queda a unos 3e-9 (BESSELK(1,5; 1)).',
  ODDLPRICE:
    'Con la base 1 (real/real) cada periodo se mide en días reales, como Excel; LibreOffice usa YEARFRAC y difiere en la séptima cifra.',
  ODDLYIELD: 'Como ODDLPRICE.',
  'FORECAST.ETS':
    'Holt-Winters propio con optimización determinista. Ni Excel ni LibreOffice publican su optimizador: el pronóstico queda a un 0,4 % de mediana del de LibreOffice; los parámetros y las medidas de error no son comparables.',
  'FORECAST.ETS.ADD': 'Como FORECAST.ETS (es su nombre en LibreOffice).',
  'FORECAST.ETS.MULT':
    'Estacionalidad multiplicativa (solo LibreOffice la tiene; Excel no). Exige valores positivos.',
  'FORECAST.ETS.CONFINT':
    'Intervalo analítico (varianza de Hyndman), determinista. El de LibreOffice sale de una simulación y cambia un ±15 % entre ejecuciones.',
  'FORECAST.ETS.PI.ADD': 'Como FORECAST.ETS.CONFINT (es su nombre en LibreOffice).',
  'FORECAST.ETS.PI.MULT':
    'Como FORECAST.ETS.CONFINT con estacionalidad multiplicativa: la varianza aditiva es una aproximación.',
  'FORECAST.ETS.SEASONALITY':
    'Autocorrelación tras quitar la tendencia; 1 si no hay estacionalidad. LibreOffice da periodos espurios (11, 14, 20) en series sin ella.',
  'FORECAST.ETS.STAT':
    'Alfa, beta y gamma son los del optimizador de Calcus; los errores se miden desde el segundo periodo.',
  'FORECAST.ETS.STAT.ADD': 'Como FORECAST.ETS.STAT (es su nombre en LibreOffice).',
  'FORECAST.ETS.STAT.MULT': 'Como FORECAST.ETS.STAT con estacionalidad multiplicativa.',
  ODDFPRICE:
    'LibreOffice 7.4 devuelve #VALUE! con cualquier argumento; se contrasta con el ejemplo de la documentación de Microsoft (113,597717).',
  ODDFYIELD:
    'LibreOffice 7.4 devuelve #VALUE! con cualquier argumento; se contrasta con el ejemplo de la documentación de Microsoft (7,72 %) y con ODDFPRICE.',
  SUM: 'El texto escrito como argumento («2») se convierte a número, como en Excel; el que devuelve una función (LEFT, INDEX…) se ignora, y Excel solo ignora el de las referencias.',
};

const SOLO_365 = new Set([
  'FILTER',
  'SORT',
  'SORTBY',
  'UNIQUE',
  'SEQUENCE',
  'RANDARRAY',
  'XLOOKUP',
  'XMATCH',
  'LET',
  'LAMBDA',
  'MAP',
  'REDUCE',
  'SCAN',
  'MAKEARRAY',
  'BYROW',
  'BYCOL',
  'ISOMITTED',
  'TAKE',
  'DROP',
  'CHOOSEROWS',
  'CHOOSECOLS',
  'HSTACK',
  'VSTACK',
  'TOROW',
  'TOCOL',
  'WRAPROWS',
  'WRAPCOLS',
  'EXPAND',
  'TEXTSPLIT',
  'TEXTBEFORE',
  'TEXTAFTER',
  'ARRAYTOTEXT',
  'VALUETOTEXT',
  'GROUPBY',
  'PIVOTBY',
  'PERCENTOF',
  'TRIMRANGE',
  'REGEXTEST',
  'REGEXEXTRACT',
  'REGEXREPLACE',
]);

/** Ejemplos comprobables de las funciones nuevas de la versión 2. */
const EJEMPLOS: Readonly<Record<string, Ejemplo>> = {
  // Los de la documentación de Excel de bonos con cupón (liquidación a mitad de periodo).
  PRICE: {
    datos: {},
    formula: '=ROUND(PRICE(DATE(2008,2,15),DATE(2017,11,15),0.0575,0.065,100,2,0),6)',
    resultado: 94.634362,
  },
  YIELD: {
    datos: {},
    formula: '=ROUND(YIELD(DATE(2008,2,15),DATE(2016,11,15),0.0575,95.04287,100,2,0),6)',
    resultado: 0.065,
  },
  DURATION: {
    datos: {},
    formula: '=ROUND(DURATION(DATE(2008,1,1),DATE(2016,1,1),0.08,0.09,2,1),6)',
    resultado: 5.993775,
  },
  MDURATION: {
    datos: {},
    formula: '=ROUND(MDURATION(DATE(2008,1,1),DATE(2016,1,1),0.08,0.09,2,1),5)',
    resultado: 5.73567,
  },
  ROMAN: { datos: {}, formula: '=ROMAN(499,1)', resultado: 'LDVLIV' },
  // Los de Excel de la versión 3 llevan el resultado de la documentación de Microsoft.
  'BINOM.DIST.RANGE': {
    datos: {},
    formula: '=ROUND(BINOM.DIST.RANGE(60,0.75,48),3)',
    resultado: 0.084,
  },
  STDEVPA: {
    datos: { A1: 2, A2: 4, A3: 'x' },
    formula: '=ROUND(STDEVPA(A1:A3),4)',
    resultado: 1.633,
  },
  VARPA: { datos: { A1: 2, A2: 4 }, formula: '=VARPA(A1:A2)', resultado: 1 },
  FVSCHEDULE: {
    datos: { A1: 0.09, A2: 0.11, A3: 0.1 },
    formula: '=ROUND(FVSCHEDULE(1,A1:A3),5)',
    resultado: 1.33089,
  },
  TBILLPRICE: {
    datos: {},
    formula: '=TBILLPRICE(DATE(2008,3,31),DATE(2008,6,1),0.09)',
    resultado: 98.45,
  },
  TBILLYIELD: {
    datos: {},
    formula: '=ROUND(TBILLYIELD(DATE(2008,3,31),DATE(2008,6,1),98.45),6)',
    resultado: 0.091417,
  },
  TBILLEQ: {
    datos: {},
    formula: '=ROUND(TBILLEQ(DATE(2008,3,31),DATE(2008,6,1),0.0914),6)',
    resultado: 0.094151,
  },
  // Los de la versión 4 llevan los ejemplos de la documentación de Microsoft.
  ODDFPRICE: {
    datos: {},
    formula:
      '=ROUND(ODDFPRICE(DATE(2008,11,11),DATE(2021,3,1),DATE(2008,10,15),DATE(2009,3,1),0.0785,0.0625,100,2,1),6)',
    resultado: 113.597717,
  },
  ODDFYIELD: {
    datos: {},
    formula:
      '=ROUND(ODDFYIELD(DATE(2008,11,11),DATE(2021,3,1),DATE(2008,10,15),DATE(2009,3,1),0.0575,84.5,100,2,0),4)',
    resultado: 0.0772,
  },
  ODDLPRICE: {
    datos: {},
    formula:
      '=ROUND(ODDLPRICE(DATE(2008,2,7),DATE(2008,6,15),DATE(2007,10,15),0.0375,0.0405,100,2,0),5)',
    resultado: 99.87829,
  },
  ODDLYIELD: {
    datos: {},
    formula:
      '=ROUND(ODDLYIELD(DATE(2008,4,20),DATE(2008,6,15),DATE(2007,12,24),0.0375,99.875,100,2,0),6)',
    resultado: 0.045192,
  },
  LEFTB: { datos: {}, formula: '=LEFTB("Calcus",3)', resultado: 'Cal' },
  RIGHTB: { datos: {}, formula: '=RIGHTB("Calcus",3)', resultado: 'cus' },
  MIDB: { datos: {}, formula: '=MIDB("Calcus",2,3)', resultado: 'alc' },
  LENB: { datos: {}, formula: '=LENB("Calcus")', resultado: 6 },
  FINDB: { datos: {}, formula: '=FINDB("c","Calcus")', resultado: 4 },
  SEARCHB: { datos: {}, formula: '=SEARCHB("C","Calcus")', resultado: 1 },
  REPLACEB: { datos: {}, formula: '=REPLACEB("Calcus",1,3,"Mar")', resultado: 'Marcus' },
  ASC: { datos: {}, formula: '=ASC("ＡＢＣ")', resultado: 'ABC' },
  DBCS: { datos: {}, formula: '=DBCS("AB")', resultado: 'ＡＢ' },
  JIS: { datos: {}, formula: '=JIS("AB")', resultado: 'ＡＢ' },
  PHONETIC: { datos: { A1: 'ひらがな' }, formula: '=PHONETIC(A1)', resultado: 'ひらがな' },
  ENCODEURL: {
    datos: {},
    formula: '=ENCODEURL("http://contoso.sharepoint.com/Finance/Profit and Loss Statement.xlsx")',
    resultado:
      'http%3A%2F%2Fcontoso.sharepoint.com%2FFinance%2FProfit%20and%20Loss%20Statement.xlsx',
  },
  BAHTTEXT: { datos: {}, formula: '=BAHTTEXT(1234)', resultado: 'หนึ่งพันสองร้อยสามสิบสี่บาทถ้วน' },
  AREAS: { datos: {}, formula: '=AREAS(A1:B2)', resultado: 1 },
  FORMULATEXT: { datos: { A1: '=1+1' }, formula: '=FORMULATEXT(A1)', resultado: '=1+1' },
  SHEET: { datos: {}, formula: '=SHEET()', resultado: 1 },
  SHEETS: { datos: {}, formula: '=SHEETS()', resultado: 1 },
  CELL: { datos: {}, formula: '=CELL("address",B3)', resultado: '$B$3' },
  PRICEMAT: {
    datos: {},
    formula: '=ROUND(PRICEMAT(DATE(2008,2,15),DATE(2008,4,13),DATE(2007,11,11),0.061,0.061,0),7)',
    resultado: 99.9844989,
  },
  YIELDMAT: {
    datos: {},
    formula:
      '=ROUND(YIELDMAT(DATE(2008,3,15),DATE(2008,11,3),DATE(2007,11,8),0.0625,100.0123,0),6)',
    resultado: 0.060954,
  },
  AMORLINC: {
    datos: {},
    formula: '=AMORLINC(2400,DATE(2008,8,19),DATE(2008,12,31),300,1,0.15,1)',
    resultado: 360,
  },
  AMORDEGRC: {
    datos: {},
    formula: '=AMORDEGRC(2400,DATE(2008,8,19),DATE(2008,12,31),300,1,0.15,1)',
    resultado: 776,
  },
  PERCENTOF: { datos: { A1: 10, A2: 30 }, formula: '=PERCENTOF(A1,A1:A2)', resultado: 0.25 },
  TRIMRANGE: { datos: { B2: 7 }, formula: '=TRIMRANGE(A1:C3)', resultado: 7 },
  ISOMITTED: {
    datos: {},
    formula: '=SUMA(4)',
    nombres: { SUMA: 'LAMBDA(a,[b],IF(ISOMITTED(b),a,a+b))' },
    resultado: 4,
  },
  GROUPBY: {
    datos: { A1: 'x', A2: 'y', A3: 'x', B1: 1, B2: 2, B3: 3 },
    formula: '=GROUPBY(A1:A3,B1:B3,SUM,0,0)',
    resultado: 'x',
  },
  PIVOTBY: {
    datos: { A1: 'x', A2: 'y', B1: 'u', B2: 'v', C1: 1, C2: 2 },
    formula: '=PIVOTBY(A1:A2,B1:B2,C1:C2,SUM,0,0,,0)',
    resultado: '',
  },
};

/** Todas las funciones del catálogo (registro + las del motor), ordenadas por nombre. */
export function catalogo(): FuncionCatalogo[] {
  const delMotor = new Set(FUNCIONES_DEL_MOTOR);
  const nombres = new Set([...buildRegistry().names(), ...delMotor]);
  return [...nombres].sort().map((nombre) => {
    const f: FuncionCatalogo = {
      nombre,
      desde: DESDE[nombre] ?? 1,
      compat: SOLO_365.has(nombre) ? 'excel365' : 'clasica',
      motor: delMotor.has(nombre),
    };
    const ejemplo = EJEMPLOS[nombre];
    const nota = NOTAS[nombre];
    return { ...f, ...(ejemplo ? { ejemplo } : {}), ...(nota ? { nota } : {}) };
  });
}

/** Nombres de todas las funciones que se pueden escribir en una fórmula. */
export function nombresDeFunciones(): string[] {
  return catalogo().map((f) => f.nombre);
}
