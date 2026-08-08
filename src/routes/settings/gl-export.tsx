import { useEffect, useState } from "react";
import { Alert, Button, Card, Select, Space, Typography } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ExportOutlined } from "@ant-design/icons";

import { DownloadFEC } from "src/api";
import { organizationIdAtom } from "src/atoms/organization";
import { fiscalYearsAtom, setFiscalYearsAtom } from "src/atoms/fiscal-period";
import { message } from "src/utils/message";

const { Title, Text } = Typography;

const SettingsGLExport = () => {
  useLingui();
  const organizationId = useAtomValue(organizationIdAtom);
  const fiscalYears = useAtomValue(fiscalYearsAtom);
  const setFiscalYears = useSetAtom(setFiscalYearsAtom);

  const [fiscalYearId, setFiscalYearId] = useState<string>("");
  const [downloading, setDownloading] = useState(false);

  useEffect(() => {
    setFiscalYears();
  }, [setFiscalYears]);

  useEffect(() => {
    if (fiscalYearId || fiscalYears.length === 0) return;
    const now = Date.now();
    const current = fiscalYears.find((y) => y.startDate <= now && now <= y.endDate);
    setFiscalYearId((current ?? fiscalYears[0]).id);
  }, [fiscalYears, fiscalYearId]);

  const handleDownloadFEC = async () => {
    if (!organizationId || !fiscalYearId) return;
    setDownloading(true);
    try {
      await DownloadFEC(organizationId, fiscalYearId);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`FEC export failed`);
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div style={{ maxWidth: 720 }}>
      <Title level={3} style={{ marginTop: 0, marginBottom: 20 }}>
        <ExportOutlined style={{ marginRight: 8 }} />
        <Trans>GL Export</Trans>
      </Title>

      <Space direction="vertical" size="middle" style={{ width: "100%" }}>
        <Card title={<Trans>France — FEC</Trans>}>
          <Space direction="vertical" size={12} style={{ width: "100%" }}>
            <Text type="secondary">
              <Trans>
                Exports every posted journal line for the selected fiscal year as a
                SirenFECAAAAMMJJ.txt file, per the mandated French tax authority format. Requires a
                9-digit SIREN in the organization's registration number.
              </Trans>
            </Text>
            <Space>
              <Select
                placeholder={t`Select a fiscal year`}
                style={{ width: 180 }}
                value={fiscalYearId || undefined}
                onChange={setFiscalYearId}
                options={fiscalYears.map((y) => ({ value: y.id, label: y.name }))}
              />
              <Button
                type="primary"
                icon={<ExportOutlined />}
                loading={downloading}
                disabled={!fiscalYearId}
                onClick={handleDownloadFEC}
              >
                <Trans>Export FEC</Trans>
              </Button>
            </Space>
          </Space>
        </Card>

        <Card title={<Trans>Germany — DATEV</Trans>}>
          <Alert
            type="info"
            showIcon
            message={<Trans>Not yet available</Trans>}
            description={
              <Trans>
                DATEV Buchungsstapel export requires validating the exact column layout and format
                version against a current DATEV EXTF specification, which isn't available in this
                environment. Shipping an unverified layout risks producing a file that looks
                plausible but is rejected on import, so this export is deferred rather than guessed
                at.
              </Trans>
            }
          />
        </Card>
      </Space>
    </div>
  );
};

export default SettingsGLExport;
