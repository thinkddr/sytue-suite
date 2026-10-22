// e2e de Grupos/Etiquetas de Sytue Contacts.
// API: el BFF proxya la API REST del servicio Go (resolución slug CardDAV ->
// UUID de libreta) — crear/listar etiquetas, asignar/quitar miembros por UID y
// eliminar (mutaciones por POST {delete:true}).
// UI: sección «Etiquetas» de la sidebar con contador, filtro de la lista por
// etiqueta y chips de asignación en la ficha del contacto.
import { randomUUID } from 'node:crypto';
import { expect, test, type APIRequestContext } from '@playwright/test';
import { APPS, sidebarNav } from './helpers';

const api = (path: string) => `${APPS.contacts}api/${path}`;

/** Sufijo único por test: Date.now() a solas puede colisionar entre workers. */
const uniq = () => `${Date.now()}-${randomUUID().slice(0, 8)}`;

// Formas mínimas de las respuestas del BFF.
interface AddressBook {
  url: string;
  displayName: string;
}
interface Group {
  id: string;
  uid: string;
  name: string;
  members: string[];
}

/** Devuelve la libreta por defecto «Contactos» (la crea el servicio Go al vuelo). */
async function getLibretaContactos(request: APIRequestContext): Promise<AddressBook> {
  const res = await request.get(api('addressbooks'));
  expect(res.status(), 'GET /api/addressbooks debe responder 200').toBe(200);
  const { addressBooks } = (await res.json()) as { addressBooks: AddressBook[] };
  const libreta = addressBooks.find((b) => b.displayName === 'Contactos');
  expect(libreta, 'debe existir la libreta «Contactos»').toBeTruthy();
  return libreta!;
}

/** Semilla: crea un contacto por API y devuelve su href/uid. */
async function crearContacto(
  request: APIRequestContext,
  libretaUrl: string,
  fullName: string,
): Promise<{ href: string; uid: string }> {
  const res = await request.post(api('contact'), {
    data: { addressBookUrl: libretaUrl, contactInput: { fullName, emails: [], phones: [] } },
  });
  expect(res.status(), 'POST /api/contact (semilla) debe responder 200').toBe(200);
  return (await res.json()) as { href: string; uid: string };
}

/** Lista los grupos de la libreta vía BFF. */
async function listarGrupos(request: APIRequestContext, libretaUrl: string): Promise<Group[]> {
  const res = await request.get(api(`groups?addressBookUrl=${encodeURIComponent(libretaUrl)}`));
  expect(res.status(), 'GET /api/groups debe responder 200').toBe(200);
  const { groups } = (await res.json()) as { groups: Group[] };
  return groups;
}

/** Limpieza best-effort de una etiqueta y un contacto de esta ejecución. */
async function limpiar(
  request: APIRequestContext,
  opts: { groupId?: string | undefined; contactHref?: string | undefined },
): Promise<void> {
  if (opts.groupId) {
    await request.post(api('group'), { data: { id: opts.groupId, delete: true } }).catch(() => {});
  }
  if (opts.contactHref) {
    await request.delete(api('contact'), { data: { href: opts.contactHref } }).catch(() => {});
  }
}

test.describe('Contacts groups (etiquetas) — API', () => {
  test('GET /api/groups responde 200 con la lista de grupos', async ({ request }) => {
    const libreta = await getLibretaContactos(request);
    const groups = await listarGrupos(request, libreta.url);
    expect(Array.isArray(groups)).toBe(true);
  });

  test('crear etiqueta, asignar/quitar miembro por UID y eliminarla', async ({ request }) => {
    const libreta = await getLibretaContactos(request);
    const id = uniq();
    const nombre = `E2E Etiqueta ${id}`;
    let groupId: string | undefined;
    let contactHref: string | undefined;

    try {
      // Alta de la etiqueta (POST /api/groups -> POST address-books/{uuid}/groups).
      const alta = await request.post(api('groups'), {
        data: { addressBookUrl: libreta.url, name: nombre },
      });
      expect(alta.status(), 'POST /api/groups debe responder 201').toBe(201);
      const grupo = (await alta.json()) as Group;
      groupId = grupo.id;
      expect(grupo.id).toBeTruthy();
      expect(grupo.name).toBe(nombre);
      expect(grupo.members).toEqual([]);

      // Aparece en el listado de la libreta.
      expect((await listarGrupos(request, libreta.url)).map((g) => g.name)).toContain(nombre);

      // Asignar un contacto como miembro (PUT reemplaza la pertenencia).
      const contacto = await crearContacto(request, libreta.url, `E2E Miembro ${id}`);
      contactHref = contacto.href;
      const asignar = await request.post(api('group'), {
        data: { id: grupo.id, name: nombre, members: [contacto.uid] },
      });
      expect(asignar.status(), 'POST /api/group (asignar) debe responder 200').toBe(200);
      expect(((await asignar.json()) as Group).members).toContain(contacto.uid);

      // Quitar el miembro (lista vacía).
      const quitar = await request.post(api('group'), {
        data: { id: grupo.id, name: nombre, members: [] },
      });
      expect(quitar.status(), 'POST /api/group (quitar) debe responder 200').toBe(200);
      expect(((await quitar.json()) as Group).members).toEqual([]);

      // Eliminar la etiqueta (POST {delete:true} -> DELETE groups/{id}).
      const borrar = await request.post(api('group'), { data: { id: grupo.id, delete: true } });
      expect(borrar.status(), 'POST /api/group (delete) debe responder 200').toBe(200);
      expect(((await borrar.json()) as { ok: boolean }).ok).toBe(true);
      groupId = undefined;

      // Ya no está en el listado.
      expect((await listarGrupos(request, libreta.url)).map((g) => g.name)).not.toContain(nombre);
    } finally {
      await limpiar(request, { groupId, contactHref });
    }
  });
});

