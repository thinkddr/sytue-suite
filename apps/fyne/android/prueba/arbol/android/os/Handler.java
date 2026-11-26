// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.os;

/**
 * De MENTIRA para ArbolDocumentosPrueba: lo que se manda al hilo principal corre en el
 * acto, en otro hilo (como en Android, no en el de quien espera).
 */
public final class Handler {
    public Handler(Looper l) {}

    public boolean post(Runnable r) {
        new Thread(r).start();
        return true;
    }
}
