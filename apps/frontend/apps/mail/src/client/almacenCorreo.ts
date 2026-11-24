/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Dónde vive el correo privado de una persona (ADR 0087, entrega 4): en la bóveda
 * post-cuántica directa o en Matrix. Lo dice la autoridad del correo que sirve
 * Rooms, que tiene UNA por ámbito:
 *
 * - de la bóveda (identidad `…:sytue-vault`) → bóveda;
 * - de Matrix → `matrix`: su correo está cifrado con el sistema anterior, y se le
 *   ofrece pasarlo a la bóveda (vaciándolo) o seguir por Matrix;
 * - Matrix retirada (`storage_matrix_retired`, entrega 5: Rooms ya no honra la
 *   autoridad de Matrix del correo) → `retirada`: solo queda pasar a la bóveda;
 * - ninguna todavía (`storage_root_enrollment_required`) → cuenta nueva → bóveda.
 *
 * Ante cualquier otra respuesta, `dudoso`: se va por Matrix, el camino de siempre,
 * sin ofrecer vaciar nada. Elegir la bóveda (o proponer vaciar) sin saber qué hay
 * podría dejar a alguien sin el correo que ya tiene. Si este navegador ya oyó a
 * Rooms decir `retirada`, la duda tampoco abre Matrix (ver `destinoCorreo`).
 */
import { encodeStorageScopeHeader, storageScopeHeader } from '@sytue/config/matrix';

export type AlmacenCorreo = 'boveda' | 'matrix' | 'retirada' | 'dudoso';

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
  if (!options.tenant) return 'dudoso';
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
    return 'dudoso';
  }
  const body = (await res.json().catch(() => null)) as {
    user_id?: unknown;
    code?: unknown;
    error?: { code?: unknown };
  } | null;
  if (res.ok) {
    if (typeof body?.user_id !== 'string' || body.user_id === '') return 'dudoso';
    return body.user_id.endsWith(':sytue-vault') ? 'boveda' : 'matrix';
  }
  const code = body?.code ?? body?.error?.code;
  if (res.status === 409 && code === 'storage_root_enrollment_required') return 'boveda';
  if (res.status === 409 && code === 'storage_matrix_retired') return 'retirada';
  return 'dudoso';
}

/**
 * El traspaso del correo de Matrix a la bóveda (ADR 0087): «vaciar y pasar a la
 * bóveda», con confirmación, tanto si el correo de Matrix abre como si no.
 *
 * - `boveda` gana SIEMPRE: con el traspaso hecho (o sin Matrix) se abre la bóveda
 *   que hay, nunca se vacía otra vez, y la marca se olvida.
 * - Con un traspaso pedido en esta pestaña → `traspaso`: se abre una bóveda NUEVA
 *   (`PqVault.reset`) y su clave se registra con la marca de traspaso, que Sytue
 *   ID solo acepta con identificación reciente. Si pide volver a entrar, la marca
 *   sobrevive a la vuelta en `sessionStorage` (esta pestaña).
 * - `retirada` (Rooms ya no honra Matrix) → `pasar`: la misma pantalla pero SIN
 *   «seguir con el anterior», que ya no existe; el texto dice lo que se pierde.
 * - `matrix` (autoridad de Matrix confirmada) → `elegir`: la pantalla que ofrece
 *   pasar el correo a la bóveda o seguir por ahora con el anterior (`seguir`).
 * - `dudoso` → `matrix`: el camino de siempre, sin proponer vaciar. Salvo que
 *   Rooms ya dijera que Matrix está retirada (`matrixRetirada`, lo que recuerda
 *   `aprenderRetirada`): entonces `reintentar`, un aviso con «Reintentar», porque
 *   Matrix ya no abriría.
 */
export type DestinoCorreo = 'boveda' | 'matrix' | 'traspaso' | 'elegir' | 'pasar' | 'reintentar';

export function destinoCorreo(
  elegido: AlmacenCorreo,
  traspasoPedido: boolean,
  seguirConMatrix = false,
  matrixRetirada = false,
): DestinoCorreo {
  if (elegido === 'boveda') return 'boveda';
  if (traspasoPedido) return 'traspaso';
  if (elegido === 'retirada') return 'pasar';
  if (elegido === 'dudoso' && matrixRetirada) return 'reintentar';
  return elegido === 'matrix' && !seguirConMatrix ? 'elegir' : 'matrix';
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

function claveSeguir(actor: string, tenant: string | undefined): string {
  return `sytue.mail.seguirMatrix:${tenant ?? ''}:${actor}`;
}

/**
 * «Seguir con el correo anterior por ahora»: en esta pestaña no se vuelve a
 * proponer pasar a la bóveda. En otra pestaña, o tras cerrar el navegador, sí.
 */
export function seguirConMatrix(
  almacen: Almacen | undefined,
  actor: string,
  tenant: string | undefined,
): void {
  try {
    almacen?.setItem(claveSeguir(actor, tenant), '1');
  } catch {
    /* Sin almacén, la elección vale para esta vista. */
  }
}

export function quiereSeguirConMatrix(
  almacen: Almacen | undefined,
  actor: string,
  tenant: string | undefined,
): boolean {
  try {
    return almacen?.getItem(claveSeguir(actor, tenant)) === '1';
  } catch {
    return false;
  }
}

function claveRetirada(actor: string, tenant: string | undefined): string {
  return `sytue.mail.matrixRetirada:${tenant ?? ''}:${actor}`;
}

/**
 * Lo que una respuesta clara de Rooms enseña de la retirada de Matrix, y lo que
 * este navegador sabe después: `retirada` se recuerda; `matrix` (el conmutador de
 * Rooms está apagado) se olvida; la bóveda y la duda no cambian nada. Se recuerda
 * en `localStorage` (todas las pestañas) porque es del servidor, no de una
 * pestaña; sin almacén, solo vale la respuesta de ahora.
 */
export function aprenderRetirada(
  almacen: Almacen | undefined,
  actor: string,
  tenant: string | undefined,
  elegido: AlmacenCorreo,
): boolean {
  try {
    if (elegido === 'retirada') almacen?.setItem(claveRetirada(actor, tenant), '1');
    else if (elegido === 'matrix') almacen?.removeItem(claveRetirada(actor, tenant));
    return elegido === 'retirada' || almacen?.getItem(claveRetirada(actor, tenant)) === '1';
  } catch {
    return elegido === 'retirada';
  }
}

/** `localStorage` del navegador, si lo hay. */
export function almacenDelNavegador(): Almacen | undefined {
  try {
    return globalThis.localStorage ?? undefined;
  } catch {
    return undefined;
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
