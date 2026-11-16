/* SPDX-License-Identifier: Apache-2.0 */
import {
  decodeStorageScopeHeader,
  storageScopeHeader,
  legacySourceCursor,
} from "../../../apps/frontend/packages/config/src/matrix.ts";
/** Shared artificial retained-source route; imports and UI use the same HTTP contract. */
export function privateDriveSourceRoute(source) {
  return async (request) => {
    const url = new URL(request.url);
    if (url.pathname === "/source-stats") return Response.json(source.calls);
    const base = "/api/matrix/storage/migration";
    if (!url.pathname.startsWith(base)) return undefined;
    try {
      const scope = decodeStorageScopeHeader(
          request.headers.get(storageScopeHeader),
        ),
        tail = url.pathname.slice(base.length).split("/").filter(Boolean);
      if (tail.length === 0) {
        if (request.method === "POST")
          return Response.json(
            await source.source.beginPrivateStorageMigration(
              scope,
              (await request.json()).epoch,
            ),
          );
        return Response.json(await source.source.privateStorageState(scope));
      }
      if (tail[1] === "rows") {
        const value = url.searchParams.get("after"),
          after = value
            ? legacySourceCursor(
                JSON.parse(Buffer.from(value, "base64url").toString("utf8")),
              )
            : undefined;
        return Response.json(
          await source.source.privateStorageSourcePage(scope, tail[0], after),
        );
      }
      if (
        tail[1] === "blobs" &&
        (tail[2] === "file" || tail[2] === "version")
      ) {
        const key = tail[2] + "/" + tail[3],
          size = source.blobs.get(key)?.length;
        if (size === undefined) return new Response(null, { status: 404 });
        return await source.source.openPrivateStorageSource(
          scope,
          tail[0],
          tail[2],
          tail[3],
          size,
        );
      }
      return new Response(null, { status: 404 });
    } catch {
      return Response.json(
        { error: { code: "artificial_source_rejected" } },
        { status: 400 },
      );
    }
  };
}
