// When a session ends (a 401), the page the user was on and the organization
// they had open are remembered for this tab, and restored after they sign in
// again — through the password form or single sign-on, which both land on
// "/" (audit F175). The organization is restored only if whoever signs in is
// a member of it: the next person at a shared counter may be someone else.

const RESUME_KEY = "fc_resume";
// A resume older than this is ignored: the user has moved on.
const RESUME_MAX_AGE_MS = 60 * 60 * 1000;

export type SessionResume = { path: string; organizationId: string | null };

// safeResumePath accepts only a same-origin path ("/invoices/abc?x=1"), never
// "//host" or a scheme, so a remembered path can't redirect anywhere else.
export function safeResumePath(path: unknown): string | null {
  if (typeof path !== "string") return null;
  if (!path.startsWith("/") || path.startsWith("//") || path.includes("\\")) return null;
  if (path === "/" || path === "/login" || path.startsWith("/login?") || path.startsWith("/login/"))
    return null;
  return path;
}

export function rememberResume(path: string, organizationId: string | null) {
  const safe = safeResumePath(path);
  if (!safe) return;
  try {
    sessionStorage.setItem(
      RESUME_KEY,
      JSON.stringify({ path: safe, organizationId, at: Date.now() }),
    );
  } catch {
    // storage unavailable — the user just lands on their home page
  }
}

// takeResume returns the remembered page once and forgets it.
export function takeResume(): SessionResume | null {
  let raw: string | null = null;
  try {
    raw = sessionStorage.getItem(RESUME_KEY);
    sessionStorage.removeItem(RESUME_KEY);
  } catch {
    return null;
  }
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    const path = safeResumePath(parsed?.path);
    if (!path || typeof parsed.at !== "number" || Date.now() - parsed.at > RESUME_MAX_AGE_MS) {
      return null;
    }
    const organizationId = typeof parsed.organizationId === "string" ? parsed.organizationId : null;
    return { path, organizationId };
  } catch {
    return null;
  }
}
