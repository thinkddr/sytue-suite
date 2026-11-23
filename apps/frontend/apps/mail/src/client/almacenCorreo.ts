/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Dónde vive el correo privado de una persona (ADR 0087, entrega 4): en la bóveda
 * post-cuántica directa o en Matrix. Lo dice la autoridad del correo que sirve
 * Rooms, que tiene UNA por ámbito:
 *
 * - de la bóveda (identidad `…:sytue-vault`) → bóveda;
 * - de Matrix → Matrix: quien ya abre su correo por Matrix sigue igual;
 * - ninguna todavía (`storage_root_enrollment_required`) → cuenta nueva → bóveda.
 *
 * Ante cualquier otra respuesta, Matrix: es el camino de siempre, y elegir la
 * bóveda sin saberlo podría dejar a alguien sin abrir el correo que ya tiene.
 */
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';

export type AlmacenCorreo = 'boveda' | 'matrix';

/** El conmutador de despliegue (variable del BFF de Mail, apagada por defecto). */
export function correoSobreBoveda(valor: string | undefined): boolean {
  return valor === 'activado';
}

export async function elegirAlmacenCorreo(options: {
  basePath: string;
  actor: string;
  tenant: string | undefined;
  fetchImpl?: typeof fetch;
  signal?: AbortSignal;
}): Promise<AlmacenCorreo> {
  if (!options.tenant) return 'matrix';
  const f = options.fetchImpl ?? fetch;
  const base =
    !options.basePath || options.basePath === '/' ? '' : options.basePath.replace(/\/+$/, '');
  let res: Response;
  try {
    res = await f(`${base}/api/matrix/storage/authority/`, {
      method: 'GET',
      credentials: 'same-origin',
      headers: {
        [storageScopeHeader]: encodeStorageScopeHeader({
          tenant: options.tenant,
          subject: options.actor,
          product: 'mail',
        }),
      },
      ...(options.signal ? { signal: options.signal } : {}),
    });
  } catch {
    options.signal?.throwIfAborted();
    return 'matrix';
  }
  const body = (await res.json().catch(() => null)) as {
    user_id?: unknown;
    code?: unknown;
    error?: { code?: unknown };
  } | null;
  if (res.ok) {
    return typeof body?.user_id === 'string' && body.user_id.endsWith(':sytue-vault')
      ? 'boveda'
      : 'matrix';
  }
  const code = body?.code ?? body?.error?.code;
  if (res.status === 409 && code === 'storage_root_enrollment_required') return 'boveda';
  return 'matrix';
}

/**
 * El traspaso del correo de Matrix a la bóveda (ADR 0087) para quien no puede
 * abrir su correo de Matrix: «vaciar y pasar a la bóveda», con confirmación.
 *
 * - `matrix` con un traspaso pedido en esta pestaña → `traspaso`: se abre una
 *   bóveda NUEVA (`PqVault.reset`) y su clave se registra con la marca de
 *   traspaso, que Sytue ID solo acepta con identificación reciente. Si pide volver
 *   a entrar, la marca sobrevive a la vuelta en `sessionStorage` (esta pestaña).
 * - `boveda` gana SIEMPRE: con el traspaso hecho (o sin Matrix) se abre la bóveda
 *   que hay, nunca se vacía otra vez, y la marca se olvida.
 */
export type DestinoCorreo = AlmacenCorreo | 'traspaso';

export function destinoCorreo(elegido: AlmacenCorreo, traspasoPedido: boolean): DestinoCorreo {
  if (elegido === 'boveda') return 'boveda';
  return traspasoPedido ? 'traspaso' : 'matrix';
}

type Almacen = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

function claveTraspaso(actor: string, tenant: string | undefined): string {
  return `sytue.mail.traspaso:${tenant ?? ''}:${actor}`;
}

/** La marca vale solo para esta persona y organización, y nunca rompe nada si no hay almacén. */
export function traspasoPedido(
  almacen: Almacen | undefined,
  actor: string,
  tenant: string | undefined,
): boolean {
  try {
    return almacen?.getItem(claveTraspaso(actor, tenant)) === '1';
  } catch {
    return false;
  }
}

export function pedirTraspaso(
  almacen: Almacen | undefined,
  actor: string,
  tenant: string | undefined,
): void {
  try {
    almacen?.setItem(claveTraspaso(actor, tenant), '1');
  } catch {
    /* Sin almacén, el traspaso sigue en esta vista; tras volver a entrar se pide otra vez. */
  }
}

export function olvidarTraspaso(
  almacen: Almacen | undefined,
  actor: string,
  tenant: string | undefined,
): void {
  try {
    almacen?.removeItem(claveTraspaso(actor, tenant));
  } catch {
    /* Nada que olvidar. */
  }
}

/** `sessionStorage` de la pestaña, si lo hay. */
export function almacenDePestana(): Almacen | undefined {
  try {
    return globalThis.sessionStorage ?? undefined;
  } catch {
    return undefined;
  }
}
