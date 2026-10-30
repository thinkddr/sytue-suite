// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Completa el login desktop de ventana-real.sh y devuelve su callback por IPC.
// La URL de vuelta contiene un código de un solo uso: no se visita ni se registra.
import { chromium } from 'playwright';
import { activarOrganizacion, codigoTOTP } from './tests/helpers.ts';
import {
  autorizarDesktop,
  codigoPublico,
  validarAutorizacion,
} from './ventana-real-login-guard.mjs';

let browser;
try {
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
    });
    process.stdout.write(`${callback}\n`);
  }
} catch (error) {
  console.error(`No se completó el acceso nativo (${codigoPublico(error)}).`);
  process.exitCode = 1;
} finally {
  await browser?.close().catch(() => {});
}
