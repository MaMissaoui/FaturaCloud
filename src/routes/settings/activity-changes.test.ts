import { describe, expect, it } from "vitest";

import type { AuditFieldChange } from "src/api";
import { fieldLabel, formatChangeValue, summarizeChanges } from "./activity-changes";

const fmt = { date: (ms: number) => `D${ms}`, dateTime: (ms: number) => `DT${ms}`, locale: "en" };
const show = (change: AuditFieldChange, side: "from" | "to" = "to") =>
  formatChangeValue(change, change[side], fmt);

describe("fieldLabel", () => {
  it("names a known field and falls back to the column", () => {
    expect(fieldLabel("vatin")).toBe("VAT ID");
    expect(fieldLabel("defaultArAccountId")).toBe("Receivables account");
    expect(fieldLabel("someNewColumn")).toBe("someNewColumn");
  });
});

describe("formatChangeValue", () => {
  it("formats amounts in cents, dates, yes/no and empty values", () => {
    expect(show({ field: "total", from: null, to: 123456 })).toBe("1,234.56");
    expect(show({ field: "total", from: null, to: 123456 }, "from")).toBe("—");
    expect(show({ field: "dueDate", from: null, to: 1000 })).toBe("D1000");
    expect(show({ field: "postedAt", from: null, to: 1000 })).toBe("DT1000");
    expect(show({ field: "isActive", from: 1, to: 0 })).toBe("No");
    expect(show({ field: "city", from: "", to: "Sfax" }, "from")).toBe("—");
  });

  it("translates states and roles, lists e-mails and keeps masked accounts", () => {
    expect(show({ field: "state", from: "draft", to: "sent" })).toBe("Sent");
    expect(show({ field: "status", from: "draft", to: "posted" })).toBe("Posted");
    expect(show({ field: "organizationRole", from: "general", to: "power_user" })).toBe("Power User");
    expect(fieldLabel("organizationRole")).toBe("Role in the organization");
    expect(show({ field: "emails", from: null, to: '["a@b.tn","c@d.tn"]' })).toBe("a@b.tn, c@d.tn");
    expect(show({ field: "emails", from: null, to: "a@b.tn" })).toBe("a@b.tn");
    expect(show({ field: "iban", from: null, to: "TN••••8831", masked: true })).toBe("TN••••8831");
  });
});

describe("summarizeChanges", () => {
  const c = (field: string): AuditFieldChange => ({ field, from: null, to: "x" });
  it("lists up to three fields, then counts the rest", () => {
    expect(summarizeChanges([c("name"), c("city")])).toBe("Name, City");
    expect(summarizeChanges([c("name"), c("city"), c("phone"), c("street"), c("iban")])).toBe(
      "Name, City, Phone and 2 more",
    );
  });
});
