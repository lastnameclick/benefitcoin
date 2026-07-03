// Human relative time ("in 3 hours", "2 days ago") using the built-in
// Intl.RelativeTimeFormat — no date library needed.

const RTF = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

const DIVISIONS: { amount: number; unit: Intl.RelativeTimeFormatUnit }[] = [
  { amount: 60, unit: "seconds" },
  { amount: 60, unit: "minutes" },
  { amount: 24, unit: "hours" },
  { amount: 7, unit: "days" },
  { amount: 4.34524, unit: "weeks" },
  { amount: 12, unit: "months" },
  { amount: Number.POSITIVE_INFINITY, unit: "years" },
];

export function relativeTime(iso: string): string {
  let duration = (new Date(iso).getTime() - Date.now()) / 1000;
  for (const division of DIVISIONS) {
    if (Math.abs(duration) < division.amount) {
      return RTF.format(Math.round(duration), division.unit);
    }
    duration /= division.amount;
  }
  return "";
}

// datetimeLocal formats a Date as the value an <input type="datetime-local">
// expects, in local time (no timezone suffix).
export function datetimeLocal(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// endOfDay returns 11:59pm local time on the same day as d.
export function endOfDay(d: Date): Date {
  const end = new Date(d);
  end.setHours(23, 59, 0, 0);
  return end;
}

// endOfWeek returns 11:59pm local time on the upcoming Sunday (today, if it
// already is one).
export function endOfWeek(d: Date): Date {
  const end = new Date(d);
  end.setDate(end.getDate() + ((7 - end.getDay()) % 7));
  end.setHours(23, 59, 0, 0);
  return end;
}
