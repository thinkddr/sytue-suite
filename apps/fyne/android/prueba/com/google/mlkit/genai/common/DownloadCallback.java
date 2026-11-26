// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package com.google.mlkit.genai.common;

/**
 * El aviso de descarga de MENTIRA, con la forma del de ML Kit (una interfaz con todo por
 * defecto), solo para IASistemaPrueba. No va en ninguna APK: está en android/prueba.
 */
public interface DownloadCallback {
    default void onDownloadStarted(long bytes) {}

    default void onDownloadCompleted() {}
}
