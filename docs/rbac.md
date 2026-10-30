# Role-based access control (RBAC)

Roles and the permissions they grant live in the database (`roles` table), not in Go code. Routes ask for a *permission*; the database decides which roles hold it.

## Model

- `roles` table: `id`, `name`, `kind` (`business` or `system`), `description`, `permissions TEXT[]`.
- Every user has two roles, both referencing `roles.name` by foreign key:
  - `users.role` — a `business` role (`artist`, `producer`).
  - `users.system_role` — a `system` role (`user`, `super_admin`).
- A user's effective permissions are the **union** of both roles' permissions.
- Permission strings are declared once, as constants in `internal/gateway/middleware/permissions.go`.

## Request flow

1. `Authorize(perms...)` runs `RequireAuth` (the JWT proves who the caller is).
2. The auth middleware re-reads the account (status, role, system role) from the database, cached for 10 s. Token claims are *not* trusted for roles, so suspensions, demotions and promotions apply within ~10 s even to unexpired tokens.
3. `RoleStore` (an in-memory copy of `roles`, refreshed every 30 s) checks that every required permission is held by one of the user's roles. Unknown roles hold nothing, and a missing store denies everything.

```go
authz := config.AuthMiddleware.Authorize
mux.Handle("GET /admin/users", authz(middleware.PermAdminUsersRead)(http.HandlerFunc(h.ListUsers)))
```

## Common tasks

### Grant an existing permission to a role

```sql
-- db/migrations/0000NN_producers_can_view_admin_orders.up.sql
UPDATE roles
SET permissions = array_append(permissions, 'admin:orders:read'), updated_at = NOW()
WHERE name = 'producer' AND NOT ('admin:orders:read' = ANY(permissions));
```

Down migration: `array_remove(permissions, 'admin:orders:read')`. No code change or restart needed; running servers pick it up within 30 s.

### Add a new role

```sql
INSERT INTO roles (id, name, kind, description, permissions)
VALUES (5, 'support', 'system', 'Support staff', '{admin:users:read}');
```

A `system` role can immediately be assigned with `PATCH /admin/users/{id}/system-role`. `GET /admin/roles` lists every role and its permissions.

### Add a new permission

1. Add a constant and an entry in `AllPermissions` (`permissions.go`).
2. Protect the route: `authz(middleware.PermYourThing)(handler)`.
3. Grant it to the right roles in a migration (see above).
4. Run `make swagger` if you added a route.

`TestPermissionsMatchMigrations` fails if a permission string in a migration has no constant, or a constant is granted by no migration, so typos are caught in CI.

## What RBAC does not cover

Role checks answer "may this *kind* of user do this *kind* of thing". Ownership ("only the producer of *this* spec") depends on the resource and stays in the service layer, e.g. spec update/delete, `GetOrder`, `VerifyPayment`.

## Notes

- The `?token=` query parameter is accepted only on `GET /ws` (browsers cannot set headers on WebSockets). Every other route needs the `Authorization: Bearer` header.
- If you ever need a user to hold several roles of the same kind, replace the two columns with a `user_roles` join table; the union semantics already match.
