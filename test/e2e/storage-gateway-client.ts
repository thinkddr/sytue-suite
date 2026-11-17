/* SPDX-License-Identifier: Apache-2.0 */
/** Synthetic opaque bytes only. Matrix encryption is exercised by storage-file.mjs. */
import {
  DriveClient,
  HttpError,
} from "../../apps/frontend/packages/api-clients/src/index.js";
declare global {
  interface Window {
    storageGatewayProbe?: { ok: boolean; bytes?: number; error?: string };
  }
}
const originalFetch = globalThis.fetch.bind(globalThis);
globalThis.fetch = async (input, init) => {
  const headers = new Headers(init?.headers);
  if (headers.has("authorization")) throw new Error("browser_received_bearer");
  return originalFetch(input, init);
};
try {
  const scope = { tenant: "acme", subject: "person", product: "mail" } as const,
    id = "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
    bytes = new Uint8Array(8 << 20);
  for (let i = 0; i < bytes.length; i++) bytes[i] = i % 251;
  const sdk = new DriveClient({ baseUrl: location.origin });
  const receipt = await sdk.putCipherBlob(scope, id, bytes);
  if (receipt.size_bytes !== bytes.length) throw new Error("receipt_size");
  const metadata = await sdk.cipherBlob(scope, id);
  if (metadata.sha256 !== receipt.sha256) throw new Error("receipt_changed");
  const downloaded = await sdk.readCipherBlob(scope, id);
  if (
    downloaded.length !== bytes.length ||
    downloaded.some((byte, index) => byte !== bytes[index])
  )
    throw new Error("ciphertext_changed");
  await fetch("/switch", { method: "POST" });
  for (const attempt of [
    () => sdk.readCipherBlob(scope, id),
    () => sdk.deleteCipherBlob(scope, id),
    () => sdk.putCipherBlob(scope, id, bytes),
  ]) {
    let denied = false;
    try {
      await attempt();
    } catch (error) {
      denied = error instanceof HttpError && error.status === 409;
    }
    if (!denied) throw new Error("scope_switch_accepted");
  }
  window.storageGatewayProbe = { ok: true, bytes: bytes.length };
} catch (error) {
  window.storageGatewayProbe = {
    ok: false,
    error: error instanceof Error ? error.message : "gateway_failed",
  };
}
