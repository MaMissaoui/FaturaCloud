import { useEffect, useRef } from "react";
import { useNavigate } from "react-router";
import { App } from "antd";
import { t } from "@lingui/core/macro";

// useUnsavedChangesWarning guards a dirty form against both browser-level
// and in-app navigation.
//
// beforeunload covers tab close / refresh / external links. The click
// capture below covers the in-app case the old beforeunload-only version
// missed: every sidebar and Settings entry in src/layouts/base.tsx is a
// react-router <Link> (an <a href>), and clicking one changes the route
// without ever firing beforeunload, silently discarding the edits. A
// capture-phase listener runs before react-router's own delegated onClick,
// so the navigation can be intercepted and confirmed first.
//
// Residual (documented, not fixed here): programmatic navigate() calls
// (e.g. a footer "Back to list" button) and browser back/forward still
// bypass this. Catching those properly needs a data router's useBlocker,
// which the app's declarative <BrowserRouter> cannot use without migrating
// the whole route tree in src/app.tsx.
const useUnsavedChangesWarning = (isDirty: boolean) => {
  const navigate = useNavigate();
  const { modal } = App.useApp();

  const dirtyRef = useRef(isDirty);
  dirtyRef.current = isDirty;

  useEffect(() => {
    if (!isDirty) return;
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", handler);
    return () => window.removeEventListener("beforeunload", handler);
  }, [isDirty]);

  useEffect(() => {
    const onClickCapture = (e: MouseEvent) => {
      if (!dirtyRef.current) return;
      // Only plain left-clicks on unmodified, same-origin, non-download
      // links are ours; modified clicks open new tabs and must pass through.
      if (e.defaultPrevented || e.button !== 0) return;
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;

      const target = e.target as HTMLElement | null;
      const anchor = target?.closest?.("a") as HTMLAnchorElement | null;
      if (!anchor) return;
      if (anchor.target && anchor.target !== "_self") return;
      if (anchor.hasAttribute("download")) return;

      const href = anchor.getAttribute("href");
      if (!href || href.startsWith("#")) return;

      const url = new URL(anchor.href, window.location.origin);
      if (url.origin !== window.location.origin) return;
      // Re-clicking the page you're already on isn't a discard.
      if (url.pathname === window.location.pathname && url.search === window.location.search)
        return;

      e.preventDefault();
      e.stopPropagation();

      const to = url.pathname + url.search + url.hash;
      modal.confirm({
        title: t`Discard unsaved changes?`,
        content: t`You have unsaved changes on this page. Leaving now will discard them.`,
        okText: t`Discard changes`,
        cancelText: t`Stay on this page`,
        okButtonProps: { danger: true },
        onOk: () => {
          // Clearing first stops the guard re-firing for the confirmed move.
          dirtyRef.current = false;
          navigate(to);
        },
      });
    };
    document.addEventListener("click", onClickCapture, true);
    return () => document.removeEventListener("click", onClickCapture, true);
  }, [modal, navigate]);
};

export default useUnsavedChangesWarning;
