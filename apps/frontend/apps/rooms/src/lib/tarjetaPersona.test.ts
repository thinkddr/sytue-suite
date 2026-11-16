import { afterEach, expect, test } from 'bun:test';
import { directorio, horaLocal, olvidarDirectorio } from './tarjetaPersona.js';

afterEach(olvidarDirectorio);

const mediodiaUTC = new Date('2026-11-10T12:00:00Z');

test('la hora local sale en la zona de su dispositivo, no en la del navegador', () => {
  expect(horaLocal('es', 'Europe/Madrid', mediodiaUTC)).toBe('13:00');
  expect(horaLocal('es', 'America/New_York', mediodiaUTC)).toBe('07:00');
});

test('sin zona, o con una que no existe, no hay hora (antes que inventarla)', () => {
  expect(horaLocal('es', undefined, mediodiaUTC)).toBeNull();
  expect(horaLocal('es', 'Marte/Olympus', mediodiaUTC)).toBeNull();
});

test('el directorio se pide una vez y se reintenta si falló', async () => {
  let llamadas = 0;
  const falla = () => {
    llamadas++;
    return Promise.resolve(new Response('', { status: 502 }));
  };
  await expect(directorio(falla)).rejects.toThrow();
  const bien = () => {
    llamadas++;
    return Promise.resolve(
      Response.json({ people: [{ sub: 'u1', email: 'ana@acme.sytue.eu', title: 'Diseño' }] }),
    );
  };
  const m = await directorio(bien);
  expect(m.get('u1')?.email).toBe('ana@acme.sytue.eu');
  await directorio(bien);
  expect(llamadas).toBe(2);
});
