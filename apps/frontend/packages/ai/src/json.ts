/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La respuesta del modelo cuando lo que se le pide son DATOS, no prosa.
 *
 * En un editor el asistente escribe texto y se ve si acierta. En Calendar o en
 * Contacts propone un evento o una ficha: campos que van a rellenar un
 * formulario, y ahí una alucinación no se ve, se guarda.
 *
 * DE AHÍ LAS DOS REGLAS DE ESTE FICHERO:
 *
 *   · Nada se acepta sin validar. `objetoDe` devuelve `unknown` a propósito, y
 *     cada app declara qué campos espera y de qué tipo. Un `as Evento` aquí
 *     habría convertido «lo que dijo el modelo» en «lo que hay en la agenda».
 *   · Nada se guarda solo. Lo que sale de aquí PRERRELLENA un formulario que la
 *     persona ve y confirma. El asistente propone; quien firma es quien pulsa.
 */

/**
 * Saca el primer objeto JSON de una respuesta.
 *
 * Los modelos envuelven el JSON en ```json aunque se les pida que no, y a veces
 * lo acompañan de una frase («Aquí tienes el evento:»). Se busca el objeto y se
 * ignora el resto en vez de fallar: pedirle al modelo que se porte bien es más
 * frágil que recortar aquí.
 *
 * Devuelve `null` si no hay nada parseable. Nunca lanza: quien llama ya tiene un
 * camino para «el asistente no supo».
 */
export function objetoDe(texto: string): unknown {
  const limpio = texto
    .replace(/^\s*```(?:json)?\s*/i, '')
    .replace(/\s*```\s*$/, '')
    .trim();

  const intentos = [limpio];
  // Si venía con prosa alrededor, se prueba con el tramo entre la primera llave
  // y la última. Tosco a propósito: un JSON con llaves dentro de una cadena
  // sobrevive igual, y lo que no, cae al `null` de abajo.
  const i = limpio.indexOf('{');
  const j = limpio.lastIndexOf('}');
  if (i >= 0 && j > i) intentos.push(limpio.slice(i, j + 1));

  for (const candidato of intentos) {
    try {
      const v: unknown = JSON.parse(candidato);
      if (v !== null && typeof v === 'object' && !Array.isArray(v)) return v;
    } catch {
      /* se prueba el siguiente */
    }
  }
  return null;
}

/** Lee una cadena no vacía de un objeto sin tipar. `undefined` si no la hay. */
export function cadena(obj: unknown, clave: string, tope = 500): string | undefined {
  if (obj === null || typeof obj !== 'object') return undefined;
  const v = (obj as Record<string, unknown>)[clave];
  if (typeof v !== 'string') return undefined;
  const s = v.trim();
  return s === '' ? undefined : s.slice(0, tope);
}

/**
 * Lee una fecha-hora ISO. `undefined` si no lo es.
 *
 * SE EXIGE ISO Y NO SE INTENTA ADIVINAR. «el jueves a las 2» lo resuelve el
 * modelo, que para eso se le da la fecha de hoy en el prompt; interpretar aquí
 * un «12/03» sería elegir entre marzo y diciembre por el usuario, y equivocarse
 * en una reunión no se perdona igual que en un párrafo.
 */
export function fechaHora(obj: unknown, clave: string): string | undefined {
  const s = cadena(obj, clave, 40);
  if (s === undefined) return undefined;
  // LA FORMA SE COMPRUEBA A MANO, no con `new Date`. El parser del motor es
  // generoso hasta el disparate: `new Date('el jueves a las 2')` devuelve una
  // fecha VÁLIDA —el 1 de febrero de 2001, medido— así que confiar en él
  // convertía una frase sin resolver en una cita en la agenda. Lo cazó su test.
  if (!/^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2})?(\.\d+)?(Z|[+-]\d{2}:?\d{2})?$/.test(s)) {
    return undefined;
  }
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

/**
 * Lista de cadenas de una clave. Filtra vacíos y duplicados; `[]` si no hay nada.
 *
 * TOLERA QUE LLEGUE UNA SOLA CADENA en vez de un array: pedir «emails» y recibir
 * `"ana@ejemplo.es"` es lo que hace el modelo cuando solo encuentra uno, y
 * descartarlo por la forma sería perder el dato por un corchete.
 */
export function lista(obj: unknown, clave: string, tope = 200, max = 10): string[] {
  if (obj === null || typeof obj !== 'object') return [];
  const v = (obj as Record<string, unknown>)[clave];
  const bruta = typeof v === 'string' ? [v] : Array.isArray(v) ? v : [];
  const salida: string[] = [];
  for (const x of bruta) {
    if (typeof x !== 'string') continue;
    const s = x.trim().slice(0, tope);
    if (s !== '' && !salida.includes(s)) salida.push(s);
    if (salida.length >= max) break;
  }
  return salida;
}
