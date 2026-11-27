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
 * Se reconoce por `data-sytue-boveda` («abriendo» o «cerrada»), que la puerta pone en
 * la pantalla que tapa la app (o en el esqueleto que enseña mientras abre): no depende
 * del idioma ni de la redacción. Las apps que no guardan nada en la bóveda la abren en
 * segundo plano y nunca la tapan: para ellas la puerta está siempre «abierta». Antes se
 * reconocía por el perfil del cifrado en el texto; desde que el perfil va dentro de
 * «Más información» se mira la marca, y el perfil queda para las puertas desplegadas
 * antes. `PqVaultGate.test.tsx` comprueba esta función contra la puerta de verdad.
 */

/** `pqVaultProfile` de `@sytue/offline/vault`. */
export const PERFIL_CIFRADO = "age-MLKEM768-X25519-v1";

/**
 * Por dónde abre Mail el correo de la cuenta: `boveda` (la puerta de `PqVaultGate` y
 * `vault/mail-private.ts`), `matrix`, `retirada` o `dudoso`. Es la misma pregunta que
 * hace `elegirAlmacenCorreo` (apps/mail/src/client/almacenCorreo.ts) a la autoridad de
 * Rooms, de solo lectura.
 *
 * POR QUÉ IMPORTA. Si la cuenta va por Matrix, la puerta de la bóveda no se monta y
 * nada de lo que se mida dice si la bóveda abre: v4.0.2 dejó roto el correo sobre la
 * bóveda (`mail.capacidades…host.authority.revalidate`) y la cuenta de auditoría, que
 * iba por Matrix, lo abría sin problema. Va a `page.evaluate`: no toca nada de fuera.
 */
export async function almacenCorreo(
  base: string,
): Promise<"boveda" | "matrix" | "retirada" | "dudoso"> {
  try {
    const s = await fetch(`${base}/auth/session`, { credentials: "same-origin" });
    const sesion = (await s.json()) as { user?: { id?: unknown }; tenant?: unknown };
    const subject = sesion.user?.id;
    const tenant = sesion.tenant;
    if (typeof subject !== "string" || typeof tenant !== "string") return "dudoso";
    const raw = new TextEncoder().encode(JSON.stringify({ tenant, subject, product: "mail" }));
    const ambito =
      "v1." +
      btoa(String.fromCharCode(...raw))
        .replaceAll("+", "-")
        .replaceAll("/", "_")
        .replace(/=+$/u, "");
    const res = await fetch(`${base}/api/matrix/storage/authority/`, {
      credentials: "same-origin",
      headers: { "X-Sytue-Storage-Scope": ambito },
    });
    const body = (await res.json().catch(() => null)) as {
      user_id?: unknown;
      code?: unknown;
      error?: { code?: unknown };
    } | null;
    if (res.ok)
      return typeof body?.user_id === "string" && body.user_id
        ? body.user_id.endsWith(":sytue-vault")
          ? "boveda"
          : "matrix"
        : "dudoso";
    const code = body?.code ?? body?.error?.code;
    if (res.status === 409 && code === "storage_root_enrollment_required") return "boveda";
    if (res.status === 409 && code === "storage_matrix_retired") return "retirada";
    return "dudoso";
  } catch {
    return "dudoso";
  }
}

/**
 * - `abierta`: no hay puerta en la página, o ya dejó pasar.
 * - `abriendo`: la puerta está preguntando al puente («Desbloqueando…»).
 * - `cerrada`: la puerta espera que se vuelva a entrar (o no pudo abrir).
 */
export type EstadoPuertaCifrado = "abierta" | "abriendo" | "cerrada";

/**
 * Estado de la puerta en el documento actual. Va a `page.evaluate`, así que no
 * puede tocar nada de fuera: el perfil llega como argumento.
 */
export function estadoPuertaCifrado(perfil: string): EstadoPuertaCifrado {
  const puerta = Array.from(
    document.querySelectorAll("[data-sytue-boveda]"),
  ).find((el) => !el.closest('[role="dialog"], dialog'));
  if (puerta)
    return puerta.getAttribute("data-sytue-boveda") === "abriendo"
      ? "abriendo"
      : "cerrada";
  // Puertas de antes de la marca: el perfil del cifrado en el texto de la pantalla.
  const marca = Array.from(document.querySelectorAll("main p")).find(
    (p) =>
      (p.textContent ?? "").includes(perfil) &&
      !p.closest('[role="dialog"], dialog'),
  );
  if (!marca) return "abierta";
  return marca.parentElement?.querySelector('[role="status"]')
    ? "abriendo"
    : "cerrada";
}
