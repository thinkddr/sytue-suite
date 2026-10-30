// SPDX-License-Identifier: Apache-2.0
// Remote only: the actual personal-channel wizard, with synthetic provider data.
import { createReportDirectory, writeReport } from './lib/report-output.mjs';
import { chromium } from "playwright";
import { expect } from "@playwright/test";
import { mkdirSync } from "node:fs";
import {
  assertWizardLayout,
  assertWizardAccessibility,
  describeWizardState,
  openWizardFixture,
} from "./lib/wizard-checks.mjs";

const BASE =
  process.env.ROOMS_URL || "http://127.0.0.1:4343/rooms/prueba/dev/conexiones";
const OUT = createReportDirectory('rooms-externos', process.env.ROOMS_OUT ? `${process.env.ROOMS_OUT}/externos` : undefined);
const browser = await chromium.launch();
const results = [];
const providers = {
  slack: { name: "Slack", host: "slack.com", remote: "C123" },
  "microsoft-teams": {
    name: "Microsoft Teams",
    host: "login.microsoftonline.com",
    remote: "team/channel",
  },
  "google-chat": {
    name: "Google Chat",
    host: "accounts.google.com",
    remote: "spaces/AAA",
  },
};
const id = "11111111-1111-4111-8111-111111111111";
const capabilities = {
  read: true,
  send: true,
  threads: true,
  edit: true,
  delete: true,
  attachments: false,
  e2ee: false,
};
async function mount(provider, options = {}) {
  const info = providers[provider];
  const context = await browser.newContext({
    viewport: { width: options.width || 1280, height: 850 },
    isMobile: Boolean(options.width),
    hasTouch: Boolean(options.width),
    reducedMotion: "reduce",
  });
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const state = {
    connected: Boolean(options.connected),
    meetings: options.meetings ?? [],
    meetingRequests: [],
    meetVisits: [],
    writes: [],
    nativeWrites: [],
    visits: [],
    failSend: Boolean(options.failSend),
    messages: [
      {
        id: "own-1",
        remote_id: info.remote,
        author_id: "self",
        author_name: "Alice",
        text: "An external message",
        created_at: "2026-10-16T10:00:00Z",
        owned: true,
        reply_count: 1,
      },
      {
        id: "other-1",
        remote_id: info.remote,
        author_id: "peer",
        author_name: "Bob",
        text: "A colleague reply",
        created_at: "2026-10-16T10:01:00Z",
        owned: false,
      },
    ],
  };
  const json = (route, value, status = 200) =>
    route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(value),
    });
  await context.route("**/rooms/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (!url.pathname.startsWith("/rooms/api/external-conversations/")) {
      if (request.method() !== "GET") state.nativeWrites.push(url.pathname);
      return json(route, {}, 500);
    }
    const path = url.pathname.split("/external-conversations/")[1];
    if (path === "accounts")
      return json(route, {
        items: state.connected
          ? [
              {
                instance_id: id,
                provider,
                name: info.name,
                connected: true,
                workspace_name: "Sytue",
              },
            ]
          : [],
        meetings:
          provider === "microsoft-teams" && state.connected
            ? [
                {
                  provider: "microsoft-teams",
                  available: true,
                  connected: true,
                  instance_id: id,
                  account_id: "fixture-owner",
                },
              ]
            : state.meetings,
        providers: Object.keys(providers).map((key) => ({
          provider: key,
          available: options.available !== false,
          capabilities,
        })),
      });
    if (path === "connect") {
      const body = request.postDataJSON();
      if (body.provider === "google-meet") {
        expect(body).toEqual({ provider: "google-meet", return_to: "rooms" });
        const instance = `meet-new-${state.meetVisits.length + 1}`;
        state.meetings.push({
          provider: "google-meet",
          available: true,
          connected: false,
          instance_id: instance,
          name: "Added Google account",
        });
        return json(route, {
          instance_id: instance,
          authorize_url: `https://accounts.google.com/sytue-meet-consent?instance=${instance}`,
          scopes: [],
        });
      }
      expect(body).toEqual({ provider, return_to: "rooms" });
      return json(route, {
        instance_id: id,
        authorize_url: `https://${info.host}/sytue-external-consent`,
        scopes: [],
      });
    }
    if (path === "meeting") {
      const body = request.postDataJSON();
      expect(body.provider).toBe("google-meet");
      expect(
        state.meetings.some(
          (item) => item.instance_id === body.instance_id && item.connected,
        ),
      ).toBe(true);
      state.meetingRequests.push(body);
      return json(
        route,
        {
          url: `https://meet.google.com/test-meet-${state.meetingRequests.length}`,
        },
        201,
      );
    }
    if (path === `${id}/channels`)
      return json(route, {
        items: [
          { id: info.remote, name: "general", kind: "channel", capabilities },
        ],
      });
    if (path === `${id}/messages` && request.method() === "GET")
      return json(route, { items: state.messages, capabilities });
    if (path === `${id}/messages`) {
      const body = request.postDataJSON();
      state.writes.push(body);
      const message = {
        id: `sent-${state.writes.length}`,
        remote_id: body.remote_id,
        author_id: "self",
        text: body.text,
        owned: true,
        created_at: "2026-10-16T11:00:00Z",
        ...(body.thread_id ? { thread_id: body.thread_id } : {}),
      };
      state.messages.push(message);
      if (state.failSend) {
        state.failSend = false;
        return route.abort("failed");
      }
      return json(route, message, 201);
    }
    if (path === `${id}/actions`) {
      const body = request.postDataJSON();
      state.writes.push(body);
      state.messages =
        body.action === "delete"
          ? state.messages.filter((message) => message.id !== body.message_id)
          : state.messages.map((message) =>
              message.id === body.message_id
                ? { ...message, text: body.text, edited: true }
                : message,
            );
      return json(route, {});
    }
    return json(route, {}, 404);
  });
  await context.route(
    `https://${info.host}/sytue-external-consent`,
    async (route) => {
      state.visits.push(provider);
      state.connected = true;
      const callback = new URL(BASE);
      callback.search = new URLSearchParams({
        lang: "en",
        keep: "preserved",
        "connect-external": "1",
        instance: id,
      }).toString();
      return route.fulfill({
        status: 302,
        headers: { location: callback.href },
      });
    },
  );
  await context.route(
    "https://accounts.google.com/sytue-meet-consent*",
    async (route) => {
      const instance = new URL(route.request().url()).searchParams.get(
        "instance",
      );
      state.meetVisits.push(instance);
      state.meetings = state.meetings.map((item) =>
        item.instance_id === instance ? { ...item, connected: true } : item,
      );
      const callback = new URL(BASE);
      callback.search = new URLSearchParams({
        lang: "en",
        keep: "preserved",
        "connect-external": "1",
        instance,
      }).toString();
      return route.fulfill({
        status: 302,
        headers: { location: callback.href },
      });
    },
  );
  await openWizardFixture(page, `${BASE}?lang=en`);
  const botonExternos = page.getByRole("button", {
    name: "External channels",
    exact: true,
  });
  await botonExternos.waitFor({ timeout: 60000 });
  await botonExternos.click();
  const dialog = page.getByRole("dialog", {
    name: "External channels",
    exact: true,
  });
  await expect(dialog).toBeVisible();
  await dialog
    .getByText(options.meetings?.length ? "Google Meet meeting" : info.name, {
      exact: true,
    })
    .click();
  await dialog.getByRole("button", { name: "Continue", exact: true }).click();
  return { context, page, dialog, state, errors, provider, info };
}
async function waitForConsentReturn(ui) {
  await ui.page.waitForURL(
    (url) =>
      url.searchParams.get("keep") === "preserved" &&
      !url.searchParams.has("instance"),
  );
}
async function open(ui) {
  if (ui.state.connected) {
    await ui.dialog
      .getByText(`${ui.info.name} · Sytue`, { exact: true })
      .click();
    await ui.dialog
      .getByRole("button", { name: "Continue", exact: true })
      .click();
  } else {
    await ui.dialog
      .getByRole("button", { name: `Continue in ${ui.info.name}`, exact: true })
      .click();
    await waitForConsentReturn(ui);
    expect(ui.state.visits).toEqual([ui.provider]);
    expect(new URL(ui.page.url()).searchParams.has("instance")).toBe(false);
  }
  await expect(
    ui.dialog.getByRole("heading", {
      name: "Choose the channels",
      exact: true,
    }),
  ).toBeVisible();
  await ui.dialog
    .getByLabel(`${ui.info.name} channel`, { exact: true })
    .selectOption(ui.info.remote);
  await ui.dialog
    .getByRole("button", { name: "Continue", exact: true })
    .click();
  await assertWizardLayout(ui);
  await assertWizardAccessibility(ui);
  await ui.dialog
    .getByRole("button", { name: "Open conversation", exact: true })
    .click();
  ui.dialog = ui.page.getByRole("dialog", { name: "general", exact: true });
  await expect(
    ui.dialog.getByText("An external message", { exact: true }),
  ).toBeVisible();
  await expect(
    ui.dialog.getByText(`${ui.info.name} · no Rooms E2E encryption`, {
      exact: true,
    }),
  ).toBeVisible();
}
async function scenario(name, provider, run, options = {}) {
  let ui;
  try {
    ui = await mount(provider, options);
    await run(ui);
    expect(ui.errors).toEqual([]);
    expect(ui.state.nativeWrites).toEqual([]);
    results.push({ name, ok: true });
    console.log(`✓ ${name}`);
  } catch (error) {
    results.push({ name, ok: false, error: String(error) });
    console.error(`✗ ${name}: ${error.message}`);
    if (ui)
      console.error(
        JSON.stringify(await describeWizardState(ui).catch(String)),
      );
  } finally {
    if (ui) await ui.context.close();
  }
}
try {
  await scenario(
    "meet-connects-from-zero-with-only-consent",
    "slack",
    async (ui) => {
      await ui.dialog
        .getByRole("button", { name: "Connect Google Meet", exact: true })
        .click();
      await waitForConsentReturn(ui);
      await expect(ui.dialog.locator("#external-meet-account")).toHaveValue(
        "meet-new-1",
      );
      await ui.dialog
        .getByRole("button", { name: "New Meet link", exact: true })
        .click();
      await expect(
        ui.dialog.getByRole("link", {
          name: "https://meet.google.com/test-meet-1",
          exact: true,
        }),
      ).toBeVisible();
      expect(ui.state.meetingRequests).toEqual([
        { provider: "google-meet", instance_id: "meet-new-1" },
      ]);
      expect(ui.state.meetVisits).toEqual(["meet-new-1"]);
    },
    { meetings: [{ provider: "google-meet", available: true }] },
  );
  await scenario(
    "meet-selects-explicit-account-and-clears-old-link",
    "slack",
    async (ui) => {
      const selector = ui.dialog.locator("#external-meet-account");
      const create = ui.dialog.getByRole("button", {
        name: "New Meet link",
        exact: true,
      });
      await expect(selector).toHaveValue("");
      await expect(create).toBeDisabled();
      await selector.selectOption("meet-second");
      await create.click();
      await expect(
        ui.dialog.getByRole("link", {
          name: "https://meet.google.com/test-meet-1",
          exact: true,
        }),
      ).toBeVisible();
      await selector.selectOption("meet-first");
      await expect(
        ui.dialog.getByRole("link", {
          name: "https://meet.google.com/test-meet-1",
          exact: true,
        }),
      ).toHaveCount(0);
      await create.click();
      expect(ui.state.meetingRequests.map((item) => item.instance_id)).toEqual([
        "meet-second",
        "meet-first",
      ]);
      const meet = ui.dialog.getByRole("region", {
        name: "Google Meet",
        exact: true,
      });
      await meet
        .getByRole("button", { name: "Add account", exact: true })
        .click();
      await waitForConsentReturn(ui);
      await expect(ui.dialog.locator("#external-meet-account")).toHaveValue(
        "meet-new-1",
      );
      expect(ui.state.meetings.filter((item) => item.connected)).toHaveLength(
        3,
      );
      await assertWizardAccessibility(ui);
    },
    {
      meetings: [
        {
          provider: "google-meet",
          available: true,
          connected: false,
          instance_id: "meet-pending",
          name: "Pending",
        },
        {
          provider: "google-meet",
          available: true,
          connected: true,
          instance_id: "meet-first",
          name: "First account",
        },
        {
          provider: "google-meet",
          available: true,
          connected: true,
          instance_id: "meet-second",
          name: "Second account",
        },
      ],
    },
  );
  for (const provider of Object.keys(providers))
    await scenario(
      `${provider}-personal-consent-read-and-send`,
      provider,
      async (ui) => {
        await open(ui);
        await ui.dialog
          .getByLabel("Write a message in general", { exact: true })
          .fill("Reply from Rooms");
        await ui.dialog
          .getByRole("button", { name: "Send", exact: true })
          .click();
        await expect(
          ui.dialog.getByText("Reply from Rooms", { exact: true }),
        ).toBeVisible();
        expect(ui.state.writes).toEqual([
          { remote_id: ui.info.remote, text: "Reply from Rooms" },
        ]);
        await ui.page.screenshot({
          path: `${OUT}/${provider}.png`,
          animations: "disabled",
        });
      },
    );
  await scenario(
    "slack-thread-edit-and-delete-own-message",
    "slack",
    async (ui) => {
      await open(ui);
      const own = ui.dialog.locator("li").filter({
        has: ui.page.getByText("An external message", { exact: true }),
      });
      await own.getByRole("button", { name: "Edit", exact: true }).click();
      await ui.dialog
        .getByLabel("Edit message", { exact: true })
        .fill("Updated externally");
      await ui.dialog
        .getByRole("button", { name: "Save", exact: true })
        .click();
      await expect(
        ui.dialog.getByText("Updated externally", { exact: true }),
      ).toBeVisible();
      await ui.dialog
        .locator("li")
        .filter({
          has: ui.page.getByText("Updated externally", { exact: true }),
        })
        .getByRole("button", { name: /Reply in thread/ })
        .click();
      await ui.dialog
        .getByLabel("Write a message in general", { exact: true })
        .fill("A threaded reply");
      await ui.dialog
        .getByRole("button", { name: "Send", exact: true })
        .click();
      expect(ui.state.writes.at(-1).thread_id).toBe("own-1");
      await ui.dialog
        .getByRole("button", { name: "Close thread", exact: true })
        .click();
      const updated = ui.dialog.locator("li").filter({
        has: ui.page.getByText("Updated externally", { exact: true }),
      });
      await updated
        .getByRole("button", { name: "Delete", exact: true })
        .click();
      await ui.page
        .getByRole("dialog", { name: "Delete", exact: true })
        .getByRole("button", { name: "Delete", exact: true })
        .click();
      await expect(
        ui.dialog.getByText("Updated externally", { exact: true }),
      ).toHaveCount(0);
      expect(ui.state.writes.at(-1)).toMatchObject({
        action: "delete",
        message_id: "own-1",
      });
    },
    { connected: true },
  );
  await scenario(
    "ambiguous-send-refreshes-without-replaying",
    "slack",
    async (ui) => {
      await open(ui);
      await ui.dialog
        .getByLabel("Write a message in general", { exact: true })
        .fill("Accepted before disconnection");
      await ui.dialog
        .getByRole("button", { name: "Send", exact: true })
        .click();
      await expect(
        ui.dialog.getByText(
          "Sending could not be confirmed. Check recent messages before sending again.",
          { exact: true },
        ),
      ).toBeVisible();
      await expect(
        ui.dialog.getByRole("button", { name: "Send", exact: true }),
      ).toBeDisabled();
      await ui.dialog
        .getByRole("button", { name: "Retry", exact: true })
        .click();
      await expect(
        ui.dialog.getByText("Accepted before disconnection", { exact: true }),
      ).toBeVisible();
      expect(ui.state.writes).toHaveLength(1);
    },
    { connected: true, failSend: true },
  );
  for (const width of [320, 390])
    await scenario(
      `mobile-${width}-external-reader`,
      "slack",
      async (ui) => {
        await open(ui);
        await assertWizardLayout(ui);
        await assertWizardAccessibility(ui);
        await ui.page.screenshot({
          path: `${OUT}/mobile-${width}.png`,
          animations: "disabled",
        });
      },
      { connected: true, width },
    );
  await scenario(
    "unconfigured-provider-does-not-start-oauth",
    "slack",
    async (ui) => {
      await expect(
        ui.dialog.getByText(
          "Connecting to Slack has not been configured yet.",
          { exact: true },
        ),
      ).toBeVisible();
      await expect(
        ui.dialog.getByRole("button", {
          name: "Continue in Slack",
          exact: true,
        }),
      ).toBeDisabled();
      expect(ui.state.visits).toEqual([]);
    },
    { available: false },
  );
} finally {
  writeReport(`${OUT}/results.json`, JSON.stringify(results, null, 2));
  await browser.close();
}
if (results.some((result) => !result.ok)) process.exitCode = 1;
