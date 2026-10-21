// Permisos sobre los OBJETOS DE DATOS de una hoja protegida: tablas, segmentadores,
// escenarios, tablas dinámicas, tablas de datos y filtros. Ejecutar: `bun test`.
//
// Esto vigila la política de la UI y del modelo (qué comando pasa con la hoja protegida y
// con qué permiso). La validación en el SERVICIO (rechazar la escritura aunque el cliente
// la salte) es de la pista de protección: no se repite aquí.
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  isPermissionGranted,
  sanitizeProtection,
  SHEET_PERMISSIONS,
  type SheetPermission,
} from '../src/editor/sheets/format';
import {
  PROTECT_CONTENT_CMDS,
  PROTECT_FORMAT_CMDS,
  PROTECT_PERMISSION_CMDS,
} from '../src/editor/sheets/SheetCanvas';
import type { SheetCommand } from '../src/editor/sheets/SheetCanvas';

/** Los comandos del lienzo, leídos del tipo `SheetCommand` (no hay otra lista en tiempo de ejecución). */
const COMANDOS = ((): string[] => {
  const src = readFileSync(
    new URL('../src/editor/sheets/SheetCanvas.tsx', import.meta.url),
    'utf8',
  );
  const bloque = /export type SheetCommand =([\s\S]*?);\n/.exec(src)?.[1] ?? '';
  return [...bloque.matchAll(/\|\s*'([a-z0-9]+)'/g)].map((m) => m[1]!);
})();

/** Comandos que tocan un objeto de datos (o su filtro): hay que decidir qué pasa con ellos. */
const DE_DATOS =
  /^(table|pivot|slicer|scen|datatable|extdata|goalseek|solver|filter|autofilter|name)/;

/** Los que pasan con la hoja protegida SIN permiso, y por qué. Añadir uno es una decisión. */
const PERMITIDOS: Record<string, string> = {
  nameset: 'los nombres son del LIBRO; el lienzo mira la protección al reescribir fórmulas',
  namedel: 'idem nameset',
};

test('los comandos de datos que escriben celdas están vetados en hoja protegida', () => {
  for (const c of [
    'pivot',
    'pivotrefresh',
    'pivotdel',
    'tableadd',
    // Cambian `tables` aunque no toquen celdas, y el servicio no lo deja con la hoja protegida.
    'tabledel',
    'tablestyle',
    'tabletotals',
    'tabletotalfn',
    'extdata',
    'datatable',
    'goalseek',
    'solverapply',
    'scenapply',
    'scenreport',
  ] as SheetCommand[])
    expect(PROTECT_CONTENT_CMDS.has(c)).toBe(true);
});

test('cada objeto de datos pide su permiso granular', () => {
  const esperado: Record<string, SheetPermission> = {
    sliceradd: 'objects',
    scenadd: 'scenarios',
    scenupdate: 'scenarios',
    scendel: 'scenarios',
    autofilter: 'autoFilter',
    filterset: 'autoFilter',
    filtercond: 'autoFilter',
    filtersort: 'sort',
  };
  for (const [cmd, perm] of Object.entries(esperado))
    expect(PROTECT_PERMISSION_CMDS.get(cmd as SheetCommand)).toBe(perm);
});

test('guardián: ningún comando de datos queda sin decidir', () => {
  expect(COMANDOS.length).toBeGreaterThan(60); // la lectura del tipo funciona
  const sinDecidir = COMANDOS.filter(
    (c) =>
      DE_DATOS.test(c) &&
      !PROTECT_CONTENT_CMDS.has(c as SheetCommand) &&
      !PROTECT_PERMISSION_CMDS.has(c as SheetCommand) &&
      !(c in PERMITIDOS),
  );
  expect(sinDecidir).toEqual([]);
});

test('guardián: PERMITIDOS no se queda con entradas que ya no hacen falta', () => {
  for (const c of Object.keys(PERMITIDOS)) {
    expect(COMANDOS).toContain(c); // el comando existe
    expect(PROTECT_CONTENT_CMDS.has(c as SheetCommand)).toBe(false); // y sigue sin vetarse
  }
});

test('ningún comando de formato de celda mueve un objeto de datos', () => {
  for (const c of PROTECT_FORMAT_CMDS) expect(DE_DATOS.test(c)).toBe(false);
});

test('con la hoja protegida, ninguno de los permisos de datos viene concedido', () => {
  const prot = sanitizeProtection({})!;
  for (const perm of ['objects', 'scenarios', 'autoFilter', 'sort'] as const) {
    expect(SHEET_PERMISSIONS).toContain(perm);
    expect(isPermissionGranted(prot, perm)).toBe(false);
  }
  // Y conceder uno no concede los demás.
  const solo = sanitizeProtection({ scenarios: true })!;
  expect(isPermissionGranted(solo, 'scenarios')).toBe(true);
  expect(isPermissionGranted(solo, 'objects')).toBe(false);
  expect(isPermissionGranted(solo, 'autoFilter')).toBe(false);
});
