/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La LETRA DE DOCS en la vista previa de una nota `doc` (api/doc-preview.ts).
 *
 * El export HTML de Docs no trae hoja de estilos —es el fichero que se descarga—, y
 * dentro del iframe salía con la del navegador: Times a 16px. Decidido por el dueño
 * (25/09): la vista previa se ve con la tipografía de la página de Docs (`.sytue-page`
 * de editor.css), que es como se escribió; la app nativa ya lo hacía así.
 *
 * La Figtree va EN LA PROPIA HOJA, como `data:`: el iframe tiene `sandbox` vacío y su
 * origen es opaco, así que pedir la fuente a esta app sería una petición entre orígenes
 * que el navegador bloquea sin cabeceras CORS. Solo el alfabeto latino, que es lo que
 * lleva casi todo documento; el resto cae en la fuente del sistema.
 */

/** La hoja de la vista previa, con la fuente ya dentro (`data:` de la Figtree). */
export function hojaVistaDoc(figtree: string): string {
  return (
    `<style>@font-face{font-family:'Figtree Variable';font-style:normal;font-display:swap;` +
    `font-weight:300 900;src:url(${figtree}) format('woff2-variations')}` +
    // `.sytue-page` de packages/office/src/editor/editor.css: letra, cuerpo y bloques.
    `body{color:#0a0a0a;font-family:'Figtree Variable','Inter Variable','Segoe UI',system-ui,` +
    `-apple-system,BlinkMacSystemFont,'Helvetica Neue',Arial,sans-serif;font-size:16px;` +
    `line-height:1.5;word-wrap:break-word}` +
    `h1{font-size:2em;font-weight:700;margin:.67em 0 .35em}` +
    `h2{font-size:1.5em;font-weight:600;margin:.83em 0 .35em}` +
    `h3{font-size:1.17em;font-weight:600;margin:1em 0 .35em}` +
    `p{margin:0 0 .5em}ul,ol{margin:0 0 .5em;padding-left:1.5em}` +
    `blockquote{margin:0 0 .5em;padding:2px 0 2px 12px;border-left:3px solid rgba(15,23,42,.16);` +
    `color:#475569;font-style:italic}hr{border:none;border-top:1px solid rgba(15,23,42,.16);margin:1em 0}` +
    `img{max-width:100%;height:auto}table{border-collapse:collapse;width:100%;table-layout:fixed;` +
    `margin:0 0 .5em}td,th{border:1px solid rgba(15,23,42,.16);padding:4px 8px;vertical-align:top}</style>`
  );
}

/** Mete la hoja en el `<head>` del export, o delante de todo si no lo trae. */
export function conLetraDeDocs(html: string, figtree: string): string {
  const hoja = hojaVistaDoc(figtree);
  const i = html.search(/<\/head>/i);
  return i < 0 ? hoja + html : html.slice(0, i) + hoja + html.slice(i);
}
