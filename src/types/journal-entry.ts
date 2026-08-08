import { t } from "@lingui/core/macro";

export type JournalEntryStatus = "draft" | "posted" | "reversed";

export const JOURNAL_ENTRY_STATUSES: JournalEntryStatus[] = ["draft", "posted", "reversed"];

// Ant Design Tag colors per status; draft is intentionally uncolored (default).
export const journalEntryStatusColor: Record<JournalEntryStatus, string | undefined> = {
  draft: undefined,
  posted: "green",
  reversed: "volcano",
};

// journalEntryStatusLabel must be called during render (not hoisted to module
// scope) so the returned label follows the currently-active locale — a
// module-scope `t` result would freeze at import-time locale and go stale on
// language switch.
export function journalEntryStatusLabel(status: string): string {
  switch (status) {
    case "draft":
      return t`Draft`;
    case "posted":
      return t`Posted`;
    case "reversed":
      return t`Reversed`;
    default:
      return status;
  }
}
