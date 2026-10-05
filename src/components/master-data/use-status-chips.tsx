import { useState, type ReactNode } from "react";
import { theme } from "antd";
import { Trans } from "@lingui/react/macro";

import FilterChips from "src/components/master-data/filter-chips";

export interface StatusChip {
  // The document status this chip selects.
  key: string;
  label: ReactNode;
  // Left out while no row has this status (Cancelled, usually), unless picked.
  hideWhenEmpty?: boolean;
}

// useStatusChips is the document lists' status filter in the master-data
// style: an All chip plus one per status, each with its count, replacing the
// status dropdown. `rows` are what the page's other filters (search, party,
// date) leave, so a chip's count is exactly the rows it shows. Returns those
// rows narrowed to the picked chip, and the chip row to render above the
// table.
export function useStatusChips<T>(
  rows: T[],
  statusOf: (row: T) => string,
  chips: StatusChip[],
  ariaLabel: string,
): { shown: T[]; picked: boolean; bar: ReactNode } {
  const { token } = theme.useToken();
  const [chip, setChip] = useState("all");
  const countOf = (key: string) =>
    key === "all" ? rows.length : rows.filter((r) => statusOf(r) === key).length;
  const shown = chip === "all" ? rows : rows.filter((r) => statusOf(r) === chip);

  const bar = (
    <div
      style={{
        borderTop: `1px solid ${token.colorBorderSecondary}`,
        marginTop: 16,
        paddingTop: 20,
      }}
    >
      <FilterChips<string>
        ariaLabel={ariaLabel}
        value={chip}
        onChange={setChip}
        chips={[
          { key: "all", label: <Trans context="document filter">All</Trans>, count: rows.length },
          ...chips
            .filter((c) => !c.hideWhenEmpty || chip === c.key || countOf(c.key) > 0)
            .map((c) => ({ key: c.key, label: c.label, count: countOf(c.key) })),
        ]}
      />
    </div>
  );
  return { shown, picked: chip !== "all", bar };
}
