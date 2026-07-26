import { message as staticMessage } from "antd";
import type { MessageInstance } from "antd/es/message/interface";

// antd's static `message.*` API can't consume the ConfigProvider theme
// context, so toasts always render light-styled even in dark mode. The fix
// is antd's own <App> + App.useApp() — but ~100 call sites live in Jotai
// atom write functions, which are plain functions and can't call hooks.
// This module is the bridge: <MessageBridge> (mounted once near the app
// root) hands the themed instance here via setThemedMessage, and atoms
// import `message` from this module instead of "antd" directly.
let themedMessage: MessageInstance | null = null;

export function setThemedMessage(instance: MessageInstance) {
  themedMessage = instance;
}

// Falls back to antd's static (unthemed) instance only in the brief window
// before <MessageBridge> has mounted — in practice never observable, since
// nothing can trigger a toast before the app has rendered.
export const message: MessageInstance = new Proxy({} as MessageInstance, {
  get(_target, prop: keyof MessageInstance) {
    return (themedMessage ?? staticMessage)[prop];
  },
});
