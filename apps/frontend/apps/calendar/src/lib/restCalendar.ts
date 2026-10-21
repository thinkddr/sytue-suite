/**
 * Mapeo compartido del calendario REST del servicio Go (domain.Calendar) a la
 * forma que consume el cliente (BFF /api/calendars y /api/subscriptions). Incluye
 * la URL de la coleccion CalDAV (para /api/events) y el estado de suscripcion ICS
 * de solo lectura. Solo forma de datos: la logica vive en Go.
 */

/** Forma del calendario REST del servicio Go (domain.Calendar). */
export interface RestCalendar {
  id: string;
  name: string;
  display_name: string;
  color: string;
  is_default: boolean;
  read_only?: boolean;
  subscription?: {
    url: string;
    last_status?: string;
    last_error?: string;
    last_fetched_at?: string;
    next_refresh_at?: string;
  } | null;
}

/** Calendario en la forma del cliente (BFF). */
export interface ClientCalendar {
  id: string;
  url: string;
  displayName: string;
  color: string;
  isDefault: boolean;
  readOnly: boolean;
  subscription?: { url: string; lastStatus?: string; lastError?: string; lastFetchedAt?: string };
}

/**
 * URL de la coleccion CalDAV del calendario (para /api/events y /api/event).
 * Replica calendarPath del backend Go: /dav/calendars/<urlSafe(sub)>/<name>/
 * (urlSafe sustituye "/" y espacio por "_").
 */
export function calDavUrl(sub: string, name: string): string {
  return `/dav/calendars/${sub.replace(/[/ ]/g, '_')}/${name}/`;
}

/**
 * Forma (aplanada) de un calendario COMPARTIDO que devuelve el servicio Go
 * (sharedCalendarDTO = domain.Calendar + permission + shared_by).
 */
export interface SharedRestCalendar {
  id: string;
  owner_sub: string;
  name: string;
  display_name: string;
  color: string;
  read_only?: boolean;
  permission: 'reader' | 'writer' | 'manage';
  shared_by: string;
  /** Siendo lector, ya ha pedido editarlo (la franja de solo lectura lo dice). */
  edit_requested?: boolean;
}

/** Calendario compartido en la forma del cliente (BFF /api/shared). */
export interface SharedClientCalendar extends ClientCalendar {
  shared: true;
  permission: 'reader' | 'writer' | 'manage';
  sharedBy: string;
  editRequested: boolean;
}

/**
 * Mapea un calendario compartido CON el usuario a la forma del cliente. Su URL de
 * coleccion es la del DUEÑO (owner_sub), que el backend resuelve por ruta dentro
 * del tenant, de modo que se pinta por el MISMO camino CalDAV que los propios. Un
 * permiso «reader» (o un calendario de suscripcion) se marca readOnly para que la
 * rejilla bloquee arrastrar/editar; un «writer» es editable.
 */
export function toSharedClient(c: SharedRestCalendar): SharedClientCalendar {
  return {
    id: c.id,
    url: calDavUrl(c.owner_sub, c.name),
    displayName: c.display_name || c.name,
    color: c.color,
    isDefault: false,
    readOnly: Boolean(c.read_only) || c.permission === 'reader',
    shared: true,
    permission: c.permission,
    sharedBy: c.shared_by,
    editRequested: Boolean(c.edit_requested),
  };
}

export function toClient(sub: string, c: RestCalendar): ClientCalendar {
  return {
    id: c.id,
    url: calDavUrl(sub, c.name),
    displayName: c.display_name || c.name,
    color: c.color,
    isDefault: c.is_default,
    readOnly: Boolean(c.read_only),
    ...(c.subscription
      ? {
          subscription: {
            url: c.subscription.url,
            ...(c.subscription.last_status ? { lastStatus: c.subscription.last_status } : {}),
            ...(c.subscription.last_error ? { lastError: c.subscription.last_error } : {}),
            ...(c.subscription.last_fetched_at
              ? { lastFetchedAt: c.subscription.last_fetched_at }
              : {}),
          },
        }
      : {}),
  };
}
