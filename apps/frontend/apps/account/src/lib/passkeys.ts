/*
 * Alta de una passkey desde el navegador.
 *
 * `navigator.credentials.create()` no se puede proxyar: el diálogo lo abre el
 * navegador y habla con el autenticador (Touch ID, una llave USB, el móvil). El
 * servidor solo pone el reto antes y verifica la attestation después, y por eso
 * esta parte vive aquí y no en el BFF.
 *
 * Los buffers viajan en base64url en las dos direcciones, que es lo que el
 * servicio guarda y compara. Es la misma conversión que hacía el script del HTML
 * de Go, traída tal cual porque ya estaba probada contra autenticadores reales.
 */

/** base64url → ArrayBuffer (el reto y los identificadores que manda el servidor). */
function aBuffer(s: string): ArrayBuffer {
  const b64 = s
    .replace(/-/g, '+')
    .replace(/_/g, '/')
    .padEnd(Math.ceil(s.length / 4) * 4, '=');
  const bin = atob(b64);
  const a = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i += 1) a[i] = bin.charCodeAt(i);
  return a.buffer;
}

/** ArrayBuffer → base64url (lo que devuelve el autenticador). */
function aBase64Url(buf: ArrayBuffer): string {
  const a = new Uint8Array(buf);
  let s = '';
  for (let i = 0; i < a.length; i += 1) s += String.fromCharCode(a[i] as number);
  return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/** Por qué no se pudo dar de alta. Cada caso se cuenta distinto. */
export type FalloPasskey =
  /** Cancelado en el diálogo del sistema. No es un fallo del que informar como si algo se hubiera roto. */
  | 'cancelado'
  /** Esa llave ya está registrada en esta cuenta: es la respuesta a lo que se preguntaba. */
  | 'duplicada'
  /** El navegador no sabe de passkeys. */
  | 'no_soportado'
  | 'error';

export function soportaPasskeys(): boolean {
  return (
    typeof window !== 'undefined' &&
    'PublicKeyCredential' in window &&
    typeof navigator.credentials?.create === 'function'
  );
}

/**
 * Da de alta una passkey. Devuelve `null` si fue bien, o el motivo si no.
 *
 * Los motivos se distinguen por el `name` de la excepción del navegador: sin eso,
 * cancelar el diálogo, tener la llave ya registrada y que fallara el servidor se
 * leen igual, y la persona no sabe si volver a intentarlo.
 */
export async function altaPasskey(nombre: string): Promise<FalloPasskey | null> {
  if (!soportaPasskeys()) return 'no_soportado';
  try {
    const res = await fetch('/api/passkeys/begin', { method: 'POST' });
    if (!res.ok) return 'error';
    const o = (await res.json()) as {
      challenge: string;
      rp: PublicKeyCredentialRpEntity;
      user: { id: string; name: string; displayName: string };
      pubKeyCredParams: PublicKeyCredentialParameters[];
      timeout?: number;
      attestation?: AttestationConveyancePreference;
      excludeCredentials?: { type: string; id: string }[];
      authenticatorSelection?: AuthenticatorSelectionCriteria;
    };

    const cred = (await navigator.credentials.create({
      publicKey: {
        challenge: aBuffer(o.challenge),
        rp: o.rp,
        user: {
          id: aBuffer(o.user.id),
          name: o.user.name,
          displayName: o.user.displayName,
        },
        pubKeyCredParams: o.pubKeyCredParams,
        ...(o.timeout !== undefined ? { timeout: o.timeout } : {}),
        ...(o.attestation !== undefined ? { attestation: o.attestation } : {}),
        excludeCredentials: (o.excludeCredentials ?? []).map((k) => ({
          type: 'public-key' as const,
          id: aBuffer(k.id),
        })),
        ...(o.authenticatorSelection !== undefined
          ? { authenticatorSelection: o.authenticatorSelection }
          : {}),
      },
    })) as PublicKeyCredential | null;
    if (!cred) return 'error';

    const att = cred.response as AuthenticatorAttestationResponse;
    const fin = await fetch('/api/passkeys/finish', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        id: cred.id,
        rawId: aBase64Url(cred.rawId),
        type: cred.type,
        name: nombre,
        attestationObject: aBase64Url(att.attestationObject),
        clientDataJSON: aBase64Url(att.clientDataJSON),
      }),
    });
    return fin.ok ? null : 'error';
  } catch (e) {
    const nombreErr = e instanceof Error ? e.name : '';
    if (nombreErr === 'NotAllowedError') return 'cancelado';
    if (nombreErr === 'InvalidStateError') return 'duplicada';
    return 'error';
  }
}
