/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NINGUNA PÁGINA REDIRIGE AL LOGIN SIN EL PREFIJO DE SU APLICACIÓN.
 *
 * Las apps se sirven bajo un prefijo —`/mail`, `/docs`, `/calendar`—, así que el
 * login vive en `/mail/auth/login` y no en la raíz del host. Con `/auth/login` a
 * secas la redirección acaba en el 404 del router, y como es la redirección que
 * recibe CUALQUIERA QUE ABRA LA SUITE SIN SESIÓN, el efecto es que no se puede
 * entrar en ninguna aplicación.
 *
 * `requireSession` de este paquete ya lo compone bien, pero veintiuna páginas
 * `.astro` se escribían la redirección a mano y ninguna llevaba el prefijo.
 * Comprobado en producción el 2026-09-06: las cinco apps que se probaron
 * redirigían a `/auth/login` y las cinco daban 404.
 *
 * NO SE VE EN DESARROLLO —cada app se sirve sola en su puerto, donde la raíz SÍ
 * es la suya— ni con la sesión ya abierta, que es como se prueba casi siempre.
 * Por eso hace falta un gate y no basta con mirar.
 */
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const APPS = join(import.meta.dir, '../../../../apps');
// La shell es un PAQUETE COMPARTIDO: su `AccountMenu` se pinta en las diez apps,
// así que un `/auth/login` a pelo ahí no rompe una superficie — las rompe todas.
// Le pasó al cambio de organización (2026-09-26): el gate solo barría `apps/` y
// el 404 nació aquí sin que nadie lo viera hasta producción. Mismo barrido.
const SHELL = join(import.meta.dir, '../../../shell/src');
// Y la ofimática común: sus vistas y su layout se pintan en Docus, Calcus, Diapos y
// CertiPDF, así que un login a pelo ahí las rompe a las cuatro.
const OFICINA = join(import.meta.dir, '../../../office/src');

function paginas(dir: string, salida: string[] = []): string[] {
  for (const entrada of readdirSync(dir)) {
    const ruta = join(dir, entrada);
    if (statSync(ruta).isDirectory()) {
      if (entrada === 'node_modules' || entrada === 'dist') continue;
      paginas(ruta, salida);
    } else if (
      (entrada.endsWith('.astro') || entrada.endsWith('.ts') || entrada.endsWith('.tsx')) &&
      // En un test la ruta es un DATO —la lista de superficies que debe servirse—
      // y no una llamada que pueda acabar en un 404.
      !entrada.includes('.test.')
    ) {
      salida.push(ruta);
    }
  }
  return salida;
}

test('el login se construye con el prefijo de la app, nunca desde la raíz', () => {
  const malas: string[] = [];
  const ficheros = [...paginas(APPS), ...paginas(SHELL), ...paginas(OFICINA)];
  for (const ruta of ficheros) {
    const texto = readFileSync(ruta, 'utf8');
    // `/auth/login` precedido de comilla o backtick = ruta absoluta desde la raíz
    // del host. Con `${bff.basePath}` delante, no casa.
    // Se mira la línea Y LA ANTERIOR: envolver en `withBase(` parte la llamada en
    // dos líneas cuando la ruta es larga, que es siempre, así que buscando solo en
    // la línea de la ruta se marcaba como malo lo que ya estaba bien. Un falso
    // positivo en un gate se paga peor que uno negativo: se aprende a ignorarlo.
    const lineas = texto.split('\n');
    for (let i = 0; i < lineas.length; i += 1) {
      const linea = lineas[i] ?? '';
      if (!/['"`]\/auth\/login/.test(linea)) continue;
      // Los comentarios hablan de la ruta para explicar por qué NO se escribe
      // así; marcarlos convierte el gate en ruido. `{/*` es la forma JSX, que
      // faltaba: en un `.tsx` el comentario que explica esto vive DENTRO del
      // marcado, y el gate lo marcaba como si fuera un enlace mal escrito.
      const limpio = linea.trim();
      if (
        limpio.startsWith('*') ||
        limpio.startsWith('//') ||
        limpio.startsWith('/*') ||
        limpio.startsWith('{/*')
      )
        continue;
      // Dos líneas de contexto: con la ruta larga, `withBase(` queda a dos de
      // distancia cuando el formateador parte también la llamada exterior.
      const contexto = `${lineas[i - 2] ?? ''}\n${lineas[i - 1] ?? ''}\n${linea}`;
      // Ya lleva el prefijo, por el objeto del BFF o por el ayudante del cliente.
      if (/withBase\(|basePath|\br\(/.test(contexto)) continue;
      malas.push(`${ruta}: ${linea.trim()}`);
    }
  }
  expect(malas).toEqual([]);
});
