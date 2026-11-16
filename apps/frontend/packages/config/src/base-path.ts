/* SPDX-License-Identifier: Apache-2.0 */

export function stripTrailingSlashes(value: string): string {
  let end = value.length;
  while (end > 0 && value.charCodeAt(end - 1) === 47) end--;
  return value.slice(0, end);
}

export function withBase(base: string, path: string): string {
  // El prefijo del HOST manda sobre el del build cuando lo hay. La consola se
  // sirve bajo `/admin` en `admin.<dominio>` y en la RAÍZ en `platform.<dominio>`
  // —misma imagen, dos superficies—, pero `import.meta.env.BASE_URL` lo sustituye
  // Vite al construir, así que vale `/admin` en las dos y en plataforma mandaba a
  // `/admin/...`, que ahí es un 404 medido. Lo publica `basePathScript` con el
  // valor que el servidor conoce (`SYTUE_BASE_PATH`).
  const delHost =
    typeof window !== 'undefined' ? (window as { __sytueBase?: string }).__sytueBase : undefined;
  const b = stripTrailingSlashes(delHost ?? base ?? '/');
  if (!b || !path.startsWith('/')) return path;
  return path === b || path.startsWith(`${b}/`) ? path : `${b}${path}`;
}
