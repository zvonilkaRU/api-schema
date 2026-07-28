# Friends Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add friend relationships to the users service — send/accept/decline friend requests, list friends, remove friends. 100 requests/day limit.

**Architecture:** New `friendships` table in PostgreSQL (1 row per friendship, Variant C). New HTTP handlers in existing users service under `/users/v1/friends`. Reuses JWT auth, rate-limit middleware, and existing repo pattern.

**Tech Stack:** Go 1.24, pgx/v5, Echo v4, PostgreSQL, OpenAPI 3.1 (oapigen code generation).

**Spec:** `docs/specs/2026-07-28-zvonilka-v2-design.md` § Phase 1

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `backend/users/migrations/005_friendships.sql` | Create | Table + indexes |
| `backend/users/migrations/006_users_friend_settings.sql` | Create | allow_friend_requests + counters columns |
| `backend/users/internal/repo/repo.go` | Modify | Add friendship methods |
| `backend/users/internal/service/friends.go` | Create | Business logic for friends |
| `backend/users/internal/service/users_server.go` | Modify | Wire friends handlers |
| `backend/api-schema/zvonilkaRU/users/src/openapi/schemas/friends/FriendRequest.yaml` | Create | API schema |
| `backend/api-schema/zvonilkaRU/users/src/openapi/resources/v1/friends/friends.yaml` | Create | API endpoints |
| `backend/api-schema/zvonilkaRU/users/src/openapi/openapi.yaml` | Modify | Register friends paths |
| `backend/api-schema/zvonilkaRU/users/src/openapi/schemas/models/User.yaml` | Modify | Add tag field if missing |

---

### Task 1: DB migration — friendships table

**Files:**
- Create: `backend/users/migrations/005_friendships.sql`
- Create: `backend/users/migrations/006_users_friend_settings.sql`

- [ ] **Step 1: Create friendships migration**

Write to `backend/users/migrations/005_friendships.sql`:

```sql
-- +goose Up
CREATE TABLE friendships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    friend_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(user_id, friend_id),
    CHECK(user_id != friend_id)
);

CREATE INDEX idx_friendships_user_status ON friendships(user_id, status);
CREATE INDEX idx_friendships_friend_status ON friendships(friend_id, status);

-- +goose Down
DROP TABLE IF EXISTS friendships;
```

- [ ] **Step 2: Create user settings migration**

Write to `backend/users/migrations/006_users_friend_settings.sql`:

```sql
-- +goose Up
ALTER TABLE users ADD COLUMN IF NOT EXISTS allow_friend_requests BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS outgoing_requests_today INT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS requests_reset_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS allow_friend_requests;
ALTER TABLE users DROP COLUMN IF EXISTS outgoing_requests_today;
ALTER TABLE users DROP COLUMN IF EXISTS requests_reset_at;
```

- [ ] **Step 3: Commit**

```bash
git add backend/users/migrations/005_friendships.sql backend/users/migrations/006_users_friend_settings.sql
git commit -m "feat(users): add friendships table + friend request settings"
```

---

### Task 2: Repo layer — friendship queries

**Files:**
- Modify: `backend/users/internal/repo/repo.go` (append new methods)

- [ ] **Step 1: Read current repo.go**

Read `/Users/n.shchugorev/zvonilka/backend/users/internal/repo/repo.go` to understand existing patterns (sql queries, error handling, types).

- [ ] **Step 2: Define Friendship type**

Add after existing type definitions in repo.go:

```go
type Friendship struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	FriendID  uuid.UUID
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}
```

- [ ] **Step 3: Add CreateFriendRequest method**

```go
func (db *DB) CreateFriendRequest(ctx context.Context, userID, friendID uuid.UUID) (*Friendship, error) {
	var f Friendship
	err := db.pool.QueryRow(ctx,
		`INSERT INTO friendships (user_id, friend_id, status)
		 VALUES ($1, $2, 'pending')
		 ON CONFLICT (user_id, friend_id) DO UPDATE SET status = 'pending', updated_at = now()
		 RETURNING id, user_id, friend_id, status, created_at, updated_at`,
		userID, friendID,
	).Scan(&f.ID, &f.UserID, &f.FriendID, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create friend request: %w", err)
	}
	return &f, nil
}
```

