/**
 * La lógica de la paleta de la suite, sin DOM: qué casa con qué capa a capa, el tope
 * por capa, lo que ya viene buscado del servidor, el registro y el atajo.
 * Ejecutar: `bun test` en packages/shell.
 */
import { expect, test } from 'bun:test';
import {
  comandosQueCasan,
  crearRegistroDePaleta,
  esAtajoDePaleta,
  type CapaDePaleta,
  type ComandoDeApp,
} from './paleta.js';

const c = (id: string, rotulo: string, grupo: string, extra: Partial<ComandoDeApp> = {}) => ({
  id,
  rotulo,
  grupo,
  ejecutar: () => {},
  ...extra,
});

const secciones: CapaDePaleta = {
  comandos: [
    c('a', 'Salas de reunión', 'Secciones'),
    c('b', 'Auditoría', 'Secciones', { palabras: 'audit log' }),
    c('c', 'Crear grupo', 'Acciones', { pista: 'Grupos' }),
  ],
};

test('consulta vacía: todo, capa a capa y en su orden', () => {
  const otra: CapaDePaleta = { comandos: [c('z', 'Zeta', 'Otra')] };
  expect(comandosQueCasan([secciones, otra], '').map((x) => x.id)).toEqual(['a', 'b', 'c', 'z']);
});

test('pliega acentos y busca en rótulo, grupo, palabras y pista', () => {
  expect(comandosQueCasan([secciones], 'reunion').map((x) => x.id)).toEqual(['a']);
  expect(comandosQueCasan([secciones], 'LOG').map((x) => x.id)).toEqual(['b']);
  expect(comandosQueCasan([secciones], 'acciones').map((x) => x.id)).toEqual(['c']);
  expect(comandosQueCasan([secciones], 'grupos').map((x) => x.id)).toEqual(['c']);
  expect(comandosQueCasan([secciones], 'nada que ver')).toEqual([]);
});

test('el tope es por capa y se aplica después de filtrar', () => {
  const muchos: CapaDePaleta = {
    comandos: Array.from({ length: 30 }, (_, i) => c(`m${i}`, `Modelo ${i}`, 'Ir a')),
    limite: 12,
  };
  expect(comandosQueCasan([muchos], '')).toHaveLength(12);
  // «Modelo 7», «Modelo 17» y «Modelo 27»: filtrar va antes que recortar.
  expect(comandosQueCasan([muchos], 'modelo 7').map((x) => x.id)).toEqual(['m7', 'm17', 'm27']);
  expect(comandosQueCasan([muchos], 'modelo 29').map((x) => x.id)).toEqual(['m29']);
  expect(comandosQueCasan([muchos, secciones], '')).toHaveLength(15);
});

test('lo que ya viene buscado del servidor no vuelve a filtrarse', () => {
  const personas: CapaDePaleta = {
    comandos: [
      c('s', 'Usuarios', 'Secciones'),
      c('p', 'Ana García', 'Personas', { yaFiltrado: true, pista: 'ana@x.test' }),
    ],
  };
  // «ana@» no casa con «Ana García · Personas» por palabras, pero el servidor ya dijo que sí.
  expect(comandosQueCasan([personas], 'ana@').map((x) => x.id)).toEqual(['p']);
});

test('el registro: poner, sustituir, quitar y avisar solo cuando cambia', () => {
  const r = crearRegistroDePaleta();
  let avisos = 0;
  const dejar = r.suscribir(() => avisos++);
  r.poner('erp', secciones);
  r.poner('erp', secciones);
  expect(avisos).toBe(1);
  const nueva: CapaDePaleta = { comandos: [] };
  r.poner('erp', nueva);
  r.poner('cloud', secciones);
  expect(r.capas()).toEqual([nueva, secciones]);
  r.quitar('erp');
  r.quitar('erp');
  expect(r.capas()).toEqual([secciones]);
  expect(avisos).toBe(4);
  dejar();
  r.quitar('cloud');
  expect(avisos).toBe(4);
});

test('⌘K y Ctrl+K abren; con Mayúsculas o Alt no (son del navegador)', () => {
  const k = { key: 'k', metaKey: false, ctrlKey: false, altKey: false, shiftKey: false };
  expect(esAtajoDePaleta({ ...k, metaKey: true })).toBe(true);
  expect(esAtajoDePaleta({ ...k, ctrlKey: true, key: 'K' })).toBe(true);
  expect(esAtajoDePaleta({ ...k, ctrlKey: true, shiftKey: true })).toBe(false);
  expect(esAtajoDePaleta({ ...k, ctrlKey: true, altKey: true })).toBe(false);
  expect(esAtajoDePaleta(k)).toBe(false);
});
