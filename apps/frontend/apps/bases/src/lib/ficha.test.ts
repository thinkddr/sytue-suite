// La ficha de un registro: qué se escribe al guardar. Son los casos de
// `TestLaFichaRellenaElRegistro` y `TestAnadirYBorrarRegistros` de la ficha de Fyne
// (`apps/fyne/apps/bases/tabla_test.go`). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesField, BasesRecord } from '@sytue/api-clients/bases';
import {
  alternar,
  borradorDe,
  cambiosDeFicha,
  editableEnFicha,
  hayCambios,
  opcionesDeSelect,
} from './ficha.js';

const campo = (id: string, kind: BasesField['kind'], extra: Partial<BasesField> = {}) =>
  ({ id, table_id: 't1', name: id, kind, position: 0, ...extra }) as BasesField;

const CAMPOS: BasesField[] = [
  campo('nom', 'text'),
  campo('eda', 'number'),
  campo('alt', 'date'),
  campo('act', 'checkbox'),
  campo('niv', 'select', { choices: ['Plata', 'Oro'] }),
  campo('eti', 'multiselect', { choices: ['vip', 'nuevo'] }),
  campo('emp', 'link', { link_table_id: 't2' }),
  campo('tot', 'formula', { formula: '{eda} * 2', result: 'number' }),
  campo('fic', 'attachment'),
];

const registro = (data: BasesRecord['data'] = {}, links: BasesRecord['links'] = {}) =>
  ({
    id: 'r1',
    table_id: 't1',
    data,
    links,
    rev: 1,
    created_by: '',
    updated_by: '',
    created_at: '',
    updated_at: '',
  }) as BasesRecord;

test('guardar una ficha sin tocar nada no escribe', () => {
  const r = registro(
    { nom: 'Ana', eda: 21, alt: '2026-10-05', act: true, niv: 'Oro', eti: ['vip'], tot: 42 },
    { emp: ['e1'] },
  );
  const c = cambiosDeFicha(CAMPOS, r, borradorDe(CAMPOS, r));
  expect(c).toEqual({ datos: {}, enlaces: {}, invalidos: [] });
  expect(hayCambios(c)).toBe(false);
  // Tampoco en un registro recién creado, vacío.
  expect(hayCambios(cambiosDeFicha(CAMPOS, registro(), borradorDe(CAMPOS, registro())))).toBe(
    false,
  );
});

test('un valor que no encaja se marca y la ficha no envía nada', () => {
  const r = registro();
  const b = { ...borradorDe(CAMPOS, r), nom: 'Ana', eda: 'treinta', alt: '2026-13-40' };
  const c = cambiosDeFicha(CAMPOS, r, b);
  expect(c.invalidos).toEqual(['eda', 'alt']);
  // Lo escrito en los demás campos sigue ahí: el borrador no se toca.
  expect(b.nom).toBe('Ana');
  expect(c.datos).toEqual({ nom: 'Ana' });
});

test('cada control da el valor de su tipo y solo viaja lo cambiado', () => {
  const r = registro();
  let b = borradorDe(CAMPOS, r);
  b = {
    ...b,
    nom: 'Ana',
    eda: '33,5',
    alt: '2026-10-05',
    act: true,
    niv: 'Oro',
    eti: alternar(b.eti as string[], 'vip', true, ['vip', 'nuevo']),
    emp: alternar(b.emp as string[], 'e1', true, ['e1', 'e2']),
  };
  const c = cambiosDeFicha(CAMPOS, r, b);
  expect(c.invalidos).toEqual([]);
  expect(c.datos).toEqual({
    nom: 'Ana',
    eda: 33.5,
    alt: '2026-10-05',
    act: true,
    niv: 'Oro',
    eti: ['vip'],
  });
  expect(c.enlaces).toEqual({ emp: ['e1'] });
  // La fórmula y los adjuntos no se escriben desde la ficha.
  expect('tot' in c.datos || 'fic' in c.datos).toBe(false);
});

test('un número escrito como se lee no es un cambio; vaciar es null', () => {
  const r = registro({ nom: 'Ana', eda: 21, act: true, niv: 'Oro', eti: ['vip'] }, { emp: ['e1'] });
  const b = {
    ...borradorDe(CAMPOS, r),
    eda: '21',
    nom: '  ',
    act: false,
    niv: '',
    eti: [],
    emp: [],
  };
  const c = cambiosDeFicha(CAMPOS, r, b);
  expect(c.datos).toEqual({ nom: null, act: null, niv: null, eti: null });
  expect(c.enlaces).toEqual({ emp: [] });
});

test('un valor que ya no es una opción se conserva al guardar', () => {
  const r = registro({ niv: 'Bronce', eti: ['viejo', 'vip'] });
  const b = borradorDe(CAMPOS, r);
  expect(opcionesDeSelect(CAMPOS[4]!, 'Bronce')).toEqual(['Plata', 'Oro', 'Bronce']);
  expect(opcionesDeSelect(CAMPOS[4]!, 'Oro')).toEqual(['Plata', 'Oro']);
  expect(cambiosDeFicha(CAMPOS, r, b).datos).toEqual({});
  // Marcar otra opción no se lleva por delante la que ya no lo es.
  const eti = alternar(b.eti as string[], 'nuevo', true, ['vip', 'nuevo']);
  expect(eti).toEqual(['vip', 'nuevo', 'viejo']);
  expect(cambiosDeFicha(CAMPOS, r, { ...b, eti }).datos).toEqual({ eti });
});

test('un enlace más allá de las opciones cargadas no se pierde al marcar otro', () => {
  const r = registro({}, { emp: ['lejano'] });
  const emp = alternar(borradorDe(CAMPOS, r).emp as string[], 'e2', true, ['e1', 'e2']);
  expect(emp).toEqual(['e2', 'lejano']);
  expect(alternar(emp, 'lejano', false, ['e1', 'e2'])).toEqual(['e2']);
});

test('lo que otra persona cambió mientras tanto en otro campo no se pisa', () => {
  const abierto = registro({ nom: 'Ana', eda: 21 });
  const b = { ...borradorDe(CAMPOS, abierto), eda: '22' };
  // Se compara con el registro tal como se abrió: «nom» no se ha tocado y no viaja.
  expect(cambiosDeFicha(CAMPOS, abierto, b).datos).toEqual({ eda: 22 });
});

test('la fórmula y los adjuntos no se editan en la ficha', () => {
  expect(CAMPOS.filter((c) => !editableEnFicha(c)).map((c) => c.id)).toEqual(['tot', 'fic']);
  expect(Object.keys(borradorDe(CAMPOS, registro()))).not.toContain('tot');
});
