import { expect, test } from 'bun:test';
import { HttpClient } from './http.js';
import {
  whatsAppSignupStatus,
  startWhatsAppSignup,
  finishWhatsAppSignup,
  listWhatsAppNumbers,
} from './whatsAppConsent.js';

test('WhatsApp signup forwards code only by POST and recovers real account numbers by instance', async () => {
  const calls: { method: string; path: string; body: unknown }[] = [];
  const client = new HttpClient({
    baseUrl: 'https://gateway.test/api/v1',
    fetchImpl: (async (input, init) => {
      const url = new URL(String(input));
      calls.push({
        method: init?.method ?? 'GET',
        path: `${url.pathname}${url.search}`,
        body: init?.body ? JSON.parse(String(init.body)) : null,
      });
      return Response.json(
        url.pathname.endsWith('/signup')
          ? { available: true }
          : { instance_id: 'instance', numbers: [] },
      );
    }) as typeof fetch,
  });
  expect(await whatsAppSignupStatus(client)).toEqual({ available: true });
  await startWhatsAppSignup(client, 'existing');
  await finishWhatsAppSignup(client, { state: 'sealed', code: 'short-lived' });
  await listWhatsAppNumbers(client, 'account/one');
  expect(calls).toEqual([
    { method: 'GET', path: '/api/v1/whatsapp/signup', body: null },
    { method: 'POST', path: '/api/v1/whatsapp/signup/start', body: { instance_id: 'existing' } },
    {
      method: 'POST',
      path: '/api/v1/whatsapp/signup/finish',
      body: { state: 'sealed', code: 'short-lived' },
    },
    { method: 'GET', path: '/api/v1/whatsapp/numbers?instance=account%2Fone', body: null },
  ]);
});
