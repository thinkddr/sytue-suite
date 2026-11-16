/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cuándo pedirle a la IA que entienda la consulta, y cómo contar lo que entendió.
 *
 * SEPARADO DEL COMPONENTE PORQUE ES LO ÚNICO QUE SE PUEDE PROBAR SIN NAVEGADOR.
 * La decisión de gastar una llamada a un modelo, y la de qué se le enseña a
 * quien busca, no deberían depender de montar un combobox para verificarlas.
 */
import type { HubClient, Interpretado, SearchResponse } from '@sytue/api-clients';

/**
 * ¿Da esta consulta para preguntarle al modelo?
 *
 * EL FILTRO ES DE COSTE. Cada interpretación es una llamada de pago, y una
 * palabra suelta no tiene nada que interpretar: ya ES el término que hay que
 * buscar. El Hub aplica la misma regla —es la frontera de verdad—; aquí se
 * repite para no gastar el viaje.
 */
export function mereceInterpretarse(consulta: string): boolean {
  const palabras = consulta.trim().split(/\s+/).filter(Boolean);
  return palabras.length >= 3 && consulta.length <= 500;
}

/** Una ficha de lo que se aplicó, para pintarla al lado de la consulta. */
export interface Ficha {
  clave: string;
  texto: string;
}

/**
 * Las fichas de una interpretación, en el orden en que se leen.
 *
 * NO SE PINTA EL TÉRMINO LITERAL como ficha: ya está en el campo de búsqueda,
 * a la vista. Lo que hay que enseñar es lo que NO se ve — que se ha acotado a
 * un producto, a un tipo o a un rango de fechas—, que es justo lo que hace que
 * una búsqueda devuelva cero resultados sin explicar por qué.
 */
export function fichasDe(
  i: Interpretado,
  nombreDeFuente: (s: string) => string,
  nombreDeTipo: (k: string) => string,
  fecha: (ms: number) => string,
  t: {
    fuente: (nombre: string) => string;
    tipo: (kind: string) => string;
    rango: (desde: string, hasta: string) => string;
    desde: (d: string) => string;
    hasta: (h: string) => string;
  },
): Ficha[] {
  const fichas: Ficha[] = [];
  for (const s of i.sources ?? []) {
    fichas.push({ clave: `s:${s}`, texto: t.fuente(nombreDeFuente(s)) });
  }
  for (const k of i.kinds ?? []) {
    fichas.push({ clave: `k:${k}`, texto: t.tipo(nombreDeTipo(k)) });
  }
  if (i.from !== undefined && i.to !== undefined) {
    fichas.push({ clave: 'f', texto: t.rango(fecha(i.from), fecha(i.to)) });
  } else if (i.from !== undefined) {
    fichas.push({ clave: 'f', texto: t.desde(fecha(i.from)) });
  } else if (i.to !== undefined) {
    fichas.push({ clave: 'f', texto: t.hasta(fecha(i.to)) });
  }
  return fichas;
}

/** Lo que deja una búsqueda en la suite: la respuesta y, si hizo falta, lo que entendió la IA. */
export interface BusquedaInterpretada {
  respuesta: SearchResponse;
  /** `null` = se buscó literalmente, que es lo normal y lo de siempre. */
  interpretado: Interpretado | null;
}

/**
 * Busca en el Hub y, si la búsqueda de siempre no encuentra nada, le pide a la IA que
 * entienda la frase. La usan el buscador de la barra y la paleta de la suite (⌘K): la
 * misma consulta tiene que dar lo mismo en los dos sitios.
 *
 * LA IA ENTRA SOLO CUANDO LA BÚSQUEDA DE SIEMPRE NO ENCUENTRA NADA, y esto es lo que
 * hace que la función salga gratis: mientras buscar funciona, no se gasta ni una
 * llamada ni un milisegundo. Cuando falla —que es cuando alguien ha escrito una frase
 * en vez de una palabra— no hay nada que perder: peor que cero resultados no se puede
 * quedar. Con `literal` (quien pulsó «Buscar las palabras exactas» para ESTA consulta)
 * no se pregunta. `alEntender` avisa de la segunda vuelta, que tarda más y se dice.
 */
export async function buscarConInterpretacion(
  hub: Pick<HubClient, 'search'>,
  consulta: string,
  opciones: { literal?: boolean; alEntender?: (entendiendo: boolean) => void } = {},
): Promise<BusquedaInterpretada> {
  const respuesta = await hub.search(consulta);
  if (respuesta.results.length > 0 || opciones.literal || !mereceInterpretarse(consulta)) {
    return { respuesta, interpretado: null };
  }
  opciones.alEntender?.(true);
  try {
    const conIa = await hub.search(consulta, undefined, { ia: true });
    if (conIa.results.length > 0)
      return { respuesta: conIa, interpretado: conIa.interpretado ?? null };
  } finally {
    opciones.alEntender?.(false);
  }
  return { respuesta, interpretado: null };
}
