/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la lógica pura de PROTECCIÓN de hoja, celda y LIBRO (paridad Excel):
 * predicados «¿puedo editar esta celda?» y «¿puedo modificar la estructura?»,
 * ocultado de fórmula, y saneado de la protección de hoja/libro + de los flags de
 * celda (locked/hidden) que solo persisten cuando difieren del valor por defecto.
 * Ejecutar: cd apps/frontend && bun test.
 */
import { expect, test } from 'bun:test';
import {
  canEditCell,
  canEditCellContent,
  canEditStructure,
  estiloParaMi,
  isFormulaHidden,
  sanitizeCellStyle,
  sanitizeProtection,
  sanitizeWorkbookProtection,
  SHEET_PERMISSIONS,
  isPermissionGranted,
} from '../src/editor/sheets/format';
import {
  AMBITO_LIBRO,
  ambitoHoja,
  esPropietario,
  fijarPropietario,
  olvidarProteccion,
  presentarClave,
  presentarNueva,
  sobreProteccion,
} from '../src/editor/state/proteccion';

/* ---------- ¿Puedo editar esta celda? (predicado central de la tarea) ---------- */

test('canEditCell: sin proteger, toda celda es editable', () => {
  expect(canEditCell(false, undefined)).toBe(true);
  expect(canEditCell(false, { locked: true })).toBe(true);
  expect(canEditCell(false, { locked: false })).toBe(true);
});

test('canEditCell: protegida ⇒ solo las desbloqueadas; el bloqueo por defecto es TRUE', () => {
  expect(canEditCell(true, undefined)).toBe(false); // sin estilo = bloqueada (Excel)
  expect(canEditCell(true, {})).toBe(false); // estilo sin locked = bloqueada
  expect(canEditCell(true, { locked: true })).toBe(false);
  expect(canEditCell(true, { fill: '#FFEE00' })).toBe(false); // otro formato no desbloquea
  expect(canEditCell(true, { locked: false })).toBe(true); // desbloqueo explícito
});

test('isFormulaHidden: oculta solo con hoja protegida y hidden=true', () => {
  expect(isFormulaHidden(true, { hidden: true })).toBe(true);
  expect(isFormulaHidden(false, { hidden: true })).toBe(false); // sin proteger no oculta
  expect(isFormulaHidden(true, { hidden: false })).toBe(false);
  expect(isFormulaHidden(true, {})).toBe(false);
  expect(isFormulaHidden(true, undefined)).toBe(false);
});

test('canEditCellContent: una FÓRMULA OCULTA no es editable aunque la celda esté desbloqueada', () => {
  // Combo peligroso: desbloqueada + hidden + fórmula. Editarla por barra/celda la
  // revelaría (el editor muestra el «=…») o la borraría (la barra se vacía al ocultar).
  expect(canEditCellContent(true, { locked: false, hidden: true }, '=SUM(A1:A2)')).toBe(false);
  // Mismo estilo pero con VALOR (no fórmula): nada que ocultar ⇒ editable.
  expect(canEditCellContent(true, { locked: false, hidden: true }, '42')).toBe(true);
  // Vacía: se puede teclear en ella (aún no hay fórmula que proteger).
  expect(canEditCellContent(true, { locked: false, hidden: true }, '')).toBe(true);
  // Desbloqueada, con fórmula pero SIN hidden ⇒ editable (se ve y se edita normal).
  expect(canEditCellContent(true, { locked: false }, '=A1')).toBe(true);
  // Bloqueada: no editable, haya o no fórmula (delega en canEditCell).
  expect(canEditCellContent(true, { hidden: true }, '=A1')).toBe(false);
  expect(canEditCellContent(true, undefined, '=A1')).toBe(false);
  // Sin proteger: todo editable, incluso una fórmula marcada hidden.
  expect(canEditCellContent(false, { locked: false, hidden: true }, '=A1')).toBe(true);
});

/* ---------- Saneado: los flags de celda solo persisten fuera de su defecto ---------- */

test('sanitizeCellStyle: guarda locked=false y hidden=true; nunca los valores por defecto', () => {
  expect(sanitizeCellStyle({ locked: false })).toEqual({ locked: false });
  expect(sanitizeCellStyle({ hidden: true })).toEqual({ hidden: true });
  // locked=true (defecto) y hidden=false (defecto) no se guardan ⇒ objeto vacío = null
  expect(sanitizeCellStyle({ locked: true })).toBeNull();
  expect(sanitizeCellStyle({ hidden: false })).toBeNull();
  // conviven con el formato visual sin pisarlo
  expect(sanitizeCellStyle({ u: true, locked: false, hidden: true })).toEqual({
    u: true,
    locked: false,
    hidden: true,
  });
});

