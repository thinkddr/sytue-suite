/**
 * Self-check de la política de confirmación de un cambio de tamaño.
 *
 * Las dos cosas que NO pueden aflojarse aquí: que bajar pida reconocimiento (y
 * subir no), y que una recomendación hecha con datos de terceros no se presente
 * como si hubiéramos medido la app del cliente. La primera es la diferencia
 * entre cobrar de más y tirar un servicio; la segunda es por qué el cliente se
 * cree —o no— la recomendación siguiente. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { confirmationFor } from '../src/lib/sizing.js';

test('bajar pide reconocimiento explícito; subir no', () => {
  expect(confirmationFor({ action: 'down', reason: 'over_provisioned' }).requiresAck).toBe(true);
  expect(confirmationFor({ action: 'up', reason: 'near_limit' }).requiresAck).toBe(false);
  expect(confirmationFor({ action: 'keep', reason: 'well_sized' }).requiresAck).toBe(false);
});

test('el ahorro del día cero se justifica con la cohorte, nunca con medidas propias', () => {
  // No hay histórico de ESTA app: `samples` vale 0 y citar «medido sobre 0
  // lecturas» sería mentir en la única frase que sostiene la recomendación.
  const c = confirmationFor({ action: 'down', reason: 'cohort_over_provisioned' });
  expect(c.evidence).toBe('cohort');
  // Y sigue siendo una bajada: el origen del dato no rebaja la confirmación.
  expect(c.requiresAck).toBe(true);
});

test('con histórico propio la prueba es la medida de la app', () => {
  expect(confirmationFor({ action: 'down', reason: 'over_provisioned' }).evidence).toBe('measured');
  expect(confirmationFor({ action: 'up', reason: 'near_limit' }).evidence).toBe('measured');
});