- [ ] **Step 4: Add GetFriendship method**

```go
func (db *DB) GetFriendship(ctx context.Context, userID, friendID uuid.UUID) (*Friendship, error) {
	var f Friendship
	err := db.pool.QueryRow(ctx,
		`SELECT id, user_id, friend_id, status, created_at, updated_at
		 FROM friendships
		 WHERE user_id = $1 AND friend_id = $2`,
		userID, friendID,
	).Scan(&f.ID, &f.UserID, &f.FriendID, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get friendship: %w", err)
	}
	return &f, nil
}
```

- [ ] **Step 5: Add UpdateFriendshipStatus method**

```go
func (db *DB) UpdateFriendshipStatus(ctx context.Context, id, userID uuid.UUID, status string) (*Friendship, error) {
	var f Friendship
	err := db.pool.QueryRow(ctx,
		`UPDATE friendships SET status = $1, updated_at = now()
		 WHERE id = $2 AND friend_id = $3
		 RETURNING id, user_id, friend_id, status, created_at, updated_at`,
		status, id, userID,
	).Scan(&f.ID, &f.UserID, &f.FriendID, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("update friendship status: %w", err)
	}
	return &f, nil
}
```

- [ ] **Step 6: Add DeleteFriendship method**

```go
func (db *DB) DeleteFriendship(ctx context.Context, id, userID uuid.UUID) error {
	tag, err := db.pool.Exec(ctx,
		`DELETE FROM friendships WHERE id = $1 AND (user_id = $2 OR friend_id = $2)`, id, userID)
	if err != nil {
		return fmt.Errorf("delete friendship: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}
```

- [ ] **Step 7: Add ListFriends method**

```go
func (db *DB) ListFriends(ctx context.Context, userID uuid.UUID) ([]UserRef, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT u.id, u.nickname, u.tag
		 FROM friendships f
		 JOIN users u ON (CASE WHEN f.user_id = $1 THEN f.friend_id ELSE f.user_id END) = u.id
		 WHERE (f.user_id = $1 OR f.friend_id = $1) AND f.status = 'accepted'
		 ORDER BY u.nickname`, userID)
	if err != nil {
		return nil, fmt.Errorf("list friends: %w", err)
	}
	defer rows.Close()
	var friends []UserRef
	for rows.Next() {
		var ref UserRef
		if err := rows.Scan(&ref.ID, &ref.Nickname, &ref.Tag); err != nil {
			return nil, fmt.Errorf("scan friend: %w", err)
		}
		friends = append(friends, ref)
	}
	return friends, rows.Err()
}
```

- [ ] **Step 8: Add ListIncomingRequests + ListOutgoingRequests methods**

```go
func (db *DB) ListIncomingRequests(ctx context.Context, userID uuid.UUID) ([]Friendship, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT f.id, f.user_id, f.friend_id, f.status, f.created_at, f.updated_at,
		        u.nickname AS sender_nickname, u.tag AS sender_tag
		 FROM friendships f
		 JOIN users u ON f.user_id = u.id
		 WHERE f.friend_id = $1 AND f.status = 'pending'
		 ORDER BY f.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list incoming requests: %w", err)
	}
	defer rows.Close()
	var requests []Friendship
	for rows.Next() {
		var f Friendship
		if err := rows.Scan(&f.ID, &f.UserID, &f.FriendID, &f.Status, &f.CreatedAt, &f.UpdatedAt,
			&f.SenderNickname, &f.SenderTag); err != nil {
			return nil, fmt.Errorf("scan request: %w", err)
		}
		requests = append(requests, f)
	}
	return requests, rows.Err()
}

