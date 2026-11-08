// Checks de «Pausar avisos». Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { DURACIONES_PAUSA, finDePausa, pausaActiva, turnosDePausa } from './pausa.js';

// Jueves 5 de noviembre de 2026, 17:00 locales.
const ahora = new Date(2026, 10, 5, 17, 0, 0, 0).getTime();

test('las duraciones cortas cuentan desde ahora', () => {
  expect(finDePausa('30m', ahora) - ahora).toBe(30 * 60_000);
  expect(finDePausa('1h', ahora) - ahora).toBe(60 * 60_000);
  expect(finDePausa('2h', ahora) - ahora).toBe(120 * 60_000);
});

test('mañana y el lunes son a las 9:00 locales', () => {
  expect(new Date(finDePausa('tomorrow', ahora))).toEqual(new Date(2026, 10, 6, 9, 0, 0, 0));
  expect(new Date(finDePausa('monday', ahora))).toEqual(new Date(2026, 10, 9, 9, 0, 0, 0));
  expect(DURACIONES_PAUSA).toEqual(['30m', '1h', '2h', 'tomorrow', 'monday']);
});

test('la pausa vale mientras no ha pasado su hora', () => {
  const iso = (ms: number) => new Date(ms).toISOString();
  expect(pausaActiva(iso(ahora + 60_000), ahora)).toBe(true);
  expect(pausaActiva(iso(ahora - 60_000), ahora)).toBe(false);
  expect(pausaActiva(undefined, ahora)).toBe(false);
  expect(pausaActiva('no es una fecha', ahora)).toBe(false);
});

test('una lectura lanzada antes de pausar no vale al llegar; una posterior, sí', () => {
  const turnos = turnosDePausa();
  const vieja = turnos.leer(); // abre el menú: relee
  turnos.escribir(); // y enseguida pausa
  turnos.escribir(); // el servicio la confirma
  expect(vieja()).toBe(false);
  const nueva = turnos.leer();
  expect(nueva()).toBe(true);
  // Lanzada entre la petición y la confirmación tampoco vale: pudo leer lo de antes.
  turnos.escribir();
  const enMedio = turnos.leer();
  turnos.escribir();
  expect(enMedio()).toBe(false);
});
