import { CSRF_HEADER } from "src/api/client";

// Browser error reporting without Sentry (which the published image ships
// without): an uncaught error or unhandled promise rejection is posted to
// POST /api/client-errors and lands in the server log as an error-level line
// with event=client_error (see docs/logging.md for how to read it). Reports
// are best-effort and fire-and-forget — the same error is sent once per page
// load, at most MAX_REPORTS per page load, and a failed report (signed out,
// offline) is dropped silently.

const MAX_REPORTS = 10;

// Noise browsers raise that isn't a bug in the app: a cross-origin script's
// opaque error, ResizeObserver's benign loop notice, and the 401 redirect's
// own "Session expired" rejection (src/api/client.ts).
const IGNORED = [/^Script error\.?$/, /ResizeObserver loop/, /^Session expired$/];

const reported = new Set<string>();

function asError(value: unknown): Error {
  if (value instanceof Error) return value;
  if (typeof value === "string") return new Error(value);
  try {
    return new Error(JSON.stringify(value));
  } catch {
    return new Error(String(value));
  }
}

export function reportClientError(value: unknown, source: string, componentStack?: string) {
  const error = asError(value);
  const message = error.message || String(value);
  if (IGNORED.some((pattern) => pattern.test(message))) return;
  const key = `${error.name}:${message}`;
  if (reported.has(key) || reported.size >= MAX_REPORTS) return;
  reported.add(key);

  void fetch("/api/client-errors", {
    method: "POST",
    headers: { "Content-Type": "application/json", [CSRF_HEADER]: "1" },
    credentials: "same-origin",
    keepalive: true,
    body: JSON.stringify({
      // Capped like the server caps them: a keepalive request over 64 KB
      // is refused outright, which would drop the report.
      name: error.name.slice(0, 100),
      message: message.slice(0, 500),
      stack: (error.stack ?? "").slice(0, 4000),
      componentStack: (componentStack ?? "").slice(0, 2000),
      source,
      url: window.location.href,
      release: __APP_VERSION__,
    }),
  }).catch(() => {
    // offline or signed out: nothing to do
  });
}

// installClientErrorReporting listens for errors nothing else caught. React
// render errors arrive through createRoot's onUncaughtError (src/main.tsx).
export function installClientErrorReporting() {
  window.addEventListener("error", (event) => {
    reportClientError(event.error ?? event.message, "window.error");
  });
  window.addEventListener("unhandledrejection", (event) => {
    reportClientError(event.reason, "unhandledrejection");
  });
}
