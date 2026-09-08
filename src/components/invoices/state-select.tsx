import { Dropdown, Space, Tag } from "antd";
import { useSetAtom } from "jotai";
import { MoreOutlined } from "@ant-design/icons";

import type { MenuProps } from "antd";

import { updateInvoiceStateAtom } from "src/atoms/invoice";
import { INVOICE_STATES, invoiceStateColor, invoiceStateLabel } from "src/types/invoice";

const InvoiceStateSelect = ({
  invoice,
  onChanged,
}: {
  invoice: { id: string; state: string };
  // Called after a successful state change. The list page (which owns
  // invoicesAtom, already updated by updateInvoiceStateAtom itself) has no
  // need for this; the details page's invoiceAtom is a separate fetch keyed
  // off invoiceIdAtom and needs its own nudge to refetch — see its usage.
  onChanged?: () => void;
}) => {
  const updateInvoiceState = useSetAtom(updateInvoiceStateAtom);

  const changeState = async (toState: string) => {
    await updateInvoiceState({ invoiceId: invoice.id, state: toState });
    onChanged?.();
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
