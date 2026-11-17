// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Completa los inicios de sesión que abre ventana-real.sh y devuelve su vuelta por IPC.
// La URL de vuelta contiene un código de un solo uso y no se registra en ningún sitio. Si
// llega como redirección, el navegador la sigue y se la lleva él a la app (Playwright no
// enruta los saltos de una redirección); si la abre el guion del relevo, la ruta la recoge
// y la entrega ventana-real.sh. En los dos casos llega a la app una vez.
//
//   bun ventana-real-login.mjs <url de /oauth2/authorize>   el inicio de sesión nativo
//   bun ventana-real-login.mjs --matrix <url del SSO>      el alta del dispositivo de Matrix
//
// El primero teclea las credenciales de la cuenta de auditoría en Sytue ID, que deriva
// en el navegador la clave del cifrado (ADR 0084), y espera al relevo, que la devuelve
// cifrada en la vuelta. Con VENTANA_NAVEGADOR=<fichero> deja ahí la sesión del navegador
// (cookies de Sytue ID) para el segundo, que la reutiliza sin teclear nada: es el SSO que
// abre una app al configurar su dispositivo de Matrix. Ese fichero vive en el directorio
// de usar y tirar de la tirada y se borra con él.
//
// Con VENTANA_DIAGNOSTICO=<carpeta>, si algo se para DESPUÉS de las credenciales, deja
// allí la captura de la página donde se quedó (`login-<código>.png`).
import { chmod } from 'node:fs/promises';
import { chromium } from 'playwright';
import { activarOrganizacion, codigoTOTP } from './tests/helpers.ts';
import {
  autorizarDesktop,
  autorizarMatrix,
  codigoPublico,
  traeDesbloqueo,
  validarAutorizacion,
  validarSSOMatrix,
} from './ventana-real-login-guard.mjs';

const navegador = process.env.VENTANA_NAVEGADOR;
const diagnostico = process.env.VENTANA_DIAGNOSTICO;
// Solo dónde se quedó, sin consulta: la consulta lleva el state y, en la vuelta, el código.
const alFallar = async (page, code) => {
  try {
    const aqui = new URL(page.url());
    console.error(`login: se quedó en ${aqui.origin}${aqui.pathname}`);
  } catch {}
  if (diagnostico) await page.screenshot({ path: `${diagnostico}/login-${code}.png` });
};

let browser;
try {
  const matrix = process.argv[2] === '--matrix';
  if (matrix) {
    const sso = validarSSOMatrix(process.argv[3]).href;
    if (!navegador) throw new Error('VENTANA_NAVEGADOR');
    browser = await chromium.launch();
    const context = await browser.newContext({ locale: 'es-ES', storageState: navegador });
    const vuelta = await autorizarMatrix(context, sso, {
      alFallar,
      semilla: process.env.SYTUE_AUDIT_TOTP,
      codigoTOTP,
    });
    process.stdout.write(`${vuelta}\n`);
  } else {
    const destination = validarAutorizacion(process.argv[2]);
    const email = process.env.SYTUE_AUDIT_EMAIL;
    const clave = process.env.SYTUE_AUDIT_PASS;
    if (!email || !clave) {
      console.error(
        'uso: SYTUE_AUDIT_EMAIL=… SYTUE_AUDIT_PASS=… [SYTUE_AUDIT_TOTP=…] bun ventana-real-login.mjs <url>',
      );
      process.exitCode = 2;
    } else {
      browser = await chromium.launch();
      const context = await browser.newContext({ locale: 'es-ES' });
      const callback = await autorizarDesktop(context, destination, {
        email,
        clave,
        semilla: process.env.SYTUE_AUDIT_TOTP,
        organizacion: process.env.SYTUE_AUDIT_ORG,
        activarOrganizacion,
        codigoTOTP,
        alFallar,
      });
      // Si la vuelta no trae la clave, la app entra igual pero el cifrado se queda
      // cerrado: dicho aquí, que es donde se sabe por qué.
      console.error(
        traeDesbloqueo(callback)
          ? 'login: la vuelta trae la clave de desbloqueo cifrada para la app'
          : 'login: la vuelta NO trae la clave de desbloqueo (el relevo no la entregó)',
      );
      if (navegador) {
        await context.storageState({ path: navegador });
        await chmod(navegador, 0o600);
      }
      process.stdout.write(`${callback}\n`);
    }
  }
} catch (error) {
  console.error(`No se completó el acceso nativo (${codigoPublico(error)}).`);
  process.exitCode = 1;
} finally {
  await browser?.close().catch(() => {});
}
