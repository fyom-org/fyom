# Library Access Security Model

This document describes how fyom enforces per-user library access across its
authorization realms, how `allowed_libraries` is interpreted, how presigned
media URLs are validated, and what operators should expect when revoking
library access.

It is required reading for anyone touching:

- `internal/middleware/permissions.go`
- `internal/middleware/presign.go`
- `pkg/presign/presign.go`
- `internal/handler/media.go`
- `internal/server/server.go`

---

## 1. Authorization realms

fyom has three related but distinct authorization realms that must not be
collapsed into one mental model.

| Realm | Guarded by | Carries | Used for |
|-------|------------|---------|----------|
| **JWT user API** | `AuthMiddlewareWithUserRepo` + `ResolvePermissions` | user identity, role, allowed library IDs | `/api/v1/library/*`, `/api/v1/libraries`, `/api/v1/media/{id}/progress`, `/api/v1/media/{id}/status` |
| **JWT admin API** | `AuthMiddlewareWithUserRepo` + `RequireAdmin` | user identity, role | `/api/v1/admin/*`, admin-only import/delete routes |
| **Presigned media** | `RequireValidPresign` | a path-bound, time-limited HMAC signature | `/api/v1/media/{id}/{stream,poster,backdrop,logo}` |

The presigned realm exists because `<img>` and `<video>` tags cannot reliably
send `Authorization` headers. A presigned URL is therefore a capability token:
possession of a valid URL authorizes direct `GET` or `HEAD` access to exactly
the signed resource path for a limited time.

The JWT user API and the presigned media API are reconciled by a single
chokepoint:

```go
middleware.IsLibraryAllowed(r, libraryID)
````

That function understands both JWT-derived library permissions and validated
presigned media requests.

***

## 2. The `allowed_libraries` semantics

`ResolvePermissions` loads the current user's library permissions and stores
them in request context as `allowed_libraries`.

This context value has three distinct states that must never be collapsed:

| Value                            | Meaning                                           | Who gets this                               |
| -------------------------------- | ------------------------------------------------- | ------------------------------------------- |
| `nil`                            | unrestricted access; no library filter is applied | `admin` and reserved `owner` roles          |
| `[]string{}` non-nil empty slice | no library access; fail closed                    | regular users with zero library permissions |
| `[]string{"lib-A", ...}`         | restricted to the listed library IDs              | regular users with explicit grants          |

### Fail-closed default

If `ResolvePermissions` has not run for a request, `GetAllowedLibraryIDs`
applies this rule:

* privileged role (`admin` or reserved `owner`) -> `nil` unrestricted
* any other role -> `[]string{}` no access

This is the single most important security property in the codebase:

> A forgotten middleware mount must never silently grant unrestricted access.

Routes that legitimately skip `ResolvePermissions`, such as the presigned
media group, rely on `RequireValidPresign` and `IsPresignedAccess` instead.

***

## 3. How `IsLibraryAllowed` decides

Handlers should call `IsLibraryAllowed(r, libraryID)` to check library access.
Its decision tree is:

```text
1. libraryID == ""                       -> false
2. IsPresignedAccess(r) == true          -> true
3. GetAllowedLibraryIDs(r):
     nil                                 -> true
     []                                  -> false
     [..]                                -> true iff libraryID is in the list
