/**
 * openInApp — «Abrir en la aplicación»: salta de la web a la app instalada (escritorio o
 * móvil) mostrando el MISMO documento, igual que Office Online con `ms-word:ofe|u|…`.
 *
 * El puente es un esquema propio POR PRODUCTO —`sytue-mail://open?url=<https>`— que
 * registran las apps de Fyne (`esquema` de apps/fyne/cmd/<app>/FyneApp.toml). Esto es solo
 * el emisor; el destino lo tiene que validar quien lo reciba.
 *
 * En Fyne lo lee `api.DesdeLaWeb` (apps/nucleo/api/abrir.go) en la sesión de cada app:
 * solo https y solo un host de la suite, y de la página se queda la ruta y la consulta,
 * que llegan al receptor como `sytue-<p>:<ruta>?<consulta>`, lo mismo que abre la
 * búsqueda. `TestAbrirEnLaAplicacionLlegaALaPagina` lee ESTE fichero: si cambia la forma
 * del enlace, se entera.
 */
import { appDomains, type ProductKey } from '@sytue/config';

/**
 * Esquema de cada producto. Cada app registra el SUYO: si todas declararan `sytue://`, el
 * sistema operativo elegiría una arbitrariamente y «abrir en la aplicación» abriría la que
 * no toca. Es SIEMPRE `sytue-<clave>` y debe coincidir con el `esquema` de cada
 * apps/fyne/cmd/<app>/FyneApp.toml; el test lo compara valor a valor.
 *
 * `one` es el Hub (`sytue-one`). `sytue` a secas lo registraba el lanzador del escritorio
 * de Wails, retirado el 2026-09-23, y ya no lo atiende ninguna app de escritorio.
 */
export const PRODUCT_SCHEMES = {
  one: 'sytue-one',
  mail: 'sytue-mail',
  drive: 'sytue-drive',
  docs: 'sytue-docs',
  sheets: 'sytue-sheets',
  slides: 'sytue-slides',
  pdf: 'sytue-pdf',
  calendar: 'sytue-calendar',
  contacts: 'sytue-contacts',
  notes: 'sytue-notes',
  photos: 'sytue-photos',
  erp: 'sytue-erp',
  cloud: 'sytue-cloud',
  admin: 'sytue-admin',
  rooms: 'sytue-rooms',
  cast: 'sytue-cast',
  bases: 'sytue-bases',
} as const;

export type ProductScheme = keyof typeof PRODUCT_SCHEMES;

/**
 * schemeForURL deduce el producto por el subdominio del destino, buscándolo en los
 * hosts de @sytue/config: calcus.sytue.eu → `sytue-sheets`, mail.sytue.eu →
 * `sytue-mail`. Desde el 2026-10-12 cada producto de la ofimática tiene su host (ADR
 * 0076), así que el host basta también para ellos. Un host que no reconozcamos —el del
 * tenant, `corporate.sytue.eu`, entre ellos— cae al Hub.
 */
function schemeForURL(url: string): string {
  try {
    const host = new URL(url).hostname.split('.')[0]?.toLowerCase() ?? '';
    const clave =
      (Object.keys(PRODUCT_SCHEMES) as ProductScheme[]).find(
        (k) =>
          k in appDomains && new URL(appDomains[k as ProductKey]).hostname.split('.')[0] === host,
      ) ?? host;
    return (PRODUCT_SCHEMES as Record<string, string>)[clave] ?? PRODUCT_SCHEMES.one;
  } catch {
    return PRODUCT_SCHEMES.one;
  }
}

/** Cuánto se espera a que el sistema se lleve el foco antes de darlo por no instalado. */
const HANDOFF_MS = 1200;

/**
 * inNativeShell — ¿ya estamos DENTRO de la app nativa (la móvil, bajo Capacitor)? Dentro,
 * ofrecer «abrir en la aplicación» no tiene sentido: ya estás en ella. Es la misma
 * comprobación que usa el shell offline, así que se importa de ahí y no se copia.
 */
export { isNativeShell as inNativeShell } from '@sytue/offline';

/**
 * sameSuite — ¿ese host es de la MISMA suite que la página actual?
 *
 * El apex se deriva del host propio (`one.sytue.eu` → `sytue.eu`, `one.sytue.localhost` →
 * `sytue.localhost`) en vez de fijar una lista: así vale en producción y en dev sin
 * ramificar. Comprobación por sufijo de punto y no `includes`, que dejaría pasar
 * `sytue.eu.evil.com`.
 */
export function sameSuite(host: string): boolean {
  if (typeof window === 'undefined') return false;
  const apex = window.location.hostname.split('.').slice(-2).join('.');
  const h = host.toLowerCase();
  return apex !== '' && (h === apex || h.endsWith('.' + apex));
}

