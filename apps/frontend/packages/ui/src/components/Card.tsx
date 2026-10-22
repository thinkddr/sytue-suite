/**
 * Card — superficie blanca que se despega del lienzo con sombra suave (radio lg), con
 * cabecera/cuerpo/pie opcionales. El color de grupo de un recurso NO tiñe la tarjeta:
 * va en su icono, sobre un relleno sólido (`CardIcon`). docs/design.md §9.
 */
import type { ComponentChildren, JSX } from 'preact';
import { cx } from '../lib/cx.js';

export const CARD_TONES = [
  'blue',
  'violet',
  'mint',
  'rose',
  'amber',
  'cyan',
  'coral',
  'lime',
  'slate',
] as const;
export type CardTone = (typeof CARD_TONES)[number] | 'neutral';
const solidClass: Record<CardTone, string> = {
  blue: 'bg-card-solid-blue text-on-card-solid-blue',
  violet: 'bg-card-solid-violet text-on-card-solid-violet',
  mint: 'bg-card-solid-mint text-on-card-solid-mint',
  rose: 'bg-card-solid-rose text-on-card-solid-rose',
  amber: 'bg-card-solid-amber text-on-card-solid-amber',
  cyan: 'bg-card-solid-cyan text-on-card-solid-cyan',
  coral: 'bg-card-solid-coral text-on-card-solid-coral',
  lime: 'bg-card-solid-lime text-on-card-solid-lime',
  slate: 'bg-card-solid-slate text-on-card-solid-slate',
  neutral: 'bg-card-solid-slate text-on-card-solid-slate',
};

export interface CardProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /**
   * Color de grupo del recurso. Pinta la tarjeta con `fill="solid"` y, en la forma
   * «Marco», siempre que se pase: ahí las tarjetas de recurso son de color pleno
   * (`.sytue-card-tone`, en tokens/appearance.css). Sin `tone`, la tarjeta es un panel.
   */
  tone?: CardTone;
  /**
   * `tinted` (por defecto) es la superficie blanca: el nombre se conserva por los
   * consumidores, pero ya no hay fondo pastel. `solid` rellena la tarjeta con el tono.
   */
  fill?: 'tinted' | 'solid';
  /** Eleva con sombra (por defecto true). */
  raised?: boolean;
  /** Convierte la card en landmark `section` con aria-label. */
  sectionLabel?: string;
}

export function Card({
  tone,
  fill = 'tinted',
  raised = true,
  sectionLabel,
  class: cls,
  style,
  children,
  ...rest
}: CardProps): JSX.Element {
  // El relleno y su tinta viajan como variables: la hoja decide si los pinta (Marco).
  const recurso = tone !== undefined && fill !== 'solid' ? cardTone(tone) : null;
  return (
    <div
      class={cx(
        // min-w-0: una tarjeta casi siempre vive en una rejilla o en un flex, y
        // ahí NO baja de su ancho mínimo de contenido salvo que se le diga. Es lo
        // que sacaba el panel de «Organizaciones» fuera de la pantalla hasta en un
        // Pixel 7 (412px): la rejilla medía por la fila más rígida de dentro.
        'rounded-card min-w-0 border',
        // Sobre el blanco con sombra la línea es la suave: la sombra ya dibuja el canto.
        fill === 'solid'
          ? cx('border-border', solidClass[tone ?? 'blue'])
          : cx('sytue-glass-surface border-border-subtle bg-surface text-text', recurso?.class),
        // `raised={false}` significa PLANA. Antes daba `--shadow-xs` igualmente,
        // así que la prop mentía en los 11 sitios que la piden plana; y en tema
        // oscuro esa sombra es negro al 6 % sobre grafito: invisible, pero se
        // pinta. El borde hairline ya separa la tarjeta del fondo.
        raised ? 'shadow-[var(--shadow-sm)]' : '',
        cls,
      )}
      role={sectionLabel ? 'region' : undefined}
      aria-label={sectionLabel}
      style={
        recurso ? Object.assign({}, typeof style === 'object' ? style : null, recurso.style) : style
      }
      {...rest}
    >
      {children}
    </div>
  );
}

/**
 * Lo que hace de CUALQUIER elemento una tarjeta de recurso: la clase y las dos variables
 * (relleno y su tinta) que la hoja pinta en la forma «Marco» y deja estar en «Carril».
 * `Card` lo usa con su `tone`; lo que no es un `Card` —un enlace, la tesela de una
 * carpeta— lo reparte entre su `class` y su `style`. Dentro de un contenedor
 * `sytue-tiles`, además, esos elementos se disponen como teselas (appearance.css).
 */
export function cardTone(tone: CardTone): { class: string; style: Record<string, string> } {
  // `neutral` pinta con la familia grafito, como en `solidClass`.
  const t = tone === 'neutral' ? 'slate' : tone;
  return {
    class: 'sytue-card-tone',
    style: {
      '--sytue-card-fill': `var(--sytue-card-solid-${t})`,
      '--sytue-card-on-fill': `var(--sytue-on-card-solid-${t})`,
    },
  };
}

export interface CardIconProps {
  /** Color de grupo del recurso (los mismos nueve que `Card`). */
  tone?: CardTone;
  children: ComponentChildren;
  class?: string;
}

/** El icono de una tarjeta de recurso: glifo sobre un relleno SÓLIDO del color de grupo. */
export function CardIcon({ tone = 'blue', children, class: cls }: CardIconProps): JSX.Element {
  return (
    <span
      aria-hidden="true"
      class={cx(
        'sytue-card-icon grid h-8 w-8 shrink-0 place-items-center rounded-md',
        solidClass[tone],
        cls,
      )}
    >
      {children}
    </span>
  );
}

export function CardHeader({
  class: cls,
  children,
  ...rest
}: JSX.HTMLAttributes<HTMLDivElement>): JSX.Element {
  return (
    <div
      class={cx(
        // flex-wrap: el título y el select de filtro de la consola de plataforma
        // no caben en la misma línea de un iPhone SE («Was globale
        // Administratoren getan haben» + «Globaler Geltungsbereich» sumaban
        // ~432px de mínimo y empujaban main#contenido +59px fuera), así que en
        // estrecho el control baja a una segunda fila en vez de ensanchar la
        // tarjeta — el mismo criterio que ya usa SectionHeader.
        'border-border-subtle flex flex-wrap items-center justify-between gap-x-3 gap-y-2 border-b px-4 py-3',
        cls,
      )}
      {...rest}
    >
      {children}
    </div>
  );
}

export function CardTitle({
  children,
  as: Tag = 'h3',
}: {
  children: ComponentChildren;
  /** Nivel de encabezado para evitar saltos de jerarquía (WCAG 1.3.1); el
   *  estilo visual (sytue-h4) no cambia. Por defecto h3. */
  as?: 'h2' | 'h3' | 'h4';
}): JSX.Element {
  return <Tag class="sytue-h4">{children}</Tag>;
}

export function CardBody({
  class: cls,
  children,
  ...rest
}: JSX.HTMLAttributes<HTMLDivElement>): JSX.Element {
  return (
    <div class={cx('px-4 py-4', cls)} {...rest}>
      {children}
    </div>
  );
}

export function CardFooter({
  class: cls,
  children,
  ...rest
}: JSX.HTMLAttributes<HTMLDivElement>): JSX.Element {
  return (
    <div
      class={cx('border-border-subtle flex items-center justify-end gap-2 border-t px-4 py-3', cls)}
      {...rest}
    >
      {children}
    </div>
  );
}
