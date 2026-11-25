/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * DE DÓNDE SALE EL DOCUMENTO QUE TRABAJA EL ESTUDIO.
 *
 * El estudio de CertiPDF (`PdfApp`, los paneles y los diálogos) pedía cada cosa al BFF
 * por la ruta del fichero en el Drive del servidor (`api.ts`): el contenido, las páginas
 * dibujadas, el texto, aplicar un cambio, verificar, comparar, las versiones… Con el
 * Drive de la persona en la bóveda (ADR 0086) eso no puede ser —el servidor no ve el
 * PDF—, y el estudio de la bóveda tenía que rehacer a mano lo que necesitaba, así que se
 * quedaba sin proteger, censurar, sellos, campos, índice, comparar, versiones, imprimir
 * ni leer en voz alta.
 *
 * Ahora el estudio habla con una FUENTE: la del servidor (`fuenteDelServidor`, la de
 * siempre, que llama al BFF con la ruta) o la del dispositivo
 * (`dispositivo/fuenteLocal.ts`: el motor de PDF en WebAssembly y la bóveda). Los
 * paneles y diálogos la toman del contexto (`useFuentePdf`); sin contexto, la del
 * servidor, así que todo lo que no sabe de esto sigue igual.
 *
 * `local` dice que no hay servidor que valga: lo que solo hace él (pedir firma,
 * reconocer texto, el asistente, abrir con la aplicación oficial) no se ofrece.
 */
import { createContext } from 'preact';
import { useContext, useMemo } from 'preact/hooks';
import type { FileVersionPage } from '@sytue/api-clients';
import {
  api,
  versiones,
  type AnalisisEdicion,
  type Coincidencia,
  type ComparacionVisual,
  type Contenido,
  type DiferenciaPagina,
  type DiferenciaVisual,
  type Fragmento,
  type InformeArchivo,
  type PropuestaCampo,
} from './api';
import type { InformeVerificacion } from './verificacion';
import { r } from '../lib/rutas';

/** Lo que contesta aplicar un cambio (el cuerpo de `/api/pdf-aplicar`). */
export type ResultadoAplicar = Awaited<ReturnType<typeof api.aplicar>>;

export interface FuentePdf {
  /** Sin servidor (la bóveda): lo que solo hace el servidor no se ofrece. */
  readonly local: boolean;
  contenido: (clave?: string) => Promise<Contenido>;
  /** Un lote de páginas («1-4») en SVG: número de página → SVG. */
  render: (
    paginas: string,
    escala: number,
    clave?: string,
  ) => Promise<{ paginas: Record<string, string> }>;
  texto: (clave?: string) => Promise<{ paginas: (Fragmento[] | null)[] }>;
  /** El cuerpo de `/api/pdf-aplicar` sin `path`: dónde queda lo decide la fuente. */
  aplicar: (cuerpo: Record<string, unknown>) => Promise<ResultadoAplicar>;
  verificar: (clave?: string) => Promise<InformeVerificacion>;
  analizarEdicion: (pagina: number, clave?: string) => Promise<AnalisisEdicion>;
  comprobarArchivo: (clave?: string) => Promise<InformeArchivo>;
  buscarTexto: (
    busqueda: Record<string, unknown>,
    clave?: string,
  ) => Promise<{ coincidencias: Coincidencia[] | null }>;
  exportarDatos: (clave?: string) => Promise<Blob>;
  detectarCampos: (
    pagina: number,
    clave?: string,
  ) => Promise<{ propuestas: PropuestaCampo[] | null }>;
  adjunto: (nombre: string, clave?: string) => Promise<Blob>;
  /** Comparar con OTRO documento de la misma fuente, por su ruta. */
  comparar: (otra: string) => Promise<{ diferencias: DiferenciaPagina[] | null; iguales: boolean }>;
  compararVisual: (otra: string, tolerancia: number) => Promise<ComparacionVisual>;
  compararVisualPagina: (
    otra: string,
    pagina: number,
    tolerancia: number,
  ) => Promise<{ pagina: DiferenciaVisual }>;
  versiones: {
    listar: (cursor?: string) => Promise<FileVersionPage>;
    restaurar: (version: number) => Promise<unknown>;
    /** Dónde se ve una versión archivada (el servidor la sirve por su URL)… */
    enlace?: (version: number) => string;
    /** …o cómo abrirla aquí (la bóveda la descifra en memoria). */
    abrir?: (version: number) => Promise<void>;
  };
  /** Dónde se baja el documento tal como está y con qué nombre, si la URL no lo dice. */
  descarga: () => { url: string; nombre?: string | undefined };
}

/** La fuente de siempre: el BFF, con la ruta del fichero en el Drive del servidor. */
export function fuenteDelServidor(path: string): FuentePdf {
  return {
    local: false,
    contenido: (clave) => api.contenido(path, clave),
    render: (paginas, escala, clave) => api.render(path, paginas, escala, clave),
    texto: (clave) => api.texto(path, clave),
    aplicar: (cuerpo) => api.aplicar({ path, ...cuerpo }),
    verificar: (clave) => api.verificar(path, clave),
    analizarEdicion: (pagina, clave) => api.analizarEdicion(path, pagina, clave),
    comprobarArchivo: (clave) => api.comprobarArchivo(path, clave),
    buscarTexto: (busqueda, clave) => api.buscarTexto(path, busqueda, clave),
    exportarDatos: (clave) => api.exportarDatos(path, clave),
    detectarCampos: (pagina, clave) => api.detectarCampos(path, pagina, clave),
    adjunto: (nombre, clave) => api.adjunto(path, nombre, clave),
    comparar: (otra) => api.comparar(path, otra),
    compararVisual: (otra, tolerancia) => api.compararVisual(path, otra, tolerancia),
    compararVisualPagina: (otra, pagina, tolerancia) =>
      api.compararVisualPagina(path, otra, pagina, tolerancia),
    versiones: {
      listar: (cursor) => versiones.listar(path, cursor),
      restaurar: (version) => versiones.restaurar(path, version),
      enlace: (version) => r(versiones.rutaDeVersion(path, version)),
    },
    descarga: () => ({ url: r(`/api/pdf-descargar?path=${encodeURIComponent(path)}`) }),
  };
}

/** La fuente del estudio que se está pintando; sin ella, la del servidor. */
export const FuenteContext = createContext<FuentePdf | null>(null);

/**
 * La fuente para un panel o un diálogo del estudio del documento `path`: la del
 * contexto si el estudio puso una, y si no, la del servidor por esa ruta.
 */
export function useFuentePdf(path: string): FuentePdf {
  const delContexto = useContext(FuenteContext);
  const delServidor = useMemo(() => fuenteDelServidor(path), [path]);
  return delContexto ?? delServidor;
}
