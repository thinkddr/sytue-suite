/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MOTOR DE PDF EN EL DISPOSITIVO (ADR 0086, «Firmar en el dispositivo», tercera
 * entrega).
 *
 * Para un PDF de la bóveda el servicio de Docs no puede dibujar ni editar nada: no ve
 * el fichero. Así que aquí corre el MISMO motor (`apps/nucleo/pdf`) compilado a
 * WebAssembly (`apps/nucleo/cmd/pdf-wasm`), servido como un módulo más del núcleo de
 * cliente (`/api/client-core/?module=pdf`, con su manifiesto y su huella SHA-256, como
 * `office` y `mail`) y ejecutado en un worker por `PqcRuntime`. Se carga SOLO cuando el
 * estudio de la bóveda abre un PDF (`import()` dinámico): el estudio del servidor y el
 * resto de la app no lo descargan nunca.
 *
 * El módulo no tiene red, ni claves, ni almacenamiento. Los bytes entran y salen por
 * `postMessage`; cifrarlos y guardarlos en la bóveda es cosa de quien llama. Ni el PDF
 * ni nada de él sale de este dispositivo por aquí: lo único que se pide a la red son
 * los tres ficheros del módulo (manifiesto, `wasm_exec.js` y `core.wasm`), que no
 * llevan nada del documento.
 */
import { ErrorPDF, type Contenido } from '../api';

/** Lo que una operación que cambia el documento devuelve. */
export interface EdicionLocal {
  /** El documento nuevo (o el JSON/XFDF de una lectura del formulario). */
  datos: Uint8Array<ArrayBuffer>;
  tipo: string;
  /** La edición quitó (`perdido`) o rebajó (`rebajado`) la marca PDF/A. */
  pdfa?: 'perdido' | 'rebajado' | undefined;
  /** Comprimir: bytes antes, después y ahorro en %. */
  antes?: number | undefined;
  despues?: number | undefined;
  ahorro?: number | undefined;
  importados?: number | undefined;
  /** Propiedades: el documento lleva XMP y el cambio puede no verse en otro lector. */
  xmp?: boolean | undefined;
  /** Buscar y reemplazar: cuántos fragmentos cambiaron. */
  reemplazos?: number | undefined;
}

/** El motor, tal y como lo usa el estudio de la bóveda. Sustituible en pruebas. */
export interface MotorPdf {
  /** Abre un PDF (el motor se queda con una COPIA) y devuelve su id y su contenido. */
  abrir: (pdf: Uint8Array, clave?: string) => Promise<{ doc: string; contenido: Contenido }>;
  /** Dibuja un lote de páginas («1-4») en SVG: número de página → SVG. */
  render: (doc: string, paginas: string, escala: number) => Promise<Record<number, string>>;
  anotar: (doc: string, campos: Record<string, string>, autor: string) => Promise<EdicionLocal>;
  /** `datos` es el XFDF de `importar-datos`. */
  formulario: (
    doc: string,
    campos: Record<string, string>,
    nombre: string,
    datos?: Uint8Array,
  ) => Promise<EdicionLocal>;
  /**
   * `combinar` (varios, en su orden), `extraer` (o reordenar), `borrar`, `rotar` y las
   * capas de un documento (`marca-agua`, `fondo`, `numerar`, `bates`, `encabezado`, `pie`).
   */
  paginas: (docs: string[], campos: Record<string, string>) => Promise<EdicionLocal>;
  herramienta: (doc: string, campos: Record<string, string>) => Promise<EdicionLocal>;
  /** `proteger` (AES-256), `desproteger`, `metadatos`, `sanear` y `redactar`. */
  proteger: (doc: string, campos: Record<string, string>) => Promise<EdicionLocal>;
  /**
   * `analizar` y buscar sin aplicar (JSON); `texto`, `buscar-reemplazar` con
   * `aplicar=1`, `imagen` (con `imagen`, o sin ella para quitarla), `capas`, `indice`.
   */
  editar: (
    doc: string,
    campos: Record<string, string>,
    imagen?: Uint8Array,
  ) => Promise<EdicionLocal>;
  /** Compara dos documentos abiertos: el JSON de `/pdf/compare`. */
  comparar: (a: string, b: string, campos: Record<string, string>) => Promise<unknown>;
  /** El texto de cada página con su posición (`/pdf/text`). */
  texto: (doc: string) => Promise<unknown>;
  /** El informe de las firmas, sin anclas de confianza (`anclas: false`). */
  verificar: (doc: string) => Promise<unknown>;
  /** Un fichero incrustado, en bytes opacos. */
  adjunto: (doc: string, nombre: string) => Promise<Uint8Array<ArrayBuffer>>;
  cerrar: (doc: string) => Promise<void>;
  /** Para el worker y borra la memoria del motor. */
  apagar: () => void;
}

/** Lo que `PqcRuntime` hace con el módulo: solo `invoke` y `close`. */
export interface EjecutorModulo {
  invoke: <T>(request: Record<string, unknown>, signal?: AbortSignal) => Promise<T>;
  close: () => void;
}

const codificar = (campos: Record<string, string>) =>
  new TextEncoder().encode(JSON.stringify(campos));

type Respuesta = Record<string, unknown> & { error?: { codigo?: string; mensaje?: string } };

