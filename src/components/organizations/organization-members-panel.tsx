import { Button, Card, Input, Popconfirm, Select, Space, Table, Tag } from "antd";
import { DeleteOutlined } from "@ant-design/icons";
import { Trans } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import type { OrganizationMember, OrganizationRole } from "src/api";

// Shared by both the member-list role Select and the "add member" Select
// below — one place naming the six roles and their labels.
const roleOptions = (): { value: OrganizationRole; label: string }[] => [
  { value: "admin", label: t`Admin` },
  { value: "general", label: t`General` },
  { value: "sales", label: t`Sales` },
  { value: "purchasing", label: t`Purchasing` },
  { value: "accounting", label: t`Accounting` },
  { value: "cashbook", label: t`Cash Book` },
];

export interface OrganizationMembersPanelProps {
  members: OrganizationMember[];
  membersLoading: boolean;
  newMemberEmail: string;
  newMemberRole: OrganizationRole;
  addingMember: boolean;
  memberActionId: string | null;
  onNewMemberEmailChange: (value: string) => void;
  onNewMemberRoleChange: (value: OrganizationRole) => void;
  onAddMember: () => void;
  onMemberRoleChange: (userId: string, role: OrganizationRole) => void;
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
            width={160}
            render={(_: unknown, record: OrganizationMember) => (
              <Select
                size="small"
                value={record.role}
                style={{ width: 140 }}
                disabled={memberActionId === record.userId}
                onChange={(role) => onMemberRoleChange(record.userId, role)}
                options={roleOptions()}
              />
            )}
          />
          <Table.Column
            title=""
            key="actions"
            width={60}
            align="center"
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
                  aria-label={t`Remove member`}
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
            style={{ width: 140 }}
            onChange={onNewMemberRoleChange}
            options={roleOptions()}
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
