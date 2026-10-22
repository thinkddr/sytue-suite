/**
 * Catálogo de instaladores nativos (SOLO servidor).
 *
 * POR QUÉ ESTO NO VIVE EN EL SITIO PÚBLICO. Los instaladores son parte del
 * producto, no material de marketing: se descargan con una cuenta de Sytue y una
 * suscripción viva. `www` es estático y anónimo por diseño, así que la lista y
 * los ficheros se sirven desde el portal, que sí tiene sesión.
 *
 * LA BARRERA NO SE COMPRUEBA AQUÍ, Y NO ES UN OLVIDO. Sytue ID no emite ni
 * refresca un token si la organización no tiene derecho a la suite —lo mira
 * `orgEntitled` en cada emisión y en cada refresco, cruzando la suscripción con
 * los miembros activos y con los asientos concedidos—. Esos asientos son los que
 * cubren los casos de regalo y de empleado, así que TENER SESIÓN VÁLIDA ya
 * significa tener derecho. Repetir la comprobación aquí con otra consulta sería
 * un segundo criterio que puede discrepar del que manda.
 *
 * DE DÓNDE SALEN LOS FICHEROS. De un bucket PRIVADO, con una credencial que solo
 * puede leer ESE bucket: la aplicación IAM no lleva ningún permission set —los de
 * Object Storage de Scaleway son de proyecto entero— y lo que le da acceso es la
 * política del propio bucket, que la nombra. Si esa clave se filtrara, el daño se
 * limita a descargar unos instaladores que además son el producto que
 * distribuimos.
 *
 * Se descartó tirar de la API de la forja: habría exigido un token con
 * `read:repository`, que da lectura sobre TODO el código fuente. Para bajar un
 * .dmg eso es desproporcionado, y el token vive en el contenedor web.
 *
 * Y se descartó proxear los bytes: se firma una URL de corta vida y se redirige,
 * así que los cientos de megas de cada instalador van del almacenamiento al
 * navegador sin pasar por el proceso del portal.
 */
import { CATALOGO, withBase } from '@sytue/ui';
import { serverEnv } from '@sytue/auth/server';

const env = serverEnv(import.meta.env);

/** Plataformas que reconocemos por el nombre del fichero. */
export type Plataforma = 'macos' | 'windows' | 'linux' | 'android';

/** Arquitectura de un instalador, sacada de su nombre. `universal` vale para las dos. */
export type Arquitectura = 'amd64' | 'arm64' | 'universal';

export interface Instalador {
  /** Nombre del objeto tal cual se publicó (`SytueOne-macos-universal.dmg`). */
  fichero: string;
  /** Producto legible, derivado del propio nombre (`Sytue One`). */
  producto: string;
  plataforma: Plataforma;
  /** Para qué procesador es. `null` si el nombre no lo dice. */
  arquitectura: Arquitectura | null;
  /** Tamaño en bytes, si el almacenamiento lo informa. */
  bytes: number;
  /** Ruta del BFF que redirige. NUNCA la URL firmada, que caduca. */
  href: string;
}

export interface Catalogo {
  /** Versión de la que salen (`v2.0.0`), leída del prefijo. Vacía si no hay. */
  version: string;
  instaladores: Instalador[];
  /**
   * Por qué no hay nada, cuando no lo hay. Se distingue «aún no se ha publicado
   * ninguna versión» de «este despliegue no sabe dónde buscarlas»: la primera se
   * arregla cortando una release y la segunda configurando una credencial, y
   * confundirlas manda a alguien a mirar donde no es.
   */
  motivo?: 'sin-configurar' | 'sin-release' | 'sin-instaladores';
}

