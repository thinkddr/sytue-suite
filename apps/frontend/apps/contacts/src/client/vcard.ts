/**
 * Importar/Exportar vCard (cliente).
 *  - EXPORTAR usa la vCard cruda que cada Contact ya trae (campo `vcard`, tal
 *    cual la sirve el servicio Go CardDAV): no hace falta un endpoint nuevo.
 *  - IMPORTAR parte el .vcf en tarjetas y manda cada una ENTERA al servicio
 *    (POST /api/import-vcard → `text/vcard`), que la entiende: así una foto
 *    binaria de vCard 3.0 se ve igual que en la app nativa.
 */
import { parseVcard } from '@sytue/api-clients/contacts';
import { importCard } from './contactsApi.js';

/** Descarga una o varias vCards como un único .vcf (attachment) en el navegador. */
export function downloadVcf(filename: string, vcards: string[]): void {
  // Cada vCard ya viene con sus saltos CRLF; el join produce un .vcf
  // multi-tarjeta válido (RFC 6350: tarjetas concatenadas).
  const body = vcards.map((v) => v.trim()).join('\r\n') + '\r\n';
  const url = URL.createObjectURL(new Blob([body], { type: 'text/vcard;charset=utf-8' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = filename.endsWith('.vcf') ? filename : `${filename}.vcf`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/**
 * Indica si una vCard cruda lleva una FOTO embebida (PHOTO con data URI). El
 * avatar se sirve aparte (bytes por el endpoint /api/photo, cacheable) para no
 * pintar el data URI entero en el DOM; esto solo decide si mostrar foto o
 * iniciales. Desdobla las líneas plegadas (RFC 6350 3.2: una PHOTO larga se pliega
 * en varias líneas físicas) antes de buscar la propiedad.
 */
export function hasEmbeddedPhoto(vcard: string): boolean {
  const unfolded = vcard.replace(/\r?\n[ \t]/g, '');
  for (const line of unfolded.split(/\r?\n/)) {
    // Nombre de propiedad sin prefijo de grupo (item1.PHOTO) ni parámetros.
    const head = line.slice(0, Math.max(line.indexOf(':'), 0));
    const prop = ((head.split(';')[0] ?? '').split('.').pop() ?? '').toUpperCase();
    if (prop === 'PHOTO' && /:\s*data:image\//i.test(line)) return true;
  }
  return false;
}

/** Nombre de fichero .vcf seguro a partir de un texto libre (conserva tildes). */
export function exportFilename(name: string): string {
  const base =
    name
      .trim()
      .replace(/[^\p{L}\p{N}]+/gu, '-')
      .replace(/^-+|-+$/g, '')
      .slice(0, 60) || 'contacto';
  return `${base}.vcf`;
}

/**
 * Las tarjetas de un .vcf, TAL CUAL, sin las que no traen nombre (FN: parseVcard
 * las marca como '(Sin nombre)'). Aquí no se interpreta nada más: lo demás lo lee
 * el servicio. Es el `Tarjetas` de la app nativa.
 */
export function tarjetasDe(text: string): string[] {
  const cards = text.match(/BEGIN:VCARD[\s\S]*?END:VCARD/gi) ?? [];
  return cards.filter((card) => {
    const { fullName } = parseVcard(card);
    return !!fullName && fullName !== '(Sin nombre)';
  });
}

/**
 * Importa un .vcf a la libreta `addressBookId` (id REST), una tarjeta por
 * petición y en serie para no ametrallar el servicio. Devuelve el recuento de
 * éxitos/fallos.
 *
 * `onProgress` recibe (hechas, total) antes de empezar y tras cada tarjeta. El
 * total se conoce ANTES de la primera escritura, así que el contador es real: no
 * es una barra inventada de las que la casa prohíbe. Importa: 2.000 tarjetas son
 * minutos, y sin contador no hay forma de distinguir «va bien» de «se ha colgado».
 */
export async function importVcf(
  addressBookId: string,
  text: string,
  onProgress?: (done: number, total: number) => void,
): Promise<{ imported: number; failed: number }> {
  let imported = 0;
  let failed = 0;
  const tarjetas = tarjetasDe(text);
  onProgress?.(0, tarjetas.length);
  for (const vcard of tarjetas) {
    try {
      await importCard(addressBookId, vcard);
      imported++;
    } catch {
      failed++;
    }
    onProgress?.(imported + failed, tarjetas.length);
  }
  return { imported, failed };
}
