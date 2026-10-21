/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import { HttpClient, type HttpClientOptions } from '../core/http.js';

/** Una foto tal como la devuelve la API de Sytue Photos: dimensiones ya orientadas. */
export interface Foto {
  id: string;
  name: string;
  contentType: string;
  sizeBytes: number;
  /** Ancho y alto que se ven, con la orientación EXIF ya aplicada. */
  width: number;
  height: number;
  takenAt: string;
  /** false = la fecha sale de la subida, no del EXIF. */
  takenAtFromExif: boolean;
  favorite: boolean;
  description?: string;
  tags?: string[];
  location?: { lat: number; lon: number };
  camera?: string;
  lens?: string;
  driveFileId: string;
}

export interface PaginaFotos {
  items: Foto[];
  nextCursor?: string;
}

export interface PhotosOptions extends HttpClientOptions {
  baseUrl: string;
}

export class PhotosClient {
  private http: HttpClient;

  constructor(opts: PhotosOptions) {
    this.http = new HttpClient(opts);
  }

  async listPhotos(params?: { cursor?: string; limit?: number }): Promise<PaginaFotos> {
    const q = new URLSearchParams();
    if (params?.cursor) q.set('cursor', params.cursor);
    if (params?.limit) q.set('limit', String(params.limit));
    const qs = q.toString();
    return this.http.json<PaginaFotos>({ path: `/api/v1/photos${qs ? `?${qs}` : ''}` });
  }

  async getPhoto(id: string): Promise<Foto> {
    return this.http.json<Foto>({ path: `/api/v1/photos/${encodeURIComponent(id)}` });
  }
}