/**
 * deepLinkFor construye el deep link que abre una URL https de la suite en la app
 * instalada. El esquema sale del subdominio del destino salvo que se diga otra cosa:
 * `product` existe para Sheets, Slides y PDF, que comparten host con Docs y no se pueden
 * distinguir mirando la URL.
 */
export function deepLinkFor(url: string, product?: ProductScheme): string {
  const scheme = product ? PRODUCT_SCHEMES[product] : schemeForURL(url);
  return `${scheme}://open?url=${encodeURIComponent(url)}`;
}

/**
 * openInApp lanza el deep link y responde si la app se lo llevó.
 *
 * No hay forma fiable en ningún navegador de preguntar «¿está instalada esta app?»: el
 * navegador no lo cuenta, a propósito (sería huella digital). Lo único observable es que
 * el sistema se lleve el foco al abrir el manejador, así que se mira eso. Un `false` por
 * tanto significa «probablemente no está instalada», no una certeza — y por eso quien
 * llama debe OFRECER la descarga, no afirmar que falta.
 *
 * @param url URL https del documento a abrir en la app.
 * @param product Fuerza el producto destino. Necesario en Sheets y Slides, que comparten
 *   host con Docs; en el resto se deduce del subdominio.
 * @returns true si la página perdió el foco (la app se abrió).
 */
export function openInApp(url: string, product?: ProductScheme): Promise<boolean> {
  if (typeof window === 'undefined') return Promise.resolve(false);

  return new Promise((resolve) => {
    let done = false;
    const finish = (opened: boolean): void => {
      if (done) return;
      done = true;
      window.removeEventListener('blur', onLeave);
      document.removeEventListener('visibilitychange', onVisibility);
      resolve(opened);
    };
    const onLeave = (): void => finish(true);
    const onVisibility = (): void => {
      if (document.visibilityState === 'hidden') finish(true);
    };

    window.addEventListener('blur', onLeave);
    document.addEventListener('visibilitychange', onVisibility);

    // location.href y no un iframe oculto: Chrome bloquea la navegación a esquemas
    // propios desde iframes desde hace años, y el truco clásico ya no dispara nada.
    try {
      window.location.href = deepLinkFor(url, product);
    } catch {
      finish(false);
      return;
    }

    window.setTimeout(() => finish(false), HANDOFF_MS);
  });
}

/**
 * listenForDeepLinks — el lado receptor en las apps MÓVILES.
 *
 * En iOS/Android el shell es un webview que carga la suite (`server.url`), así que un
 * `sytue://` abre la app pero aterriza en la raíz, no en el documento: la URL llega
 * aparte, por el evento `appUrlOpen` de Capacitor. Esto la recoge y navega.
 *
 * En escritorio no hace falta: no hay webview y el enlace lo recibe el proceso Go de Fyne
 * (ver la cabecera). En un navegador normal no hay `Capacitor` y esto es un no-op.
 *
 * FRONTERA DE CONFIANZA: un `sytue://open?url=…` lo puede emitir cualquiera —una web, un
 * correo—, y el sistema operativo no comprueba quién. Sin validar el destino bastaría un
 * enlace para llevarse el webview, con la sesión dentro, a un sitio ajeno. Se valida con
 * `sameSuite`, que deriva el dominio del de la propia página: así vale igual en
 * producción y en el dev local sin listas que mantener.
 *
 * @returns función para dejar de escuchar.
 */
export function listenForDeepLinks(): () => void {
  if (typeof window === 'undefined') return () => {};
  const plugins = (window as unknown as { Capacitor?: { Plugins?: Record<string, unknown> } })
    .Capacitor?.Plugins;
  const app = plugins?.['App'] as
    { addListener?: (e: string, cb: (d: { url?: string }) => void) => unknown } | undefined;
  if (typeof app?.addListener !== 'function') return () => {};

  const handle = app.addListener('appUrlOpen', ({ url }) => {
    if (!url) return;
    let target: URL;
    try {
      const incoming = new URL(url);
      // Cualquiera de los esquemas de la suite: cada app registra el suyo, y la que reciba
      // el enlace es la que lo atiende.
      const suyo = Object.values(PRODUCT_SCHEMES).some(
        (s) => `${s}:` === incoming.protocol.toLowerCase(),
      );
      if (!suyo) return;
      target = new URL(incoming.searchParams.get('url') ?? '');
    } catch {
      return;
    }
    if (target.protocol !== 'https:' || !sameSuite(target.hostname)) return;
    window.location.assign(target.toString());
  }) as { remove?: () => void } | undefined;

  return () => handle?.remove?.();
}
