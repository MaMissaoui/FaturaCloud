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
export default function StatusFlow<S extends string>({
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

  return (
    <Popover
      title={<Trans>Status flow</Trans>}
      content={
        <Space direction="vertical" size={4}>
          {rows.map((status) => {
            const next = transitions[status] ?? [];
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
        </Space>
      }
    >
      <QuestionCircleOutlined style={{ marginLeft: 8, color: "#999", cursor: "pointer" }} />
    </Popover>
  );
}
