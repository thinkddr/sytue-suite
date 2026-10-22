// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

// # Avisos con la app cerrada, en Android
//
// En escritorio el Hub se queda en la bandeja y su flujo sigue vivo (`sesion.Residente`).
// Android no tiene bandeja y mata el proceso al rato de irse la app: lo que lo mantiene
// vivo es un servicio en primer plano (`eu.sytue.nativo.Avisos`, tipo specialUse, ADR
// 0039), con su notificación fija de importancia mínima. Solo lo declara el manifiesto
// del Hub. En los demás sistemas estas dos no hacen nada.

// MantenerAvisos arranca el servicio con `titulo` y `texto` en su notificación fija.
// Solo funciona con la app delante (Android 12+).
func MantenerAvisos(titulo, texto string) error { return mantenerAvisos(titulo, texto) }

// SoltarAvisos lo para: sin nadie dentro no hay de qué avisar.
func SoltarAvisos() error { return soltarAvisos() }
