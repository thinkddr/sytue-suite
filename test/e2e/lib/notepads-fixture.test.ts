// SPDX-License-Identifier: Apache-2.0
import { expect, test } from "bun:test";
import { routeNotepadsFixture } from "./notepads-fixture.mjs";

const fixtureURL = "http://127.0.0.1:4343/notes/prueba/dev/app";
type Endpoint = { service: string; path: string };
type Route = { request: () => { url: () => string }; continue: () => void };
type Handler = (route: Route) => unknown;

async function routing() {
  let handler: Handler | undefined;
  const handled: Endpoint[] = [];
  const continued: string[] = [];
  await routeNotepadsFixture(
    {
      async route(_pattern: string, callback: Handler) {
        handler = callback;
      },
    },
    fixtureURL,
    async (_route: Route, endpoint: Endpoint) => {
      handled.push(endpoint);
    },
  );
  return {
    handled,
    continued,
    async request(path: string) {
      if (!handler) throw new Error("El interceptor del fixture debe estar instalado");
      const url = new URL(path, fixtureURL).href;
      await handler({
        request: () => ({ url: () => url }),
        continue: () => {
          continued.push(path);
        },
      });
    },
  };
}

test("Notepads deja pasar los módulos de Vite aunque sus rutas contengan auth o api", async () => {
  const routes = await routing();
  const modules = [
    "/notes/@vite/client",
    "/notes/@fs/__w/sytue-one/apps/frontend/packages/auth/src/index.ts",
    "/notes/@fs/__w/sytue-one/apps/frontend/packages/auth/src/client/index.ts?t=42",
    "/notes/@fs/__w/sytue-one/apps/frontend/apps/notes/src/pages/api/notebooks/index.ts",
    "/notes/@id/@sytue/auth/client",
    "/notes/node_modules/.vite/deps/@sytue_auth_client.js?v=42",
    "/notes/src/client/notesApi.ts",
    "/notes/prueba/dev/app",
  ];
  for (const path of modules) {
    await routes.request(path);
  }
  expect(routes.continued).toEqual(modules);
  expect(routes.handled).toEqual([]);
});

test("Notepads simula únicamente sus namespaces públicos del mismo origen", async () => {
  const routes = await routing();
  for (const path of [
    "/notes/api/notebooks",
    "/notes/api/notebooks/cuaderno-proyecto/notes?sort=position",
    "/notes/api/notes/nota-0/move",
    "/notes/auth/token",
    "/auth/token",
    "/auth/organizations",
    "/hub/api/v1/notifications",
  ]) {
    await routes.request(path);
  }
  expect(routes.handled).toEqual([
    { service: "notes", path: "notebooks" },
    { service: "notes", path: "notebooks/cuaderno-proyecto/notes" },
    { service: "notes", path: "notes/nota-0/move" },
    { service: "auth", path: "token" },
    { service: "auth", path: "token" },
    { service: "auth", path: "organizations" },
    { service: "hub", path: "api/v1/notifications" },
  ]);
  const others = [
    "/api/notebooks",
    "/other/auth/token",
    "/notes/apiary/module.js",
    "/notes/authors/module.js",
    "https://example.invalid/notes/api/notebooks",
  ];
  for (const path of others) {
    await routes.request(path);
  }
  expect(routes.continued).toEqual(others);
  expect(routes.handled).toHaveLength(7);
});
