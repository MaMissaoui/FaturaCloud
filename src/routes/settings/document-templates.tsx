import { useEffect, useState } from "react";
import { App, Button, Card, Select, Space, Tag, Typography, Upload } from "antd";
import type { UploadProps } from "antd";
import { useAtomValue } from "jotai";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import {
  FileExcelOutlined,
  UploadOutlined,
  DeleteOutlined,
  DownloadOutlined,
} from "@ant-design/icons";
import isEmpty from "lodash/isEmpty";

import { organizationAtom } from "src/atoms/organization";
import {
  ListDocumentTemplates,
  DownloadDocumentTemplate,
  UploadDocumentTemplate,
  DeleteDocumentTemplate,
  GetDocumentTemplateOrientation,
  UpdateDocumentTemplateOrientation,
} from "src/api";

const { Title, Text } = Typography;

// One card per document type. v1 (issue #115) covered only Invoice; every
// other document type — Purchase Order, Order, Incoming Invoice, Outbound
// Delivery, Inbound Delivery — reuses this same component with a different
// documentType/label.
function DocumentTemplateCard({
  documentType,
  label,
  orgId,
  hasOverride,
  onChange,
}: {
  documentType: string;
  label: string;
  orgId: string;
  hasOverride: boolean;
  onChange: (documentType: string, hasOverride: boolean) => void;
}) {
  const { message } = App.useApp();
  const [uploading, setUploading] = useState(false);
  const [deleting, setDeleting] = useState(false);
  // "" (no override set yet) renders identically to "portrait" — every
  // embedded default template has no <pageSetup> orientation of its own —
  // so the Select just displays "portrait" for that case rather than
  // needing a third "unset" option with no visible difference.
  const [orientation, setOrientation] = useState<"portrait" | "landscape">("portrait");
  const [orientationSaving, setOrientationSaving] = useState(false);

  useEffect(() => {
    GetDocumentTemplateOrientation(orgId, documentType)
      .then((res) => {
        if (res.orientation === "landscape") setOrientation("landscape");
      })
      .catch((error) => {
        message.error(error instanceof Error ? error.message : t`Failed to load page orientation`);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- message is stable from App.useApp()
  }, [orgId, documentType]);

  const handleOrientationChange = async (value: "portrait" | "landscape") => {
    const previous = orientation;
    setOrientation(value);
    setOrientationSaving(true);
    try {
      await UpdateDocumentTemplateOrientation(orgId, documentType, value);
    } catch (error) {
      setOrientation(previous);
      message.error(error instanceof Error ? error.message : t`Failed to save page orientation`);
    } finally {
      setOrientationSaving(false);
    }
  };

  const uploadProps: UploadProps = {
    accept: ".xlsx",
    showUploadList: false,
    beforeUpload: async (file) => {
      setUploading(true);
      try {
        await UploadDocumentTemplate(orgId, documentType, file);
        onChange(documentType, true);
        message.success(t`Template uploaded`);
      } catch (error) {
        message.error(error instanceof Error ? error.message : t`Upload failed`);
      } finally {
        setUploading(false);
      }
      return false; // prevent antd's own auto-upload — UploadDocumentTemplate already sent it
    },
  };

  const handleDownload = async (variant: "current" | "default") => {
    try {
      if (variant === "default") {
        // A dedicated "download default" isn't a separate endpoint — the
        // same GET already falls back to it when there's no override, which
        // is exactly what "reset to default" wants to show the user first.
        await DownloadDocumentTemplate(orgId, documentType, `${documentType}_default.xlsx`);
      } else {
        await DownloadDocumentTemplate(orgId, documentType, `${documentType}.xlsx`);
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Download failed`);
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    try {
      await DeleteDocumentTemplate(orgId, documentType);
      onChange(documentType, false);
      message.success(t`Reverted to the default template`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : t`Delete failed`);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Card
      size="small"
      title={
        <Space>
          <FileExcelOutlined />
          {label}
        </Space>
      }
      style={{ marginBottom: 16 }}
      extra={
        hasOverride ? (
          <Tag color="blue">
            <Trans>Custom</Trans>
          </Tag>
        ) : (
          <Tag>
            <Trans>Default</Trans>
          </Tag>
        )
      }
    >
      <Text type="secondary">
        <Trans>
          Download the template, edit the placeholders in a spreadsheet app, then upload it back.
          There's no preview — export a document to see the result.
        </Trans>
      </Text>
      <div style={{ marginTop: 16 }}>
        <Space wrap>
          <Button icon={<DownloadOutlined />} onClick={() => handleDownload("current")}>
            <Trans>Download current</Trans>
          </Button>
          <Button icon={<DownloadOutlined />} onClick={() => handleDownload("default")}>
            <Trans>Download default</Trans>
          </Button>
          <Upload {...uploadProps}>
            <Button icon={<UploadOutlined />} loading={uploading}>
              <Trans>Upload template</Trans>
            </Button>
          </Upload>
          {hasOverride && (
            <Button danger icon={<DeleteOutlined />} loading={deleting} onClick={handleDelete}>
              <Trans>Reset to default</Trans>
            </Button>
          )}
        </Space>
      </div>
      <div style={{ marginTop: 12 }}>
        <Space align="center">
          <Text type="secondary">
            <Trans>Page orientation</Trans>
          </Text>
          <Select<"portrait" | "landscape">
            value={orientation}
            onChange={handleOrientationChange}
            loading={orientationSaving}
            disabled={orientationSaving}
            style={{ width: 140 }}
            options={[
              { value: "portrait", label: t`Portrait` },
              { value: "landscape", label: t`Landscape` },
            ]}
          />
        </Space>
      </div>
    </Card>
  );
}

function SettingsDocumentTemplates() {
  const organization = useAtomValue(organizationAtom);
  const { message } = App.useApp();
  // documentType -> whether it currently has an uploaded override. Loaded
  // once from the server (ListDocumentTemplates) so the cards reflect
  // reality on page load, not just what happened in this session.
  const [overrides, setOverrides] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (!organization?.id) return;
    ListDocumentTemplates(organization.id)
      .then((templates) => {
        const byType: Record<string, boolean> = {};
        for (const tpl of templates) byType[tpl.documentType] = true;
        setOverrides(byType);
      })
      .catch((error) => {
        message.error(error instanceof Error ? error.message : t`Failed to load templates`);
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- message is stable from App.useApp()
  }, [organization?.id]);

  if (isEmpty(organization)) return null;

  const handleChange = (documentType: string, hasOverride: boolean) => {
    setOverrides((prev) => ({ ...prev, [documentType]: hasOverride }));
  };

  return (
    <div style={{ maxWidth: 900 }}>
      <Title level={3} style={{ marginTop: 0, marginBottom: 12 }}>
        <FileExcelOutlined style={{ marginRight: 8 }} />
        <Trans>Document Templates</Trans>
      </Title>
      <Text type="secondary" style={{ display: "block", marginBottom: 16 }}>
        <Trans>
          Upload a custom Excel layout for a document type, or download the current one to edit it.
          To use a custom template for invoices, set the invoice layout to "Custom" in the
          Organizations Formatting settings.
        </Trans>
      </Text>

      <DocumentTemplateCard
        documentType="invoice"
        label={t`Invoice`}
        orgId={organization.id}
        hasOverride={!!overrides.invoice}
        onChange={handleChange}
      />
      <DocumentTemplateCard
        documentType="purchase_order"
        label={t`Purchase Order`}
        orgId={organization.id}
        hasOverride={!!overrides.purchase_order}
        onChange={handleChange}
      />
      <DocumentTemplateCard
        documentType="order"
        label={t`Order`}
        orgId={organization.id}
        hasOverride={!!overrides.order}
        onChange={handleChange}
      />
      <DocumentTemplateCard
        documentType="incoming_invoice"
        label={t`Incoming Invoice`}
        orgId={organization.id}
        hasOverride={!!overrides.incoming_invoice}
        onChange={handleChange}
      />
      <DocumentTemplateCard
        documentType="delivery"
        label={t`Outbound Delivery`}
        orgId={organization.id}
        hasOverride={!!overrides.delivery}
        onChange={handleChange}
      />
      <DocumentTemplateCard
        documentType="inbound_delivery"
        label={t`Inbound Delivery`}
        orgId={organization.id}
        hasOverride={!!overrides.inbound_delivery}
        onChange={handleChange}
      />
    </div>
  );
}

export default SettingsDocumentTemplates;