/**
 * Resuelve el endpoint de S3 para descargas.
 *
 * Si se pide endpoint público pero SYTUE_DOWNLOADS_PUBLIC_ENDPOINT no está configurado
 * o es una cadena vacía, debe caer SIEMPRE al endpoint interno (`SYTUE_DOWNLOADS_ENDPOINT`
 * o defecto), NUNCA a cadena vacía ni a undefined (lo que causaría que el SDK de Bun
 * resuelva por defecto al endpoint de AWS S3 https://s3.<region>.amazonaws.com).
 */
export function resolverEndpointDescargas(
  publico: boolean,
  envVars: { SYTUE_DOWNLOADS_ENDPOINT?: string; SYTUE_DOWNLOADS_PUBLIC_ENDPOINT?: string },
): string {
  const interno = envVars.SYTUE_DOWNLOADS_ENDPOINT?.trim() || 'https://s3.fr-par.scw.cloud';
  const publicoEndpoint = envVars.SYTUE_DOWNLOADS_PUBLIC_ENDPOINT?.trim();
  return publico && publicoEndpoint ? publicoEndpoint : interno;
}

/**
 * ¿Ese host solo existe dentro del clúster o en una red privada/interna?
 *
 * Se mira el nombre y no se intenta resolverlo: una consulta DNS por descarga es
 * latencia por algo que no cambia, y estos sufijos no los usa nadie de cara al
 * público. Si alguien publicara su S3 en un `.local`, tiene la variable
 * `downloadsPublicEndpoint` para decirlo.
 */
export function esInterno(url: string): boolean {
  try {
    const h = new URL(url).hostname.toLowerCase();
    return (
      h === 'localhost' ||
      h === '127.0.0.1' ||
      h === '::1' ||
      h.endsWith('.local') ||
      h.endsWith('.internal') ||
      h.endsWith('.svc') ||
      h.endsWith('.svc.cluster.local') ||
      !h.includes('.') ||
      h.startsWith('10.') ||
      h.startsWith('192.168.') ||
      /^172\.(1[6-9]|2[0-9]|3[0-1])\./.test(h) ||
      h.includes('eu-local-1')
    );
  } catch {
    return false;
  }
}

/**
 * Cliente del bucket. Bun trae firma S3 nativa, así que no hace falta un SDK: son
 * cientos de kilobytes de dependencia para construir una URL con HMAC.
 *
 * `null` si falta configuración — que es un estado válido, no un fallo.
 */
function almacen(publico = false): { cliente: Bun.S3Client; prefijo: string } | null {
  const accessKeyId = env.SYTUE_DOWNLOADS_ACCESS_KEY?.trim() ?? '';
  const secretAccessKey = env.SYTUE_DOWNLOADS_SECRET_KEY?.trim() ?? '';
  const bucket = env.SYTUE_DOWNLOADS_BUCKET?.trim() ?? '';
  if (!accessKeyId || !secretAccessKey || !bucket) return null;
  const endpoint = resolverEndpointDescargas(publico, env);
  return {
    cliente: new Bun.S3Client({
      accessKeyId,
      secretAccessKey,
      bucket,
      region: env.SYTUE_DOWNLOADS_REGION?.trim() || 'fr-par',
      // DOS ENDPOINTS PARA DOS COSAS DISTINTAS, y hasta ahora era uno.
      //
      // Listar el bucket lo hace este proceso desde dentro; la URL firmada la
      // abre el NAVEGADOR de quien descarga. Con un S3 dentro del clúster —el
      // caso del piloto, `seaweedfs.sytue-data.svc.cluster.local:8333`— el mismo
      // endpoint valía para lo primero y era irresoluble para lo segundo: el 302
      // llevaba a un nombre que solo existe en la red interna. Medido el
      // 2026-09-12: `getaddrinfo ETIMEOUT` en el navegador.
      //
      // No se puede firmar con el interno y reescribir el host después: en SigV4
      // el host va DENTRO de la firma (`SignedHeaders=host`), así que cambiarlo
      // la invalida. Hay que firmar ya contra el host público si existe.
      //
      // Donde el S3 ya es público —Scaleway— basta con no definir la segunda y
      // se comporta igual que antes.
      //
      // Si SYTUE_DOWNLOADS_PUBLIC_ENDPOINT está vacío o sin definir, caemos al
      // endpoint interno y la ruta BFF `/api/descargas/...` se encarga de servir
      // el fichero en streaming hacia el navegador.
      endpoint,
    }),
    // El CI publica bajo `latest/`, que es lo que la web enlaza siempre: así una
    // versión nueva no obliga a tocar nada aquí.
    prefijo: env.SYTUE_DOWNLOADS_PREFIX?.trim() || 'latest/',
  };
}

