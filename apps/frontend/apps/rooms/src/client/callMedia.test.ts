/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { RoomEvent } from 'livekit-client';
import {
  enableAuthorizedMedia,
  endAutomaticReconnect,
  finishFailedCall,
  registerReconnectBarrier,
} from './callMedia.js';

test.each([RoomEvent.Reconnecting, RoomEvent.SignalReconnecting])(
  'the %s event invokes the same fail-closed media barrier',
  (event) => {
    const handlers = new Map<RoomEvent, () => void>();
    const trace: string[] = [];
    const track = {
      mediaStreamTrack: { enabled: true },
      stop: () => trace.push('stop-track'),
    };
    registerReconnectBarrier(
      (name, listener) => handlers.set(name, listener),
      () =>
        endAutomaticReconnect(
          [track],
          () => trace.push('invalidate'),
          () => trace.push('fail-closed'),
        ),
    );
    expect(handlers.size).toBe(2);
    handlers.get(event)?.();
    expect(track.mediaStreamTrack.enabled).toBe(false);
    expect(trace).toEqual(['invalidate', 'stop-track', 'fail-closed']);
  },
);

test('automatic reconnect invalidates permission, stops all tracks and closes the call immediately', () => {
  let allowed = true;
  const stopped: string[] = [];
  const make = (name: string) => ({
    mediaStreamTrack: { enabled: true },
    stop: () => {
      expect(allowed).toBe(false);
      stopped.push(name);
      if (name === 'video') throw new Error('track already stopped');
    },
  });
  const audio = make('audio');
  const video = make('video');
  const screen = make('screen');
  let failed = false;
  endAutomaticReconnect(
    [audio, video, screen],
    () => {
      allowed = false;
      expect(audio.mediaStreamTrack.enabled).toBe(true);
    },
    () => {
      failed = true;
    },
  );
  expect(allowed).toBe(false);
  expect(stopped).toEqual(['audio', 'video', 'screen']);
  expect(failed).toBe(true);
  expect([audio, video, screen].map((track) => track.mediaStreamTrack.enabled)).toEqual([
    false,
    false,
    false,
  ]);
});

test('transport disconnect failure cannot prevent key, worker and call-state teardown', async () => {
  for (const rejectAsync of [false, true]) {
    const trace: string[] = [];
    finishFailedCall(
      () => {
        trace.push('keys');
        throw new Error('listener');
      },
      () => {
        trace.push('worker');
        throw new Error('worker');
      },
      () => {
        trace.push('disconnect');
        if (rejectAsync) return Promise.reject(new Error('offline'));
        throw new Error('offline');
      },
      () => trace.push('failed'),
    );
    await Promise.resolve();
    expect(trace).toEqual(['keys', 'worker', 'disconnect', 'failed']);
  }
});

test('a media request completing after authorization ends stops the newly acquired track', async () => {
  let authorized = true;
  let release!: () => void;
  let entered!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  const started = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const calls: boolean[] = [];
  const operation = enableAuthorizedMedia(
    () => authorized,
    async (enabled) => {
      calls.push(enabled);
      if (enabled) {
        entered();
        await pending;
      }
    },
  );
  await started;
  authorized = false;
  release();
  expect(await operation).toBe(false);
  expect(calls).toEqual([true, false]);
});

test('a stopped authorization cannot start capture, while a valid one succeeds', async () => {
  const calls: boolean[] = [];
  const enable = async (enabled: boolean): Promise<void> => {
    calls.push(enabled);
  };
  expect(await enableAuthorizedMedia(() => false, enable)).toBe(false);
  expect(calls).toEqual([]);
  expect(await enableAuthorizedMedia(() => true, enable)).toBe(true);
  expect(calls).toEqual([true]);
});

test('a revoked request cleans up even if the SDK rejects after partial capture', async () => {
  let authorized = true;
  const calls: boolean[] = [];
  expect(
    await enableAuthorizedMedia(
      () => authorized,
      async (enabled) => {
        calls.push(enabled);
        if (enabled) {
          authorized = false;
          throw new Error('capture interrupted');
        }
      },
    ),
  ).toBe(false);
  expect(calls).toEqual([true, false]);
});
