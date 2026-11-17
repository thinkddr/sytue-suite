/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Enlaces clicables de Calcus: el destino de HYPERLINK y las URL escritas en una celda,
 * con la lista cerrada de esquemas que se pueden abrir (http, https y mailto).
 */
import { expect, test } from 'bun:test';
import { Engine } from '@sytue/formula';
import {
  abrirEnlace,
  enlaceDeCelda,
  formulaDelDestino,
  hrefSeguro,
  rotuloDeEnlace,
} from '../src/editor/sheets/enlaces';

function motor(celdas: Record<string, string>): (f: string) => ReturnType<Engine['evaluate']> {
  const e = new Engine({ sheets: ['Hoja1'] });
  for (const [ref, v] of Object.entries(celdas)) e.setCell('Hoja1', ref, v);
  return (f) => e.evaluate('Hoja1', f, 0, 0);
}

const sinMotor = (): never => {
  throw new Error('no debía evaluar');
};

test('solo http, https y mailto son enlaces que se abren', () => {
  expect(hrefSeguro('https://sytue.eu/precios')).toBe('https://sytue.eu/precios');
  expect(hrefSeguro('  HTTP://Ejemplo.org ')).toBe('http://ejemplo.org/');
  expect(hrefSeguro('mailto:ana@ejemplo.org')).toBe('mailto:ana@ejemplo.org');
  for (const malo of [
    'javascript:alert(1)',
    'JavaScript:alert(1)',
    'data:text/html,<b>x</b>',
    'file:///etc/passwd',
    'vbscript:msgbox',
    'ftp://ejemplo.org',
  ]) {
    expect(hrefSeguro(malo)).toBeNull();
  }
});

test('un texto que no es una URL completa no se vuelve enlace', () => {
  expect(hrefSeguro('www.ejemplo.org')).toBeNull();
  expect(hrefSeguro('https://ejemplo.org y más')).toBeNull();
  expect(hrefSeguro('http://')).toBeNull();
  expect(hrefSeguro('mailto:')).toBeNull();
  expect(hrefSeguro('Total')).toBeNull();
  expect(hrefSeguro('')).toBeNull();
});

test('una celda con una URL escrita es un enlace, con o sin formato', () => {
  expect(enlaceDeCelda('https://ejemplo.org/a', sinMotor)).toBe('https://ejemplo.org/a');
  expect(enlaceDeCelda('<b>https://ejemplo.org/a?x=1&amp;y=2</b>', sinMotor)).toBe(
    'https://ejemplo.org/a?x=1&y=2',
  );
  expect(enlaceDeCelda('hola', sinMotor)).toBeNull();
  expect(enlaceDeCelda('', sinMotor)).toBeNull();
});

test('HYPERLINK con destino literal da ese destino aunque el valor sea el nombre', () => {
  expect(enlaceDeCelda('=HYPERLINK("https://ejemplo.org";"Ejemplo")', sinMotor)).toBe(
    'https://ejemplo.org/',
  );
  expect(enlaceDeCelda('=hyperlink("mailto:ana@ejemplo.org")', sinMotor)).toBe(
    'mailto:ana@ejemplo.org',
  );
  expect(enlaceDeCelda('=HYPERLINK("javascript:alert(1)";"Pulsa")', sinMotor)).toBeNull();
});

test('HYPERLINK con destino calculado lo evalúa con el motor de la hoja', () => {
  const evaluar = motor({ A1: 'https://ejemplo.org', B1: 'javascript:alert(1)' });
  expect(enlaceDeCelda('=HYPERLINK(A1&"/ficha?id="&2; "Ficha")', evaluar)).toBe(
    'https://ejemplo.org/ficha?id=2',
  );
  expect(enlaceDeCelda('=HYPERLINK(B1)', evaluar)).toBeNull();
  expect(enlaceDeCelda('=HYPERLINK(42)', evaluar)).toBeNull();
});

test('solo cuenta un HYPERLINK de raíz; otras fórmulas no son enlaces', () => {
  const evaluar = motor({ A1: 'https://ejemplo.org' });
  expect(enlaceDeCelda('=A1', evaluar)).toBeNull();
  expect(enlaceDeCelda('=UPPER(HYPERLINK("https://ejemplo.org"))', evaluar)).toBeNull();
  expect(enlaceDeCelda('=HYPERLINK("https://a.org";"b";"c")', evaluar)).toBeNull();
  expect(enlaceDeCelda('=HYPERLINK(', evaluar)).toBeNull();
});

test('la fórmula del destino conserva los argumentos y su separador', () => {
  expect(formulaDelDestino('= hyperlink (A1; "x")')).toBe('=IF(TRUE,A1; "x")');
  expect(formulaDelDestino('=SUM(A1)')).toBeNull();
});

test('el chip enseña la máquina o la dirección de correo', () => {
  expect(rotuloDeEnlace('https://www.ejemplo.org/a/b')).toBe('ejemplo.org');
  expect(rotuloDeEnlace('mailto:ana%40x@ejemplo.org')).toBe('ana@x@ejemplo.org');
});

test('abrir usa otra pestaña con noopener y noreferrer, y nunca un esquema vetado', () => {
  const llamadas: unknown[][] = [];
  const abrir = (...a: unknown[]): void => {
    llamadas.push(a);
  };
  expect(abrirEnlace('https://ejemplo.org', abrir)).toBe(true);
  expect(abrirEnlace('javascript:alert(1)', abrir)).toBe(false);
  expect(llamadas).toEqual([['https://ejemplo.org/', '_blank', 'noopener,noreferrer']]);
});
