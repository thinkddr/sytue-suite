/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Consentimiento del manifiesto de un script, COMÚN a los productos. Se pide antes de la
 * PRIMERA ejecución y de nuevo ante CUALQUIER cambio del manifiesto: lo que se recuerda es
 * el par (script, hash del manifiesto canónico), así que un manifiesto distinto es otra
 * clave y no hay consentimiento heredado. Se guarda en el navegador de quien consiente
 * (localStorage, por producto y persona); si falta, simplemente se vuelve a preguntar.
 * Es una pregunta, no la frontera de seguridad. PURO salvo `cargar`/`guardar`.
 */

const MAX_CONSENTIMIENTOS = 500;
/** `producto`: 'docs', 'bases'… Cada uno tiene su lista: los ids de script no se cruzan. */
const clave = (producto: string, sub: string): string => `sytue:${producto}:scripts:consent:${sub}`;
const entrada = (script: string, hash: string): string => `${script}:${hash}`;

export function leerConsentimientos(json: string | null): Set<string> {
  if (!json) return new Set();
  try {
    const d: unknown = JSON.parse(json);
    if (!Array.isArray(d)) return new Set();
    return new Set(
      d
        .filter((x): x is string => typeof x === 'string' && x.length <= 200)
        .slice(-MAX_CONSENTIMIENTOS),
    );
  } catch {
    return new Set();
  }
}

export const haConsentido = (s: ReadonlySet<string>, script: string, hash: string): boolean =>
  s.has(entrada(script, hash));

/** Añade (el más antiguo sale al llegar al tope). Devuelve un conjunto nuevo. */
export function consentir(s: ReadonlySet<string>, script: string, hash: string): Set<string> {
  const e = entrada(script, hash);
  return new Set(
    [...s]
      .filter((x) => x !== e)
      .slice(-(MAX_CONSENTIMIENTOS - 1))
      .concat(e),
  );
}

export function cargarConsentimientos(producto: string, sub: string): Set<string> {
  try {
    return leerConsentimientos(localStorage.getItem(clave(producto, sub)));
  } catch {
    return new Set();
  }
}

export function guardarConsentimientos(
  producto: string,
  sub: string,
  s: ReadonlySet<string>,
): void {
  try {
    localStorage.setItem(clave(producto, sub), JSON.stringify([...s]));
  } catch {
    /* sin almacenamiento: se volverá a preguntar */
  }
}

/** SHA-256 hexadecimal de un texto: el consentimiento se da sobre el del manifiesto canónico. */
export async function sha256Hex(texto: string): Promise<string> {
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(texto));
  return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, '0')).join('');
}
