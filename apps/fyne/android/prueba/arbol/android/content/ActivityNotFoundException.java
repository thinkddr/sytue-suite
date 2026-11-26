// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.content;

/** La de Android, para ArbolDocumentosPrueba: no hay actividad que atienda el Intent. */
public class ActivityNotFoundException extends RuntimeException {
    public ActivityNotFoundException(String mensaje) { super(mensaje); }
}
