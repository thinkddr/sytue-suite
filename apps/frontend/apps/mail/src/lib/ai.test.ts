/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { construirPeticion, leerRespuesta, MAX_CONTEXTO } from './ai.js';

test('cada acción lleva su directiva y el tipo es «email»', () => {
  const p = construirPeticion({ accion: 'acortar', texto: 'hola' });
  expect(p.action).toBe('acortar');
  expect(p.kind).toBe('email');
  expect(p.instruction).toContain('Acorta');
  expect(p.context).toBe('hola');
});

// Sin esto el modelo contesta «Claro, aquí tienes tu correo mejorado:» y ese
// preámbulo acaba pegado en el cuerpo del mensaje que alguien envía.
test('todas las directivas piden SOLO el texto', () => {
  const acciones = ['mejorar', 'corregir', 'acortar', 'formal', 'cercano', 'responder'] as const;
  for (const accion of acciones) {
    expect(construirPeticion({ accion }).instruction.toLowerCase()).toContain('devuelve solo');
  }
  expect(construirPeticion({ accion: 'traducir' }).instruction.toLowerCase()).toContain(
    'devuelve solo',
  );
});

test('traducir lleva el idioma pedido, y uno por defecto si no se elige', () => {
  expect(construirPeticion({ accion: 'traducir', idioma: 'alemán' }).instruction).toContain(
    'alemán',
  );
  expect(construirPeticion({ accion: 'traducir', idioma: '  ' }).instruction).toContain('inglés');
  expect(construirPeticion({ accion: 'traducir' }).instruction).toContain('inglés');
});

// El tope existe para no mandar una cadena de correo de cien mensajes al
// proveedor —se paga por token— ni chocar con el límite del servicio.
test('el contexto se recorta al tope', () => {
  const largo = 'x'.repeat(MAX_CONTEXTO + 500);
  expect(construirPeticion({ accion: 'mejorar', texto: largo }).context).toHaveLength(MAX_CONTEXTO);
});

test('sin texto, el contexto va vacío y no revienta', () => {
  expect(construirPeticion({ accion: 'mejorar' }).context).toBe('');
});

test('leerRespuesta acepta lo válido y rechaza lo demás', () => {
  expect(leerRespuesta({ text: '  hola  ' })).toBe('hola');
  expect(leerRespuesta({ text: '   ' })).toBeNull();
  expect(leerRespuesta({ text: 42 })).toBeNull();
  expect(leerRespuesta({})).toBeNull();
  expect(leerRespuesta(null)).toBeNull();
  expect(leerRespuesta('hola')).toBeNull();
});
