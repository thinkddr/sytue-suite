/* SPDX-License-Identifier: Apache-2.0 */
/** Actual browser UI and byte transport; synthetic authenticated record/SDK lifecycle port. */
import { render } from "preact";
import { useState } from "preact/hooks";
import { I18nProvider } from "../../apps/frontend/packages/i18n/src/preact";
import {
  ThemeProvider,
  ToastProvider,
} from "../../apps/frontend/packages/ui/src/index";
import { MatrixDeviceGate } from "../../apps/frontend/packages/shell/src/matrix";
import { DriveClient } from "../../apps/frontend/packages/api-clients/src/drive";
import type {
  MatrixBootstrap,
  SuiteMatrix,
  StorageBlobPort,
} from "../../apps/frontend/packages/offline/src/matrix";
import { storageWriteFixture } from "../../apps/frontend/packages/offline/src/matrix/storage-write-fixture";
import { createPrivateDriveGate } from "../../apps/frontend/apps/drive/src/client/private-drive";
import { PrivateDriveProvider } from "../../apps/frontend/apps/drive/src/client/private-browser";
import { PrivateFileBrowser } from "../../apps/frontend/apps/drive/src/components/PrivateFileBrowser";
const state = {
  stage: "fixture",
  ok: false,
  folders: 0,
  uploaded: false,
  preview: false,
  restored: false,
  favorites: false,
  versions: false,
  invalidLink: false,
  accessRequest: false,
  closed: false,
  reason: "",
  dom: "",
  preparation: "",
};
function Explorer() {
  const [section, setSection] = useState("my");
  const [generation, setGeneration] = useState(0);
  return (
    <>
      <button
        data-private-reset
        onClick={() => {
          history.replaceState(null, "", "/");
          setSection("my");
          setGeneration((value) => value + 1);
        }}
      >
        Reset fixture link
      </button>
      <button
        data-private-invalid
        onClick={() => {
          history.replaceState(
            null,
            "",
            "/?folder=00000000-0000-4000-8000-000000000099",
          );
          setSection("my");
          setGeneration((value) => value + 1);
        }}
      >
        Invalid fixture link
      </button>
      <button data-private-my onClick={() => setSection("my")}>
        My Drive fixture navigation
      </button>
      <button data-private-favorites onClick={() => setSection("favorites")}>
        Favorites fixture navigation
      </button>
      <button data-private-recent onClick={() => setSection("recent")}>
        Recent fixture navigation
      </button>
      <button data-private-trash onClick={() => setSection("trash")}>
        Trash fixture navigation
      </button>
      <PrivateFileBrowser
        key={generation}
        section={section}
        onOpenFolder={() => setSection("my")}
      />
    </>
  );
}
Object.assign(window, { privateDriveUIProbe: state });
async function until(check: () => boolean | Promise<boolean>) {
  const deadline = Date.now() + 30000;
  while (!(await check())) {
    if (state.preparation) throw new Error(state.preparation);
    if (Date.now() > deadline)
      throw new Error("UI stage timeout: " + state.stage);
    await new Promise((done) => setTimeout(done, 20));
  }
}
function button(label: string) {
  const node = Array.from(document.querySelectorAll("button")).find(
    (entry) => entry.textContent?.trim() === label,
  );
  if (!node) throw new Error("missing UI action: " + label);
  return node;
}
function inputValue(input: HTMLInputElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
}
void (async () => {
  const vector = await (await fetch("/fixture")).json();
  if (vector.artificial !== true || vector.scope.subject !== "artificial")
    throw new Error("disposable UI fixture required");
  const scope = vector.scope,
    device = storageWriteFixture(scope);
  const bootstrap: MatrixBootstrap = {
    actor: scope.subject,
    tenant: scope.tenant,
    homeserver_url: "https://matrix.example",
    matrix_user_id: "@artificial:matrix.example",
    controller_user_id: "@controller:matrix.example",
    sso_provider_id: "sytue",
    channels: [],
    encryption: "m.megolm.v1.aes-sha2",
    storage_product: "drive",
    storage_room_id: "!private:matrix.test",
    storage_room_ids: ["!private:matrix.test"],
  };
  const gate = createPrivateDriveGate({
    actor: scope.subject,
    tenant: scope.tenant,
    source: () => new DriveClient({ baseUrl: location.origin }),
  });
  const prepare = gate.host.prepare!;
  gate.host.prepare = async (...args) => {
    try {
      await prepare(...args);
    } catch (error) {
      state.preparation = String(error);
      throw error;
    }
  };
  let sdk: SuiteMatrix | undefined;
  gate.host.bootstrap = async () => bootstrap;
  gate.host.open = async (_bootstrap, ui) => {
    let closed = false;
    const instance = {
      bootstrap,
      get isClosed() {
        return closed;
      },
      client: { on: () => {} },
      trusted: async () => true,
      devices: async () => [
        {
          id: "DEVICE",
          name: "Artificial web device",
          verified: true,
          current: true,
        },
      ],
      unverifiedRecipients: async () => {
        throw new Error("private peer enumeration");
      },
      canVerifyUser: () => true,
      storage: (blobs?: StorageBlobPort) =>
        device.repository(undefined, undefined, blobs),
      close: () => {
        closed = true;
        ui.cancelPending?.();
      },
    };
    sdk = instance as unknown as SuiteMatrix;
    return sdk;
  };
  const root = document.body.appendChild(document.createElement("div"));
  // Functional fixture layout only; this probe does not validate production CSS or screenshots.
  const css = document.head.appendChild(document.createElement("style"));
  css.textContent =
    '[role="list"]{height:450px;width:720px;overflow:auto}[role="listitem"]{height:53px}';
  try {
    state.stage = "mount-and-import";
    history.replaceState(
      null,
      "",
      "/?request=artificial-request&folder=00000000-0000-4000-8000-000000000099",
    );
    render(
      <I18nProvider locale="en">
        <ThemeProvider>
          <ToastProvider>
            <MatrixDeviceGate
              actor={scope.subject}
              tenant={scope.tenant}
              locale="en"
              host={gate.host}
            >
              {(client) => (
                <PrivateDriveProvider
                  session={gate.current(client)}
                  baseURL={location.origin}
                >
                  <Explorer />
                </PrivateDriveProvider>
              )}
            </MatrixDeviceGate>
          </ToastProvider>
        </ThemeProvider>
      </I18nProvider>,
      root,
    );
    state.stage = "access-request-link";
    await until(
      () => root.querySelector("h1")?.textContent === "Edit access request",
    );
    if (
      root.querySelector('[aria-label="Files and folders"]') ||
      root.querySelector("input[type=file]")
    )
      throw new Error(
        "access request opened private content or upload actions",
      );
    button("My drive").click();
    await until(
      () => root.querySelector('[aria-label="Files and folders"]') !== null,
    );
    if (
      new URL(location.href).searchParams.has("request") ||
      new URL(location.href).searchParams.has("folder")
    )
      throw new Error("request return retained an untrusted destination");
    state.accessRequest = true;
    root.querySelector<HTMLButtonElement>("[data-private-invalid]")!.click();
    state.stage = "invalid-link";
    await until(
      () =>
        root.textContent?.includes("The folder could not be loaded") === true,
    );
    if (
      !button("New folder").disabled ||
      !button("Upload").disabled ||
      root.querySelector('[aria-label="Files and folders"]')
    )
      throw new Error(
        "invalid private destination must not silently open the root",
      );
    state.invalidLink = true;
    root.querySelector<HTMLButtonElement>("[data-private-recent]")!.click();
    await until(
      () =>
        root.querySelector("h1")?.textContent === "Recent" &&
        root.querySelector('[aria-label="Files and folders"]') !== null,
    );
    root.querySelector<HTMLButtonElement>("[data-private-reset]")!.click();
    await until(
      () =>
        root.querySelector('[aria-label="Files and folders"]') !== null &&
        Array.from(root.querySelectorAll("button")).some(
          (entry) =>
            entry.textContent?.trim() === "New folder" && !entry.disabled,
        ),
    );
    state.stage = "create-folder-dialog";
    button("New folder").click();
    await until(
      () => document.querySelector('input[aria-label="Folder name"]') !== null,
    );
    inputValue(
      document.querySelector<HTMLInputElement>(
        'input[aria-label="Folder name"]',
      )!,
      "Browser 100%",
    );
    await until(() => !button("Create").disabled);
    state.stage = "create-folder-commit";
    button("Create").click();
    await until(() => root.textContent?.includes("Browser 100%") === true);
    state.folders++;
    root
      .querySelector<HTMLElement>(
        '[aria-label="Open the folder Browser 100%"]',
      )!
      .click();
    await until(
      () =>
        root.querySelector("nav")?.textContent?.includes("Browser 100%") ===
        true,
    );

    state.stage = "upload";
    const transfer = new DataTransfer();
    transfer.items.add(
      new File(["ARTIFICIAL PRIVATE UI CONTENT"], "browser-private.txt", {
        type: "text/plain",
      }),
    );
    const picker = root.querySelector<HTMLInputElement>("input[type=file]")!;
    picker.files = transfer.files;
    picker.dispatchEvent(new Event("change", { bubbles: true }));
    await until(
      () =>
        root.querySelector('[aria-label="Open browser-private.txt"]') !== null,
    );
    state.uploaded = true;
    state.stage = "favorite-file";
    root
      .querySelector<HTMLButtonElement>(
        '[aria-label="Actions for browser-private.txt"]',
      )!
      .click();
    await until(() =>
      Array.from(document.querySelectorAll("button")).some(
        (entry) => entry.textContent?.trim() === "Add to favourites",
      ),
    );
    button("Add to favourites").click();
    await until(
      () =>
        root.querySelector('[aria-label="Open browser-private.txt"]') !==
          null && !root.querySelector('[role="menu"]'),
    );
    state.stage = "favorite-folder";
    root.querySelector<HTMLButtonElement>("[data-private-my]")!.click();
    // Already in My Drive: use the breadcrumb to return to the private root.
    button("My drive").click();
    await until(
      () =>
        root.querySelector('[aria-label="Actions for Browser 100%"]') !== null,
    );
    root
      .querySelector<HTMLButtonElement>(
        '[aria-label="Actions for Browser 100%"]',
      )!
      .click();
    await until(() =>
      Array.from(document.querySelectorAll("button")).some(
        (entry) => entry.textContent?.trim() === "Add to favourites",
      ),
    );
    button("Add to favourites").click();
    root.querySelector<HTMLButtonElement>("[data-private-favorites]")!.click();
    await until(
      () =>
        root.querySelector("h1")?.textContent === "Starred" &&
        root
          .querySelector('[aria-label="Open browser-private.txt"]')
          ?.textContent?.includes("Browser 100%") === true &&
        root.querySelector('[aria-label="Open the folder Browser 100%"]') !==
          null,
    );
    state.stage = "open-favorite-folder";
    root
      .querySelector<HTMLElement>(
        '[aria-label="Open the folder Browser 100%"]',
      )!
      .click();
    await until(
      () =>
        root.querySelector("nav")?.textContent?.includes("Browser 100%") ===
          true &&
        root.querySelector('[aria-label="Open browser-private.txt"]') !== null,
    );
    if (
      new URL(location.href).searchParams.get("section") !== "my" ||
      !new URL(location.href).searchParams.get("folder")
    )
      throw new Error(
        "favorite folder must open its opaque private destination",
      );
    state.stage = "replace-private-content";
    const replacement = new DataTransfer();
    replacement.items.add(
      new File(
        ["ARTIFICIAL PRIVATE UI REPLACEMENT".padEnd(2048, ".")],
        "browser-private.txt",
        { type: "text/plain" },
      ),
    );
    const replacePicker =
      root.querySelector<HTMLInputElement>("input[type=file]")!;
    replacePicker.files = replacement.files;
    replacePicker.dispatchEvent(new Event("change", { bubbles: true }));
    await until(
      () =>
        root
          .querySelector('[aria-label="Open browser-private.txt"]')
          ?.textContent?.includes("2.0 KB") === true &&
        !button("Upload").disabled,
    );
    state.stage = "private-version-dialog";
    root
      .querySelector<HTMLButtonElement>(
        '[aria-label="Actions for browser-private.txt"]',
      )!
      .click();
    await until(() =>
      Array.from(document.querySelectorAll("button")).some(
        (entry) => entry.textContent?.trim() === "Version history",
      ),
    );
    button("Version history").click();
    await until(
      () =>
        document.body.textContent?.includes("Version 2") === true &&
        document.body.textContent?.includes("Version 1") === true,
    );
    if (
      document.querySelector(
        'a[href*="/api/content"],a[href*="/api/version-content"]',
      )
    )
      throw new Error("private versions must not expose legacy download links");
    state.stage = "download-private-archive";
    document
      .querySelector<HTMLButtonElement>('[aria-label="Download version 1"]')!
      .click();
    await until(
      async () => (await (await fetch("/ui-stats")).json()).downloads === 1,
    );
    state.stage = "restore-private-archive";
    const historyRow = Array.from(document.querySelectorAll("li")).find(
      (node) => node.textContent?.includes("Version 1"),
    )!;
    Array.from(historyRow.querySelectorAll("button"))
      .find((node) => node.textContent?.trim() === "Restore")!
      .click();
    await until(() =>
      Array.from(historyRow.querySelectorAll("button")).some(
        (node) => node.textContent?.trim() === "Restore this version",
      ),
    );
    Array.from(historyRow.querySelectorAll("button"))
      .find((node) => node.textContent?.trim() === "Restore this version")!
      .click();
    await until(
      () =>
        !document.querySelector('[aria-label="Download version 1"]') &&
        root.querySelector('[aria-label="Open browser-private.txt"]') !== null,
    );
    state.versions = true;
    state.stage = "remove-favorite";
    root.querySelector<HTMLButtonElement>("[data-private-favorites]")!.click();
    await until(
      () =>
        root.querySelector("h1")?.textContent === "Starred" &&
        root.querySelector('[aria-label="Actions for browser-private.txt"]') !==
          null,
    );
    root
      .querySelector<HTMLButtonElement>(
        '[aria-label="Actions for browser-private.txt"]',
      )!
      .click();
    await until(() =>
      Array.from(document.querySelectorAll("button")).some(
        (entry) => entry.textContent?.trim() === "Remove from favourites",
      ),
    );
    button("Remove from favourites").click();
    await until(
      () =>
        !root.querySelector('[aria-label="Open browser-private.txt"]') &&
        root.querySelector('[aria-label="Open the folder Browser 100%"]') !==
          null,
    );
    state.favorites = true;
    state.stage = "recent";
    root.querySelector<HTMLButtonElement>("[data-private-recent]")!.click();
    await until(
      () =>
        root.querySelector("h1")?.textContent === "Recent" &&
        root
          .querySelector('[aria-label="Open browser-private.txt"]')
          ?.textContent?.includes("Browser 100%") === true,
    );
    state.stage = "trash";
    root
      .querySelector<HTMLButtonElement>(
        '[aria-label="Actions for browser-private.txt"]',
      )!
      .click();
    await until(() =>
      Array.from(document.querySelectorAll("button")).some(
        (entry) => entry.textContent?.trim() === "Move to trash",
      ),
    );
    button("Move to trash").click();
    await until(
      () => !root.querySelector('[aria-label="Open browser-private.txt"]'),
    );
    root.querySelector<HTMLButtonElement>("[data-private-trash]")!.click();
    await until(
      () =>
        root.querySelector("h1")?.textContent === "Trash" &&
        root.textContent?.includes("browser-private.txt") === true,
    );
    if (
      Array.from(document.querySelectorAll("button")).some(
        (entry) => entry.textContent?.trim() === "Delete permanently",
      )
    )
      throw new Error("private purge must remain unavailable");
    const trashNames = () =>
      Array.from(root.querySelectorAll<HTMLElement>("li span[title]")).map(
        (node) => node.title,
      );
    const retainedTrash = trashNames().filter(
      (name) => name !== "browser-private.txt",
    );
    state.stage = "trash-restore";
    const row = root
      .querySelector<HTMLElement>('[title="browser-private.txt"]')!
      .closest("li")!;
    Array.from(row.querySelectorAll("button"))
      .find((entry) => entry.textContent?.trim() === "Restore")!
      .click();
    await until(
      () =>
        !trashNames().includes("browser-private.txt") &&
        retainedTrash.every((name) => trashNames().includes(name)),
    );
    state.restored = true;
    root.querySelector<HTMLButtonElement>("[data-private-recent]")!.click();
    await until(
      () =>
        root.querySelector("h1")?.textContent === "Recent" &&
        root
          .querySelector('[aria-label="Open browser-private.txt"]')
          ?.textContent?.includes("Browser 100%") === true,
    );
    state.stage = "preview";
    root
      .querySelector<HTMLElement>('[aria-label="Open browser-private.txt"]')!
      .click();
    await until(() =>
      Array.from(document.querySelectorAll("pre")).some(
        (node) => node.textContent === "ARTIFICIAL PRIVATE UI CONTENT",
      ),
    );
    state.preview = true;
    const link = new URL(location.href);
    if (
      !link.searchParams.get("file") ||
      link.href.includes("browser-private.txt")
    )
      throw new Error("private link must contain only an opaque ID");
    const stats = await (await fetch("/ui-stats")).json();
    if (stats.unexpected.length || stats.plaintext || stats.writes < 3)
      throw new Error(
        "unexpected content request or missing opaque upload " +
          JSON.stringify(stats),
      );
    state.stage = "sdk-close";
    sdk!.close();
    await until(
      () =>
        !document.body.textContent?.includes("ARTIFICIAL PRIVATE UI CONTENT") &&
        !document.body.textContent?.includes("browser-private.txt") &&
        !root.querySelector('[aria-label="Files and folders"]'),
    );
    state.closed = true;
    state.stage = "complete";
    state.ok = true;
  } catch (error) {
    state.dom = document.body.innerText.slice(-5000);
    throw error;
  } finally {
    render(null, root);
    root.remove();
    css.remove();
    await device.close();
  }
})().catch((error: unknown) => {
  state.stage = "failed:" + state.stage;
  state.reason = String(error);
});
