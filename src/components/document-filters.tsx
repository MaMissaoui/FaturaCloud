import type { ReactNode } from "react";
import { DatePicker, Select, Space } from "antd";
import { t } from "@lingui/core/macro";
import type { Dayjs } from "dayjs";

import { useDatePickerFormat } from "src/utils/date";

export interface DocumentFilterOption {
  value: string;
  label: ReactNode;
}

interface DocumentFiltersProps {
  // A [start, end] day range (RangePicker value), or null for "any date".
  dateRange: [Dayjs, Dayjs] | null;
  onDateRangeChange: (range: [Dayjs, Dayjs] | null) => void;
  status: string;
  onStatusChange: (status: string) => void;
  statusOptions: DocumentFilterOption[];
  // Defaults to "All states" — pages whose column is labelled "Status" pass
  // "All statuses" so the two agree.
  statusPlaceholder?: string;
  statusAriaLabel?: string;
  // The customer/vendor selector — pass empty arrays for a document with no
  // such party (imports, production orders).
  partyOptions: DocumentFilterOption[];
  partyValue: string;
  onPartyChange: (value: string) => void;
  partyPlaceholder: string;
}

// Shared filter bar for the document list pages (state + date range + a
// customer/vendor picker), rendered in the PageHeader's `extra` slot. Every
// list computed the same three filters by hand; this keeps them identical.
const DocumentFilters = ({
  dateRange,
  onDateRangeChange,
  status,
  onStatusChange,
  statusOptions,
  statusPlaceholder,
  statusAriaLabel,
  partyOptions,
  partyValue,
  onPartyChange,
  partyPlaceholder,
}: DocumentFiltersProps) => {
  const dateFormat = useDatePickerFormat();

  return (
    <Space wrap size="small" style={{ alignItems: "start" }}>
      <DatePicker.RangePicker
        value={dateRange}
        onChange={(value) => onDateRangeChange(value as [Dayjs, Dayjs] | null)}
        format={dateFormat}
        allowClear
        placeholder={[t`From`, t`To`]}
        aria-label={t`Date range`}
      />
      {statusOptions.length > 0 && (
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          placeholder={statusPlaceholder ?? t`All states`}
          aria-label={statusAriaLabel ?? t`Filter by state`}
          value={status || undefined}
          onChange={(value) => onStatusChange(value ?? "")}
          options={statusOptions}
          style={{ minWidth: 160 }}
        />
      )}
      {partyOptions.length > 0 && (
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          placeholder={partyPlaceholder}
          aria-label={partyPlaceholder}
          value={partyValue || undefined}
          onChange={(value) => onPartyChange(value ?? "")}
          options={partyOptions}
          style={{ minWidth: 220 }}
        />
      )}
    </Space>
  );
};

export default DocumentFilters;

// The filtering itself, shared so every list page applies the same rules:
// case-insensitive text search across the given fields, an exact status/state
// match, an exact customer/vendor id match, and an inclusive day range on the
// document's own date field.
export function matchesDocumentFilters(opts: {
  search: string;
  searchFields: (string | number | null | undefined)[];
  status: string;
  rowStatus: string | null | undefined;
  partyId: string;
  rowPartyId: string | null | undefined;
  dateRange: [Dayjs, Dayjs] | null;
  rowDate: number | null | undefined;
}): boolean {
  const { search, searchFields, status, rowStatus, partyId, rowPartyId, dateRange, rowDate } = opts;
  if (search) {
    const needle = search.toLowerCase();
    if (!searchFields.some((f) => (f ?? "").toString().toLowerCase().includes(needle))) {
      return false;
    }
  }
  if (status && rowStatus !== status) return false;
  if (partyId && rowPartyId !== partyId) return false;
  if (dateRange?.[0] && (rowDate ?? 0) < dateRange[0].startOf("day").valueOf()) return false;
  if (dateRange?.[1] && (rowDate ?? 0) > dateRange[1].endOf("day").valueOf()) return false;
  return true;
}
