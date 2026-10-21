// e2e de la Búsqueda global federada de la Sytue Shell (topbar) vía Sytue Hub.
//
// La shell (isla Preact del portal) llama a hub.sytue.localhost/api/v1/search con
// el access token de la sesión (resuelto en el navegador por el BFF /auth/token) y
// pinta un popover con los resultados AGRUPADOS por producto. Usa el storageState
// de admin del proyecto (cookie de sesión en .sytue.localhost): el fixture
// `request` crea datos vía los BFF de Drive/Contacts, y `page` maneja la UI.
//
// AUDITORÍA (2026-07): la federación del Hub es código real pero contra endpoints
// upstream inexistentes. Solo Drive+Contacts tienen endpoint de búsqueda; sus
// conectores se corrigieron en services/hub/internal/connectors (ruta/forma). Con
// el hub RECOMPILADO, /search federa Drive+Contacts (verificado a mano). El
// contenedor en ejecución aún trae los conectores antiguos -> el test de
// federación va como test.fixme hasta el REBUILD del contenedor `hub`.
//
// Calendar/Chat/Mail NO exponen búsqueda federable (sin endpoint upstream): fuera
// de alcance, no se inventan. Su conector degrada como error parcial (no rompe).
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { APPS } from './helpers';

const DRIVE = APPS.drive; // https://drive.sytue.localhost/
const DRIVE_ORIGIN = DRIVE.replace(/\/$/, '');
const CONTACTS = APPS.contacts;

/**
 * Término único de búsqueda: un solo lexema alfanumérico (sin guiones ni
 * caracteres que websearch_to_tsquery('simple') de Contacts trocee), así casa
 * tanto el LIKE por nombre de Drive como el tsquery de Contacts. El prefijo
 * distingue el propósito; el sufijo evita colisiones entre workers.
 */
const termino = (prefijo: string): string =>
  `${prefijo}${Date.now()}${crypto.randomUUID().replace(/-/g, '').slice(0, 6)}`;

interface AddressBook {
  url: string;
  displayName: string;
}

async function libretaContactos(request: APIRequestContext): Promise<AddressBook> {
  const res = await request.get(`${CONTACTS}api/addressbooks`);
  expect(res.status(), 'GET /api/addressbooks debe responder 200').toBe(200);
  const { addressBooks } = (await res.json()) as { addressBooks: AddressBook[] };
  const libreta = addressBooks.find((b) => b.displayName === 'Contactos') ?? addressBooks[0];
  expect(libreta, 'debe existir una libreta de contactos').toBeTruthy();
  return libreta;
}

/** Crea un archivo de texto en la raíz de Drive (octet-stream + ?type= para
 * esquivar el checkOrigin de Astro, patrón de apps/drive/.../content.ts). */
async function crearArchivoDrive(request: APIRequestContext, nombre: string): Promise<void> {
  const res = await request.put(`${DRIVE}api/content?path=/${nombre}.txt&type=text%2Fplain`, {
    headers: { origin: DRIVE_ORIGIN, 'content-type': 'application/octet-stream' },
    data: `contenido de prueba ${nombre}`,
  });
  expect(res.status(), 'PUT /api/content debe crear el archivo (201)').toBe(201);
}

async function borrarArchivoDrive(request: APIRequestContext, nombre: string): Promise<void> {
  await request.delete(`${DRIVE}api/files?path=/${nombre}.txt`, {
    headers: { origin: DRIVE_ORIGIN, 'content-type': 'application/octet-stream' },
  });
}

/** Crea un contacto y devuelve su href (para limpieza). */
async function crearContacto(
  request: APIRequestContext,
  libretaUrl: string,
  nombre: string,
): Promise<string> {
  const res = await request.post(`${CONTACTS}api/contact`, {
    data: {
      addressBookUrl: libretaUrl,
      contactInput: {
        fullName: `${nombre} Persona`,
        emails: [`${nombre}@sytue.localhost`],
        phones: [],
      },
    },
  });
  expect(res.status(), 'POST /api/contact debe responder 200').toBe(200);
  const body = (await res.json()) as { href: string };
  return body.href;
}