/** Cuánto vive la URL firmada. Cinco minutos: lo que tarda en empezar la descarga
 *  —una vez empezada, el enlace ya no se vuelve a usar—, y poco para reenviarla. */
const FIRMA_SEGUNDOS = 300;

/**
 * Reconoce un artefacto de usuario final y de qué plataforma es. Deja fuera todo
 * lo demás que pueda acabar en el bucket (SBOM, firmas, checksums), que no pinta
 * nada en una página de descargas.
 */
export function clasificar(nombre: string): Plataforma | null {
  const m = NOMBRE_INSTALADOR.exec(nombre);
  return m ? (m[2]!.split('-')[0] as Plataforma) : null;
}

/**
 * Lo que publica `apps/fyne/scripts/empaquetar.sh`, y nada más: `sytue-<bundle>-` y un
 * sufijo por plataforma y procesador. Fyne empaqueta Linux en `.tar.xz`; el `.tar.gz`
 * era el de Wails. Ni APK ni AAB: con Play App Signing un APK nuestro no se
 * actualizaría desde Play, y el AAB solo lo instala Play (docs/operations.md,
 * «Android: Play App Signing»).
 *
 * Es LA MISMA que `nombreInstalador` del Hub (services/hub/internal/instaladores), que
 * es de donde las apps nativas sacan sus descargas y sus actualizaciones. Los dos
 * pasan los vectores de `nombres-instaladores.json`.
 */
const NOMBRE_INSTALADOR =
  /^sytue-([a-z0-9]+)-(linux-(?:amd64|arm64)\.tar\.xz|windows-(?:amd64|arm64)-installer\.exe|macos-(?:universal|amd64|arm64)\.dmg)$/;

/** «sytue-one-linux-amd64.tar.xz» → `amd64`. Lo dice el nombre que da empaquetar.sh. */
export function arquitecturaDe(nombre: string): Arquitectura | null {
  const m = /-(amd64|arm64|universal)(?:[-.]|$)/.exec(nombre);
  return (m?.[1] as Arquitectura | undefined) ?? null;
}

/**
 * Sistema y procesador de quien mira, por el user agent. Es una PISTA para destacar su
 * descarga, no una decisión: todo lo demás sigue en la lista de abajo.
 *
 * El procesador solo se sabe de verdad en Linux (`aarch64`/`arm64` frente a `x86_64`).
 * Un Mac con Apple silicon sigue diciendo «Intel Mac OS X», y Chrome en Windows ARM dice
 * «Win64; x64»: en esos dos casos se deja en `null`, que no descarta nada.
 */
export function equipoDelUA(ua: string): {
  plataforma: Plataforma | null;
  arquitectura: 'amd64' | 'arm64' | null;
} {
  if (/Android/i.test(ua)) return { plataforma: 'android', arquitectura: null };
  if (/iPhone|iPad|iPod/i.test(ua)) return { plataforma: null, arquitectura: null };
  if (/Mac OS X|Macintosh/i.test(ua)) return { plataforma: 'macos', arquitectura: null };
  if (/Windows/i.test(ua))
    return { plataforma: 'windows', arquitectura: /ARM64|aarch64/i.test(ua) ? 'arm64' : null };
  if (/Linux|X11|CrOS/i.test(ua)) {
    const arquitectura = /aarch64|arm64|armv8/i.test(ua)
      ? 'arm64'
      : /x86_64|amd64|x64/i.test(ua)
        ? 'amd64'
        : null;
    return { plataforma: 'linux', arquitectura };
  }
  return { plataforma: null, arquitectura: null };
}

