import { theme } from "antd";
import dayjs from "dayjs";
import { useAtomValue } from "jotai";
import { plural } from "@lingui/core/macro";

import { themeAtom } from "src/atoms/generic";
import { lateTextColor } from "src/components/master-data/aging";

const DAY_MS = 24 * 60 * 60 * 1000;

// DueDate is a list's due-date cell: the date, and for a document still
// unpaid past it, the date in the aging ramp's late colour with how many days
// late it is — the same colour the summary panels use for long-overdue
// amounts, instead of a red tag. The page says how late: the server's
// daysOverdue for an outstanding document, or daysLate() as a fallback.
export default function DueDate({
  date,
  daysLate,
  format,
}: {
  date: number | null | undefined;
  daysLate: number;
  format: (date: number) => string;
}) {
  const { token } = theme.useToken();
  const dark = useAtomValue(themeAtom) === "dark";
  if (!date) return <span style={{ color: token.colorTextSecondary }}>—</span>;
  if (daysLate <= 0) {
    return <span style={{ whiteSpace: "nowrap" }}>{format(date)}</span>;
  }
  return (
    // Stacked like the total's "left" line beside it, so the column stays as
    // narrow as the date.
    <span style={{ whiteSpace: "nowrap", color: lateTextColor(dark), fontWeight: 500 }}>
      {format(date)}
      <div style={{ fontWeight: 400, fontSize: 12 }}>
        {plural(daysLate, { one: "# day late", other: "# days late" })}
      </div>
    </span>
  );
}

// daysLate counts whole days from the due date's day to today (`today` = the
// start of the current day), so a document due today is not late yet — the
// aging reports' rule. Rounded, not floored: a day across a daylight-saving
// change is 23 or 25 hours.
export const daysLate = (dueDate: number, today: number) =>
  Math.max(0, Math.round((today - dayjs(dueDate).startOf("day").valueOf()) / DAY_MS));
