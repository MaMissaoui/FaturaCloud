import { Button, Card, Input, Popconfirm, Select, Space, Table, Tag } from "antd";
import { DeleteOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import type { OrganizationMember } from "src/api";

export interface OrganizationMembersPanelProps {
  members: OrganizationMember[];
  membersLoading: boolean;
  newMemberEmail: string;
  newMemberRole: "admin" | "user";
  addingMember: boolean;
  memberActionId: string | null;
  onNewMemberEmailChange: (value: string) => void;
  onNewMemberRoleChange: (value: "admin" | "user") => void;
  onAddMember: () => void;
  onMemberRoleChange: (userId: string, role: "admin" | "user") => void;
  onRemoveMember: (userId: string) => void;
}

// Purely presentational — every fetch/handler lives in the parent page
// (src/routes/organizations/index.tsx). Extracted from that file (F151,
// 2026-09-09) to cut its size, not to change when or how data loads.
export default function OrganizationMembersPanel({
  members,
  membersLoading,
  newMemberEmail,
  newMemberRole,
  addingMember,
  memberActionId,
  onNewMemberEmailChange,
  onNewMemberRoleChange,
  onAddMember,
  onMemberRoleChange,
  onRemoveMember,
}: OrganizationMembersPanelProps) {
  return (
    <Card size="small" title={<Trans>Members</Trans>} style={{ marginTop: 12 }}>
      <Space direction="vertical" size={8} style={{ width: "100%" }}>
        <Table
          size="small"
          loading={membersLoading}
          dataSource={members}
          rowKey="userId"
          pagination={false}
        >
          <Table.Column
            title={<Trans>User</Trans>}
            key="user"
            render={(_: unknown, record: OrganizationMember) => (
              <>
                {record.displayName || record.email}
                {!record.isActive && (
                  <Tag color="default" style={{ marginLeft: 8 }}>
                    <Trans>Inactive</Trans>
                  </Tag>
                )}
              </>
            )}
          />
          <Table.Column
            title={<Trans>Role</Trans>}
            key="role"
            width={140}
            render={(_: unknown, record: OrganizationMember) => (
              <Select
                size="small"
                value={record.role}
                style={{ width: 110 }}
                disabled={memberActionId === record.userId}
                onChange={(role) => onMemberRoleChange(record.userId, role)}
                options={[
                  { value: "admin", label: t`Admin` },
                  { value: "user", label: t`User` },
                ]}
              />
            )}
          />
          <Table.Column
            title=""
            key="actions"
            width={60}
            render={(_: unknown, record: OrganizationMember) => (
              <Popconfirm
                title={t`Remove this member?`}
                onConfirm={() => onRemoveMember(record.userId)}
              >
                <Button
                  size="small"
                  danger
                  type="text"
                  icon={<DeleteOutlined />}
                  loading={memberActionId === record.userId}
                />
              </Popconfirm>
            )}
          />
        </Table>
        <Space.Compact style={{ width: "100%" }}>
          <Input
            placeholder={t`Email of an existing user`}
            value={newMemberEmail}
            onChange={(e) => onNewMemberEmailChange(e.target.value)}
            onPressEnter={onAddMember}
          />
          <Select
            value={newMemberRole}
            style={{ width: 110 }}
            onChange={onNewMemberRoleChange}
            options={[
              { value: "admin", label: t`Admin` },
              { value: "user", label: t`User` },
            ]}
          />
          <Button
            type="primary"
            loading={addingMember}
            disabled={!newMemberEmail.trim()}
            onClick={onAddMember}
          >
            <Trans>Add</Trans>
          </Button>
        </Space.Compact>
      </Space>
    </Card>
  );
}
