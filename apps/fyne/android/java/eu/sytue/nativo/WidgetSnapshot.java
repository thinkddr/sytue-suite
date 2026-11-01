// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

/** Contrato de apps/nucleo/widgets, sin dependencias de Android ni acceso a secretos. */
public final class WidgetSnapshot {
    public static final long MAX_AGE = 30 * 60;
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

    public boolean visible(String expectedProduct, String currentGeneration, boolean locked, long now) {
        if (locked || !active || version != 1 || product == null || !product.equals(expectedProduct)
                || generation == null || generation.isEmpty() || generation.length() > 128
                || !generation.equals(currentGeneration) || updatedAt <= 0 || updatedAt > now + 5
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
}
