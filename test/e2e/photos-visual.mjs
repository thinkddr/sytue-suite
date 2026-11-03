// SPDX-License-Identifier: Apache-2.0
// Capturas de PhotosApp y Albums REALES, sin sesión, red ni imágenes de personas.
// Runner remoto: bash test/e2e/correr-photos-visual.sh
import { chromium } from 'playwright';
import { expect } from '@playwright/test';
import { join } from 'node:path';
import { createReportDirectory, writeReport } from './lib/report-output.mjs';
import {
  assertFixtureTheme, describeWizardState, openWizardFixture, prepareFixtureTheme,
} from './lib/wizard-checks.mjs';
import { es } from '../../apps/frontend/apps/photos/src/i18n/messages/es.ts';

const fixture = new URL(
  process.env.PHOTOS_VISUAL_URL ?? 'http://127.0.0.1:4343/photos/prueba/dev/app',
);
if (
  !['127.0.0.1', 'localhost'].includes(fixture.hostname) ||
  !fixture.pathname.endsWith('/dev/app')
) {
  throw new Error('Photos visual only accepts the local DEV fixture.');
}
const out = createReportDirectory('photos-visual', process.env.PHOTOS_VISUAL_OUT);
const user = {
  id: 'photos-fixture',
  tenant: 'prueba',
  email: 'photos-fixture@sytue.test',
  name: 'Cuenta de prueba',
  emailVerified: true,
  roles: [],
  groups: [],
};
const stamp = '2026-09-24T10:00:00Z';
const shapes = [
  [1600, 900],
  [1200, 1600],
  [1200, 1200],
  [1600, 1200],
];
const photos = [9, 7, 2].flatMap((month) =>
  shapes.map(([width, height], index) => ({
    id: `foto-${month}-${index}`,
    name: `Imagen de prueba ${month}-${index}.svg`,
    contentType: 'image/svg+xml',
    sizeBytes: 2048,
    width,
    height,
    takenAt: `2026-${String(month).padStart(2, '0')}-${24 - index}T10:00:00Z`,
    takenAtFromExif: index !== 3,
    favorite: index === 1,
    description: `Imagen sintética ${month}-${index}`,
    tags: [],
    driveFileId: `drive-sintetico-${month}-${index}`,
  })),
);
const albumTitle =
  'Un recorrido de prueba con un nombre de álbum largo para comprobar la tarjeta en el teléfono';
const initialAlbums = [
  { id: 'album-portada', title: 'Camino de verano', coverAssetId: photos[0].id, count: 8 },
  { id: 'album-largo', title: albumTitle, coverAssetId: photos[1].id, count: 4 },
  { id: 'album-vacio', title: 'Álbum vacío', coverAssetId: null, count: 0 },
].map((album) => ({ ...album, createdAt: stamp, updatedAt: stamp }));

function image(photo) {
  const colors = ['#336b78', '#bb7957', '#657b57', '#766589'];
  const fill = colors[photos.indexOf(photo) % colors.length];
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${photo.width}" height="${photo.height}" viewBox="0 0 400 300" preserveAspectRatio="xMidYMid slice"><rect width="400" height="300" fill="${fill}"/><path d="M0 250L160 100L300 210L400 130V300H0Z" fill="#e1d6b9"/><circle cx="300" cy="65" r="24" fill="#f2e8ca"/></svg>`;
}
const json = (route, body, status = 200) =>
  route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });

