/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `salirDelPasoDeAutorizacion`: los barridos no deciden dónde está la página
 * mientras sigue en `/oauth2/authorize`. El de pantallas lo hacía, y daba «el IdP
 * muestra /mail/corporate/, que no es el formulario de acceso» en la portada de
 * cada app con la app ya en pantalla (2026-11-23 y 24). Ejecutar: `bun run test`
 * desde test/e2e.
 */
import { expect, test } from "bun:test";
import { esPasoDeAutorizacion, salirDelPasoDeAutorizacion } from "./tests/helpers.ts";

const AUTORIZAR =
  "https://id.sytue.eu/oauth2/authorize?response_type=code&client_id=sytue-mail";
const APP = "https://mail.sytue.eu/mail/corporate/";

/** Una página de mentira: `pasos` es la URL que enseña tras cada espera. */
function pagina(inicial: string, pasos: string[]) {
  let actual = inicial;
  const hechas = { esperas: 0, recargas: 0, pausas: 0 };
  return {
    hechas,
    url: () => actual,
    waitForURL: async (cumple: (u: URL) => boolean) => {
      hechas.esperas++;
      actual = pasos.shift() ?? actual;
      if (!cumple(new URL(actual))) throw new Error("Timeout 20000ms exceeded.");
      return null;
    },
    waitForTimeout: async () => {
      hechas.pausas++;
    },
    reload: async () => {
      hechas.recargas++;
      return null;
    },
  };
}

test("reconoce el paso con y sin barra final, y no otras pantallas del IdP", () => {
  expect(esPasoDeAutorizacion(new URL(AUTORIZAR))).toBe(true);
  expect(esPasoDeAutorizacion(new URL("https://id.sytue.eu/oauth2/authorize/"))).toBe(true);
  expect(esPasoDeAutorizacion(new URL("https://id.sytue.eu/login"))).toBe(false);
  expect(esPasoDeAutorizacion(new URL("https://id.sytue.eu/account"))).toBe(false);
});

test("fuera del paso no espera nada", async () => {
  const p = pagina(APP, []);
  expect(await salirDelPasoDeAutorizacion(p)).toBe(false);
  expect(p.hechas).toEqual({ esperas: 0, recargas: 0, pausas: 0 });
});

test("en el paso espera a que vuelva solo a la app, sin recargar", async () => {
  const p = pagina(AUTORIZAR, [APP]);
  expect(await salirDelPasoDeAutorizacion(p)).toBe(true);
  expect(p.url()).toBe(APP);
  expect(p.hechas).toEqual({ esperas: 1, recargas: 0, pausas: 0 });
});

test("si el paso no se mueve, lo relanza recargando, como mucho las veces pedidas", async () => {
  const p = pagina(AUTORIZAR, []);
  expect(await salirDelPasoDeAutorizacion(p, { intentos: 3, pausa: () => 0 })).toBe(true);
  expect(p.url()).toBe(AUTORIZAR);
  expect(p.hechas).toEqual({ esperas: 3, recargas: 2, pausas: 2 });
});

test("una recarga que sale del paso termina la espera", async () => {
  const p = pagina(AUTORIZAR, [AUTORIZAR, "https://id.sytue.eu/login?return_to=%2F"]);
  expect(await salirDelPasoDeAutorizacion(p, { pausa: () => 0 })).toBe(true);
  expect(p.url()).toBe("https://id.sytue.eu/login?return_to=%2F");
  expect(p.hechas).toEqual({ esperas: 2, recargas: 1, pausas: 1 });
});
