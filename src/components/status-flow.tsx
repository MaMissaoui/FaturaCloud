import { memo } from "react";
import { Popover, Space, Tag, Typography } from "antd";
import { QuestionCircleOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";

const { Text } = Typography;

interface StatusFlowProps<S extends string> {
  current: S;
  statuses: readonly S[];
  transitions: Partial<Record<S, readonly S[]>>;
  getLabel: (status: S) => string;
  getColor: (status: S) => string | undefined;
}

// A small info icon + popover showing a document type's full status
// transition matrix — every legal move from every status, not just the ones
// reachable from the current one (the footer's transition buttons already
// cover that). Only meaningful for document types with a real state machine
// enforced server-side (purchase orders, orders, deliveries, inbound
// deliveries — each has its own db/*.go transitions map); invoices and
// incoming invoices move freely between states with no matrix to show, so
// this component is deliberately not used on those two detail pages.
// Placed next to the status Tag on each applicable detail page.
function StatusFlowInner<S extends string>({
  current,
  statuses,
  transitions,
  getLabel,
  getColor,
}: StatusFlowProps<S>) {
  // A status with no outgoing moves adds nothing on its own row — it's
  // already visible as an arrow target wherever it's reachable from, so
  // spelling out "(final)" for it again is pure repetition. The one
  // exception is the current status: if it happens to be terminal, it still
  // needs its own row so "(current)" has somewhere to attach.
  const rows = statuses.filter(
    (status) => status === current || (transitions[status]?.length ?? 0) > 0,
  );

  // "cancelled" is reachable from nearly every other status (an escape hatch,
  // not a normal forward step), so inlining it as a target on each row
  // repeats the same tag across most of the popover. Pull it out of every
  // row's inline list and fold it into one summary line instead.
  const cancelSources = rows.filter((status) =>
    (transitions[status] ?? []).includes("cancelled" as S),
  );

  return (
    <Popover
      title={<Trans>Status flow</Trans>}
      content={
        <Space direction="vertical" size={4}>
          {rows.map((status) => {
            const next = (transitions[status] ?? []).filter((n) => n !== "cancelled");
            return (
              <div key={status}>
                <Tag color={getColor(status)}>{getLabel(status)}</Tag>
                {status === current && (
                  <Text type="secondary" italic>
                    {" "}
                    <Trans>(current)</Trans>
                  </Text>
                )}
                {next.length > 0 && (
                  <>
                    {" → "}
                    {next.map((n, i) => (
                      <span key={n}>
                        {i > 0 && ", "}
                        <Tag color={getColor(n)}>{getLabel(n)}</Tag>
                      </span>
                    ))}
                  </>
                )}
              </div>
            );
          })}
          {cancelSources.length > 0 && (
            <div>
              <Tag color={getColor("cancelled" as S)}>{getLabel("cancelled" as S)}</Tag>
              {" ← "}
              {cancelSources.map((s, i) => (
                <span key={s}>
                  {i > 0 && ", "}
                  <Tag color={getColor(s)}>{getLabel(s)}</Tag>
                </span>
              ))}
            </div>
          )}
        </Space>
      }
    >
      <QuestionCircleOutlined style={{ marginLeft: 8, color: "#999", cursor: "pointer" }} />
    </Popover>
  );
}

// memo() erases the generic signature on its own — the cast restores it so
// callers still get a type-checked `current`/`statuses`/etc. per status enum
// (PurchaseOrderStatus, OrderStatus, ...), same as before this was memoized.
// Callers must still pass a referentially stable getColor/getLabel (a
// module-level function, not an inline arrow) for this memo to do anything.
const StatusFlow = memo(StatusFlowInner) as typeof StatusFlowInner;

export default StatusFlow;
