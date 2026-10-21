// Spec e2e de Sytue Drive — API del BFF (https://drive.sytue.localhost/api/*).
// Cubre: listar la raíz, crear carpeta (y verla al re-listar) y subir/descargar
// un txt. Usa el storageState de admin del proyecto (cookie de sesión httpOnly
// de la suite, dominio .sytue.localhost), así que el fixture `request` ya va
// autenticado. Nombres únicos por test para que sean independientes y
// re-ejecutables (también en paralelo); limpieza best-effort al final de cada uno.
import { expect, test, type APIRequestContext } from '@playwright/test';
import { APPS } from './helpers';

const API = APPS.drive; // https://drive.sytue.localhost/
const ORIGIN = API.replace(/\/$/, '');

/**
 * Sufijo único por test. Date.now() a secas puede colisionar cuando dos
 * workers ejecutan el mismo test a la vez (p. ej. --repeat-each con workers>1,
 * que reparte cada repetición a un worker distinto): mismo milisegundo ⇒
 * misma ruta ⇒ conflicto espurio al crear. El tramo aleatorio lo evita.
 */
const unico = () => `${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;

/**
 * Limpieza best-effort (papelera). DELETE con content-type octet-stream (no «de
 * formulario»), el mismo patrón que la subida, para no disparar la protección
 * CSRF de Astro tras el proxy (ver el test de borrado al final).
 */
async function borrar(request: APIRequestContext, ruta: string): Promise<void> {
  await request.delete(`${API}api/files?path=${ruta}`, {
    headers: { origin: ORIGIN, 'content-type': 'application/octet-stream' },
  });
}

test.describe('Drive (BFF /api)', () => {
  // El dev server (Vite sobre bind-mount Docker) compila rutas bajo demanda y
  // en frío puede tardar >45 s (observados PUT de 25-55 s y algún timeout del
  // module-runner a los 60 s). Margen holgado para no falsear fallos de infra.
  test.describe.configure({ timeout: 120_000 });

  test('lista la raíz: GET /api/files?path=/ responde 200 con items', async ({ request }) => {
    const res = await request.get(`${API}api/files?path=/`);
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body.items)).toBe(true);
  });

  test('crea una carpeta única y aparece al re-listar', async ({ request }) => {
    const nombre = `e2e-carpeta-${unico()}`;

    const creada = await request.post(`${API}api/folders`, { data: { path: `/${nombre}` } });
    expect(creada.status()).toBe(201);
    const { resource } = await creada.json();
    expect(resource).toMatchObject({ name: nombre, isDirectory: true });

    const listado = await request.get(`${API}api/files?path=/`);
    expect(listado.status()).toBe(200);
    const { items } = await listado.json();
    expect(items).toEqual(
      expect.arrayContaining([expect.objectContaining({ name: nombre, isDirectory: true })]),
    );

    await borrar(request, `/${nombre}`);
  });

  test('sube un txt y descargarlo devuelve el contenido', async ({ request }) => {
    const nombre = `e2e-nota-${unico()}.txt`;
    const ruta = encodeURIComponent(`/${nombre}`);
    const contenido = `Hola desde el e2e de Drive — ${nombre}\nCon tildes: áéíóúñ.\n`;

    // El cuerpo viaja como octet-stream y el tipo real en ?type= (evita la
    // protección CSRF de Astro con content-types «de formulario»).
    const subida = await request.put(`${API}api/content?path=${ruta}&type=text%2Fplain`, {
      headers: { 'content-type': 'application/octet-stream' },
      data: contenido,
    });
    expect(subida.status()).toBe(201);
    const { resource } = await subida.json();
    expect(resource).toMatchObject({ name: nombre, isDirectory: false });

    const descarga = await request.get(`${API}api/content?path=${ruta}`);
    expect(descarga.status()).toBe(200);
    expect(descarga.headers()['content-type']).toContain('text/plain');
    expect(await descarga.text()).toBe(contenido);

    await borrar(request, ruta);
  });

  // La papelera funciona con el Origin real del navegador (https://) porque el
  // DELETE viaja con content-type octet-stream —no «de formulario»—, el mismo
  // patrón que la subida. Sin ese header, el checkOrigin de Astro compara el
  // Origin del navegador (https://drive.sytue.localhost) con context.url.origin,
  // que tras Traefik (sin X-Forwarded-Proto) es http://drive.sytue.localhost, no
  // coinciden y responde 403 «Cross-site DELETE form submissions are forbidden».
  // Con un content-type no-form la comprobación se salta, igual que en el PUT.
  test('borra a la papelera: DELETE /api/files responde 200 con el Origin real', async ({ request }) => {
    const nombre = `e2e-borrar-${unico()}.txt`;
    const ruta = encodeURIComponent(`/${nombre}`);
    await request.put(`${API}api/content?path=${ruta}&type=text%2Fplain`, {
      headers: { 'content-type': 'application/octet-stream' },
      data: 'contenido temporal',
    });
    // Exactamente lo que envía la UI real: Origin https:// + content-type octet-stream.
    const del = await request.delete(`${API}api/files?path=${ruta}`, {
      headers: { origin: ORIGIN, 'content-type': 'application/octet-stream' },
    });
    expect(del.status()).toBe(200);
  });
});