func (db *DB) ListOutgoingRequests(ctx context.Context, userID uuid.UUID) ([]Friendship, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT f.id, f.user_id, f.friend_id, f.status, f.created_at, f.updated_at,
		        u.nickname AS receiver_nickname, u.tag AS receiver_tag
		 FROM friendships f
		 JOIN users u ON f.friend_id = u.id
		 WHERE f.user_id = $1 AND f.status = 'pending'
		 ORDER BY f.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list outgoing requests: %w", err)
	}
	defer rows.Close()
	var requests []Friendship
	for rows.Next() {
		var f Friendship
		if err := rows.Scan(&f.ID, &f.UserID, &f.FriendID, &f.Status, &f.CreatedAt, &f.UpdatedAt,
			&f.ReceiverNickname, &f.ReceiverTag); err != nil {
			return nil, fmt.Errorf("scan request: %w", err)
		}
		requests = append(requests, f)
	}
	return requests, rows.Err()
}
```

- [ ] **Step 9: Update Friendship type with sender/receiver nickname fields**

Add to the Friendship struct:

```go
type Friendship struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	FriendID         uuid.UUID
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	SenderNickname   string
	SenderTag        string
	ReceiverNickname string
	ReceiverTag      string
}
```

- [ ] **Step 10: Add daily request counter methods**

```go
func (db *DB) GetOutgoingRequestCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	var resetAt time.Time
	err := db.pool.QueryRow(ctx,
		`SELECT outgoing_requests_today, requests_reset_at FROM users WHERE id = $1`, userID,
	).Scan(&count, &resetAt)
	if err != nil {
		return 0, fmt.Errorf("get request count: %w", err)
	}
	if resetAt.IsZero() || time.Now().After(resetAt) {
		return 0, nil
	}
	return count, nil
}

func (db *DB) IncrementOutgoingRequestCount(ctx context.Context, userID uuid.UUID) error {
	_, err := db.pool.Exec(ctx,
		`UPDATE users SET
		   outgoing_requests_today = CASE WHEN requests_reset_at IS NULL OR requests_reset_at < now()
		     THEN 1 ELSE outgoing_requests_today + 1 END,
		   requests_reset_at = CASE WHEN requests_reset_at IS NULL OR requests_reset_at < now()
		     THEN now() + interval '24 hours' ELSE requests_reset_at END
		 WHERE id = $1`, userID)
	return err
}
```

- [ ] **Step 11: Add GetAllowFriendRequests method**

```go
func (db *DB) GetAllowFriendRequests(ctx context.Context, userID uuid.UUID) (bool, error) {
	var allow bool
	err := db.pool.QueryRow(ctx,
		`SELECT allow_friend_requests FROM users WHERE id = $1`, userID,
	).Scan(&allow)
	if err != nil {
		return false, fmt.Errorf("get allow friend requests: %w", err)
	}
	return allow, nil
}
```

- [ ] **Step 12: Build check**

```bash
cd /Users/n.shchugorev/zvonilka/backend/users && go build ./...
```

- [ ] **Step 13: Commit**

```bash
git add internal/repo/repo.go
git commit -m "feat(users): add friendship repo methods"
```

---

### Task 3: Service layer — friends business logic

**Files:**
- Create: `backend/users/internal/service/friends.go`

- [ ] **Step 1: Create service file with sentinel errors**

Write to `backend/users/internal/service/friends.go`:

```go
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/zvonilkaRU/users/internal/repo"
)

