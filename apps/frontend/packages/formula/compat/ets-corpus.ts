/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El corpus del suavizado exponencial triple (FORECAST.ETS y familia): series SINTÉTICAS y
 * deterministas (un generador congruencial fijo, sin Math.random) que cubren lo que pide el
 * algoritmo: tendencia, estacionalidad aditiva y multiplicativa, ruido, huecos en la línea
 * de tiempo, duplicados, desorden, fechas mensuales y series cortas. Cada serie ocupa dos
 * columnas de la Hoja1 (línea de tiempo y valores) a partir de la T, a la derecha de la
 * columna de resultados (P..R) del corpus principal. `libreoffice.ts` calcula las fórmulas
 * con LibreOffice y `ets-esperado-libreoffice.json` guarda lo que contestó.
 */

export interface Serie {
  nombre: string;
  xs: number[];
  ys: number[];
  /** ¿Valores estrictamente positivos? (solo así tiene sentido el modelo multiplicativo). */
  positiva: boolean;
}

/** Generador congruencial lineal (Numerical Recipes): reproducible en cualquier máquina. */
function azar(semilla: number): () => number {
  let s = semilla >>> 0;
  return () => {
    s = (Math.imul(s, 1664525) + 1013904223) >>> 0;
    return s / 4294967296;
  };
}

const r3 = (x: number): number => Math.round(x * 1000) / 1000;
const rango = (n: number, ini = 1): number[] => Array.from({ length: n }, (_, i) => ini + i);

function construir(): Serie[] {
  const r = azar(20261004);
  const ruido = (a: number): number => (r() - 0.5) * 2 * a;
  const tend = rango(30).map((t) => r3(10 + 2 * t + ruido(0.8)));
  const adit = rango(48).map((t) =>
    r3(50 + 0.5 * t + 8 * Math.sin((2 * Math.PI * t) / 12) + ruido(0.8)),
  );
  const mult = rango(36).map((t) =>
    r3((20 + 0.4 * t) * (1 + 0.3 * Math.sin((2 * Math.PI * t) / 6)) + ruido(0.6)),
  );
  const rui = rango(30).map(() => r3(100 + ruido(5)));
  // Huecos: faltan los puntos 7, 8, 19, 23 y 24 de la línea de tiempo.
  const falta = new Set([7, 8, 19, 23, 24]);
  const hx = rango(30).filter((t) => !falta.has(t));
  const hy = hx.map((t) => tend[t - 1]!);
  // Duplicados: los instantes 5, 12 y 20 aparecen varias veces con valores distintos.
  const dx: number[] = [];
  const dy: number[] = [];
  for (const t of rango(24)) {
    const veces = t === 12 ? 3 : t === 5 || t === 20 ? 2 : 1;
    for (let k = 0; k < veces; k++) {
      dx.push(t);
      dy.push(r3(tend[t - 1]! + (k === 0 ? 0 : k * 1.5 * (t % 2 ? 1 : -1))));
    }
  }
  // Fechas mensuales (el primer día de cada mes desde enero de 2023).
  const mx = rango(36).map((i) => {
    const y = 2023 + Math.floor((i - 1) / 12);
    const m = (i - 1) % 12;
    return Math.round(Date.UTC(y, m, 1) / 86400000) + 25569;
  });
  const my = rango(36).map((t) =>
    r3(200 + 3 * t + 25 * Math.cos((2 * Math.PI * t) / 12) + ruido(2)),
  );
  // Semanal (paso 7) con estacionalidad de 4.
  const sx = rango(40).map((i) => 45000 + 7 * i);
  const sy = rango(40).map((t) =>
    r3(80 + 1.2 * t + 6 * Math.sin((2 * Math.PI * t) / 4) + ruido(0.5)),
  );
  // La aditiva, desordenada (la línea de tiempo no tiene por qué venir ordenada).
  const orden = rango(48).map((_, i) => (i * 7) % 48);
  const ox = orden.map((i) => i + 1);
  const oy = orden.map((i) => adit[i]!);
  // Corta: seis puntos.
  const cy = [4, 6, 9, 11, 14, 16];
  return [
    { nombre: 'tendencia', xs: rango(30), ys: tend, positiva: true },
    { nombre: 'aditiva', xs: rango(48), ys: adit, positiva: true },
    { nombre: 'multiplicativa', xs: rango(36), ys: mult, positiva: true },
    { nombre: 'ruido', xs: rango(30), ys: rui, positiva: true },
    { nombre: 'huecos', xs: hx, ys: hy, positiva: true },
    { nombre: 'duplicados', xs: dx, ys: dy, positiva: true },
    { nombre: 'mensual', xs: mx, ys: my, positiva: true },
    { nombre: 'semanal', xs: sx, ys: sy, positiva: true },
    { nombre: 'desordenada', xs: ox, ys: oy, positiva: true },
    { nombre: 'corta', xs: rango(6), ys: cy, positiva: true },
  ];
}

export const SERIES: Serie[] = construir();

/** Nombre de columna de la posición n (1 = A). */
const letra = (n: number): string => {
  let s = '';
  for (let k = n; k > 0; k = Math.floor((k - 1) / 26))
    s = String.fromCharCode(65 + ((k - 1) % 26)) + s;
  return s;
};

/** Los rangos de cada serie en la Hoja1: la línea de tiempo y los valores. */
export function rangos(i: number): { t: string; v: string } {
  const ct = letra(20 + 2 * i);
  const cv = letra(21 + 2 * i);
  const n = SERIES[i]!.xs.length;
  return { t: `${ct}1:${ct}${n}`, v: `${cv}1:${cv}${n}` };
}

