// SPDX-License-Identifier: Apache-2.0
// Runs remotely against the real personal consent wizard, never against Microsoft.
import { chromium } from "playwright";
import { expect } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import {
  assertWizardLayout,
  assertWizardAccessibility,
  openWizardFixture,
} from "./lib/wizard-checks.mjs";
const base = process.env.ROOMS_URL;
const out = process.env.ROOMS_OUT;
mkdirSync(out, { recursive: true });
const browser = await chromium.launch();
const results = [];
const account = "12345678-1234-1234-1234-123456789abc";
async function scenario(name, options, run) {
  const context = await browser.newContext({
    viewport: { width: options.mobile ? 390 : 1280, height: 850 },
    reducedMotion: "reduce",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(20000);
  const state = {
    writes: [],
    shares: [],
    visits: [],
    connected: !options.empty,
    failShare: true,
  };
  const errors = [];
  page.on("pageerror", (err) => errors.push(err.message));
  const json = (route, data, status = 200) =>
    route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(data),
    });
  await context.route("**/rooms/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.split("/rooms/api/")[1];
    if (path === "external-conversations/accounts")
      return json(route, {
        items: [],
        providers: [{ provider: "slack", available: true, capabilities: {} }],
        meetings: [
          {
            provider: "microsoft-teams",
            available: true,
            ...(state.connected
              ? {
                  instance_id: "selected",
                  account_id: account,
                  connected: true,
                  name: "Work account",
                }
              : {}),
          },
        ],
      });
    if (path === "external-conversations/connect") {
      state.visits.push(request.postDataJSON());
      return json(route, {
        instance_id: "selected",
        authorize_url:
          "https://login.microsoftonline.com/common/oauth2/v2.0/authorize?state=fixture",
        scopes: ["offline_access", "User.Read", "OnlineMeetings.ReadWrite"],
      });
    }
    if (path === "external-conversations/meeting") {
      state.writes.push(request.postDataJSON());
      if (options.unknown) return route.abort("failed");
      if (options.unprotected)
        return json(
          route,
          { error: { code: "external_meeting_unprotected" } },
          409,
        );
      return json(
        route,
        {
          provider: "microsoft-teams",
          meeting_id: "opaque",
          url: "https://teams.microsoft.com/l/meetup-join/opaque",
          e2ee: true,
          official_client_required: true,
        },
        201,
      );
    }
    if (path === "dev/meeting-share") {
      state.shares.push(request.postDataJSON());
      if (state.failShare) {
        state.failShare = false;
        return route.abort("failed");
      }
      return json(route, {});
    }
    return json(route, {}, 500);
  });
  await context.route("https://login.microsoftonline.com/**", (route) => {
    const callback = new URL(base);
    callback.search =
      "lang=en&connect-external=1&meeting=1&instance=selected&oauth_error=denied";
    return route.fulfill({ status: 302, headers: { location: callback.href } });
  });
  try {
    await openWizardFixture(page, `${base}?lang=en`);
    await page
      .getByRole("button", { name: "External channels", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    await dialog.getByText("Microsoft Teams meeting", { exact: true }).click();
    await dialog.getByRole("button", { name: "Continue", exact: true }).click();
    const block = page.getByRole("region", {
      name: "Microsoft Teams",
      exact: true,
    });
    await expect(block).toBeVisible();
    await expect(
      block.getByText("Chat is not end-to-end encrypted.", { exact: false }),
    ).toBeVisible();
    await run({ page, dialog, block, state });
    await assertWizardLayout({ page, dialog });
    await assertWizardAccessibility({ page });
    expect(errors).toEqual([]);
    await page.screenshot({ path: `${out}/${name}.png` });
    results.push({ name, ok: true });
  } catch (error) {
    results.push({ name, ok: false, error: String(error) });
    await page.screenshot({ path: `${out}/${name}-failed.png` });
  } finally {
    await context.close();
  }
}
try {
  for (const mobile of [false, true])
    await scenario(
      `protected-${mobile ? "mobile" : "desktop"}`,
      { mobile },
      async ({ block, state }) => {
        await block
          .getByRole("button", {
            name: "Create protected meeting",
            exact: true,
          })
          .click();
        await expect(
          block.getByText("End-to-end encryption enabled", { exact: true }),
        ).toBeVisible();
        expect(state.writes).toEqual([
          {
            provider: "microsoft-teams",
            instance_id: "selected",
            expected_account_id: account,
            e2ee: true,
          },
        ]);
        expect(
          await block
            .getByRole("link", { name: "Open", exact: true })
            .getAttribute("href"),
        ).toBe("msteams://teams.microsoft.com/l/meetup-join/opaque");
        await block
          .getByRole("button", { name: "Share in Rooms", exact: true })
          .click();
        await expect(
          block.getByText("It could not be shared", { exact: true }),
        ).toBeVisible();
        await block
          .getByRole("button", { name: "Share in Rooms", exact: true })
          .click();
        await expect(
          block.getByRole("button", { name: "Done", exact: true }),
        ).toBeDisabled();
        expect(state.shares).toHaveLength(2);
        expect(state.shares[0]).toEqual(state.shares[1]);
        expect(state.shares[0].clientId).toBeTruthy();
        expect(state.writes).toHaveLength(1);
      },
    );
  await scenario(
    "unprotected",
    { unprotected: true },
    async ({ block, state }) => {
      await block
        .getByRole("button", { name: "Create protected meeting" })
        .click();
      await expect(
        block.getByText("Teams did not confirm encryption.", { exact: false }),
      ).toBeVisible();
      await expect(
        block.getByRole("link", { name: "Open", exact: true }),
      ).toHaveCount(0);
      expect(state.shares).toHaveLength(0);
    },
  );
  await scenario("uncertain", { unknown: true }, async ({ block, state }) => {
    await block
      .getByRole("button", { name: "Create protected meeting" })
      .click();
    await expect(
      block.getByText("The meeting may have been created.", { exact: false }),
    ).toBeVisible();
    await expect(
      block.getByRole("button", { name: "Create protected meeting" }),
    ).toBeDisabled();
    expect(state.writes).toHaveLength(1);
  });
  await scenario(
    "empty-consent-cancel",
    { empty: true },
    async ({ page, block, state }) => {
      await block
        .getByRole("button", { name: "Authorize your account", exact: true })
        .click();
      await expect.poll(() => state.visits.length).toBe(1);
      expect(state.visits).toEqual([
        { provider: "microsoft-teams", return_to: "rooms", purpose: "meeting" },
      ]);
      await expect(page.getByRole("dialog")).toBeVisible({ timeout: 60000 });
      // No provider meeting is created as a side effect of consent or denial.
      expect(state.writes).toHaveLength(0);
    },
  );
} finally {
  writeFileSync(`${out}/results.json`, JSON.stringify(results, null, 2));
  await browser.close();
}
if (results.some((row) => !row.ok)) throw new Error(JSON.stringify(results));
