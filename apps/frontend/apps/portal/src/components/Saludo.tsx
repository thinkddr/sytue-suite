/* SPDX-License-Identifier: Apache-2.0 */
/**
 * El saludo de la portada («Buenas tardes, Daniel»), con el nombre entero.
 *
 * El título donde va (`.sytue-h1`) lleva `hyphens: auto` para que las palabras largas
 * de la web pública no desborden el móvil, y en un teléfono partía el nombre de la
 * persona con guion («Da-niel»). El nombre va con `hyphens-none`: si no cabe, pasa
 * entero a la línea de abajo. Solo uno más ancho que la línea entera se cortaría, y sin
 * guion (`overflow-wrap: break-word` del título).
 */
export function Saludo({ texto, nombre }: { texto: string; nombre?: string | undefined }) {
  if (!nombre) return <>{texto}</>;
  return (
    <>
      {texto},{' '}
      <span class="hyphens-none" data-nombre-saludo>
        {nombre}
      </span>
    </>
  );
}
