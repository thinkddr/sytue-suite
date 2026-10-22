/**
 * Web Push del lado navegador (Ola 18): pide permiso, se suscribe con la clave VAPID vía
 * pushManager y registra la suscripción en el backend (BFF). El Service Worker (public/sw.js)
 * recibe los push y muestra la notificación cuando la app está cerrada. La lógica pura
 * (urlBase64ToUint8Array) va aparte para poder testearla sin navegador (bun test).
 */
import { subscribePush, unsubscribePush } from '../client/chatApi.js';

/**
 * urlBase64ToUint8Array decodifica una clave VAPID en base64url (sin relleno, como la emite
 * el backend) al Uint8Array que espera `applicationServerKey`. Repone el relleno `=` y
 * traduce el alfabeto url-safe (-_) al estándar (+/). Pura y testeable.
 */
export function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4);
  const normalized = (base64 + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(normalized);
  // Respalda con un ArrayBuffer concreto (no ArrayBufferLike) para que valga como
  // BufferSource de applicationServerKey (evita el estrechamiento a SharedArrayBuffer).
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

/** ¿Soporta este navegador Web Push (SW + PushManager + Notification)? */
export function pushSupported(): boolean {
  return (
    typeof navigator !== 'undefined' &&
    'serviceWorker' in navigator &&
    typeof window !== 'undefined' &&
    'PushManager' in window &&
    typeof Notification !== 'undefined'
  );
}

/** ¿Hay ya una suscripción push activa en este navegador? */
export async function isPushSubscribed(): Promise<boolean> {
  if (!pushSupported()) return false;
  const reg = await navigator.serviceWorker.ready;
  return (await reg.pushManager.getSubscription()) !== null;
}

/**
 * subscribeToPush pide permiso, se suscribe con la clave VAPID y registra la suscripción en
 * el backend. Devuelve true si quedó suscrito; false si el permiso se denegó o no hay soporte.
 * Reusa la suscripción existente (idempotente) para no rotar el endpoint en cada activación.
 */
export async function subscribeToPush(vapidKey: string): Promise<boolean> {
  if (!pushSupported() || !vapidKey) return false;
  const perm = await Notification.requestPermission();
  if (perm !== 'granted') return false;

  const reg = await navigator.serviceWorker.ready;
  const existing = await reg.pushManager.getSubscription();
  const sub =
    existing ??
    (await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidKey),
    }));

  const json = sub.toJSON();
  if (!json.endpoint || !json.keys?.p256dh || !json.keys?.auth) return false;
  await subscribePush({
    endpoint: json.endpoint,
    keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
  });
  return true;
}

/** Da de baja el push: lo borra en el backend y en el navegador. Idempotente. */
export async function unsubscribeFromPush(): Promise<void> {
  if (!pushSupported()) return;
  const reg = await navigator.serviceWorker.ready;
  const sub = await reg.pushManager.getSubscription();
  if (!sub) return;
  try {
    await unsubscribePush(sub.endpoint);
  } finally {
    await sub.unsubscribe();
  }
}
