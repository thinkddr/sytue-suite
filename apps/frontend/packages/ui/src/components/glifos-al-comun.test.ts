/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NINGÚN ICONO DE INTERFAZ CON ENVOLTORIO PROPIO FUERA DE `Glifo.tsx`.
 *
 * El `<svg … stroke-width={2}>` estaba escrito más de sesenta veces: uno por
 * catálogo (Icon, Rooms, Drive, Mail, la cinta de Docs), veintisiete en el editor
 * de Pages y una veintena de iconos sueltos en línea. Cuando se aprobó el estilo
 * «duotono con acento» —trazo de 1,5 px en pantalla y relleno en las formas
 * cerradas— habría hecho falta tocarlos todos, y el grosor ya divergía solo.
 * Ahora el estilo vive en `Glifo` y los catálogos solo guardan el DIBUJO.
 *
 * QUÉ SE MIRA. La etiqueta `<svg` que se delata como icono de interfaz: o lleva
 * `stroke-width` en la propia etiqueta, o está en la rejilla de la casa
 * (`viewBox="0 0 24 24"`). Los lienzos de CONTENIDO —gráficos, diapositivas,
 * anotaciones de un PDF— ponen el grosor en sus figuras y usan su propia caja,
 * así que no caen aquí: no son iconos y no llevan el estilo de los iconos.
 *
 * SI ESTE TEST TE PARA: usa `Icon` o `Glifo` de '@sytue/ui'. `<Glifo d="…" />`
 * para un dibujo de un solo trazado, o las figuras como hijos. Las excepciones se
 * comparan POR IGUALDAD: si una deja de serlo, bórrala de aquí.
 *
 * VISTO EN ROJO (2026-10-04, en el frontal), en los dos sentidos: con un
 * `<svg … stroke-width={2}>` nuevo en `apps/mail/src/components/` el test nombra el
 * fichero que sobra; y con una excepción apuntando a un catálogo ya migrado, nombra
 * la excepción que falta por borrar.
 */
import { expect, test } from 'bun:test';
import { subtrazosCerrados } from './Glifo.js';

const RAIZ = new URL('../../../../', import.meta.url).pathname; // apps/frontend/
const COMUN = 'packages/ui/src/components/Glifo.tsx';

/** La etiqueta de apertura entera; `=>` no la cierra. */
const ETIQUETA_SVG = /<svg\b(?:=>|[^>])*>/g;
const ES_ICONO = /\bstroke-?[wW]idth=|viewBox="0 0 24 24"/;

/** Fichero → [cuántos lleva, por qué]. Se cuenta para que no se cuele otro al lado. */
const EXCEPCIONES: Record<string, [number, string]> = {
  'packages/ui/src/components/Spinner.tsx': [
    1,
    'la rueda de carga: dos arcos con opacidades distintas que giran, no un glifo del catálogo',
  ],
  'packages/pages-render/src/pintar/index.ts': [
    1,
    'HTML de la página publicada (cadena, no JSX): el sitio público no carga @sytue/ui',
  ],
  'apps/notes/src/components/NotebookDashboard.tsx': [
    1,
    'el anillo de progreso de un cuaderno: un medidor en su caja de 16, no un glifo',
  ],
  // PENDIENTE, no excepción de verdad: el catálogo de Notepads está en manos de otra
  // sesión. Cuando cierre pasa a `Glifo` y esta línea se borra.
  'apps/notes/src/components/glyphs.tsx': [1, 'PENDIENTE: catálogo de Notepads, en otra sesión'],
};

/*
 * Las fuentes se leen UNA vez y al cargar el fichero, no dentro de cada test: son
 * unos mil ficheros, y con la máquina cargada la lectura sola se comía los 5 s de
 * plazo de un test (visto en el frontal: los dos caían por tiempo, no por contenido).
 * Sin comentarios, porque la documentación también escribe `<svg`.
 */
const FUENTES = new Map<string, string>();
for await (const rel of new Bun.Glob('{apps,packages}/*/src/**/*.{ts,tsx,astro}').scan({
  cwd: RAIZ,
})) {
  if (rel.includes('.test.')) continue;
  const texto = await Bun.file(RAIZ + rel).text();
  FUENTES.set(rel, texto.replace(/\/\*[\s\S]*?\*\/|^\s*\/\/.*$/gm, ''));
}

test('los iconos de interfaz salen de Icon/Glifo', () => {
  const conEnvoltorio: Record<string, number> = {};
  for (const [rel, codigo] of FUENTES) {
    if (rel === COMUN) continue;
    const n = (codigo.match(ETIQUETA_SVG) ?? []).filter((e) => ES_ICONO.test(e)).length;
    if (n) conEnvoltorio[rel] = n;
  }
  const esperado = Object.fromEntries(Object.entries(EXCEPCIONES).map(([f, [n]]) => [f, n]));
  expect(conEnvoltorio).toEqual(esperado);
});

/*
 * El techo de `subtrazosCerrados`, vigilado: una forma cerrada que empiece con `m`
 * relativa JUSTO DESPUÉS de un subtrazo abierto no se puede sacar de su sitio sin
 * interpretar el trazado entero, así que se queda sin relleno y nadie lo nota —es
 * un icono un poco más pálido que los de al lado—. Ningún catálogo tiene una; si
 * alguien la dibuja, que lo diga este test y no una captura dentro de un mes.
 */
test('ningún trazado tiene una forma cerrada que se quede sin relleno', () => {
  const sinRelleno: string[] = [];
  // Cadenas que son un `d` y nada más: empiezan por `M` y solo llevan órdenes y números.
  const TRAZADO = /(['"`])([Mm][\d\s.,+\-MmLlHhVvCcSsQqTtAaZz]{6,})\1/g;
  for (const [rel, codigo] of FUENTES) {
    for (const [, , d = ''] of codigo.matchAll(TRAZADO)) {
      const cerrados = d.split(/(?=[Mm])/).filter((s) => /z\s*$/i.test(s)).length;
      const rellenos = subtrazosCerrados(d).split(/(?=M)/).filter(Boolean).length;
      if (cerrados !== rellenos) sinRelleno.push(`${rel}: ${d.slice(0, 60)}…`);
    }
  }
  expect(sinRelleno).toEqual([]);
});
