/* SPDX-License-Identifier: Apache-2.0 */

/**
 * «Subir carpeta» con un botón: el `<input type="file" webkitdirectory>`.
 *
 * Hasta ahora una carpeta solo se subía SOLTÁNDOLA, y en un portátil sin ratón —o con el
 * panel lateral del sistema cerrado— eso cuesta. El navegador da los ficheros de la
 * carpeta elegida en una lista plana, cada uno con su `webkitRelativePath`
 * («Proyecto/planos/a.dwg»); de ahí sale la misma forma `{file, dir}` que produce
 * `collectDropEntries` al soltar, y por tanto el mismo camino de subida, que crea las
 * subcarpetas antes de subir.
 */

/** Lo que consume la subida: el fichero y su subcarpeta relativa ('' = la abierta). */
export interface EntradaCarpeta {
  file: File;
  dir: string;
}

/**
 * entradasDeCarpeta — de la lista del input a entradas de subida. `dir` es la ruta
 * relativa SIN el nombre del fichero y empezando por la carpeta elegida, que es lo que
 * deja un drop. Se normalizan separadores y se descartan los tramos vacíos, «.» y «..»:
 * la ruta la escribe el navegador, pero acaba formando una ruta del servidor.
 */
export function entradasDeCarpeta(files: ArrayLike<File>): EntradaCarpeta[] {
  return Array.from(files, (file) => {
    const tramos = (file.webkitRelativePath || '')
      .replace(/\\/g, '/')
      .split('/')
      .filter((t) => t !== '' && t !== '.' && t !== '..');
    tramos.pop(); // el propio fichero
    return { file, dir: tramos.join('/') };
  });
}

/**
 * admiteSubirCarpeta — si el navegador sabe elegir una carpeta. Safari para iOS (y
 * cualquier navegador de iPhone o iPad, que es el mismo motor) tiene la propiedad pero
 * no el selector: allí la opción se oculta. Un iPad se anuncia como Mac; lo delata la
 * pantalla táctil. En Android tampoco se ofrece: según navegador y versión el input abre
 * el selector de FICHEROS, y la pieza pide que en el móvil no esté, como en la app nativa.
 */
export function admiteSubirCarpeta(
  input: object | null = typeof document === 'undefined' ? null : document.createElement('input'),
  nav: Pick<Navigator, 'userAgent' | 'maxTouchPoints'> | null = typeof navigator === 'undefined'
    ? null
    : navigator,
): boolean {
  if (!input || !nav || !('webkitdirectory' in input)) return false;
  const movil =
    /iPhone|iPad|iPod|Android/.test(nav.userAgent) ||
    (/Macintosh/.test(nav.userAgent) && (nav.maxTouchPoints ?? 0) > 1);
  return !movil;
}
