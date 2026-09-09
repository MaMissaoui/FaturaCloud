import { Button, Card, Checkbox, Popconfirm, Space, Typography, theme } from "antd";
import { ExclamationCircleOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";

const { Text } = Typography;

export interface OrganizationDangerZoneProps {
  resetMasterData: boolean;
  resetTransactionalData: boolean;
  resetting: boolean;
  resetBreakdown: [number, string][];
  resetSelected: boolean;
  onMasterDataChange: (checked: boolean) => void;
  onTransactionalDataChange: (checked: boolean) => void;
  onPopconfirmOpenChange: (open: boolean) => void;
  onConfirmReset: () => void;
}

// Purely presentational — every fetch/handler lives in the parent page
// (src/routes/organizations/index.tsx). Extracted from that file (F151,
// 2026-09-09) to cut its size, not to change when or how data loads.
export default function OrganizationDangerZone({
  resetMasterData,
  resetTransactionalData,
  resetting,
  resetBreakdown,
  resetSelected,
  onMasterDataChange,
  onTransactionalDataChange,
  onPopconfirmOpenChange,
  onConfirmReset,
}: OrganizationDangerZoneProps) {
  const { token } = theme.useToken();

  return (
    <Card
      size="small"
      title={<Trans>Danger zone</Trans>}
      style={{ marginTop: 12, borderColor: token.colorErrorBorder }}
    >
      <Space direction="vertical" size={8} style={{ width: "100%" }}>
        <Text type="secondary">
          <Trans>
            Permanently delete this organization's data without deleting the organization itself.
          </Trans>
        </Text>
        <Checkbox
          checked={resetMasterData}
          onChange={(e) => {
            const checked = e.target.checked;
            onMasterDataChange(checked);
            if (checked) onTransactionalDataChange(true);
          }}
        >
          <Trans>Master data</Trans>{" "}
          <Text type="secondary">({t`clients, vendors, products, tax rates`})</Text>
        </Checkbox>
        <Checkbox
          checked={resetTransactionalData}
          disabled={resetMasterData}
          onChange={(e) => onTransactionalDataChange(e.target.checked)}
        >
          <Trans>Transactional data</Trans>{" "}
          <Text type="secondary">
            (
            {t`invoices, orders, deliveries, purchase orders, goods receipts, incoming invoices, stock movements`}
            )
          </Text>
        </Checkbox>
        {resetMasterData && (
          <Text type="secondary" style={{ fontSize: 12 }}>
            <Trans>
              Master data can't be reset on its own — documents reference clients and vendors, so
              transactional data is included automatically.
            </Trans>
          </Text>
        )}
        <Popconfirm
          title={t`Reset this organization's data?`}
          description={
            <div style={{ maxWidth: 280 }}>
              {resetBreakdown.length > 0 ? (
                <>
                  <div>
                    <Trans>This will permanently delete:</Trans>
                  </div>
                  <ul style={{ margin: "4px 0", paddingLeft: 18 }}>
                    {resetBreakdown.map(([n, label]) => (
                      <li key={label}>
                        {n} {label}
                      </li>
                    ))}
                  </ul>
                </>
              ) : (
                <div>
                  <Trans>Nothing matches the current selection.</Trans>
                </div>
              )}
              <div>
                <Trans>This cannot be undone.</Trans>
              </div>
            </div>
          }
          okButtonProps={{ danger: true }}
          onOpenChange={onPopconfirmOpenChange}
          onConfirm={onConfirmReset}
          disabled={!resetSelected}
        >
          <Button
            danger
            icon={<ExclamationCircleOutlined />}
            loading={resetting}
            disabled={!resetSelected}
          >
            <Trans>Reset selected data</Trans>
          </Button>
        </Popconfirm>
      </Space>
    </Card>
  );
}
