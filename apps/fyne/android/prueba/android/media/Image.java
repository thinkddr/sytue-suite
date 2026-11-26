// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.media;

import android.graphics.Rect;
import java.nio.ByteBuffer;

/**
 * Un android.media.Image de MENTIRA para VideoSistemaPrueba: el fotograma YUV_420_888 que da
 * MediaCodec.getOutputImage, con sus tres planos (búfer, paso de fila y de muestra) y su
 * recorte. {@link #cerradas} cuenta los close().
 */
public class Image {
    public static int cerradas;

    private final int ancho, alto;
    private final Rect recorte;
    private final Plane[] planos;

    public Image(int ancho, int alto, Rect recorte, Plane[] planos) {
        this.ancho = ancho;
        this.alto = alto;
        this.recorte = recorte;
        this.planos = planos;
    }

    public int getWidth() {
        return ancho;
    }

    public int getHeight() {
        return alto;
    }

    /** Como en Android: una copia, y sin recorte, la imagen entera. */
    public Rect getCropRect() {
        return recorte == null ? new Rect(0, 0, ancho, alto) : new Rect(recorte);
    }

    public Plane[] getPlanes() {
        return planos;
    }

    public void close() {
        cerradas++;
    }

    public static class Plane {
        private final ByteBuffer b;
        private final int fila, pixel;

        public Plane(ByteBuffer b, int fila, int pixel) {
            this.b = b;
            this.fila = fila;
            this.pixel = pixel;
        }

        public ByteBuffer getBuffer() {
            return b;
        }

        public int getRowStride() {
            return fila;
        }

        public int getPixelStride() {
            return pixel;
        }
    }
}