/**
 * ¿Este instalador sirve en ese equipo? Es lo que decide qué se DESTACA como «para tu
 * sistema»: un `linux-amd64` no arranca en un Linux ARM, y antes se le ofrecía igual.
 *
 * Windows en ARM ejecuta los binarios x64 por emulación, así que ahí un `amd64` sí vale.
 */
export function sirveEn(
  i: Pick<Instalador, 'plataforma' | 'arquitectura'>,
  plataforma: Plataforma | null,
  arquitectura: 'amd64' | 'arm64' | null,
): boolean {
  if (!plataforma || i.plataforma !== plataforma) return false;
  if (!arquitectura || !i.arquitectura || i.arquitectura === 'universal') return true;
  if (i.arquitectura === arquitectura) return true;
  return plataforma === 'windows' && arquitectura === 'arm64' && i.arquitectura === 'amd64';
}

/**
 * «SytueOne-macos-universal.dmg» → «Sytue One». El nombre sale del fichero y no
 * de una tabla paralela: si mañana se publica un producto nuevo, aparece solo.
 */
export function productoDe(nombre: string): string {
  // Fyne publica `sytue-<app>-<plataforma>…` (empaquetar.sh). El nombre de marca
  // sale del CATÁLOGO —por clave o por nombre— y no de una tabla paralela: con
  // `split('-')[0]` las trece apps salían como «Sytue» a secas.
  const fyne = /^sytue-([a-z0-9]+)-/.exec(nombre);
  if (fyne) {
    const app = fyne[1] ?? '';
    // El bundle de la suite en escritorio (`sytue-one-…`). En el catálogo, `one` es la
    // clave del Hub, que es solo una de las apps que lleva dentro.
    if (app === 'one') return 'Sytue One';
    for (const e of CATALOGO.values()) {
      if (e.key === app || e.name.toLowerCase().replace(/\s+/g, '') === `sytue${app}`)
        return e.name;
    }
    return `Sytue ${app.charAt(0).toUpperCase()}${app.slice(1)}`;
  }
  const base = nombre.split('-')[0] ?? nombre;
  if (base.toLowerCase().startsWith('sytue')) {
    const resto = base.slice(5).replace(/[._].*$/, '');
    return resto ? `Sytue ${resto}` : 'Sytue';
  }
  // Los APK usan minúsculas con guiones: `sytue-one-android.apk`.
  return nombre
    .replace(/-android\.apk$/, '')
    .split('-')
    .map((p) => p.charAt(0).toUpperCase() + p.slice(1))
    .join(' ');
}

/**
 * La ruta del BFF que redirige a UN instalador, con el prefijo de la aplicación.
 *
 * Está aparte y recibe la base para poder probarla: es una línea, y es la línea
 * que tuvo el portal roto —los enlaces salían sin el `/one` y en el host de
 * organización no los enrutaba nadie—. Un fallo que solo se ve entrando a la
 * página y pulsando un botón necesita algo que lo sujete.
 */
export function hrefDe(nombre: string, base: string = import.meta.env.BASE_URL): string {
  return withBase(base, `/api/descargas/${encodeURIComponent(nombre)}`);
}

/**
 * Lista lo que hay en el bucket. Nunca lanza: esta página no puede caerse porque
 * el almacenamiento tarde, y un fallo de red se cuenta como «todavía no hay» con
 * su motivo, no como una pantalla rota.
 */
