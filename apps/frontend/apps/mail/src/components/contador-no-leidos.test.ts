import { expect, test } from 'bun:test';

/*
 * EL CONTADOR ROJO DE NO LEÍDOS SALE DE `listMailboxes`, NO DE LA LISTA.
 *
 * Marcar un correo como leído parchea la lista visible de forma optimista, que
 * es lo correcto —se ve al instante—, pero el número del lateral lo calcula el
 * servidor. Sin pedirlo otra vez se queda con el valor viejo hasta recargar la
 * página, que es exactamente lo que pasaba: se refrescaba al MOVER correo y no
 * al LEERLO, que es lo que más se hace.
 *
 * Se mira el código y no el DOM porque lo que falla no es lo que se pinta: es
 * que falta una petición después de una que sí se hace.
 */
const fuente = await Bun.file(new URL('./MailApp.tsx', import.meta.url)).text();

/**
 * Los bloques que llaman a `batchSetFlag`, hasta el final de su función.
 *
 * SE CORTA POR EL CIERRE DE LA FUNCIÓN (`\n  }`) y no por un número de
 * caracteres: con una ventana fija, un comentario largo entre la llamada y el
 * `.then` deja el refresco fuera y el test acusa a código que SÍ está bien.
 * Pasó al escribirlo con 400.
 */
function llamadas(): string[] {
  return (
    fuente
      .split('batchSetFlag(')
      .slice(1)
      // `?? t`: con `noUncheckedIndexedAccess`, el `[0]` de un `split` es
      // `string | undefined` para el compilador aunque nunca lo sea en ejecución.
      // Se cae al trozo entero y no a la cadena vacía: si algún día el cierre no
      // apareciera, el test debe seguir MIRANDO ese bloque en vez de darlo por
      // bueno sin leerlo.
      .map((t) => t.split('\n  }')[0] ?? t)
  );
}

test('cada cambio de $seen refresca los contadores del lateral', () => {
  const sinRefresco: string[] = [];
  for (const bloque of llamadas()) {
    if (!bloque.includes('$seen')) continue; // $flagged no mueve el contador
    if (!bloque.includes('refreshMailboxCounts')) sinRefresco.push(bloque.slice(0, 90));
  }
  expect(sinRefresco, 'un cambio de $seen sin refrescar deja el número rojo viejo').toEqual([]);
});

test('sigue existiendo la función que los refresca', () => {
  // Si alguien la renombra, el test de arriba pasaría en falso.
  expect(fuente).toContain('function refreshMailboxCounts()');
});
