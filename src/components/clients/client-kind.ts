import type { Client } from "src/types/client";

// A business client is one with a tax identifier; everyone else is a private
// individual. Used for the Clients screen's "Businesses"/"Private" filters
// and the summary panel.
export const isBusinessClient = (client: Client): boolean =>
  !!(client.vatin?.trim() || client.tax_number?.trim());

// emails is a JSON-encoded string array on the wire (src/types/client.ts).
export const parseClientEmails = (emails: string | null | undefined): string[] => {
  if (!emails) return [];
  try {
    const parsed = JSON.parse(emails);
    return Array.isArray(parsed) ? parsed.filter((e) => typeof e === "string" && e) : [];
  } catch {
    return [];
  }
};

// duplicateNameKeys returns the normalized names (trimmed, lower-cased) that
// more than one client carries, for the "Possible duplicate" tag.
export const duplicateNameKeys = (clients: Client[]): Set<string> => {
  const seen = new Map<string, number>();
  for (const c of clients) {
    const key = nameKey(c.name);
    if (key) seen.set(key, (seen.get(key) ?? 0) + 1);
  }
  return new Set([...seen].filter(([, n]) => n > 1).map(([k]) => k));
};

export const nameKey = (name: string | null | undefined) => (name ?? "").trim().toLowerCase();