/* ---------- Saneado de la protección de hoja (presencia = protegida) ---------- */

test('sanitizeProtection: null solo si no es objeto; {} = protegida con opciones por defecto', () => {
  expect(sanitizeProtection(null)).toBeNull();
  expect(sanitizeProtection('x')).toBeNull();
  expect(sanitizeProtection(42)).toBeNull();
  expect(sanitizeProtection({})).toEqual({}); // protegida, sin contraseña, defaults
});

test('sanitizeProtection: la contraseña NUNCA entra en el modelo; opciones solo si difieren del defecto', () => {
  // La contraseña va al servidor aparte (state/proteccion.ts); lo único que el modelo
  // dice es que HAY una, y eso lo fija el servidor (hasPassword).
  expect(sanitizeProtection({ password: 'ab' })).toEqual({});
  expect(sanitizeProtection({ hasPassword: true })).toEqual({ hasPassword: true });
  expect(sanitizeProtection({ hasPassword: 'sí' })).toEqual({});
  // selectCells por defecto true (solo se guarda el false); formatCells por defecto false (solo el true)
  expect(sanitizeProtection({ selectCells: true, formatCells: false })).toEqual({});
  expect(sanitizeProtection({ selectCells: false, formatCells: true })).toEqual({
    selectCells: false,
    formatCells: true,
  });
});

/* ---------- Protección de LIBRO: ¿puedo modificar la estructura? ---------- */

test('canEditStructure: sin protección de libro se puede; con protección (aunque {}) no', () => {
  expect(canEditStructure(undefined)).toBe(true); // libro sin proteger
  expect(canEditStructure({})).toBe(false); // presencia = protegido, sin contraseña
  expect(canEditStructure({ hasPassword: true })).toBe(false); // protegido con contraseña
});

test('sanitizeWorkbookProtection: null solo si no es objeto; presencia = protegido; la contraseña no entra', () => {
  expect(sanitizeWorkbookProtection(null)).toBeNull();
  expect(sanitizeWorkbookProtection('x')).toBeNull();
  expect(sanitizeWorkbookProtection(42)).toBeNull();
  expect(sanitizeWorkbookProtection({})).toEqual({}); // protegido, sin contraseña
  expect(sanitizeWorkbookProtection({ password: 'ab' })).toEqual({});
  expect(sanitizeWorkbookProtection({ hasPassword: true })).toEqual({ hasPassword: true });
  // ignora campos ajenos (no hay opciones a nivel libro salvo el indicador de contraseña)
  expect(sanitizeWorkbookProtection({ hasPassword: true, foo: 1 })).toEqual({ hasPassword: true });
});

/* ---------- Permisos granulares del cuadro «Proteger hoja» (paridad Excel) ---------- */

test('permisos granulares: ausente = NO concedido (criterio Excel)', () => {
  const prot = sanitizeProtection({})!;
  for (const p of SHEET_PERMISSIONS) expect(isPermissionGranted(prot, p)).toBe(false);
  // Sin protección todo vale (el predicado no depende de la hoja).
  for (const p of SHEET_PERMISSIONS) expect(isPermissionGranted(undefined, p)).toBe(true);
});

test('permisos granulares: solo se concede lo marcado', () => {
  const prot = sanitizeProtection({ insertRows: true, sort: true })!;
  expect(isPermissionGranted(prot, 'insertRows')).toBe(true);
  expect(isPermissionGranted(prot, 'sort')).toBe(true);
  expect(isPermissionGranted(prot, 'deleteRows')).toBe(false);
  expect(isPermissionGranted(prot, 'objects')).toBe(false);
  expect(isPermissionGranted(prot, 'scenarios')).toBe(false);
});

test('saneado: persiste solo los permisos CONCEDIDOS (JSON mínimo) y descarta basura', () => {
  expect(sanitizeProtection({ insertRows: true, deleteRows: false })).toEqual({ insertRows: true });
  // Valores no booleanos no conceden nada (entrada no confiable de la persistencia).
  expect(sanitizeProtection({ sort: 'sí', objects: 1, scenarios: null })).toEqual({});
  // Convive con los campos históricos sin pisarlos.
  expect(sanitizeProtection({ hasPassword: true, formatCells: true, objects: true })).toEqual({
    hasPassword: true,
    formatCells: true,
    objects: true,
  });
});

