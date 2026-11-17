/*
 * El desbloqueo del cifrado al tocar la seguridad de la cuenta (ADR 0085).
 *
 * «Mi cuenta» se sirve en el origen de Sytue ID, que es donde vive la clave de
 * desbloqueo de este navegador (como CryptoKey no extraíble). Dos momentos la tocan:
 *
 * - Al cambiar la contraseña, el puente de Sytue ID cierra la clave con la nueva si la
 *   actual abre el sobre vigente, y ese sobre viaja con el cambio: la contraseña nueva
 *   desbloquea desde ya, sin esperar al siguiente inicio de sesión.
 * - Al dar de alta una passkey con PRF, la página /e2e/passkey de Sytue ID le hace su
 *   sobre en el momento (pide la passkey una vez) y vuelve aquí.
 *
 * Nada de esto bloquea: si falla, el cambio o el alta siguen y el sobre se rehace al
 * entrar, como antes.
 */
import { appDomains } from '@sytue/config';
import { passwordEnvelopeInBridge } from '@sytue/offline/matrix/unlock-bridge';

/** El sobre de la contraseña nueva, o null si este navegador no puede hacerlo. */
export async function sobreDeContrasena(
  sub: string,
  actual: string,
  nueva: string,
): Promise<Record<string, unknown> | null> {
  if (!sub) return null;
  try {
    const r = await passwordEnvelopeInBridge(
      { idOrigin: appDomains.id, subject: sub },
      { current: actual, next: nueva },
    );
    return r.ok ? r.envelope : null;
  } catch {
    return null;
  }
}

/**
 * A dónde ir tras dar de alta una passkey que admite PRF: la página de Sytue ID que
 * le hace su sobre y vuelve a `vuelta`. `null` si no hay nada que hacer.
 */
export function paginaDeLaPasskeyNueva(credencial: string, vuelta: string): string | null {
  if (!credencial || !URL.canParse(appDomains.id)) return null;
  const url = new URL('/e2e/passkey', appDomains.id);
  url.searchParams.set('cred', credencial);
  url.searchParams.set('return_to', vuelta);
  return url.toString();
}
