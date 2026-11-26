/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «SUGERIR UNA CONTRASEÑA SEGURA» al cambiarla en Mi cuenta.
 *
 * Quien cambia la contraseña porque un aviso de seguridad le ha asustado acaba
 * escribiendo la de siempre con un número al final. Esto ofrece una de verdad:
 * aleatoria, larga y que ya cumple la política de la organización, para guardarla
 * en un gestor de contraseñas.
 *
 * Cómo se hace, y por qué así:
 *
 *   - Grupos de cinco separados por guiones («k7qm2-…»): se leen y se teclean sin
 *     perderse, que es lo que pasa con veinte caracteres seguidos.
 *   - Un alfabeto sin los que se confunden (0/o, 1/l/i): si alguien la copia a mano
 *     de una pantalla a otra, no se equivoca por la letra.
 *   - Aleatoriedad de `crypto.getRandomValues` con RECHAZO del sesgo: un valor módulo
 *     31 favorece a los primeros símbolos; descartando los de la cola, todos salen
 *     igual de probables. Son ~4,95 bits por símbolo: con 4 grupos de 5, unos
 *     99 bits, de sobra para una contraseña que nadie va a recordar.
 *   - Cumple la política que sirve `/me` (`PoliticaPassword`): si la longitud
 *     mínima pide más, se añaden grupos; y lleva siempre al menos una letra y un
 *     dígito, que es lo que el servidor entiende por «compleja».
 *
 * No viaja a ningún sitio: se genera aquí y solo sale con el formulario de cambio.
 */
import type { PoliticaPassword } from './requisitosPassword.js';

/** Sin 0, 1, i, l ni o: los que se confunden entre sí o con otras letras. */
export const ALFABETO = 'abcdefghjkmnpqrstuvwxyz23456789';
const LETRAS = 'abcdefghjkmnpqrstuvwxyz';
const DIGITOS = '23456789';

/** Cuántos símbolos por grupo y cuántos grupos como mínimo. */
export const POR_GRUPO = 5;
export const GRUPOS_MINIMOS = 4;

export type Aleatorio = (destino: Uint32Array) => Uint32Array;

const aleatorioDelNavegador: Aleatorio = (destino) => crypto.getRandomValues(destino);

const DOS_A_LA_32 = 2 ** 32;

/** Un índice uniforme en `[0, n)`, sin sesgo de módulo: se descartan los valores de
 *  la cola que no llenan una vuelta entera. */
function indice(n: number, aleatorio: Aleatorio): number {
  const tope = DOS_A_LA_32 - (DOS_A_LA_32 % n);
  const valor = new Uint32Array(1);
  for (;;) {
    aleatorio(valor);
    if (valor[0]! < tope) return valor[0]! % n;
  }
}

function simbolo(de: string, aleatorio: Aleatorio): string {
  return de[indice(de.length, aleatorio)]!;
}

/**
 * Una contraseña aleatoria que cumple `politica` (o la de por defecto si no se
 * conoce). `aleatorio` se inyecta en las pruebas.
 */
export function sugerirContrasena(
  politica: PoliticaPassword | null,
  aleatorio: Aleatorio = aleatorioDelNavegador,
): string {
  const minimo = Math.max(0, politica?.min_length ?? 0);
  // Grupos necesarios para llegar al mínimo contando los guiones, que también son
  // caracteres de la contraseña: g grupos miden g·5 + (g−1).
  let grupos = GRUPOS_MINIMOS;
  while (grupos * POR_GRUPO + (grupos - 1) < minimo) grupos++;

  const simbolos: string[] = [];
  for (let i = 0; i < grupos * POR_GRUPO; i++) simbolos.push(simbolo(ALFABETO, aleatorio));
  // Una letra y un dígito garantizados, en posiciones al azar y distintas: no se
  // deja al azar que la política de «compleja» se cumpla.
  const posLetra = indice(simbolos.length, aleatorio);
  let posDigito = indice(simbolos.length - 1, aleatorio);
  if (posDigito >= posLetra) posDigito++;
  simbolos[posLetra] = simbolo(LETRAS, aleatorio);
  simbolos[posDigito] = simbolo(DIGITOS, aleatorio);

  const trozos: string[] = [];
  for (let g = 0; g < grupos; g++) {
    trozos.push(simbolos.slice(g * POR_GRUPO, (g + 1) * POR_GRUPO).join(''));
  }
  return trozos.join('-');
}