var (
	ErrAlreadyFriends    = errors.New("already friends")
	ErrRequestExists     = errors.New("friend request already exists")
	ErrDailyLimitReached = errors.New("daily friend request limit reached")
	ErrRequestsBlocked   = errors.New("user is not accepting friend requests")
	ErrCannotSendToSelf  = errors.New("cannot send friend request to yourself")
	ErrNotReceiver       = errors.New("only the receiver can perform this action")
	ErrNotSender         = errors.New("only the sender can perform this action")
)
```

- [ ] **Step 2: Add SendRequest method**

```go
func (s *UsersService) SendFriendRequest(ctx context.Context, senderID uuid.UUID, receiverTag string) (*repo.Friendship, error) {
	receiver, err := s.db.GetUserByTag(ctx, receiverTag)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find receiver: %w", err)
	}
	if receiver.ID == senderID {
		return nil, ErrCannotSendToSelf
	}

	// Check if already friends or request pending
	existing, _ := s.db.GetFriendship(ctx, senderID, receiver.ID)
	if existing != nil {
		if existing.Status == "accepted" {
			return nil, ErrAlreadyFriends
		}
		return nil, ErrRequestExists
	}
	existing, _ = s.db.GetFriendship(ctx, receiver.ID, senderID)
	if existing != nil {
		if existing.Status == "accepted" {
			return nil, ErrAlreadyFriends
		}
		return nil, ErrRequestExists
	}

	// Check receiver settings
	allow, err := s.db.GetAllowFriendRequests(ctx, receiver.ID)
	if err != nil {
		return nil, fmt.Errorf("check settings: %w", err)
	}
	if !allow {
		return nil, ErrRequestsBlocked
	}

	// Daily limit check
	count, err := s.db.GetOutgoingRequestCount(ctx, senderID)
	if err != nil {
		return nil, fmt.Errorf("check daily limit: %w", err)
	}
	const maxRequestsPerDay = 100
	if count >= maxRequestsPerDay {
		return nil, ErrDailyLimitReached
	}

	// Create the request
	f, err := s.db.CreateFriendRequest(ctx, senderID, receiver.ID)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Increment counter
	_ = s.db.IncrementOutgoingRequestCount(ctx, senderID)

	return f, nil
}
```

- [ ] **Step 3: Add AcceptRequest method**

```go
func (s *UsersService) AcceptFriendRequest(ctx context.Context, requestID, userID uuid.UUID) (*repo.Friendship, error) {
	f, err := s.db.UpdateFriendshipStatus(ctx, requestID, userID, "accepted")
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotReceiver
	}
	if err != nil {
		return nil, fmt.Errorf("accept request: %w", err)
	}
	return f, nil
}
```

- [ ] **Step 4: Add DeclineRequest method**

```go
func (s *UsersService) DeclineFriendRequest(ctx context.Context, requestID, userID uuid.UUID) error {
	err := s.db.DeleteFriendship(ctx, requestID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotReceiver
	}
	return err
}
```

- [ ] **Step 5: Add RemoveFriend, CancelRequest, List methods**

```go
func (s *UsersService) RemoveFriend(ctx context.Context, friendshipID, userID uuid.UUID) error {
	return s.db.DeleteFriendship(ctx, friendshipID, userID)
}

func (s *UsersService) CancelFriendRequest(ctx context.Context, requestID, userID uuid.UUID) error {
	err := s.db.DeleteFriendship(ctx, requestID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotSender
	}
	return err
}

func (s *UsersService) ListFriends(ctx context.Context, userID uuid.UUID) ([]repo.UserRef, error) {
	return s.db.ListFriends(ctx, userID)
}

func (s *UsersService) ListIncomingRequests(ctx context.Context, userID uuid.UUID) ([]repo.Friendship, error) {
	return s.db.ListIncomingRequests(ctx, userID)
}

func (s *UsersService) ListOutgoingRequests(ctx context.Context, userID uuid.UUID) ([]repo.Friendship, error) {
	return s.db.ListOutgoingRequests(ctx, userID)
}
```

- [ ] **Step 6: Build check**

```bash
cd /Users/n.shchugorev/zvonilka/backend/users && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add internal/service/friends.go
git commit -m "feat(users): add friends service layer"
```

---

### Task 4: HTTP handlers — wire friends API

**Files:**
- Modify: `backend/users/internal/service/users_server.go` (add handler methods)
- Modify: `backend/api-schema/zvonilkaRU/users/src/openapi/openapi.yaml` (add friends paths)
- Create: `backend/api-schema/zvonilkaRU/users/src/openapi/resources/v1/friends/requests.yaml`
- Create: `backend/api-schema/zvonilkaRU/users/src/openapi/resources/v1/friends/friend.yaml`
- Create: `backend/api-schema/zvonilkaRU/users/src/openapi/schemas/friends/FriendRequest.yaml`
- Create: `backend/api-schema/zvonilkaRU/users/src/openapi/schemas/models/UserRef.yaml` (if missing)

- [ ] **Step 1: Add handler methods to users_server.go**

Append to `backend/users/internal/service/users_server.go`:

```go
// --- Friends ---

