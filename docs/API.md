# go-rnd API

HTTP API of the R&D base: local accounts (bcrypt passwords, ES256 JWTs with rotating refresh
tokens) and `GET /api/me`. The whole API speaks **JSON:API 1.1** — request documents, content
negotiation, query parameters and errors. Experiments add their own endpoints here.

The examples under *Response conventions* use a hypothetical `notes` resource to show the generic
shapes (relationships, `included`, pagination links); no such endpoint ships.

- **Base path:** `/api` (plus the unauthenticated `GET /healthz`)
- **Host:** value of `SERVER_ADDRESS` (e.g. `http://localhost:8080`)
- **Media type:** `application/vnd.api+json` for every request body and every response — see
  [Content negotiation](#content-negotiation).
- **Request id:** every response carries an `X-Request-ID` header — see [Request ids](#request-ids).

---

## Endpoints

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| `GET` | `/healthz` | — | Liveness probe + build version |
| `POST` | `/api/auth/register` | — | [Create an account](#post-apiauthregister) (role `user`) |
| `POST` | `/api/auth/login` | — | [Log in](#post-apiauthlogin): sets the auth cookies, returns the tokens |
| `POST` | `/api/auth/refresh` | refresh token | [Rotate the token pair](#post-apiauthrefresh) |
| `POST` | `/api/auth/logout` | refresh token | [Log out](#post-apiauthlogout): revoke the session, clear the cookies |
| `GET` | `/api/me` | user | [The caller's own account](#get-apime) |

---

## Running the `curl` examples

Every endpoint below has a copy-pasteable `curl`. They use three shell variables — set them once:

```bash
export BASE="http://localhost:8080"          # or your SERVER_ADDRESS
export CT="Content-Type: application/vnd.api+json"
export TOKEN="<meta.access_token from POST /api/auth/login>"   # no "Bearer " prefix
```

Instead of `-H "Authorization: Bearer $TOKEN"` a browser-like client may keep the cookies:
`curl -c jar.txt -b jar.txt …` on login, then `-b jar.txt` everywhere else.

---

## Content negotiation

Checked first on every `/api` route, before authentication.

| Rule | Otherwise |
| --- | --- |
| A request **with a body** sends `Content-Type: application/vnd.api+json`, with no media type parameter except `profile` | **415** `unsupported_media_type` |
| `Accept`, when it lists `application/vnd.api+json`, must list it at least once **without** parameters other than `profile` (`Accept: */*`, `application/json` or no `Accept` at all are fine) | **406** `not_acceptable` |
| No JSON:API extensions (`ext=…`) are supported | 415 / 406 |

Every response carries `Vary: Accept`. `/healthz` sits outside `/api` and skips these checks.

```json
{
  "jsonapi": { "version": "1.1" },
  "errors": [
    {
      "status": "415", "code": "unsupported_media_type", "title": "Unsupported Media Type",
      "detail": "Send the request body as application/vnd.api+json.",
      "source": { "header": "Content-Type" }
    }
  ]
}
```

---

## Authentication

Every endpoint except `/healthz` and `/api/auth/*` sits behind the access-token middleware. The
access token comes from [logging in](#post-apiauthlogin) (or [refreshing](#post-apiauthrefresh))
and is sent one of two ways:

| Method | How |
| --- | --- |
| Cookie | `app-access-token=<jwt>` — set by login / refresh, a browser sends it on its own |
| Header | `Authorization: Bearer <jwt>` |

The cookie is checked first; when it is absent the `Authorization` header is used.

The token is verified (ES256 signature, `iss`, `aud` = `CLIENT_ID`, `exp`, `typ` = `access`), then
the user is **re-read from the database on every request**: a user deactivated (or deleted) after
the token was issued gets **401** on the very next call, not when the token expires.

| Failure | Status | `detail` |
| --- | --- | --- |
| No token, bad signature, expired, a refresh token used as an access token | 401 | `You're not authorized to perform this action.` |
| The user is inactive / gone | 401 | `Your account is no longer active.` |

→ on a `401` *"not authorized"*, [refresh](#post-apiauthrefresh) once and retry the call. On
*"no longer active"*, drop the local state and go to the login screen.

### Who is the caller?

The access token's payload (decode the JWT; the server verifies it, the client doesn't need to):

| Claim | Type | Use it for |
| --- | --- | --- |
| `sub` | number | the caller's **user id** |
| `name` / `email` | string | display |
| `role` | string | `user` / `admin` ([Enums](#enums)) — **a hint only**: a snapshot from when the token was issued. `RequireRole` authorizes on it, but a refresh re-reads the role, so drive role-based UI from `role` in [`GET /api/me`](#get-apime) (live) |
| `typ` | string | always `access` (a refresh token says `refresh`) |
| `exp` / `iat` | number | unix seconds; after `exp` every call is `401` → refresh |
| `iss` / `aud` / `jti` | string | issuer, `CLIENT_ID`, token id |

Ids in **responses are strings** (`"id": "42"`) while `sub` is a number — compare with `String(sub)`.

### How a session works

| Piece | Where it lives | Lifetime |
| --- | --- | --- |
| Access token (ES256 JWT) | cookie `app-access-token` (path `/`), also `meta.access_token` in the login / refresh body | `ACCESS_TOKEN_DURATION` (e.g. 15 min) |
| Refresh token (ES256 JWT, rotating) | cookie `app-refresh-token` (path `/api/auth`), also `meta.refresh_token` in the login / refresh body | `REFRESH_TOKEN_DURATION` **from its own issue** (default 7 days); on an untrusted device never past the session's end |

Both cookies are **HttpOnly**, `SameSite=Lax`, `Secure` unless `COOKIE_SECURE=false` (plain-HTTP
dev), scoped to `COOKIE_DOMAIN` when set. A browser client never reads them: it calls the API with
`credentials: "include"`.

- **Every login starts a session** (a refresh-token *family*): one per device / browser.
- **Refresh rotates**: each `POST /api/auth/refresh` spends the presented refresh token and returns a
  brand-new pair. The new refresh token lives `REFRESH_TOKEN_DURATION` from *now*, so it is an
  **idle timeout** — a user who keeps coming back is never logged out (on a trusted device, below);
  one who's away longer logs in again.
- **Trusted vs untrusted device**: a login with `"trust_device": true` slides as above with no cap.
  Without it (the default) the session ends `UNTRUSTED_SESSION_DURATION` (default 7 days) after the
  login, however often it refreshes: the last refresh token expires exactly then, and a refresh
  after it is a `401`.
- **Reuse detection**: presenting an **already-spent** refresh token revokes the **whole session**
  (it was probably stolen) → `401`, log in again. Exception: within **10 s** of its rotation it's
  treated as two tabs refreshing at once — `401`, but the session survives (the other tab already
  holds the new pair). **Single-flight** your refresh calls anyway.
- **Logout** revokes the session's refresh tokens. The **access token stays valid until it expires**
  (it's a stateless JWT) — that's why `ACCESS_TOKEN_DURATION` is short.

---

## Login, registration and sessions

All `/api/auth/*` endpoints are **public** (no access token).

### `POST /api/auth/register`

Creates an account. The role is always `user` — there is no way to register an admin.
**No `Location`** header: a plain user has no URL of its own (use [`GET /api/me`](#get-apime)).

**Request document** — `type` `users`, no `id`:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `name` | string | yes | not blank, ≤ 255; trimmed |
| `email` | string | yes | valid email, ≤ 255; trimmed; **unique ignoring case** |
| `password` | string | yes | 8–72 characters (72 = bcrypt's input limit) |

Any other attribute (`role`, `is_active`, …) is a **403**.

```bash
curl -X POST "$BASE/api/auth/register" -H "$CT" -d '{
  "data": {
    "type": "users",
    "attributes": { "name": "Jane Doe", "email": "jane@example.com", "password": "Passw0rd!" }
  }
}'
```

**Response — `201 Created`**

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "users", "id": "1",
    "attributes": { "email": "jane@example.com", "name": "Jane Doe", "role": "user" }
  },
  "links": { "self": "/api/auth/register" }
}
```

Supports `fields[users]`.

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 409 | `conflict` | `That email is already registered.` (any case: `JANE@example.com` = `jane@example.com`) |
| 403 | `forbidden` | an attribute that isn't writable — `"role" can't be set on this endpoint.` |
| 422 | `unprocessable_entity` | a field failed validation — one error per field (see [Validation error](#validation-error)) |
| 500 | `internal_error` | hashing / DB failure |

### `POST /api/auth/login`

**Request document** — `type` `sessions`:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `email` | string | yes | valid email, ≤ 255 |
| `password` | string | yes | ≤ 72 |
| `trust_device` | boolean | no (default `false`) | `true` ("remember this device"): the session slides with no cap; otherwise it ends `UNTRUSTED_SESSION_DURATION` after the login |

```bash
curl -X POST "$BASE/api/auth/login" -H "$CT" -c jar.txt -d '{
  "data": {
    "type": "sessions",
    "attributes": { "email": "jane@example.com", "password": "Passw0rd!" }
  }
}'
```

**Response — `200 OK`** — sets both cookies, answers with the **account** as `data` and the
**tokens** under `meta` (for clients that don't use cookies: mobile, server-to-server):

```
Set-Cookie: app-access-token=eyJhbGciOi…; Path=/; Max-Age=899; HttpOnly; SameSite=Lax
Set-Cookie: app-refresh-token=eyJhbGciOi…; Path=/api/auth; Max-Age=604799; HttpOnly; SameSite=Lax
```

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "users", "id": "1",
    "attributes": { "email": "jane@example.com", "name": "Jane Doe", "role": "user" }
  },
  "meta": {
    "access_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9…",
    "access_token_expires_at": "2026-10-09T17:27:59.2966735+07:00",
    "refresh_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9…",
    "refresh_token_expires_at": "2026-10-16T17:12:59.2976996+07:00"
  },
  "links": { "self": "/api/auth/login" }
}
```

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 401 | `unauthorized` | `The email or password is incorrect.` — the **same** message for an unknown email and a wrong password, so an account's existence never leaks |
| 401 | `unauthorized` | `Your account is no longer active.` — only **after** the password matched |
| 422 | `unprocessable_entity` | a field failed validation |
| 500 | `internal_error` | token signing / DB failure |

### `POST /api/auth/refresh`

Rotates the pair: spends the presented refresh token, issues a **new access + refresh token**
(both cookies replaced, both tokens in `meta`). The user is re-read, so the new access token carries
the **current** role and a deactivated user gets none.

The refresh token is taken from the `app-refresh-token` cookie; **without the cookie**, send it in
an optional `sessions` document:

| Attribute | Type | Required | Rules |
| --- | --- | --- | --- |
| `refresh_token` | string | no (when the cookie is sent) | ≤ 4096 |

```bash
# browser-style (cookie jar from the login)
curl -X POST "$BASE/api/auth/refresh" -b jar.txt -c jar.txt

# token in the body
curl -X POST "$BASE/api/auth/refresh" -H "$CT" -d '{
  "data": { "type": "sessions", "attributes": { "refresh_token": "<meta.refresh_token>" } }
}'
```

**Response — `200 OK`** — a **meta-only** document; store **both** new tokens, the old refresh
token is spent:

```json
{
  "jsonapi": { "version": "1.1" },
  "meta": {
    "access_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9…",
    "access_token_expires_at": "2026-10-09T17:27:59.4334119+07:00",
    "refresh_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9…",
    "refresh_token_expires_at": "2026-10-16T17:12:59.4339876+07:00"
  },
  "links": { "self": "/api/auth/refresh" }
}
```

**Errors**

| Status | `code` | When |
| --- | --- | --- |
| 401 | `unauthorized` | `Your session has expired. Please log in again.` — no / invalid / expired token, an access token sent instead, an unknown token, a revoked session, or a **reused** spent token (which also revokes the session — see [How a session works](#how-a-session-works)) |
| 401 | `unauthorized` | `Your account is no longer active.` |
| 500 | `internal_error` | DB / token signing failure |

→ on any `401` here: drop the local state, go to the login screen. **Never retry a refresh in a loop.**

### `POST /api/auth/logout`

Revokes the presented refresh token's whole session and clears both cookies. The token comes from
the cookie or the same optional `sessions` document as [refresh](#post-apiauthrefresh). A missing
or invalid token is **still a `204`** — logging out never fails from the client's point of view.

```bash
curl -X POST "$BASE/api/auth/logout" -b jar.txt -c jar.txt
```

**Response — `204 No Content`** (no body)

```
Set-Cookie: app-access-token=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax
Set-Cookie: app-refresh-token=; Path=/api/auth; Max-Age=0; HttpOnly; SameSite=Lax
```

**Errors:** a malformed `sessions` document (`400` / `409` / `422`, see [Request documents](#request-documents)); `500` (DB failure while revoking).

### `GET /api/me`

Any authenticated user. The caller's own account, read **live** from the database — `role` here may
be newer than the token's `role` claim.

```bash
curl -X GET "$BASE/api/me" -H "Authorization: Bearer $TOKEN"
```

**Response — `200 OK`**

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "users", "id": "1",
    "attributes": {
      "created_at": "2026-10-09T10:12:58.940147Z",
      "email": "jane@example.com",
      "name": "Jane Doe",
      "role": "user"
    }
  },
  "links": { "self": "/api/me" }
}
```

**Errors:** `401` (no / bad token, inactive account), `404` `Your account was not found.`, `500`.

---

## Response conventions

### Success

Every body has `jsonapi.version` and `links.self`; the payload is `data` (one resource or an array),
plus `included` and `meta` when there are any.

```json
{
  "jsonapi": { "version": "1.1" },
  "data": {
    "type": "notes", "id": "1",
    "attributes": { "title": "Groceries", "body": "milk, eggs", "created_at": "…", "updated_at": "…" },
    "relationships": { "created_by": { "data": { "type": "users", "id": "1" } } }
  },
  "included": [
    { "type": "users", "id": "1", "attributes": { "email": "jane@example.com", "name": "Jane Doe" } }
  ],
  "links": { "self": "/api/notes/1" }
}
```

- **Ids are strings.**
- **Timestamps** are RFC 3339; resource timestamps are UTC (`2026-10-09T10:12:59.318351Z`).
- **`included`** carries each related resource **once**, and never one that's also primary data.
- A **create** is `201` with a `Location` header; a **delete** / **logout** is `204` with no body.
- A response with no primary data (refresh, `/healthz`) is **meta-only**.

### Error

```json
{
  "jsonapi": { "version": "1.1" },
  "errors": [
    {
      "status": "404",
      "code": "not_found",
      "title": "Not Found",
      "detail": "We couldn't find that note. It may have been removed."
    }
  ]
}
```

Every `detail` is **user-safe text** (never a stack trace or SQL) — show it as-is. When the error
is about one part of the request, `source` says which — **exactly one** of:

| `source` member | Points at | Example |
| --- | --- | --- |
| `pointer` | a member of the request document (JSON Pointer) | `/data/attributes/title`, `/data/id` |
| `parameter` | a query parameter | `sort`, `filter[is_active]` |
| `header` | a request header | `Content-Type`, `Accept` |

Several errors share the HTTP status when they have the same one; otherwise the response is `400`
(all `4xx`) or `500`.

### Request ids

**Every response** — success, error, `404` / `405`, even a crash — carries an `X-Request-ID` header,
and every server log line that request writes has the same `request_id`. A well-formed id the client
(or a proxy) sends in `X-Request-ID` is kept (letters, digits and `._:-`, at most 128 characters);
anything else is replaced with a new UUID.

- **Show it next to error messages**, e.g. *"We couldn't save that note. (ref: 3f2a9c1e…)"*. When a
  user reports it, one search of the logs finds everything that request did.
- A **cross-origin** browser client can only read the header if the server (or the proxy in front of
  it) sends `Access-Control-Expose-Headers: X-Request-ID`.

```bash
curl -i "$BASE/api/notes/999" -H "Authorization: Bearer $TOKEN"
# HTTP/1.1 404 Not Found
# X-Request-ID: 3f2a9c1e-7b4d-4e0a-9f1b-2c3d4e5f6a7b
```

### Validation error

A failed rule is **422 Unprocessable Entity**, one error per field, each with its `source.pointer`
(so the form can put the message under the right input):

```json
{
  "jsonapi": { "version": "1.1" },
  "errors": [
    { "status": "422", "code": "unprocessable_entity", "title": "Unprocessable Entity",
      "detail": "Name cannot be blank.", "source": { "pointer": "/data/attributes/name" } },
    { "status": "422", "code": "unprocessable_entity", "title": "Unprocessable Entity",
      "detail": "Email must be a valid email address.", "source": { "pointer": "/data/attributes/email" } },
    { "status": "422", "code": "unprocessable_entity", "title": "Unprocessable Entity",
      "detail": "Password must be at least 8 characters long.", "source": { "pointer": "/data/attributes/password" } }
  ]
}
```

The `detail` names the field by its **label**, not its JSON name: `title` → "Title", `body` →
"Content", `name` → "Name", `email` → "Email", `password` → "Password", `refresh_token` →
"Refresh token", `trust_device` → "Trust device", `is_active` → "Active flag".

### Request documents

Every body is a JSON:API document with **one resource object** under `data`:

```json
{ "data": { "type": "notes", "id": "1", "attributes": { "title": "…" } } }
```

| Problem | Status | `detail` / `source.pointer` |
| --- | --- | --- |
| not JSON | 400 | `The request body is not valid JSON.` |
| no `data`, or `data` isn't one object | 400 | `The request body must be a JSON:API document with a data member.` — `/data` |
| wrong `type` | 409 | `This endpoint takes "notes" resources.` — `/data/type` |
| an `id` on a create | 403 | `Client-generated ids aren't supported.` — `/data/id` |
| an update without `id`, or an `id` other than the URL's | 409 | `The id in the body doesn't match the URL.` — `/data/id` |
| any `relationships` | 403 | `Relationships can't be set on this endpoint.` — `/data/relationships` |
| an attribute the endpoint doesn't take (read-only / unknown) | 403 | `"role" can't be set on this endpoint.` — `/data/attributes/role` |
| an attribute of the wrong JSON type (`"title": 5`) | 422 | `/data/attributes/title` |
| a failed rule | 422 | one per field — see [Validation error](#validation-error) |

### Query parameters

Each endpoint accepts only what its section lists. Anything else **reserved by JSON:API** (an
all-lowercase name such as `foo` or `page`) is a **400** with `source.parameter`; a name with any
other character (`_t`, `cacheBust`) is implementation-specific and **ignored**.

| Parameter | Meaning |
| --- | --- |
| `include` | comma list of relationships to expand into `included`. Each endpoint has a **default** (notes: `created_by`); `include=` (empty) expands nothing |
| `fields[TYPE]` | sparse fieldsets: comma list of the attributes / relationships of `TYPE` to return, e.g. `fields[notes]=title&fields[users]=name` |
| `sort` | **one** field, `-` prefix for descending, e.g. `-created_at` |
| `page[number]` / `page[size]` | 1-based page / size. Clamped, never rejected: number < 1 → 1, size < 1 → 20, size > 100 → 100 |
| `filter[NAME]` | the endpoint's filters |

A paginated list answers with `meta` and four pagination links (`null` when there's no such page):

```json
"meta": { "page": 1, "page_size": 1, "total": 2, "total_page": 2 },
"links": {
  "self":  "/api/notes?page[size]=1",
  "first": "/api/notes?page%5Bnumber%5D=1&page%5Bsize%5D=1",
  "last":  "/api/notes?page%5Bnumber%5D=2&page%5Bsize%5D=1",
  "prev":  null,
  "next":  "/api/notes?page%5Bnumber%5D=2&page%5Bsize%5D=1"
}
```

The links keep the request's other parameters (filters, sort, fields) — follow them as-is.

### Error code reference

| `code` | HTTP | Meaning |
| --- | --- | --- |
| `bad_request` | 400 | Malformed body / unsupported or invalid query parameter |
| `unauthorized` | 401 | Missing / invalid / expired token, inactive account, wrong credentials, expired session |
| `forbidden` | 403 | A role the route doesn't allow (`middleware.RequireRole`); a client-generated id, a relationship or a read-only attribute in the body |
| `not_found` | 404 | Unknown route; a missing resource (or someone else's); a non-numeric path id |
| `method_not_allowed` | 405 | e.g. `PUT` on a resource (updates are `PATCH`) |
| `not_acceptable` | 406 | See [Content negotiation](#content-negotiation) |
| `conflict` | 409 | Email already registered; wrong resource `type`; body `id` ≠ URL id |
| `unsupported_media_type` | 415 | See [Content negotiation](#content-negotiation) |
| `unprocessable_entity` | 422 | A field failed validation / has the wrong JSON type |
| `internal_error` | 500 | Unexpected server error (`Something went wrong, please try again.`) — also a panic |

---

## Enums

| Field | Value | Description |
| --- | --- | --- |
| `role` | `user` | default; what [register](#post-apiauthregister) always creates |
| `role` | `admin` | passes `middleware.RequireRole(enum.RoleAdmin)`. Granted directly in the database (no endpoint) |

---

## Health

### `GET /healthz`

Unauthenticated liveness probe, outside `/api` (no content negotiation). `version` is the build's
`main.Version`, baked in from the `VERSION` file by `build.sh` (`dev` under `make run`).

```bash
curl "$BASE/healthz"
```

**Response — `200 OK`**

```json
{
  "jsonapi": { "version": "1.1" },
  "meta": { "status": "up", "version": "1.0.0" },
  "links": { "self": "/healthz" }
}
```

---

## Client guide

### A typical browser session

1. `POST /api/auth/login` with `credentials: "include"` → cookies are set; keep `data` (the account).
2. `GET /api/me` for the live role.
3. Call the API with `credentials: "include"` — no `Authorization` header needed.
4. On a `401` *"You're not authorized to perform this action."*: call `POST /api/auth/refresh`
   **once** (single-flight across tabs), then retry the original call.
5. Refresh `401` or *"Your account is no longer active."* → go to the login screen.
6. `POST /api/auth/logout` on sign-out.
7. On any error, show `errors[].detail` with the response's `X-Request-ID` as a *"ref: …"* the user
   can quote to support ([Request ids](#request-ids)).

### A non-browser client (mobile, service)

Same flow, but store `meta.access_token` / `meta.refresh_token` from login, send
`Authorization: Bearer <access_token>`, and refresh with the token in a `sessions` document. After
every refresh **replace both** stored tokens — the old refresh token is spent, and presenting it
again later logs the session out.

### Current limitations

- No password reset / change, no email verification, no way to change a user's role over the API.
- Logout doesn't revoke the access token (it lives until `exp`).
- `refresh_tokens` grows by one row per refresh; nothing prunes it yet.
