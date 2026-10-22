/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL TEST QUE OBLIGA A LA PARIDAD web → nativas (CONVENTIONS.md §13.5).
 *
 * Cada función nueva de las web apps tiene que entrar en el registro
 * `paridad.ts` en el MISMO PR, con estado en escritorio y móvil. Si existe en
 * web y falta aquí, el test no deja pasar: la paridad se gobierna con un
 * fichero y no con la memoria de quien revisa.
 *
 * SI ESTE TEST TE PARA: tu PR añade (o toca) una función de web y no la has
 * registrado. Rellénala: 'si' si ya está en las nativas, 'planificado' SOLO
 * con `deuda` apuntando a roadmap.md. 'no' no es un estado válido para algo
 * que la web tiene.
 */
import { expect, test } from 'bun:test';
import {
  FUNCIONES_PARIDAD,
  PRODUCTOS_PARIDAD,
  type EstadoParidad,
  type FuncionParidad,
  TOPE_DEUDA,
} from './paridad.js';

const PRODUCTOS = PRODUCTOS_PARIDAD;

function conWeb(): FuncionParidad[] {
  return FUNCIONES_PARIDAD.filter((f) => f.web);
}

test('ids únicos y estables', () => {
  const ids = FUNCIONES_PARIDAD.map((f) => f.id);
  expect(new Set(ids).size).toBe(ids.length);
  for (const id of ids) {
    expect(id).toMatch(/^[a-z0-9]+(-[a-z0-9]+)*$/);
  }
});

test('producto conocido o suite', () => {
  for (const f of FUNCIONES_PARIDAD) {
    if (f.producto !== 'suite') {
      expect(PRODUCTOS.has(f.producto)).toBe(true);
    }
  }
});

test('TODA función de web está decidida en escritorio y móvil — NORMATIVO §13.5', () => {
  const fallos: string[] = [];
  for (const f of conWeb()) {
    for (const superficie of ['escritorio', 'movil'] as const) {
      const estado: EstadoParidad | undefined = f[superficie];
      if (!estado) {
        fallos.push(`${f.id}: sin estado en ${superficie}`);
      } else if (estado === 'planificado' && !f.deuda) {
        fallos.push(`${f.id}: 'planificado' en ${superficie} exige 'deuda' (roadmap/incidencia)`);
      }
    }
  }
  expect(fallos).toEqual([]);
});

/*
 * Que la deuda apunte a ALGÚN SITIO DE VERDAD. El gate solo exigía que `deuda`
 * no estuviera vacía, y con eso las 35 entradas acabaron señalando tres
 * secciones de roadmap que no existen con ese título —dos de ellas, además, de
 * un puerto cancelado (Gio)—. Una deuda que apunta a la nada es una promesa sin
 * dirección: nadie la encuentra para cerrarla.
 *
 * Compara contra los ENCABEZADOS del roadmap, no contra el texto suelto: un
 * párrafo que mencione la frase no es un sitio donde cerrar deuda.
 */
test('cada deuda apunta a una sección que existe en el roadmap — NORMATIVO §13.5', async () => {
  const roadmap = await Bun.file(new URL('../../../../../docs/roadmap.md', import.meta.url)).text();
  const encabezados = roadmap
    .split('\n')
    .filter((l) => l.startsWith('#'))
    .map((l) => l.replace(/^#+\s*/, '').trim());
  const colgadas: string[] = [];
  for (const f of FUNCIONES_PARIDAD) {
    for (const m of (f.deuda ?? '').matchAll(/docs\/roadmap\.md § (.+)/g)) {
      const seccion = m[1]!.trim();
      if (!encabezados.includes(seccion)) {
        colgadas.push(`${f.id}: «${seccion}» no es un encabezado de docs/roadmap.md`);
      }
    }
  }
  expect(colgadas).toEqual([]);
});

test('las funciones que SOLO existen en nativas están documentadas como tal', () => {
  // web:false es la excepción (integraciones de SO que la web no tiene);
  // si algún día suben a la web, su fila pasa a web:true y el gate de arriba
  // pasa a exigir decisión en las dos superficies.
  for (const f of FUNCIONES_PARIDAD) {
    if (!f.web) {
      expect(f.escritorio).toBeDefined();
      expect(f.movil).toBeDefined();
    }
  }
});

// El gate que faltaba: declarar la deuda no basta, hay que acotarla.
//
// Hasta el 2026-09-21 se podía dejar TODA función de web en 'planificado' con
// una deuda apuntando al roadmap y el gate pasaba. Se prometía paridad y no se
// entregaba: 35 de 35 funciones acabaron así en móvil. Ahora el número está
// escrito y hay que moverlo a mano, en el mismo PR, donde se ve.
test('la deuda de paridad no crece sin que se vea — NORMATIVO §13.5', () => {
  const fallos: string[] = [];
  for (const superficie of ['escritorio', 'movil'] as const) {
    const pendientes = conWeb().filter((f) => f[superficie] === 'planificado');
    const tope = TOPE_DEUDA[superficie];
    if (pendientes.length > tope) {
      fallos.push(
        `${superficie}: ${pendientes.length} funciones sin portar y el tope es ${tope}. ` +
          `Si la función nueva de verdad no puede ir a las nativas en esta ola, sube ` +
          `TOPE_DEUDA.${superficie} a ${pendientes.length} en paridad.ts y di por qué en el PR.`,
      );
    }
    // Y al revés: si se ha portado algo, que quede registrado bajando el tope.
    // Sin esto, cerrar deuda deja hueco libre para la siguiente sin que nadie
    // se entere, que es como se llegó a 35 de 35.
    if (pendientes.length < tope) {
      fallos.push(
        `${superficie}: quedan ${pendientes.length} sin portar y el tope sigue en ${tope}. ` +
          `Se ha cerrado deuda: baja TOPE_DEUDA.${superficie} a ${pendientes.length}.`,
      );
    }
  }
  expect(fallos).toEqual([]);
});
