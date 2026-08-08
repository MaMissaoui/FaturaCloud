import { useEffect, useState } from "react";
import { Button, Card, Select, Space, Typography } from "antd";
import { useAtomValue, useSetAtom } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import { ExportOutlined } from "@ant-design/icons";

import { DownloadDATEV, DownloadFEC } from "src/api";
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
  const [downloadingFEC, setDownloadingFEC] = useState(false);
  const [downloadingDATEV, setDownloadingDATEV] = useState(false);

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
    setDownloadingFEC(true);
    try {
      await DownloadFEC(organizationId, fiscalYearId);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`FEC export failed`);
    } finally {
      setDownloadingFEC(false);
    }
  };

  const handleDownloadDATEV = async () => {
    if (!organizationId || !fiscalYearId) return;
    setDownloadingDATEV(true);
    try {
      await DownloadDATEV(organizationId, fiscalYearId);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`DATEV export failed`);
    } finally {
      setDownloadingDATEV(false);
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
                loading={downloadingFEC}
                disabled={!fiscalYearId}
                onClick={handleDownloadFEC}
              >
                <Trans>Export FEC</Trans>
              </Button>
            </Space>
          </Space>
        </Card>

        <Card title={<Trans>Germany — DATEV</Trans>}>
          <Space direction="vertical" size={12} style={{ width: "100%" }}>
            <Text type="secondary">
              <Trans>
                Exports every posted journal line for the selected fiscal year as a DATEV
                Buchungsstapel EXTF file. Requires a DATEV consultant number, client number, and a
                DATEV account number on every account referenced by a posted entry — configure
                these under Accounting → Chart of Accounts and Organization settings.
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
                loading={downloadingDATEV}
                disabled={!fiscalYearId}
                onClick={handleDownloadDATEV}
              >
                <Trans>Export DATEV</Trans>
              </Button>
            </Space>
          </Space>
        </Card>
      </Space>
    </div>
  );
};

export default SettingsGLExport;
