// SPDX-License-Identifier: Apache-2.0

/** Fuente de bytes aleatorios; por defecto, la criptográfica del entorno. */
export type FuenteDeBytes = (n: number) => Uint8Array;

const bytesCriptograficos: FuenteDeBytes = (n) => crypto.getRandomValues(new Uint8Array(n));

/**
 * Texto aleatorio uniforme sobre `alfabeto`, sin sesgo de módulo.
 *
 * `byte % alfabeto.length` favorece los primeros símbolos siempre que 256 no sea
 * múltiplo de la longitud (con 62 símbolos, los 8 primeros salen un 25 % más). Aquí
 * se usa muestreo por rechazo: cada byte se reduce con una máscara de bits a la menor
 * potencia de dos que cubre el alfabeto (uniforme, sin módulo) y se descarta si cae
 * fuera de él.
 */
export function textoAleatorio(
  longitud: number,
  alfabeto: string,
  fuente: FuenteDeBytes = bytesCriptograficos,
): string {
  const n = alfabeto.length;
  if (!Number.isSafeInteger(longitud) || longitud < 0) throw new RangeError('longitud');
  if (n < 1 || n > 256) throw new RangeError('alfabeto');
  let mascara = 1;
  while (mascara < n) mascara <<= 1;
  mascara -= 1;
  const partes: string[] = [];
  while (partes.length < longitud) {
    // Se pide algo más de lo que falta para cubrir los rechazos de media.
    const lote = fuente(Math.ceil(((longitud - partes.length) * (mascara + 1)) / n) + 16);
    for (const byte of lote) {
      const indice = byte & mascara;
      if (indice >= n) continue;
      partes.push(alfabeto[indice]!);
      if (partes.length === longitud) break;
    }
    lote.fill(0);
  }
  return partes.join('');
}
