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
 *
 * Olvidar es irreversible (quita la confianza y el dispositivo vuelve a ser un
 * desconocido), así que se confirma nombrándolo, y si el servicio lo rechaza la
 * tarjeta lo dice.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, EmptyState, Icon, IconButton, useConfirm } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { primeraLectura } from '../lib/primeraLectura.js';
import { FilasCargando } from './FilasCargando.js';

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
  // Confiar y olvidar comparten aviso: los dos dicen «no se ha podido» con el
  // mismo texto y nunca ocurren a la vez.
  const [falloAccion, setFalloAccion] = useState(false);
  const { confirmar, dialogo } = useConfirm();

  // `inicial`: la del montaje se comparte con la revisión de seguridad.
  async function cargar(inicial = false): Promise<void> {
    setError(false);
    try {
      const res = await (inicial ? primeraLectura('/api/devices') : fetch('/api/devices'));
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as { devices?: Dispositivo[] };
      setEquipos(datos.devices ?? []);
      setError(false);
    } catch {
      setError(true);
    }
  }

  useEffect(() => {
    void cargar(true);
  }, []);

  // Se relee en vez de tocar la lista en memoria, por lo mismo que en sesiones: si
  // el servicio rechaza la operación, lo que se ve sigue siendo lo que hay.
  async function olvidar(d: Dispositivo): Promise<void> {
    const ok = await confirmar({
      mensaje: t('devices.forget_confirm', { device: d.name }),
      confirmLabel: t('devices.forget'),
      tone: 'danger',
    });
    if (!ok) return;
    setFalloAccion(false);
    try {
      const res = await fetch(`/api/devices/${d.id}`, { method: 'DELETE' });
      if (!res.ok) setFalloAccion(true);
    } catch {
      setFalloAccion(true);
    }
    await cargar();
  }

  // Ruta de Go, no del BFF: absoluta a propósito, sin el prefijo de la app. Es de
  // Sytue ID en `id.<dominio>`, que es donde se sirve esta app (ingress del chart
  // de `id`, `rutasDeCuenta`).
  async function confiar(): Promise<void> {
    try {
      const res = await fetch('/account/devices/trust', { method: 'POST' });
      setFalloAccion(!res.ok);
      if (res.ok) setActual(((await res.json()) as { id?: string }).id ?? null);
    } catch {
      setFalloAccion(true);
    }
    await cargar();
  }

  const fecha = (iso: string): string => {
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(locale, { dateStyle: 'medium' });
  };

  return (
    <Card id="dispositivos">
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        <h2 class="text-lg font-semibold">{t('devices.title')}</h2>

        {error && (
          <div role="alert" class="flex items-center justify-between gap-3">
            <p class="text-danger-text text-sm">{t('devices.failed')}</p>
            <Button variant="ghost" onClick={() => void cargar()}>
              {t('common.retry')}
            </Button>
          </div>
        )}
        {!error && equipos === null && <FilasCargando etiqueta={t('common.loading')} />}
        {!error && equipos?.length === 0 && <EmptyState title={t('devices.empty')} />}

        {!(equipos ?? []).some((d) => d.id === actual && d.trusted) && (
          <div class="flex items-center justify-between gap-3">
            <p class="text-sm">{t('devices.current')}</p>
            <Button variant="ghost" onClick={() => void confiar()}>
              {t('devices.trust')}
            </Button>
          </div>
        )}
        {falloAccion && (
          <p role="alert" class="text-danger-text text-sm">
            {t('security.failed')}
          </p>
        )}

        <ul class="flex flex-col gap-3">
          {(equipos ?? []).map((d) => (
            <li key={d.id} class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-medium" title={d.user_agent}>
                  {d.name}
                  {(d.current || d.id === actual) && (
                    <span class="text-success-text ml-2 text-xs">{t('devices.current')}</span>
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
                <IconButton
                  size="sm"
                  tone="danger"
                  label={t('devices.forget_named', { device: d.name })}
                  onClick={() => void olvidar(d)}
                >
                  <Icon name="close" />
                </IconButton>
              </div>
            </li>
          ))}
        </ul>
      </div>
      {dialogo}
    </Card>
  );
}

export default DispositivosCard;
