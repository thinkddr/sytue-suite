/**
 * BarChart — serie temporal en barras, en SVG y sin dependencias.
 *
 * Está en el design system y no en la app que lo estrenó por dos motivos que no
 * son de reutilización: los colores salen de los tokens (una gráfica con
 * `#4f46e5` a mano rompe la paleta en cuanto alguien mira el modo oscuro),
 * y la accesibilidad de un gráfico es fácil de olvidar una vez por app. Aquí el
 * SVG es `role="img"` con una descripción completa, y debajo va una `<table>`
 * visualmente oculta con los mismos datos: un lector de pantalla lee las cifras,
 * no «gráfico».
 *
 * Deliberadamente NO tiene ejes, leyenda, zoom ni tooltip flotante. El valor de
 * cada barra se lee al pasar por encima (título nativo) y el resto de la lectura
 * la da el texto que la rodea. Una biblioteca de gráficos pesa más que toda esta
 * página.
 */
import type { JSX } from 'preact';
import { cx } from '../lib/cx.js';

export interface BarChartPoint {
  /** Etiqueta del eje X, ya formateada por quien llama (fecha corta, hora…). */
  label: string;
  value: number;
}

export interface BarChartProps {
  points: BarChartPoint[];
  /** Qué mide la serie: se usa en la descripción accesible y en cada barra. */
  seriesLabel: string;
  /** Alto del área de dibujo en píxeles. El ancho es siempre el del contenedor. */
  height?: number;
  /** Texto para cuando no hay ni un punto. Sin él no se pinta nada. */
  emptyLabel?: string;
  /** Cabeceras de la tabla accesible: [columna de etiqueta, columna de valor]. */
  tableHeaders?: [string, string];
  class?: string;
}

/** Ancho del lienzo en unidades de usuario; el SVG escala al contenedor. */
const VIEW_W = 600;
const GAP = 2;

export function BarChart({
  points,
  seriesLabel,
  height = 120,
  emptyLabel,
  tableHeaders = ['', ''],
  class: cls,
}: BarChartProps): JSX.Element {
  if (points.length === 0) {
    return <p class={cx('text-text-subtle py-6 text-center text-sm', cls)}>{emptyLabel ?? ''}</p>;
  }

  // El máximo nunca es 0: dividir por él daría NaN y todas las barras
  // desaparecerían justo cuando el dato es «todo a cero», que es información.
  const max = Math.max(1, ...points.map((p) => p.value));
  const slot = VIEW_W / points.length;
  const barW = Math.max(1, slot - GAP);
  const total = points.reduce((n, p) => n + p.value, 0);

  return (
    <div class={cls}>
      <svg
        viewBox={`0 0 ${VIEW_W} ${height}`}
        width="100%"
        height={height}
        preserveAspectRatio="none"
        role="img"
        aria-label={`${seriesLabel}: ${total}`}
        class="overflow-visible"
      >
        {points.map((p, i) => {
          // Suelo de 1 unidad: una barra de alto 0 es indistinguible de «no hay
          // barra», y el usuario no puede saber si ese día no hubo visitas o si
          // el día no existe en la serie.
          const h = p.value === 0 ? 1 : Math.max(1, Math.round((p.value / max) * height));
          const x = i * slot;
          return (
            <rect
              // Clave por posición: dos puntos pueden compartir etiqueta (dos
              // horas «12:00» de días distintos) y la serie es posicional.
              key={i}
              x={x}
              y={height - h}
              width={barW}
              height={h}
              rx={1}
              fill={p.value === 0 ? 'var(--sytue-accent-200)' : 'var(--sytue-accent)'}
            >
              <title>{`${p.label} · ${p.value} ${seriesLabel}`}</title>
            </rect>
          );
        })}
      </svg>
      {/* Los mismos datos en tabla, ocultos a la vista y no al lector de
          pantalla: un <svg role="img"> solo puede anunciar su etiqueta, y «1.204
          visitas» no dice cómo se repartieron. */}
      <table class="sr-only">
        <caption>{seriesLabel}</caption>
        <thead>
          <tr>
            <th scope="col">{tableHeaders[0]}</th>
            <th scope="col">{tableHeaders[1]}</th>
          </tr>
        </thead>
        <tbody>
          {points.map((p, i) => (
            <tr key={i}>
              <th scope="row">{p.label}</th>
              <td>{p.value}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
