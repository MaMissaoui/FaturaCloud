import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Button,
  Card,
  Col,
  Divider,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Typography,
  Upload,
  message,
} from "antd";
import type { UploadFile } from "antd";
import {
  CloudDownloadOutlined,
  DatabaseOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SaveOutlined,
  UploadOutlined,
} from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import dayjs from "dayjs";

import {
  type BackupConfig,
  type BackupEntry,
  GetBackupConfig,
  ListBackups,
  RestoreDatabase,
  RestoreNamedBackup,
  SetBackupConfig,
  TriggerBackup,
} from "src/api";

const { Title, Text } = Typography;

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

function SettingsBackup() {
  useLingui();
  const [messageApi, contextHolder] = message.useMessage();
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [backups, setBackups] = useState<BackupEntry[]>([]);
  const [loadingList, setLoadingList] = useState(true);
  const [triggering, setTriggering] = useState(false);
  const [restoringName, setRestoringName] = useState<string | null>(null);
  const [uploadFile, setUploadFile] = useState<UploadFile | null>(null);
  const [uploadRestoring, setUploadRestoring] = useState(false);

  const [config, setConfig] = useState<BackupConfig>({
    enabled: false,
    scheduleHour: 2,
    retentionDays: 30,
  });
  const [savingConfig, setSavingConfig] = useState(false);

  const fetchList = useCallback(() => {
    setLoadingList(true);
    ListBackups()
      .then(setBackups)
      .catch(() => setBackups([]))
      .finally(() => setLoadingList(false));
  }, []);

  useEffect(() => {
    fetchList();
    GetBackupConfig()
      .then(setConfig)
      .catch(() => {});
  }, [fetchList]);

  const nextRunLabel = useMemo(() => {
    const candidate = new Date();
    candidate.setUTCHours(config.scheduleHour, 0, 0, 0);
    if (candidate.getTime() <= Date.now()) candidate.setUTCDate(candidate.getUTCDate() + 1);
    return dayjs(candidate).format("DD/MM/YYYY HH:mm");
  }, [config.scheduleHour]);

  const handleSaveConfig = async () => {
    setSavingConfig(true);
    try {
      const saved = await SetBackupConfig(config);
      setConfig(saved);
      messageApi.success(t`Schedule saved`);
    } catch {
      messageApi.error(t`Failed to save schedule`);
    } finally {
      setSavingConfig(false);
    }
  };

  const handleTrigger = async () => {
    setTriggering(true);
    try {
      const filename = await TriggerBackup();
      messageApi.success(t`Backup downloaded: ${filename}`);
      fetchList();
    } catch (err) {
      messageApi.error(t`Backup failed: ${String(err)}`);
    } finally {
      setTriggering(false);
    }
  };

  const confirmRestoreNamed = (name: string) => {
    Modal.confirm({
      title: t`Restore database`,
      content: t`Replace all current data with backup "${name}"? This cannot be undone.`,
      okText: t`Restore`,
      okButtonProps: { danger: true },
      cancelText: t`Cancel`,
      onOk: async () => {
        setRestoringName(name);
        try {
          await RestoreNamedBackup(name);
          messageApi.success(t`Database restored — reload to see changes`);
        } catch (err) {
          messageApi.error(t`Restore failed: ${String(err)}`);
        } finally {
          setRestoringName(null);
        }
      },
    });
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = "";
    Modal.confirm({
      title: t`Restore database`,
      content: t`Replace all current data with the uploaded file? This cannot be undone.`,
      okText: t`Restore`,
      okButtonProps: { danger: true },
      cancelText: t`Cancel`,
      onOk: async () => {
        setUploadRestoring(true);
        try {
          await RestoreDatabase(file);
          messageApi.success(t`Database restored — reload to see changes`);
        } catch (err) {
          messageApi.error(t`Restore failed: ${String(err)}`);
        } finally {
          setUploadRestoring(false);
        }
      },
    });
  };

  const isRestoring = restoringName !== null || uploadRestoring;

  return (
    <div style={{ maxWidth: 720 }}>
      {contextHolder}
      <input
        ref={fileInputRef}
        type="file"
        accept=".db"
        style={{ display: "none" }}
        onChange={handleFileChange}
      />

      <Title level={4} style={{ marginTop: 0, marginBottom: 20 }}>
        <DatabaseOutlined style={{ marginRight: 8 }} />
        <Trans>Backup & Restore</Trans>
      </Title>

      <Space direction="vertical" size="middle" style={{ width: "100%" }}>

        {/* ── Schedule ── */}
        <Card title={<Trans>Automatic schedule</Trans>}>
          <Space direction="vertical" size={8}>
            <Space wrap align="center" size={[20, 8]}>
              <Space size={8}>
                <Switch
                  checked={config.enabled}
                  onChange={(v) => setConfig((c) => ({ ...c, enabled: v }))}
                />
                <Text><Trans>Enable automatic backups</Trans></Text>
              </Space>
              <Space size={6}>
                <Text type="secondary"><Trans>Hour (UTC)</Trans>:</Text>
                <Select
                  value={config.scheduleHour}
                  onChange={(v) => setConfig((c) => ({ ...c, scheduleHour: v }))}
                  style={{ width: 130 }}
                  disabled={!config.enabled}
                  options={Array.from({ length: 24 }, (_, i) => ({
                    value: i,
                    label: `${String(i).padStart(2, "0")}:00 UTC`,
                  }))}
                />
              </Space>
              <Space size={6}>
                <Text type="secondary"><Trans>Retention (days)</Trans>:</Text>
                <InputNumber
                  min={1}
                  max={365}
                  value={config.retentionDays}
                  onChange={(v) => setConfig((c) => ({ ...c, retentionDays: v ?? 30 }))}
                  disabled={!config.enabled}
                  style={{ width: 70 }}
                />
              </Space>
              <Button
                type="primary"
                size="small"
                icon={<SaveOutlined />}
                loading={savingConfig}
                onClick={handleSaveConfig}
              >
                <Trans>Save</Trans>
              </Button>
            </Space>
            {config.enabled && (
              <Text type="secondary" style={{ fontSize: 12 }}>
                <Trans>Next run: {nextRunLabel}</Trans>
              </Text>
            )}
          </Space>
        </Card>

        {/* ── Manual / upload ── */}
        <Card title={<Trans>Manual backup</Trans>}>
          <Space direction="vertical" size={12} style={{ width: "100%" }}>
            <Row gutter={[16, 0]} align="middle">
              <Col>
                <Button
                  type="primary"
                  icon={<CloudDownloadOutlined />}
                  loading={triggering}
                  onClick={handleTrigger}
                >
                  <Trans>Download backup</Trans>
                </Button>
              </Col>
              <Col>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  <Trans>Creates a snapshot, saves it to the backup list, and downloads it.</Trans>
                </Text>
              </Col>
            </Row>

            <Divider style={{ margin: "4px 0" }} />

            <Row gutter={[16, 0]} align="middle">
              <Col>
                <Upload
                  accept=".db"
                  maxCount={1}
                  beforeUpload={() => false}
                  fileList={uploadFile ? [uploadFile] : []}
                  onChange={({ fileList }) =>
                    setUploadFile(fileList.length > 0 ? fileList[fileList.length - 1] : null)
                  }
                >
                  <Button size="small" icon={<UploadOutlined />}>
                    <Trans>Choose file</Trans>
                  </Button>
                </Upload>
              </Col>
              <Col>
                <Button
                  size="small"
                  danger
                  icon={<RollbackOutlined />}
                  disabled={!uploadFile || isRestoring}
                  loading={uploadRestoring}
                  onClick={() => {
                    if (uploadFile?.originFileObj) {
                      Modal.confirm({
                        title: t`Restore database`,
                        content: t`Replace all current data with the uploaded file? This cannot be undone.`,
                        okText: t`Restore`,
                        okButtonProps: { danger: true },
                        cancelText: t`Cancel`,
                        onOk: async () => {
                          setUploadRestoring(true);
                          try {
                            await RestoreDatabase(uploadFile.originFileObj as File);
                            messageApi.success(t`Database restored — reload to see changes`);
                            setUploadFile(null);
                          } catch (err) {
                            messageApi.error(t`Restore failed: ${String(err)}`);
                          } finally {
                            setUploadRestoring(false);
                          }
                        },
                      });
                    }
                  }}
                >
                  <Trans>Restore from file</Trans>
                </Button>
              </Col>
              <Col>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  <Trans>Upload a .db file to replace the current database.</Trans>
                </Text>
              </Col>
            </Row>
          </Space>
        </Card>

        {/* ── Backup history ── */}
        <Card
          title={<Trans>Backup history</Trans>}
          extra={
            <Button
              size="small"
              icon={<ReloadOutlined />}
              loading={loadingList}
              onClick={fetchList}
            >
              <Trans>Refresh</Trans>
            </Button>
          }
        >
          <Table<BackupEntry>
            dataSource={backups}
            rowKey="name"
            size="small"
            loading={loadingList}
            pagination={false}
          >
            <Table.Column<BackupEntry>
              title={<Trans>File</Trans>}
              dataIndex="name"
              key="name"
              render={(name) => <Text code style={{ fontSize: 12 }}>{name}</Text>}
            />
            <Table.Column<BackupEntry>
              title={<Trans>Size</Trans>}
              dataIndex="size"
              key="size"
              width={90}
              render={(size) => formatSize(size)}
            />
            <Table.Column<BackupEntry>
              title={<Trans>Date</Trans>}
              dataIndex="createdAt"
              key="createdAt"
              width={150}
              render={(v) => dayjs(v).format("DD/MM/YYYY HH:mm")}
            />
            <Table.Column<BackupEntry>
              title=""
              key="actions"
              width={110}
              render={(_, record) => (
                <Button
                  size="small"
                  danger
                  icon={<RollbackOutlined />}
                  loading={restoringName === record.name}
                  disabled={isRestoring && restoringName !== record.name}
                  onClick={() => confirmRestoreNamed(record.name)}
                >
                  <Trans>Restore</Trans>
                </Button>
              )}
            />
          </Table>
        </Card>

      </Space>
    </div>
  );
}

export default SettingsBackup;