async function mockBff(context, diagnostics) {
  let albums = structuredClone(initialAlbums);
  // Match PATH PREFIXES, never `includes('auth')`: Vite module URLs also contain auth.
  await context.route(
    (url) => url.origin === fixture.origin && url.pathname.startsWith('/photos/auth/'),
    (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/photos/auth/token')
        return json(route, { accessToken: 'photos-synthetic', expiresAt: Date.now() + 3600000 });
      if (path === '/photos/auth/session') return json(route, { authenticated: true, user });
      if (path === '/photos/auth/organizations') return json(route, { organizations: [] });
      diagnostics.unexpected.push({ method: route.request().method(), path });
      return json(route, { error: 'unmocked_fixture_auth' }, 501);
    },
  );
  await context.route(
    (url) => url.origin === fixture.origin && url.pathname.startsWith('/photos/api/'),
    async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const path = url.pathname.slice('/photos/api/'.length);
      diagnostics.requests.push({ method: request.method(), path });
      const variant = /^v1\/assets\/([^/]+)\/variants\/(miniatura|previa|grande)$/.exec(path);
      if (variant) {
        const photo = photos.find((candidate) => candidate.id === variant[1]);
        if (photo) return route.fulfill({ contentType: 'image/svg+xml', body: image(photo) });
      }
      if (path === 'v1/timeline') {
        const query = (url.searchParams.get('q') ?? '').toLowerCase();
        return json(route, {
          items: photos.filter((photo) => photo.name.toLowerCase().includes(query)),
        });
      }
      if (path === 'v1/memories') return json(route, { memories: [] });
      if (path === 'v1/settings')
        return json(route, { excludeAiBatch: false, canManageAiBatch: false });
      if (path === 'shared') return json(route, { items: [] });
      if (path === 'v1/albums' && request.method() === 'GET') return json(route, { items: albums });
      if (path === 'v1/albums' && request.method() === 'POST') {
        const title = request.postDataJSON().title.trim();
        const album = {
          id: 'album-creado',
          title,
          count: 0,
          coverAssetId: null,
          createdAt: stamp,
          updatedAt: stamp,
        };
        albums = [album, ...albums];
        return json(route, album, 201);
      }
      // The real shell uses the synthetic Hub under the SAME BFF prefix.
      if (path === 'fixture-hub/api/v1/notifications/stream') {
        return route.fulfill({ contentType: 'text/event-stream', body: ': synthetic fixture\n\n' });
      }
      if (path === 'fixture-hub/api/v1/notifications/unread_count')
        return json(route, { unread: 0, mail_unread: 0 });
      if (path === 'fixture-hub/api/v1/notifications') return json(route, { notifications: [] });
      if (path === 'fixture-hub/api/v1/notifications/prefs') return json(route, { prefs: [] });
      if (path === 'fixture-hub/api/v1/presence') return json(route, { presence: [] });
      if (path === 'fixture-hub/api/v1/apps') return json(route, { apps: ['photos'] });
      diagnostics.unexpected.push({ method: request.method(), path });
      return json(route, { error: 'unmocked_fixture_api' }, 501);
    },
  );
  // No production service or third-party host may receive even synthetic identity.
  await context.route(
    (url) => /^https?:$/.test(url.protocol) && url.origin !== fixture.origin,
    (route) => {
      diagnostics.unexpected.push({ outsideOrigin: new URL(route.request().url()).origin });
      return route.abort('blockedbyclient');
    },
  );
}

async function loadedImages(page, selector) {
  for (const img of await page.locator(selector).all()) {
    await img.scrollIntoViewIfNeeded();
    await expect
      .poll(() => img.evaluate((element) => element.complete && element.naturalWidth > 0))
      .toBe(true);
  }
  await page.evaluate(async () => {
    await document.fonts.ready;
    for (const element of document.querySelectorAll('*'))
      if (element.scrollTop) element.scrollTop = 0;
    window.scrollTo(0, 0);
  });
}

async function galleryBounds(page) {
  const geometry = await page.locator('ul.filas').evaluateAll((lists) =>
    lists.map((list) => {
      const parent = list.getBoundingClientRect();
      const tiles = [...list.children].map((tile) => tile.getBoundingClientRect());
      return {
        outside: tiles.some((rect) => rect.left < parent.left - 1 || rect.right > parent.right + 1),
        empty: tiles.some((rect) => rect.width <= 0 || rect.height <= 0),
        overlaps: tiles.some((a, i) =>
          tiles
            .slice(i + 1)
            .some(
              (b) =>
                a.left < b.right - 1 &&
                a.right > b.left + 1 &&
                a.top < b.bottom - 1 &&
                a.bottom > b.top + 1,
            ),
        ),
      };
    }),
  );
  expect(geometry).toHaveLength(3);
  for (const month of geometry)
    expect(month).toEqual({ outside: false, empty: false, overlaps: false });
  return geometry;
}

async function navigationBounds(page) {
  const nav = page.locator('main nav');
  const links = nav.getByRole('link');
  await expect(links).toHaveCount(6);
  expect(
    await links.evaluateAll((elements) => elements.map((element) => element.getAttribute('href'))),
  ).toEqual(['./', 'favorites', 'albums', 'shared', 'duplicates', 'trash']);
  const geometry = [];
  for (const link of await links.all()) {
    await link.scrollIntoViewIfNeeded();
    const measured = await link.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const frame = element.closest('nav').getBoundingClientRect();
      const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2);
      return {
        href: element.getAttribute('href'),
        height: rect.height,
        minimum: matchMedia('(pointer: coarse)').matches ? 44 : 24,
        contained:
          rect.top >= frame.top - 1 &&
          rect.bottom <= frame.bottom + 1 &&
          rect.left >= Math.max(frame.left, 0) - 1 &&
          rect.right <= Math.min(frame.right, innerWidth) + 1,
        receivesPointer: hit?.closest('a') === element,
      };
    });
    expect(measured.height).toBeGreaterThanOrEqual(measured.minimum);
    expect(measured.contained).toBe(true);
    expect(measured.receivesPointer).toBe(true);
    geometry.push(measured);
  }
  await nav.evaluate((element) => {
    element.scrollLeft = 0;
  });
  return geometry;
}

