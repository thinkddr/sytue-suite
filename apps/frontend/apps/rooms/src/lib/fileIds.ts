/**
 * Saneado de la lista de file_ids (adjuntos de Sytue Drive) que llega al BFF desde
 * el navegador: acepta solo UUIDs bien formados, quita duplicados y acota el número
 * por mensaje (frontera de confianza — nunca fiarse del cliente). El servicio Go
 * revalida y resuelve cada id contra Drive.
 */
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Máximo de adjuntos por mensaje (acota resolveAttachments en el backend). */
export const MAX_ATTACHMENTS = 10;

export function sanitizeFileIds(input: unknown): string[] {
  if (!Array.isArray(input)) return [];
  const out: string[] = [];
  const seen = new Set<string>();
  for (const v of input) {
    if (typeof v !== 'string') continue;
    const id = v.trim().toLowerCase();
    if (!UUID_RE.test(id) || seen.has(id)) continue;
    seen.add(id);
    out.push(id);
    if (out.length >= MAX_ATTACHMENTS) break;
  }
  return out;
}
