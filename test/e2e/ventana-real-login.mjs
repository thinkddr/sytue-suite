// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// ventana-real-login.mjs <url de autorización> — completa en un Chromium sin pantalla el
// login que una app nativa abre en el navegador, y escribe en la salida la URL de vuelta
// (`http://127.0.0.1:8921/auth/callback?code=…&state=…`) SIN visitarla.
//
// Es la otra mitad de `ventana-real.sh`: la app corre en la máquina con pantalla y la
// contraseña se queda en ÉSTA (`~/.config/sytue/audit.env` del piloto). Lo único que
// cruza es la URL de vuelta, y su código solo lo puede canjear la app que guarda el
// verificador PKCE, una vez y durante un minuto.
//
//   set -a; . ~/.config/sytue/audit.env; set +a
//   bun ~/sytue-one/test/e2e/ventana-real-login.mjs "$URL"
//
// Vive en test/e2e —y no junto a ventana-real.sh— para resolver `playwright` y el
// TOTP del arnés sin copiarlo a otra carpeta (ESM no mira NODE_PATH). Con Bun, como
// el resto de sondas: el Node del piloto es el de Debian, sin soporte de TypeScript
// (ERR_NO_TYPESCRIPT al importar `helpers.ts`).
//
// SEGUNDO FACTOR: si la cuenta lo pide, el código sale de `SYTUE_AUDIT_TOTP` (la
// semilla, no un código) en el momento de enviarlo. Sin esa variable, una cuenta con
// segundo factor se queda en ese paso y el guion lo dice. Ni la semilla ni el código
// se escriben en ninguna salida.
import { chromium } from "playwright";
import { activarOrganizacion, codigoTOTP } from "./tests/helpers.ts";

const destino = process.argv[2];
const { SYTUE_AUDIT_EMAIL: email, SYTUE_AUDIT_PASS: clave, SYTUE_AUDIT_TOTP: semilla } = process.env;
if (!destino || !email || !clave) {
  console.error("uso: SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… [SYTUE_AUDIT_TOTP=…] node ventana-real-login.mjs <url>");
  process.exit(2);
}

const navegador = await chromium.launch();
try {
  const ctx = await navegador.newContext({ locale: "es-ES" });
  const p = await ctx.newPage();
  // UNA sola autorización con ese `code_challenge`, y su vuelta NO se visita (el servidor
  // efímero de la app está en otra máquina). Chromium no deja ver la 302 que lleva a
  // 127.0.0.1 —aborta la navegación antes—, así que el POST del login se hace por fuera
  // del navegador, sin seguir su redirección, y la autorización se pide una vez con la
  // cookie que deja: comparten almacén.
  //
  // Con segundo factor son DOS envíos: `/login/` contesta 302 a `/login/?step_up=1`
  // (y deja la cookie de continuación), y es `/login/mfa` quien abre la sesión. Los
  // dos se interceptan igual; el paso intermedio sí se visita, para rellenarlo.
  let entrado;
  const envio = () => new Promise((listo) => (entrado = listo));
  let hecho = envio();
  await p.route(/\/login\/(mfa)?$/, async (r) => {
    if (r.request().method() !== "POST") return r.continue();
    const res = await r.fetch({ maxRedirects: 0 });
    entrado({ estado: res.status(), a: res.headers().location ?? "" });
    return r.fulfill({ status: 204 });
  });
  await p.goto(destino, { waitUntil: "domcontentloaded" });
  const formulario = p.waitForSelector("input[name=password]", { timeout: 30_000 }).then(async () => {
    await p.fill("input[name=email]", email);
    await p.fill("input[name=password]", clave);
    // El ALTCHA del login (`services/id/internal/http/signup.go`) se resuelve solo al
    // enviar: el formulario hace la prueba de trabajo en el propio Chromium y se envía
    // él al terminar. Marcar antes su casilla fallaba una vez de cada pocas.
    await p.click("button[type=submit]");
  });
  const url = await Promise.race([
    formulario.then(() => hecho).then(async ({ estado, a }) => {
      if (estado === 302 && a.includes("step_up=1")) {
        if (!semilla) throw new Error("la cuenta pide segundo factor y audit.env no trae SYTUE_AUDIT_TOTP");
        hecho = envio();
        await p.goto(new URL(a, destino).href, { waitUntil: "domcontentloaded" });
        await p.fill("input[name=code]", codigoTOTP(semilla));
        await p.click('form[action="/login/mfa"] button[type=submit]');
        ({ estado } = await hecho);
      }
      if (estado !== 302) throw new Error(`el login contesta ${estado}`);
      // Con SYTUE_AUDIT_ORG, la organización se cambia ANTES de autorizar: el login
      // abre la sesión en la primera de la cuenta. El cambio se hace desde el portal
      // (su BFF), que es lo que tiene un token con el que pedirlo.
      if (process.env.SYTUE_AUDIT_ORG) {
        const dominio = new URL(destino).hostname.replace(/^id\./, "");
        await p.goto(`https://one.${dominio}/one/`, { waitUntil: "domcontentloaded" });
        await p.waitForURL((u) => u.hostname !== new URL(destino).hostname, { timeout: 60_000 });
        await activarOrganizacion(p);
      }
      const res = await ctx.request.get(destino, { maxRedirects: 0 });
      const l = res.headers().location ?? "";
      if (!l.startsWith("http://127.0.0.1:8921/")) throw new Error(`la autorización no vuelve: ${res.status()}`);
      return l;
    }),
    // `unref`: el código de la vuelta caduca a los 60 s (AUTH_CODE_TTL) y un reloj vivo
    // retenía el proceso los 120 enteros después de escribirla, así que llegaba muerto.
    new Promise((_, no) => setTimeout(() => no(new Error("sin vuelta")), 120_000).unref()),
  ]).catch(async (e) => {
    // Lo que dice la página (sin la contraseña, que va en un campo y no en el texto).
    const texto = await p.evaluate(() => document.body.innerText).catch(() => "");
    throw new Error(`${e.message} en ${new URL(p.url()).pathname}: ${texto.replace(/\s+/g, " ").slice(0, 300)}`);
  });
  process.stdout.write(`${url}\n`);
} finally {
  await navegador.close();
}
