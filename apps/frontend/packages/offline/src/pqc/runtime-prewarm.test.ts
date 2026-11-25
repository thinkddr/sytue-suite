/* SPDX-License-Identifier: Apache-2.0 */
/**
 * EL NÚCLEO EMPIEZA A BAJAR A LA VEZ QUE EL BOOTSTRAP (roadmap, «El núcleo empieza a
 * bajar tarde», 2026-11-24): `prewarmClientCore` arranca el worker antes de que exista
 * el arrendamiento (el de la bóveda, `vault`), con los resúmenes que el servidor dejó
 * en la página, y el primer
 * `PqcRuntime` que lo necesita lo ADOPTA (no crea otro). Un worker sigue siendo de un
 * solo arrendamiento: se adopta una vez y, si nadie lo adopta, se termina.
 *
 * Visto en ROJO: con `adoptWarm` devolviendo siempre `null` se crean dos workers.
 */
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { PqcRuntime, pageClientCoreDigests, prewarmClientCore } from './runtime.js';

const wasm = 'a'.repeat(64);
const runtime = 'b'.repeat(64);

class FakeWorker {
  static all: FakeWorker[] = [];
  posted: unknown[] = [];
  terminated = false;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onmessageerror: (() => void) | null = null;
  constructor(
    readonly url: URL,
    readonly options: WorkerOptions,
  ) {
    FakeWorker.all.push(this);
  }
  postMessage(value: unknown): void {
    this.posted.push(value);
    const message = value as { id?: number; request?: unknown };
    // Contesta a las peticiones como el núcleo: con lo que se le pidió.
    if (typeof message.id === 'number')
      queueMicrotask(() =>
        this.emit({ type: 'result', id: message.id, ok: true, value: message.request }),
      );
  }
  terminate(): void {
    this.terminated = true;
  }
  emit(data: unknown): void {
    this.onmessage?.({ data } as MessageEvent);
  }
}

const original = {
  Worker: (globalThis as { Worker?: unknown }).Worker,
  document: (globalThis as { document?: unknown }).document,
  location: (globalThis as { location?: unknown }).location,
};

beforeEach(() => {
  FakeWorker.all = [];
  Object.assign(globalThis, {
    Worker: FakeWorker,
    location: { origin: 'https://drive.sytue.test' },
    document: {
      querySelector: (selector: string) =>
        selector === 'meta[name="sytue-client-core"]'
          ? { getAttribute: () => `${wasm} ${runtime}` }
          : null,
    },
  });
});
afterEach(() => {
  Object.assign(globalThis, original);
});

const lease = () => {
  const life = new AbortController();
  return { life, lease: { signal: life.signal, assertOpen: () => life.signal.throwIfAborted() } };
};

test('los resúmenes de la página: solo dos sha256 bien formados', () => {
  expect(pageClientCoreDigests()).toEqual({ wasm_sha256: wasm, runtime_sha256: runtime });
  for (const content of [
    '',
    wasm,
    `${wasm} nope`,
    `${wasm} ${runtime} ${wasm}`,
    `${wasm}X ${runtime}`,
  ]) {
    (globalThis as { document?: unknown }).document = {
      querySelector: () => ({ getAttribute: () => content }),
    };
    expect(pageClientCoreDigests()).toBeNull();
  }
});

test('el worker precalentado baja el núcleo con los resúmenes de la página y lo adopta el primer arrendamiento', async () => {
  // Sin módulo: el de la bóveda, que es el que arranca un PqcRuntime sin módulo.
  const soltar = prewarmClientCore();
  expect(FakeWorker.all).toHaveLength(1);
  const [warm] = FakeWorker.all;
  expect(warm!.posted[0]).toEqual({
    type: 'initialize',
    assetBaseURL: 'https://drive.sytue.test/api/client-core/',
    module: 'vault',
    digests: { wasm_sha256: wasm, runtime_sha256: runtime },
  });
  // Contesta antes de que nadie lo adopte: se guarda y se entrega al adoptarlo.
  warm!.emit({ type: 'progress' });
  warm!.emit({ type: 'ready', module_id: 'vault' });

  const { life, lease: l } = lease();
  const core = new PqcRuntime(l);
  const out = await core.invoke<{ operation: string }>({ operation: 'identity.create' });
  expect(out.operation).toBe('identity.create');
  // Un solo worker: el precalentado, ahora del arrendamiento.
  expect(FakeWorker.all).toHaveLength(1);
  expect(warm!.posted).toHaveLength(2);
  // Soltarlo después de adoptarlo no lo toca: ya no es del precalentado.
  soltar();
  expect(warm!.terminated).toBe(false);
  life.abort();
  expect(warm!.terminated).toBe(true);

  // El siguiente arrendamiento no hereda nada: worker nuevo.
  const otro = lease();
  const segundo = new PqcRuntime(otro.lease);
  const pendiente = segundo.invoke({ operation: 'identity.public' });
  expect(FakeWorker.all).toHaveLength(2);
  FakeWorker.all[1]!.emit({ type: 'ready', module_id: 'vault' });
  await pendiente;
  otro.life.abort();
});

test('el precalentado de la bóveda vale aunque un Rooms anterior le dé el núcleo completo', async () => {
  prewarmClientCore();
  const [warm] = FakeWorker.all;
  // worker.ts cayó a `core` porque el servidor no conocía `vault`.
  warm!.emit({ type: 'ready', module_id: 'core' });
  const { life, lease: l } = lease();
  const vault = new PqcRuntime(l);
  await vault.invoke({ operation: 'identity.create' });
  expect(FakeWorker.all).toHaveLength(1);
  life.abort();
});

test('la bóveda adopta un núcleo completo precalentado; el completo no adopta la bóveda', async () => {
  prewarmClientCore('core');
  FakeWorker.all[0]!.emit({ type: 'ready', module_id: 'core' });
  const vault = lease();
  await new PqcRuntime(vault.lease).invoke({ operation: 'identity.create' });
  expect(FakeWorker.all).toHaveLength(1);
  vault.life.abort();

  const soltar = prewarmClientCore();
  const full = lease();
  const pendiente = new PqcRuntime(full.lease, 'core').invoke({ operation: 'rooms.search.parse' });
  expect(FakeWorker.all).toHaveLength(3);
  expect((FakeWorker.all[2]!.posted[0] as { module?: string }).module).toBe('core');
  FakeWorker.all[2]!.emit({ type: 'ready', module_id: 'core' });
  await pendiente;
  full.life.abort();
  soltar();
  expect(FakeWorker.all[1]!.terminated).toBe(true);
});

test('un worker precalentado que nadie adopta se termina: al soltarlo o al caducar', async () => {
  const soltar = prewarmClientCore('core');
  // Pedirlo otra vez no arranca otro.
  prewarmClientCore('core');
  expect(FakeWorker.all).toHaveLength(1);
  soltar();
  expect(FakeWorker.all[0]!.terminated).toBe(true);

  prewarmClientCore('core', 5);
  await new Promise((r) => setTimeout(r, 20));
  expect(FakeWorker.all[1]!.terminated).toBe(true);
});

test('solo el núcleo de la bóveda lleva los resúmenes de la página; otro módulo pide su manifiesto', () => {
  for (const module of ['pdf', 'core'] as const) {
    FakeWorker.all = [];
    const soltar = prewarmClientCore(module);
    expect(FakeWorker.all[0]!.posted[0]).toEqual({
      type: 'initialize',
      assetBaseURL: 'https://drive.sytue.test/api/client-core/',
      module,
    });
    soltar();
  }
});
