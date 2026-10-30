// SPDX-License-Identifier: Apache-2.0
// Requires an explicitly supplied remote Rooms development fixture.
import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { chromium } from "playwright";
import { openWizardFixture } from "./lib/wizard-checks.mjs";

const base = process.env.SYTUE_LANGUAGE_FIXTURE_BASE;
const fixtureURL =
  process.env.SYSTEM_LANGUAGE_URL ??
  (base ? `${base.replace(/\/$/, "")}/dev/idioma-sistema` : undefined);
if (!fixtureURL)
  throw new Error(
    "Set SYSTEM_LANGUAGE_URL to the full remote fixture URL or SYTUE_LANGUAGE_FIXTURE_BASE to its Rooms base",
  );
const artifacts = process.env.SYSTEM_LANGUAGE_OUT;
if (artifacts) await mkdir(artifacts, { recursive: true });
const browser = await chromium.launch({
  ...(process.env.SYTUE_CHROMIUM
    ? { executablePath: process.env.SYTUE_CHROMIUM }
    : {}),
  args: ["--no-sandbox"],
});
const context = await browser.newContext({
  locale: "de-DE",
  viewport: { width: 390, height: 844 },
});
let serverLanguage = "it-IT,it;q=0.9";
await context.route(
  (url) => url.pathname.endsWith("/dev/idioma-sistema"),
  (route) =>
    route.continue({
      headers: {
        ...route.request().headers(),
        "accept-language": serverLanguage,
      },
    }),
);
const page = await context.newPage();
page.setDefaultTimeout(30000);
page.setDefaultNavigationTimeout(60000);
const errors = [];
page.on("pageerror", (error) => errors.push(error.message));
page.on("console", (message) => {
  if (message.type() === "error") console.error(message.text());
});
const target = new URL(fixtureURL);
target.searchParams.set("draft", "keep");
target.hash = "draft";
const url = target.href;
const origin = new URL(url).origin;
const cookie = async () =>
  (await context.cookies()).find((item) => item.name === "sytue_locale");
const readySelector = "astro-island:not([ssr]) select";
const selected = () => page.locator(readySelector).inputValue();
const lang = () => page.locator("html").getAttribute("lang");
async function choose(value, expectedLanguage) {
  await page.locator(readySelector).waitFor();
  const navigation = page.waitForEvent(
    "framenavigated",
    (frame) => frame === page.mainFrame(),
  );
  await page.locator(readySelector).selectOption(value);
  await navigation;
  await page.locator(readySelector).waitFor();
  await page.waitForFunction(
    (expected) => document.documentElement.lang === expected,
    expectedLanguage,
  );
  assert.equal(page.url(), url, "changing language lost path/query/hash");
}
try {
  await context.addCookies([
    { name: "opaque_probe", value: "keep", url: origin },
  ]);
  await openWizardFixture(page, url, readySelector);
  assert.equal(
    await lang(),
    "it",
    "SSR must use Accept-Language, not navigator.language",
  );
  assert.equal(await selected(), "system");
  assert.equal(await page.locator(readySelector).locator("option").count(), 27);

  await context.addCookies([
    { name: "sytue_locale", value: "fr", url: origin },
  ]);
  await page.reload();
  await page.waitForFunction(
    () => document.querySelector("select")?.value === "fr",
  );
  assert.equal(await lang(), "fr");
  await choose("system", "it");
  assert.equal(await selected(), "system");
  assert.equal(
    await cookie(),
    undefined,
    "System must delete the explicit preference",
  );
  assert.equal(
    (await context.cookies()).find((item) => item.name === "opaque_probe")
      ?.value,
    "keep",
  );

  // Choosing the already effective language is still an explicit choice.
  await choose("it", "it");
  assert.equal((await cookie())?.value, "it");
  assert.equal(await selected(), "it");
  serverLanguage = "es-ES,es;q=0.9";
  await page.reload();
  await page.waitForFunction(
    () => document.querySelector("select")?.value === "it",
  );
  assert.equal(await lang(), "it");
  await choose("system", "es");
  assert.equal(await cookie(), undefined);
  serverLanguage = "ja-JP";
  await page.reload();
  await page.waitForFunction(
    () =>
      document.documentElement.lang === "en" &&
      document.querySelector("select")?.value === "system",
  );
  assert.equal(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    true,
  );
  assert.deepEqual(errors, []);
  const report = {
    passed: true,
    checks: [
      "SSR negotiation",
      "explicit preference",
      "System reset",
      "equal-language choice",
      "unsupported-language fallback",
      "unchanged URL",
      "mobile layout",
    ],
  };
  console.log(JSON.stringify(report, null, 2));
  if (artifacts) {
    await page.screenshot({ path: `${artifacts}/mobile.png` });
    await writeFile(
      `${artifacts}/results.json`,
      `${JSON.stringify(report, null, 2)}\n`,
    );
  }
} catch (error) {
  if (artifacts) await page.screenshot({ path: `${artifacts}/failure.png` });
  console.error({
    errors,
    body: (await page.locator("body").innerText()).slice(0, 1500),
    islands: await page.locator("astro-island").evaluateAll((items) =>
      items.map((item) => ({
        ssr: item.hasAttribute("ssr"),
        component: item.getAttribute("component-url"),
      })),
    ),
  });
  throw error;
} finally {
  await browser.close();
}
