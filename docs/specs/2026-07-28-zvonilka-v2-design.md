# Zvonilka v2 — Friends, Servers, Channels, Direct Calls

**Date:** 2026-07-28
**Status:** Draft
**Spec for:** 4-phase feature rollout

---

## Overview

Zvonilka v2 adds four subsystems:

1. **Friends** — user-to-user friend relationships
2. **Servers + Channels** — Discord-like servers with persistent channel templates and ephemeral voice rooms
3. **Invitations** — 3 methods to join servers (invite link, direct invite, join request)
4. **Direct Calls** — friend-to-friend calls without servers, reusing existing rooms infrastructure

---

## Phase 1: Friends

### Data Model

```sql
CREATE TABLE friendships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id),
    friend_id   UUID NOT NULL REFERENCES users(id),
    status      TEXT NOT NULL DEFAULT 'pending', -- 'pending' | 'accepted'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(user_id, friend_id),
    CHECK(user_id != friend_id)
);

-- Partial indexes for list queries
CREATE INDEX idx_friendships_user_status ON friendships(user_id, status);
CREATE INDEX idx_friendships_friend_status ON friendships(friend_id, status);
```

**Semantics:**
- `user_id` is always the sender of the friend request
- `friend_id` is always the receiver
- 1 row per friendship (Variant C — no duplication)
- `status = 'pending'`: sender sent, receiver hasn't acted
- `status = 'accepted'`: both are friends

```sql
ALTER TABLE users ADD COLUMN allow_friend_requests BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN outgoing_requests_today INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN requests_reset_at TIMESTAMPTZ;
```

### API Endpoints

All under `/users/v1/friends`. JWT required on all endpoints.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/requests` | Send friend request. Body: `{tag: "A1B2C3D"}`. Validates: not self, not existing friend/pending, not blocked, <100/day. |
| GET | `/requests` | List incoming pending requests (where I am friend_id, status=pending) |
| POST | `/requests/:id/accept` | Accept → status='accepted'. Only receiver can accept. |
| POST | `/requests/:id/decline` | Decline → DELETE row. Only receiver can decline. |
| GET | `/friends` | List friends (status=accepted, both directions). Returns UserRef[]. |
| DELETE | `/friends/:id` | Remove friend → DELETE row. Either direction. |
| GET | `/outgoing` | List outgoing pending requests (where I am user_id, status=pending) |
| DELETE | `/outgoing/:id` | Cancel outgoing request → DELETE row. Only sender can cancel. |

### Rules

- Send by user tag (e.g. `Alice#A1B2C3D`)
- **100 outgoing requests per day** (sliding window based on `requests_reset_at`)
- Can't send to: self, existing friend (accepted), pending request (either direction), blocked user
- `allow_friend_requests = false` rejects incoming requests at API level
- Notification: polling only initially (`GET /requests`). WebSocket push later.

### Error Codes

| Error | HTTP |
|-------|------|
| User not found by tag | 404 |
| Already friends | 409 |
| Request already pending | 409 |
| Daily limit exceeded | 429 |
| Incoming requests blocked | 403 |
| Not the receiver of this request | 403 |
| Not the sender of this request | 403 |

---

## Phase 2: Servers + Channels

### Data Model

```sql
CREATE TABLE servers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    icon_url    TEXT,
    owner_id    UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE server_members (
    server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id),
    role        TEXT NOT NULL DEFAULT 'member', -- 'owner' | 'admin' | 'member'
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY(server_id, user_id)
);

CREATE TABLE channels (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'voice', -- 'voice' (future: 'text')
    position    INT NOT NULL DEFAULT 0,
    created_by  UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(server_id, name)
);
```

### Room Lifecycle

```
Channel (template, permanent)
    │
    ├─ First user joins → LiveKit room created
    │   Room name = "channel:" + channel_id (hex)
    │   WS event: "channel.active"
    │
    ├─ Users join/leave → participant.joined/left via existing hub
    │
    ├─ Last user leaves → LiveKit room destroyed (existing call.ended flow)
    │   WS event: "channel.idle"
    │
    └─ Template remains — ready for next join
```

