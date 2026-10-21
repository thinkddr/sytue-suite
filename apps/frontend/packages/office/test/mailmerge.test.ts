/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las ayudas PURAS de la combinación de correspondencia:
 *  - parseMergeSource: CSV/TSV pegado → {fields, records} (delimitador, comillas,
 *    filas vacías, celdas faltantes, BOM, «;» europeo).
 *  - mergeFieldHtml / substituteMergeFields: marcador de campo y su sustitución
 *    por el valor del registro ESCAPADO (frontera de confianza, anti-XSS).
 * Sin DOM (bun corre sin navegador). Ejecutar:
 *   cd apps/frontend && bun test packages/office/test/mailmerge.test.ts
 */
import { expect, test } from 'bun:test';
import { mergeFieldHtml, parseMergeSource, substituteMergeFields } from '../src/editor/mailmerge';

test('parseMergeSource: CSV con cabecera y registros', () => {
  const src = parseMergeSource('Nombre,Email\nAna,ana@x.com\nLuis,luis@x.com');
  expect(src.fields).toEqual(['Nombre', 'Email']);
  expect(src.records).toEqual([
    { Nombre: 'Ana', Email: 'ana@x.com' },
    { Nombre: 'Luis', Email: 'luis@x.com' },
  ]);
});

test('parseMergeSource: TSV y celdas faltantes → ""', () => {
  const src = parseMergeSource('A\tB\tC\n1\t2');
  expect(src.fields).toEqual(['A', 'B', 'C']);
  expect(src.records).toEqual([{ A: '1', B: '2', C: '' }]);
});

test('parseMergeSource: comillas con comas, comillas escapadas y salto interno', () => {
  const src = parseMergeSource('Nombre,Nota\n"Pérez, Ana","dijo ""hola"""\n"multi\nlínea",ok');
  expect(src.fields).toEqual(['Nombre', 'Nota']);
  expect(src.records).toEqual([
    { Nombre: 'Pérez, Ana', Nota: 'dijo "hola"' },
    { Nombre: 'multi\nlínea', Nota: 'ok' },
  ]);
});

test('parseMergeSource: filas vacías descartadas; BOM y «;» europeo', () => {
  const src = parseMergeSource('﻿Ciudad;País\n\nMadrid;España\n');
  expect(src.fields).toEqual(['Ciudad', 'País']);
  expect(src.records).toEqual([{ Ciudad: 'Madrid', País: 'España' }]);
});

test('parseMergeSource: cabecera vacía → «Campo N»; origen en blanco → sin campos', () => {
  expect(parseMergeSource('   ')).toEqual({ fields: [], records: [] });
  const src = parseMergeSource('Nombre,,Ciudad\nAna,x,Madrid');
  expect(src.fields).toEqual(['Nombre', 'Campo 2', 'Ciudad']);
  expect(src.records).toEqual([{ Nombre: 'Ana', 'Campo 2': 'x', Ciudad: 'Madrid' }]);
});

test('mergeFieldHtml: escapa el nombre del campo en atributo y texto', () => {
  expect(mergeFieldHtml('Nombre')).toBe(
    '<span data-mergefield="Nombre" contenteditable="false">«Nombre»</span>',
  );
  expect(mergeFieldHtml('R&D "x"')).toBe(
    '<span data-mergefield="R&amp;D &quot;x&quot;" contenteditable="false">«R&amp;D &quot;x&quot;»</span>',
  );
});

test('substituteMergeFields: sustituye por el valor ESCAPADO (anti-XSS)', () => {
  const tpl =
    'Hola <b><span data-mergefield="Nombre" contenteditable="false">«Nombre»</span></b>, ' +
    '<span data-mergefield="Nota" contenteditable="false">«Nota»</span>';
  const out = substituteMergeFields(tpl, {
    Nombre: 'Ana',
    Nota: '<img src=x onerror=alert(1)>',
  });
  expect(out).toBe('Hola <b>Ana</b>, &lt;img src=x onerror=alert(1)&gt;');
});

test('substituteMergeFields: campo ausente → vacío; nombre con «&»', () => {
  const tpl =
    '<span data-mergefield="R&amp;D" contenteditable="false">«R&amp;D»</span>|' +
    '<span contenteditable="false" data-mergefield="X">«X»</span>'; // atributos en otro orden
  expect(substituteMergeFields(tpl, { 'R&D': 'ok' })).toBe('ok|');
});
