import { clearOfflineStorage } from './storage.js';

export type Connectivity = 'online' | 'offline';

export interface NativeOfflineOptions {
  ownerId: string;
  tenantId: string;
  /** Marca efímera sin tokens: invalida respuestas del montaje anterior. */
  scopeId?: string;
  product: string;
  workerUrl?: string;
}

export function isNativeShell(): boolean {
  if (typeof window === 'undefined') return false;
  // Solo Capacitor: el `__sytueBridge` lo inyectaba el escritorio de Wails, retirado el
  // 2026-09-23, y las apps de Fyne no cargan la web.
  return Boolean((window as typeof window & { Capacitor?: unknown }).Capacitor);
}

// `onLine === false`, no `!onLine`: Node ≥21 define `globalThis.navigator` —con
// userAgent, language y poco más— pero SIN `onLine`, así que `!undefined` daba
// `true` y TODO el SSR se renderizaba como si el usuario estuviera sin conexión.
// En la topbar eso dejaba un cuadro gris vacío: el servidor mandaba el aviso de
// «sin conexión», el cliente hidrataba, veía que sí hay red, devolvía null y se
// llevaba el punto y el rótulo... dejando el `<span>` con su borde y su fondo
// huérfano en el DOM. Lo veían todos los usuarios conectados, release tras
// release, y no se encontraba leyendo el componente porque el componente está
// bien. Lo mismo afectaba a los seis `useConnectivity() === 'offline'` de Drive,
// Mail, Calendar, Contacts, Rooms y Docs. marker-b2
export function currentConnectivity(): Connectivity {
  return typeof navigator !== 'undefined' && navigator.onLine === false ? 'offline' : 'online';
}

export function subscribeConnectivity(listener: (state: Connectivity) => void): () => void {
  if (typeof window === 'undefined') return () => undefined;
  const onOnline = (): void => listener('online');
  const onOffline = (): void => listener('offline');
  const onWorkerMessage = (event: MessageEvent): void => {
    const data = event.data as { type?: string; state?: Connectivity } | null;
    if (
      data?.type === 'SYTUE_NETWORK_STATE' &&
      (data.state === 'online' || data.state === 'offline')
    ) {
      listener(data.state);
    }
  };
  window.addEventListener('online', onOnline);
  window.addEventListener('offline', onOffline);
  navigator.serviceWorker?.addEventListener('message', onWorkerMessage);
  return () => {
    window.removeEventListener('online', onOnline);
    window.removeEventListener('offline', onOffline);
    navigator.serviceWorker?.removeEventListener('message', onWorkerMessage);
  };
}

function loadedAssetUrls(): string[] {
  const urls = new Set<string>();
  for (const entry of performance.getEntriesByType('resource')) {
    if (
      entry instanceof PerformanceResourceTiming &&
      (entry.initiatorType === 'script' ||
        entry.initiatorType === 'css' ||
        entry.initiatorType === 'link')
    ) {
      urls.add(entry.name);
    }
  }
  for (const node of document.querySelectorAll<HTMLLinkElement | HTMLScriptElement>(
    'link[rel="stylesheet"][href],script[src]',
  )) {
    const raw = node instanceof HTMLLinkElement ? node.href : node.src;
    if (raw) urls.add(raw);
  }
  return [...urls];
}

function loadedPrivateUrls(): string[] {
  const urls = new Set<string>([location.href, new URL('/', location.href).href]);
  for (const entry of performance.getEntriesByType('resource')) {
    if (
      entry instanceof PerformanceResourceTiming &&
      entry.initiatorType !== 'script' &&
      entry.initiatorType !== 'css' &&
      entry.initiatorType !== 'link'
    ) {
      urls.add(entry.name);
    }
  }
  return [...urls];
}

/**
 * Instala el shell offline solo dentro de Capacitor. Un navegador web
 * compartido no conserva automáticamente páginas/API autenticadas.
 */
export async function installNativeOffline(
  { ownerId, tenantId, scopeId, product, workerUrl = '/sw.js' }: NativeOfflineOptions,
  current: () => boolean = () => true,
): Promise<void> {
  if (!isNativeShell() || !('serviceWorker' in navigator)) return;
  const revision = scopeId ?? crypto.randomUUID();
  const registration = await navigator.serviceWorker.register(workerUrl);
  const ready = await navigator.serviceWorker.ready;
  const worker = ready.active ?? registration.active ?? registration.waiting;
  if (!current()) return;
  worker?.postMessage({
    type: 'INITIALISE',
    ownerId,
    tenantId,
    scopeId: revision,
    product,
    assetUrls: loadedAssetUrls(),
    privateUrls: loadedPrivateUrls(),
  });
}

/**
 * Precachea al montar y otra vez al recuperar red. Capacitor inyecta su puente antes
 * que los scripts de la página, así que basta con mirarlo una vez: la espera de seis
 * segundos era para el puente de Wails, que llegaba después de hidratar.
 */
export function activateNativeOffline(options: NativeOfflineOptions): () => void {
  if (typeof window === 'undefined' || !isNativeShell()) return () => undefined;
  let active = true;
  const scoped = { ...options, scopeId: options.scopeId ?? crypto.randomUUID() };
  const initialise = (): void => {
    if (isNativeShell()) void installNativeOffline(scoped, () => active).catch(() => undefined);
  };
  initialise();
  const onOnline = (): void => {
    initialise();
  };
  const onWorkerMessage = (event: MessageEvent): void => {
    const data = event.data as { type?: string } | null;
    if (data?.type === 'SYTUE_CLEAR_OFFLINE') {
      void clearOfflineStorage();
    }
  };
  window.addEventListener('online', onOnline);
  navigator.serviceWorker?.addEventListener('message', onWorkerMessage);
  return () => {
    active = false;
    navigator.serviceWorker?.controller?.postMessage({
      type: 'INVALIDATE',
      scopeId: scoped.scopeId,
    });
    window.removeEventListener('online', onOnline);
    navigator.serviceWorker?.removeEventListener('message', onWorkerMessage);
  };
}
