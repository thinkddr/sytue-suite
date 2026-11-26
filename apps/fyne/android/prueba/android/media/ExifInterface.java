// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.media;

import java.io.IOException;
import java.util.HashMap;
import java.util.Map;

/**
 * Un ExifInterface de MENTIRA para CarretePrueba: las etiquetas de cada fichero viven en
 * {@link #ETIQUETAS} (ruta → etiqueta → valor) y saveAttributes las deja ahí. Los nombres son
 * los de Android.
 */
public class ExifInterface {
    public static final Map<String, Map<String, String>> ETIQUETAS = new HashMap<>();

    public static final String TAG_DATETIME = "DateTime";
    public static final String TAG_DATETIME_ORIGINAL = "DateTimeOriginal";
    public static final String TAG_DATETIME_DIGITIZED = "DateTimeDigitized";
    public static final String TAG_OFFSET_TIME = "OffsetTime";
    public static final String TAG_OFFSET_TIME_ORIGINAL = "OffsetTimeOriginal";
    public static final String TAG_OFFSET_TIME_DIGITIZED = "OffsetTimeDigitized";
    public static final String TAG_SUBSEC_TIME = "SubSecTime";
    public static final String TAG_SUBSEC_TIME_ORIGINAL = "SubSecTimeOriginal";
    public static final String TAG_SUBSEC_TIME_DIGITIZED = "SubSecTimeDigitized";
    public static final String TAG_GPS_LATITUDE = "GPSLatitude";
    public static final String TAG_GPS_LATITUDE_REF = "GPSLatitudeRef";
    public static final String TAG_GPS_LONGITUDE = "GPSLongitude";
    public static final String TAG_GPS_LONGITUDE_REF = "GPSLongitudeRef";
    public static final String TAG_GPS_ALTITUDE = "GPSAltitude";
    public static final String TAG_GPS_ALTITUDE_REF = "GPSAltitudeRef";
    public static final String TAG_GPS_TIMESTAMP = "GPSTimeStamp";
    public static final String TAG_GPS_DATESTAMP = "GPSDateStamp";
    public static final String TAG_ORIENTATION = "Orientation";
    public static final String TAG_MAKE = "Make";
    public static final String TAG_MODEL = "Model";
    public static final String TAG_F_NUMBER = "FNumber";
    public static final String TAG_EXPOSURE_TIME = "ExposureTime";
    public static final String TAG_ISO_SPEED_RATINGS = "ISOSpeedRatings";
    public static final String TAG_FOCAL_LENGTH = "FocalLength";
    public static final String TAG_FOCAL_LENGTH_IN_35MM_FILM = "FocalLengthIn35mmFilm";
    public static final String TAG_FLASH = "Flash";
    public static final String TAG_WHITE_BALANCE = "WhiteBalance";
    public static final String TAG_IMAGE_DESCRIPTION = "ImageDescription";
    public static final String TAG_ARTIST = "Artist";
    public static final String TAG_COPYRIGHT = "Copyright";

    private final String ruta;
    private final Map<String, String> etiquetas;

    public ExifInterface(String ruta) throws IOException {
        this.ruta = ruta;
        this.etiquetas = new HashMap<>(ETIQUETAS.getOrDefault(ruta, Map.of()));
    }

    public String getAttribute(String etiqueta) {
        return etiquetas.get(etiqueta);
    }

    public void setAttribute(String etiqueta, String valor) {
        etiquetas.put(etiqueta, valor);
    }

    public void saveAttributes() throws IOException {
        ETIQUETAS.put(ruta, new HashMap<>(etiquetas));
    }
}
