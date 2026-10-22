// Spec e2e de Sytue Drive — enlace de compartición PÚBLICO (descarga sin sesión).
//
// Flujo real:
//   1. Sube un fichero por el BFF autenticado (cookie de sesión httpOnly, admin).
//   2. Crea un enlace público: POST /api/shares → { share: { url, ... } }. El BFF
//      llama a DriveClient.createShare (kind:link) sobre el servicio Go; la url es
//      https://drive-dav.sytue.localhost/public/{token} — el TOKEN va en la RUTA.
//      El host -dav es un alias de Traefik al servicio Go (puerto 8082); el host de
//      la UI (drive.sytue.localhost) NO sirve /public/{token} (es el Astro, 404).
//   3. Descarga por ese enlace en un contexto SIN sesión (storageState vacío, sin
//      cookies ni Bearer) y verifica el contenido byte a byte. Contrato de la
//      descarga: 200, Content-Type = el del fichero, Content-Disposition attachment.
//
// ESTADO: el segundo test está en test.fixme — REQUIERE REBUILD del contenedor
// `drive`. La resolución del token estaba rota: `shares` tiene FORCE ROW LEVEL
// SECURITY y el rol de la app no tiene BYPASSRLS, así que la función SECURITY
// DEFINER drive_resolve_share_tenant devolvía NULL para CUALQUIER token (incluso
// válido) → GET /public/{token} respondía 500. Lo arregla la migración
// services/drive/db/migrations/0003_fix_link_resolver.up.sql (SETOF text + salto de
// RLS acotado por GUC), embebida en el binario: se aplica al reconstruir `drive`.
// Tras el rebuild, quitar el `.fixme` y el test pasa.
import { expect, test, request as apiRequest, type APIRequestContext } from '@playwright/test';
import { APPS } from './helpers';

const BFF = APPS.drive; // https://drive.sytue.localhost/
const ORIGIN = BFF.replace(/\/$/, '');

/** Sufijo único por test (evita colisiones entre workers/repeticiones). */
const unico = () => `${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;

/** Limpieza best-effort (papelera). Mismo patrón CSRF-safe que la subida. */
async function borrar(request: APIRequestContext, rutaEnc: string): Promise<void> {
  await request
    .delete(`${BFF}api/files?path=${rutaEnc}`, {
      headers: { origin: ORIGIN, 'content-type': 'application/octet-stream' },
    })
    .catch(() => {});
}

/** Sube un .txt por el BFF y crea un enlace público; devuelve la url pública. */
async function subirYCompartir(
  request: APIRequestContext,
  nombre: string,
  contenido: string,
): Promise<{ url: string; rutaEnc: string }> {
  const rutaEnc = encodeURIComponent(`/${nombre}`);
  // Cuerpo octet-stream + ?type= para esquivar la protección CSRF de Astro con
  // content-types «de formulario».
  const subida = await request.put(`${BFF}api/content?path=${rutaEnc}&type=text%2Fplain`, {
    headers: { 'content-type': 'application/octet-stream' },
    data: contenido,
  });
  expect(subida.status()).toBe(201);

  const compartir = await request.post(`${BFF}api/shares`, {
    data: { path: `/${nombre}`, permission: 'view' },
  });
  expect(compartir.status()).toBe(201);
  const { share } = (await compartir.json()) as { share: { url: string } };
  return { url: share.url, rutaEnc };
}

test.describe('Drive — enlace público de compartición', () => {
  // El dev-server (Vite sobre bind-mount Docker) compila rutas en frío; margen holgado.
  test.describe.configure({ timeout: 120_000 });

  // Pasa YA: valida el contrato BFF → DriveClient → servicio Go de creación del
  // enlace, y que la url apunta al host público resoluble (-dav, alias Traefik).
  test('crea el enlace público con el token en la url del host resoluble', async ({ request }) => {
    const nombre = `e2e-share-${unico()}.txt`;
    const { url, rutaEnc } = await subirYCompartir(request, nombre, 'contenido de prueba\n');
    try {
      // El token va en la RUTA, sobre el host -dav (Traefik → servicio Go, /public/).
      expect(url).toMatch(/^https:\/\/drive-dav\.sytue\.localhost\/public\/[^/]+$/);
    } finally {
      // Limpieza garantizada aunque falle la aserción (evita basura al repetir).
      await borrar(request, rutaEnc);
    }
  });

  // REQUIERE REBUILD del contenedor `drive` (migración 0003, ver cabecera). Tras el
  // rebuild, quitar `.fixme`: descarga end-to-end SIN sesión y verifica el contenido.
  test(
    'descarga por el enlace público SIN sesión y coincide el contenido (requiere rebuild drive)',
    async ({ request }) => {
      const nombre = `e2e-share-${unico()}.txt`;
      const contenido = `Público sin sesión — ${nombre}\nCon tildes: áéíóúñ.\n`;
      const { url, rutaEnc } = await subirYCompartir(request, nombre, contenido);

      // Contexto SIN sesión: sin storageState, sin cookies, sin Authorization.
      const anon = await apiRequest.newContext({ ignoreHTTPSErrors: true });
      try {
        const pub = await anon.get(url);
        expect(pub.status()).toBe(200);
        expect(pub.headers()['content-type']).toContain('text/plain');
        expect(pub.headers()['content-disposition'] ?? '').toContain('attachment');
        expect(await pub.text()).toBe(contenido);

        // Un token inexistente/revocado debe dar 404 (no 500).
        const bad = await anon.get('https://drive-dav.sytue.localhost/public/token-inexistente-xyz');
        expect(bad.status()).toBe(404);
      } finally {
        await anon.dispose();
        // Limpieza garantizada aunque falle una aserción.
        await borrar(request, rutaEnc);
      }
    },
  );
});
