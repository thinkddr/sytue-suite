/**
 * DemoBanner — estado de origen de los datos de la página.
 *
 * Distingue dos situaciones que antes se presentaban IGUAL: no hay control-plane
 * conectado (modo demostración legítimo, datos de ejemplo) y el control-plane
 * respondió con un error —típicamente 403— en cuyo caso llamar a eso «demostración»
 * oculta el problema real: el usuario cree que está viendo datos de ejemplo cuando
 * en realidad no tiene permiso o el backend está fallando.
 */
import type { Locale } from '@sytue/i18n';
import { errorMessage, useT } from '../i18n/index.js';

interface Props {
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
  /** Error REAL del control-plane, si lo hubo (lo adjunta Sourced.error). */
  error?: { status: number; code: string } | undefined;
}

export default function DemoBanner({ live, locale, error }: Props) {
  const t = useT(locale);

  // El error manda sobre `live`. Cada página calcula `live` con las fuentes que
  // le parecen importantes, y basta con olvidar una para que su fallo quede
  // mudo: en `usage.astro` un 403 de /billing/account dejaba la página «viva»
  // pintando 0 € de gasto y una etiqueta verde de «al día». Si una fuente
  // RESPONDIÓ con error, no está viva, y decirlo aquí lo arregla para todas.
  if (error) {
    const denied = error.status === 403 || error.status === 401;
    return (
      <div
        class="border-danger bg-danger-bg text-danger-text border-b px-4 py-2 text-center text-sm"
        role="alert"
        // Marca para la auditoría de móvil: esta pantalla está pintando su cromo
        // SIN los datos. Sin ella, las ocho rutas de Cloud respondían 200 con esta
        // tarjeta y el barrido nocturno las contaba como medidas y aprobadas —120
        // de 299 combinaciones—. El estado HTTP no lo distingue y el texto depende
        // del idioma; el componente que lo sabe es este. Ver test/e2e/mobile-audit.mjs.
        data-sin-datos={denied ? 'permiso' : 'error'}
      >
        {denied
          ? t('demo.denied')
          : t('demo.loadError', { error: errorMessage(t, error.status, error.code) })}
      </div>
    );
  }
  if (live) return null;

  return (
    <div
      class="border-warning bg-warning-bg text-warning-text border-b px-4 py-2 text-center text-sm"
      role="status"
    >
      {t('demo.banner')}
    </div>
  );
}
