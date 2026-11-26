// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package com.google.mlkit.genai.prompt;

/**
 * Un ML Kit de MENTIRA con la forma de la Prompt API (un `object` de Kotlin: INSTANCE y
 * getClient), solo para IASistemaPrueba. No va en ninguna APK: está en android/prueba.
 */
public final class Generation {
    public static final Generation INSTANCE = new Generation();

    private Generation() {}

    public Modelo getClient() {
        return new Modelo();
    }

    /** El GenerativeModel de Kotlin. */
    public static final class Modelo {}
}
