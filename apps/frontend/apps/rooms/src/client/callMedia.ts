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

/**
 * Para las pistas locales al momento: el SDK puede volver a arrancarlas al reconectar
 * solo, y en una llamada cifrada nada se publica hasta el acuse nuevo
 * (client/reanudacion.ts). Cada pista por separado: que una ya estuviera parada no deja
 * a las demás en marcha.
 */
export function detenerPistas(
  tracks: Iterable<{ mediaStreamTrack: { enabled: boolean }; stop: () => void }>,
): void {
  for (const track of tracks) {
    try {
      track.mediaStreamTrack.enabled = false;
    } catch {
      // Stopping must still run if a browser rejects the change.
    }
    try {
      track.stop();
    } catch {
      // Continue stopping the remaining tracks.
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
