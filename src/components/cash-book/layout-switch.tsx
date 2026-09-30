import { Segmented, Tooltip } from "antd";
import { useAtom } from "jotai";
import { t } from "@lingui/core/macro";

import { cashBookLayoutAtom, type CashBookLayout } from "src/atoms/generic";

// Current vs. new Cash Book layout, shown in both layouts' header. Disabled
// while a sale is in progress: each layout keeps its own sale state, so
// switching mid-sale would drop the lines already entered.
const CashBookLayoutSwitch = ({ disabled }: { disabled: boolean }) => {
  const [layout, setLayout] = useAtom(cashBookLayoutAtom);
  return (
    <Tooltip title={disabled ? t`Finish or leave the sale to switch layout` : undefined}>
      <Segmented<CashBookLayout>
        aria-label={t`Cash Book layout`}
        value={layout}
        onChange={setLayout}
        disabled={disabled}
        options={[
          { label: t`Current layout`, value: "v1" },
          { label: t`New layout`, value: "v2" },
        ]}
      />
    </Tooltip>
  );
};

export default CashBookLayoutSwitch;
