// Contrato de seguridad de Sytue Admin (admin.sytue.localhost):
// un usuario con sesión válida pero SIN rol de consola (admin/owner) recibe
// 403 «No autorizado para este recurso.» y NO se le filtra contenido de
// administración (guard en packages/auth/src/server/guard.ts).
//
// Fixture del usuario member: el seed de dev provisiona un segundo usuario SIN
// rol de consola —member@sytue.localhost, rol `member`, SIN MFA, misma
// contraseña de dev— en services/id/cmd/id/seed.go (idempotente). Con él
// ejercemos el contrato de denegación de principio a fin: el member ENTRA en la
// suite (login válido sin segundo factor, porque `member` no exige MFA) pero al
// ir a Console recibe 403. El contrato inverso (admin CON step-up MFA ve el
// panel) vive en console-admin.spec.ts.
//
// El login del member se hace UNA sola vez en beforeAll y se persiste su
// storageState propio (.auth/member.json); los tests solo cargan esa sesión y
// navegan. Así el login (parte lenta/frágil) queda fuera del reintento de cada
// test: un retry re-ejecuta solo la navegación a Console, no el login. No es
// serial: cada test parte de la sesión guardada de forma independiente.
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';
import { APPS, irAlFormularioDelIdP, loginEnIdP, sidebarNav } from './helpers';

const MEMBER_EMAIL = process.env.SYTUE_E2E_MEMBER_EMAIL || 'member@sytue.localhost';
const MEMBER_PASS = process.env.SYTUE_E2E_MEMBER_PASS || 'sytue-dev-admin';

/** storageState propio de la sesión member (lo escribe el beforeAll de abajo). */
const MEMBER_STATE = fileURLToPath(new URL('../.auth/member.json', import.meta.url));

/** Marcadores de la UI de administración que NUNCA deben aparecer en el 403. */
const MARCADORES_ADMIN = [
  /Sytue Admin/i,
  /Administraci/i, // «Administracion»/«Administración» (sidebar del shell)
  /Organizaciones/i,
  /Auditor/i, // «Auditoria»/«Auditoría»
  /admin@sytue\.localhost/i, // datos del panel de usuarios (PII de identidad)
];

test.describe('console: member sin rol de consola', () => {
  // Sesión del member creada una vez y reutilizada por todos los tests del bloque.
  test.use({ storageState: MEMBER_STATE, navigationTimeout: 90_000 });

  test.beforeAll(async ({ browser }) => {
    // Cadenas OIDC sobre dev-servers fríos + reintentos del lockout del IdP:
    // presupuesto amplio. Contexto manual para HACER el login desde cero:
    //  - ignoreHTTPSErrors explícito (cert autofirmado de la pila dev).
    //  - storageState: undefined IMPRESCINDIBLE: browser.newContext hereda las
    //    opciones del test (incluido el storageState: MEMBER_STATE del use de
    //    arriba); sin anularlo intentaría LEER member.json —que aún no existe—
    //    y fallaría con ENOENT (huevo-y-gallina). Aquí lo ESCRIBIMOS, no leemos.
    test.setTimeout(180_000);
    const ctx = await browser.newContext({ ignoreHTTPSErrors: true, storageState: undefined });
    const page = await ctx.newPage();
    try {
      await irAlFormularioDelIdP(page);
      // `member` no exige MFA: loginEnIdP no intercala segundo factor y aterriza
      // autenticado en el portal. Prueba que el member ES un usuario válido.
      await loginEnIdP(page, MEMBER_EMAIL, MEMBER_PASS);
      await page.waitForURL((url) => url.hostname === 'one.sytue.localhost', {
        waitUntil: 'domcontentloaded',
      });
      await ctx.storageState({ path: MEMBER_STATE });
    } finally {
      await ctx.close();
    }
  });

  // La RAÍZ ya no responde 403 desde la admin delegada: a quien no tiene ninguna
  // sección se le lleva a «Mi cuenta» (firstSectionPath) en vez de plantarle un
  // error. Lo que sigue siendo contrato es que las rutas de administración
  // denieguen y que nada de administración se filtre por el camino.
  test('console /: member sin rol de consola aterriza en su cuenta, sin administración', async ({
    page,
  }) => {
    test.slow();
    const resp = await page.goto(new URL('/', APPS.console).href, {
      waitUntil: 'domcontentloaded',
    });
    expect(resp, 'la navegación debe producir una respuesta').not.toBeNull();
    expect(new URL(page.url()).hostname).toBe('admin.sytue.localhost');
    expect(new URL(page.url()).pathname, 'sin secciones administrables → Mi cuenta').toBe(
      '/account',
    );

    // Aquí el member SÍ está dentro de Console (es su cuenta), así que la marca
    // del producto, el <meta description> y el rótulo «Administración» del menú
    // aparecen por fuerza. Lo que no puede aparecer son DATOS de administración
    // (en el contenido) ni ENLACES a secciones que su permiso no abre.
    const contenido = (await page.locator('main#contenido').innerText()).toLowerCase();
    for (const dato of ['organizaciones', 'auditor', 'admin@sytue.localhost', 'invitar']) {
      expect(contenido, `no debe filtrarse contenido admin («${dato}»)`).not.toContain(dato);
    }
    const nav = sidebarNav(page);
    for (const seccion of ['Usuarios', 'Grupos', 'Auditoría', 'Dispositivos', 'Aplicaciones']) {
      await expect(
        nav.getByRole('link', { name: seccion, exact: true }),
        `el menú no debe ofrecer «${seccion}» a quien no tiene ese permiso`,
      ).toHaveCount(0);
    }
  });

  for (const ruta of ['/users', '/audit'] as const) {
    test(`console ${ruta}: member sin rol de consola recibe 403 sin filtrar administración`, async ({
      page,
    }) => {
      // La primera compilación de una ruta en el dev-server Astro puede superar
      // los 45 s del timeout base; slow() lo triplica para absorber el arranque frío.
      test.slow();
      // Con sesión SSO válida (member), Console autoriza en el IdP y crea sesión,
      // pero el guard de rol (admin/owner) deniega: el status final es 403.
      const resp = await page.goto(new URL(ruta, APPS.console).href, {
        waitUntil: 'domcontentloaded',
      });

      // El contrato: 403 explícito con el mensaje del guard, sin acabar en login.
      expect(resp, 'la navegación debe producir una respuesta').not.toBeNull();
      expect(resp!.status(), 'status HTTP de la respuesta final').toBe(403);
      expect(new URL(page.url()).hostname).toBe('admin.sytue.localhost');
      await expect(page.locator('body')).toContainText('No autorizado para este recurso.');

      // Y que NO se filtra nada del panel de administración en la respuesta.
      const html = await page.content();
      for (const marcador of MARCADORES_ADMIN) {
        expect(html, `no debe filtrarse contenido admin (${marcador})`).not.toMatch(marcador);
      }
    });
  }
});
