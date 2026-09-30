import { useState } from "react";
import { Tabs } from "antd";
import { Trans } from "@lingui/react/macro";
import { WalletOutlined } from "@ant-design/icons";

import PageHeader from "src/components/page-header";
import CashBookModals from "src/components/cash-book/modals";
import CashBookLayoutSwitch from "src/components/cash-book/layout-switch";
import Counter from "src/components/cash-book/counter";
import LoanRegister from "src/components/cash-book/loan-register";
import { useCashBook } from "src/components/cash-book/use-cash-book";
import type { Client } from "src/types/models";

// The new Cash Book layout: a Counter tab (serve a customer, ring up the
// sale, the till beside it) and a Customer loans tab (every customer with a
// loan, grouped, stalled ones flagged). Same hook, same modals and the same
// server calls as the current layout; only the arrangement differs. The loan
// query stays unscoped so the register always has every customer's rows.
const CashBookV2 = () => {
  const cb = useCashBook({ scopeLoanStatusToClient: false });
  const [tab, setTab] = useState<"counter" | "loans">("counter");

  const serve = (client: Client) => {
    cb.selectClient(client);
    setTab("counter");
  };

  return (
    <>
      <PageHeader
        icon={<WalletOutlined />}
        title={<Trans>Cash Book</Trans>}
        extra={<CashBookLayoutSwitch disabled={cb.inSale} />}
        style={{ marginBottom: 8 }}
      />
      <Tabs
        activeKey={tab}
        onChange={(key) => setTab(key as "counter" | "loans")}
        items={[
          {
            key: "counter",
            label: <Trans context="cash book tab">Counter</Trans>,
            children: <Counter cb={cb} />,
          },
          {
            key: "loans",
            label: <Trans>Customer loans</Trans>,
            children: <LoanRegister cb={cb} onServe={serve} />,
          },
        ]}
      />
      <CashBookModals cb={cb} />
    </>
  );
};

export default CashBookV2;
