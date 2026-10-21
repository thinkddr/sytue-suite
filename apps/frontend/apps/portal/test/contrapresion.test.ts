// GUARDIÁN del parche `apps/frontend/patches/astro@<versión>.patch`.
//
// El adaptador de node de Astro vuelca el cuerpo de la respuesta con un bucle que no mira
// lo que devuelve `write()`: con un cliente más lento que el origen, el proceso acumula el
// cuerpo ENTERO en memoria. Medido en producción el 2026-09-25: un instalador de 660 MB
// por el túnel de casa (~4 MB/s) llevó el pod del portal a 751 MB y el kernel lo mató por
// OOM; la descarga se cortaba a los 25-48 MB y, como el borde comprime, el navegador la
// daba por buena. Lo mismo vale para cualquier BFF que reenvíe un fichero grande (Drive).
//
// Si Renovate sube `astro` y el parche deja de aplicarse, esto se pone en rojo.
// Ejecutar: cd apps/frontend/apps/portal && bun test
import { EventEmitter } from 'node:events';
import { expect, test } from 'bun:test';
import { writeResponse } from 'astro/app/node';

/** Un destino que se «llena» en cada escritura hasta que alguien emite `drain`. */
class DestinoLento extends EventEmitter {
  escrituras = 0;
  destroyed = false;
  statusMessage = '';
  writeHead(): void {}
  write(): boolean {
    this.escrituras++;
    return false;
  }
  end(): void {
    this.emit('finish');
  }
}

test('writeResponse espera a «drain» antes de seguir leyendo del origen', async () => {
  let leidos = 0;
  const cuerpo = new ReadableStream<Uint8Array>({
    pull(c) {
      if (++leidos > 50) return c.close();
      c.enqueue(new Uint8Array(1024));
    },
  });
  const destino = new DestinoLento();
  const hecho = writeResponse(new Response(cuerpo), destino as never);

  await new Promise((r) => setTimeout(r, 50));
  // Sin el parche las 50 escrituras ya habrían ocurrido: todo el cuerpo en memoria.
  expect(destino.escrituras).toBe(1);

  for (let i = 0; i < 60 && destino.escrituras < 50; i++) {
    destino.emit('drain');
    await new Promise((r) => setTimeout(r, 1));
  }
  destino.emit('drain');
  await hecho;
  expect(destino.escrituras).toBe(50);
});

test('si el cliente se va, no se queda esperando un «drain» que no llegará', async () => {
  // Finito a propósito: sin el parche el bucle lo vuelca entero sin soltar el hilo, y un
  // flujo infinito colgaría la prueba en vez de ponerla en rojo.
  let leidos = 0;
  const cuerpo = new ReadableStream<Uint8Array>({
    pull(c) {
      if (++leidos > 1000) return c.close();
      c.enqueue(new Uint8Array(1024));
    },
  });
  const destino = new DestinoLento();
  const hecho = writeResponse(new Response(cuerpo), destino as never);
  await new Promise((r) => setTimeout(r, 10));
  destino.destroyed = true;
  destino.emit('close');
  await hecho;
  expect(destino.escrituras).toBe(1);
});
