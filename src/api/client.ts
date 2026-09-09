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
    // T is only actually `void`/`undefined`-shaped here in practice — every
    // 204 response in this app comes from a DELETE endpoint called through
    // del(path) with no type argument (see the overload below). This cast
    // exists to satisfy the generic return type for the callers that DO ask
    // for real data (a 200 with a JSON body); it is not a claim that this
    // branch actually produces a T.
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

export const get = <T>(path: string) => request<T>("GET", path);
export const post = <T>(path: string, body: unknown) => request<T>("POST", path, body);
export const put = <T>(path: string, body: unknown) => request<T>("PUT", path, body);
export const patch = <T>(path: string, body: unknown) => request<T>("PATCH", path, body);

// del has two call shapes: most DELETE endpoints in this app respond 204 (no
// body) and should be called as del(path) — typed void so nothing downstream
// can accidentally treat the (nonexistent) response as data. A few endpoints
// instead respond 200 with a small JSON body (e.g. { deleted: boolean }) and
// must specify that shape explicitly via del<T>(path); those are the only
// case where request<T>'s 204 branch above would matter, and it's never hit
// for them since they never actually respond 204.
export function del(path: string): Promise<void>;
export function del<T>(path: string): Promise<T>;
export function del<T>(path: string): Promise<T> {
  return request<T>("DELETE", path);
}
