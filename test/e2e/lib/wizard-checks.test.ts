/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "bun:test";
import { openWizardFixture } from "./wizard-checks.mjs";

const e2eDir = join(import.meta.dir, "..");

test("openWizardFixture espera al selector hidratado aunque main ya exista en SSR y reintenta tras 504 Outdated Optimize Dep", async () => {
  const listeners = new Set<
    (response: { status: () => number; statusText: () => string }) => void
  >();
  const waitedSelectors: Array<{ selector: string; timeout?: number }> = [];
  let gotoCalls = 0;
  let hydrated = false;

  const page = {
    on(
      event: string,
      fn: (response: {
        status: () => number;
        statusText: () => string;
      }) => void,
    ) {
      if (event === "response") listeners.add(fn);
    },
    off(
      event: string,
      fn: (response: {
        status: () => number;
        statusText: () => string;
      }) => void,
    ) {
      if (event === "response") listeners.delete(fn);
    },
    async goto(_url: string) {
      gotoCalls += 1;
      if (gotoCalls === 1) {
        for (const fn of listeners) {
          fn({ status: () => 504, statusText: () => "Outdated Optimize Dep" });
        }
      }
    },
    locator(selector: string) {
      return {
        async waitFor(opts?: { state?: string; timeout?: number }) {
          waitedSelectors.push({ selector, timeout: opts?.timeout });
          if (selector === "main") {
            return;
          }
          if (gotoCalls === 1) {
            throw new Error("Timeout waiting for client island hydration");
          }
          hydrated = true;
        },
      };
    },
  };

  await openWizardFixture(
    page as never,
    "http://127.0.0.1:4343/rooms/prueba/dev/idioma-sistema",
    "astro-island:not([ssr]) select",
  );

  expect(hydrated).toBe(true);
  expect(gotoCalls).toBe(2);
  expect(waitedSelectors).toEqual([
    { selector: "astro-island:not([ssr]) select", timeout: 60000 },
    { selector: "astro-island:not([ssr]) select", timeout: 60000 },
  ]);
  expect(listeners.size).toBe(0);
});

test("todas las sondas wizard de almacenamiento-externo usan openWizardFixture y margen >= 30 s en astro dev frío", () => {
  const systemLanguage = readFileSync(
    join(e2eDir, "system-language.mjs"),
    "utf8",
  );
  expect(systemLanguage).toContain(
    "openWizardFixture(page, url, readySelector)",
  );
  expect(systemLanguage).not.toMatch(
    /setDefaultTimeout\((?:12000|15000|20000)\)/,
  );

  const dmAlerts = readFileSync(
    join(e2eDir, "rooms-dm-notifications.mjs"),
    "utf8",
  );
  expect(dmAlerts).toContain("openWizardFixture(page, fixtureURL)");

  for (const script of [
    "almacenamiento-externo.mjs",
    "rooms-conexiones.mjs",
    "rooms-canales-externos.mjs",
    "rooms-teams-protegido.mjs",
    "conectar-app.mjs",
    "system-language.mjs",
    "rooms-dm-notifications.mjs",
  ]) {
    const content = readFileSync(join(e2eDir, script), "utf8");
    expect(content).toContain("setDefaultTimeout(30000)");
  }
});
