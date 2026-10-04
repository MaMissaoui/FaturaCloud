import { Tag } from "antd";
import { Trans } from "@lingui/react/macro";

// Marks a test/demo organization (organizations.isTest, migration 0099) so
// seeded or sandbox data is never mistaken for a real business — shown next
// to the header's organization switcher, in its dropdown and in the
// Organizations list.
export default function TestOrgTag() {
  return (
    <Tag color="orange" style={{ marginInlineEnd: 0, marginInlineStart: 6 }}>
      <Trans>Test</Trans>
    </Tag>
  );
}
