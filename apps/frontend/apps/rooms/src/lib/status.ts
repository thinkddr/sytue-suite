/**
 * Estado personalizado con EXPIRACIÓN (Mattermost/Slack: «Borrar después de»).
 *
 * El estado (emoji + texto, un solo campo) y su instante de caducidad se persisten en
 * localStorage; al montar y con un temporizador, si `now >= expiresAt` el estado se
 * limpia y se reenvía el heartbeat sin status (ver ChatApp). La lógica de cálculo de la
 * caducidad (`expiryAt`) e `isExpired` son PURAS (now inyectable) para poder testearlas
 * sin reloj real. Todo degrada en silencio si localStorage no está disponible.
 */

/** Opción de expiración ofrecida al fijar el estado. */
export type StatusExpiry = 'never' | '30m' | '1h' | 'today' | 'week';

/**
 * Opciones del menú, en orden. Solo el VALOR vive aquí: la etiqueta es
 * `status.expiry.<valor>` del catálogo y la resuelve quien pinta el menú.
 */
export const EXPIRY_OPTIONS: readonly StatusExpiry[] = ['never', '30m', '1h', 'today', 'week'];

/**
 * Instante (epoch ms) en que caduca el estado según la opción, o null si «No borrar».
 * Puro (`now` inyectable). «Hoy» = próxima medianoche LOCAL; «Esta semana» = medianoche
 * local del próximo lunes (fin de la semana laboral europea). Usa setDate/setHours para
 * respetar el huso y los cambios de hora locales.
 */
export function expiryAt(choice: StatusExpiry, now: number = Date.now()): number | null {
  switch (choice) {
    case 'never':
      return null;
    case '30m':
      return now + 30 * 60_000;
    case '1h':
      return now + 60 * 60_000;
    case 'today': {
      const end = new Date(now);
      end.setHours(24, 0, 0, 0); // próxima medianoche local (fin de hoy)
      return end.getTime();
    }
    case 'week': {
      const end = new Date(now);
      end.setHours(24, 0, 0, 0); // arranca en la próxima medianoche local
      // Avanza hasta el próximo lunes (getDay: 0=dom … 1=lun). Máx. 6 iteraciones.
      while (end.getDay() !== 1) end.setDate(end.getDate() + 1);
      return end.getTime();
    }
  }
}

/** ¿Ha caducado un estado con vencimiento `expiresAt` (epoch ms; null = nunca)? Puro. */
export function isExpired(expiresAt: number | null, now: number = Date.now()): boolean {
  return expiresAt != null && now >= expiresAt;
}

/** Clave de localStorage del estado personalizado propio + su caducidad. */
const STATUS_KEY = 'sytue-rooms:custom-status';

interface StoredStatus {
  status: string;
  expiresAt: number | null;
}

/** Lee el JSON persistido (o null si no hay / no se puede leer). */
function read(): StoredStatus | null {
  try {
    const raw = localStorage.getItem(STATUS_KEY);
    if (!raw) return null;
    const s = JSON.parse(raw) as StoredStatus;
    return typeof s?.status === 'string'
      ? { status: s.status, expiresAt: typeof s.expiresAt === 'number' ? s.expiresAt : null }
      : null;
  } catch {
    return null;
  }
}

/**
 * Restaura el estado personalizado persistido, CADUCÁNDOLO si venció (efecto: borra la
 * clave). Devuelve '' si no hay estado o ya expiró. Es el valor con el que arranca el
 * heartbeat propio al montar Rooms. `now` inyectable para test.
 */
export function loadCustomStatus(now: number = Date.now()): string {
  const s = read();
  if (!s) return '';
  if (isExpired(s.expiresAt, now)) {
    clearCustomStatus();
    return '';
  }
  return s.status;
}

/** Vencimiento persistido (epoch ms; null = no caduca / no hay). Para programar el timer. */
export function storedExpiry(): number | null {
  return read()?.expiresAt ?? null;
}

/** Persiste el estado con su caducidad; `status` vacío borra la clave (⟺ sin estado). */
export function saveCustomStatus(status: string, expiresAt: number | null): void {
  try {
    if (!status) localStorage.removeItem(STATUS_KEY);
    else localStorage.setItem(STATUS_KEY, JSON.stringify({ status, expiresAt }));
  } catch {
    // Almacenamiento no disponible (SSR/privado): el estado vive solo en memoria.
  }
}

/** Borra el estado personalizado persistido. */
export function clearCustomStatus(): void {
  try {
    localStorage.removeItem(STATUS_KEY);
  } catch {
    // Nada que borrar si no hay almacenamiento.
  }
}
