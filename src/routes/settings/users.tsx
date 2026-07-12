import { useEffect, useState } from "react";
import {
  Button,
  Col,
  Drawer,
  Form,
  Input,
  message,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from "antd";
import { atom, useAtom, useAtomValue } from "jotai";
import { DeleteOutlined, TeamOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import { useLingui } from "@lingui/react";
import dayjs from "dayjs";

import {
  type UserRecord,
  CreateUser,
  DeleteUser,
  GetUser,
  ListUsers,
  UpdateUser,
} from "src/api";
import { currentUserAtom } from "src/atoms/auth";

const { Title } = Typography;

const searchAtom = atom("");
const drawerOpenAtom = atom(false);
const editingIdAtom = atom<string | null>(null);

export default function SettingsUsers() {
  useLingui();
  const [form] = Form.useForm();
  const me = useAtomValue(currentUserAtom);

  const [users, setUsers] = useState<UserRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  const [search, setSearch] = useAtom(searchAtom);
  const [drawerOpen, setDrawerOpen] = useAtom(drawerOpenAtom);
  const [editingId, setEditingId] = useAtom(editingIdAtom);

  const fetchUsers = async (q?: string) => {
    setLoading(true);
    try {
      setUsers(await ListUsers(q));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchUsers();
  }, []);

  const openNew = () => {
    setEditingId(null);
    form.resetFields();
    setDrawerOpen(true);
  };

  const openEdit = async (id: string) => {
    setEditingId(id);
    form.resetFields();
    setDrawerOpen(true);
    try {
      const user = await GetUser(id);
      form.setFieldsValue({ ...user, password: "", passwordConfirm: "" });
    } catch {}
  };

  const handleClose = () => {
    setDrawerOpen(false);
    setEditingId(null);
    form.resetFields();
  };

  const handleSubmit = async (values: any) => {
    setSubmitting(true);
    try {
      if (editingId) {
        const update: any = { displayName: values.displayName, role: values.role, isActive: values.isActive ? 1 : 0 };
        if (values.password) update.password = values.password;
        await UpdateUser(editingId, update);
      } else {
        await CreateUser({ email: values.email, password: values.password, displayName: values.displayName, role: values.role });
      }
      handleClose();
      fetchUsers(search);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`Failed to save user`);
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await DeleteUser(id);
      fetchUsers(search);
      return true;
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`Failed to delete user`);
      return false;
    }
  };

  const handleDeleteFromDrawer = async () => {
    if (!editingId) return;
    setSubmitting(true);
    try {
      const success = await handleDelete(editingId);
      if (success) handleClose();
    } finally {
      setSubmitting(false);
    }
  };

  const handleToggleActive = async (id: string, active: boolean) => {
    try {
      await UpdateUser(id, { isActive: active ? 1 : 0 });
    } catch (err) {
      message.error(err instanceof Error ? err.message : t`Failed to update user`);
    } finally {
      fetchUsers(search);
    }
  };

  const isEdit = !!editingId;

  return (
    <>
      <Row style={{ marginBottom: 16 }}>
        <Col flex="auto">
          <Title level={3} style={{ margin: 0 }}>
            <TeamOutlined style={{ marginRight: 8 }} />
            <Trans>Users</Trans>
          </Title>
        </Col>
        <Col>
          <Space>
            <Input.Search
              placeholder={t`Search`}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onSearch={(v) => fetchUsers(v)}
              allowClear
              onClear={() => { setSearch(""); fetchUsers(); }}
            />
            <Button type="primary" onClick={openNew}>
              <Trans>New user</Trans>
            </Button>
          </Space>
        </Col>
      </Row>

      <Table<UserRecord>
        dataSource={users}
        rowKey="id"
        loading={loading}
        pagination={false}
        size="middle"
        onRow={(record) => ({ onClick: () => openEdit(record.id), style: { cursor: "pointer" } })}
      >
        <Table.Column<UserRecord>
          title={<Trans>Name</Trans>}
          dataIndex="displayName"
          key="displayName"
          sorter={(a, b) => a.displayName.localeCompare(b.displayName)}
        />
        <Table.Column<UserRecord>
          title={<Trans>Email</Trans>}
          dataIndex="email"
          key="email"
          sorter={(a, b) => a.email.localeCompare(b.email)}
        />
        <Table.Column<UserRecord>
          title={<Trans>Role</Trans>}
          dataIndex="role"
          key="role"
          sorter={(a, b) => a.role.localeCompare(b.role)}
          render={(role) =>
            role === "admin"
              ? <Tag color="red"><Trans>Admin</Trans></Tag>
              : <Tag color="blue"><Trans>User</Trans></Tag>
          }
        />
        <Table.Column<UserRecord>
          title={<Trans>Active</Trans>}
          dataIndex="isActive"
          key="isActive"
          sorter={(a, b) => a.isActive - b.isActive}
          render={(v, record) => (
            <Switch
              checked={v === 1}
              size="small"
              disabled={record.id === me?.id}
              onClick={(_, e) => e.stopPropagation()}
              onChange={(checked) => handleToggleActive(record.id, checked)}
            />
          )}
        />
        <Table.Column<UserRecord>
          title={<Trans>Last login</Trans>}
          dataIndex="lastLoginAt"
          key="lastLoginAt"
          sorter={(a, b) => (a.lastLoginAt ?? 0) - (b.lastLoginAt ?? 0)}
          render={(v) => (v ? dayjs(v).format("DD/MM/YYYY HH:mm") : "—")}
        />
        <Table.Column<UserRecord>
          title=""
          key="actions"
          width={80}
          render={(_, record) => (
            <Popconfirm
              title={t`Delete this user?`}
              onConfirm={(e) => { e?.stopPropagation(); handleDelete(record.id); }}
              onCancel={(e) => e?.stopPropagation()}
              disabled={record.id === me?.id}
            >
              <Button
                size="small"
                danger
                disabled={record.id === me?.id}
                onClick={(e) => e.stopPropagation()}
              >
                <Trans>Delete</Trans>
              </Button>
            </Popconfirm>
          )}
        />
      </Table>

      <Drawer
        title={isEdit ? <Trans>Edit user</Trans> : <Trans>New user</Trans>}
        open={drawerOpen}
        placement="right"
        width={480}
        onClose={handleClose}
        footer={
          <div style={{ display: "flex", justifyContent: "space-between" }}>
            <div>
              {isEdit && editingId !== me?.id && (
                <Popconfirm
                  title={t`Delete this user?`}
                  onConfirm={handleDeleteFromDrawer}
                  okText={<Trans>Yes</Trans>}
                  cancelText={<Trans>No</Trans>}
                  placement="topRight"
                >
                  <Button danger icon={<DeleteOutlined />} loading={submitting}>
                    <Trans>Delete</Trans>
                  </Button>
                </Popconfirm>
              )}
            </div>
            <Space>
              <Button onClick={handleClose}><Trans>Cancel</Trans></Button>
              <Button type="primary" loading={submitting} onClick={() => form.submit()}>
                <Trans>Save</Trans>
              </Button>
            </Space>
          </div>
        }
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Row gutter={[16, 0]}>
            <Col xs={24} md={isEdit ? 16 : 24}>
              <Form.Item name="displayName" label={<Trans>Name</Trans>} rules={[{ required: true }]}>
                <Input />
              </Form.Item>
            </Col>
            {isEdit && (
              <Col xs={24} md={8}>
                <Form.Item
                  name="isActive"
                  label={<Trans>Active</Trans>}
                  valuePropName="checked"
                  getValueFromEvent={(v) => v}
                  getValueProps={(v) => ({ checked: v === 1 || v === true })}
                >
                  <Switch disabled={editingId === me?.id} />
                </Form.Item>
              </Col>
            )}
            <Col xs={24}>
              <Form.Item
                name="email"
                label={<Trans>Email</Trans>}
                rules={isEdit ? [] : [{ required: true, type: "email" as const }]}
              >
                <Input disabled={isEdit} />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item
                name="role"
                label={<Trans>Role</Trans>}
                initialValue={isEdit ? undefined : "user"}
              >
                <Select
                  options={[
                    { label: t`Admin`, value: "admin" },
                    { label: t`User`, value: "user" },
                  ]}
                />
              </Form.Item>
            </Col>
          </Row>

          <Form.Item
            name="password"
            label={isEdit ? <Trans>New password (leave blank to keep current)</Trans> : <Trans>Password</Trans>}
            rules={isEdit
              ? [{ min: 6, message: t`Minimum 6 characters` }]
              : [{ required: true, min: 6, message: t`Minimum 6 characters` }]
            }
          >
            <Input.Password />
          </Form.Item>

          {isEdit && (
            <Form.Item
              name="passwordConfirm"
              label={<Trans>Confirm password</Trans>}
              dependencies={["password"]}
              rules={[
                ({ getFieldValue }) => ({
                  validator(_, value) {
                    const pw = getFieldValue("password");
                    if (!pw && !value) return Promise.resolve();
                    if (pw === value) return Promise.resolve();
                    return Promise.reject(new Error(t`Passwords do not match`));
                  },
                }),
              ]}
            >
              <Input.Password />
            </Form.Item>
          )}
        </Form>
      </Drawer>
    </>
  );
}
