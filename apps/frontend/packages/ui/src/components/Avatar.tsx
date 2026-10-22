/**
 * Avatar — usuario o grupo; imagen o iniciales; punto de presencia opcional.
 * Presencia con texto alternativo y con FORMA, no solo color (WCAG 1.4.1): en línea es
 * el disco lleno, ausente una media luna, no molestar el disco con una raya y
 * desconectado un aro hueco. docs/design.md §9.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { cx } from '../lib/cx.js';
import type { PresenceStatus } from '@sytue/config';
import { useI18n } from '@sytue/i18n/preact';

export type AvatarSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl';

export interface AvatarProps {
  /** Nombre para iniciales y aria-label. */
  name: string;
  /** URL de imagen (si existe se prefiere a las iniciales). */
  src?: string | undefined;
  size?: AvatarSize;
  presence?: PresenceStatus;
  class?: string;
}

const sizePx: Record<AvatarSize, number> = { xs: 24, sm: 32, md: 40, lg: 48, xl: 64 };

/**
 * La forma de cada presencia DENTRO del disco de color, en `surface` (el color del aro
 * que lo separa del avatar): la muesca de la luna, la raya y el hueco del aro. Las
 * medidas son fracciones del disco, las mismas que pinta `ui.Avatar` en Fyne.
 */
const FORMA: Record<PresenceStatus, JSX.CSSProperties | null> = {
  online: null,
  // Media luna: un disco del 70 % desplazado arriba a la derecha tapa el resto.
  away: { width: '70%', height: '70%', left: '35%', top: '-5%', borderRadius: '9999px' },
  // Raya horizontal centrada: el «prohibido el paso» de no molestar.
  dnd: {
    width: '60%',
    height: 'max(1px, 22%)',
    left: '20%',
    top: '50%',
    transform: 'translateY(-50%)',
  },
  // Aro hueco: el agujero es la mitad del disco.
  offline: { width: '50%', height: '50%', left: '25%', top: '25%', borderRadius: '9999px' },
};

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return (parts[0] ?? '?').slice(0, 2).toUpperCase();
  return ((parts[0]?.[0] ?? '') + (parts[parts.length - 1]?.[0] ?? '')).toUpperCase();
}

/** Color de fondo deterministico a partir del nombre (solo tokens de marca). */
function bgFor(name: string): string {
  const palette = [
    // Seis colores de persona bien separados; iniciales en blanco AA (≥ 4,6:1).
    'var(--sytue-avatar-1)',
    'var(--sytue-avatar-2)',
    'var(--sytue-avatar-3)',
    'var(--sytue-avatar-4)',
    'var(--sytue-avatar-5)',
    'var(--sytue-avatar-6)',
  ];
  let hash = 0;
  for (let i = 0; i < name.length; i += 1) hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
  return palette[hash % palette.length] ?? palette[0]!;
}

export function Avatar({ name, src, size = 'md', presence, class: cls }: AvatarProps): JSX.Element {
  const { t } = useI18n();
  const px = sizePx[size];
  // El punto lleva dentro su FORMA (ver FORMA), y a 8 px —con el aro de 2 dentro— el
  // disco se quedaba en 4 y la luna o la raya no se distinguían: 10 como mínimo y un
  // tercio del avatar, así un `sm` tiene un disco de 7.
  const dot = Math.max(10, Math.round(px * 0.34));
  // Una foto que no carga dejaba un CUADRO VACÍO donde debería estar la persona:
  // el `<img>` roto conserva su caja de 32 px con el `ring`, así que se lee como
  // un hueco gris en la topbar de la suite, no como un fallo. Pasa en cuanto la
  // URL del proveedor de identidad caduca, cambia de dominio o la bloquea el
  // navegador —una foto ajena es una petición a un tercero—, y no hay forma de
  // que el usuario lo entienda ni lo arregle.
  //
  // Se recuerda la src que falló en vez de un booleano: si el usuario cambia de
  // foto, la nueva se intenta sola, sin efecto que resetear.
  const [rota, setRota] = useState<string | null>(null);
  // Y el `onError` solo no basta: la barra se pinta en el SERVIDOR, así que el
  // `<img>` empieza a cargar antes de que Preact hidrate, y si falla en ese rato el
  // evento se pierde —aún no había nadie escuchando—. Quedaba el icono de imagen
  // rota, que es justo lo que el dueño vio en Drive el 2026-09-24. Al montar se
  // mira si ya falló: `complete` sin píxeles es un `<img>` roto (uno que aún carga,
  // también el diferido, no está `complete`).
  const img = useRef<HTMLImageElement>(null);
  useEffect(() => {
    const el = img.current;
    if (src && el?.complete && el.naturalWidth === 0) setRota(src);
  }, [src]);
  return (
    <span
      class={cx('relative inline-flex shrink-0 select-none', cls)}
      style={{ width: px, height: px }}
    >
      {src && rota !== src ? (
        <img
          ref={img}
          src={src}
          alt={name}
          width={px}
          height={px}
          // Una lista de personas dispara UNA petición por fila: sin esto, abrir
          // una libreta con foto en cada contacto encola cientos de GET a la vez,
          // casi todos por debajo del pliegue. Ningún avatar es nunca el LCP, y
          // los que ya están en el viewport (el de la topbar) Chrome los carga
          // igual en el primer pintado: `lazy` solo pospone lo que no se ve.
          loading="lazy"
          decoding="async"
          onError={() => setRota(src)}
          class="h-full w-full rounded-full object-cover ring-1 ring-[var(--sytue-border)] ring-inset"
        />
      ) : (
        <span
          class="flex h-full w-full items-center justify-center rounded-full font-semibold text-white"
          style={{ backgroundColor: bgFor(name), fontSize: Math.round(px * 0.4) }}
          aria-label={name}
          role="img"
        >
          {initials(name)}
        </span>
      )}
      {presence ? (
        <span
          class="border-surface absolute -right-0.5 -bottom-0.5 overflow-hidden rounded-full border-2"
          style={{ width: dot, height: dot, backgroundColor: `var(--sytue-presence-${presence})` }}
          role="img"
          aria-label={t(`presence.${presence}`)}
          title={t(`presence.${presence}`)}
          data-presencia={presence}
        >
          {FORMA[presence] ? (
            <span class="bg-surface absolute" style={FORMA[presence]} aria-hidden="true" />
          ) : null}
        </span>
      ) : null}
    </span>
  );
}
