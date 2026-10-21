// e2e de Importar/Exportar vCard en Sytue Contacts (UI, Chromium).
// Exporta un contacto y comprueba que el .vcf descargado contiene su FN/EMAIL;
// luego importa un .vcf de dos tarjetas nuevas y verifica que aparecen en la
// lista y el aviso «N importados». Sesión vía storageState del admin dev.
import { readFileSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import { expect, test, type APIRequestContext } from '@playwright/test';
import { APPS } from './helpers';

const api = (path: string) => `${APPS.contacts}api/${path}`;

interface AddressBook {
  url: string;
  displayName: string;
}
interface Contact {
  href: string;
  fullName: string;
}

async function getLibreta(request: APIRequestContext): Promise<AddressBook> {
  const res = await request.get(api('addressbooks'));
  expect(res.status(), 'GET /api/addressbooks debe responder 200').toBe(200);
  const { addressBooks } = (await res.json()) as { addressBooks: AddressBook[] };
  const libreta = addressBooks.find((b) => b.displayName === 'Contactos');
  expect(libreta, 'debe existir la libreta «Contactos»').toBeTruthy();
  return libreta!;
}

/** Una tarjeta vCard 3.0 mínima para el fichero de importación. */
const card = (fn: string, email: string) =>
  `BEGIN:VCARD\r\nVERSION:3.0\r\nFN:${fn}\r\nEMAIL:${email}\r\nEND:VCARD\r\n`;

/** Limpieza: borra por API todo contacto de esta ejecución (contiene el token). */
async function limpiar(request: APIRequestContext, run: string): Promise<void> {
  const libreta = await getLibreta(request);
  const res = await request.get(api(`contacts?addressBookUrl=${encodeURIComponent(libreta.url)}`));
  if (res.status() !== 200) return;
  const { contacts } = (await res.json()) as { contacts: Contact[] };
  for (const c of contacts.filter((c) => c.fullName.includes(run))) {
    await request.delete(api('contact'), { data: { href: c.href } });
  }
}

test.describe('Contacts vCard', () => {
  // Compilación en frío del island (Vite sobre bind-mount) + CardDAV: margen amplio.
  test.slow();

  test('exporta la vCard de un contacto e importa un .vcf nuevo', async ({ page, request }) => {
    // Token único POR invocación (no a nivel de módulo): con --repeat-each cada
    // repetición usa nombres distintos, así un DELETE de limpieza que tarde en
    // propagarse no colisiona con la siguiente repetición.
    const run = randomUUID().slice(0, 8);
    const seedName = `E2E Export ${run}`;
    const seedEmail = `export-${run}@sytue.localhost`;
    const libreta = await getLibreta(request);

    // Semilla vía API (el flujo de alta ya está cubierto por contacts.spec.ts).
    const alta = await request.post(api('contact'), {
      data: { addressBookUrl: libreta.url, contactInput: { fullName: seedName, emails: [seedEmail], phones: [] } },
    });
    expect(alta.status(), 'POST /api/contact (semilla) debe responder 200').toBe(200);

    try {
      await page.goto(APPS.contacts, { waitUntil: 'domcontentloaded' });

      // Acota la lista al token de la ejecución y espera a que aparezca la semilla.
      const buscar = page.getByRole('searchbox', { name: 'Buscar contactos' });
      await expect(buscar).toBeVisible({ timeout: 30_000 });
      await buscar.fill(run);
      await expect(page.getByText(seedName)).toBeVisible({ timeout: 20_000 });

      // EXPORTAR: seleccionar el contacto y descargar su vCard desde la ficha.
      // «Exportar» y «Combinar con…» viven en el menú «Más acciones» de la
      // cabecera: cuatro botones de texto seguidos envolvían al traducirlos.
      await page.getByText(seedName).click();
      // Por nombre en los dos idiomas: la suite sirve en inglés por defecto y
      // este fichero se escribió cuando la UI solo hablaba español.
      await page
        .locator('header')
        .getByRole('button', { name: /Más acciones|More actions/ })
        .click();
      const [download] = await Promise.all([
        page.waitForEvent('download'),
        page.getByRole('menuitem', { name: /^(Exportar|Export)/ }).click(),
      ]);
      const vcf = readFileSync(await download.path(), 'utf8');
      expect(vcf, 'el .vcf exportado debe contener el FN del contacto').toContain(`FN:${seedName}`);
      expect(vcf, 'el .vcf exportado debe contener el EMAIL del contacto').toContain(seedEmail);

      // IMPORTAR: fichero .vcf con dos tarjetas nuevas (ejercita el multi-tarjeta).
      const impA = `E2E Import A ${run}`;
      const impB = `E2E Import B ${run}`;
      const fichero = card(impA, `import-a-${run}@sytue.localhost`) + card(impB, `import-b-${run}@sytue.localhost`);
      // Acotado por `accept`: la página tiene tres inputs de fichero ocultos
      // (vCard, CSV y foto del contacto) y el selector genérico los captaba todos.
      await page
        .locator('input[type="file"][accept*="vcard"]')
        .setInputFiles({ name: 'nuevos.vcf', mimeType: 'text/vcard', buffer: Buffer.from(fichero) });

      // Aviso «N importados» (requisito de la feature).
      await expect(page.getByText('2 contactos importados')).toBeVisible({ timeout: 15_000 });

      // Aparecen en la lista tras la recarga.
      await buscar.fill(run);
      await expect(page.getByText(impA)).toBeVisible({ timeout: 15_000 });
      await expect(page.getByText(impB)).toBeVisible({ timeout: 15_000 });
    } finally {
      await limpiar(request, run);
    }
  });
});