func (s *Server) SendFriendRequest(ctx context.Context, req *apiclient.SendFriendRequestRequest) (*apiclient.SendFriendRequestResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return &apiclient.SendFriendRequestResponse{Code: 401}, nil
	}
	f, err := s.svc.SendFriendRequest(ctx, userID, req.Body.Tag)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return &apiclient.SendFriendRequestResponse{Code: 404}, nil
		}
		if errors.Is(err, ErrAlreadyFriends) || errors.Is(err, ErrRequestExists) {
			return &apiclient.SendFriendRequestResponse{Code: 409}, nil
		}
		if errors.Is(err, ErrRequestsBlocked) {
			return &apiclient.SendFriendRequestResponse{Code: 403}, nil
		}
		if errors.Is(err, ErrDailyLimitReached) {
			return &apiclient.SendFriendRequestResponse{Code: 429}, nil
		}
		if errors.Is(err, ErrCannotSendToSelf) {
			return &apiclient.SendFriendRequestResponse{Code: 422}, nil
		}
		return nil, err
	}
	resp := any(toFriendRequestResponse(f))
	return &apiclient.SendFriendRequestResponse{Code: 201, Response201: &resp}, nil
}

func (s *Server) ListIncomingRequests(ctx context.Context, _ *apiclient.ListIncomingRequestsRequest) (*apiclient.ListIncomingRequestsResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return &apiclient.ListIncomingRequestsResponse{Code: 401}, nil
	}
	requests, err := s.svc.ListIncomingRequests(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]any, len(requests))
	for i, r := range requests {
		items[i] = toFriendRequestResponse(&r)
	}
	resp := any(items)
	return &apiclient.ListIncomingRequestsResponse{Code: 200, Response200: &resp}, nil
}

func (s *Server) AcceptFriendRequest(ctx context.Context, req *apiclient.AcceptFriendRequestRequest) (*apiclient.AcceptFriendRequestResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return &apiclient.AcceptFriendRequestResponse{Code: 401}, nil
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		return &apiclient.AcceptFriendRequestResponse{Code: 422}, nil
	}
	f, err := s.svc.AcceptFriendRequest(ctx, id, userID)
	if err != nil {
		if errors.Is(err, ErrNotReceiver) {
			return &apiclient.AcceptFriendRequestResponse{Code: 403}, nil
		}
		return nil, err
	}
	resp := any(toFriendRequestResponse(f))
	return &apiclient.AcceptFriendRequestResponse{Code: 200, Response200: &resp}, nil
}

func (s *Server) DeclineFriendRequest(ctx context.Context, req *apiclient.DeclineFriendRequestRequest) (*apiclient.DeclineFriendRequestResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return &apiclient.DeclineFriendRequestResponse{Code: 401}, nil
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		return &apiclient.DeclineFriendRequestResponse{Code: 422}, nil
	}
	if err := s.svc.DeclineFriendRequest(ctx, id, userID); err != nil {
		if errors.Is(err, ErrNotReceiver) {
			return &apiclient.DeclineFriendRequestResponse{Code: 403}, nil
		}
		return nil, err
	}
	return &apiclient.DeclineFriendRequestResponse{Code: 204, Response204: true}, nil
}

func (s *Server) ListFriends(ctx context.Context, _ *apiclient.ListFriendsRequest) (*apiclient.ListFriendsResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return &apiclient.ListFriendsResponse{Code: 401}, nil
	}
	friends, err := s.svc.ListFriends(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]any, len(friends))
	for i, f := range friends {
		items[i] = map[string]string{"id": f.ID.String(), "nickname": f.Nickname, "tag": f.Tag}
	}
	resp := any(items)
	return &apiclient.ListFriendsResponse{Code: 200, Response200: &resp}, nil
}

func (s *Server) RemoveFriend(ctx context.Context, req *apiclient.RemoveFriendRequest) (*apiclient.RemoveFriendResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return &apiclient.RemoveFriendResponse{Code: 401}, nil
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		return &apiclient.RemoveFriendResponse{Code: 422}, nil
	}
	if err := s.svc.RemoveFriend(ctx, id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &apiclient.RemoveFriendResponse{Code: 404}, nil
		}
		return nil, err
	}
	return &apiclient.RemoveFriendResponse{Code: 204, Response204: true}, nil
}

func (s *Server) ListOutgoingRequests(ctx context.Context, _ *apiclient.ListOutgoingRequestsRequest) (*apiclient.ListOutgoingRequestsResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return &apiclient.ListOutgoingRequestsResponse{Code: 401}, nil
	}
	requests, err := s.svc.ListOutgoingRequests(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]any, len(requests))
	for i, r := range requests {
		items[i] = toFriendRequestResponse(&r)
	}
	resp := any(items)
	return &apiclient.ListOutgoingRequestsResponse{Code: 200, Response200: &resp}, nil
}

