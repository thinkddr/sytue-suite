/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue Meet — SDK al servicio Go propio de salas/senalizacion (rooms.sytue.eu,
 * dev :8089). Implementacion clean-room. Los medios WebRTC los sirve LiveKit
 * (Apache-2.0, unica app de terceros): el BFF `sytue-meet` mintea el JWT de
 * LiveKit tras validar OIDC, y este SDK lo solicita junto con la URL del SFU.
 * La app web usa el LiveKit web SDK (livekit-client) con `serverUrl` + `token`
 * que devuelve `joinRoom`. Estado en PostgreSQL. Autorizacion Bearer (Sytue ID).
 *
 * REST base:  /api/v1
 */
import { HttpClient, type AuthProvider } from '../core/http.js';

export interface MeetOptions {
  /** URL base, p.ej. https://rooms.sytue.eu */
  baseUrl: string;
  auth: AuthProvider;
  fetchImpl?: typeof fetch | undefined;
}

export interface Room {
  id: string;
  /** Nombre de sala usado tambien como `room` en el JWT de LiveKit. */
  name: string;
  title?: string;
  createdAt: string;
  /** Numero de participantes activos. */
  participants: number;
}

export interface CreateRoomInput {
  title?: string;
  /** Sala con espera/lobby (admision por el host). */
  lobby?: boolean;
}

/**
 * Datos de conexion a LiveKit que el BFF entrega tras validar OIDC. El cliente
 * los pasa a `Room.connect(serverUrl, token)` del LiveKit web SDK.
 */
export interface LiveKitConnection {
  /** URL WSS del SFU LiveKit (p.ej. wss://rooms-sfu.sytue.eu o el endpoint LiveKit). */
  serverUrl: string;
  /** JWT de acceso a LiveKit minteado por el BFF `sytue-meet`. */
  token: string;
  /** Nombre de la sala (claim `video.room`). */
  room: string;
  /** Identidad del participante (claim `sub`). */
  identity: string;
}

export class MeetClient {
  private readonly http: HttpClient;

  constructor(options: MeetOptions) {
    this.http = new HttpClient({
      baseUrl: `${options.baseUrl.replace(/\/$/, '')}/api/v1`,
      auth: options.auth,
      fetchImpl: options.fetchImpl,
    });
  }

  /** Salas activas/visibles para el usuario. */
  listRooms(): Promise<Room[]> {
    return this.http.json<Room[]>({ path: '/rooms' });
  }

  /** Crea una sala de reunion. */
  createRoom(input?: CreateRoomInput): Promise<Room> {
    // El servicio no tiene campo raíz `lobby` (y su DecodeJSON rechaza campos
    // desconocidos): la preferencia viaja como metadata {lobby:true}.
    const { lobby, ...rest } = input ?? {};
    const body = lobby ? { ...rest, metadata: { lobby: true } } : rest;
    return this.http.postJson<Room>('/rooms', body);
  }

  /** Metadatos de una sala. */
  getRoom(roomName: string): Promise<Room> {
    return this.http.json<Room>({ path: `/rooms/${encodeURIComponent(roomName)}` });
  }

  /**
   * Solicita unirse a una sala: el BFF valida la sesion OIDC y mintea el JWT de
   * LiveKit. Devuelve `serverUrl` + `token` para el LiveKit web SDK.
   */
  joinRoom(roomName: string, options?: { displayName?: string }): Promise<LiveKitConnection> {
    return this.http.postJson<LiveKitConnection>(`/rooms/${encodeURIComponent(roomName)}/join`, {
      displayName: options?.displayName,
    });
  }

  /** Finaliza una sala (solo host/owner). */
  async endRoom(roomName: string): Promise<void> {
    await this.http.request({ method: 'DELETE', path: `/rooms/${encodeURIComponent(roomName)}` });
  }
}
