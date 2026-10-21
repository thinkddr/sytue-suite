/**
 * Cliente de la API de Sytue Photos desde el navegador.
 *
 * Todo va por el BFF de esta app —mismo origen, cookie httpOnly— y NUNCA
 * directamente al servicio Go: el token de acceso no baja al navegador.
 */
import { withBase } from '@sytue/ui';
import type { Foto } from '@sytue/api-clients';
import type { Receta } from '../editor/receta.js';

export type { Foto };

export interface Pagina {
  items: Foto[];
  nextCursor?: string;
}

const base = (ruta: string): string => withBase(import.meta.env.BASE_URL, ruta);

async function pedir<T>(ruta: string): Promise<T> {
  const res = await fetch(base(ruta), {
    credentials: 'include',
    headers: { accept: 'application/json' },
  });
  if (res.status === 401) {
    // `withBase` escrito aquí y no a través de `base()`: el gate de
    // packages/auth (rutaDeLogin.test.ts) busca `withBase(` junto a cualquier
    // `/auth/login` para saber que lleva el prefijo de la app, y un alias se lo
    // esconde. La regla existe porque veintiuna páginas redirigían a la raíz del
    // host y nadie podía entrar.
    window.location.assign(
      withBase(
        import.meta.env.BASE_URL,
        `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
      ),
    );
    throw new Error('No autenticado');
  }
  if (!res.ok) throw new Error(`Sytue Fotos respondió ${res.status}`);
  return (await res.json()) as T;
}

/** Una página de la línea de tiempo, de la más reciente a la más antigua. */
export function lineaDeTiempo(
  cursor?: string,
  limite = 100,
  texto = '',
  soloFavoritas = false,
  tambien: string[] = [],
): Promise<Pagina> {
  const q = new URLSearchParams({ limit: String(limite) });
  if (cursor) q.set('cursor', cursor);
  if (texto.trim()) q.set('q', texto.trim());
  if (soloFavoritas) q.set('favoritas', '1');
  if (tambien.length > 0) q.set('tambien', tambien.join(','));
  return pedir<Pagina>(`/api/v1/timeline?${q}`);
}

/**
 * La URL de una variante. NO se pide por fetch: va en el `src` de un `<img>`,
 * para que el navegador la programe con su propia prioridad y la reutilice de
 * su caché — que es inmutable, así que una foto vista se pinta sin red.
 */
export function urlDeVariante(id: string, variante: 'miniatura' | 'previa' | 'grande'): string {
  return base(`/api/v1/assets/${encodeURIComponent(id)}/variants/${variante}`);
}

/** Una foto por id, con sus dimensiones ya orientadas. */
export function foto(id: string): Promise<Foto> {
  return pedir<Foto>(`/api/v1/assets/${encodeURIComponent(id)}`);
}

/** La receta vigente y su huella. Sin editar, la receta vacía con la suya. */
export function obtenerReceta(id: string): Promise<{ recipe: Receta; fingerprint: string }> {
  return pedir(`/api/v1/assets/${encodeURIComponent(id)}/recipe`);
}

/**
 * Guarda la receta ENTERA. Devuelve la huella que calculó el servidor: es la
 * que nombra las derivadas, y usarla en vez de la calculada aquí es la red por
 * si las dos implementaciones discrepan — el test de paridad dice que no, pero
 * el servidor es quien guarda.
 */
export async function guardarReceta(id: string, recipe: Receta): Promise<{ fingerprint: string }> {
  const res = await fetch(base(`/api/v1/assets/${encodeURIComponent(id)}/recipe`), {
    method: 'PUT',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify({ recipe }),
  });
  if (!res.ok) throw new Error(`Sytue Fotos respondió ${res.status} al guardar la receta`);
  return (await res.json()) as { fingerprint: string };
}

/** URL de una variante para una huella concreta (la vista previa de una edición). */
export function urlDeVarianteCon(
  id: string,
  variante: 'miniatura' | 'previa' | 'grande',
  huella: string,
): string {
  return `${urlDeVariante(id, variante)}?f=${encodeURIComponent(huella)}`;
}

/** Un álbum que otra persona ha compartido conmigo (solo para ver). */
export interface AlbumCompartido {
  id: string;
  title: string;
  coverAssetId: string | null;
  count: number;
  /** Quién lo compartió: su `sub` y, si el directorio lo conoce, su nombre. */
  sharedBy: string;
  sharedByName?: string;
  updatedAt: string;
  /** `write`: me lo compartieron para editar (añadir, quitar y renombrar). */
  permission: 'read' | 'write';
}

/** Una foto de un álbum compartido conmigo: lo justo para la cuadrícula. */
export interface FotoCompartida {
  id: string;
  name: string;
  width: number;
  height: number;
  takenAt: string;
}

/** «Compartido conmigo»: los álbumes que otras personas han compartido conmigo. */
export async function albumesCompartidos(): Promise<AlbumCompartido[]> {
  return (await pedir<{ items?: AlbumCompartido[] }>('/api/shared')).items ?? [];
}

/** Un álbum compartido conmigo, con sus fotos. */
export function albumCompartido(
  id: string,
): Promise<{ album: AlbumCompartido; items: FotoCompartida[] }> {
  return pedir(`/api/shared?album=${encodeURIComponent(id)}`);
}

/** La URL de una foto de un álbum compartido conmigo: la verja es el álbum. */
export function urlDeVarianteCompartida(
  album: string,
  id: string,
  variante: 'miniatura' | 'previa' | 'grande',
): string {
  return base(
    `/api/v1/shared/albums/${encodeURIComponent(album)}/assets/${encodeURIComponent(id)}/variants/${variante}`,
  );
}

export interface Album {
  id: string;
  title: string;
  coverAssetId: string | null;
  /** Tiene enlace público. El enlace solo sale al generarlo: no se guarda. */
  shared?: boolean;
  count: number;
  createdAt: string;
  updatedAt: string;
  /** Es de otra persona, que me lo ha compartido para editar: su nombre. */
  compartidoPor?: string;
}

/** Envía un cuerpo JSON y devuelve la respuesta parseada (o nada en 204). */
async function mandar<T>(
  metodo: 'POST' | 'PUT' | 'PATCH' | 'DELETE',
  ruta: string,
  cuerpo?: unknown,
): Promise<T> {
  const res = await fetch(base(ruta), {
    method: metodo,
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: cuerpo === undefined ? null : JSON.stringify(cuerpo),
  });
  if (res.status === 401) {
    window.location.href = withBase(
      import.meta.env.BASE_URL,
      `/auth/login?returnTo=${encodeURIComponent(window.location.pathname + window.location.search)}`,
    );
    throw new Error('sin sesión');
  }
  if (!res.ok) throw new ErrorDeFotos(res.status);
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
}

/** Un fallo del servicio con su estado: 507 es que no cabe (la cuota de la organización). */
export class ErrorDeFotos extends Error {
  constructor(readonly status: number) {
    super(`Sytue Fotos respondió ${status}`);
    this.name = 'ErrorDeFotos';
  }
}

/** 507: lo que alguien con edición añade se copia al Drive de la dueña, y no cabe. */
export const sinEspacio = (e: unknown): boolean => e instanceof ErrorDeFotos && e.status === 507;

export const albumes = () => pedir<{ items: Album[] }>('/api/v1/albums').then((r) => r.items);
export const crearAlbum = (title: string) => mandar<Album>('POST', '/api/v1/albums', { title });
export const album = (id: string) => pedir<Album>(`/api/v1/albums/${encodeURIComponent(id)}`);
export const renombrarAlbum = (id: string, title: string, coverAssetId?: string) =>
  mandar<Album>('PATCH', `/api/v1/albums/${encodeURIComponent(id)}`, { title, coverAssetId });
export const borrarAlbum = (id: string) =>
  mandar<void>('DELETE', `/api/v1/albums/${encodeURIComponent(id)}`);
export const fotosDelAlbum = (id: string) =>
  pedir<{ items: Foto[] }>(`/api/v1/albums/${encodeURIComponent(id)}/assets`).then((r) => r.items);
/**
 * Los álbumes a los que se puede AÑADIR: los míos y los que me han compartido para
 * editar. Lo que se añade a uno de estos segundos se copia al Drive de su dueña.
 */
export async function albumesParaAnyadir(): Promise<Album[]> {
  const [mios, compartidos] = await Promise.all([albumes(), albumesCompartidos().catch(() => [])]);
  return [
    ...mios,
    ...compartidos
      .filter((a) => a.permission === 'write')
      .map((a) => ({
        id: a.id,
        title: a.title,
        coverAssetId: a.coverAssetId,
        count: a.count,
        createdAt: a.updatedAt,
        updatedAt: a.updatedAt,
        compartidoPor: a.sharedByName ?? a.sharedBy,
      })),
  ];
}

export const anyadirAlAlbum = (id: string, ids: string[]) =>
  mandar<{ added: number }>('POST', `/api/v1/albums/${encodeURIComponent(id)}/assets`, { ids });
export const quitarDelAlbum = (id: string, foto: string) =>
  mandar<void>(
    'DELETE',
    `/api/v1/albums/${encodeURIComponent(id)}/assets/${encodeURIComponent(foto)}`,
  );

/** Formatos de exportación: JPEG (calidad 92) o PNG sin pérdida. */
export type FormatoExportacion = 'jpeg' | 'png';

/** URL de descarga de la foto a resolución completa con la receta vigente. */
export const urlDeExportacion = (id: string, formato: FormatoExportacion = 'jpeg') =>
  base(`/api/v1/assets/${encodeURIComponent(id)}/export?formato=${formato}`);

/**
 * Da de alta en la galería una foto que ya está en Drive. El servicio solo la
 * indexa (128 KiB de cabecera) y contesta; las derivadas se generan al pedirlas.
 */
export async function indexar(driveFileId: string): Promise<Foto> {
  const res = await fetch(base('/api/v1/assets'), {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify({ driveFileId }),
  });
  if (!res.ok) throw new Error(`Sytue Fotos respondió ${res.status} al indexar`);
  return (await res.json()) as Foto;
}
export const alaPapelera = (id: string) =>
  mandar<void>('DELETE', `/api/v1/assets/${encodeURIComponent(id)}`);
export const papelera = () => pedir<{ items: Foto[] }>('/api/v1/trash').then((r) => r.items);
export const restaurar = (id: string) =>
  mandar<void>('POST', `/api/v1/assets/${encodeURIComponent(id)}/restore`);

/**
 * Marca o desmarca una favorita. El VALOR va en el cuerpo y no se alterna en el
 * servidor: dos pulsaciones rápidas —o un reintento— tienen que dejar la foto
 * donde dice la interfaz, no en el estado contrario.
 */
export const marcarFavorita = (id: string, favorite: boolean) =>
  mandar<void>('PUT', `/api/v1/assets/${encodeURIComponent(id)}/favorite`, { favorite });

/** Borra una foto de la papelera para siempre. El original sigue en Drive. */
export const purgar = (id: string) =>
  mandar<{ deleted: number }>('DELETE', `/api/v1/trash/${encodeURIComponent(id)}`);

/** Vacía la papelera entera. Devuelve cuántas se fueron. */
export const vaciarPapelera = () => mandar<{ deleted: number }>('DELETE', '/api/v1/trash');

/** Comparte el álbum y devuelve el token del enlace. Repetirlo da el MISMO. */
export const compartirAlbum = (id: string) =>
  mandar<{ token: string }>('POST', `/api/v1/albums/${encodeURIComponent(id)}/share`);

/** Apaga el enlace. Los que ya se mandaron dejan de valer. */
export const dejarDeCompartirAlbum = (id: string) =>
  mandar<void>('DELETE', `/api/v1/albums/${encodeURIComponent(id)}/share`);

/** La dirección que se copia y se manda. `/p/{token}` es una página sin sesión. */
export const enlaceDelAlbum = (token: string) =>
  new URL(base(`/p/${encodeURIComponent(token)}`), window.location.origin).toString();

/**
 * El TEXTO ALTERNATIVO de una foto.
 *
 * La descripción si la hay y el nombre del fichero si no. Un
 * «IMG_20260714_193045.jpg» leído por un lector de pantalla no dice nada, pero
 * es lo único que hay hasta que alguien pide describirla — y dejar el `alt`
 * vacío sería peor: la foto desaparecería del todo para quien no la ve.
 */
export const textoAlternativo = (f: { name: string; description?: string }) =>
  f.description?.trim() ? f.description : f.name;

/** Pide a Sytue AI que mire la foto. No la guarda: eso lo decide quien llama. */
export async function describirFoto(id: string): Promise<{ description: string; tags: string[] }> {
  const res = await fetch(base('/api/describir'), {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify({ id }),
  });
  const cuerpo = (await res.json().catch(() => ({}))) as {
    description?: string;
    tags?: string[];
    error?: string;
  };
  if (!res.ok) throw new Error(cuerpo.error ?? `ia_${res.status}`);
  return { description: cuerpo.description ?? '', tags: cuerpo.tags ?? [] };
}

/**
 * Pregunta algo sobre la foto. La respuesta NO se guarda en ninguna parte: es
 * una conversación sobre la foto, no un dato suyo.
 */
export async function preguntarFoto(id: string, pregunta: string): Promise<string> {
  const res = await fetch(base('/api/preguntar'), {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify({ id, pregunta }),
  });
  const cuerpo = (await res.json().catch(() => ({}))) as { answer?: string; error?: string };
  if (!res.ok) throw new Error(cuerpo.error ?? `ia_${res.status}`);
  return cuerpo.answer ?? '';
}

/** Guarda qué sale en la foto. */
export const guardarDescripcion = (id: string, description: string, tags: string[]) =>
  mandar<void>('PUT', `/api/v1/assets/${encodeURIComponent(id)}/description`, {
    description,
    tags,
  });

/**
 * Las fotos que más se parecen a esta, de más a menos.
 *
 * Lista VACÍA no es un fallo: quiere decir que el lote de vectores todavía no
 * ha mirado esa foto. La interfaz lo dice con esas palabras en vez de con un
 * error, porque no hay nada que arreglar, solo que esperar.
 */
export const parecidas = (id: string, limite = 24) =>
  pedir<{ items: Foto[] }>(`/api/v1/assets/${encodeURIComponent(id)}/similar?limit=${limite}`).then(
    (r) => r.items,
  );

/**
 * «Más como estas»: lo que se parece al CONJUNTO de las dadas, no a cada una.
 *
 * Es lo que convierte describir unas pocas en encontrar el resto: la búsqueda
 * literal solo saca lo que alguien escribió, y desde esos aciertos esto llega a
 * las que nadie describió nunca.
 *
 * Los ids son los que ya están en pantalla —resultados o selección—, así que el
 * servidor no repite la búsqueda. Doce como mucho: el resto los ignora él.
 */
export const parecidasAVarias = (ids: string[], limite = 24) =>
  pedir<{ items: Foto[] }>(
    `/api/v1/similar?ids=${encodeURIComponent(ids.slice(0, 12).join(','))}&limit=${limite}`,
  ).then((r) => r.items);

/**
 * Las fotos hechas cerca de esta, de la más próxima a la más lejana.
 *
 * «Cerca» lo decide el servicio y son dos kilómetros: el mismo sitio, no la
 * misma ciudad. Lista VACÍA si la foto no trae coordenadas, que es la mayoría
 * de las de una cámara sin GPS.
 */
export const cerca = (id: string, limite = 48) =>
  pedir<{ items: Foto[] }>(`/api/v1/assets/${encodeURIComponent(id)}/nearby?limit=${limite}`).then(
    (r) => r.items,
  );

/**
 * Los montones que la galería propone: fotos que van juntas por lo que se ve.
 *
 * Es la vuelta de «más como estas»: en vez de partir de algo, mira lo que hay.
 * Se pide A PETICIÓN y no al abrir la pantalla, porque el servicio recorre los
 * vectores de la biblioteca para contestar y eso no se paga por si acaso.
 */
export const temas = () =>
  pedir<{ themes: { items: Foto[] }[] }>('/api/v1/themes').then((r) =>
    r.themes.map((g) => g.items),
  );

/** Un grupo de fotos que se repiten entre sí. */
export interface GrupoRepetido {
  items: Foto[];
}

/**
 * Las fotos que sobran: ráfagas y reintentos del mismo momento.
 *
 * El servicio PROPONE y no borra. Quien decide es quien mira, porque proponer
 * y ejecutar de una vez es cómo se pierde una foto que no sobraba.
 */
export const duplicadas = (limite = 50) =>
  pedir<{ groups: GrupoRepetido[] }>(`/api/v1/duplicates?limit=${limite}`).then((r) => r.groups);

/**
 * Otras formas de decir lo que se escribió, para ampliar una búsqueda que no
 * encuentra nada. Devuelve lista vacía si Sytue AI no está o no supo qué decir:
 * entonces la búsqueda se queda como estaba, que es lo correcto.
 */
export async function terminosRelacionados(q: string): Promise<string[]> {
  const res = await fetch(base('/api/terminos'), {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify({ q }),
  });
  if (!res.ok) return [];
  const cuerpo = (await res.json().catch(() => ({}))) as { terminos?: string[] };
  return cuerpo.terminos ?? [];
}

/** Las fotos de un día como hoy en años anteriores. */
export interface Recuerdo {
  year: number;
  items: Foto[];
}

/**
 * El día lo pone el SERVIDOR, no esta llamada: el recuerdo es de hoy, y
 * dejarlo elegir sería otra cosa —una búsqueda por fecha— con otra pantalla.
 */
export const recuerdos = () =>
  pedir<{ memories: Recuerdo[] }>('/api/v1/memories').then((r) => r.memories);
