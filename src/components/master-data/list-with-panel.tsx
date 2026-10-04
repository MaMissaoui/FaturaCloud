import type { ReactNode } from "react";
import { Drawer, Grid, theme } from "antd";

// ListWithPanel lays a master-data list beside its summary panel: list on the
// left, panel on the right (sticky, so it stays in view while the list
// scrolls). Below the md breakpoint there's no room for both, so the panel
// opens in a full-width drawer instead, and only while a record is picked.
export default function ListWithPanel({
  list,
  panel,
  panelOpen,
  onClosePanel,
  panelTitle,
}: {
  list: ReactNode;
  panel: ReactNode;
  // Whether a record is picked (on a phone, whether the drawer is open).
  panelOpen: boolean;
  onClosePanel: () => void;
  // The drawer's title on a phone.
  panelTitle?: ReactNode;
}) {
  const { token } = theme.useToken();
  const screens = Grid.useBreakpoint();
  const wide = !!screens.md;

  if (!wide) {
    return (
      <>
        {list}
        <Drawer
          open={panelOpen}
          onClose={onClosePanel}
          placement="right"
          size="100%"
          title={panelTitle}
          destroyOnHidden
        >
          {panel}
        </Drawer>
      </>
    );
  }

  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
      <div style={{ flex: "1 1 560px", minWidth: 0 }}>{list}</div>
      <aside
        style={{
          flex: "0 1 380px",
          minWidth: 0,
          position: "sticky",
          top: 16,
          border: `1px solid ${token.colorBorderSecondary}`,
          borderRadius: 8,
          padding: 20,
          boxSizing: "border-box",
        }}
      >
        {panel}
      </aside>
    </div>
  );
}
