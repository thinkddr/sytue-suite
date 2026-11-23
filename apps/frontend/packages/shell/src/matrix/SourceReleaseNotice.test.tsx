/* SPDX-License-Identifier: Apache-2.0 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { sessionOwnerHeader } from '@sytue/config/session-owner';
import { storageScopeHeader } from '@sytue/config/matrix';
import { SourceReleaseNotice, type SourceReleaseNavigation } from './SourceReleaseNotice.js';
import { rememberPendingRelease, takePendingRelease } from './source-release.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const actor = '11111111-1111-4111-8111-111111111111';
const epoch = '12345678-1234-4234-8234-123456789abc';
const generation = 'a'.repeat(64);

async function until(check: () => boolean): Promise<void> {
  for (let i = 0; i < 100; i++) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw new Error('timeout waiting for condition');
}

function server(mode: string, release: { status: number; code?: string }[]) {
  const calls: Request[] = [];
  const fetchImpl = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = new Request(new URL(String(input), 'https://notes.test'), init);
    calls.push(request);
    const path = new URL(request.url).pathname;
    if (path.endsWith('/auth/session'))
      return Response.json({
        authenticated: true,
        owner: { v: 1, actor, tenant: 'acme', client_id: 'sytue-notes', generation },
      });
    if (path.endsWith('/private-storage/migration'))
      return Response.json({ mode, epoch: mode === 'legacy' ? '' : epoch });
    if (path.endsWith('/private-storage/migration/release')) {
      const next = release.shift() ?? { status: 500 };
      if (next.status === 200) mode = 'legacy';
      return Response.json(
        next.code ? { error: { code: next.code } } : { mode: 'legacy', released_epoch: epoch },
        { status: next.status },
      );
    }
    return new Response(null, { status: 404 });
  }) as typeof fetch;
  return { calls, fetchImpl };
}

function navigation() {
  const seen = { reloads: 0, assigned: [] as string[] };
  const nav: SourceReleaseNavigation = {
    reload: () => seen.reloads++,
    assign: (url) => seen.assigned.push(url),
    here: () => '/notes/app?x=1',
  };
  return { seen, nav };
}

async function mount(fetchImpl: typeof fetch, nav: SourceReleaseNavigation) {
  const root = document.createElement('div');
  document.body.appendChild(root);
  await act(() => {
    render(
      <SourceReleaseNotice
        actor={actor}
        tenant="acme"
        locale="en"
        product="notes"
        basePath="/notes"
        fetchImpl={fetchImpl}
        navigation={nav}
      />,
      root,
    );
  });
  return root;
}

function button(label: string): HTMLButtonElement {
  const found = [...document.querySelectorAll('button')].filter((b) => b.textContent === label);
  return found[found.length - 1] as HTMLButtonElement;
}

test('legacy source: nothing is offered', async () => {
  const { fetchImpl, calls } = server('legacy', []);
  const root = await mount(fetchImpl, navigation().nav);
  await until(() => calls.length >= 2);
  await new Promise((r) => setTimeout(r, 20));
  expect(root.textContent).toBe('');
  render(null, root);
});

test('frozen source: confirmation is required, reauthentication is asked and the release is repeated on return', async () => {
  sessionStorage.clear();
  const { fetchImpl, calls } = server('frozen', [
    { status: 401, code: 'insufficient_user_authentication' },
    { status: 200 },
  ]);
  const { seen, nav } = navigation();
  const root = await mount(fetchImpl, nav);
  await until(() => (root.textContent ?? '').includes('moved to encryption'));
  await act(() => button('Empty and go back to the server data').click());
  const danger = button('Empty and go back to the server data');
  expect(danger.disabled).toBe(true);
  await act(() => danger.click());
  expect(calls.some((c) => c.url.endsWith('/release'))).toBe(false);
  const box = document.querySelector('input[type="checkbox"]') as HTMLInputElement;
  await act(() => box.click());
  await act(() => button('Empty and go back to the server data').click());
  await until(() => seen.assigned.length === 1);
  expect(seen.assigned[0]).toBe('/notes/auth/login?reauth=1&returnTo=%2Fnotes%2Fapp%3Fx%3D1');
  const sent = calls.find((c) => c.url.endsWith('/release'))!;
  expect(sent.method).toBe('POST');
  expect(sent.headers.get(sessionOwnerHeader)).toBe(generation);
  expect(sent.headers.get(storageScopeHeader)).toBeTruthy();
  expect(JSON.parse(await sent.text())).toEqual({ epoch, discard_private_writes: true });
  render(null, root);

  // De vuelta de Sytue ID: se repite sola, una vez, y la app se recarga.
  const back = await mount(fetchImpl, nav);
  await until(() => seen.reloads === 1);
  expect(calls.filter((c) => c.url.endsWith('/release'))).toHaveLength(2);
  expect(takePendingRelease({ actor, tenant: 'acme', product: 'notes' })).toBeNull();
  render(null, back);
});

test('a confirmation for an earlier epoch is not replayed on a source frozen again', async () => {
  sessionStorage.clear();
  rememberPendingRelease(
    { actor, tenant: 'acme', product: 'notes' },
    '87654321-1234-4234-8234-123456789abc',
  );
  const { fetchImpl, calls } = server('frozen', [{ status: 200 }]);
  const { seen, nav } = navigation();
  const root = await mount(fetchImpl, nav);
  await until(() => (root.textContent ?? '').includes('moved to encryption'));
  await new Promise((r) => setTimeout(r, 30));
  expect(calls.some((c) => c.url.endsWith('/release'))).toBe(false);
  expect(seen.reloads).toBe(0);
  render(null, root);
});

test('a pending release from another account, product or a stale epoch is never replayed', () => {
  sessionStorage.clear();
  const owner = { actor, tenant: 'acme', product: 'notes' as const };
  rememberPendingRelease(owner, epoch, 1_000);
  expect(takePendingRelease({ ...owner, actor: 'someone-else' }, 1_000)).toBeNull();
  rememberPendingRelease(owner, epoch, 1_000);
  expect(takePendingRelease(owner, 1_000 + 11 * 60 * 1000)).toBeNull();
  rememberPendingRelease(owner, epoch, 1_000);
  expect(takePendingRelease(owner, 2_000)).toBe(epoch);
  expect(takePendingRelease(owner, 2_000)).toBeNull();
});
