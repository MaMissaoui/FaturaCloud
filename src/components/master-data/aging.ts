import { t } from "@lingui/core/macro";

import type { OutstandingBucket } from "src/api";

// The aging ramp shared by the Dashboard's owed panel and the master-data
// summaries: one hue, darker with age, so the buckets differ in lightness and
// not only in colour. "Not due yet" sits outside the ramp in a neutral
// blue-grey: it isn't late.
export const AGING_BUCKETS: { key: OutstandingBucket; color: string }[] = [
  { key: "current", color: "#8FA3BF" },
  { key: "days1To30", color: "#F3C29B" },
  { key: "days31To60", color: "#E08A4E" },
  { key: "days61To90", color: "#B9531A" },
  { key: "days90Plus", color: "#6E2E0A" },
];

// agingBucketLabel must be called during render (it reads the active locale).
export const agingBucketLabel = (key: OutstandingBucket) => {
  switch (key) {
    case "current":
      return t`Not due yet`;
    case "days1To30":
      return t`1 to 30 days late`;
    case "days31To60":
      return t`31 to 60 days late`;
    case "days61To90":
      return t`61 to 90 days late`;
    case "days90Plus":
      return t`More than 90 days late`;
  }
};

// The text colour for a long-overdue amount: the ramp's darkest step on a
// light background, a light step of the same hue on a dark one (the dark
// step is unreadable there).
export const lateTextColor = (dark: boolean) => (dark ? "#F0A774" : "#8A3A0E");
