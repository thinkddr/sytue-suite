/**
 * Leer la lista de términos que devuelve Sytue AI.
 *
 * Aparte y con tests por lo mismo que `respuestaIA.ts`: un modelo contesta el
 * array pelado, o envuelto en un bloque de código, o con una frase delante, y
 * un `JSON.parse` a secas falla en los dos últimos casos. Si no hay una lista
 * de palabras usable se devuelve vacía, y quien llama simplemente no amplía la
 * búsqueda — que es mejor que ampliarla con basura.
 */
export function terminosDeRespuesta(texto: string): string[] {
  const abre = texto.indexOf('[');
  const cierra = texto.lastIndexOf(']');
  if (abre < 0 || cierra <= abre) return [];
  let dato: unknown;
  try {
    dato = JSON.parse(texto.slice(abre, cierra + 1));
  } catch {
    return [];
  }
  if (!Array.isArray(dato)) return [];
  return dato
    .filter((x): x is string => typeof x === 'string')
    .map((x) => x.trim().toLowerCase())
    .filter((x, i, todos) => x !== '' && x.length <= 40 && todos.indexOf(x) === i)
    .slice(0, 6);
}
