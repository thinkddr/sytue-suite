/**
 * «Un día como hoy»: las fotos de este mismo día en años anteriores, en una
 * tira sobre la galería.
 *
 * SE CALLA CUANDO NO HAY NADA, que es la mayoría de los días. Una sección con
 * un «no hay recuerdos» permanente ocupa sitio arriba del todo para no decir
 * nada, y acaba siendo lo primero que la gente aprende a ignorar.
 *
 * SE PUEDE OCULTAR, y se recuerda. Un aniversario no siempre se quiere ver: si
 * alguien lo cierra hoy, no vuelve a salir hoy.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Icon, useAhora } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { recuerdos, textoAlternativo, urlDeVariante, type Recuerdo } from '../client/photosApi.js';
import { proporcion } from '../lib/filas.js';
import { useT } from '../i18n/index.js';

/** Clave del día, para que ocultar valga solo para hoy. */
const claveDeHoy = () => `sytue-fotos-recuerdos-ocultos:${new Date().toISOString().slice(0, 10)}`;

export function Recuerdos({
  locale,
  onAbrir,
}: {
  locale: Locale;
  /** Abrir una foto en el visor de la galería, para no montar otro. */
  onAbrir?: ((id: string) => void) | undefined;
}): JSX.Element | null {
  const t = useT(locale);
  const ahora = useAhora();
  const [lista, setLista] = useState<Recuerdo[] | null>(null);
  const [oculto, setOculto] = useState(true);

  useEffect(() => {
    // El acceso va en try: en una ventana privada o con el almacenamiento
    // bloqueado, `localStorage` lanza, y un recuerdo no puede tumbar la galería.
    let yaOculto = false;
    try {
      yaOculto = window.localStorage.getItem(claveDeHoy()) === '1';
    } catch {
      yaOculto = false;
    }
    setOculto(yaOculto);
    if (yaOculto) return;
    recuerdos()
      .then(setLista)
      // Sin recuerdos no se dice nada: no es un error, es un día cualquiera.
      .catch(() => setLista([]));
  }, []);

  const ocultar = () => {
    setOculto(true);
    try {
      window.localStorage.setItem(claveDeHoy(), '1');
    } catch {
      // Sin almacenamiento, ocultar dura lo que dure la pestaña. Suficiente.
    }
  };

  if (oculto || lista === null || lista.length === 0) return null;

  const hoy = ahora.getFullYear();
  return (
    <section class="border-border mb-6 rounded-md border p-4" aria-label={t('memories.title')}>
      <header class="mb-3 flex items-center gap-2">
        <Icon name="clock" size={16} />
        <h2 class="text-sm font-semibold">{t('memories.title')}</h2>
        <Button variant="ghost" size="xs" class="ml-auto" onClick={ocultar}>
          {t('memories.hide')}
        </Button>
      </header>
      {lista.map((r) => (
        <div key={r.year} class="mb-3 last:mb-0">
          <p class="text-text-muted mb-1 text-xs">
            {hoy - r.year === 1
              ? t('memories.oneYear')
              : t('memories.years', { count: hoy - r.year })}
          </p>
          {/* Tira horizontal: un recuerdo es para echar un vistazo, no para
              recorrer una cuadrícula. Cada foto con su proporción, como en la
              galería de debajo: mismo alto, el ancho que le toque.
              `data-scroll-x` declara que la tira se desplaza de lado a propósito. */}
          <ul data-scroll-x class="flex gap-2 overflow-x-auto pb-1">
            {r.items.map((f) => (
              <li key={f.id} class="shrink-0">
                <button
                  type="button"
                  class="sytue-focus-ring block rounded-sm"
                  onClick={() => onAbrir?.(f.id)}
                  aria-label={textoAlternativo(f)}
                >
                  <img
                    src={urlDeVariante(f.id, 'miniatura')}
                    alt={textoAlternativo(f)}
                    width={f.width}
                    height={f.height}
                    loading="lazy"
                    decoding="async"
                    style={{ aspectRatio: proporcion(f) }}
                    class="bg-bg-subtle h-24 w-auto max-w-none rounded-sm object-cover"
                  />
                </button>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </section>
  );
}
