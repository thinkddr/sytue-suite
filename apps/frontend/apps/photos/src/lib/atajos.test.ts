/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La tabla de atajos de Fotos: cada tecla que cuenta la hoja hace algo, y cada
 * cosa que hace el teclado está en la hoja.
 */
import { expect, test } from 'bun:test';
import { useT as traductor } from '../i18n/index.js';
import { accionDeGaleria, accionDelVisor, gruposDeAtajos } from './atajos.js';

const t = traductor('es');

test('la galería: «u» sube, Escape sale de la selección y «?» abre la hoja', () => {
  expect(accionDeGaleria('u')).toBe('subir');
  expect(accionDeGaleria('U')).toBe('subir');
  expect(accionDeGaleria('Escape')).toBe('salirDeSeleccion');
  expect(accionDeGaleria('?')).toBe('ayuda');
  // Las del visor no son de la galería.
  expect(accionDeGaleria('ArrowLeft')).toBeNull();
  expect(accionDeGaleria('i')).toBeNull();
});

test('el visor: flechas, Inicio y Fin, Espacio, I, Z, Supr y «?»', () => {
  expect(accionDelVisor('ArrowLeft')).toBe('anterior');
  expect(accionDelVisor('ArrowRight')).toBe('siguiente');
  expect(accionDelVisor('Home')).toBe('primera');
  expect(accionDelVisor('End')).toBe('ultima');
  expect(accionDelVisor(' ')).toBe('pase');
  // Con el bloqueo de mayúsculas puesto, la letra llega en mayúscula.
  expect(accionDelVisor('i')).toBe('ficha');
  expect(accionDelVisor('I')).toBe('ficha');
  expect(accionDelVisor('z')).toBe('tamanyoReal');
  expect(accionDelVisor('Z')).toBe('tamanyoReal');
  expect(accionDelVisor('Delete')).toBe('papelera');
  expect(accionDelVisor('Backspace')).toBe('papelera');
  expect(accionDelVisor('?')).toBe('ayuda');
  expect(accionDelVisor('u')).toBeNull();
  expect(accionDelVisor('ArrowUp')).toBeNull();
});

test('la hoja de la galería trae los dos grupos; la del visor, solo el suyo', () => {
  const galeria = gruposDeAtajos(t, 'galeria');
  expect(galeria.map((g) => g.titulo)).toEqual([
    t('shortcuts.group.gallery'),
    t('shortcuts.group.viewer'),
  ]);
  expect(galeria[0]!.atajos.map((a) => a.teclas)).toEqual([['U'], ['Esc'], ['?']]);
  const visor = gruposDeAtajos(t, 'visor');
  expect(visor.map((g) => g.titulo)).toEqual([t('shortcuts.group.viewer')]);
  expect(visor[0]!.atajos.map((a) => a.teclas)).toEqual([
    ['←'],
    ['→'],
    [t('shortcuts.keyHome')],
    [t('shortcuts.keyEnd')],
    [t('shortcuts.keySpace')],
    ['I'],
    ['Z'],
    [t('shortcuts.keyDelete'), '⌫'],
    ['Esc'],
    ['?'],
  ]);
  // Las teclas con nombre van en el idioma de quien lee.
  expect(t('shortcuts.keyHome')).toBe('Inicio');
  expect(t('shortcuts.keyDelete')).toBe('Supr');
});

test('la hoja no promete lo que ese visor no atiende', () => {
  const descripciones = (o: { saltos?: boolean; papelera?: boolean }) =>
    gruposDeAtajos(t, 'visor', o)[0]!.atajos.map((a) => a.descripcion);
  const sinNada = descripciones({ saltos: false, papelera: false });
  expect(sinNada).not.toContain(t('shortcuts.first'));
  expect(sinNada).not.toContain(t('shortcuts.last'));
  expect(sinNada).not.toContain(t('viewer.delete'));
  expect(sinNada).toContain(t('shortcuts.info'));
  const todo = descripciones({});
  expect(todo).toContain(t('shortcuts.first'));
  expect(todo).toContain(t('viewer.delete'));
});
