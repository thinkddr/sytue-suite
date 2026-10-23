// SPDX-License-Identifier: Apache-2.0
// Accounts and passwords below are synthetic. Exercise the real Mail component.
import { expect as baseExpect } from "@playwright/test";
const expect = baseExpect.configure({ timeout: 15000 });

export async function comprobarCuentaNueva(page) {
  const account = {
    instance_id: "11111111-1111-4111-8111-111111111111",
    provider: "imap",
    name: "Account A",
    address: "a@example.eu",
    connected: true,
    services: ["mail", "calendar"],
    configuration: {
      incoming: {
        host: "imap-a.example.eu",
        port: 1993,
        username: "user-a",
        security: "tls",
      },
      outgoing: {
        host: "smtp-a.example.eu",
        port: 2525,
        username: "sender-a",
        security: "starttls",
      },
    },
    capabilities: { read: true, send: true },
  };
  const payloads = [];
  await page.route("**/api/external-mail/accounts", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({
        json: {
          items: [account],
          providers: ["gmail", "imap"].map((provider) => ({
            provider,
            available: true,
            services: [
              { service: "mail", available: true },
              { service: "calendar", available: true },
            ],
          })),
        },
      });
    } else {
      payloads.push(route.request().postDataJSON());
      await route.fulfill({
        status: 422,
        json: { error: { code: "mail_tls" } },
      });
    }
  });
  await page
    .getByRole("button", { name: "Add email account", exact: true })
    .click();
  const dialog = page.getByRole("dialog");
  const field = (name) => dialog.locator(`[name="${name}"]`);
  await dialog
    .getByRole("button", { name: "Connect account", exact: true })
    .click();
  await expect(field("incoming_user")).toHaveValue("user-a");
  await expect(field("smtp_port")).toHaveValue("2525");
  const details = dialog.locator("details");
  for (const index of [0, 1]) {
    await details.nth(index).locator("summary").click();
    await details.nth(index).locator("summary").click();
  }
  await expect(field("incoming_user")).toHaveValue("user-a");
  await expect(field("smtp_port")).toHaveValue("2525");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await dialog
    .getByRole("button", { name: "Add email account", exact: true })
    .click();
  const failures = [];
  if (
    !(await dialog
      .getByRole("radio", { name: "Gmail", exact: true })
      .isChecked())
  )
    failures.push("new account retained the previous provider");
  else {
    await dialog.getByRole("button", { name: "Continue", exact: true }).click();
    const choices = dialog.locator('input[type="checkbox"]');
    if (
      (await choices.count()) !== 2 ||
      !(await choices.nth(0).isChecked()) ||
      (await choices.nth(1).isChecked())
    )
      failures.push("new account retained optional consent services");
    await dialog.getByRole("button", { name: "Back", exact: true }).click();
  }
  await dialog.getByRole("radio", { name: "IMAP", exact: true }).focus();
  await page.keyboard.press("Space");
  await dialog.getByRole("button", { name: "Continue", exact: true }).click();
  for (const name of [
    "display_name",
    "email",
    "incoming_host",
    "incoming_user",
    "incoming_password",
    "smtp_host",
    "smtp_user",
    "smtp_password",
  ]) {
    if ((await field(name).inputValue()) !== "")
      failures.push(`new account retained ${name}`);
  }
  if ((await field("incoming_port").inputValue()) !== "993")
    failures.push("new account did not reset IMAP port");
  if ((await field("smtp_port").inputValue()) !== "465")
    failures.push("new account retained SMTP port");
  if ((await field("smtp_security").inputValue()) !== "tls")
    failures.push("new account retained SMTP security");
  if (!(await details.evaluateAll((els) => els.every((el) => !el.open))))
    failures.push("new account advanced options not folded");
  await field("email").fill("b@example.eu");
  await field("incoming_host").fill("imap-b.example.eu");
  await field("incoming_password").fill("synthetic-password-b");
  await field("smtp_host").fill("smtp-b.example.eu");
  await dialog.getByRole("button", { name: "Review", exact: true }).click();
  await dialog
    .getByRole("button", { name: "Connect account", exact: true })
    .click();
  await expect.poll(() => payloads.length).toBe(1);
  const next = payloads[0];
  if (next.incoming.username !== "b@example.eu")
    failures.push("B request inherited A incoming username");
  if (next.outgoing.username !== "b@example.eu")
    failures.push("B request inherited A SMTP username");
  if (next.outgoing.port !== 465 || next.outgoing.security !== "tls")
    failures.push("B request inherited A SMTP transport");
  if (next.instance_id !== undefined || next.smtp_password !== undefined)
    failures.push("B request inherited account instance or SMTP password");
  if (
    next.address !== "b@example.eu" ||
    next.password !== "synthetic-password-b"
  )
    failures.push("B request lost its own credentials");
  expect(failures).toEqual([]);
}