/**
 * Los errores del motor llegan como el `ErrorPDF` del estudio, con el mismo código que
 * daría el servicio (`pdf_invalidates_signatures`, `pdf_password_required`…): así los
 * enseñan `mensajeDeError` e `invalidaFirmas` sin saber de dónde vienen.
 */
const fallo = (codigo: string, mensaje = codigo) => new ErrorPDF(mensaje, codigo);

function comprobar<T extends Respuesta>(r: T): T {
  if (r.error) throw fallo(r.error.codigo ?? 'pdf_failed', r.error.mensaje ?? '');
  return r;
}

function edicion(r: Respuesta): EdicionLocal {
  comprobar(r);
  if (!(r.body instanceof Uint8Array)) throw fallo('pdf_failed');
  const numero = (v: unknown) => (typeof v === 'number' ? v : undefined);
  return {
    datos: new Uint8Array(r.body),
    tipo: typeof r.content_type === 'string' ? r.content_type : 'application/pdf',
    pdfa: r.pdfa === 'perdido' || r.pdfa === 'rebajado' ? r.pdfa : undefined,
    antes: numero(r.antes),
    despues: numero(r.despues),
    ahorro: numero(r.ahorro),
    importados: numero(r.importados),
    xmp: r.xmp === true ? true : undefined,
    reemplazos: numero(r.reemplazos),
  };
}

/** El cuerpo JSON de una lectura (`comparar`, `texto`, `verificar`). */
function json(r: Respuesta): unknown {
  comprobar(r);
  if (!(r.body instanceof Uint8Array)) throw fallo('pdf_failed');
  return JSON.parse(new TextDecoder().decode(r.body)) as unknown;
}

/** El motor sobre un ejecutor del módulo `pdf` (el worker de `PqcRuntime`). */
export function motorSobre(ejecutor: EjecutorModulo): MotorPdf {
  const llamar = (operation: string, peticion: Record<string, unknown>) =>
    ejecutor.invoke<Respuesta>({ operation, ...peticion });
  return {
    async abrir(pdf, clave) {
      // Una copia propia: el worker borra lo que recibe al terminar, y quien llama
      // decide cuándo borra la suya.
      const r = comprobar(
        await llamar('pdf.abrir', { pdf: pdf.slice(), ...(clave ? { clave } : {}) }),
      );
      if (typeof r.doc !== 'string' || !(r.contenido instanceof Uint8Array))
        throw fallo('pdf_failed');
      return {
        doc: r.doc,
        contenido: JSON.parse(new TextDecoder().decode(r.contenido)) as Contenido,
      };
    },
    async render(doc, paginas, escala) {
      const r = comprobar(
        await llamar('pdf.render', { doc, campos: codificar({ paginas, escala: String(escala) }) }),
      );
      if (!(r.body instanceof Uint8Array)) throw fallo('pdf_failed');
      const lote = JSON.parse(new TextDecoder().decode(r.body)) as {
        paginas: Record<string, string>;
      };
      const out: Record<number, string> = {};
      for (const [n, svg] of Object.entries(lote.paginas)) out[Number(n)] = svg;
      return out;
    },
    anotar: async (doc, campos, autor) =>
      edicion(await llamar('pdf.anotar', { doc, autor, campos: codificar(campos) })),
    formulario: async (doc, campos, nombre, datos) =>
      edicion(
        await llamar('pdf.formulario', {
          doc,
          nombre,
          campos: codificar(campos),
          ...(datos ? { datos: datos.slice() } : {}),
        }),
      ),
    paginas: async (docs, campos) =>
      edicion(await llamar('pdf.paginas', { docs, campos: codificar(campos) })),
    herramienta: async (doc, campos) =>
      edicion(await llamar('pdf.herramienta', { doc, campos: codificar(campos) })),
    proteger: async (doc, campos) =>
      edicion(await llamar('pdf.proteger', { doc, campos: codificar(campos) })),
    editar: async (doc, campos, imagen) =>
      edicion(
        await llamar('pdf.editar', {
          doc,
          campos: codificar(campos),
          ...(imagen ? { imagen: imagen.slice() } : {}),
        }),
      ),
    comparar: async (a, b, campos) =>
      json(await llamar('pdf.comparar', { docs: [a, b], campos: codificar(campos) })),
    texto: async (doc) => json(await llamar('pdf.texto', { doc })),
    verificar: async (doc) => json(await llamar('pdf.verificar', { doc })),
    async adjunto(doc, nombre) {
      const r = comprobar(await llamar('pdf.adjunto', { doc, nombre }));
      if (!(r.body instanceof Uint8Array)) throw fallo('pdf_failed');
      return new Uint8Array(r.body);
    },
    async cerrar(doc) {
      await llamar('pdf.cerrar', { doc }).catch(() => undefined);
    },
    apagar: () => ejecutor.close(),
  };
}

/**
 * El motor del navegador: carga `PqcRuntime` (y con él el worker y el wasm) SOLO al
 * llamarse. Vive mientras viva `signal`.
 */
export async function motorDelNavegador(signal: AbortSignal): Promise<MotorPdf> {
  const { PqcRuntime } = await import('@sytue/offline/pqc/runtime');
  const ejecutor = new PqcRuntime({ signal, assertOpen: () => signal.throwIfAborted() }, 'pdf');
  return motorSobre(ejecutor);
}
