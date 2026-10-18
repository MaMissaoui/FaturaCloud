import { useEffect } from "react";
import { App } from "antd";
import { setThemedMessage } from "src/utils/message";

// antd's static message.* API can't read the ConfigProvider theme, so
// toasts render light-styled even in dark mode. App.useApp() is the fix,
// but most callers are Jotai atom write functions (plain functions, can't
// call hooks) — so this bridges the themed instance out to a module those
// atoms can import instead. See src/utils/message.ts.
//
// Mount it first inside <App>. Component tests mount it too: without it a
// toast goes to antd's static instance, a separate React root that test
// cleanup never unmounts, and its auto-close timer can then fire after
// jsdom is gone ("window is not defined" failing a green run).
export default function MessageBridge() {
  const { message } = App.useApp();
  useEffect(() => {
    setThemedMessage(message);
  }, [message]);
  return null;
}