async function viewportBounds(page) {
  const geometry = await page.evaluate(() => {
    const nav = document.querySelector('main nav');
    const navRow = nav?.parentElement;
    const rows = [...document.querySelectorAll('main form')];
    if (navRow && getComputedStyle(navRow).flexWrap === 'wrap') rows.push(navRow);
    return {
      overflow: document.documentElement.scrollWidth > innerWidth + 1,
      collapsedNavigation:
        !nav ||
        nav.getBoundingClientRect().height < (matchMedia('(pointer: coarse)').matches ? 44 : 24),
      // Horizontal strips are intentional; their VIEWPORT still stays inside main.
      outside: [
        ...document.querySelectorAll('main nav, main input[type="search"], main form'),
      ].some((element) => {
        const rect = element.getBoundingClientRect();
        return rect.width > 0 && (rect.left < -1 || rect.right > innerWidth + 1);
      }),
      overlappingControls: rows.some((row) => {
        const controls = [...row.children]
          .map((element) => element.getBoundingClientRect())
          .filter((rect) => rect.width > 0 && rect.height > 0);
        return controls.some((a, i) =>
          controls
            .slice(i + 1)
            .some(
              (b) =>
                a.left < b.right - 1 &&
                a.right > b.left + 1 &&
                a.top < b.bottom - 1 &&
                a.bottom > b.top + 1,
            ),
        );
      }),
    };
  });
  expect(geometry).toEqual({
    overflow: false,
    collapsedNavigation: false,
    outside: false,
    overlappingControls: false,
  });
  return geometry;
}

