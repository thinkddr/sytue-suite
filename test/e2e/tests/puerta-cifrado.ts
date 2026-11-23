/* SPDX-License-Identifier: Apache-2.0 */
/**
 * LA PUERTA DEL CIFRADO, vista desde un barrido (ADR 0084, 0086).
 *
 * Las apps con datos cifrados (Docus, Calcus, Diapos, Notes, Calendar, Contacts,
 * Drive, Fotos, Mail, One…) montan su vista DENTRO de `PqVaultGate`
 * (`@sytue/shell/matrix`). La puerta solo deja pasar si el puente de Sytue ID abre la
 * bóveda, y el puente abre con la clave de desbloqueo que la página de acceso del IdP
 * deriva de la contraseña al ENTRAR y guarda en este navegador como `CryptoKey` no
 * extraíble, en el IndexedDB `sytue-e2e` de `id.<dominio>`
 * (`services/id/internal/http/e2e/login.js` y `comun.js`).
 *
 * Un contexto de Playwright que arranca de un `storageState` guardado trae las cookies
 * (la sesión) pero NO ese IndexedDB —y una clave no extraíble no se puede serializar—,
 * así que la app responde con su puerta («Volver a entrar en Sytue ID») y la vista no
 * llega a montarse. Es lo correcto en el producto: sin la clave no hay nada que
 * descifrar. Para el barrido era un agujero: medía la puerta como si fuera la app.
 *
 * Se reconoce por el perfil del cifrado, que la puerta escribe en su texto
 * (`pqVaultProfile`): no depende del idioma ni de la redacción, y una vez abierta solo
 * reaparece dentro de su diálogo de detalles. `PqVaultGate.test.tsx` comprueba esta
 * función contra la puerta de verdad.
 */

/** `pqVaultProfile` de `@sytue/offline/vault`. */
export const PERFIL_CIFRADO = 'age-MLKEM768-X25519-v1';

/**
 * - `abierta`: no hay puerta en la página, o ya dejó pasar.
 * - `abriendo`: la puerta está preguntando al puente («Desbloqueando…»).
 * - `cerrada`: la puerta espera que se vuelva a entrar (o no pudo abrir).
 */
export type EstadoPuertaCifrado = 'abierta' | 'abriendo' | 'cerrada';

/**
 * Estado de la puerta en el documento actual. Va a `page.evaluate`, así que no
 * puede tocar nada de fuera: el perfil llega como argumento.
 */
export function estadoPuertaCifrado(perfil: string): EstadoPuertaCifrado {
  const marca = Array.from(document.querySelectorAll('main p')).find(
    (p) => (p.textContent ?? '').includes(perfil) && !p.closest('[role="dialog"], dialog'),
  );
  if (!marca) return 'abierta';
  return marca.parentElement?.querySelector('[role="status"]') ? 'abriendo' : 'cerrada';
}
