// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.media;

import java.util.HashMap;
import java.util.Map;

/**
 * Un MediaFormat de MENTIRA para VideoSistemaPrueba, con las claves de verdad. Como el de
 * Android, getInteger y getLong de una clave que no está lanzan (NullPointerException).
 */
public final class MediaFormat {
    public static final String KEY_MIME = "mime";
    public static final String KEY_WIDTH = "width";
    public static final String KEY_HEIGHT = "height";
    public static final String KEY_DURATION = "durationUs";
    public static final String KEY_ROTATION = "rotation-degrees";
    public static final String KEY_COLOR_FORMAT = "color-format";
    public static final String KEY_SAMPLE_RATE = "sample-rate";
    public static final String KEY_CHANNEL_COUNT = "channel-count";
    public static final String KEY_PCM_ENCODING = "pcm-encoding";
    public static final String KEY_STRIDE = "stride";
    public static final String KEY_SLICE_HEIGHT = "slice-height";

    private final Map<String, Object> valores = new HashMap<>();

    public MediaFormat() {}

    public MediaFormat(MediaFormat otro) {
        valores.putAll(otro.valores);
    }

    public boolean containsKey(String clave) {
        return valores.containsKey(clave);
    }

    public String getString(String clave) {
        return (String) valores.get(clave);
    }

    public int getInteger(String clave) {
        return (Integer) valores.get(clave);
    }

    public long getLong(String clave) {
        return (Long) valores.get(clave);
    }

    public void setString(String clave, String valor) {
        valores.put(clave, valor);
    }

    public void setInteger(String clave, int valor) {
        valores.put(clave, valor);
    }

    public void setLong(String clave, long valor) {
        valores.put(clave, valor);
    }
}