test.describe('Contacts groups (etiquetas) — UI', () => {
  // Compilación en frío del island (Vite sobre bind-mount) + CardDAV: margen amplio.
  test.slow();

  test('sidebar con contador, filtro por etiqueta y chips en la ficha', async ({ page, request }) => {
    const run = randomUUID().slice(0, 8);
    const nombre = `E2E Tag ${run}`;
    const contactoNombre = `E2E Etiquetado ${run}`;
    const libreta = await getLibretaContactos(request);
    let groupId: string | undefined;
    let contactHref: string | undefined;

    try {
      // Semilla: contacto por API (el alta UI ya la cubre contacts.spec.ts).
      const contacto = await crearContacto(request, libreta.url, contactoNombre);
      contactHref = contacto.href;

      await page.goto(APPS.contacts, { waitUntil: 'domcontentloaded' });
      const nav = sidebarNav(page);
      // El nombre del contacto aparece en la lista Y en la ficha al seleccionarlo:
      // acotar a la lista evita la violación de strict mode. Es role=list (no
      // listbox) desde que la lista se virtualiza: cada fila es un botón que
      // abre la ficha, que es lo que hace, y no una opción de un selector.
      const lista = page.getByRole('list', { name: 'Contactos' });

      // Crear la etiqueta desde la sidebar (modal «Nueva etiqueta»). La sección
      // aparece cuando cargan las libretas; margen amplio para el island frío.
      const nuevaEtiqueta = nav.getByTitle('Nueva etiqueta');
      await expect(nuevaEtiqueta).toBeVisible({ timeout: 30_000 });
      await nuevaEtiqueta.click();
      await page.getByLabel('Nombre').fill(nombre);
      await page.getByRole('button', { name: 'Crear' }).click();
      await expect(nav.getByTitle(nombre), 'la etiqueta debe aparecer en la sidebar').toBeVisible({
        timeout: 15_000,
      });

      // Guardar el id para la limpieza.
      groupId = (await listarGrupos(request, libreta.url)).find((g) => g.name === nombre)?.id;

      // Abrir la ficha del contacto semilla.
      const buscar = page.getByRole('searchbox', { name: 'Buscar contactos' });
      await buscar.fill(run);
      await lista.getByText(contactoNombre).click();
      // Limpiar la búsqueda: la selección persiste y así el estado vacío de más
      // abajo corresponde al filtro de etiqueta (no a la búsqueda).
      await buscar.fill('');

      // Asignar la etiqueta desde la ficha (menú «Añadir» -> chip). `exact` porque
      // sin él la coincidencia es por subcadena y casa también con «Añadir foto
      // al contacto», que está en la misma ficha.
      await page.getByRole('button', { name: 'Añadir', exact: true }).click();
      await page.getByRole('menuitem', { name: nombre }).click();
      const quitarChip = page.getByRole('button', { name: `Quitar etiqueta ${nombre}` });
      await expect(quitarChip, 'el chip de la etiqueta debe aparecer en la ficha').toBeVisible();
      await page.keyboard.press('Escape'); // cierra el menú «Añadir»

      // El contador de la sidebar refleja el miembro. Se ancla al span del
      // Badge con texto exacto «1»: el propio nombre (token hex) podría
      // contener un 1 y daría un falso positivo con toContainText.
      await expect(
        nav.getByTitle(nombre).locator('span').filter({ hasText: /^1$/ }),
        'el contador de la etiqueta debe mostrar 1 miembro',
      ).toBeVisible();

      // Filtrar la lista por la etiqueta: el contacto sigue visible.
      await nav.getByTitle(nombre).click();
      await expect(lista.getByText(contactoNombre)).toBeVisible();

      // Quitar la etiqueta desde el chip: con el filtro activo la lista queda vacía.
      await quitarChip.click();
      await expect(quitarChip).toHaveCount(0);
      await expect(page.getByText('Sin contactos', { exact: true })).toBeVisible();

      // Quitar el filtro desde el estado vacío restaura la lista.
      await page.getByRole('button', { name: 'Quitar filtro' }).click();
      await expect(lista.getByText(contactoNombre)).toBeVisible();
    } finally {
      await limpiar(request, { groupId, contactHref });
    }
  });
});
