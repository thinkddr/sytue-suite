// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.graphics;

import java.io.IOException;
import java.io.OutputStream;
import java.nio.charset.StandardCharsets;

/**
 * Un Bitmap de MENTIRA para VideoSistemaPrueba: solo su tamaño. compress escribe un texto que
 * lo dice («FALSO JPEG 512x288 q85»), y {@link #vivos} cuenta los que no se han reciclado.
 */
public final class Bitmap {
    public enum CompressFormat { JPEG, PNG, WEBP }

    public static int vivos;

    private final int ancho, alto;
    private boolean reciclado;

    public Bitmap(int ancho, int alto) {
        this.ancho = ancho;
        this.alto = alto;
        vivos++;
    }

    public int getWidth() {
        return ancho;
    }

    public int getHeight() {
        return alto;
    }

    public boolean compress(CompressFormat formato, int calidad, OutputStream o) {
        try {
            o.write(("FALSO " + formato + " " + ancho + "x" + alto + " q" + calidad).getBytes(StandardCharsets.UTF_8));
            return true;
        } catch (IOException e) {
            return false;
        }
    }

    public static Bitmap createScaledBitmap(Bitmap b, int ancho, int alto, boolean filtrar) {
        return new Bitmap(ancho, alto);
    }

    public void recycle() {
        if (!reciclado) {
            reciclado = true;
            vivos--;
        }
    }
}
