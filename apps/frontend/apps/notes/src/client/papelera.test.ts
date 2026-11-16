import { expect, test } from 'bun:test';
import { caducidad } from './papelera.js';

const AHORA = new Date('2026-11-12T10:00:00Z');
const dentroDe = (ms: number) => new Date(AHORA.getTime() + ms).toISOString();
const DIA = 24 * 60 * 60 * 1000;

test('lo borrado hoy se borra en 30 días, no en 29', () => {
  // El servicio pone purge_at = deleted_at + 30 días; un minuto después siguen siendo 30.
  expect(caducidad({ purge_at: dentroDe(30 * DIA - 60_000) }, AHORA)).toEqual({
    clave: 'trash.purgeIn',
    dias: 30,
  });
});

test('lo que se va en unas horas dice 1 día, y lo vencido «en breve»', () => {
  expect(caducidad({ purge_at: dentroDe(3 * 60 * 60 * 1000) }, AHORA)).toEqual({
    clave: 'trash.purgeIn',
    dias: 1,
  });
  // El recolector pasa cada hora: lo vencido aún puede estar ahí un rato.
  expect(caducidad({ purge_at: dentroDe(-60_000) }, AHORA)).toEqual({ clave: 'trash.purgeSoon' });
});

test('sin purge_at no caduca (retención legal o purga apagada)', () => {
  expect(caducidad({}, AHORA)).toEqual({ clave: 'trash.kept' });
  expect(caducidad({ purge_at: 'no es una fecha' }, AHORA)).toEqual({ clave: 'trash.kept' });
});
