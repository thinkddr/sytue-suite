/*
 * La firma guardada. CertiPDF la guarda, cifrada en la cuenta, cuando al firmar a
 * mano se marca «Recordar mi firma»; aquí se ve y se borra. Es un dato que permite
 * firmar en nombre de alguien, así que tiene que poder quitarse desde el sitio
 * donde se gestiona la cuenta, sin tener que abrir un PDF para ello.
 *
 * La vista previa se pinta con los mismos trazos que guarda el lienzo de CertiPDF
 * (480×148): una imagen guardada aparte podría no coincidir con lo que se estampa.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Card, useConfirm, EmptyState } from '@sytue/ui';
import type { T } from '../i18n/index.js';
import { FilasCargando } from './FilasCargando.js';

interface Rubrica {
  trazos: [number, number][][];
  updated_at?: string;
}

/** `null` cargando, `false` sin firma guardada. */
type Estado = Rubrica | false | null;

export function RubricaCard({ t, locale }: { t: T; locale: string }): JSX.Element {
  const [rubrica, setRubrica] = useState<Estado>(null);
  const [error, setError] = useState(false);
  const [falloBorrar, setFalloBorrar] = useState(false);
  const { confirmar, dialogo } = useConfirm();

  async function cargar(): Promise<void> {
    setError(false);
    try {
      const res = await fetch('/api/rubrica');
      if (res.status === 404) {
        setRubrica(false);
        return;
      }
      if (!res.ok) throw new Error(String(res.status));
      const datos = (await res.json()) as Rubrica;
      setRubrica(Array.isArray(datos.trazos) && datos.trazos.length > 0 ? datos : false);
    } catch {
      setError(true);
    }
  }

  useEffect(() => {
    void cargar();
  }, []);

  async function borrar(): Promise<void> {
    const ok = await confirmar({
      mensaje: t('signature.delete_confirm'),
      confirmLabel: t('signature.delete'),
      tone: 'danger',
    });
    if (!ok) return;
    setFalloBorrar(false);
    try {
      const res = await fetch('/api/rubrica', { method: 'DELETE' });
      if (!res.ok) setFalloBorrar(true);
    } catch {
      setFalloBorrar(true);
    }
    await cargar();
  }

  const fecha = (iso: string | undefined): string => {
    const d = new Date(iso ?? '');
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString(locale, { dateStyle: 'medium' });
  };

  return (
    <Card id="firma">
      <div class="flex flex-col gap-4 p-4 sm:p-5">
        <div>
          <h2 class="text-lg font-semibold">{t('signature.title')}</h2>
          <p class="text-text-muted mt-1 text-sm text-pretty">{t('signature.help')}</p>
        </div>

        {error && (
          <div role="alert" class="flex items-center justify-between gap-3">
            <p class="text-danger-text text-sm">{t('signature.failed')}</p>
            <Button variant="ghost" onClick={() => void cargar()}>
              {t('common.retry')}
            </Button>
          </div>
        )}
        {!error && rubrica === null && <FilasCargando etiqueta={t('common.loading')} />}
        {!error && rubrica === false && <EmptyState compact title={t('signature.empty')} />}
        {!error && rubrica && (
          <div class="flex flex-col gap-3">
            <svg
              viewBox="0 0 480 148"
              role="img"
              aria-label={t('signature.preview')}
              data-vista-firma
              class="border-border bg-surface text-text h-auto w-full max-w-sm rounded-md border"
            >
              {rubrica.trazos.map((trazo, i) => (
                <polyline
                  key={i}
                  points={trazo.map(([x, y]) => `${x},${y}`).join(' ')}
                  fill="none"
                  stroke="currentColor"
                  stroke-width={2.5}
                  stroke-linecap="round"
                  stroke-linejoin="round"
                />
              ))}
            </svg>
            <div class="flex flex-wrap items-center justify-between gap-3">
              <p class="text-text-muted text-xs">
                {fecha(rubrica.updated_at) &&
                  t('signature.saved_on', { date: fecha(rubrica.updated_at) })}
              </p>
              <Button variant="ghost" onClick={() => void borrar()}>
                {t('signature.delete')}
              </Button>
            </div>
          </div>
        )}
        {falloBorrar && (
          <p role="alert" class="text-danger-text text-sm">
            {t('security.failed')}
          </p>
        )}
      </div>
      {dialogo}
    </Card>
  );
}

export default RubricaCard;
