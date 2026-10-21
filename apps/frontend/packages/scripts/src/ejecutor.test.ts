// El bucle del anfitrión, con un sandbox de mentira. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { ejecutarAislado, type Sandbox } from './ejecutor';

/** Un sandbox que, al arrancar, manda de golpe los mensajes dados. */
function sandboxQueManda(mensajes: unknown[]): Sandbox {
  return {
    recibir: (fn) => {
      queueMicrotask(() => {
        for (const m of mensajes) fn(m);
      });
    },
    enviar: () => {},
    destruir: () => {},
  };
}

test('tras abortar, las llamadas que ya estaban en cola no se atienden', async () => {
  const atendidas: string[] = [];
  const r = await ejecutarAislado({
    codigo: '',
    operaciones: ['borrar'] as const,
    maxOps: 10,
    maxMs: 1000,
    // Tres llamadas lanzadas sin esperar; la primera se deniega.
    crearSandbox: () =>
      sandboxQueManda([
        { t: 'ready' },
        { t: 'call', seq: 1, op: 'borrar', args: ['a'] },
        { t: 'call', seq: 2, op: 'borrar', args: ['b'] },
        { t: 'call', seq: 3, op: 'borrar', args: ['c'] },
      ]),
    atender: async (_op, args) => {
      atendidas.push(String(args[0]));
      return args[0] === 'a'
        ? { tipo: 'rechazo', codigo: 'denegado', mensaje: 'no' }
        : { tipo: 'valor', valor: null };
    },
  });
  expect(r.outcome).toBe('denied');
  // La cola sigue corriendo después de resolver: se le da tiempo antes de mirar.
  await new Promise((listo) => setTimeout(listo, 20));
  expect(atendidas).toEqual(['a']);
});
