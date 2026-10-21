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
import type { Interpretado } from '@sytue/api-clients';

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
