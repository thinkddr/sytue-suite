/**
 * Interpretar lo que contesta Sytue AI al describir una foto.
 *
 * ESTO ES LA PARTE FRÁGIL Y POR ESO ESTÁ APARTE Y PROBADA. Se le pide al modelo
 * un JSON con `descripcion` y `etiquetas`, y un modelo contesta lo que le
 * parece: a veces el JSON pelado, a veces envuelto en un bloque de código, a
 * veces con una frase de cortesía delante. Un `JSON.parse` a secas falla en los
 * tres últimos casos y deja a la persona con un error en vez de con su
 * descripción.
 *
 * Lo que NO se hace es adivinar: si no hay un objeto con una descripción usable,
 * se devuelve `null` y quien llama lo dice. Inventar una descripción a partir de
 * texto suelto sería escribir en el `alt` de la foto algo que el modelo no dijo.
 */
export interface Descripcion {
  descripcion: string;
  etiquetas: string[];
}

/** Lo que se guarda, acotado igual que en el servicio. */
export const MAX_DESCRIPCION = 500;
export const MAX_ETIQUETAS = 12;

export function interpretarRespuesta(texto: string): Descripcion | null {
  const crudo = extraerObjeto(texto);
  if (crudo === null) return null;
  let dato: unknown;
  try {
    dato = JSON.parse(crudo);
  } catch {
    return null;
  }
  if (typeof dato !== 'object' || dato === null || Array.isArray(dato)) return null;
  const obj = dato as Record<string, unknown>;
  const descripcion = typeof obj['descripcion'] === 'string' ? obj['descripcion'].trim() : '';
  if (descripcion === '') return null;
  const etiquetas = Array.isArray(obj['etiquetas'])
    ? obj['etiquetas']
        .filter((e): e is string => typeof e === 'string')
        .map((e) => e.trim().toLowerCase())
        .filter((e, i, todas) => e !== '' && e.length <= 60 && todas.indexOf(e) === i)
        .slice(0, MAX_ETIQUETAS)
    : [];
  return { descripcion: descripcion.slice(0, MAX_DESCRIPCION), etiquetas };
}

/**
 * Saca el primer objeto JSON del texto.
 *
 * Se BUSCAN LAS LLAVES en vez de recortar los ``` del principio y el final:
 * el modelo puede poner texto antes, después, o las dos cosas, y quitar la
 * valla solo resuelve uno de los tres casos. Se cuentan las llaves para no
 * cortar en la primera `}` de un objeto anidado, y se ignoran las que van
 * dentro de una cadena —una descripción con una llave dentro es texto, no
 * estructura—.
 */
function extraerObjeto(texto: string): string | null {
  const inicio = texto.indexOf('{');
  if (inicio < 0) return null;
  let nivel = 0;
  let enCadena = false;
  let escapado = false;
  for (let i = inicio; i < texto.length; i++) {
    const c = texto[i]!;
    if (enCadena) {
      if (escapado) escapado = false;
      else if (c === '\\') escapado = true;
      else if (c === '"') enCadena = false;
      continue;
    }
    if (c === '"') enCadena = true;
    else if (c === '{') nivel++;
    else if (c === '}') {
      nivel--;
      if (nivel === 0) return texto.slice(inicio, i + 1);
    }
  }
  return null;
}
