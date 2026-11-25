/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA FUENTE DEL DISPOSITIVO: el estudio entero sobre un PDF de la bóveda (ADR 0086,
 * cuarta entrega).
 *
 * Es la `FuentePdf` del estudio (`../fuente.ts`) hecha con el motor de PDF en
 * WebAssembly (`motorPdf.ts`) y el repositorio cifrado de la bóveda: el estudio de
 * siempre (`PdfApp`, sus paneles y diálogos) pide el contenido, las páginas, el texto,
 * aplicar un cambio, verificar, comparar o las versiones, y aquí se contesta SIN el
 * servidor. El PDF solo existe en claro en la memoria de esta pestaña y del worker del
 * motor; lo que se guarda vuelve cifrado (`editarEnBoveda.ts`).
 *
 * - El contenido abre los bytes en el motor UNA vez por contraseña: un PDF protegido
 *   pide la suya con la pantalla de siempre (`pdf_password_required`), y el motor la
 *   guarda con el documento para lo que venga después.
 * - Aplicar es `aplicarEnDispositivo` + `guardarPdfEnBoveda`: versión nueva del mismo
 *   fichero (y se vuelve a abrir) o fichero nuevo a su lado, con su ruta para abrirlo.
 * - Comparar abre el otro PDF por su ruta de la bóveda, descifrado aquí; se queda
 *   abierto mientras se recorre la comparación página a página.
 * - Las versiones son las del repositorio cifrado: se listan, se abre una (descifrada
 *   en un `blob:` local) y se restaura.
 * - Lo que solo hace el servidor (reconocer texto, pedir firma, el asistente) no está:
 *   `local: true`, y el estudio no lo ofrece.
 */
import { toFileVersion, type RawVersion } from '@sytue/api-clients';
import type { PrivateDriveRepository } from '@sytue/offline/matrix';
import { collectPrivateDriveFile } from '@sytue/offline/vault/drive';
import {
  ErrorPDF,
  type AnalisisEdicion,
  type Coincidencia,
  type ComparacionVisual,
  type Contenido,
  type DiferenciaPagina,
  type DiferenciaVisual,
  type Fragmento,
  type InformeArchivo,
  type PropuestaCampo,
} from '../api';
import type { FuentePdf, ResultadoAplicar } from '../fuente';
import type { InformeVerificacion } from '../verificacion';
import { aplicarEnDispositivo } from './aplicarLocal';
import { guardarPdfEnBoveda, descifrarPdfDeBoveda } from './editarEnBoveda';
import { MAX_PDF_LOCAL, abrirPdfDeBoveda, type PdfDeBoveda } from './firmarEnBoveda';
import type { MotorPdf } from './motorPdf';

/** Lo que la fuente usa del repositorio de Drive de la bóveda. */
export type RepositorioEstudio = Pick<
  PrivateDriveRepository,
  | 'recent'
  | 'file'
  | 'download'
  | 'children'
  | 'upload'
  | 'root'
  | 'versions'
  | 'downloadVersion'
  | 'restoreVersion'
>;

/** Una fuente del dispositivo: la del estudio, y cómo soltarla al salir. */
export interface FuenteLocal extends FuentePdf {
  /** Los bytes en claro del documento tal como está ahora (para firmarlo). */
  datos: () => Uint8Array<ArrayBuffer>;
  /** La contraseña con la que se abrió, si la pidió. */
  clave: () => string;
  /**
   * El documento reescrito SIN su contraseña (`desproteger`), para firmarlo: anexar a
   * un PDF cifrado exigiría cifrar la firma con su clave, y el servidor también firma
   * reescribiendo. Los bytes son de quien llama.
   */
  sinProteccion: () => Promise<Uint8Array<ArrayBuffer>>;
  /** Cierra los documentos del motor y borra los bytes en claro. */
  cerrar: () => Promise<void>;
}

const decodificar = <T>(datos: Uint8Array): T => JSON.parse(new TextDecoder().decode(datos)) as T;

