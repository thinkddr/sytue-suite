// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.net;

/** Un ConnectivityManager de MENTIRA para CarretePrueba: si la red es de pago lo pone la prueba. */
public class ConnectivityManager {
    public boolean medida = true;

    public boolean isActiveNetworkMetered() {
        return medida;
    }
}
