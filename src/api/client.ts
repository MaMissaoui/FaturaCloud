const BASE = "/api";

// The session token lives in an httpOnly cookie the browser sends automatically
// (see api/auth.go), so there's nothing for JavaScript to read or store. Because
// the cookie travels on same-origin requests on its own, we only need to (a) opt
// into sending cookies and (b) attach a custom header the server requires on
// state-changing requests — cross-site pages can't set custom headers without a
// CORS preflight this server never grants, which is what blocks CSRF.
export const CSRF_HEADER = "X-CSRF-Protection";

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { [CSRF_HEADER]: "1" };
  if (body !== undefined) headers["Content-Type"] = "application/json";

  const res = await fetch(`${BASE}${path}`, {
    method,
    headers,
    credentials: "same-origin",
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  if (res.status === 401) {
    if (window.location.pathname !== "/login") {
      window.location.href = "/login";
    }
    throw new Error("Session expired");
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error ?? res.statusText);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

export const get = <T>(path: string) => request<T>("GET", path);
export const post = <T>(path: string, body: unknown) => request<T>("POST", path, body);
export const put = <T>(path: string, body: unknown) => request<T>("PUT", path, body);
export const patch = <T>(path: string, body: unknown) => request<T>("PATCH", path, body);
export const del = <T>(path: string) => request<T>("DELETE", path);
