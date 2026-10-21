/* SPDX-License-Identifier: Apache-2.0 */
import { suiteApex } from '@sytue/config';
import type { WhatsAppSignup, WhatsAppSignupFinish } from '@sytue/api-clients';

const consentOrigin = `https://integrations.${suiteApex}`;

export interface MetaConsentLabels {
  continue: string;
  loading: string;
  failed: string;
  done: string;
}

/** Only the isolated Integrations window may return this attempt's short-lived code. */
export function readMetaConsentMessage(
  event: Pick<MessageEvent<unknown>, 'origin' | 'source' | 'data'>,
  popup: Window,
  origin: string,
  state: string,
): { type: 'ready' } | { type: 'code'; code: string } | { type: 'error' } | null {
  if (
    event.origin !== origin ||
    event.source !== popup ||
    !event.data ||
    typeof event.data !== 'object'
  )
    return null;
  const message = event.data as { type?: unknown; state?: unknown; code?: unknown };
  if (message.type === 'sytue:whatsapp:ready') return { type: 'ready' };
  if (message.state !== state) return null;
  if (message.type === 'sytue:whatsapp:error') return { type: 'error' };
  if (
    message.type === 'sytue:whatsapp:code' &&
    typeof message.code === 'string' &&
    message.code.length > 0 &&
    message.code.length <= 8192
  )
    return { type: 'code', code: message.code };
  return null;
}

/** Call directly from a click. No third-party script executes on the Rooms origin. */
export function openMetaSignup(
  signup: WhatsAppSignup,
  labels: MetaConsentLabels,
  signal: AbortSignal,
): Promise<WhatsAppSignupFinish> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new Error('cancelled'));
      return;
    }
    let popup: Window | null = null;
    let settled = false;
    let bootstrapped = false;
    const cleanup = (): void => {
      window.removeEventListener('message', receive);
      signal.removeEventListener('abort', abort);
      clearInterval(closed);
      clearTimeout(timeout);
      popup?.close();
    };
    const fail = (): void => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(new Error('cancelled'));
    };
    const receive = (event: MessageEvent<unknown>): void => {
      if (!popup || settled || signal.aborted) return;
      const message = readMetaConsentMessage(event, popup, consentOrigin, signup.state);
      if (message?.type === 'ready' && !bootstrapped) {
        bootstrapped = true;
        popup.postMessage(
          {
            type: 'sytue:whatsapp:start',
            state: signup.state,
            app_id: signup.app_id,
            config_id: signup.config_id,
            graph_version: signup.graph_version,
            labels,
          },
          consentOrigin,
        );
      } else if (bootstrapped && message?.type === 'code') {
        settled = true;
        cleanup();
        // Resolve immediately: the BFF must exchange Meta's code within thirty seconds.
        resolve({ state: signup.state, code: message.code });
      } else if (bootstrapped && message?.type === 'error') fail();
    };
    const abort = (): void => fail();
    const closed = setInterval(() => {
      if (popup?.closed) fail();
    }, 500);
    const timeout = setTimeout(fail, 10 * 60 * 1000);
    window.addEventListener('message', receive);
    signal.addEventListener('abort', abort, { once: true });
    try {
      popup = window.open(
        `${consentOrigin}/consent/whatsapp`,
        '_blank',
        'popup,width=560,height=720',
      );
      if (!popup) fail();
    } catch {
      fail();
    }
  });
}
