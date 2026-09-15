import { useEffect, useState } from "react";
import { Alert, InputNumber, Input, Modal, Segmented, Select, Space, Typography } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import uniq from "lodash/uniq";

import { productSerialNumbersAtom, loadProductSerialNumbersAtom } from "src/atoms/serial-number";

const { Text } = Typography;

export interface SerialCaptureLine {
  lineItemId: string;
  productId: string;
  productName: string;
  quantity: number;
}

// The reserved range a Production Order's linked Import may carry (see
// CLAUDE.md's db/import.go note) — used only to prefill and hint the
// "produce" mode's range generator; the server is the actual authority on
// whether a generated serial falls inside it.
export interface SerialCaptureImportRange {
  prefix: string;
  start: number;
  end: number;
  importNumber: string;
}

interface SerialCaptureModalProps {
  open: boolean;
  // "receive": free-text entry for new/returning units (inbound delivery).
  // "ship": pick from this product's currently in-stock units (outbound
  // delivery) — the server enforces both independently either way.
  // "produce": free-text entry for units a production order just made,
  // same as "receive" plus an optional range generator (see below).
  mode: "receive" | "ship" | "produce";
  lines: SerialCaptureLine[];
  onCancel: () => void;
  onConfirm: (serialNumbers: Record<string, string[]>) => void;
  confirming?: boolean;
  // "produce" only — prefills the range generator and shows a hint. Null/
  // undefined when the order has no linked import or that import has no
  // range configured.
  importRange?: SerialCaptureImportRange | null;
}

// Generates `count` consecutive serials from `start`, unpadded — matches
// the plain-integer convention shown by the Import form's own range fields
// (e.g. prefix "SN-", range 1001-1050 -> "SN-1001", "SN-1002", ...).
const generateRangeSerials = (prefix: string, start: number, count: number): string[] =>
  Array.from({ length: Math.max(count, 0) }, (_, i) => `${prefix}${start + i}`);

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
  importRange,
}: SerialCaptureModalProps) => {
  // Read-only cache access via useAtomValue and a plain useSetAtom write
  // trigger — never useAtom on the async write atom inside a Modal, or the
  // mask gets orphaned and freezes the app. All per-line selection is local
  // useState, never a module-level atom.
  const serialNumbersByProduct = useAtomValue(productSerialNumbersAtom);
  const loadSerialNumbers = useSetAtom(loadProductSerialNumbersAtom);
  const [selection, setSelection] = useState<Record<string, string[]>>({});
  // "produce" only — per-line toggle between typing serials and generating
  // them from a prefix+start range, and the range inputs' own values.
  const [entryMode, setEntryMode] = useState<Record<string, "type" | "range">>({});
  const [rangeValues, setRangeValues] = useState<
    Record<string, { prefix: string; start: number | null }>
  >({});

  useEffect(() => {
    if (!open) return;
    setSelection({});
    setEntryMode({});
    setRangeValues({});
    if (mode === "ship") {
      uniq(lines.map((l) => l.productId)).forEach((productId) => loadSerialNumbers(productId));
    }
  }, [open, mode, lines, loadSerialNumbers]);

  const handleChange = (lineItemId: string, values: string[]) => {
    setSelection((prev) => ({ ...prev, [lineItemId]: values }));
  };

  // Recomputes a line's selection from its current range inputs — called on
  // every prefix/start edit so the count always tracks the line's quantity
  // with no separate "generate" step.
  const applyRange = (
    lineItemId: string,
    quantity: number,
    prefix: string,
    start: number | null,
  ) => {
    setRangeValues((prev) => ({ ...prev, [lineItemId]: { prefix, start } }));
    if (start === null) {
      handleChange(lineItemId, []);
      return;
    }
    handleChange(lineItemId, generateRangeSerials(prefix, start, quantity));
  };

  const switchEntryMode = (line: SerialCaptureLine, next: "type" | "range") => {
    setEntryMode((prev) => ({ ...prev, [line.lineItemId]: next }));
    if (next === "range") {
      const existing = rangeValues[line.lineItemId];
      const prefix = existing?.prefix ?? importRange?.prefix ?? "";
      const start = existing?.start ?? importRange?.start ?? null;
      applyRange(line.lineItemId, line.quantity, prefix, start);
    } else {
      handleChange(line.lineItemId, []);
    }
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
        ) : mode === "produce" ? (
          <Trans>Enter produced serial numbers</Trans>
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
            ) : mode === "produce" ? (
              <Trans>Enter exactly one serial number per unit produced.</Trans>
            ) : (
              <Trans>Select exactly one serial number per unit shipped.</Trans>
            )
          }
        />
        {mode === "produce" && importRange && (
          <Alert
            type="info"
            showIcon
            message={
              <Trans>
                Import {importRange.importNumber} reserves {importRange.prefix}
                {importRange.start} to {importRange.prefix}
                {importRange.end}.
              </Trans>
            }
          />
        )}
        {lines.map((line) => {
          const picked = selection[line.lineItemId] ?? [];
          const excluded = pickedElsewhere(line.productId, line.lineItemId);
          // Native filter rather than lodash's: with an explicitly typed
          // predicate lodash's overloads widen the element type and lose
          // SerialNumber (audit 2026-09-14 F90).
          const options = (serialNumbersByProduct[line.productId] ?? [])
            .filter((s) => s.inStock && !excluded.has(s.serialNumber))
            .map((s) => ({ value: s.serialNumber, label: s.serialNumber }));
          const lineEntryMode = entryMode[line.lineItemId] ?? "type";
          const lineRange = rangeValues[line.lineItemId];

          return (
            <div key={line.lineItemId}>
              <Text strong>
                {line.productName} — {picked.length} / {line.quantity}
              </Text>
              {mode === "produce" && (
                <div style={{ marginTop: 4 }}>
                  <Segmented
                    size="small"
                    value={lineEntryMode}
                    onChange={(value) => switchEntryMode(line, value as "type" | "range")}
                    options={[
                      { label: t`Type manually`, value: "type" },
                      { label: t`Generate range`, value: "range" },
                    ]}
                  />
                </div>
              )}
              <div style={{ marginTop: 4 }}>
                {mode === "produce" && lineEntryMode === "range" ? (
                  <Space.Compact style={{ width: "100%" }}>
                    <Input
                      style={{ width: "40%" }}
                      placeholder={t`Prefix`}
                      value={lineRange?.prefix ?? ""}
                      onChange={(e) =>
                        applyRange(
                          line.lineItemId,
                          line.quantity,
                          e.target.value,
                          lineRange?.start ?? null,
                        )
                      }
                    />
                    <InputNumber
                      style={{ width: "60%" }}
                      placeholder={t`Range start`}
                      value={lineRange?.start ?? null}
                      onChange={(value) =>
                        applyRange(line.lineItemId, line.quantity, lineRange?.prefix ?? "", value)
                      }
                    />
                  </Space.Compact>
                ) : mode === "receive" || mode === "produce" ? (
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
              {mode === "produce" && lineEntryMode === "range" && picked.length > 0 && (
                <Text type="secondary" style={{ fontSize: 12 }}>
                  {picked[0]}
                  {picked.length > 1 ? ` … ${picked[picked.length - 1]}` : ""}
                </Text>
              )}
            </div>
          );
        })}
      </Space>
    </Modal>
  );
};

export default SerialCaptureModal;