```

Step 2 means:

```text
The request path already passed HMAC validation in RequireValidPresign.
```

`IsLibraryAllowed` itself does not validate signatures. It trusts the context
marker set only by `RequireValidPresign` after successful validation.

This is what makes the presigned media route group safe without mounting
`AuthMiddlewareWithUserRepo` or `ResolvePermissions`. A request that failed
presigned validation is rejected with `403` by middleware and never reaches the
media handler.

***

## 4. Handler-level enforcement

Every media endpoint enforces library access through one of two helpers in
`internal/handler/media.go`.

### `getAccessibleMediaItem(w, r, id)`

This helper:

1. loads the media item by ID
2. verifies `IsLibraryAllowed(r, item.LibraryID)`
3. returns `404 "resource not found"` when access is denied

The handler intentionally returns `404`, not `403`, for denied media resources.
This is anti-enumeration behavior: a user cannot probe media IDs to discover
which resources exist inside libraries they cannot access.

### `filterMediaItemsByAllowedLibraries(r, items)`

This helper post-filters a slice of media items by allowed library IDs. It is
used when a user-specific query may return items from libraries the user can no
longer access, for example after access was revoked.

### Enforced endpoints

| Endpoint                                               | Mechanism                                                                                         |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------- |
| `GET /api/v1/library`                                  | optional `library_id` checked through `IsLibraryAllowed`; repository receives `allowedIDs`        |
| `GET /api/v1/library/{id}`                             | `getAccessibleMediaItem`                                                                          |
| `GET /api/v1/library/{id}/episodes`                    | parent `getAccessibleMediaItem`, show-type guard, episode filtering                               |
| `GET /api/v1/library/by-status`                        | `filterMediaItemsByAllowedLibraries`                                                              |
| `GET /api/v1/library/continue`                         | SQL-layer filter using allowed library IDs                                                        |
| `GET /api/v1/libraries`                                | filters library list by `allowedIDs`                                                              |
| `PUT /api/v1/media/{id}/progress`                      | `getAccessibleMediaItem`, show-type guard, progress validation                                    |
| `PUT /api/v1/media/{id}/status`                        | `getAccessibleMediaItem`                                                                          |
| `GET /api/v1/media/{id}/status`                        | `getAccessibleMediaItem`                                                                          |
| `GET /api/v1/library/jobs/{id}`                        | `IsLibraryAllowed(r, job.LibraryID)`                                                              |
| `DELETE /api/v1/library/{id}`                          | admin-only route gate plus `IsUnrestrictedLibraryAccess`                                          |
| `POST /api/v1/library/import`                          | admin-only route gate plus `IsUnrestrictedLibraryAccess` and `IsLibraryAllowed(r, req.LibraryID)` |
| `GET /api/v1/media/{id}/{stream,poster,backdrop,logo}` | `RequireValidPresign` plus `getAccessibleMediaItem`, which honors `IsPresignedAccess`             |

`ServeContent` is a low-level primitive that performs no permission checks.
Every caller must verify library access before invoking it.

The approved callers are:

* `Stream`
* `Poster`
* `ServeBackdrop`
* `ServeLogo`

Each must call `getAccessibleMediaItem` before sending bytes.

***

## 5. Defense in depth: route-layer gates

The route table applies additional gates independent of handler logic.

### Admin routes

```text
/api/v1/admin/*
```

These routes use:

```go
AuthMiddlewareWithUserRepo
RequireAdmin
```

They do not use `ResolvePermissions`.

`RequireAdmin` accepts only:

```text
admin
```

The reserved `owner` role is accepted by library access checks, but it does not
currently pass `RequireAdmin`.

No current code path should mint an `owner` role. Until an owner role is
formally introduced, `owner` must be treated as reserved and must not be emitted
by login, registration, bootstrap, or user-management flows.

### Admin-only user routes

These routes are mounted inside the authenticated user group but are wrapped
with `RequireAdmin`:

```text
POST   /api/v1/library/import
DELETE /api/v1/library/{id}
```

They also perform handler-level checks. This is intentional defense in depth.

### Presigned media routes

```text
/api/v1/media/{id}/{stream,poster,backdrop,logo}
```

These routes use:

```go
RequireValidPresign
```

They do not use JWT authentication or `ResolvePermissions`.

This is intentional. Browsers fetch these resources through `<img>` and
`<video>` tags, where Authorization headers are not reliable.

***

## 6. Presigned URL contract

A presigned URL is generated by:

```go
presign.Signer.Generate(basePath)
```

It has the form:

```text
{basePath}?exp={unix-seconds}&sig={hex-hmac-sha256}
```

The HMAC is bound to:

```text
method + path + exp
```

The canonical string is:

```text
{METHOD}
{PATH}
{EXP}
```

### Method normalization

`METHOD` is normalized as follows:

* empty method -> `GET`
* `GET` -> `GET`
* `HEAD` -> `GET`
* other methods remain distinct

`GET` and `HEAD` intentionally share a signature because browsers and
intermediaries may use `HEAD` for probing resources that were signed for `GET`.

### Path normalization

`PATH` is the URL path. Query strings are stripped before signing.

This means clients may add unrelated query parameters after the path as long as
the signed path and `exp` remain valid. The signature does not authorize a
different path.

### Expiry validation

`ValidateMethod` rejects a signature when any of these conditions hold:

* signer is nil
* signer secret is empty
* path normalizes to empty
* `exp` is missing or non-numeric
* `sig` is missing or non-hex
* `now > exp`
* `exp > now + expirySeconds + clockSkewSeconds`
* decoded signature does not `hmac.Equal` the expected MAC

The far-future expiry check prevents accepting arbitrarily long-lived tokens
even when the MAC is otherwise valid.

### Middleware behavior

`RequireValidPresign`:

* allows only `GET` and `HEAD`
* rejects other methods with `405` and `Allow: GET, HEAD`
* rejects missing, malformed, expired, far-future, or tampered signatures with
  `403`
* sets `Cache-Control: no-store` on failure
* sets `Cache-Control: public, max-age=3600, immutable` on success
* marks the request context with `IsPresignedAccess=true`
* stores the validated path retrievable through `GetPresignedPath`

***

## 7. Revoking library access does not instantly invalidate presigned URLs

This is the most commonly misunderstood property of the model:

> When an admin revokes a user's library access, any presigned URL that user
> already obtained remains valid until its `exp`.

This is by design.

A presigned URL is a capability token. It is the authorization for direct media
fetches by `<img>` and `<video>` tags. It cannot be retroactively invalidated
without introducing a revocation mechanism.

The handler-level `IsLibraryAllowed` short-circuit on `IsPresignedAccess`
intentionally honors a valid presigned URL regardless of current library
permissions.

### Cache lifetime must track URL lifetime

The current middleware sends this header on successful presigned media
responses:

```text
Cache-Control: public, max-age=3600, immutable
```

This matches the default presign expiry of 3600 seconds.

If operators shorten `expirySeconds`, they must also reduce the media response
cache lifetime. Otherwise, a browser may continue serving previously cached
bytes until `max-age` expires, even though the URL would fail validation on a
fresh network request.

In other words:

* network authorization lasts until `exp`
* client-side cached bytes may remain usable until `Cache-Control: max-age`

Keep these two windows aligned unless the longer client-side cache window is
explicitly acceptable.

### If instant revocation is required

Choose one of the following designs:

1. Shorten `expirySeconds`.
   This reduces the maximum exposure window but increases presign churn.

2. Add signature versioning.
   Include a `kid` or `v` value in the signed string and rotate secrets to
   invalidate outstanding URLs.

3. Add a denylist.
   Track revoked resource tokens or `(media_id, exp)` pairs and check them on
   every media request.

4. Use per-user presign keys.
   Derive the HMAC key from a per-user secret so revoking a user invalidates
   only that user's URLs.

The current default, a 3600-second shared-secret presign window, is the intended
tradeoff for a self-hosted media catalog and resource dispatcher.

***

## 8. Frontend permission UI is not a security boundary

The web UI hides libraries and actions the current user cannot access. This is
UX only.

Every endpoint listed in section 4 enforces access on the backend
independently. A user who crafts raw HTTP requests against an unauthorized
library receives:

* `404` for hidden media/library resources
* `403` for admin-only writes

Never rely on hidden UI elements as an access-control mechanism.

***

## 9. Desktop bootstrap session semantics

Desktop bootstrap is separate from library access, but it is part of fyom's
security model and must remain constrained.

The endpoint is:

```text
GET /api/v1/internal/bootstrap-session
```

It is:

* unauthenticated
* localhost-only
* used only by the Wails desktop frontend before it has a token
* available only while a bootstrap admin has `password_change_required=true`

It returns a normal JWT session:

```json
{
  "token": "...",
  "access_token": "...",
  "token_type": "Bearer",
  "expires_in": 86400,
  "user": {
    "password_change_required": true
  },
  "password_change_required": true
}
```

After the user changes their password:

```text
password_change_required=false
```

The endpoint naturally returns:

```text
404 no bootstrap session available
```

Changing password does not rotate the current JWT. The current session remains
valid and is rehydrated through `/api/v1/auth/me` on subsequent desktop starts.

Server mode does not use this endpoint as a login bypass. Server mode still
requires login with the generated bootstrap password.

***

## 10. Testing the security model

The security-critical code paths are covered by the following tests.

### Middleware tests

```text
internal/middleware/permissions_test.go
```

Covers:

* `GetAllowedLibraryIDs`
* `IsLibraryAllowed`
* `ArePermissionsResolved`
* `IsUnrestrictedLibraryAccess`
* DB-backed `ResolvePermissions`
* fail-closed behavior when permissions are unresolved
* presigned access marker handling

```text
internal/middleware/presign_test.go
```

Covers:

* `RequireValidPresign`
* GET and HEAD method handling
* non-GET/HEAD method rejection
* missing, tampered, expired, and far-future signatures
* context marking through `IsPresignedAccess`
* validated path storage through `GetPresignedPath`

### Presign package tests

```text
pkg/presign/presign_test.go
```

Covers:

* `ValidateMethod`
* method binding
* `GET`/`HEAD` normalization
* legacy `Validate` compatibility
* expired and far-future expiry rejection
* path binding
* query string stripping
* malformed signature handling
* key isolation

### Media access integration tests

```text
internal/handler/media_access_test.go
```

Covers end-to-end behavior through the real chi route stack:

* denied-library `404` on media detail
* denied-library `404` on progress update
* denied-library `404` on status read/write
* denied-library `404` on job status
* `GetByStatus` filtering
* `ContinueWatching` filtering
* library list filtering
* presigned media gating and bypass
* presigned path binding
* admin-only import/delete
* admin cross-library access
* fail-closed behavior when `ResolvePermissions` is absent

### Desktop bootstrap tests

```text
internal/handler/auth_bootstrap_test.go
```

Covers:

* bootstrap session issuance for `password_change_required=true` admin/owner
* no password hash leakage
* token and `access_token` compatibility fields
* JWT claims
* endpoint returns `404` when no bootstrap user exists
* regular users cannot receive bootstrap sessions
* endpoint returns `404` after password change flag is cleared
* non-local requests are rejected

***

## 11. Required verification before merging

When changing any of the files listed in section 1, run:

```bash
task test
task lint
```

At minimum, the following direct commands must pass:

```bash
go test ./...
```

If frontend auth, permissions UI, or media playback code also changed, run:

```bash
cd web
npm run build
```

Expected outcome:

```text
All commands exit 0.
No permission enforcement tests are skipped.
No presigned media tests are skipped.
```

***

## 12. Strict maintenance rules

Do not change these semantics casually:

* Do not treat missing `ResolvePermissions` as unrestricted for regular users.
* Do not collapse `nil` and empty slices in `allowed_libraries`.
* Do not use frontend visibility as a permission boundary.
* Do not allow presigned URLs to authorize write operations.
* Do not mount `ResolvePermissions` on the presigned media route group.
* Do not remove handler-level checks just because route-layer gates exist.
* Do not return `403` for hidden media resources unless intentionally changing
  the anti-enumeration policy.
* Do not extend the `owner` role without updating route gates, tests, and this
  document.
