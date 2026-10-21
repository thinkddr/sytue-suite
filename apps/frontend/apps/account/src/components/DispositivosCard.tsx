/*
 * Dispositivos reconocidos. Lista lo que devuelve `GET /api/v1/me/devices`,
 * permite olvidar cualquiera y marcar de confianza el actual.
 *
 * «De confianza» es solo para el dispositivo desde el que se está mirando: Sytue ID
 * exige prueba de posesión de SU cookie firmada. Esta app no la ve ni la reenvía
 * (docs/security.md#riesgo-aceptado-mi-cuenta-en-el-origen-del-idp), así que el
 * navegador pide a Go directamente, `POST /account/devices/trust`, y la cookie va
 * sola. Como la lista llega sin ella, cuál es «este dispositivo» se sabe al
 * marcarlo: el servicio devuelve cuál era. Un dispositivo robado no se marca a
 * distancia; se olvida.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, EmptyState, Icon } from '@sytue/ui';
import type { T } from '../i18n/index.js';

interface Dispositivo {
  id: string;
  name: string;
  user_agent: string;
  trusted: boolean;
  last_seen: string;
  last_ip: string;
  current: boolean;
}

export function DispositivosCard({ t, locale }: { t: T; locale: string }): JSX.Element {
  const [equipos, setEquipos] = useState<Dispositivo[] | null>(null);
  const [error, setError] = useState(false);
  const [actual, setActual] = useState<string | null>(null);
  const [falloConfianza, setFalloConfianza] = useState(false);

  async function cargar(): Promise<void> {
    try {
      const res = await fetch('/api/devices');
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { devices?: Dispositivo[] };
      setEquipos(datos.devices ?? []);
      setError(false);
    } catch {
      setError(true);
    }
  }

  useEffect(() => {
    void cargar();
  }, []);

  // Se relee en vez de tocar la lista en memoria, por lo mismo que en sesiones: si
  // el servicio rechaza la operación, lo que se ve sigue siendo lo que hay.
  async function olvidar(id: string): Promise<void> {
    await fetch(`/api/devices/${id}`, { method: 'DELETE' });
    await cargar();
  }

  // Ruta de Go, no del BFF: absoluta a propósito, sin el prefijo de la app. Es de
  // Sytue ID en `id.<dominio>`, que es donde se sirve esta app (ingress del chart
  // de `id`, `rutasDeCuenta`).
  async function confiar(): Promise<void> {
    const res = await fetch('/account/devices/trust', { method: 'POST' });
    setFalloConfianza(!res.ok);
    if (res.ok) setActual(((await res.json()) as { id?: string }).id ?? null);
    await cargar();
  }

  const fecha = (iso: string): string => {
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(locale, { dateStyle: 'medium' });
  };

  return (
    <Card class="mb-6 break-inside-avoid">
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('devices.title')}</h2>

        {error && <p class="text-danger text-sm">{t('devices.failed')}</p>}
        {!error && equipos === null && <p class="text-text-muted text-sm">{t('common.loading')}</p>}
        {!error && equipos?.length === 0 && <EmptyState title={t('devices.empty')} />}

        {!(equipos ?? []).some((d) => d.id === actual && d.trusted) && (
          <div class="flex items-center justify-between gap-3">
            <p class="text-sm">{t('devices.current')}</p>
            <Button variant="ghost" onClick={() => void confiar()}>
              {t('devices.trust')}
            </Button>
          </div>
        )}
        {falloConfianza && <p class="text-danger text-sm">{t('security.failed')}</p>}

        <ul class="flex flex-col gap-3">
          {(equipos ?? []).map((d) => (
            <li key={d.id} class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-medium" title={d.user_agent}>
                  {d.name}
                  {(d.current || d.id === actual) && (
                    <span class="text-success ml-2 text-xs">{t('devices.current')}</span>
                  )}
                  {d.trusted && (
                    <span class="text-text-muted ml-2 text-xs">{t('devices.trusted')}</span>
                  )}
                </p>
                <p class="text-text-muted truncate text-xs">
                  {[d.last_ip, fecha(d.last_seen)].filter(Boolean).join(' · ')}
                </p>
              </div>
              <div class="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  onClick={() => void olvidar(d.id)}
                  aria-label={t('devices.forget')}
                  class="text-text-muted hover:text-danger p-1"
                >
                  <Icon name="close" />
                </button>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </Card>
  );
}

export default DispositivosCard;
