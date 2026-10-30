/* SPDX-License-Identifier: Apache-2.0 */
import { MatrixSecurityError, isMatrixDeviceId } from './types.js';
import type { DeviceVerificationStatus } from 'matrix-js-sdk/lib/crypto-api';

type Fetch = typeof globalThis.fetch;

/**
 * Private storage must never deliver a room key to another person, even if that
 * person is verified for Rooms and joins between the UI's membership check and
 * RustCrypto's asynchronous key sharing. Check the SDK's actual outgoing batch.
 * This adapter inspects recipients, never decrypts or constructs key messages.
 */
export function privateDeviceFetch(
  home: string,
  own: string,
  verification: (device: string) => Promise<DeviceVerificationStatus | null>,
  closed: () => boolean,
  send: Fetch = globalThis.fetch,
): Fetch {
  const origin = new URL(home).origin;
  const guarded = async (input: Parameters<Fetch>[0], init?: Parameters<Fetch>[1]) => {
    if (closed()) throw new MatrixSecurityError('device');
    const url = new URL(input instanceof Request ? input.url : String(input), origin);
    if (url.origin !== origin) throw new MatrixSecurityError('identity');
    const roomEvent = /^\/_matrix\/client\/[^/]+\/rooms\/[^/]+\/send\/([^/]+)\/[^/]+$/.exec(
      url.pathname,
    );
    if (roomEvent) {
      if (decodeURIComponent(roomEvent[1] ?? '') !== 'm.room.encrypted')
        throw new MatrixSecurityError('unverified');
    } else if (
      url.pathname.startsWith('/_matrix/client/') &&
      url.pathname.includes('/rooms/') &&
      url.pathname.includes('/send/')
    )
      throw new MatrixSecurityError('unsupported');
    const match = /^\/_matrix\/client\/[^/]+\/sendToDevice\/([^/]+)\/[^/]+$/.exec(url.pathname);
    if (!match) {
      if (url.pathname.includes('sendToDevice')) throw new MatrixSecurityError('unsupported');
      return send.call(globalThis, input, init);
    }
    let type: string;
    try {
      type = decodeURIComponent(match[1] ?? '');
    } catch {
      throw new MatrixSecurityError('unsupported');
    }
    // The maintained SDK serializes to-device batches as JSON strings. Other
    // body forms fail closed, rather than checking one stream and sending another.
    if (
      !init ||
      init.method?.toUpperCase() !== 'PUT' ||
      input instanceof Request ||
      typeof init.body !== 'string' ||
      init.body.length > 1 << 20 ||
      (type !== 'm.room.encrypted' &&
        !type.startsWith('m.key.verification.') &&
        type !== 'm.room_key_request' &&
        type !== 'm.secret.request')
    )
      throw new MatrixSecurityError('unsupported');
    const body = init.body;
    // Freeze URL, headers, body and cancellation before any asynchronous trust
    // lookup. Never inspect one mutable request and send a later replacement.
    const outgoing = new Request(url.href, { ...init, body, redirect: 'error' });
    let value: unknown;
    try {
      value = JSON.parse(body);
    } catch {
      throw new MatrixSecurityError('identity');
    }
    if (!value || typeof value !== 'object' || Array.isArray(value))
      throw new MatrixSecurityError('identity');
    const messages = (value as { messages?: unknown }).messages;
    if (!messages || typeof messages !== 'object' || Array.isArray(messages))
      throw new MatrixSecurityError('identity');
    for (const [user, devices] of Object.entries(messages as Record<string, unknown>)) {
      if (user !== own || !devices || typeof devices !== 'object' || Array.isArray(devices))
        throw new MatrixSecurityError('identity');
      if (type === 'm.room.encrypted') {
        for (const device of Object.keys(devices)) {
          if (
            !isMatrixDeviceId(device) ||
            device === '*' ||
            !(await verification(device))?.isVerified()
          )
            throw new MatrixSecurityError('unverified');
        }
      }
    }
    if (closed()) throw new MatrixSecurityError('device');
    // Avoid redirects carrying an encrypted key batch to a different endpoint.
    return send.call(globalThis, outgoing);
  };
  // Preserve runtime fetch helpers without replacing their implementation. Only
  // actual requests pass through the recipient policy.
  return new Proxy(send, {
    apply: (_target, _receiver, args) => guarded(...(args as Parameters<Fetch>)),
  });
}