async function galleryInteractions(page, id) {
  const first = page.getByRole('button', {
    name: es['gallery.open'].replace('{name}', photos[0].name),
    exact: true,
  });
  await first.click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  const mainImage = dialog.locator('img').first();
  await expect(mainImage).toHaveAttribute(
    'src',
    new RegExp(`/assets/${photos[0].id}/variants/grande`),
  );
  await expect
    .poll(() => mainImage.evaluate((element) => element.complete && element.naturalWidth > 0))
    .toBe(true);
  await dialog.getByRole('button', { name: es['viewer.next'], exact: true }).click();
  await expect(mainImage).toHaveAttribute(
    'src',
    new RegExp(`/assets/${photos[1].id}/variants/grande`),
  );
  await dialog.getByRole('button', { name: es['viewer.prev'], exact: true }).click();
  await expect(mainImage).toHaveAttribute(
    'src',
    new RegExp(`/assets/${photos[0].id}/variants/grande`),
  );
  await dialog.getByRole('button', { name: es['viewer.close'], exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await page.getByRole('button', { name: es['gallery.select'], exact: true }).click();
  const tile = page.getByRole('button', {
    name: es['gallery.selectTile'].replace('{name}', photos[0].name),
    exact: true,
  });
  await tile.click();
  await expect(tile).toHaveAttribute('aria-pressed', 'true');
  await expect(
    page.getByRole('status').filter({ hasText: es['gallery.selectedCount'].one }),
  ).toBeVisible();
  await page.getByRole('button', { name: es['gallery.selectMonth'], exact: true }).first().click();
  await expect(page.locator('ul.filas').first().locator('button[aria-pressed="true"]')).toHaveCount(
    4,
  );
  const toolbar = page.getByRole('toolbar', { name: es['gallery.select'], exact: true });
  await expect(toolbar).toBeVisible();
  await page.screenshot({ path: join(out, `${id}-selection.png`), animations: 'disabled' });
  await page.getByRole('button', { name: es['gallery.cancelSelection'], exact: true }).click();
  await expect(toolbar).toHaveCount(0);
  await expect(first).toBeVisible();
}

async function albumsBounds(page) {
  const covers = page.locator('main a[href*="/album?id="] > div:first-child');
  await expect(covers).toHaveCount(3);
  const geometry = await covers.evaluateAll((elements) => {
    const cards = elements.map((element) => element.closest('li').getBoundingClientRect());
    return elements.map((element, i) => {
      const rect = element.getBoundingClientRect();
      const card = cards[i];
      return {
        ratio: rect.width / rect.height,
        outside:
          rect.left < card.left - 1 ||
          rect.right > card.right + 1 ||
          card.left < -1 ||
          card.right > innerWidth + 1,
        overlappingCards: cards.some(
          (other, j) =>
            i !== j &&
            card.left < other.right - 1 &&
            card.right > other.left + 1 &&
            card.top < other.bottom - 1 &&
            card.bottom > other.top + 1,
        ),
      };
    });
  });
  for (const cover of geometry) {
    expect(cover.ratio).toBeCloseTo(4 / 3, 2);
    expect(cover.outside).toBe(false);
    expect(cover.overlappingCards).toBe(false);
  }
  const empty = page.locator('main a[href$="id=album-vacio"]');
  await expect(empty.locator('img')).toHaveCount(0);
  await expect(empty).toContainText('0 fotos');
  const long = page.locator('main a[href$="id=album-largo"]');
  await expect(long).toContainText(albumTitle);
  await expect(long).toHaveAttribute('href', '/photos/prueba/album?id=album-largo');
  return geometry;
}

const results = [];
let browser;
try {
  browser = await chromium.launch({ headless: true });
  for (const width of [320, 390, 1280])
    for (const theme of ['light', 'dark'])
      for (const view of ['gallery', 'albums']) {
        const id = `${view}-${width}-${theme}`;
        const diagnostics = {
          requests: [],
          unexpected: [],
          errors: [],
          failedResponses: [],
          failedRequests: [],
        };
        const result = { id, width, theme, view, ok: false, diagnostics };
        results.push(result);
        const context = await browser.newContext({
          viewport: { width, height: width < 600 ? 844 : 900 },
          colorScheme: theme,
          locale: 'es-ES',
          timezoneId: 'UTC',
          hasTouch: width < 600,
          isMobile: width < 600,
        });
        await prepareFixtureTheme(context, theme);
        const page = await context.newPage();
        try {
          page.on('pageerror', (error) => diagnostics.errors.push(error.message));
          page.on('response', (response) => {
            const url = new URL(response.url());
            if (url.origin === fixture.origin && response.status() >= 400) {
              diagnostics.failedResponses.push({ path: url.pathname, status: response.status() });
            }
          });
          page.on('requestfailed', (request) => {
            const url = new URL(request.url());
            if (url.origin === fixture.origin) {
              diagnostics.failedRequests.push({
                path: url.pathname,
                error: request.failure()?.errorText,
              });
            }
          });
          await mockBff(context, diagnostics);
          const url = new URL(fixture);
          url.searchParams.set('vista', view);
          const ready = view === 'gallery' ? 'ul.filas img' : 'main a[href*="/album?id="]';
          // La espera de arranque es única; abajo se comprueban TODAS las imágenes y tarjetas.
          await openWizardFixture(page, url.href, `${ready} >> nth=0`);
          await assertFixtureTheme(page, theme);
          await expect(page.locator('html')).toHaveAttribute('data-app', 'photos');
          await expect(page.locator('main nav [aria-current="page"]')).toHaveText(
            es[view === 'gallery' ? 'nav.photos' : 'nav.albums'],
          );
          await loadedImages(
            page,
            view === 'gallery' ? 'ul.filas img' : 'main a[href*="/album?id="] img',
          );
          result.viewport = await viewportBounds(page);
          result.navigation = await navigationBounds(page);
          if (view === 'gallery') {
            await expect(page.locator('ul.filas > li')).toHaveCount(photos.length);
            result.gallery = await galleryBounds(page);
          } else result.covers = await albumsBounds(page);
          result.capture = `${id}.png`;
          await page.screenshot({ path: join(out, result.capture), animations: 'disabled' });
          if (view === 'gallery') await galleryInteractions(page, id);
          else {
            const title = 'Álbum creado en la prueba';
            await page.getByRole('textbox', { name: es['albums.title'], exact: true }).fill(title);
            await page.getByRole('button', { name: es['albums.create'], exact: true }).click();
            await expect(page.locator('main a[href$="id=album-creado"]')).toContainText(title);
          }
          await viewportBounds(page);
          expect(diagnostics.unexpected).toEqual([]);
          expect(diagnostics.errors).toEqual([]);
          result.ok = true;
        } catch (failure) {
          result.error = String(failure);
          result.state = await describeWizardState({ page }).catch(() => null);
          await page
            .screenshot({ path: join(out, `${id}-failure.png`), animations: 'disabled' })
            .catch(() => {});
          console.error(`${id}: ${result.error}`);
        } finally {
          await context.close();
          writeReport(join(out, `${id}.json`), JSON.stringify(result, null, 2));
        }
      }
} catch (failure) {
  results.push({ id: 'startup', ok: false, error: String(failure) });
} finally {
  await browser?.close();
  writeReport(join(out, 'results.json'), JSON.stringify({ synthetic: true, results }, null, 2));
}
if (results.some((result) => !result.ok)) process.exitCode = 1;
