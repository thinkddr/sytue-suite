/**
 * Detección de contactos duplicados (cliente, puro): agrupa las fichas que muy
 * probablemente son la misma persona porque comparten un correo, un teléfono o
 * el nombre completo (todo normalizado). Union-Find para que las coincidencias
 * sean transitivas (A~B por correo, B~C por teléfono ⇒ un único grupo). El
 * usuario resuelve cada grupo con la fusión «Combinar con…» ya existente.
 */
import type { Contact } from '@sytue/api-clients';

/** Placeholder que pone parseVcard cuando la tarjeta no trae FN: no agrupa. */
const NO_NAME = '(Sin nombre)';

/** Minúsculas sin espacios ni separadores: compara nombres/correos con tolerancia al formato. */
function norm(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[\s.\-_()]/g, '');
}

/** Solo dígitos: compara teléfonos ignorando espacios, guiones y paréntesis. */
function digits(value: string): string {
  return value.replace(/\D/g, '');
}

/** Claves de identidad de un contacto (correo/teléfono/nombre) ya normalizadas. */
function keysOf(c: Contact): string[] {
  const keys: string[] = [];
  for (const e of c.emails) {
    const k = norm(e);
    if (k) keys.push('e:' + k);
  }
  for (const p of c.phones) {
    const k = digits(p);
    if (k.length >= 6) keys.push('t:' + k); // ignora extensiones/números muy cortos
  }
  const name = norm(c.fullName);
  if (name && c.fullName.trim() !== NO_NAME) keys.push('n:' + name);
  return keys;
}

/**
 * Agrupa los contactos duplicados. Devuelve solo los grupos de 2+ fichas, cada
 * contacto en un único grupo. O(n · claves) con Union-Find (α casi constante).
 */
export function findDuplicateGroups(contacts: Contact[]): Contact[][] {
  const parent = contacts.map((_, i) => i);
  const find = (i: number): number => {
    while (parent[i] !== i) {
      parent[i] = parent[parent[i]!]!; // compresión de caminos
      i = parent[i]!;
    }
    return i;
  };
  const union = (a: number, b: number): void => {
    parent[find(a)] = find(b);
  };

  const firstWithKey = new Map<string, number>();
  contacts.forEach((c, i) => {
    for (const key of keysOf(c)) {
      const prev = firstWithKey.get(key);
      if (prev === undefined) firstWithKey.set(key, i);
      else union(prev, i);
    }
  });

  const byRoot = new Map<number, Contact[]>();
  contacts.forEach((c, i) => {
    const root = find(i);
    const group = byRoot.get(root);
    if (group) group.push(c);
    else byRoot.set(root, [c]);
  });
  return [...byRoot.values()].filter((g) => g.length > 1);
}
