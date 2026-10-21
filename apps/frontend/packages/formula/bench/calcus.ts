/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Benchmark reproducible del motor de Calcus con 10.000 y 100.000 celdas. Mide lo que
 * nota quien usa una hoja grande: cuánto tarda en cargarse, en calcularse entera, en
 * reaccionar a UNA edición (suelta, con cascada y volátil) y cuánta memoria ocupa.
 *
 *   cd apps/frontend/packages/formula && bun run bench            # 10k y 100k
 *   bun run bench -- --tamanos 10000 --repeticiones 7 --json      # salida para máquinas
 *
 * Es un script, no un test: los tiempos dependen de la máquina y no deben poner rojo el CI.
 * Se publica el tiempo de CPU además del de pared porque las máquinas de prueba se comparten:
 * con la carga en 50 sobre 6 núcleos el reloj de pared de una carga de 100.000 celdas se
 * multiplicó por 16 y la CPU no.
 * Lo que sí es un test (y falla) es la corrección: `bench/calcus.test.ts` ejecuta los
 * mismos escenarios en pequeño y comprueba los resultados.
 *
 * Disposición de la hoja de `n` celdas (n/4 filas × 4 columnas):
 *   A  valor literal           B  =A*1,21            C  =IF(B>100;1;0)
 *   D  acumulado =D(i-1)+B(i)  → una cadena de n/4 dependencias encadenadas (la cascada)
 * más cuatro celdas de resumen (SUM, COUNTIF, AVERAGE y un VLOOKUP) que leen columnas
 * enteras, como las que pone cualquiera al pie de una tabla.
 */
import os from 'node:os';

import { Engine } from '../src/engine.js';

/** La máquina, para que la cifra se pueda situar. `SYTUE_BENCH_MAQUINA` la fija a mano: en un
 *  contenedor ARM `os.cpus()` no trae el modelo. */
const maquina = (): string =>
  process.env['SYTUE_BENCH_MAQUINA'] ??
  `${os.cpus()[0]?.model ?? '?'} × ${os.cpus().length}, ${os.arch()}`;

export interface Medida {
  escenario: string;
  /** Celdas del libro (literales + fórmulas). */
  celdas: number;
  /** Mediana de las repeticiones, en ms de reloj de pared. */
  ms: number;
  /** Mediana de la CPU (usuario + sistema) del proceso, en ms. Es la cifra que se compara:
   *  en una máquina compartida o con carga el reloj de pared mide también a los vecinos. */
  cpuMs: number;
  /** Fórmulas que el motor evaluó de verdad en ese escenario (una repetición). */
  evaluadas: number;
}

/** Construye la hoja de referencia con `n` celdas y la devuelve ya cargada. */
export function cargarHoja(n: number): { engine: Engine; filas: number } {
  const filas = Math.max(1, Math.floor(n / 4));
  const e = new Engine({ now: () => new Date(Date.UTC(2026, 9, 2)), random: () => 0.5 });
  for (let i = 1; i <= filas; i++) {
    e.setCell('Hoja1', `A${i}`, String((i * 37) % 1000));
    e.setCell('Hoja1', `B${i}`, `=A${i}*1.21`);
    e.setCell('Hoja1', `C${i}`, `=IF(B${i}>100,1,0)`);
    e.setCell('Hoja1', `D${i}`, i === 1 ? '=B1' : `=D${i - 1}+B${i}`);
  }
  e.setCell('Hoja1', 'F1', `=SUM(B1:B${filas})`);
  e.setCell('Hoja1', 'F2', `=COUNTIF(C1:C${filas},1)`);
  e.setCell('Hoja1', 'F3', `=AVERAGE(A1:A${filas})`);
  e.setCell('Hoja1', 'F4', `=VLOOKUP(${Math.floor(filas / 2)},A1:D${filas},4,FALSE)`);
  return { engine: e, filas };
}

