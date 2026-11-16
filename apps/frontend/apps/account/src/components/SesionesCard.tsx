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
 *
 * Las dos salidas son irreversibles y por eso se confirman, diciendo QUÉ sesión
 * se cierra (el aspa lleva el nombre del dispositivo) y qué pasa después. Y la
 * respuesta se mira: si el `DELETE` falla, la tarjeta lo dice en vez de releer la
 * lista y callar.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, EmptyState, Icon, IconButton, useConfirm } from '@sytue/ui';
import { formatTimeAgo, type Locale } from '@sytue/i18n';
import type { T } from '../i18n/index.js';
import { primeraLectura } from '../lib/primeraLectura.js';
import { FilasCargando } from './FilasCargando.js';

interface Sesion {
  id: string;
  ip: string;
  user_agent: string;
  device: string;
  auth_time: string;
  expires_at: string;
  current: boolean;
}

export function SesionesCard({ t, locale }: { t: T; locale: Locale }): JSX.Element {
  const [sesiones, setSesiones] = useState<Sesion[] | null>(null);
  const [error, setError] = useState(false);
  const [falloAccion, setFalloAccion] = useState(false);
  const { confirmar, dialogo } = useConfirm();

  // `inicial`: la del montaje se comparte con la revisión de seguridad.
  async function cargar(inicial = false): Promise<void> {
    setError(false);
    try {
      const res = await (inicial ? primeraLectura('/api/sessions') : fetch('/api/sessions'));
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { sessions?: Sesion[] };
      setSesiones(datos.sessions ?? []);
      setError(false);
    } catch {
      setError(true);
    }
  }

  useEffect(() => {
    void cargar(true);
  }, []);

  async function cerrar(sesion?: Sesion): Promise<void> {
    const ok = await confirmar(
      sesion
        ? {
            mensaje: t('sessions.close_one_confirm', { device: sesion.device }),
            confirmLabel: t('sessions.close_one'),
            tone: 'danger',
          }
        : {
            mensaje: t('sessions.close_all_confirm'),
            confirmLabel: t('sessions.close_others'),
            tone: 'danger',
          },
    );
    if (!ok) return;
    setFalloAccion(false);
    // Optimista NO: cerrar una sesión es irreversible y el listado es corto, así
    // que se relee. Si el servidor falla, la lista sigue diciendo la verdad y la
    // tarjeta avisa de que no se ha cerrado nada.
    try {
      const res = await fetch(`/api/sessions${sesion ? `/${sesion.id}` : ''}`, {
        method: 'DELETE',
      });
      if (!res.ok) setFalloAccion(true);
    } catch {
      setFalloAccion(true);
    }
    await cargar();
  }

  const inicio = (iso: string): string => {
    const d = new Date(iso);
    return Number.isNaN(d.getTime())
      ? ''
      : t('sessions.signed_in', { time: formatTimeAgo(locale, d) });
  };

  const otras = (sesiones ?? []).filter((s) => !s.current).length;

  return (
    <Card id="sesiones">
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
        {error && (
          <div role="alert" class="flex items-center justify-between gap-3">
            <p class="text-danger-text text-sm">{t('sessions.failed')}</p>
            <Button variant="ghost" onClick={() => void cargar()}>
              {t('common.retry')}
            </Button>
          </div>
        )}
        {falloAccion && (
          <p role="alert" class="text-danger-text text-sm">
            {t('security.failed')}
          </p>
        )}
        {!error && sesiones === null && <FilasCargando etiqueta={t('common.loading')} />}
        {!error && sesiones?.length === 0 && <EmptyState title={t('sessions.empty')} />}

        <ul class="flex flex-col gap-3">
          {(sesiones ?? []).map((s) => (
            <li key={s.id} class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-medium">
                  {s.device}
                  {s.current && (
                    <span class="text-success-text ml-2 text-xs">{t('sessions.current')}</span>
                  )}
                </p>
                {/* El User-Agent completo en el title: el resumen legible basta
                    para reconocer el dispositivo, y el literal sirve cuando no. */}
                <p class="text-text-muted truncate text-xs" title={s.user_agent}>
                  {[s.ip, inicio(s.auth_time)].filter(Boolean).join(' · ')}
                </p>
              </div>
              {/* `IconButton` y no un `<button>` a pelo: trae el anillo de foco y la
                  diana de 24 px (44 con el dedo). El nombre lleva el dispositivo:
                  con un rótulo igual para todas, un lector de pantalla oía «Cerrar
                  esta sesión» tres veces sin saber cuál. */}
              {!s.current && (
                <IconButton
                  size="sm"
                  tone="danger"
                  label={t('sessions.close_named', { device: s.device })}
                  onClick={() => void cerrar(s)}
                >
                  <Icon name="close" />
                </IconButton>
              )}
            </li>
          ))}
        </ul>
      </div>
      {dialogo}
    </Card>
  );
}

export default SesionesCard;
