/**
 * Quita el prefijo del despliegue de una ruta antes de reenviarla al servicio Go.
 *
 * El navegador pide `/photos/api/v1/…` porque la app se sirve bajo su prefijo,
 * y `services/syphotos` solo monta `/api/v1`. Sin recortar, el servicio responde
 * 404 a todo y la galería enseña «Error: Sytue Fotos respondió 404».
 *
 * Vive aquí y no dentro del proxy para poder probarlo: la primera versión usaba
 * `new RegExp('^' + BASE_URL + 'api')` y no recortaba NADA, porque `BASE_URL`
 * llega sin barra final y el patrón quedaba en `^/photosapi`. Con test, ese
 * fallo dura lo que tarda en ejecutarse.
 */
export function rutaInterna(pathname: string, base: string): string {
  // Comparación de prefijo y no expresión regular: el prefijo es un dato, y como
  // dato puede traer caracteres que en un patrón significarían otra cosa.
  //
  // Se exige la barra (`${base}/`) para no morder un prefijo que solo COMPARTE
  // principio: con base `/photo`, la ruta `/photos/api` no es suya.
  return base && pathname.startsWith(`${base}/`) ? pathname.slice(base.length) : pathname;
}
