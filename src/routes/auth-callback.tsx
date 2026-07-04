import { useEffect } from "react";

import { setToken } from "src/api/client";
import Loading from "src/components/loading";

// Landing point for the OIDC SSO redirect (see api/oidc.go's oidcCallback).
// The JWT rides in the URL fragment, not a query string, so it's never sent
// to the server in an access log or a Referer header — read it client-side
// only, store it exactly like a local-login token, then hand off to the app.
//
// A full page navigation (not react-router's navigate) is deliberate: it
// remounts the whole app so the existing mount-time "load current user from
// token" effect in app.tsx runs fresh and picks up the freshly-stored token.
// A client-side navigate could race that effect (which only runs once, on
// mount) and leave the app looking logged out despite having a valid token.
export default function AuthCallback() {
  useEffect(() => {
    const match = window.location.hash.match(/token=([^&]+)/);
    if (match) {
      setToken(decodeURIComponent(match[1]));
    }
    window.location.href = "/";
  }, []);

  return <Loading />;
}