export function fuenteLocal(options: {
  motor: MotorPdf;
  repositorio: RepositorioEstudio;
  signal: AbortSignal;
  pdf: PdfDeBoveda;
  /** Los bytes en claro: la fuente se queda con ellos y los borra al cerrar. */
  datos: Uint8Array<ArrayBuffer>;
  /** Quien anota, para firmar los comentarios. */
  autor: string;
  /** Abre una URL en otra pestaña (una versión descifrada); sustituible en pruebas. */
  abrirUrl?: ((url: string) => void) | undefined;
}): FuenteLocal {
  const { motor, repositorio, signal, pdf } = options;
  const carpeta = pdf.ruta.replace(/\/[^/]*$/, '');
  let datos = options.datos;
  let url = URL.createObjectURL(new Blob([datos], { type: 'application/pdf' }));
  let doc: string | null = null;
  let clave = '';
  let contenido: Contenido | null = null;
  /** El otro documento de una comparación, abierto mientras se recorre. */
  let otro: { ruta: string; doc: string } | null = null;

  const abierto = (): string => {
    if (!doc) throw new ErrorPDF('pdf_not_open', 'pdf_not_open');
    return doc;
  };

  /** Abre `bytes` en el motor con `conClave` y suelta el documento anterior. */
  const abrir = async (bytes: Uint8Array<ArrayBuffer>, conClave: string) => {
    const r = await motor.abrir(bytes, conClave || undefined);
    const anterior = doc;
    doc = r.doc;
    contenido = r.contenido;
    clave = conClave;
    if (anterior) await motor.cerrar(anterior);
    return r.contenido;
  };

  /** El documento cambió (versión nueva o restaurada): se abre lo nuevo. */
  const sustituir = async (bytes: Uint8Array<ArrayBuffer>) => {
    await abrir(bytes, clave);
    const viejo = datos;
    datos = bytes;
    URL.revokeObjectURL(url);
    url = URL.createObjectURL(new Blob([datos], { type: 'application/pdf' }));
    if (viejo !== bytes) viejo.fill(0);
  };

  /** Un PDF de la bóveda por su ruta, abierto en el motor (el de una comparación). */
  const abrirOtro = async (ruta: string): Promise<string> => {
    const encontrado = await abrirPdfDeBoveda(repositorio, ruta.trim(), signal);
    if (!encontrado) throw new ErrorPDF('download_failed', 'download_failed', 404);
    try {
      return (await motor.abrir(encontrado.datos)).doc;
    } finally {
      encontrado.datos.fill(0);
    }
  };

  const otroAbierto = async (ruta: string): Promise<string> => {
    if (otro?.ruta === ruta) return otro.doc;
    if (otro) await motor.cerrar(otro.doc);
    otro = null;
    otro = { ruta, doc: await abrirOtro(ruta) };
    return otro.doc;
  };

  const leerJSON = async <T>(r: Promise<{ datos: Uint8Array<ArrayBuffer> }>): Promise<T> => {
    const { datos: bytes } = await r;
    try {
      return decodificar<T>(bytes);
    } finally {
      bytes.fill(0);
    }
  };

  const fuente: FuenteLocal = {
    local: true,
    async contenido(conClave = '') {
      if (doc && contenido && conClave === clave) return contenido;
      return abrir(datos, conClave);
    },
    async render(paginas, escala) {
      const lote = await motor.render(abierto(), paginas, escala);
      const out: Record<string, string> = {};
      for (const [n, svg] of Object.entries(lote)) out[n] = svg;
      return { paginas: out };
    },
    async texto() {
      return (await motor.texto(abierto())) as { paginas: (Fragmento[] | null)[] };
    },
    async aplicar(cuerpo): Promise<ResultadoAplicar> {
      const cambio = await aplicarEnDispositivo({
        motor,
        doc: abierto(),
        nombre: pdf.nombre,
        cuerpo,
        autor: options.autor,
        abrirOtro,
      });
      let guardado;
      try {
        guardado = await guardarPdfEnBoveda({
          repositorio,
          id: pdf.id,
          datos: cambio.datos,
          sufijo: cambio.copia ? cambio.sufijo : undefined,
          signal,
        });
      } catch (e) {
        cambio.datos.fill(0);
        throw e;
      }
      if (guardado.nuevo) cambio.datos.fill(0);
      else await sustituir(cambio.datos);
      return {
        path: guardado.nuevo ? `${carpeta}/${guardado.nombre}` : pdf.ruta,
        copia: guardado.nuevo,
        ...(cambio.pdfa ? { pdfa: cambio.pdfa } : {}),
        ...(cambio.importados !== undefined ? { camposImportados: cambio.importados } : {}),
        ...(cambio.reemplazos !== undefined ? { reemplazos: cambio.reemplazos } : {}),
        ...(cambio.xmp ? { metadatosXmp: true } : {}),
        ...(cambio.ahorro !== undefined
          ? { ahorro: cambio.ahorro, bytes: cambio.despues ?? 0 }
          : {}),
      };
    },
    verificar: async () => (await motor.verificar(abierto())) as InformeVerificacion,
    analizarEdicion: (pagina) =>
      leerJSON<AnalisisEdicion>(
        motor.editar(abierto(), { op: 'analizar', pagina: String(pagina) }),
      ),
    comprobarArchivo: () =>
      leerJSON<InformeArchivo>(motor.herramienta(abierto(), { op: 'comprobar-archivo' })),
    buscarTexto: (busqueda) =>
      leerJSON<{ coincidencias: Coincidencia[] | null }>(
        motor.editar(abierto(), { op: 'buscar-reemplazar', busqueda: JSON.stringify(busqueda) }),
      ),
    async exportarDatos() {
      const r = await motor.formulario(abierto(), { op: 'exportar-datos' }, pdf.nombre);
      return new Blob([r.datos], { type: r.tipo || 'application/vnd.adobe.xfdf' });
    },
    detectarCampos: (pagina) =>
      leerJSON<{ propuestas: PropuestaCampo[] | null }>(
        motor.formulario(abierto(), { op: 'detectar', pagina: String(pagina) }, pdf.nombre),
      ),
    async adjunto(nombre) {
      // Bytes opacos: el tipo que declara el documento no decide cómo se sirve.
      return new Blob([await motor.adjunto(abierto(), nombre)], {
        type: 'application/octet-stream',
      });
    },
    comparar: async (ruta) =>
      (await motor.comparar(abierto(), await otroAbierto(ruta), {})) as {
        diferencias: DiferenciaPagina[] | null;
        iguales: boolean;
      },
    compararVisual: async (ruta, tolerancia) =>
      (await motor.comparar(abierto(), await otroAbierto(ruta), {
        modo: 'visual',
        tolerancia: String(tolerancia),
      })) as ComparacionVisual,
    compararVisualPagina: async (ruta, pagina, tolerancia) =>
      (await motor.comparar(abierto(), await otroAbierto(ruta), {
        modo: 'visual',
        pagina: String(pagina),
        tolerancia: String(tolerancia),
      })) as { pagina: DiferenciaVisual },
    versiones: {
      async listar(cursor) {
        const p = await repositorio.versions(pdf.id, cursor ?? '', signal);
        return {
          versions: p.versions.map((v) => toFileVersion(v as RawVersion)),
          ...(p.next_cursor ? { nextCursor: p.next_cursor } : {}),
        };
      },
      async restaurar(version) {
        await repositorio.restoreVersion(pdf.id, version, signal);
        // Lo restaurado es ya el documento: se descifra y se abre lo nuevo.
        await sustituir(await descifrarPdfDeBoveda(repositorio, pdf.id, signal));
        return { ok: true };
      },
      async abrir(version) {
        const fila = (
          await repositorio.versions(pdf.id, String(version + 1), signal)
        ).versions.find((v) => v.version === version);
        if (!fila) throw new ErrorPDF('not_found', 'not_found', 404);
        const blob = await collectPrivateDriveFile({
          size: fila.size_bytes,
          maximum: MAX_PDF_LOCAL,
          type: 'application/pdf',
          run: (sink) => repositorio.downloadVersion(pdf.id, version, sink, signal),
        });
        const enlace = URL.createObjectURL(blob);
        (options.abrirUrl ?? ((u: string) => window.open(u, '_blank', 'noopener')))(enlace);
        // La pestaña nueva ya tiene el `blob:`; aquí no hace falta más que un rato.
        setTimeout(() => URL.revokeObjectURL(enlace), 60_000);
      },
    },
    descarga: () => ({ url, nombre: pdf.nombre }),
    datos: () => datos,
    clave: () => clave,
    sinProteccion: async () => (await motor.proteger(abierto(), { op: 'desproteger' })).datos,
    async cerrar() {
      if (otro) await motor.cerrar(otro.doc);
      if (doc) await motor.cerrar(doc);
      otro = null;
      doc = null;
      contenido = null;
      URL.revokeObjectURL(url);
      datos.fill(0);
    },
  };
  return fuente;
}