/** Lee las celdas de fórmula de la hoja (fuerza el cálculo de lo que esté sucio). */
export function leerTodo(e: Engine, filas: number): void {
  for (let i = 1; i <= filas; i++) {
    e.getValue('Hoja1', `B${i}`);
    e.getValue('Hoja1', `C${i}`);
    e.getValue('Hoja1', `D${i}`);
  }
  for (const r of ['F1', 'F2', 'F3', 'F4']) e.getValue('Hoja1', r);
}

/** Lo que el editor lee de verdad tras una edición: las filas que se ven (una ventana de
 *  60) y los resúmenes. No recorre las 100.000 celdas, que es lo que hace `leerTodo`. */
export function leerVentana(e: Engine): void {
  for (let i = 1; i <= 60; i++) {
    e.getValue('Hoja1', `B${i}`);
    e.getValue('Hoja1', `C${i}`);
    e.getValue('Hoja1', `D${i}`);
  }
  for (const r of ['F1', 'F2', 'F3', 'F4']) e.getValue('Hoja1', r);
}

/** CPU (usuario + sistema) que lleva gastada el proceso, en ms. */
const cpu = (): number => {
  const u = process.cpuUsage();
  return (u.user + u.system) / 1000;
};

/** Ejecuta `f` y devuelve lo que ha tardado en pared y en CPU, en ms. */
function cronometrar(f: () => void): { ms: number; cpuMs: number } {
  const c0 = cpu();
  const t0 = performance.now();
  f();
  return { ms: performance.now() - t0, cpuMs: cpu() - c0 };
}

function mediana(xs: number[]): number {
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.floor(s.length / 2)]!;
}

/** Ejecuta `f` `veces` veces y devuelve la mediana en ms y las evaluaciones de la última. */
function medir(
  e: () => Engine,
  veces: number,
  preparar: (eng: Engine) => void,
  f: (eng: Engine) => void,
): { ms: number; cpuMs: number; evaluadas: number } {
  const tiempos: number[] = [];
  const cpus: number[] = [];
  let evaluadas = 0;
  for (let k = 0; k < veces; k++) {
    const eng = e();
    preparar(eng);
    const antes = eng.evaluaciones;
    const t = cronometrar(() => f(eng));
    tiempos.push(t.ms);
    cpus.push(t.cpuMs);
    evaluadas = eng.evaluaciones - antes;
  }
  return { ms: mediana(tiempos), cpuMs: mediana(cpus), evaluadas };
}

