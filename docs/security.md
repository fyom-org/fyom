Library access:
- Frontend permissions UI is not a security boundary.
- Backend media, progress, status, stream, poster, backdrop, logo, and job endpoints enforce library access.
- Missing permission resolution fails closed for non-privileged users.
- nil allowed library IDs means unrestricted admin/owner access.
- empty allowed library IDs means no library access.

Presigned media URLs:
- Presigned URLs are path-bound and time-limited.
- Valid presigned URLs authorize direct GET/HEAD media resource access without JWT.
- Revoking library access does not invalidate already-issued presigned URLs before expiration.
- Immediate revocation would require shorter expiry, key rotation, denylist, or permission-versioned signatures.

Desktop bootstrap:
- /api/v1/internal/bootstrap-session is localhost-only.
- It only works while a bootstrap admin has password_change_required=true.
- It returns a normal JWT session.
- Changing password clears password_change_required but does not rotate the JWT.
- After password change, bootstrap-session returns 404.
