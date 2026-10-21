/**
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Confirmar la identidad (step-up) sin salir de lo que se estaba haciendo.
 *
 * Las acciones sensibles —quitar a alguien de una organización, restablecer una
 * contraseña— exigen haberse autenticado hace poco. Sytue ID contesta 401
 * `insufficient_user_authentication` cuando no es así. Antes, la consola decía «cierra
 * sesión y vuelve a entrar»: ahora lleva al login con `reauth=1` (que pide `max_age=0`
 * a Sytue ID) y vuelve a esta misma página, donde basta con repetir la acción.
 */
import { r } from './rutas.js';

export async function pedirReautenticacion(res: Response): Promise<boolean> {
  if (res.status !== 401) return false;
  const c = (await res
    .clone()
    .json()
    .catch(() => ({}))) as { code?: string };
  if (c.code !== 'insufficient_user_authentication') return false;
  const aqui = window.location.pathname + window.location.search;
  window.location.assign(r(`/auth/login?reauth=1&returnTo=${encodeURIComponent(aqui)}`));
  return true;
}
