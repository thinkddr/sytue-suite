// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.os;

/** De MENTIRA para ArbolDocumentosPrueba. */
public final class Looper {
    private static final Looper PRINCIPAL = new Looper();

    public static Looper getMainLooper() { return PRINCIPAL; }
}