test('retrocompatible: una hoja protegida antigua no concede ningún permiso nuevo', () => {
  const viejo = sanitizeProtection({ hasPassword: true, selectCells: false })!;
  expect(viejo).toEqual({ hasPassword: true, selectCells: false });
  for (const p of SHEET_PERMISSIONS) expect(isPermissionGranted(viejo, p)).toBe(false);
});

/* ---------- Contraseñas en tránsito hacia el servidor (state/proteccion.ts) ---------- */

test('proteccion: la contraseña viaja en un sobre aparte y se olvida al confirmar', () => {
  olvidarProteccion();
  expect(sobreProteccion()).toBeUndefined(); // nada pendiente: el cuerpo no lleva el campo
  presentarNueva(ambitoHoja('Cuentas'), 's3creta');
  presentarNueva(AMBITO_LIBRO, '');
  expect(sobreProteccion()).toEqual({ new: { 'hoja:Cuentas': 's3creta' } }); // vacía = sin contraseña
  presentarClave('vigente');
  expect(sobreProteccion()).toEqual({ password: 'vigente', new: { 'hoja:Cuentas': 's3creta' } });
  olvidarProteccion();
  expect(sobreProteccion()).toBeUndefined();
});

test('proteccion: los ámbitos coinciden con los del servicio Go (libro / hoja:<nombre>)', () => {
  // domain.AmbitoLibro y domain.AmbitoHoja; si cambian allí, esto debe cambiar aquí.
  expect(AMBITO_LIBRO).toBe('libro');
  expect(ambitoHoja('Hoja 1')).toBe('hoja:Hoja 1');
});

test('proteccion: solo el propietario figura como tal', () => {
  fijarPropietario(false);
  expect(esPropietario()).toBe(false);
  fijarPropietario(true);
  expect(esPropietario()).toBe(true);
  fijarPropietario(false);
});

/* ---------- Rangos editables por persona o grupo (permisos-2) ---------- */

test('estiloParaMi: desbloquea solo las celdas del rango concedido a esa persona', () => {
  const protect = {
    ranges: [
      { ref: 'B2:B3', users: ['ana'] },
      { ref: 'D1', groups: ['conta'] },
    ],
  };
  const ana = { sub: 'ana' };
  expect(canEditCell(true, estiloParaMi(protect, 'B2', undefined, ana))).toBe(true);
  expect(canEditCell(true, estiloParaMi(protect, 'B3', { locked: true }, ana))).toBe(true);
  expect(canEditCell(true, estiloParaMi(protect, 'B4', undefined, ana))).toBe(false);
  expect(canEditCell(true, estiloParaMi(protect, 'B2', undefined, { sub: 'luis' }))).toBe(false);
  expect(canEditCell(true, estiloParaMi(protect, 'B2', undefined, { sub: '' }))).toBe(false);
  // El resto del estilo se conserva.
  expect(estiloParaMi(protect, 'B2', { color: '#ff0000' }, ana)).toEqual({
    color: '#ff0000',
    locked: false,
  });
  // Grupo: lo conoce el cliente solo si se le pasa.
  expect(
    canEditCell(true, estiloParaMi(protect, 'D1', undefined, { sub: 'x', groups: ['conta'] })),
  ).toBe(true);
  // Sin rangos, el estilo pasa tal cual.
  expect(estiloParaMi({}, 'B2', undefined, ana)).toBeUndefined();
});

test('sanitizeProtection: conserva los rangos legibles con alguien concedido y descarta el resto', () => {
  const p = sanitizeProtection({
    ranges: [
      { ref: 'b2:$d$10', users: ['ana', '', 7] },
      { ref: 'no-es-un-rango', users: ['ana'] },
      { ref: 'A1' },
      { ref: 'C1', groups: ['conta'] },
      'basura',
    ],
  });
  expect(p?.ranges).toEqual([
    { ref: 'B2:D10', users: ['ana'] },
    { ref: 'C1', groups: ['conta'] },
  ]);
  expect(sanitizeProtection({ ranges: 'A1' })?.ranges).toBeUndefined();
});
