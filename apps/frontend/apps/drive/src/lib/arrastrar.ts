/**
 * Qué se puede soltar dónde al mover ficheros arrastrando.
 *
 * Vive aparte del componente porque las tres reglas que hay aquí no fallan con
 * un error: fallan moviendo cosas. Soltar una carpeta dentro de sí misma deja un
 * árbol que se referencia a sí mismo —y en un almacén de objetos eso no es un
 * ciclo bonito, es una carpeta que desaparece con todo lo que tenía dentro—.
 */

/** Tipo MIME propio del arrastre interno; distingue mover de subir del escritorio. */
export const MIME_ARRASTRE = 'application/x-sytue-drive';

/** Carpeta que contiene a `path` ('' para la raíz). */
export function carpetaDe(path: string): string {
  const i = path.lastIndexOf('/');
  return i <= 0 ? '' : path.slice(0, i);
}

/**
 * ¿`posible` está dentro de `carpeta` (a cualquier profundidad)?
 *
 * Compara con la barra puesta a propósito: sin ella, `/informes` parecería
 * contener a `/informes-2025`, y mover una carpeta a su vecina de nombre
 * parecido quedaría prohibido sin motivo.
 */
export function esDescendiente(posible: string, carpeta: string): boolean {
  if (carpeta === '') return true; // la raíz contiene todo
  return posible === carpeta || posible.startsWith(`${carpeta}/`);
}

/**
 * ¿Se puede soltar `origen` dentro de `destino`? `destino` es SIEMPRE una carpeta.
 *
 * Se rechaza:
 *   - soltarse a sí misma;
 *   - soltar una carpeta dentro de su propio contenido (el ciclo);
 *   - soltar donde ya está, que no es un error pero tampoco un movimiento: hacer
 *     la llamada igualmente gastaría una escritura y un refresco por nada.
 */
export function sePuedeSoltar(origen: string, destino: string): boolean {
  if (origen === destino) return false;
  if (esDescendiente(destino, origen)) return false;
  return carpetaDe(origen) !== destino;
}

/** Los que de verdad se moverían, de una selección arrastrada a un destino. */
export function movibles(origenes: string[], destino: string): string[] {
  return origenes.filter((o) => sePuedeSoltar(o, destino));
}

/** Ruta que tendrá `origen` dentro de `destino` (destino '' = raíz). */
export function rutaDestino(origen: string, destino: string): string {
  const nombre = origen.slice(origen.lastIndexOf('/') + 1);
  return destino === '' ? `/${nombre}` : `${destino}/${nombre}`;
}
