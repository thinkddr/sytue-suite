/**
 * Ayudas puras para la pantalla de escala (sin DOM: se comprueban con `bun test`).
 *
 * Elegir tamaño e instancias es una decisión con dos consecuencias que el usuario
 * no puede deducir de un desplegable: lo que va a COSTAR y lo que va a recibir de
 * verdad. Aquí se traducen las dos, para que la elección se tome viéndolas y no a
 * ciegas. Nada de esto decide nada: el precio real lo factura el control-plane y
 * los recursos los fija su catálogo.
 */
import type { Framework } from './types.js';
import { METRICS, round2 } from './pricing.js';
import { formatNumber, type Locale } from '@sytue/i18n';
import { formatBytes } from '@sytue/ui';

/**
 * Horas de un mes de facturación. 730 = 8760/12, el promedio que usan
 * DigitalOcean y Scaleway para estimar el mensual de un recurso siempre encendido.
 */
export const HOURS_PER_MONTH = 730;

/**
 * Coste mensual ORIENTATIVO del cómputo de una app encendida todo el mes.
 * Es memoria × instancias × horas al precio de compute_gb_hours, la misma métrica
 * que se factura de verdad. No incluye build ni almacenamiento: esos dependen de
 * cuánto despliegues y de lo que guardes, no de la escala que elijas aquí.
 */
export function monthlyComputeEur(memoryGiB: number, instances: number): number {
  const precio = METRICS.compute_gb_hours?.pricePerUnit;
  if (!(memoryGiB > 0) || !(instances > 0) || !precio) return 0;
  return round2(memoryGiB * instances * HOURS_PER_MONTH * precio);
}

/**
 * Traduce la notación de Kubernetes a lo que entiende quien elige un plan.
 * «1000m» son 1 vCPU y «512Mi» son 512 MB: nadie debería tener que saber eso para
 * escalar su app. Devuelve «» si el catálogo no trae recursos (modo demostración),
 * porque inventarlos sería peor que no enseñar nada.
 */
export function humanResources(locale: Locale, cpuLimit: string, memoryLimit: string): string {
  const cpu = humanCpu(locale, cpuLimit);
  const mem = humanMemory(locale, memoryLimit);
  if (!cpu && !mem) return '';
  return [cpu, mem].filter(Boolean).join(' · ');
}

function humanCpu(locale: Locale, limit: string): string {
  const m = /^(\d+(?:\.\d+)?)(m?)$/.exec(limit.trim());
  if (!m) return '';
  const vcpu = m[2] === 'm' ? Number(m[1]) / 1000 : Number(m[1]);
  if (!Number.isFinite(vcpu) || vcpu <= 0) return '';
  // «0,5 vCPU» se lee mejor que «500m»; el separador, el del idioma.
  return `${formatNumber(locale, vcpu, { maximumFractionDigits: 1 })} vCPU`;
}

function humanMemory(locale: Locale, limit: string): string {
  const m = /^(\d+(?:\.\d+)?)(Mi|Gi|M|G)$/.exec(limit.trim());
  if (!m) return '';
  const value = Number(m[1]);
  if (!Number.isFinite(value) || value <= 0) return '';
  const mb = m[2]?.startsWith('G') ? value * 1024 : value;
  return formatBytes(locale, mb * 1024 * 1024);
}

/**
 * Puerto en el que ESCUCHA por convención cada framework, para ofrecerlo de un
 * clic en vez de dejar que se adivine. Es una SUGERENCIA, no una verdad: la app
 * manda, y por eso el campo sigue siendo editable.
 *
 * Los frameworks que ya respetan `$PORT` —que la plataforma inyecta— no necesitan
 * sugerencia: cualquier puerto les vale, así que devuelven undefined y la interfaz
 * no ofrece nada. Sugerir ahí sería ruido.
 */
export function typicalPortFor(framework: Framework): number | undefined {
  switch (framework) {
    case 'astro':
      return 4321; // `astro preview`/adaptador de Node
    case 'next':
      return 3000;
    case 'sveltekit':
      return 3000; // adapter-node
    default:
      return undefined; // node/preact/static: respetan $PORT o los sirve el contenedor
  }
}
