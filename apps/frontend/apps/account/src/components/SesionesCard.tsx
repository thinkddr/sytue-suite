/*
 * Sesiones abiertas. Lista lo que devuelve `GET /api/v1/me/sessions` y permite
 * cerrar una, o TODAS —esta incluida—.
 *
 * La sesión EN USO se marca y no lleva aspa: cerrarla de una en una expulsaría
 * por descuido a quien está mirando la pantalla, que es lo que hace el botón de
 * salir y no esta lista.
 *
 * El botón de arriba es otra cosa y hasta hoy MENTÍA. Decía «cerrar las demás»,
 * pero `DELETE /api/v1/me/sessions` solo perdona la actual si la petición trae la
 * cookie de sesión del IdP, y desde la web NUNCA la trae: el BFF llama a Sytue ID
 * con el bearer y nada más (`proxyId` en `lib/bff.ts`). Sin cookie el servicio no
 * puede saber cuál es la tuya y las cierra todas, que es la opción segura y está
 * escrita así a propósito (`services/id/internal/http/me_sessions.go`). Lo que
 * faltaba era decirlo: el rótulo dice «todas» y la nota avisa de que hay que
 * volver a entrar.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, EmptyState, Icon } from '@sytue/ui';
import type { T } from '../i18n/index.js';

interface Sesion {
  id: string;
  ip: string;
  user_agent: string;
  device: string;
  auth_time: string;
  expires_at: string;
  current: boolean;
}

export function SesionesCard({ t }: { t: T }): JSX.Element {
  const [sesiones, setSesiones] = useState<Sesion[] | null>(null);
  const [error, setError] = useState(false);

  async function cargar(): Promise<void> {
    try {
      const res = await fetch('/api/sessions');
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { sessions?: Sesion[] };
      setSesiones(datos.sessions ?? []);
      setError(false);
    } catch {
      setError(true);
    }
  }

  useEffect(() => {
    void cargar();
  }, []);

  async function cerrar(id?: string): Promise<void> {
    // Optimista NO: cerrar una sesión es irreversible y el listado es corto, así
    // que se relee. Si el servidor falla, la lista sigue diciendo la verdad.
    await fetch(`/api/sessions${id ? `/${id}` : ''}`, { method: 'DELETE' });
    await cargar();
  }

  const otras = (sesiones ?? []).filter((s) => !s.current).length;

  return (
    <Card class="mb-6 break-inside-avoid">
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-lg font-semibold">{t('sessions.title')}</h2>
          {otras > 0 && (
            <Button variant="ghost" onClick={() => void cerrar()}>
              {t('sessions.close_others')}
            </Button>
          )}
        </div>
        {otras > 0 && (
          <p class="text-text-muted -mt-2 text-xs">{t('sessions.close_others_note')}</p>
        )}

        {/* `danger-text`, no `danger`: el rojo de relleno sobre la superficie
            oscura da 3,17:1 y el texto necesita 4,5:1. */}
        {error && <p class="text-danger-text text-sm">{t('sessions.failed')}</p>}
        {!error && sesiones === null && (
          <p class="text-text-muted text-sm">{t('common.loading')}</p>
        )}
        {!error && sesiones?.length === 0 && <EmptyState title={t('sessions.empty')} />}

        <ul class="flex flex-col gap-3">
          {(sesiones ?? []).map((s) => (
            <li key={s.id} class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-medium">
                  {s.device}
                  {s.current && (
                    <span class="text-success ml-2 text-xs">{t('sessions.current')}</span>
                  )}
                </p>
                {/* El User-Agent completo en el title: el resumen legible basta
                    para reconocer el dispositivo, y el literal sirve cuando no. */}
                <p class="text-text-muted truncate text-xs" title={s.user_agent}>
                  {s.ip}
                </p>
              </div>
              {!s.current && (
                <button
                  type="button"
                  onClick={() => void cerrar(s.id)}
                  aria-label={t('sessions.close_one')}
                  class="text-text-muted hover:text-danger shrink-0 p-1"
                >
                  <Icon name="close" />
                </button>
              )}
            </li>
          ))}
        </ul>
      </div>
    </Card>
  );
}

export default SesionesCard;
