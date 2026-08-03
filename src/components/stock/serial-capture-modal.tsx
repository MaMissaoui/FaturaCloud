import { useEffect, useState } from "react";
import { Alert, Modal, Select, Space, Typography } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import filter from "lodash/filter";
import uniq from "lodash/uniq";

import { productSerialNumbersAtom, loadProductSerialNumbersAtom } from "src/atoms/serial-number";

const { Text } = Typography;

export interface SerialCaptureLine {
  lineItemId: string;
  productId: string;
  productName: string;
  quantity: number;
}

interface SerialCaptureModalProps {
  open: boolean;
  // "receive": free-text entry for new/returning units (inbound delivery).
  // "ship": pick from this product's currently in-stock units (outbound
  // delivery) — the server enforces both independently either way.
  mode: "receive" | "ship";
  lines: SerialCaptureLine[];
  onCancel: () => void;
  onConfirm: (serialNumbers: Record<string, string[]>) => void;
  confirming?: boolean;
}

// Serial capture reads from a document's already-persisted line items (the
// caller passes `lines` resolved from GetDeliveryLineItems/
// GetInboundDeliveryLineItems), not live form state — an inbound receipt's
// productId can be resolved server-side from a linked purchase-order line,
// so it isn't reliably present in the antd form before that save round-trips.
const SerialCaptureModal = ({
  open,
  mode,
  lines,
  onCancel,
  onConfirm,
  confirming,
}: SerialCaptureModalProps) => {
  // Read-only cache access via useAtomValue and a plain useSetAtom write
  // trigger — never useAtom on the async write atom inside a Modal, or the
  // mask gets orphaned and freezes the app. All per-line selection is local
  // useState, never a module-level atom.
  const serialNumbersByProduct = useAtomValue(productSerialNumbersAtom);
  const loadSerialNumbers = useSetAtom(loadProductSerialNumbersAtom);
  const [selection, setSelection] = useState<Record<string, string[]>>({});

  useEffect(() => {
    if (!open) return;
    setSelection({});
    if (mode === "ship") {
      uniq(lines.map((l) => l.productId)).forEach((productId) => loadSerialNumbers(productId));
    }
  }, [open, mode, lines, loadSerialNumbers]);

  const handleChange = (lineItemId: string, values: string[]) => {
    setSelection((prev) => ({ ...prev, [lineItemId]: values }));
  };

  // A serial picked for one line of a product is removed from the pool
  // offered to another line of the same product in this same modal — a
  // client-side convenience only; the server re-validates independently.
  const pickedElsewhere = (productId: string, exceptLineItemId: string): Set<string> => {
    const picked = new Set<string>();
    lines.forEach((l) => {
      if (l.productId === productId && l.lineItemId !== exceptLineItemId) {
        (selection[l.lineItemId] ?? []).forEach((s) => picked.add(s));
      }
    });
    return picked;
  };

  const allSatisfied =
    lines.length > 0 && lines.every((l) => (selection[l.lineItemId] ?? []).length === l.quantity);

  return (
    <Modal
      title={
        mode === "receive" ? (
          <Trans>Enter received serial numbers</Trans>
        ) : (
          <Trans>Select serial numbers to ship</Trans>
        )
      }
      open={open}
      onOk={() => onConfirm(selection)}
      onCancel={onCancel}
      okButtonProps={{ disabled: !allSatisfied }}
      confirmLoading={confirming}
      okText={<Trans>Confirm</Trans>}
      cancelText={<Trans>Cancel</Trans>}
      width={560}
      destroyOnHidden
    >
      <Space direction="vertical" style={{ width: "100%" }} size="middle">
        <Alert
          type="info"
          showIcon
          message={
            mode === "receive" ? (
              <Trans>Enter exactly one serial number per unit received.</Trans>
            ) : (
              <Trans>Select exactly one serial number per unit shipped.</Trans>
            )
          }
        />
        {lines.map((line) => {
          const picked = selection[line.lineItemId] ?? [];
          const excluded = pickedElsewhere(line.productId, line.lineItemId);
          const options = filter(
            serialNumbersByProduct[line.productId] ?? [],
            (s: any) => s.inStock && !excluded.has(s.serialNumber),
          ).map((s: any) => ({ value: s.serialNumber, label: s.serialNumber }));

          return (
            <div key={line.lineItemId}>
              <Text strong>
                {line.productName} — {picked.length} / {line.quantity}
              </Text>
              <div style={{ marginTop: 4 }}>
                {mode === "receive" ? (
                  <Select
                    mode="tags"
                    open={false}
                    suffixIcon={null}
                    style={{ width: "100%" }}
                    placeholder={t`Type a serial number and press Enter`}
                    tokenSeparators={[",", "\n"]}
                    value={picked}
                    onChange={(values) => handleChange(line.lineItemId, values)}
                  />
                ) : (
                  <Select
                    mode="multiple"
                    showSearch
                    style={{ width: "100%" }}
                    placeholder={t`Select in-stock serial numbers`}
                    options={options}
                    value={picked}
                    onChange={(values) => handleChange(line.lineItemId, values)}
                  />
                )}
              </div>
            </div>
          );
        })}
      </Space>
    </Modal>
  );
};

export default SerialCaptureModal;
