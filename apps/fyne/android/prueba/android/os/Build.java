// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.os;

/**
 * Un android.os.Build de MENTIRA para VideoSistemaPrueba (no va en ninguna APK: está en
 * android/prueba). SDK_INT se puede cambiar para probar los dos caminos.
 */
public final class Build {
    private Build() {}

    public static final class VERSION {
        private VERSION() {}

        public static int SDK_INT = 36;
    }
}
