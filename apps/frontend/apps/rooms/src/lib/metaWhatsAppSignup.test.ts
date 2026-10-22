import { expect, test } from 'bun:test';
import { openMetaSignup, readMetaConsentMessage } from './metaWhatsAppSignup.js';

const origin = 'https://integrations.sytue.eu';
const signup = {
  instance_id: 'instance',
  state: 'sealed-fixture-state',
  app_id: '123',
  config_id: '456',
  graph_version: 'v26.0',
};
const labels = { continue: 'Continue', loading: 'Loading', failed: 'Retry', done: 'Done' };

test('consent results require the exact origin, window and pending state', () => {
  const popup = {} as Window;
  const data = { type: 'sytue:whatsapp:code', state: signup.state, code: 'short-lived-code' };
  const event = { origin, source: popup, data };
  expect(readMetaConsentMessage(event, popup, origin, signup.state)).toEqual({
    type: 'code',
    code: 'short-lived-code',
  });
  for (const invalid of [
    { ...event, origin: 'https://integrations.sytue.eu.attacker.invalid' },
    { ...event, origin: 'http://integrations.sytue.eu' },
    { ...event, origin: 'null' },
    { ...event, source: {} as Window },
    { ...event, data: { ...data, state: 'other-attempt' } },
    { ...event, data: { ...data, code: '' } },
    { ...event, data: { ...data, code: 'x'.repeat(8193) } },
    { ...event, data: { ...data, type: 'unexpected' } },
  ])
    expect(readMetaConsentMessage(invalid, popup, origin, signup.state)).toBeNull();
});

test('the isolated popup opens in the click stack and returns the code without another event', async () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'window');
  const events = new EventTarget();
  const sent: { data: unknown; origin: string }[] = [];
  let closed = false;
  const popup = {
    closed: false,
    close: () => {
      closed = true;
    },
    postMessage: (data: unknown, target: string) => sent.push({ data, origin: target }),
  } as unknown as Window;
  let opened = '';
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: Object.assign(events, {
      open: (url: string) => {
        opened = url;
        return popup;
      },
    }),
  });
  const emit = (data: unknown): void => {
    const event = new Event('message');
    Object.defineProperties(event, {
      origin: { value: origin },
      source: { value: popup },
      data: { value: data },
    });
    events.dispatchEvent(event);
  };
  try {
    const result = openMetaSignup(signup, labels, new AbortController().signal);
    expect(opened).toBe(`${origin}/consent/whatsapp`);
    expect(opened).not.toContain(signup.state);
    emit({ type: 'sytue:whatsapp:code', state: signup.state, code: 'too-early' });
    emit({ type: 'sytue:whatsapp:ready' });
    expect(sent).toEqual([
      {
        origin,
        data: {
          type: 'sytue:whatsapp:start',
          state: signup.state,
          app_id: '123',
          config_id: '456',
          graph_version: 'v26.0',
          labels,
        },
      },
    ]);
    emit({ type: 'sytue:whatsapp:code', state: signup.state, code: 'fixture-code' });
    expect(await result).toEqual({ state: signup.state, code: 'fixture-code' });
    expect(closed).toBe(true);
    const controller = new AbortController();
    controller.abort();
    await expect(openMetaSignup(signup, labels, controller.signal)).rejects.toThrow('cancelled');
  } finally {
    if (original) Object.defineProperty(globalThis, 'window', original);
    else Reflect.deleteProperty(globalThis, 'window');
  }
});