export async function cargarCatalogo(): Promise<Catalogo> {
  const a = almacen();
  if (!a) return { version: '', instaladores: [], motivo: 'sin-configurar' };
  try {
    const listado = await a.cliente.list({ prefix: a.prefijo, maxKeys: 200 });
    const instaladores: Instalador[] = [];
    for (const obj of listado.contents ?? []) {
      const clave = obj.key ?? '';
      const nombre = clave.slice(clave.lastIndexOf('/') + 1);
      const plataforma = clasificar(nombre);
      if (!plataforma) continue;
      instaladores.push({
        fichero: nombre,
        producto: productoDe(nombre),
        plataforma,
        arquitectura: arquitecturaDe(nombre),
        bytes: obj.size ?? 0,
        // CON EL PREFIJO DE LA APP. El portal se sirve con `base: '/one'`, así
        // que un enlace absoluto desde la raíz se sale de la aplicación: en el
        // host de organización, `/api/descargas/…` no lo enruta nadie y Traefik
        // contesta un 404 en texto plano. Medido en el piloto el 2026-09-12:
        // `corporate.sytue.eu/api/descargas/X` → 404, y `…/one/api/descargas/X`
        // → 401, que es la ruta buena pidiendo sesión.
        href: hrefDe(nombre),
      });
    }
    // Por producto y, dentro, por plataforma: quien entra busca SU app, no su
    // sistema operativo — de eso ya se encarga la sección destacada de arriba.
    instaladores.sort(
      (x, y) => x.producto.localeCompare(y.producto) || x.plataforma.localeCompare(y.plataforma),
    );
    return {
      version: (await versionDe(a, instaladores[0]?.fichero)) || (env.SYTUE_RELEASE ?? ''),
      instaladores,
      ...(instaladores.length === 0 ? { motivo: 'sin-instaladores' as const } : {}),
    };
  } catch {
    return { version: '', instaladores: [], motivo: 'sin-release' };
  }
}

/**
 * La release de la que salen los ficheros, leída de ellos (`x-amz-meta-sytue-tag`, que
 * pone traer-instaladores.sh). No es la desplegada: si la última release llegó sin
 * instaladores completos, el almacén se queda con la anterior entera, y la página decía
 * «Versión v3.6.26» sobre unos .dmg de la v3.6.23. Todos los ficheros son de la misma
 * release —el guion no sube nada de una incompleta—, así que basta con mirar uno.
 */
async function versionDe(
  a: { cliente: Bun.S3Client; prefijo: string },
  fichero: string | undefined,
): Promise<string> {
  if (!fichero) return '';
  try {
    const url = a.cliente.presign(`${a.prefijo}${fichero}`, { method: 'HEAD', expiresIn: 60 });
    const r = await fetch(url, { method: 'HEAD', signal: AbortSignal.timeout(3000) });
    return r.ok ? (r.headers.get('x-amz-meta-sytue-tag') ?? '') : '';
  } catch {
    return '';
  }
}

/**
 * URL firmada para descargar UN instalador. `null` si ese fichero no está en el
 * catálogo.
 *
 * El nombre se valida contra el catálogo real en vez de sanearlo: una lista de
 * caracteres prohibidos siempre se queda corta, y aquí se sabe exactamente qué
 * ficheros existen. Sin esto, un `..%2f` firmaría cualquier objeto del bucket.
 */
export async function firmarInstalador(fichero: string): Promise<string | null> {
  const a = almacen(true);
  if (!a) return null;
  const cat = await cargarCatalogo();
  if (!cat.instaladores.some((i) => i.fichero === fichero)) return null;
  return a.cliente.presign(`${a.prefijo}${fichero}`, {
    method: 'GET',
    expiresIn: FIRMA_SEGUNDOS,
    // El navegador guarda el fichero con su nombre y no con la clave del objeto.
    // La opción se llama `contentDisposition` en Bun y viaja como el parámetro
    // firmado `response-content-disposition`.
    contentDisposition: `attachment; filename="${fichero.replace(/"/g, '')}"`,
  });
}