/** Todos los escenarios para una hoja de `n` celdas. */
export function medirTamano(n: number, repeticiones: number): Medida[] {
  const filas = Math.max(1, Math.floor(n / 4));
  const celdas = filas * 4 + 4;
  const out: Medida[] = [];
  const m = (escenario: string, r: { ms: number; cpuMs: number; evaluadas: number }): void => {
    out.push({ escenario, celdas, ...r });
  };
  const nueva = (): Engine => cargarHoja(n).engine;

  // Carga: escribir todas las celdas (parseo + grafo de dependencias), sin calcular.
  const carga = Array.from({ length: repeticiones }, () => cronometrar(() => cargarHoja(n)));
  m('carga', {
    ms: mediana(carga.map((x) => x.ms)),
    cpuMs: mediana(carga.map((x) => x.cpuMs)),
    evaluadas: 0,
  });

  // Cálculo completo en frío: leer todo tras cargar.
  m(
    'calculo-completo',
    medir(
      nueva,
      repeticiones,
      () => {},
      (e) => leerTodo(e, filas),
    ),
  );

  // Recálculo completo: `recalc()` fuerza todas las fórmulas del libro (lo que hace
  // «Calcular ahora» o abrir un libro sin valores guardados).
  m(
    'recalculo-completo',
    medir(
      nueva,
      repeticiones,
      () => {},
      (e) => e.recalc(),
    ),
  );

  // Ediciones, leyendo solo lo que se ve (lo que hace el editor): 60 filas y los resúmenes.
  // Edición suelta: una celda fuera de lo que leen las fórmulas.
  m(
    'edicion-suelta',
    medir(
      nueva,
      repeticiones,
      (e) => leerTodo(e, filas),
      (e) => {
        e.setCell('Hoja1', 'H1', 'nota');
        leerVentana(e);
      },
    ),
  );

  // Edición con cascada: la primera fila, de la que cuelga toda la cadena de la columna D.
  m(
    'edicion-cascada',
    medir(
      nueva,
      repeticiones,
      (e) => leerTodo(e, filas),
      (e) => {
        e.setCell('Hoja1', 'A1', '999');
        leerVentana(e);
      },
    ),
  );

  // La misma cascada leyéndolo TODO: el coste de recalcular la cadena entera.
  m(
    'edicion-cascada-completa',
    medir(
      nueva,
      repeticiones,
      (e) => leerTodo(e, filas),
      (e) => {
        e.setCell('Hoja1', 'A1', '999');
        leerTodo(e, filas);
      },
    ),
  );

  // Con una fórmula volátil en el libro: cada edición reevalúa solo a ella y lo suyo.
  m(
    'edicion-con-volatil',
    medir(
      () => {
        const e = nueva();
        e.setCell('Hoja1', 'H2', '=NOW()');
        return e;
      },
      repeticiones,
      (e) => leerTodo(e, filas),
      (e) => {
        e.setCell('Hoja1', 'H1', 'nota');
        leerVentana(e);
        e.getValue('Hoja1', 'H2');
      },
    ),
  );

  // Muchas volátiles (una por cada cien celdas): cada edición las reevalúa todas.
  const volatiles = Math.max(1, Math.floor(n / 100));
  m(
    'edicion-con-volatiles-muchas',
    medir(
      () => {
        const e = nueva();
        for (let i = 1; i <= volatiles; i++) e.setCell('Hoja1', `H${i + 1}`, '=RAND()');
        return e;
      },
      repeticiones,
      (e) => leerTodo(e, filas),
      (e) => {
        e.setCell('Hoja1', 'H1', 'nota');
        leerVentana(e);
        for (let i = 1; i <= volatiles; i++) e.getValue('Hoja1', `H${i + 1}`);
      },
    ),
  );
  return out;
}

/** Escribe una línea en stdout (1) o stderr (2): `console` está vetado por el lint. */
function salida(fd: 1 | 2, texto: string): void {
  (fd === 1 ? process.stdout : process.stderr).write(`${texto}\n`);
}

function main(): void {
  const args = process.argv.slice(2);
  const val = (k: string): string | undefined => {
    const i = args.indexOf(k);
    return i >= 0 ? args[i + 1] : undefined;
  };
  const tamanos = (val('--tamanos') ?? '10000,100000').split(',').map(Number);
  const rep = Number(val('--repeticiones') ?? 5);
  const json = args.includes('--json');
  const todas: Medida[] = [];
  for (const n of tamanos) {
    const t0 = performance.now();
    todas.push(...medirTamano(n, rep));
    if (!json) salida(2, `# ${n} celdas medidas en ${Math.round(performance.now() - t0)} ms`);
  }
  if (json) {
    salida(
      1,
      JSON.stringify({
        runtime: `bun ${process.versions.bun}`,
        fecha: new Date().toISOString(),
        maquina: maquina(),
        cargaMedia: os.loadavg()[0],
        repeticiones: rep,
        medidas: todas,
      }),
    );
    return;
  }
  salida(
    1,
    `bun ${process.versions.bun} · ${maquina()} · carga ${os.loadavg()[0]?.toFixed(1)} · mediana de ${rep} repeticiones`,
  );
  salida(
    1,
    'escenario'.padEnd(30) +
      'celdas'.padStart(9) +
      'ms'.padStart(10) +
      'cpu ms'.padStart(10) +
      'evaluadas'.padStart(12),
  );
  for (const x of todas) {
    salida(
      1,
      x.escenario.padEnd(30) +
        String(x.celdas).padStart(9) +
        x.ms.toFixed(1).padStart(10) +
        x.cpuMs.toFixed(1).padStart(10) +
        String(x.evaluadas).padStart(12),
    );
  }
}

if (import.meta.main) main();
