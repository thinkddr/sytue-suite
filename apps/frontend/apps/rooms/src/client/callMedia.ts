/* SPDX-License-Identifier: Apache-2.0 */
import { RoomEvent } from 'livekit-client';

/** Both full and signal-only reconnects may resume without a fresh server ACK. */
export function registerReconnectBarrier(
  register: (
    event: RoomEvent.Reconnecting | RoomEvent.SignalReconnecting,
    listener: () => void,
  ) => void,
  stop: () => void,
): void {
  register(RoomEvent.Reconnecting, stop);
  register(RoomEvent.SignalReconnecting, stop);
}

/** An automatic SDK reconnect may restart tracks: end the call synchronously. */
export function endAutomaticReconnect(
  tracks: Iterable<{ mediaStreamTrack: { enabled: boolean }; stop: () => void }>,
  invalidate: () => void,
  fail: () => void,
): void {
  try {
    invalidate();
  } finally {
    try {
      for (const track of tracks) {
        try {
          track.mediaStreamTrack.enabled = false;
        } catch {
          // Stopping and closing must still run if a browser rejects a change.
        }
        try {
          track.stop();
        } catch {
          // Continue stopping the remaining tracks.
        }
      }
    } finally {
      fail();
    }
  }
}

/** Make all local teardown independent of an unavailable or broken transport. */
export function finishFailedCall(
  closeKeys: () => void,
  terminateWorker: () => void,
  disconnect: () => Promise<unknown>,
  markFailed: () => void,
): void {
  try {
    closeKeys();
  } catch {
    // Continue even if a listener cannot be removed.
  }
  try {
    terminateWorker();
  } catch {
    // Continue even if the worker is already gone.
  }
  try {
    void disconnect().catch(() => undefined);
  } catch {
    // A transport failure must not hide the failed call state.
  }
  markFailed();
}

/**
 * Device capture can resolve after a call lease has ended. Check both sides of
 * the async SDK call and stop any track it created after invalidation.
 */
export async function enableAuthorizedMedia(
  authorized: () => boolean,
  setEnabled: (enabled: boolean) => Promise<unknown>,
): Promise<boolean> {
  if (!authorized()) return false;
  try {
    await setEnabled(true);
  } catch (error) {
    if (!authorized()) {
      await setEnabled(false).catch(() => undefined);
      return false;
    }
    throw error;
  }
  if (authorized()) return true;
  await setEnabled(false);
  return false;
}
