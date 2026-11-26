/* SPDX-License-Identifier: Apache-2.0 */
/**
 * El código corto de por qué una bóveda no abrió (o su app no pudo prepararse),
 * para «Más información» y la consola. Sirve para saber en producción QUÉ
 * comprobación lo rechazó sin reproducirlo a mano.
 *
 * Solo lleva lo que es fijo en el código: el nombre de la clase del error, su
 * motivo cerrado (`reason`, `check`), el estado HTTP y el código de error del
 * servicio (`private_storage_snapshot_changed`). Nunca mensajes, URLs, nombres,
 * identificadores ni nada que venga de los datos de la persona. Va por nombres y
 * no por `instanceof` para no arrastrar aquí el motor de Drive ni el cliente HTTP.
 */
const token = /^[a-z0-9_]{1,64}$/u;
const className = /^[A-Za-z]{1,40}$/u;
// Lo que dice cada navegador cuando `fetch` no llega al servidor: Chromium, Firefox
// y Safari (en sus variantes de sin red, sin servidor y tiempo agotado).
const fetchFailure =
  /^(?:failed to fetch|networkerror when attempting to fetch resource|network error|load failed|the network connection was lost|could not connect to the server|the internet connection appears to be offline|a server with the specified hostname could not be found|the request timed out)/iu;

function part(value: unknown): string {
  return typeof value === 'string' && token.test(value) ? value : '';
}

/** El código de error de un servicio (`{"error":{"code"}}`, `{"code"}` o `{"error":"…"}`). */
function serviceCode(error: { code?: unknown; body?: unknown }): string {
  try {
    const direct = part(error.code);
    if (direct) return direct;
  } catch {
    // Un getter que falla no tapa el resto del código.
  }
  if (typeof error.body !== 'string' || error.body.length > 4096) return '';
  try {
    const root = JSON.parse(error.body) as Record<string, unknown> | null;
    if (!root || typeof root !== 'object') return '';
    const inner = root.error;
    if (inner && typeof inner === 'object') return part((inner as Record<string, unknown>).code);
    return part(root.code) || part(inner);
  } catch {
    return '';
  }
}

export function vaultFailureCode(error: unknown): string {
  if (!(error instanceof Error)) return 'unknown';
  const e = error as Error & {
    reason?: unknown;
    check?: unknown;
    status?: unknown;
    code?: unknown;
    body?: unknown;
    errors?: unknown;
  };
  const reason = part(e.reason),
    check = part(e.check);
  switch (e.name) {
    case 'PqVaultError':
      return 'vault.' + (reason || 'other') + (check ? '.' + check : '');
    case 'DriveImportError':
      return 'drive.import.' + (check || 'other');
    case 'PrivateDriveError':
      return 'drive.' + (reason || 'other') + (check ? '.' + check : '');
    case 'MatrixSecurityError':
      return 'security.' + (reason || 'other');
    case 'StorageRecordError':
      return 'record.' + (reason || 'other');
    case 'StorageMigrationError':
      return 'source.invalid';
    case 'CipherBlobError':
      return 'blob.invalid';
    case 'EncryptedOfflineError':
      return 'offline.unavailable';
    case 'ResponseTooLargeError':
      return 'http.too_large';
    case 'HttpError': {
      const status =
        typeof e.status === 'number' && Number.isSafeInteger(e.status) ? String(e.status) : 'other';
      const code = serviceCode(e);
      return 'http.' + status + (code ? '.' + code : '');
    }
    case 'AbortError':
    case 'TimeoutError':
      return 'aborted';
    case 'AggregateError': {
      const first = Array.isArray(e.errors) ? (e.errors as unknown[])[0] : undefined;
      return first === undefined ? 'aggregate' : vaultFailureCode(first);
    }
    // `fetch` sin red (o cortado por el navegador) es un TypeError, pero no todo
    // TypeError es la red: «Illegal invocation» o leer de `undefined` son fallos del
    // código, y contarlos como `network` escondió durante días el de la bóveda (ver
    // `PqVault`). El mensaje solo se compara, nunca se copia al código.
    case 'TypeError':
      return typeof e.message === 'string' && fetchFailure.test(e.message)
        ? 'network'
        : 'error.TypeError';
    default:
      return 'error.' + (className.test(e.name) ? e.name : 'other');
  }
}
