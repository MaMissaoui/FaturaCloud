import { Dropdown, Space, Tag } from "antd";
import { useSetAtom } from "jotai";
import { MoreOutlined } from "@ant-design/icons";

import type { MenuProps } from "antd";

import { updateInvoiceStateAtom } from "src/atoms/invoice";
import { INVOICE_STATES, invoiceStateColor, invoiceStateLabel } from "src/types/invoice";

const InvoiceStateSelect = ({ invoice }: { invoice: { id: string; state: string } }) => {
  const updateInvoiceState = useSetAtom(updateInvoiceStateAtom);

  const changeState = async (toState: string) => {
    await updateInvoiceState({ invoiceId: invoice.id, state: toState });
  };

  const items: MenuProps["items"] = INVOICE_STATES.map((state) => ({
    key: state,
    label: invoiceStateLabel(state),
  }));

  const color = INVOICE_STATES.includes(invoice.state as never)
    ? invoiceStateColor[invoice.state as keyof typeof invoiceStateColor]
    : undefined;

  return (
    <Dropdown
      menu={{
        items,
        selectable: true,
        selectedKeys: [invoice.state],
        onSelect: ({ key }) => {
          changeState(key);
        },
      }}
    >
      <Tag color={color} style={{ marginInlineEnd: 0, cursor: "pointer" }}>
        <Space size={4} style={{ fontSize: 12 }}>
          {invoiceStateLabel(invoice.state)}
          <MoreOutlined />
        </Space>
      </Tag>
    </Dropdown>
  );
};

export default InvoiceStateSelect;
