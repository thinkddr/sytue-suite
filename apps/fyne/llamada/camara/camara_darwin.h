// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

#include <stdint.h>

// La cámara de AVFoundation (macOS e iOS). Una sola abierta a la vez: la llamada no
// publica dos cámaras.
int sytueCamPermiso(void);                                   // 1 concedido, 0 negado
int sytueCamAbrir(const char *id, int ancho, int alto, int fps); // 0 bien, -1 sin cámara, -2 sin permiso
// sytueCamLeer copia el último fotograma en I420 si es posterior a *secuencia. Devuelve
// 1 si copió; ancho y alto dicen el tamaño (y, si no caben en cap, no copia).
int sytueCamLeer(uint8_t *dst, int cap, int *ancho, int *alto, long *secuencia);
void sytueCamCerrar(void);
// sytueCamLista escribe «id\tnombre\n» por cámara en dst.
int sytueCamLista(char *dst, int cap);
// La pantalla (solo macOS): 1 con permiso; captura RGBA en dst. 0 bien, -1 sin pantalla,
// -2 sin permiso, -3 no cabe (ancho y alto dicen cuánto hace falta).
int sytuePantallaPermiso(void);
int sytuePantallaCapturar(uint8_t *dst, int cap, int *ancho, int *alto);