func (s *Server) CancelFriendRequest(ctx context.Context, req *apiclient.CancelFriendRequestRequest) (*apiclient.CancelFriendRequestResponse, error) {
	userID, err := userIDFromContext(ctx)
	if err != nil || userID == uuid.Nil {
		return &apiclient.CancelFriendRequestResponse{Code: 401}, nil
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		return &apiclient.CancelFriendRequestResponse{Code: 422}, nil
	}
	if err := s.svc.CancelFriendRequest(ctx, id, userID); err != nil {
		if errors.Is(err, ErrNotSender) {
			return &apiclient.CancelFriendRequestResponse{Code: 403}, nil
		}
		return nil, err
	}
	return &apiclient.CancelFriendRequestResponse{Code: 204, Response204: true}, nil
}

func toFriendRequestResponse(f *repo.Friendship) map[string]any {
	return map[string]any{
		"id":        f.ID.String(),
		"userId":    f.UserID.String(),
		"friendId":  f.FriendID.String(),
		"status":    f.Status,
		"createdAt": f.CreatedAt,
	}
}
```

- [ ] **Step 2: Add missing imports**

Add `"database/sql"` to imports in users_server.go if not already present.

- [ ] **Step 3: Add `GetUserByTag` to repo if missing**

In repo.go — add if not already present:

```go
func (db *DB) GetUserByTag(ctx context.Context, tag string) (*User, error) {
	var u User
	err := db.pool.QueryRow(ctx,
		`SELECT id, login, email, nickname, tag, password_hash, status, email_verified, created_at, updated_at
		 FROM users WHERE tag = $1`, strings.ToUpper(tag),
	).Scan(&u.ID, &u.Login, &u.Email, &u.Nickname, &u.Tag, &u.PasswordHash, &u.Status, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("get user by tag: %w", err)
	}
	return &u, nil
}
```

- [ ] **Step 4: Build check**

```bash
cd /Users/n.shchugorev/zvonilka/backend/users && go build ./...
```

Expected: may fail if generated API types are missing. If so, create them in Task 5.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(users): add friends HTTP handlers"
```

---

### Task 5: OpenAPI specs + code generation

**Files:**
- Create: `backend/api-schema/zvonilkaRU/users/src/openapi/resources/v1/friends/requests.yaml`
- Create: `backend/api-schema/zvonilkaRU/users/src/openapi/resources/v1/friends/friend.yaml`
- Create: `backend/api-schema/zvonilkaRU/users/src/openapi/resources/v1/friends/outgoing.yaml`
- Modify: `backend/api-schema/zvonilkaRU/users/src/openapi/openapi.yaml` (add friends paths)

- [ ] **Step 1: Check existing OpenAPI patterns**

Read `/Users/n.shchugorev/zvonilka/backend/api-schema/zvonilkaRU/users/src/openapi/resources/v1/auth/register.yaml` to understand the endpoint schema format.

- [ ] **Step 2: Follow existing oapigen workflow**

Since this project uses oapigen (not plain OpenAPI), follow the existing pattern:
1. Add schemas to `schemas/friends/`
2. Add resource endpoints to `resources/v1/friends/`
3. Register paths in `openapi.yaml`
4. Run `oapigen generate` (ask user for the exact command)

- [ ] **Step 3: Commit specs**

```bash
git add -A
git commit -m "feat(api-schema): add friends API specs"
```

---

### Task 6: Verify, test, finalize

- [ ] **Step 1: Full build**

```bash
cd /Users/n.shchugorev/zvonilka/backend/users && go build ./... && cd /Users/n.shchugorev/zvonilka/backend/api-schema && go build ./...
```

- [ ] **Step 2: Run existing tests**

```bash
cd /Users/n.shchugorev/zvonilka/backend/users && go test ./internal/... -count=1
```

Expected: all existing tests pass. Friends handlers don't have dedicated tests yet (no test DB).

- [ ] **Step 3: Commit final**

```bash
git add -A && git commit -m "feat(users): friends subsystem complete"
```
