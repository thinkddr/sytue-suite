/**
 * Quién ocupa el escenario grande de una videollamada.
 *
 * Vive fuera del componente porque el orden de las reglas ES la función: si el
 * fijado no gana a la pantalla compartida, fijar a alguien deja de servir en
 * cuanto otro comparte; y si el hablante no se recuerda, el escenario se
 * derrumba a mosaico en cada silencio, que es como se siente un producto roto.
 */

/** Mosaico: todos iguales. Orador: manda quien habla (o quien esté fijado). */
export type ModoEscenario = 'mosaico' | 'orador';

export interface Escena {
  modo: ModoEscenario;
  /** Identidad fijada a mano, o null. Gana a todo lo demás. */
  fijado: string | null;
  /** Quién habló por última vez (no quién habla AHORA: ver abajo). */
  ultimoHablante: string | null;
  /** Quién está compartiendo pantalla, si alguien. */
  compartiendo: string | null;
  /** Identidades presentes. Lo que no esté aquí no puede salir en grande. */
  presentes: string[];
}

/**
 * Devuelve la identidad que va en grande, o null para mosaico.
 *
 * El orden importa:
 *
 *  1. Lo FIJADO a mano gana siempre. Es una decisión explícita de quien mira, y
 *     que se la pise el sistema es exactamente lo que hace que nadie use la
 *     función.
 *  2. Quien comparte pantalla va en grande aunque el modo sea mosaico: se
 *     comparte para que se vea, y nueve miniaturas iguales no lo enseñan.
 *  3. En modo orador, el ÚLTIMO que habló —no el que habla ahora—. La señal del
 *     SFU se vacía en cada pausa, y usarla tal cual haría que el escenario se
 *     desmontara entre frase y frase.
 *  4. Si nada de eso aplica, mosaico.
 *
 * Y en todos los casos, quien ya no está en la sala no sale en grande: colgar
 * dejaba el escenario ocupado por un hueco negro con el nombre de quien se fue.
 */
export function protagonista(e: Escena): string | null {
  const presente = (id: string | null): string | null =>
    id && e.presentes.includes(id) ? id : null;

  return (
    presente(e.fijado) ??
    presente(e.compartiendo) ??
    (e.modo === 'orador' ? presente(e.ultimoHablante) : null)
  );
}

/**
 * Ordena los tiles dejando al protagonista el primero, sin tocar el orden
 * relativo del resto.
 *
 * Reordenar y no mover de contenedor es deliberado: el `<video>` se engancha al
 * nodo por fuera de Preact, así que sacar un tile a otro sitio del árbol lo
 * destruiría —y con él la imagen y, peor, el audio—. Reordenar dentro del mismo
 * padre es un `insertBefore`, que conserva el nodo y todo lo que cuelga de él.
 */
export function ordenarPorProtagonista<T extends { id: string }>(
  tiles: T[],
  protagonistaID: string | null,
): T[] {
  if (!protagonistaID) return tiles;
  const i = tiles.findIndex((t) => t.id === protagonistaID);
  if (i <= 0) return tiles;
  const copia = tiles.slice();
  const [p] = copia.splice(i, 1);
  return p ? [p, ...copia] : tiles;
}
