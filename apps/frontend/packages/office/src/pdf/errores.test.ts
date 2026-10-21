/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LO QUE SE VE CUANDO ALGO VA MAL.
 *
 * Medido con el motor real (arnés del frontal, 2026-10-16): un PDF con contraseña
 * enseñaba «pdf_failed» en rojo en vez de pedirla, y uno roto, lo mismo. El BFF leía
 * el código en la raíz del cuerpo y `httpx.Fail` lo manda en un sobre; y lo que no
 * tenía traducción se enseñaba tal cual. Esto fija las dos mitades: el código se lee
 * del sobre, y lo que no se sabe traducir dice lo que se intentaba, nunca un código ni
 * la frase en inglés del motor.
 */
import { expect, test } from 'bun:test';
import { cuerpoDeError } from '@sytue/api-clients';
import { docsT } from '../i18n';
import { ErrorPDF, pideContrasena } from './api';
import { ERRORES, mensajeDeError } from './errores';

const t = docsT('es');

test('el código del motor llega aunque venga en el sobre de httpx', () => {
  const motor = '{"error":{"code":"pdf_password_required","message":"“a.pdf” needs a password"}}';
  const { code } = cuerpoDeError(motor);
  expect(code).toBe('pdf_password_required');
  // Y con él la interfaz PIDE la contraseña en vez de enseñar un error.
  expect(pideContrasena(new ErrorPDF('x', code, 400))).toBe(true);
});

test('cada código conocido dice lo suyo, en el idioma de la interfaz', () => {
  for (const [codigo, clave] of Object.entries(ERRORES)) {
    const frase = mensajeDeError(new ErrorPDF('In English', codigo, 400), t, 'pdf.error.open');
    expect(frase).toBe(t(clave));
    expect(frase).not.toContain(codigo);
  }
  // «No es un PDF» y «está dañado» ya no se ven igual.
  expect(t(ERRORES['pdf_not_pdf']!)).not.toBe(t(ERRORES['pdf_unreadable']!));
});

test('lo que no se sabe traducir no sale crudo ni en inglés', () => {
  for (const codigo of ['pdf_failed', 'bad_request', 'algo_nuevo', undefined]) {
    const e = new ErrorPDF('“a.pdf” could not be read.', codigo, 400);
    expect(mensajeDeError(e, t, 'pdf.error.open')).toBe(t('pdf.error.open'));
  }
  // Lo que no llega a ser una respuesta es la red, no el documento.
  expect(mensajeDeError(new TypeError('fetch failed'), t, 'pdf.error.open')).toBe(
    t('pdf.error.offline'),
  );
});
