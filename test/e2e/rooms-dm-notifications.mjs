// SPDX-License-Identifier: Apache-2.0
// Remote-only fixture: supply its full CI URL or an explicit remote /rooms base.
import assert from "node:assert/strict";
import { mkdir, writeFile } from "node:fs/promises";
import { chromium } from "playwright";

const base = process.env.SYTUE_DM_FIXTURE_BASE;
const fixtureURL =
  process.env.DM_ALERTS_URL ??
  (base ? `${base.replace(/\/$/, "")}/dev/avisos-directos` : undefined);
if (!fixtureURL)
  throw new Error(
    "Set DM_ALERTS_URL to the full remote fixture URL or SYTUE_DM_FIXTURE_BASE to its Rooms base",
  );
const artifacts = process.env.DM_ALERTS_OUT ?? process.env.SYTUE_DM_ARTIFACTS;
if (artifacts) await mkdir(artifacts, { recursive: true });
const browser = await chromium.launch({
  headless: true,
  ...(process.env.SYTUE_CHROMIUM
    ? { executablePath: process.env.SYTUE_CHROMIUM }
    : {}),
  args: ["--no-sandbox"],
});
const results = [];
const contexts = [];
let activePage;

async function fixture({
  legacy,
  error = false,
  lostWrite = false,
  viewport,
} = {}) {
  const context = await browser.newContext({
    viewport: viewport ?? { width: 1280, height: 800 },
  });
  contexts.push(context);
  await context.addInitScript(
    ({ legacy }) => {
      if (legacy !== undefined)
        localStorage.setItem("sytue-rooms-dm-notify", legacy);
      window.__permissionRequests = 0;
      if (typeof Notification !== "undefined")
        Notification.requestPermission = async () => {
          window.__permissionRequests++;
          return "denied";
        };
    },
    { legacy },
  );
  const page = await context.newPage();
  activePage = page;
  let enabled = true;
  let fail = error;
  const writes = [];
  await page.route("**/api/dm-notifications", async (route) => {
    if (route.request().method() === "PUT") {
      writes.push({
        owner: route.request().headers()["x-sytue-preference-owner"],
        tenant: route.request().headers()["x-sytue-preference-tenant"],
        body: route.request().postDataJSON(),
      });
      enabled = route.request().postDataJSON().enabled;
      return route.fulfill({
        status: lostWrite ? 502 : 200,
        json: { ok: !lostWrite },
      });
    }
    return route.fulfill({
      status: fail ? 502 : 200,
      json: fail ? { error: "unavailable" } : { enabled },
    });
  });
  await page.goto(fixtureURL);
  await page
    .getByRole("button", { name: "Organization", exact: true })
    .waitFor();
  return {
    page,
    context,
    writes,
    recover: () => {
      fail = false;
    },
  };
}
async function openMenu(page) {
  const trigger = page.locator('button[aria-haspopup="menu"]');
  if ((await trigger.getAttribute("aria-expanded")) !== "true")
    await trigger.click();
}
async function expectText(page, value) {
  await page.waitForFunction(
    (value) =>
      document.querySelector('[data-testid="browser-enabled"]')?.textContent ===
      value,
    value,
  );
}

try {
  {
    const { page, writes } = await fixture({ legacy: "0" });
    await openMenu(page);
    const dmAction = page.getByRole("menuitem", {
      name: "Turn off direct message alerts",
      exact: true,
    });
    await dmAction.scrollIntoViewIfNeeded();
    await expectText(page, "false");
    assert.equal(
      await page.evaluate(() => localStorage.getItem("sytue-rooms-dm-notify")),
      "0",
    );
    assert.equal(writes.length, 0);
    await page
      .getByRole("button", { name: "Use account setting", exact: true })
      .click();
    await expectText(page, "true");
    assert.equal(
      await page.evaluate(() => localStorage.getItem("sytue-rooms-dm-notify")),
      null,
    );
    assert.equal(writes.length, 0);
    assert.equal(await page.evaluate(() => window.__permissionRequests), 0);
    await page
      .getByRole("menuitem", {
        name: "Turn off direct message alerts",
        exact: true,
      })
      .click();
    await expectText(page, "false");
    await openMenu(page);
    await page
      .getByRole("menuitem", {
        name: "Turn on direct message alerts",
        exact: true,
      })
      .waitFor();
    assert.deepEqual(writes, [
      { owner: "person", tenant: "org-a", body: { enabled: false } },
    ]);
    results.push(
      "legacy silence retained; explicit discard sends no PUT; account toggle sends one scoped PUT; no permission prompt",
    );
  }
  {
    const { page, writes, recover } = await fixture({ error: true });
    await openMenu(page);
    await page.getByRole("alert").waitFor();
    assert.equal(
      await page
        .getByRole("menuitem", {
          name: "Turn on direct message alerts",
          exact: true,
        })
        .isDisabled(),
      true,
    );
    recover();
    await page.getByRole("button", { name: "Retry", exact: true }).click();
    await expectText(page, "true");
    assert.equal(writes.length, 0);
    results.push("load failure blocks changes and Retry only reads");
  }
  {
    const { page, writes } = await fixture({ lostWrite: true });
    await openMenu(page);
    await page
      .getByRole("menuitem", {
        name: "Turn off direct message alerts",
        exact: true,
      })
      .click();
    await page
      .getByTestId("preference-state")
      .filter({ hasText: "error" })
      .waitFor();
    await openMenu(page);
    await page.getByRole("button", { name: "Retry", exact: true }).click();
    await page
      .getByRole("menuitem", {
        name: "Turn on direct message alerts",
        exact: true,
      })
      .waitFor();
    assert.equal(writes.length, 1);
    results.push("lost write acknowledgement reconciles by GET without replay");
  }
  {
    const { page, writes } = await fixture({
      viewport: { width: 390, height: 844 },
    });
    await openMenu(page);
    const dmAction = page.getByRole("menuitem", {
      name: "Turn off direct message alerts",
      exact: true,
    });
    await dmAction.scrollIntoViewIfNeeded();
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      true,
    );
    if (artifacts) await page.screenshot({ path: `${artifacts}/mobile.png` });
    await page.keyboard.press("Escape");
    await page
      .getByRole("button", { name: "Organization", exact: true })
      .click();
    await openMenu(page);
    await page
      .getByRole("menuitem", {
        name: "Turn off direct message alerts",
        exact: true,
      })
      .waitFor();
    await page
      .getByRole("menuitem", {
        name: "Turn off direct message alerts",
        exact: true,
      })
      .click();
    await page
      .getByTestId("preference-state")
      .filter({ hasText: "ready" })
      .waitFor();
    assert.equal(writes.at(-1).tenant, "org-b");
    results.push(
      "mobile menu fits; identity change remounts scoped preference",
    );
  }
  const report = JSON.stringify({ passed: results.length, results }, null, 2);
  console.log(report);
  if (artifacts) await writeFile(`${artifacts}/results.json`, `${report}\n`);
} catch (error) {
  console.error(await activePage?.locator("body").innerText());
  if (artifacts && activePage)
    await activePage.screenshot({ path: `${artifacts}/failure.png` });
  throw error;
} finally {
  await Promise.all(contexts.map((context) => context.close()));
  await browser.close();
}
