// Login real contra el IdP (id.sytue.localhost) y guardado del storageState
// que reutilizan los demás proyectos (vía dependencies: ['setup']).
import { expect, test as setup } from "@playwright/test";
import { STORAGE_STATE } from "../playwright.config";
import { APPS, completarMFASiHaceFalta } from "./helpers";

const EMAIL = process.env.SYTUE_E2E_EMAIL || "admin@sytue.localhost";
const PASS = process.env.SYTUE_E2E_PASS || "sytue-dev-admin";

setup("autenticación como admin dev", async ({ page }) => {
  // El callback OIDC del portal en dev tarda a veces >30 s en frío (Vite sobre
  // bind-mount Docker); margen amplio para no falsear fallos de infra.
  setup.slow();
  // El paseo por las once apps de abajo se suma al login: con `slow()` a secas
  // el propio test se quedaba sin tiempo antes de llegar a guardar el estado.
  setup.setTimeout(300_000);

  // El IdP aplica lockout suave por cuenta (token bucket 5/min por email,
  // services/id server.go/login.go) que cuenta también los logins correctos:
  // runs e2e consecutivos agotan la ráfaga y el POST devuelve la página
  // genérica de fallo. Se detecta pronto y se reintenta por encima del refill
  // (1 token/12 s; la denegación no consume tokens).
  await expect(async () => {
    await page.goto("https://one.sytue.localhost/", {
      waitUntil: "domcontentloaded",
    });

    // Sin sesión, el portal redirige al formulario del IdP (…/login/…).
    if (/\/login\//.test(page.url())) {
      await page.fill("input[name=email]", EMAIL);
      await page.fill("input[name=password]", PASS);
      await page.click("button[type=submit]");
      const lockout = await page
        .getByRole("heading", { name: "No se pudo iniciar sesión" })
        .waitFor({ timeout: 3_000 })
        .then(
          () => true,
          () => false,
        );
      if (lockout)
        throw new Error("Lockout por cuenta del IdP: reintento tras el refill");
      // El admin de dev es admin (rol elevado) con MFA: el IdP pide el segundo
      // factor tras las credenciales. Se completa con el TOTP de dev. No-op si
      // la cuenta no tuviera MFA.
      await completarMFASiHaceFalta(page);
    }

    // De vuelta en el portal, autenticados (predicado por hostname: la URL de
    // login lleva one.sytue.localhost en el redirect_uri y engañaría a un regex).
    await page.waitForURL((url) => url.hostname === "one.sytue.localhost", {
      timeout: 90_000,
    });
    expect(page.url()).not.toMatch(/\/login\//);
  }).toPass({ intervals: [13_000], timeout: 120_000 });
  await page
    .waitForLoadState("networkidle", { timeout: 15_000 })
    .catch(() => {});

  // Cada app tiene su propia cookie de sesión, y el BFF la emite en la PRIMERA
  // visita a su subdominio. Sin pasar por aquí, un spec que llame a la API de su
  // app antes de navegar a ella recibe 401 —le faltan las cookies, no la
  // sesión—, y el fallo depende del orden de las líneas del test: `api/create`
  // de Docs devolvía «unauthenticated» solo por crear el documento antes del
  // primer `goto`. Se visitan todas para que el estado guardado sea completo.
  // Timeout corto y a propósito: hay que dejar que la redirección al IdP y su
  // vuelta terminen —de ahí sale la cookie—, pero no más. En
  // un entorno con parte de la suite apagada —lo normal en local, donde no cabe
  // entera en memoria— cada app parada se comía los 30 s por defecto y el setup
  // moría antes de guardar nada.
  for (const url of Object.values(APPS)) {
    await page
      .goto(url, { waitUntil: "domcontentloaded", timeout: 10_000 })
      .catch(() => {});
  }

  await page.context().storageState({ path: STORAGE_STATE });
});