export const DATOS_ETS: Record<string, number> = (() => {
  const d: Record<string, number> = {};
  SERIES.forEach((s, i) => {
    s.xs.forEach((x, k) => {
      d[`${letra(20 + 2 * i)}${k + 1}`] = x;
      d[`${letra(21 + 2 * i)}${k + 1}`] = s.ys[k]!;
    });
  });
  return d;
})();

/** Los nombres de LibreOffice (lo que contesta un .xlsx) de cada variante. */
export const NOMBRES = {
  add: 'FORECAST.ETS.ADD',
  mult: 'FORECAST.ETS.MULT',
  piAdd: 'FORECAST.ETS.PI.ADD',
  piMult: 'FORECAST.ETS.PI.MULT',
  statAdd: 'FORECAST.ETS.STAT.ADD',
  statMult: 'FORECAST.ETS.STAT.MULT',
  estacionalidad: 'FORECAST.ETS.SEASONALITY',
} as const;

function formulasDe(i: number): string[] {
  const s = SERIES[i]!;
  const { t, v } = rangos(i);
  const ult = Math.max(...s.xs);
  // Objetivos a uno y a tres pasos del final (en la serie mensual, el primer día de cada mes).
  const [o1, o3] = (() => {
    if (s.nombre !== 'mensual') {
      const p = s.nombre === 'semanal' ? 7 : 1;
      return [ult + p, ult + 3 * p];
    }
    const mas = (k: number): number => {
      const d = new Date((ult - 25569) * 86400000);
      return Math.round(Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + k, 1) / 86400000) + 25569;
    };
    return [mas(1), mas(3)];
  })();
  const f: string[] = [];
  for (const [add, mult, pa, pm, sa, sm] of [
    [NOMBRES.add, NOMBRES.mult, NOMBRES.piAdd, NOMBRES.piMult, NOMBRES.statAdd, NOMBRES.statMult],
  ] as const) {
    f.push(`=${add}(${o1},${v},${t})`);
    f.push(`=${add}(${o3},${v},${t})`);
    f.push(`=${add}(${o1},${v},${t},0)`);
    f.push(`=${mult}(${o1},${v},${t})`);
    f.push(`=${mult}(${o3},${v},${t},0)`);
    f.push(`=${pa}(${o1},${v},${t})`);
    f.push(`=${pa}(${o3},${v},${t},0.9)`);
    f.push(`=${pm}(${o1},${v},${t})`);
    for (let k = 1; k <= 8; k++) f.push(`=${sa}(${v},${t},${k})`);
    for (let k = 1; k <= 8; k++) f.push(`=${sm}(${v},${t},${k})`);
  }
  f.push(`=${NOMBRES.estacionalidad}(${v},${t})`);
  return f;
}

/** Las variantes con argumentos opcionales, en las series donde interesan. */
function extras(): string[] {
  const dup = rangos(5);
  const hue = rangos(4);
  const ad = rangos(1);
  const out: string[] = [];
  for (const agg of [1, 2, 3, 4, 5, 6, 7])
    out.push(`=${NOMBRES.add}(25,${dup.v},${dup.t},1,1,${agg})`);
  for (const dc of [0, 1]) out.push(`=${NOMBRES.add}(31,${hue.v},${hue.t},1,${dc})`);
  out.push(`=${NOMBRES.add}(31,${hue.v},${hue.t},0,0)`);
  for (const sea of [0, 1, 2, 3, 4, 6, 12, 24])
    out.push(`=${NOMBRES.add}(60,${ad.v},${ad.t},${sea})`);
  for (const sea of [0, 4, 12]) out.push(`=${NOMBRES.mult}(60,${ad.v},${ad.t},${sea})`);
  for (const nc of [0.5, 0.8, 0.99]) out.push(`=${NOMBRES.piAdd}(60,${ad.v},${ad.t},${nc})`);
  // Los nombres de Excel (los mismos modelos aditivos).
  out.push(`=FORECAST.ETS(60,${ad.v},${ad.t})`);
  out.push(`=FORECAST.ETS.CONFINT(60,${ad.v},${ad.t},0.95)`);
  out.push(`=FORECAST.ETS.STAT(${ad.v},${ad.t},7)`);
  // Errores: tamaños distintos, objetivo dentro del histórico, argumentos fuera de rango.
  out.push(`=${NOMBRES.add}(60,${ad.v},${rangos(0).t})`);
  out.push(`=${NOMBRES.add}(20,${ad.v},${ad.t})`);
  out.push(`=${NOMBRES.add}(60,${ad.v},${ad.t},-1)`);
  out.push(`=${NOMBRES.add}(60,${ad.v},${ad.t},1,2)`);
  out.push(`=${NOMBRES.add}(60,${ad.v},${ad.t},1,1,8)`);
  out.push(`=${NOMBRES.piAdd}(60,${ad.v},${ad.t},1)`);
  out.push(`=${NOMBRES.piAdd}(60,${ad.v},${ad.t},0)`);
  out.push(`=${NOMBRES.statAdd}(${ad.v},${ad.t},9)`);
  out.push(`=${NOMBRES.statAdd}(${ad.v},${ad.t},0)`);
  return out;
}

export const FORMULAS_ETS: string[] = [...SERIES.flatMap((_, i) => formulasDe(i)), ...extras()];
