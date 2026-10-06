// Type declaration for Promise.withResolvers
declare global {
  interface PromiseConstructor {
    withResolvers<T>(): {
      promise: Promise<T>;
      resolve: (value: T | PromiseLike<T>) => void;
      reject: (reason?: any) => void;
    };
  }
}

// Polyfill for Promise.withResolvers for older JavaScript environments
if (!Promise.withResolvers) {
  Promise.withResolvers = function <T>() {
    let resolve: (value: T | PromiseLike<T>) => void;
    let reject: (reason?: any) => void;
    const promise = new Promise<T>((res, rej) => {
      resolve = res;
      reject = rej;
    });
    return { promise, resolve: resolve!, reject: reject! };
  };
}

import React from "react";
import ReactDOM from "react-dom/client";

import App from "src/app";
import { installClientErrorReporting, reportClientError } from "src/utils/client-errors";

// A deploy changes every build's hashed chunk filenames. A tab whose JS was
// already loaded before that deploy still has old chunk URLs baked into its
// closures — any dynamic import() from that point on (a lazy route in
// app.tsx, a locale .po file in src/utils/lingui.tsx's dynamicActivate) 404s
// against the new server, with no way to recover in place. Vite fires this
// event specifically for that case; reloading once picks up the current
// build. Registered before the first render so it's active immediately,
// not just once some route happens to lazy-load.
window.addEventListener("vite:preloadError", (event) => {
  event.preventDefault();
  window.location.reload();
});

installClientErrorReporting();

ReactDOM.createRoot(document.getElementById("root")!, {
  // A render error no error boundary caught: report it with the component
  // stack, then log it as React would by default.
  onUncaughtError: (error, errorInfo) => {
    reportClientError(error, "react", errorInfo.componentStack);
    console.error(error);
  },
}).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
