// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Calendar;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.TimeZone;

/**
 * Contrato de apps/nucleo/widgets (formato 2), sin dependencias de Android ni acceso a
 * secretos. Los mismos casos que Go y Swift: apps/nucleo/widgets/testdata/contrato.tsv.
 */
public final class WidgetSnapshot {
    /** Solo el formato 2: `generation` es la época de identidad, no la marca de sesión. */
    public static final long VERSION = 2;
    /** Caducidad máxima, 24 h: pasado un día el «hoy» sería el de ayer (ADR 0078). */
    public static final long MAX_AGE = 86400;
    /** Por debajo de 15 min el dato se considera vivo (Snapshot.Rancio en Go). */
    public static final long FRESCO = 900;
    public static final int MAX_BYTES = 16 * 1024;
    public static final int MAX_ITEMS = 8;
    private static final long MAX_TIMESTAMP = 10_000_000_000L;
    private static final Set<String> KINDS = new HashSet<>(Arrays.asList("unread", "files", "events", "documents",
            "contacts", "notes", "rows", "pending", "channels", "photos", "episodes", "apps", "calendars", "address_books", "storage_percent"));

    public static final class Counter {
        public final String kind;
        public final long value;

        public Counter(String kind, long value) {
            this.kind = kind;
            this.value = value;
        }
    }

    public static final class Item {
        public final String title;
        public final String subtitle;
        public final String badge;
        public final String color;
        public final long startAt;
        public final long endAt;
        public final boolean allDay;
        public final boolean unread;
        public final boolean done;

        public Item(String title, String subtitle, String badge, String color,
                long startAt, long endAt, boolean allDay, boolean unread, boolean done) {
            this.title = title;
            this.subtitle = subtitle == null ? "" : subtitle;
            this.badge = badge == null ? "" : badge;
            this.color = color == null ? "" : color;
            this.startAt = startAt;
            this.endAt = endAt;
            this.allDay = allDay;
            this.unread = unread;
            this.done = done;
        }
    }

    public static final class CalendarGrid {
        public final int year;
        public final int month;
        public final List<Integer> busyDays;
        public final Map<String, String> dayColors;

        public CalendarGrid(int year, int month, List<Integer> busyDays, Map<String, String> dayColors) {
            this.year = year;
            this.month = month;
            this.busyDays = busyDays == null ? Collections.emptyList()
                    : Collections.unmodifiableList(new ArrayList<>(busyDays));
            this.dayColors = dayColors == null ? Collections.emptyMap()
                    : Collections.unmodifiableMap(new HashMap<>(dayColors));
        }
    }

    /**
     * Lo que el lector comprueba FUERA del snapshot (widgets.Entorno en Go): la época
     * actual, la marca con que se confirmó, la marca de sesión actual, si hay un cierre
     * (`*.cerrada`) y el bloqueo de la suite.
     */
    public static final class Entorno {
        public final String epoca;
        public final String confirmada;
        public final String marca;
        public final boolean cerrada;
        public final boolean bloqueo;

        public Entorno(String epoca, String confirmada, String marca, boolean cerrada, boolean bloqueo) {
            this.epoca = epoca == null ? "" : epoca;
            this.confirmada = confirmada == null ? "" : confirmada;
            this.marca = marca == null ? "" : marca;
            this.cerrada = cerrada;
            this.bloqueo = bloqueo;
        }

        public boolean permite() {
            return !bloqueo && !cerrada && !epoca.isEmpty() && !marca.isEmpty() && confirmada.equals(marca);
        }
    }

    public final long version;
    public final String product;
    public final String generation;
    public final long updatedAt;
    public final long expiresAt;
    public final boolean active;
    public final List<Counter> counters;
    public final List<Item> items;
    public final CalendarGrid calendar;

    public WidgetSnapshot(long version, String product, String generation, long updatedAt,
            long expiresAt, boolean active, List<Counter> counters) {
        this(version, product, generation, updatedAt, expiresAt, active, counters, null, null);
    }

    public WidgetSnapshot(long version, String product, String generation, long updatedAt,
            long expiresAt, boolean active, List<Counter> counters, List<Item> items, CalendarGrid calendar) {
        this.version = version;
        this.product = product;
        this.generation = generation;
        this.updatedAt = updatedAt;
        this.expiresAt = expiresAt;
        this.active = active;
        this.counters = counters == null ? Collections.emptyList()
                : Collections.unmodifiableList(new ArrayList<>(counters));
        this.items = items == null ? Collections.emptyList()
                : Collections.unmodifiableList(new ArrayList<>(items));
        this.calendar = calendar;
    }

