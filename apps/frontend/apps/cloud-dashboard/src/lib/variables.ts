/**
 * Lógica de la pantalla de variables que no depende del DOM: filtrar la lista y
 * componer la plantilla `.env.example`. Se prueba en Bun (test/variables.test.ts).
 */
import { contiene } from '@sytue/i18n';
import type { EnvScope, EnvVar } from './types.js';

/** Ámbito por el que se filtra: '' son todos; 'managed', las que pone un servicio. */
export type FiltroAmbito = '' | EnvScope | 'managed';

/**
 * Filtra por nombre (sin acentos ni mayúsculas) y por ámbito.
 *
 * Con veinte variables la lista se lee entera; con ochenta —un monorepo con tres
 * servicios gestionados— encontrar `STRIPE_WEBHOOK_SECRET` era hacer scroll a ojo.
 */
export function filtrarVariables(vars: EnvVar[], q: string, ambito: FiltroAmbito): EnvVar[] {
  const texto = q.trim();
  if (!texto && !ambito) return vars;
  return vars.filter((v) => {
    if (ambito === 'managed' && v.source === 'manual') return false;
    if (ambito && ambito !== 'managed' && !v.scopes.includes(ambito)) return false;
    return !texto || contiene(v.key, texto);
  });
}

/** Rótulos ya traducidos que la plantilla escribe como comentarios. */
export interface RotulosPlantilla {
  /** Primera línea: qué es el fichero y que no lleva valores. */
  cabecera: string;
  /** «Solo producción» / «Solo previews», para las acotadas a un entorno. */
  soloProduccion: string;
  soloPreview: string;
  /** Rótulo de la sección de las que inyecta un servicio gestionado. */
  gestionadas: string;
}

/**
 * La plantilla `.env.example` del proyecto: las CLAVES, nunca los valores.
 *
 * Es lo que se sube al repositorio para que quien clone sepa qué tiene que
 * rellenar. La descarga del `.env` completo revela los secretos y por eso es de
 * admin/owner; esta no revela nada —ni siquiera los valores no secretos, que
 * suelen ser de un entorno concreto y no deben acabar como «defecto» en el código—,
 * así que la puede bajar cualquiera que vea la pantalla.
 *
 * Primero las propias, por orden alfabético; después las que inyecta un servicio
 * gestionado, aparte, porque en local hay que levantar ese servicio y no inventar
 * su valor.
 */
export function plantillaEnvExample(vars: EnvVar[], r: RotulosPlantilla): string {
  const porClave = (a: EnvVar, b: EnvVar) => a.key.localeCompare(b.key);
  const propias = vars.filter((v) => v.source === 'manual').sort(porClave);
  const gestionadas = vars.filter((v) => v.source !== 'manual').sort(porClave);

  const linea = (v: EnvVar) => {
    const solo =
      v.scopes.length === 1 && v.scopes[0] === 'production'
        ? r.soloProduccion
        : v.scopes.length === 1 && v.scopes[0] === 'preview'
          ? r.soloPreview
          : '';
    return solo ? `${v.key}= # ${solo}` : `${v.key}=`;
  };

  const out = [`# ${r.cabecera}`, ''];
  out.push(...propias.map(linea));
  if (gestionadas.length > 0) {
    if (propias.length > 0) out.push('');
    out.push(`# ${r.gestionadas}`);
    out.push(...gestionadas.map(linea));
  }
  return `${out.join('\n')}\n`;
}
