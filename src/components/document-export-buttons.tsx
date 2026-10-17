import { useState } from "react";
import { Button, Space, Tooltip } from "antd";
import { FileExcelOutlined, FilePdfOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";

import { message } from "src/utils/message";

export type DocumentExportFormat = "pdf" | "xlsx";

// DocumentExportButtons is the PDF and Excel pair every document detail page
// shows (audit F185). Both fill the organization's Excel template on the
// server (db/xlsx_export*.go, db/pdf_convert.go) from the saved document, so
// they are disabled while the form has unsaved changes.
export default function DocumentExportButtons({
  isDirty,
  exportDocument,
}: {
  isDirty: boolean;
  exportDocument: (format: DocumentExportFormat) => Promise<unknown>;
}) {
  const [downloading, setDownloading] = useState<DocumentExportFormat | null>(null);
  const saveFirst = isDirty ? t`Save your changes before exporting` : undefined;

  const run = (format: DocumentExportFormat) => async () => {
    setDownloading(format);
    try {
      await exportDocument(format);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Export failed`);
    } finally {
      setDownloading(null);
    }
  };

  return (
    <Space size="small">
      <Tooltip title={saveFirst}>
        <Button disabled={isDirty} loading={downloading === "pdf"} onClick={run("pdf")}>
          <FilePdfOutlined /> PDF
        </Button>
      </Tooltip>
      <Tooltip title={saveFirst}>
        <Button disabled={isDirty} loading={downloading === "xlsx"} onClick={run("xlsx")}>
          <FileExcelOutlined /> <Trans>Excel</Trans>
        </Button>
      </Tooltip>
    </Space>
  );
}
