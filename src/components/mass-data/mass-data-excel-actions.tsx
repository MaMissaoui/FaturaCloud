import { useState } from "react";
import { App, Button, Modal, Space, Table, Typography } from "antd";
import type { UploadProps } from "antd";
import { Upload } from "antd";
import { DownloadOutlined, UploadOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";

import {
  DownloadMassData,
  ImportMassData,
  type MassDataResource,
  type MassDataImportResult,
  type MassDataRowResult,
} from "src/api";

const { Text } = Typography;

// MassDataExcelActions is the Download/Upload Excel pair every master-data
// list page (Clients, Vendors, Products, Tax Rates, Payment Terms, Units of
// Measure, Chart of Accounts) renders next to its own "New ..." button —
// one shared component rather than seven near-identical copies, since the
// mechanics (fetch a blob, save it; upload a file, show what happened) are
// identical and only the resource/filename differ. See db/mass_data.go for
// what the exported/imported spreadsheet actually looks like per table.
export default function MassDataExcelActions({
  organizationId,
  resource,
  filenamePrefix,
  onImported,
}: {
  organizationId: string;
  resource: MassDataResource;
  filenamePrefix: string;
  onImported: () => void;
}) {
  const { message } = App.useApp();
  const [downloading, setDownloading] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [result, setResult] = useState<MassDataImportResult | null>(null);

  const handleDownload = async () => {
    setDownloading(true);
    try {
      await DownloadMassData(organizationId, resource, `${filenamePrefix}.xlsx`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Download failed`);
    } finally {
      setDownloading(false);
    }
  };

  const uploadProps: UploadProps = {
    accept: ".xlsx",
    showUploadList: false,
    beforeUpload: async (file) => {
      setUploading(true);
      try {
        const res = await ImportMassData(organizationId, resource, file);
        setResult(res);
        if (res.failed === 0) {
          message.success(t`Import complete: ${res.created} created, ${res.updated} updated.`);
        }
        onImported();
      } catch (error) {
        message.error(error instanceof Error ? error.message : t`Upload failed`);
      } finally {
        setUploading(false);
      }
      return false; // prevent antd's own auto-upload — ImportMassData already sent it
    },
  };

  const failedRows: MassDataRowResult[] = result?.rows.filter((r) => r.action === "error") ?? [];

  return (
    <>
      <Space wrap>
        <Button icon={<DownloadOutlined />} loading={downloading} onClick={handleDownload}>
          <Trans>Download Excel</Trans>
        </Button>
        <Upload {...uploadProps}>
          <Button icon={<UploadOutlined />} loading={uploading}>
            <Trans>Upload Excel</Trans>
          </Button>
        </Upload>
      </Space>
      <Modal
        open={result !== null && result.failed > 0}
        title={<Trans>Import results</Trans>}
        onCancel={() => setResult(null)}
        onOk={() => setResult(null)}
        okText={<Trans>Close</Trans>}
        cancelButtonProps={{ style: { display: "none" } }}
        width={640}
      >
        {result && (
          <>
            <Text>
              {t`${result.created} created, ${result.updated} updated, ${result.failed} failed.`}
            </Text>
            <Table
              size="small"
              rowKey="row"
              pagination={false}
              dataSource={failedRows}
              style={{ marginTop: 16 }}
              columns={[
                { title: t`Row`, dataIndex: "row", width: 60 },
                { title: t`Record`, dataIndex: "identifier" },
                { title: t`Error`, dataIndex: "error" },
              ]}
            />
          </>
        )}
      </Modal>
    </>
  );
}