async function borrarContacto(request: APIRequestContext, href: string): Promise<void> {
  await request.delete(`${CONTACTS}api/contact`, { data: { href } });
}

/** Enfoca el buscador global de la topbar y teclea la consulta. Escritura real
 * carácter a carácter: el input controlado de Preact no reacciona a fill(). */
async function buscar(page: Page, q: string): Promise<void> {
  const input = page.getByRole('searchbox', { name: 'Búsqueda global' });
  await expect(input, 'la topbar debe montar el buscador global').toBeVisible({ timeout: 30_000 });
  await input.click();
  await input.pressSequentially(q, { delay: 25 });
}

test.describe('Búsqueda global federada (Sytue Shell)', () => {
  // El dev server (Vite sobre bind-mount) compila rutas bajo demanda; margen holgado.
  test.describe.configure({ timeout: 120_000 });

  test('consulta el Hub y muestra el estado «sin resultados» para un término inexistente', async ({
    page,
  }) => {
    // Término imposible de casar: prueba el cableado input -> debounce -> Hub ->
    // mapeo de la respuesta -> popover, sin depender de datos ni del rebuild.
    const prefijo = 'zznada';
    const q = termino(prefijo);
    await page.goto(APPS.one, { waitUntil: 'domcontentloaded' });
    await buscar(page, q);

    const popover = page.locator('#sytue-search-results');
    await expect(popover).toBeVisible({ timeout: 20_000 });
    // Durante el tecleo el popover puede mostrar el estado vacío de una consulta
    // intermedia; el prefijo único aparece en todas, así que casarlo evita esa
    // carrera y sigue probando que buscó NUESTRO término (sin resultados).
    await expect(popover.getByText(new RegExp(`Sin resultados para «${prefijo}`))).toBeVisible({
      timeout: 15_000,
    });
    await expect(page.getByRole('option')).toHaveCount(0);
  });

  // REQUIERE REBUILD del contenedor `hub`: los conectores Drive/Contacts corregidos
  // (services/hub/internal/connectors/{drive,contacts}.go) viven en el binario Go,
  // que no se recompila en caliente. Con el hub recompilado el fan-out devuelve el
  // archivo y el contacto recién creados (comprobado a mano, curl al /search). Al
  // quitar `.fixme` tras el rebuild, este test pasa.
  test(
    'agrupa por producto los resultados creados (Drive + Contacts) y navega al deep-link',
    async ({ page, request }) => {
      const q = termino('zqxsrch');
      const libreta = await libretaContactos(request);
      let contactoHref = '';
      await crearArchivoDrive(request, q);
      try {
        contactoHref = await crearContacto(request, libreta.url, q);

        await page.goto(APPS.one, { waitUntil: 'domcontentloaded' });
        await buscar(page, q);

        const popover = page.locator('#sytue-search-results');
        await expect(popover).toBeVisible({ timeout: 20_000 });

        // Agrupado por producto: cabeceras Drive y Contacts, cada una con su hit.
        await expect(popover.getByText('Drive', { exact: true })).toBeVisible();
        await expect(popover.getByText('Contacts', { exact: true })).toBeVisible();
        const archivo = page.getByRole('option').filter({ hasText: `${q}.txt` });
        const contacto = page.getByRole('option').filter({ hasText: `${q} Persona` });
        await expect(archivo, 'el archivo de Drive aparece en resultados').toHaveCount(1);
        await expect(contacto, 'el contacto aparece en resultados').toHaveCount(1);

        // Deep-link cross-app al recurso de Drive.
        await expect(archivo.locator('a')).toHaveAttribute(
          'href',
          /https:\/\/drive\.sytue\.localhost\/files\//,
        );

        // Teclado: flecha abajo enfoca el primer resultado, Enter navega a su deep-link.
        await page.getByRole('searchbox', { name: 'Búsqueda global' }).focus();
        await page.keyboard.press('ArrowDown');
        await page.keyboard.press('Enter');
        await page.waitForURL(/drive\.sytue\.localhost\/files\//, { timeout: 20_000 });
      } finally {
        await borrarArchivoDrive(request, q);
        if (contactoHref) await borrarContacto(request, contactoHref);
      }
    },
  );
});