    public static int daysInMonth(int year, int month) {
        switch (month) {
            case 1: case 3: case 5: case 7: case 8: case 10: case 12:
                return 31;
            case 4: case 6: case 9: case 11:
                return 30;
            case 2:
                boolean leap = (year % 4 == 0 && year % 100 != 0) || (year % 400 == 0);
                return leap ? 29 : 28;
            default:
                return 0;
        }
    }

    public static boolean isHexColor(String s) {
        if (s == null || s.length() != 7 || s.charAt(0) != '#') {
            return false;
        }
        for (int i = 1; i < 7; i++) {
            char ch = s.charAt(i);
            if (!((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f'))) {
                return false;
            }
        }
        return true;
    }

    private static boolean validText(String s, int maxBytes) {
        if (s == null || s.toLowerCase(Locale.ROOT).contains("enc:v1:")) {
            return false;
        }
        byte[] bytes = s.getBytes(StandardCharsets.UTF_8);
        if (bytes.length > maxBytes) {
            return false;
        }
        for (int i = 0; i < s.length(); i++) {
            char ch = s.charAt(i);
            if (ch < 0x20 || ch == 0x7f) {
                return false;
            }
        }
        return true;
    }

    public boolean visible(String expectedProduct, Entorno entorno, long now) {
        if (entorno == null || !entorno.permite() || !active || version != VERSION || product == null
                || !product.equals(expectedProduct)
                || generation == null || generation.isEmpty() || generation.length() > 128
                || !generation.equals(entorno.epoca) || updatedAt <= 0 || updatedAt > now + 5
                || expiresAt <= now || expiresAt <= updatedAt || expiresAt - updatedAt > MAX_AGE
                || counters.size() > 12 || items.size() > MAX_ITEMS) {
            return false;
        }
        Set<String> seen = new HashSet<>();
        for (Counter c : counters) {
            if (c == null || !KINDS.contains(c.kind) || !seen.add(c.kind)
                    || c.value < 0 || c.value > 1_000_000_000_000L
                    || "storage_percent".equals(c.kind) && c.value > 100) {
                return false;
            }
        }
        for (Item item : items) {
            if (item == null || item.title == null || item.title.trim().isEmpty()
                    || !validText(item.title, 120)
                    || !validText(item.subtitle, 160)
                    || !validText(item.badge, 24)
                    || (!item.color.isEmpty() && !isHexColor(item.color))
                    || item.startAt < 0 || item.startAt > MAX_TIMESTAMP
                    || item.endAt < 0 || item.endAt > MAX_TIMESTAMP
                    || (item.endAt > 0 && item.startAt > 0 && item.endAt < item.startAt)) {
                return false;
            }
        }
        if (calendar != null) {
            if (calendar.year < 1970 || calendar.year > 2200 || calendar.month < 1 || calendar.month > 12) {
                return false;
            }
            int maxDay = daysInMonth(calendar.year, calendar.month);
            if (calendar.busyDays.size() > 31 || calendar.dayColors.size() > 31) {
                return false;
            }
            int previous = 0;
            for (Integer day : calendar.busyDays) {
                if (day == null || day < 1 || day > maxDay || day <= previous) {
                    return false;
                }
                previous = day;
            }
            for (Map.Entry<String, String> entry : calendar.dayColors.entrySet()) {
                String key = entry.getKey();
                String color = entry.getValue();
                if (key == null || !isHexColor(color)) {
                    return false;
                }
                try {
                    int day = Integer.parseInt(key);
                    if (!Integer.toString(day).equals(key) || day < 1 || day > maxDay) {
                        return false;
                    }
                } catch (NumberFormatException ignored) {
                    return false;
                }
            }
        }
        return true;
    }

    /** El lector dice de cuándo es el dato a partir de FRESCO (Snapshot.Rancio en Go). */
    public boolean rancio(long now) {
        return now - updatedAt >= FRESCO;
    }

    /**
     * widgets.Vigentes: en Calendar, un evento con hora vale mientras no haya terminado (o
     * no haya empezado); uno de día completo es FLOTANTE —medianoche UTC— y vale hasta la
     * medianoche local que sigue a su último día, con el final exclusivo. Sin fechas, vale.
     */
    public static boolean vigente(Item item, long now, TimeZone zona) {
        if (item.startAt == 0 && item.endAt == 0) {
            return true;
        }
        if (!item.allDay) {
            return item.endAt > now || item.startAt >= now;
        }
        return now < finDeDiaCompleto(item, zona);
    }

    /** La medianoche local que sigue al último día (UTC) de un evento de día completo. */
    static long finDeDiaCompleto(Item item, TimeZone zona) {
        long ultimo = item.endAt > item.startAt ? item.endAt - 1 : item.startAt;
        Calendar utc = Calendar.getInstance(TimeZone.getTimeZone("UTC"), Locale.ROOT);
        utc.setTimeInMillis(ultimo * 1000L);
        Calendar local = Calendar.getInstance(zona, Locale.ROOT);
        local.clear();
        local.set(utc.get(Calendar.YEAR), utc.get(Calendar.MONTH), utc.get(Calendar.DAY_OF_MONTH));
        local.add(Calendar.DAY_OF_MONTH, 1);
        return local.getTimeInMillis() / 1000L;
    }

    /** El mismo snapshot con solo los elementos vigentes a `now` (solo cambia en Calendar). */
    public WidgetSnapshot conVigentes(long now, TimeZone zona) {
        if (!"calendar".equals(product)) {
            return this;
        }
        List<Item> quedan = new ArrayList<>();
        for (Item item : items) {
            if (vigente(item, now, zona)) {
                quedan.add(item);
            }
        }
        return new WidgetSnapshot(version, product, generation, updatedAt, expiresAt, active, counters, quedan, calendar);
    }

    /**
     * Cuándo hay que volver a pintar aunque nadie publique: la caducidad, el próximo evento
     * que deja de valer y la medianoche local (la rejilla y el «hoy» cambian). Sin
     * snapshot, solo la medianoche: la rejilla de Calendar se pinta igual. Los segundos
     * después de la medianoche dan margen a un reloj que llega justo.
     */
    public static long proximoRepintado(WidgetSnapshot snapshot, long now, TimeZone zona) {
        Calendar manana = Calendar.getInstance(zona, Locale.ROOT);
        manana.setTimeInMillis(now * 1000L);
        manana.set(Calendar.HOUR_OF_DAY, 0);
        manana.set(Calendar.MINUTE, 0);
        manana.set(Calendar.SECOND, 0);
        manana.set(Calendar.MILLISECOND, 0);
        manana.add(Calendar.DAY_OF_MONTH, 1);
        long proximo = manana.getTimeInMillis() / 1000L + 5;
        if (snapshot == null) {
            return proximo;
        }
        if (snapshot.expiresAt > now) {
            proximo = Math.min(proximo, snapshot.expiresAt);
        }
        if ("calendar".equals(snapshot.product)) {
            for (Item item : snapshot.items) {
                long cambio = item.allDay ? finDeDiaCompleto(item, zona)
                        : item.endAt > now ? item.endAt : item.startAt >= now ? item.startAt + 1 : 0;
                if (cambio > now) {
                    proximo = Math.min(proximo, cambio);
                }
            }
        }
        return proximo;
    }

    /** Qué hacer con un widget ya puesto según el dispositivo y su anfitrión (Widgets.java). */
    public enum Pintado { PINTAR, CONSERVAR, EN_BLANCO }

    /** AppWidgetProviderInfo.WIDGET_CATEGORY_KEYGUARD, sin depender de Android. */
    public static final int CATEGORIA_KEYGUARD = 2;

    /**
     * Con el dispositivo bloqueado el receptor no lee nada (los datos van tras el bloqueo
     * del sistema). En la pantalla de inicio, que con el teléfono bloqueado no se ve, se
     * CONSERVA la última imagen: borrarla dejaba el widget en blanco al desbloquear, porque
     * nada garantiza repintar entonces. Un anfitrión de la pantalla de bloqueo
     * (KEYGUARD), que sí se ve bloqueado, se pinta EN BLANCO.
     */
    public static Pintado pintado(boolean dispositivoBloqueado, int categoriaAnfitrion) {
        if (!dispositivoBloqueado) {
            return Pintado.PINTAR;
        }
        return (categoriaAnfitrion & CATEGORIA_KEYGUARD) != 0 ? Pintado.EN_BLANCO : Pintado.CONSERVAR;
    }
}
