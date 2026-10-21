/**
 * Lee el claim `tenant` de un access token de Sytue ID.
 *
 * SIN VERIFICAR LA FIRMA, y da igual: esto no autoriza nada. La autoridad es el
 * backend, que rechaza con 403 `no_active_organization` cualquier token sin
 * organización. Aquí solo se decide si enseñar un aviso, y para eso el peor caso
 * de un token manipulado es que alguien se oculte a sí mismo un mensaje.
 *
 * Existe porque un token emitido en modo bootstrap —sesión SIN organización,
 * cuando la suscripción de esa organización no da derecho de uso— deja las diez
 * superficies mostrando «no se pudieron cargar los datos» a la vez, sin decir en
 * ninguna qué pasa. La shell es el único sitio que está en las diez.
 */
export function tokenTieneOrganizacion(token: string | undefined | null): boolean {
  if (!token) return false;
  const partes = token.split('.');
  // Tres partes o no es un JWT que esta shell entienda. Se calla, igual que ante
  // un payload ilegible: lo que NO puede hacer es tomarlo por «sin organización»
  // y avisar de una suscripción ausente a quien la tiene pagada.
  if (partes.length !== 3) return true;
  try {
    // base64url → base64. `atob` no acepta `-` ni `_`, y el relleno es opcional
    // en JWT: sin reponerlo, un payload de longitud no múltiplo de 4 lanza.
    let b64 = partes[1]!.replace(/-/g, '+').replace(/_/g, '/');
    b64 += '='.repeat((4 - (b64.length % 4)) % 4);
    const payload: unknown = JSON.parse(
      decodeURIComponent(
        atob(b64)
          .split('')
          .map((c) => '%' + c.charCodeAt(0).toString(16).padStart(2, '0'))
          .join(''),
      ),
    );
    const tenant = (payload as { tenant?: unknown } | null)?.tenant;
    return typeof tenant === 'string' && tenant.length > 0;
  } catch {
    // Un token ilegible NO es un token sin organización: puede ser un formato que
    // esta versión de la shell no conoce. Se calla y deja que el backend hable,
    // que es quien decide de verdad.
    return true;
  }
}
