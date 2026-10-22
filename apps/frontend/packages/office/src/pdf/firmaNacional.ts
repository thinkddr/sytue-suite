/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Dónde está la aplicación OFICIAL de firma de cada país del Espacio Económico
 * Europeo.
 *
 * POR QUÉ ESTA LISTA EXISTE. Desde el navegador no se puede firmar con el
 * almacén de certificados del sistema: no hay ninguna interfaz web que lo
 * permita, ni con Llavero en macOS, ni con el almacén de Windows, ni con NSS en
 * Linux. Lo único parecido es autenticarse con certificado de cliente al abrir
 * una conexión, que no sirve para firmar un documento. Así que quien tiene su
 * certificado en el sistema —o en una tarjeta, o en el DNI electrónico— firma
 * con la aplicación de su país y vuelve con el PDF ya firmado.
 *
 * QUÉ ENTRA AQUÍ: solo herramientas OFICIALES y GRATUITAS, verificadas una a
 * una. No entran las de los prestadores privados aunque tengan versión gratuita,
 * porque entonces esto sería una lista de recomendaciones comerciales y no una
 * ayuda.
 *
 * Y LO QUE NO ESTÁ, NO SE INVENTA. Para el resto del Espacio Económico Europeo
 * se enlaza el navegador de listas de confianza de la Comisión Europea, que es
 * la fuente oficial de quién presta servicios de confianza en cada país. Es
 * mejor mandar ahí que adivinar el nombre de una aplicación.
 */

/** Una aplicación oficial de firma, con su país. */
export interface HerramientaNacional {
  /** ISO 3166-1 alfa-2, que es lo que trae la etiqueta de idioma del navegador. */
  pais: string;
  /** Nombre propio: es una marca y no se traduce en ningún idioma. */
  nombre: string;
  url: string;
}

/**
 * El navegador de listas de confianza de la Comisión Europea. Cubre todo el
 * Espacio Económico Europeo y es la salida para cualquier país sin entrada
 * propia aquí.
 */
export const LISTA_DE_CONFIANZA_UE = 'https://eidas.ec.europa.eu/efda/tl-browser/';

/**
 * Verificadas el 2026-09-10. Cada una es de una administración o de una entidad
 * pública del país, y ninguna cobra por firmar.
 */
export const HERRAMIENTAS_NACIONALES: readonly HerramientaNacional[] = [
  // Gobierno de España. La de referencia para el DNI electrónico y los
  // certificados de la FNMT.
  { pais: 'ES', nombre: 'AutoFirma', url: 'https://firmaelectronica.gob.es/descargas' },
  // Agência para a Modernização Administrativa. Cartão de Cidadão y Chave Móvel.
  { pais: 'PT', nombre: 'Autenticação.gov', url: 'https://www.autenticacao.gov.pt/' },
  // RIA, la agencia estatal estonia de sistemas de información.
  { pais: 'EE', nombre: 'DigiDoc4', url: 'https://www.id.ee/en/rubriik/digidoc4-client/' },
  // A-SIT, el centro austriaco de tecnología segura. Firma PDF de escritorio.
  { pais: 'AT', nombre: 'PDF-Over', url: 'https://technology.a-sit.at/en/pdf-over-3/' },
  // LVRTC, empresa estatal letona.
  { pais: 'LV', nombre: 'eParaksts', url: 'https://www.eparaksts.lv/' },
  // BOSA, el servicio federal belga de política y apoyo.
  { pais: 'BE', nombre: 'eID', url: 'https://eid.belgium.be/' },
  // Servicio del Estado polaco: firma en el navegador con el perfil de confianza,
  // sin instalar nada. Va aquí porque es la vía oficial y gratuita del país.
  {
    pais: 'PL',
    nombre: 'Podpis zaufany',
    url: 'https://www.gov.pl/web/gov/podpisz-dokument-elektronicznie-wykorzystaj-podpis-zaufany',
  },
];

/**
 * La herramienta del país de quien está delante, si la hay.
 *
 * Se mira la REGIÓN de la etiqueta de idioma (`es-ES` → `ES`) y no el idioma
 * suelto: un austriaco y un alemán escriben los dos `de`, y sus herramientas no
 * son la misma. Sin región no se adivina nada y se ofrece la lista entera.
 */
export function herramientaDe(etiqueta: string | undefined): HerramientaNacional | undefined {
  const region = (etiqueta ?? '').split('-')[1]?.toUpperCase();
  if (!region) return undefined;
  return HERRAMIENTAS_NACIONALES.find((h) => h.pais === region);
}
