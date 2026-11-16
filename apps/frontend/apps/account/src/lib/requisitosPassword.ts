/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS REQUISITOS DE LA CONTRASEÑA NUEVA, COMPROBADOS MIENTRAS SE ESCRIBE.
 *
 * La política la decide el servicio (`password_policy` de `/api/v1/me`: la más
 * estricta de las organizaciones de la persona, con el suelo ya puesto) y la
 * aplica `domain.ValidatePassword`. Aquí solo se REPITE la comprobación para
 * enseñarla antes de enviar, así que tiene que contar igual que el servidor:
 *
 *   - la longitud en puntos de código (las runas de Go), no en unidades UTF-16:
 *     «ñ» o un emoji cuentan uno, como allí;
 *   - la complejidad es «una letra (de cualquier alfabeto) y un dígito ASCII»,
 *     lo mismo que `hasComplexity` (`unicode.IsLetter` y `'0'..'9'`).
 *
 * Si alguna vez discrepan, manda el servidor: su 400 sigue saliendo en la
 * tarjeta. Lo que no puede pasar es lo contrario —la lista en verde y el
 * servidor diciendo que no—, y para eso están los tests de esta función.
 */

/** La política tal como la sirve `/me`. */
export interface PoliticaPassword {
  min_length: number;
  require_complexity: boolean;
}

export type ClaveRequisito = 'length' | 'complexity' | 'different';

export interface Requisito {
  clave: ClaveRequisito;
  cumplido: boolean;
}

/** Lee `password_policy` de la respuesta de `/me`; `null` si no viene o no encaja. */
export function leerPolitica(datos: unknown): PoliticaPassword | null {
  const p = (datos as { password_policy?: unknown } | null)?.password_policy as
    { min_length?: unknown; require_complexity?: unknown } | undefined;
  if (!p || typeof p.min_length !== 'number' || !Number.isFinite(p.min_length)) return null;
  return { min_length: p.min_length, require_complexity: p.require_complexity === true };
}

const LETRA = /\p{L}/u;
const DIGITO = /[0-9]/;

/**
 * Puntos de código, como `utf8.RuneCountInString`. Aquí partir un emoji
 * compuesto en varias piezas es justo lo que se quiere: Go también las cuenta
 * por separado.
 */
function runas(s: string): number {
  let n = 0;
  for (const _ of s) n++;
  return n;
}

/** La lista que se enseña, en orden, con lo que la contraseña nueva ya cumple. */
export function requisitosPassword(
  politica: PoliticaPassword,
  nueva: string,
  actual: string,
): Requisito[] {
  const lista: Requisito[] = [{ clave: 'length', cumplido: runas(nueva) >= politica.min_length }];
  if (politica.require_complexity) {
    lista.push({ clave: 'complexity', cumplido: LETRA.test(nueva) && DIGITO.test(nueva) });
  }
  // El servicio también rechaza repetir la actual (`changeUserPassword`).
  lista.push({ clave: 'different', cumplido: nueva !== '' && nueva !== actual });
  return lista;
}