**Room tracking:** `rooms` table gets `type` column:
```sql
ALTER TABLE rooms ADD COLUMN type TEXT NOT NULL DEFAULT 'public';
-- 'public'  = original rooms (kept for backward compat)
-- 'server'  = server channel rooms
-- 'direct'  = friend-to-friend calls
```

### Server API

All under `/servers/v1/servers`. JWT required. Server membership required for read access.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/servers` | any user | Create server. Creator becomes owner. |
| GET | `/servers` | any user | List servers I'm a member of |
| GET | `/servers/:id` | member | Get server + channels + online counts |
| PATCH | `/servers/:id` | admin+ | Update name/icon |
| DELETE | `/servers/:id` | owner only | Delete server (cascades to channels) |
| GET | `/servers/:id/members` | member | List members with roles |
| PATCH | `/servers/:id/members/:uid` | admin+ | Change role (member↔admin). Can't demote owner. |
| DELETE | `/servers/:id/members/:uid` | admin+ | Kick member. Can't kick owner. Admin can't kick admin (owner only). |
| POST | `/servers/:id/transfer` | owner only | Transfer ownership to another member |

### Channel API

All under `/servers/v1/servers/:id/channels`. Server membership required.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/channels` | admin+ | Create channel template |
| PATCH | `/:cid` | admin+ | Rename / reorder |
| DELETE | `/:cid` | admin+ | Delete channel |
| POST | `/:cid/join` (at `/servers/v1/channels/:cid/join`) | member | Join channel → LiveKit token. Creates LiveKit room if first. |
| GET | `/:cid/events` (at `/servers/v1/channels/:cid/events`) | member | WebSocket for channel events |

### Admin Rules

- **Owner:** create server → owner. Can do everything. Only owner can: delete server, transfer ownership, demote admins. Cannot be demoted/kicked by anyone.
- **Admin:** promoted by owner. Can: manage channels (CRUD), kick members, promote members to admin. Cannot: demote other admins, kick other admins, demote/kick owner.
- **Member:** can join channels, see server info.

---

## Phase 3: Invitations

### Data Model

```sql
CREATE TABLE invites (
    code        TEXT PRIMARY KEY, -- 8-char random alphanumeric
    server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    created_by  UUID NOT NULL REFERENCES users(id),
    max_uses    INT NOT NULL DEFAULT 0, -- 0 = unlimited
    use_count   INT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ,           -- NULL = never
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE direct_invites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id),  -- receiver
    invited_by  UUID NOT NULL REFERENCES users(id),  -- sender
    status      TEXT NOT NULL DEFAULT 'pending', -- 'pending'|'accepted'|'declined'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(server_id, user_id)
);

CREATE TABLE join_requests (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id),
    status      TEXT NOT NULL DEFAULT 'pending', -- 'pending'|'approved'|'rejected'
    reviewed_by UUID REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(server_id, user_id)
);
```

### Method 1: Invite Link

| Method | Path | Description |
|--------|------|-------------|
| POST | `/servers/v1/servers/:id/invites` | Create invite link. Body: `{max_uses?, expires_at?}`. Returns `{code}`. Admin+. |
| DELETE | `/servers/v1/servers/:id/invites/:code` | Revoke invite. Admin+. |
| GET | `/servers/v1/servers/:id/invites` | List active invites. Admin+. |
| POST | `/servers/v1/servers/join/:code` | Join via code. Any authenticated user. |

### Method 2: Direct Invite

| Method | Path | Description |
|--------|------|-------------|
| POST | `/servers/v1/servers/:id/invites/users` | Invite user by tag. Any member. |
| GET | `/users/v1/invites` | List my server invites (separate from friend requests) |
| POST | `/users/v1/invites/:id/accept` | Accept → joined as member |
| POST | `/users/v1/invites/:id/decline` | Decline → status='declined' |

