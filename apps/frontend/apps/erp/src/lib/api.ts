/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El cliente del BFF de esta app. Una función, porque la API del servicio es una
 * sola: `/api/v1/{modelo}` sirve a los treinta y seis modelos.
 *
 * El prefijo `/erp` lo pone el shim de `basePath` (@sytue/ui), así que aquí las
 * rutas se escriben desde `/api/` y nunca con el prefijo a mano.
 */

/** Error del servicio con su estado, para que la interfaz pueda distinguirlos. */
export class ErrorApi extends Error {
  readonly estado: number;
  /**
   * Qué campo falló, cuando el servicio lo dice. El 422 de validación trae
   * `details.fields` —el `validation.Errors` de la plataforma— y sin recogerlo
   * aquí la ficha solo puede pintar una frase única arriba del formulario.
   */
  readonly porCampo: Record<string, string>;
  /** El `code` del error de la plataforma (`sin_permiso`…): lo que se traduce. */
  readonly codigo: string;

  constructor(mensaje: string, estado: number, porCampo: Record<string, string> = {}, codigo = '') {
    super(mensaje);
    this.name = 'ErrorApi';
    this.estado = estado;
    this.porCampo = porCampo;
    this.codigo = codigo;
  }
}

/** El módulo existe, pero la administración no le ha dado acceso a esta persona. */
export function esSinPermiso(err: unknown): boolean {
  return err instanceof ErrorApi && err.codigo === 'sin_permiso';
}

/**
 * Lo que se le dice a la persona cuando una llamada falla.
 *
 * El mensaje del servicio es un RESPALDO EN INGLÉS —la plataforma manda `code` y
 * «el cliente traduce por Code» (`platform/pkg/httpx`)—, y se pintaba tal cual:
 * quien no tenía acceso a un módulo leía «You do not have access to this module…»
 * en mitad de la interfaz en español (medido en producción, 2026-09-21). Lo que el
 * ERP sabe traducir se traduce aquí, una vez, para los treinta sitios que avisan.
 */
export function mensajeDeError(
  err: unknown,
  t: (
    clave:
      | 'error.generic'
      | 'error.sinPermiso'
      | 'vf.error.invalidCertificate'
      | 'vf.error.certificateTaxIdMismatch',
  ) => string,
): string {
  if (esSinPermiso(err)) return t('error.sinPermiso');
  if (err instanceof ErrorApi) {
    if (err.codigo === 'certificado_invalido') return t('vf.error.invalidCertificate');
    if (err.codigo === 'certificado_nif_no_coincide') return t('vf.error.certificateTaxIdMismatch');
  }
  return err instanceof Error ? err.message : t('error.generic');
}

export async function api<T>(ruta: string, init?: RequestInit): Promise<T> {
  // `Headers` y no un objeto con spread: `HeadersInit` admite también un array de
  // pares, y esparcirlo produciría claves numéricas en vez de cabeceras.
  const headers = new Headers(init?.headers);
  // JSON salvo que quien llama diga otra cosa: la importación manda `text/csv`.
  if (!headers.has('content-type')) headers.set('content-type', 'application/json');
  const resp = await fetch(`/api/${ruta}`, { ...init, headers });
  if (!resp.ok) {
    const cuerpo = (await resp.json().catch(() => null)) as {
      error?: {
        code?: string;
        message?: string;
        details?: { fields?: { field: string; message: string }[] };
      };
    } | null;
    const campos = Object.fromEntries(
      (cuerpo?.error?.details?.fields ?? []).map((f) => [f.field, f.message]),
    );
    throw new ErrorApi(
      cuerpo?.error?.message ?? `HTTP ${resp.status}`,
      resp.status,
      campos,
      cuerpo?.error?.code ?? '',
    );
  }
  if (resp.status === 204) return undefined as T;
  return (await resp.json()) as T;
}

/** Construye `?a=b&c=d` saltándose lo vacío. */
export function consulta(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [clave, valor] of Object.entries(params)) {
    if (valor !== undefined && valor !== '') q.set(clave, String(valor));
  }
  const s = q.toString();
  return s ? `?${s}` : '';
}

/**
 * El prefijo bajo el que se sirve esta app, tal y como lo publicó el servidor.
 * `basePathScript` lo deja en `window.__sytueBase`; el respaldo es el del build.
 */
export function baseDeLaApp(): string {
  return (globalThis as { __sytueBase?: string }).__sytueBase ?? '/erp';
}
