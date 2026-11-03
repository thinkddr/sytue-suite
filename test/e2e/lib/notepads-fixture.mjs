// SPDX-License-Identifier: Apache-2.0

// Solo endpoints públicos del fixture. Las URL @fs de Vite también contienen
// /auth/ y /api/: responderles con JSON impide hidratar la isla de Notepads.
export function notepadsFixtureEndpoint(pathname) {
  for (const [prefix, service] of [
    ['/notes/api/', 'notes'],
    ['/notes/auth/', 'auth'],
    ['/auth/', 'auth'],
    ['/hub/', 'hub'],
  ]) {
    if (pathname.startsWith(prefix)) return { service, path: pathname.slice(prefix.length) };
  }
  return null;
}

export function routeNotepadsFixture(page, fixtureURL, respond) {
  const origin = new URL(fixtureURL).origin;
  return page.route('**/*', (route) => {
    const url = new URL(route.request().url());
    const endpoint = url.origin === origin ? notepadsFixtureEndpoint(url.pathname) : null;
    return endpoint ? respond(route, endpoint) : route.continue();
  });
}
