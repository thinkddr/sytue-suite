/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cifrado de cuadernos es la única pieza del producto donde un fallo no se ve:
 * una nota mal cifrada parece guardada. Estas comprobaciones son las que separan
 * «cifra» de «parece que cifra».
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
// El contenido descifrado se sanea con el analizador del navegador.
if (typeof document === 'undefined') GlobalRegistrator.register();

import { expect, test } from 'bun:test';
import {
  abrir,
  cifrarTexto,
  descifrarTexto,
  desempaquetar,
  empaquetar,
  nuevosParametros,
} from './crypto.js';

test('lo cifrado vuelve tal cual con la contraseña correcta', async () => {
  const { key } = await nuevosParametros('una contraseña larga');
  const cifrado = await cifrarTexto(key, 'secreto con acentos: ñáü 漢字');
  expect(cifrado.startsWith('encv1:')).toBe(true);
  expect(cifrado).not.toContain('secreto');
  expect(await descifrarTexto(key, cifrado)).toBe('secreto con acentos: ñáü 漢字');
});

test('una contraseña equivocada no abre el cuaderno', async () => {
  const { params } = await nuevosParametros('la buena');
  expect(await abrir(params, 'la mala')).toBeNull();
  expect(await abrir(params, 'la buena')).not.toBeNull();
});

test('el título viaja DENTRO del criptograma, no al lado', async () => {
  const { key } = await nuevosParametros('contraseña de prueba');
  const paquete = await empaquetar(key, { title: 'Contraseñas del banco', content: '<p>1234</p>' });
  expect(paquete).not.toContain('Contraseñas');
  expect(paquete).not.toContain('1234');
  const claro = await desempaquetar(key, paquete);
  expect(claro?.title).toBe('Contraseñas del banco');
  expect(claro?.content).toBe('<p>1234</p>');
});

test('un criptograma manipulado no descifra (AES-GCM autentica)', async () => {
  const { key } = await nuevosParametros('contraseña de prueba');
  const paquete = await empaquetar(key, { title: 'x', content: 'y' });
  // Se cambia un carácter del base64 por otro válido del alfabeto.
  const roto =
    paquete.slice(0, paquete.length - 2) + (paquete.at(-2) === 'A' ? 'B' : 'A') + paquete.at(-1);
  expect(await desempaquetar(key, roto)).toBeNull();
});

test('dos cuadernos con la MISMA contraseña no comparten sal', async () => {
  const a = await nuevosParametros('idéntica');
  const b = await nuevosParametros('idéntica');
  expect(a.params.salt).not.toBe(b.params.salt);
  // Y por tanto la clave de uno no abre el otro.
  expect(await descifrarTexto(a.key, b.params.check).catch(() => null)).toBeNull();
});

// XSS-03: el contenido de un cuaderno cifrado no lo puede sanear el servidor, y otra
// persona con la clave lo pinta como HTML. Sale de aquí saneado, con las casillas de las
// tareas intactas.
test('lo descifrado sale saneado, con las casillas de las tareas', async () => {
  const { key } = await nuevosParametros('contraseña de prueba');
  const hostil =
    '<p onclick="x()">a<img src="/a.png" onerror="alert(1)"><script>alert(2)</script></p>';
  const claro = await desempaquetar(key, await empaquetar(key, { title: 't', content: hostil }));
  expect(claro?.content).not.toMatch(/onclick|onerror|script|alert/);
  expect(claro?.content).toContain('<p>a<img src="/a.png"></p>');

  const tareas = '<ul><li><input type="checkbox" checked=""> comprar pan</li></ul>';
  const t = await desempaquetar(key, await empaquetar(key, { title: 't', content: tareas }));
  expect(t?.content).toBe(tareas);
});
