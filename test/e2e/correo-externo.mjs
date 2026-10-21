// SPDX-License-Identifier: Apache-2.0
// Real Mail wizard and reader. Every account, provider and message is synthetic.
import { chromium } from "playwright";
import { expect as baseExpect } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import {
  assertWizardLayout,
  assertWizardAccessibility,
  openWizardFixture,
  describeWizardState,
} from "./lib/wizard-checks.mjs";
const expect = baseExpect.configure({ timeout: 15000 });
const BASE =
  process.env.MAIL_EXTERNAL_URL ||
  "http://127.0.0.1:4343/mail/prueba/dev/correo-externo";
const OUT = process.env.MAIL_EXTERNAL_OUT || "/tmp/correo-externo";
mkdirSync(OUT, { recursive: true });
const browser = await chromium.launch();
const report = [];
const account = {
  instance_id: "11111111-1111-4111-8111-111111111111",
  provider: "imap",
  name: "Fixture",
  address: "fixture@example.eu",
  connected: true,
  configuration: {
    address: "fixture@example.eu",
    incoming: {
      host: "imap.example.eu",
      port: 993,
      username: "fixture@example.eu",
      security: "tls",
    },
    outgoing: {
      host: "smtp.example.eu",
      port: 587,
      username: "fixture@example.eu",
      security: "starttls",
    },
  },
  capabilities: {
    read: true,
    send: true,
    actions: true,
    attachments: true,
    continuous_sync: false,
  },
};
const providers = ["gmail", "microsoft", "yahoo", "imap", "pop3"].map(
  (provider) => ({
    provider,
    available: provider === "imap" || provider === "pop3",
    authorization: ["imap", "pop3"].includes(provider) ? "password" : "oauth2",
  }),
);
async function contextFor(viewport, connected = false) {
  const context = await browser.newContext({ viewport });
  let configured = connected,
    configCalls = 0,
    sends = 0,
    mode = "success";
  const calls = [];
  await context.route("**/api/external-mail/**", async (route) => {
    const req = route.request(),
      url = new URL(req.url());
    calls.push({ method: req.method(), path: url.pathname });
    let body,
      status = 200;
    if (url.pathname.endsWith("/accounts") && req.method() === "GET")
      body = { items: configured ? [account] : [], providers };
    else if (url.pathname.endsWith("/accounts") && req.method() === "POST") {
      configCalls++;
      const data = req.postDataJSON();
      expect(data.provider).toBe("imap");
      expect(data.incoming.security).toBe("tls");
      expect(data.outgoing.security).toBe("starttls");
      expect(data.password).toBe("synthetic-app-password");
      if (configCalls === 1) {
        status = 422;
        body = { error: { code: "mail_tls" } };
      } else {
        configured = true;
        body = { instance_id: account.instance_id };
        status = 201;
      }
    } else if (url.pathname.endsWith("/folders"))
      body = {
        items: [
          { id: "INBOX", name: "Inbox", role: "inbox" },
          { id: "TRASH", name: "Trash", role: "trash" },
        ],
      };
    else if (url.pathname.endsWith("/messages"))
      body = {
        items: [
          {
            id: "message-1",
            subject: "Synthetic email",
            from: [{ email: "sender@example.eu" }],
            to: [{ email: "fixture@example.eu" }],
            date: "2026-10-20T12:00:00Z",
            snippet: "Fixture body",
            seen: false,
            flagged: false,
          },
        ],
      };
    else if (url.pathname.endsWith("/message"))
      body = {
        id: "message-1",
        subject: "Synthetic email",
        from: [{ email: "sender@example.eu" }],
        to: [],
        date: "2026-10-20T12:00:00Z",
        text: "Fixture body",
        html: '<img src="https://tracking.invalid/pixel"><script>throw 1</script>',
        seen: false,
        flagged: false,
      };
    else if (url.pathname.endsWith("/send")) {
      sends++;
      const data = req.postDataJSON();
      expect(data.expected_address).toBe("fixture@example.eu");
      expect(data.to[0].email).toBe("recipient@example.eu");
      expect(data.bcc[0].email).toBe("blind@example.eu");
      if (mode === "unknown") {
        status = 502;
        body = { error: { code: "mail_send_unknown" } };
      } else body = { sent_copy_saved: false };
    } else if (req.method() === "DELETE") {
      configured = false;
      status = 204;
      body = undefined;
    } else throw new Error(`Unmocked mail request ${req.method()} ${url}`);
    await route.fulfill({
      status,
      contentType: "application/json",
      ...(body ? { body: JSON.stringify(body) } : {}),
    });
  });
  return {
    context,
    calls,
    setMode: (value) => {
      mode = value;
    },
    get configCalls() {
      return configCalls;
    },
    get sends() {
      return sends;
    },
  };
}
try {
  for (const viewport of [
    { width: 1280, height: 900 },
    { width: 390, height: 844 },
  ]) {
    const fixture = await contextFor(viewport);
    const page = await fixture.context.newPage();
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await openWizardFixture(page, BASE);
    await page
      .getByRole("button", { name: "Add email account", exact: true })
      .click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Add email account", exact: true })
      .click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByText("Provider", { exact: true }).first(),
    ).toBeVisible();
    await assertWizardLayout({ page, dialog });
    await dialog.getByRole("button", { name: "Continue", exact: true }).click();
    await expect(
      dialog.getByText(/not available for your account/),
    ).toBeVisible();
    await expect(
      dialog.getByRole("button", { name: "Continue to provider", exact: true }),
    ).toBeDisabled();
    await dialog.getByRole("button", { name: "Back", exact: true }).click();
    await dialog.getByText("IMAP", { exact: true }).click();
    await dialog.getByRole("button", { name: "Continue", exact: true }).click();
    await dialog.locator('input[name="email"]').fill("fixture@example.eu");
    await dialog.locator('input[name="incoming_host"]').fill("imap.example.eu");
    await dialog
      .locator('input[name="incoming_password"]')
      .fill("synthetic-app-password");
    await dialog.locator('input[name="smtp_host"]').fill("smtp.example.eu");
    await dialog
      .locator('select[name="smtp_security"]')
      .selectOption("starttls");
    await assertWizardAccessibility({ page });
    await page.screenshot({
      path: `${OUT}/wizard-${viewport.width}.png`,
      fullPage: true,
    });
    await dialog.getByRole("button", { name: "Review", exact: true }).click();
    await dialog
      .getByRole("button", { name: "Connect account", exact: true })
      .click();
    await expect(
      dialog.getByText(/secure connection could not be verified/),
    ).toBeVisible();
    await dialog
      .getByRole("button", { name: "Connect account", exact: true })
      .click();
    await expect(
      dialog.getByRole("button", { name: "Synthetic email", exact: false }),
    ).toBeVisible();
    expect(fixture.configCalls).toBe(2);
    await dialog
      .getByRole("button", { name: "Synthetic email", exact: false })
      .click();
    await expect(
      dialog.getByText("Fixture body", { exact: true }),
    ).toBeVisible();
    expect(await page.locator('img[src*="tracking.invalid"]').count()).toBe(0);
    await dialog.getByRole("button", { name: "Compose", exact: true }).click();
    await dialog.locator('input[name="to"]').fill("recipient@example.eu");
    await dialog.locator('input[name="bcc"]').fill("blind@example.eu");
    await dialog
      .locator('textarea[name="body"]')
      .fill("Synthetic outgoing email");
    fixture.setMode("unknown");
    await dialog.getByRole("button", { name: "Send", exact: true }).click();
    await expect(dialog.getByText(/server may have accepted/)).toBeVisible();
    expect(fixture.sends).toBe(1);
    await expect(dialog.locator('textarea[name="body"]')).toHaveValue(
      "Synthetic outgoing email",
    );
    fixture.setMode("success");
    await dialog.getByRole("button", { name: "Send", exact: true }).click();
    await expect(
      dialog.getByText(/copy could not be saved in Sent/),
    ).toBeVisible();
    expect(fixture.sends).toBe(2);
    await expect(dialog.locator("textarea")).toHaveCount(0);
    await page.screenshot({
      path: `${OUT}/mailbox-${viewport.width}.png`,
      fullPage: true,
    });
    await dialog.getByRole("button", { name: "Back", exact: true }).click();
    await dialog
      .getByRole("button", { name: "Connect account", exact: true })
      .click();
    await expect(dialog.locator('input[name="incoming_host"]')).toHaveValue(
      "imap.example.eu",
    );
    await expect(dialog.locator('input[name="incoming_password"]')).toHaveValue(
      "",
    );
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    expect(errors).toEqual([]);
    report.push({
      viewport,
      configAttempts: fixture.configCalls,
      sends: fixture.sends,
      layout: "passed",
      accessibility: "passed",
      retry: "passed",
      sentCopyWarning: "passed",
    });
    await fixture.context.close();
  }
  const oauthContext = await browser.newContext({
    viewport: { width: 390, height: 844 },
  });
  let consentRequests = 0;
  await oauthContext.route("**/api/external-mail/**", async (route) => {
    const request = route.request();
    if (request.url().endsWith("/accounts")) {
      await route.fulfill({
        json: {
          items: [],
          providers: providers.map((p) => ({ ...p, available: true })),
        },
      });
      return;
    }
    expect(new URL(request.url()).pathname.endsWith("/connect")).toBe(true);
    consentRequests++;
    expect(request.postDataJSON()).toEqual({
      provider: "gmail",
      return_to: "mail",
    });
    await route.fulfill({
      json: {
        instance_id: account.instance_id,
        authorize_url:
          "https://accounts.google.com/o/oauth2/v2/auth?state=synthetic",
        scopes: ["gmail.modify"],
      },
    });
  });
  await oauthContext.route("https://accounts.google.com/**", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: "<html><title>Synthetic consent</title><body>Provider consent fixture</body></html>",
    }),
  );
  const oauthPage = await oauthContext.newPage();
  await openWizardFixture(oauthPage, BASE);
  await oauthPage
    .getByRole("button", { name: "Add email account", exact: true })
    .click();
  await oauthPage
    .getByRole("dialog")
    .getByRole("button", { name: "Add email account", exact: true })
    .click();
  await oauthPage
    .getByRole("dialog")
    .getByRole("button", { name: "Continue", exact: true })
    .click();
  await oauthPage
    .getByRole("button", { name: "Continue to provider", exact: true })
    .click();
  await expect(oauthPage).toHaveURL(/https:\/\/accounts\.google\.com\//);
  expect(consentRequests).toBe(1);
  await openWizardFixture(
    oauthPage,
    BASE + "?connect-mail=1&oauth_error=denied",
  );
  await oauthPage
    .getByRole("button", { name: "Add email account", exact: true })
    .click();
  await expect(
    oauthPage.getByRole("dialog").getByText(/Sign-in was not accepted/),
  ).toBeVisible();
  expect(new URL(oauthPage.url()).search).toBe("");
  report.push({
    oauth: "navigation passed",
    denied: "recovery passed",
    consentRequests,
  });
  await oauthContext.close();
  writeFileSync(`${OUT}/report.json`, JSON.stringify(report, null, 2));
} catch (error) {
  writeFileSync(`${OUT}/failure.txt`, String(error));
  throw error;
} finally {
  await browser.close();
}
