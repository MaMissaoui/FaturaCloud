# Logging

FaturaCloud writes one log stream: the container's standard error, which
Docker keeps on the host. This page explains what goes into it and how to
read it on the production Pi.

## What is logged

**One line per API request** (`msg=request`), written when the response is
done. Static files (the app's JavaScript, CSS, images) are not logged.

| Field | Meaning |
|---|---|
| `route` | The route that matched, as registered: `PATCH /api/invoices/{id}/state`, never the real id or the query string |
| `status`, `ms`, `bytes` | Response status, time taken, response size |
| `request_id` | A random id, also sent back as the `X-Request-ID` header |
| `ip` | The client's address (through the proxy when `TRUSTED_PROXIES` is set) |
| `user`, `user_id` | Who made the request, when signed in |
| `org`, `role` | The organization the request was for and the caller's role in it |
| `err`, `stack` | The real error behind a 500, and the stack of a crash |
| `event` | Set on the lines below that are more than an ordinary request |

**The level says how much it matters:**

| Level | Lines |
|---|---|
| `ERROR` | Every 500 (with `err`), every crash (with `stack`), every browser error report |
| `WARN` | 403 (a role refused) and 429 (rate limited); `event=login_failed`, `login_rate_limited`, `sso_rejected`; an export template with unknown placeholders |
| `INFO` | Every change (POST, PUT, PATCH, DELETE), every other 4xx, `event=login` and `sso_login`, startup |
| `DEBUG` | Successful reads (GET). Off by default |

**Never logged:** request or response bodies, query strings (the Cash Book
search sends names, phones and ID numbers), cookies, headers, passwords.

**A 500 carries a reference.** The error the user sees reads
`internal error (ref 3f9c2a1b7d4e)`. That reference is the `request_id`
of the matching log line.

**Browser errors.** When the app crashes in someone's browser (an uncaught
error, an unhandled promise rejection, a React render error), the browser
reports it to `POST /api/client-errors`. It is logged as an `ERROR` line
with `event=client_error` and these fields:

| Field | Meaning |
|---|---|
| `error` | The error's name and message |
| `page` | The page's path (no query string) |
| `source` | `window.error`, `unhandledrejection` or `react` |
| `release` | The app version the browser was running (an older one means a tab opened before a deploy) |
| `browser` | The browser's user agent |
| `stack`, `component_stack` | Where it happened |

A page reports the same error once, at most 10 per page load, and each user
at most 20 per 10 minutes. Nothing is stored in the database.

## Reading the logs on the Pi

The container is `fatura-cloud` on `pimi-01`. Connect over SSH, on the home
network or over Tailscale:

```sh
ssh mam@192.168.121.9               # home network
ssh mam@pimi-01.tailc89738.ts.net   # Tailscale
```

Then use `docker logs`. The app writes to standard error, so add `2>&1`
before piping into `grep`:

```sh
# The last hour
docker logs --since 1h fatura-cloud

# Follow live (Ctrl-C to stop)
docker logs -f fatura-cloud

# Everything that went wrong today: server errors, crashes, browser errors
docker logs --since 24h fatura-cloud 2>&1 | grep 'level=ERROR'

# Only browser errors
docker logs --since 7d fatura-cloud 2>&1 | grep 'event=client_error'

# The line behind a reference a user quoted ("ref 3f9c2a1b7d4e")
docker logs fatura-cloud 2>&1 | grep 'request_id=3f9c2a1b7d4e'

# Sign-in trouble and refused access
docker logs --since 7d fatura-cloud 2>&1 | grep -E 'event=(login_failed|login_rate_limited|sso_rejected)|status=403'

# Everything one person changed today
docker logs --since 24h fatura-cloud 2>&1 | grep 'level=INFO' | grep 'user=someone@example.com'
```

You can do the same without SSH from a laptop that has SSH access, e.g.
`ssh mam@192.168.121.9 "docker logs --since 24h fatura-cloud 2>&1 | grep event=client_error"`.

Portainer shows the same stream: Containers ▸ `fatura-cloud` ▸ Logs, with
its own search box.

### Reading a line

```
time=2026-10-06T09:14:03.512Z level=ERROR msg=request route="POST /api/client-errors"
  status=204 ms=1 request_id=8d1e0c2f9a77 ip=192.168.121.20 user=caissier@example.com
  user_id=… org=… role=cashbook event=client_error error="TypeError: x is undefined"
  source=react page=/cash-book release=v3.65.0 browser="Mozilla/5.0 …" stack="…"
```

(One line in the real output, wrapped here.) For a browser error, the
`status` is the report's own (204); `error`, `page` and `stack` describe the
crash.

## How long logs are kept

Docker keeps three files of 10 MB each per container (set in the homelab
compose file), so roughly the last 100,000–150,000 lines at the default level.
Older lines are gone: a log line is a diagnostic, not a record of who
changed what. That record is the activity history below.

## Activity history

Every change made through the app (creating, editing, deleting, changing a
state, recording a payment, importing a sheet…) is also stored in the
database, kept for two years. Open it from the Settings gear ▸ **Activity**:

- **Organization admins** see their organization's history: when, who, what
  was done to which document, and for a state change the state before and
  after. Filter by member and by dates; a document with its own page opens
  from its row.
- **What changed.** For an edit or an action on an existing document, the
  **Change** column lists the fields it changed. Expand the row (**+**) to see
  each field's value before and after.
  - Bank accounts (IBAN) are masked to their first two characters and last
    four digits. Passwords, hashes and tokens are never recorded.
  - References show the name they had at the time: a client, an account, a
    tax rate.
  - Creates and deletions list no fields, and neither do rows recorded before
    this was added. Invoice, order and delivery lines aren't compared, but
    the document's totals are.
- **Platform admins** also get a **Platform** view: users, backups and
  database restores, which belong to no organization.

The history records the change someone made, not its side effects: a payment
that marks an invoice Paid appears as the payment. Each row carries the
request id (the **Ref.** column), so the matching log line can be found with
`grep request_id=<id>` while the log still has it.

## Settings

Both are environment variables of the container. The homelab compose file
(`apps/fatura-cloud/docker-compose.yml` in homelab-deploy) passes them
through, and its `.env` template sets `LOG_LEVEL` (info unless overridden);
`LOG_FORMAT` defaults to text.

| Variable | Values | Default |
|---|---|---|
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `LOG_FORMAT` | `text`, `json` | `text` |

To see every request for a while (e.g. while chasing a bug), deploy once
with debug, from homelab-deploy's `ansible/` folder:

```sh
ansible-playbook playbooks/deploy.yml -e target_app=fatura-cloud -e fatura_cloud_log_level=debug
```

The next deploy without the flag puts it back to info. Debug fills the 30 MB
much faster, so don't leave it on.

`LOG_FORMAT=json` writes one JSON object per line, for `jq`:

```sh
docker logs --since 24h fatura-cloud 2>&1 | jq -c 'select(.event == "client_error") | {time, user, page, error}'
```
