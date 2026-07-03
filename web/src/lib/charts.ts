// Shaping helpers that turn raw chart API responses into what Recharts wants
// — numeric coin values and human axis labels — mirroring the derivation
// style of lib/txn.ts.

// Recharts plots plain numbers; coin amounts arrive as minor units.
export function coinValue(minor: number): number {
  return minor / 1000;
}

export function formatDay(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export function formatMonthYear(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, { month: "short", year: "2-digit" });
}

const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const MONTHS = [
  "Jan", "Feb", "Mar", "Apr", "May", "Jun",
  "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
];

export function weekdayLabel(n: number): string {
  return WEEKDAYS[n] ?? String(n);
}

export function monthLabel(n: number): string {
  return MONTHS[(n - 1 + 12) % 12] ?? String(n);
}

export function hourLabel(n: number): string {
  const h = ((n % 24) + 24) % 24;
  const period = h < 12 ? "AM" : "PM";
  const hour12 = h % 12 === 0 ? 12 : h % 12;
  return `${hour12}${period}`;
}

// fillRange zero-fills every bucket in [start, start+count) so a quiet
// hour/weekday/month reads as "zero," not as a gap in the chart.
export function fillRange(
  buckets: { bucket: number; count: number }[],
  count: number,
  start = 0,
): { bucket: number; count: number }[] {
  const byBucket = new Map(buckets.map((b) => [b.bucket, b.count]));
  return Array.from({ length: count }, (_, i) => {
    const b = i + start;
    return { bucket: b, count: byBucket.get(b) ?? 0 };
  });
}

// recentPeriods lists the last `n` calendar months (current month first) as
// "YYYY-MM" values with a human label, for a statement month picker.
export function recentPeriods(n = 12): { value: string; label: string }[] {
  const cursor = new Date();
  cursor.setDate(1);
  const out: { value: string; label: string }[] = [];
  for (let i = 0; i < n; i++) {
    const value = `${cursor.getFullYear()}-${String(cursor.getMonth() + 1).padStart(2, "0")}`;
    const label = cursor.toLocaleDateString(undefined, { month: "long", year: "numeric" });
    out.push({ value, label });
    cursor.setMonth(cursor.getMonth() - 1);
  }
  return out;
}
