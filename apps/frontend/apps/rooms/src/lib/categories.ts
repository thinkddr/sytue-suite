/**
 * Agrupación del sidebar por categorías (Ola 11, estilo Mattermost/Slack). Lógica pura
 * (sin estado ni red): decide en qué sección cae cada canal y agrega sus no-leídos, para
 * que el componente solo pinte. Un canal asignado a una categoría personalizada CONOCIDA
 * va únicamente en ella (gana sobre la sección por defecto según kind); si su categoryId
 * no corresponde a ninguna categoría del usuario (p. ej. recién borrada, aún sin refrescar
 * la lista), recae a la sección por defecto. Favoritos es una sección por defecto ortogonal
 * (por la estrella), no una categoría personalizada.
 */
import type { MessageKey } from '../i18n/index.js';
import type { Channel, Category } from './chat.js';

/** Claves estables de las secciones por defecto (no son categorías del backend). */
export const DEFAULT_FAVORITES = 'favorites';
export const DEFAULT_CHANNELS = 'channels';
export const DEFAULT_DMS = 'dms';

/** Una sección del sidebar: por defecto (Favoritos/Canales/DMs) o personalizada. */
export interface SidebarGroup {
  /** id de la categoría personalizada, o una de las claves DEFAULT_*. */
  key: string;
  /**
   * Título ya legible: solo lo traen las categorías PERSONALIZADAS (lo escribe el
   * usuario). Las secciones por defecto traen `titleKey` en su lugar.
   */
  title?: string;
  /** Clave de catálogo del título de una sección por defecto (Favoritos/Canales/DMs). */
  titleKey?: MessageKey;
  /** Categoría personalizada subyacente (renombrar/borrar/reordenar/colapsar); ausente en
   *  las secciones por defecto. */
  category?: Category;
  channels: Channel[];
  /** No-leídos agregados de la sección para el badge al colapsar, respetando el nivel de
   *  notificación (Ola 13): 'none' no suma, 'mentions' suma solo @menciones, 'all' suma
   *  todos los no-leídos. */
  unread: number;
}

/** Orden dentro de una sección: por sortOrder asc y, a igualdad, por nombre. */
function byOrderThenName(a: Channel, b: Channel): number {
  const sa = a.sortOrder ?? 0;
  const sb = b.sortOrder ?? 0;
  return sa !== sb ? sa - sb : a.name.localeCompare(b.name);
}

/**
 * No-leídos agregados de la sección para el badge de notificación, respetando el nivel
 * de aviso por canal (Ola 13): 'none' (⟺ muted) no cuenta; 'mentions' cuenta solo las
 * @menciones; 'all' (o sin fijar) cuenta todos los no-leídos.
 */
function unreadOf(list: Channel[]): number {
  return list.reduce((n, c) => {
    if (c.notifyLevel === 'none' || c.muted) return n;
    if (c.notifyLevel === 'mentions') return n + (c.mentions ?? 0);
    return n + (c.unread ?? 0);
  }, 0);
}

/**
 * Agrupa los canales en las secciones ordenadas del sidebar: Favoritos (solo si tiene
 * contenido), Canales, las categorías personalizadas (por sortOrder, luego nombre) y
 * Mensajes directos. Canales y DMs se muestran siempre (llevan «Explorar»/«Nuevo mensaje»
 * y son los destinos por defecto).
 */
export function groupChannels(channels: Channel[], categories: Category[]): SidebarGroup[] {
  const known = new Set(categories.map((c) => c.id));
  const custom = new Map<string, Channel[]>();
  const favorites: Channel[] = [];
  const publicChannels: Channel[] = [];
  const dms: Channel[] = [];
  for (const c of channels) {
    if (c.categoryId && known.has(c.categoryId)) {
      const list = custom.get(c.categoryId);
      if (list) list.push(c);
      else custom.set(c.categoryId, [c]);
    } else if (c.favorite) {
      favorites.push(c);
    } else if (c.kind === 'direct') {
      dms.push(c);
    } else {
      publicChannels.push(c);
    }
  }

  const groups: SidebarGroup[] = [];
  if (favorites.length > 0) {
    groups.push({
      key: DEFAULT_FAVORITES,
      titleKey: 'sidebar.favourites',
      channels: favorites.sort(byOrderThenName),
      unread: unreadOf(favorites),
    });
  }
  groups.push({
    key: DEFAULT_CHANNELS,
    titleKey: 'sidebar.channels',
    channels: publicChannels.sort(byOrderThenName),
    unread: unreadOf(publicChannels),
  });
  const ordered = [...categories].sort(
    (a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name),
  );
  for (const cat of ordered) {
    const list = (custom.get(cat.id) ?? []).sort(byOrderThenName);
    groups.push({
      key: cat.id,
      title: cat.name,
      category: cat,
      channels: list,
      unread: unreadOf(list),
    });
  }
  groups.push({
    key: DEFAULT_DMS,
    titleKey: 'sidebar.directMessages',
    channels: dms.sort(byOrderThenName),
    unread: unreadOf(dms),
  });
  return groups;
}