### Method 3: Join Request

| Method | Path | Description |
|--------|------|-------------|
| POST | `/servers/v1/servers/:id/join-request` | Request to join. Any authenticated user. |
| GET | `/servers/v1/servers/:id/join-requests` | List pending. Admin+. |
| POST | `/servers/v1/servers/:id/join-requests/:rid/approve` | Approve → user added as member. Admin+. |
| POST | `/servers/v1/servers/:id/join-requests/:rid/reject` | Reject. Admin+. |

---

## Phase 4: Direct Calls

### Concept

Friend-to-friend calls without servers. Reuses existing `rooms` table, WebSocket hub, and LiveKit integration.

### Data Model Additions

```sql
ALTER TABLE rooms ADD COLUMN call_timeout_at TIMESTAMPTZ;
ALTER TABLE rooms ADD COLUMN solo_since TIMESTAMPTZ;
```

### API Endpoints

All under `/rooms/v1/calls`. JWT required.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/calls` | Initiate call. Body: `{friend_id}`. Validates: are friends, no active call between them. Creates room (type=direct), stores timeout_at = now()+2min. Returns `{room_id, token}`. Caller gets LiveKit token immediately. Sends WebSocket event `call.incoming` to callee. |
| POST | `/calls/:id/accept` | Accept incoming call. Clears timeout. Returns `{token}`. Callee gets LiveKit token. |
| POST | `/calls/:id/decline` | Decline → delete room. |
| GET | `/calls` | Active calls (incoming + ongoing). |

### Call Expiry Rules

1. **Ring timeout:** 2 minutes. If callee doesn't accept within 2 min → room deleted. Enforced by background goroutine scanning `call_timeout_at < now()`.
2. **Idle room cleanup:** If only 1 person in room for 15 minutes → room closed. Enforced by `solo_since` timestamp: set when count goes 2→1, cleared when count goes 1→2. Background goroutine closes rooms where `solo_since < now() - 15min`.

### WebSocket Notification

User's personal WebSocket connection receives events:

```json
{
  "type": "call.incoming",
  "callId": "uuid",
  "caller": {"id": "uuid", "nickname": "Alice", "tag": "A1B2C3D"},
  "roomId": "uuid",
  "expiresAt": "ISO8601"
}
```

### Rules

- Only friends can call each other
- One active call per friend pair at a time
- Caller gets token immediately, callee gets token on accept
- Room auto-deleted: on ring timeout, on both leave (existing leaveRoom), on solo timeout
- User WebSocket connection required for incoming call notification

---

## Cross-Cutting Concerns

### New Service: servers

A new Go service at `backend/servers/` with:
- `internal/config/` — env-based config
- `internal/repo/` — PostgreSQL queries for servers, channels, members, invites
- `internal/service/` — business logic + HTTP handlers
- `main.go` — Echo server setup with JWT auth middleware

### User WebSocket (Phase 4 prerequisite)

Phase 4 requires a persistent WebSocket connection per user for `call.incoming` events. This connection is established at app startup and multiplexes events: friend requests, server invites, incoming calls. Implemented as a separate WebSocket endpoint on the users service.

### DB Migrations

Each phase includes its own migrations:
- Phase 1: `friendships` table, `users` columns
- Phase 2: `servers`, `server_members`, `channels` tables, `rooms.type` column
- Phase 3: `invites`, `direct_invites`, `join_requests` tables
- Phase 4: `rooms.call_timeout_at`, `rooms.solo_since` columns

### API Specs

New OpenAPI specs under `backend/api-schema/zvonilkaRU/`:
- `friends/` — Phase 1 endpoints (under users service)
- `servers/` — Phase 2-3 endpoints
- `calls/` — Phase 4 endpoints (under rooms service)

### IAM Integration

Server/Channel operations use IAM with new object types:
- `server:<id>` — relations: `owner`, `admin`, `member`
- `channel:<id>` — relations: `manage` (inherited from server admin)
