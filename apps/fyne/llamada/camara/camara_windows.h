// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

#include <stdint.h>
#include <wchar.h>

int sytueCamAbrir(const wchar_t *id, int ancho, int alto, int fps); // 0 bien, -1 sin cámara, -2 sin permiso
int sytueCamLeer(uint8_t *dst, int cap, int *ancho, int *alto, long *secuencia);
void sytueCamCerrar(void);
int sytueCamLista(wchar_t *dst, int cap); // «enlace\tnombre\n» por cámara; devuelve los wchar escritos
