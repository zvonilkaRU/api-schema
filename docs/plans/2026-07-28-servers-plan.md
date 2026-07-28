# Servers + Channels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** New `servers` service with Discord-like servers, persistent channel templates, ephemeral LiveKit voice rooms. Owner/admin/member roles. Channels activate on first join, deactivate on last leave.

**Architecture:** New Go service at `backend/servers/` with its own Echo server, JWT auth, PostgreSQL repo. Reuses api-schema code generation pattern. Channels room lifecycle managed through existing WebSocket hub (imported from rooms service or duplicated).

**Tech Stack:** Go 1.24, pgx/v5, Echo v4, PostgreSQL, OpenAPI 3.1 (oapigen), LiveKit server SDK.

**Spec:** `docs/specs/2026-07-28-zvonilka-v2-design.md` § Phase 2

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `backend/servers/main.go` | Create | Echo server, middleware, signal handling |
| `backend/servers/go.mod` | Create | Module definition |
| `backend/servers/internal/config/config.go` | Create | Env-based config |
| `backend/servers/internal/repo/repo.go` | Create | PostgreSQL queries |
| `backend/servers/internal/service/server.go` | Create | HTTP handler + Server struct |
| `backend/servers/internal/ws/hub.go` | Create | WebSocket hub for channels (or import from rooms) |
| `backend/servers/Dockerfile.ctx` | Create | Docker build |
| `backend/servers/Makefile` | Create | Build + deploy targets |
| `backend/servers/migrations/001_servers.sql` | Create | Servers + members + channels tables |
| `backend/servers/migrations/002_rooms_type.sql` | Create | Add type column to rooms |
| `backend/api-schema/zvonilkaRU/servers/` | Create | OpenAPI specs for servers API |
| `infra/k8s/base/backend/servers/` | Create | K8s deployment + service manifests |

---

### Task 1: DB migrations

**Files:**
- Create: `backend/servers/migrations/001_servers.sql`

- [ ] **Step 1: Create servers migration**

Write to `backend/servers/migrations/001_servers.sql`:

```sql
-- +goose Up
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
    role        TEXT NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY(server_id, user_id)
);

CREATE TABLE channels (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id   UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'voice',
    position    INT NOT NULL DEFAULT 0,
    created_by  UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(server_id, name)
);

CREATE INDEX idx_server_members_user ON server_members(user_id);
CREATE INDEX idx_channels_server_pos ON channels(server_id, position);

-- +goose Down
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS server_members;
DROP TABLE IF EXISTS servers;
```

- [ ] **Step 2: Create rooms type migration**

Write to `backend/servers/migrations/002_rooms_type.sql`:

```sql
-- +goose Up
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS type TEXT NOT NULL DEFAULT 'public';

-- +goose Down
ALTER TABLE rooms DROP COLUMN IF EXISTS type;
```

- [ ] **Step 3: Commit**

```bash
git add backend/servers/migrations/
git commit -m "feat(servers): servers + members + channels migrations"
```

---

### Task 2: New service scaffold

**Files:**
- Create: `backend/servers/go.mod`
- Create: `backend/servers/main.go`
- Create: `backend/servers/internal/config/config.go`
- Create: `backend/servers/.gitignore`
- Create: `backend/servers/Dockerfile.ctx`
- Create: `backend/servers/Makefile`
- Create: `backend/servers/.dockerignore`

- [ ] **Step 1: Initialize go module**

```bash
cd /Users/n.shchugorev/zvonilka/backend/servers
go mod init github.com/zvonilkaRU/servers
```

Edit go.mod to add `replace` directive:
```
replace github.com/zvonilkaRU/api-schema => ../api-schema
```

- [ ] **Step 2: Create config**

Write to `backend/servers/internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	ServerPort     string `env:"SERVER_PORT" envDefault:"8080"`
	DBHost         string `env:"DB_HOST" envDefault:"postgresql.backend.svc.cluster.local"`
	DBPort         string `env:"DB_PORT" envDefault:"5432"`
	DBUser         string `env:"DB_USER" envDefault:"servers"`
	DBPassword     string `env:"DB_PASSWORD,required"`
	DBName         string `env:"DB_NAME" envDefault:"servers"`
	DBSSLMode      string `env:"DB_SSLMODE" envDefault:"require"`
	UsersJWKSURL   string `env:"USERS_JWKS_URL" envDefault:"http://users.backend.svc.cluster.local:8080/users/v1/.well-known/jwks.json"`
	LiveKitHost    string `env:"LIVEKIT_HOST,required"`
	LiveKitAPIKey  string `env:"LIVEKIT_API_KEY,required"`
	LiveKitAPISecret string `env:"LIVEKIT_API_SECRET,required"`
	AllowedOrigins string `env:"ALLOWED_ORIGINS" envDefault:"https://zvonilka.space,https://app.zvonilka.space"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s&search_path=servers",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode)
}
```

- [ ] **Step 3: Create main.go**

Write to `backend/servers/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	echoserver "github.com/zvonilkaRU/api-schema/generated/servers/impl/echoserver"
	"github.com/zvonilkaRU/api-schema/pkg/auth"
	"github.com/zvonilkaRU/api-schema/pkg/metrics"

	"github.com/zvonilkaRU/servers/internal/config"
	"github.com/zvonilkaRU/servers/internal/repo"
	"github.com/zvonilkaRU/servers/internal/service"
)

var log = slog.Default()

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := repo.New(ctx, cfg.DSN())
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	jv, err := auth.NewVerifier(ctx, cfg.UsersJWKSURL)
	if err != nil {
		log.Error("jwt", "err", err)
		os.Exit(1)
	}

	srv := service.NewServer(db, cfg.LiveKitHost, cfg.LiveKitAPIKey, cfg.LiveKitAPISecret)

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Server.ReadTimeout = 10 * time.Second
	e.Server.WriteTimeout = 30 * time.Second
	e.Server.IdleTimeout = 60 * time.Second
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.BodyLimit("1M"))

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: parseOrigins(cfg.AllowedOrigins),
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

	m := metrics.New()
	e.Use(m.Middleware())
	e.GET("/servers/v1/metrics", m.Handler)

	e.Use(auth.Middleware(jv, auth.PublicPaths{
		"/servers/v1/health":  true,
		"/servers/v1/metrics": true,
	}))

	echoserver.NewServerHTTP(srv).Register(e)

	serverErr := make(chan error, 1)
	go func() {
		addr := fmt.Sprintf(":%s", cfg.ServerPort)
		log.Info("listening", "addr", addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
	case err := <-serverErr:
		log.Error("server", "err", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
}

func parseOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}
```

- [ ] **Step 4: Create Dockerfile, Makefile, .gitignore**

`backend/servers/Dockerfile.ctx` — copy pattern from `backend/rooms/Dockerfile.ctx`:
```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY api-schema/go.mod api-schema/go.sum api-schema/
COPY servers/go.mod servers/go.sum servers/
RUN cd servers && GOPRIVATE=github.com/zvonilkaRU go mod download
COPY api-schema/ api-schema/
COPY servers/ servers/
RUN cd servers && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /servers .
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
USER 1000:1000
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:8080/servers/v1/health || exit 1
ENTRYPOINT ["/servers"]
```

`backend/servers/Makefile` — copy from `backend/rooms/Makefile`, replace paths.
`backend/servers/.gitignore` — `servers` binary, `.env`.
`backend/servers/.dockerignore` — copy from rooms.

- [ ] **Step 5: Commit**

```bash
git add backend/servers/
git commit -m "feat(servers): new service scaffold"
```

---

### Task 3: Repo layer

**Files:**
- Create: `backend/servers/internal/repo/repo.go`

- [ ] **Step 1: Create repo.go**

Write to `backend/servers/internal/repo/repo.go`:

```go
package repo

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct{ pool *pgxpool.Pool }

func New(ctx context.Context, dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DSN: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 1 * time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET statement_timeout = '10s'")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{pool}, nil
}

func (db *DB) Close() { db.pool.Close() }
func (db *DB) Ping(ctx context.Context) error { return db.pool.Ping(ctx) }

// --- Types ---
type Server struct {
	ID        uuid.UUID
	Name      string
	IconURL   string
	OwnerID   uuid.UUID
	CreatedAt time.Time
}

type ServerMember struct {
	ServerID uuid.UUID
	UserID   uuid.UUID
	Nickname string
	Tag      string
	Role     string
	JoinedAt time.Time
}

type Channel struct {
	ID        uuid.UUID
	ServerID  uuid.UUID
	Name      string
	Type      string
	Position  int
	CreatedBy uuid.UUID
	CreatedAt time.Time
}
```

- [ ] **Step 2: Add Server CRUD**

```go
func (db *DB) CreateServer(ctx context.Context, name, iconURL string, ownerID uuid.UUID) (*Server, error) {
	var s Server
	err := db.pool.QueryRow(ctx,
		`INSERT INTO servers (name, icon_url, owner_id) VALUES ($1, $2, $3)
		 RETURNING id, name, COALESCE(icon_url,''), owner_id, created_at`,
		name, iconURL, ownerID,
	).Scan(&s.ID, &s.Name, &s.IconURL, &s.OwnerID, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create server: %w", err)
	}
	return &s, nil
}

func (db *DB) GetServer(ctx context.Context, id uuid.UUID) (*Server, error) {
	var s Server
	err := db.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(icon_url,''), owner_id, created_at FROM servers WHERE id = $1`, id,
	).Scan(&s.ID, &s.Name, &s.IconURL, &s.OwnerID, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("get server: %w", err)
	}
	return &s, nil
}

func (db *DB) UpdateServer(ctx context.Context, id uuid.UUID, name, iconURL *string) (*Server, error) {
	var s Server
	err := db.pool.QueryRow(ctx,
		`UPDATE servers SET name = COALESCE($1, name), icon_url = COALESCE($2, icon_url)
		 WHERE id = $3
		 RETURNING id, name, COALESCE(icon_url,''), owner_id, created_at`,
		name, iconURL, id,
	).Scan(&s.ID, &s.Name, &s.IconURL, &s.OwnerID, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("update server: %w", err)
	}
	return &s, nil
}

func (db *DB) DeleteServer(ctx context.Context, id uuid.UUID) error {
	tag, err := db.pool.Exec(ctx, `DELETE FROM servers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete server: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) ListUserServers(ctx context.Context, userID uuid.UUID) ([]Server, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT s.id, s.name, COALESCE(s.icon_url,''), s.owner_id, s.created_at
		 FROM servers s JOIN server_members sm ON s.id = sm.server_id
		 WHERE sm.user_id = $1 ORDER BY s.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.IconURL, &s.OwnerID, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan server: %w", err)
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}
```

- [ ] **Step 3: Add member management methods**

```go
func (db *DB) AddMember(ctx context.Context, serverID, userID uuid.UUID, role string) error {
	_, err := db.pool.Exec(ctx,
		`INSERT INTO server_members (server_id, user_id, role) VALUES ($1, $2, $3)`, serverID, userID, role)
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

func (db *DB) GetMember(ctx context.Context, serverID, userID uuid.UUID) (*ServerMember, error) {
	var m ServerMember
	err := db.pool.QueryRow(ctx,
		`SELECT sm.server_id, sm.user_id, u.nickname, u.tag, sm.role, sm.joined_at
		 FROM server_members sm JOIN users u ON sm.user_id = u.id
		 WHERE sm.server_id = $1 AND sm.user_id = $2`, serverID, userID,
	).Scan(&m.ServerID, &m.UserID, &m.Nickname, &m.Tag, &m.Role, &m.JoinedAt)
	if err == pgx.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("get member: %w", err)
	}
	return &m, nil
}

func (db *DB) ListMembers(ctx context.Context, serverID uuid.UUID) ([]ServerMember, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT sm.server_id, sm.user_id, u.nickname, u.tag, sm.role, sm.joined_at
		 FROM server_members sm JOIN users u ON sm.user_id = u.id
		 WHERE sm.server_id = $1 ORDER BY sm.role, u.nickname`, serverID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	var members []ServerMember
	for rows.Next() {
		var m ServerMember
		if err := rows.Scan(&m.ServerID, &m.UserID, &m.Nickname, &m.Tag, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (db *DB) UpdateMemberRole(ctx context.Context, serverID, userID uuid.UUID, role string) error {
	tag, err := db.pool.Exec(ctx,
		`UPDATE server_members SET role = $1 WHERE server_id = $2 AND user_id = $3`,
		role, serverID, userID)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) RemoveMember(ctx context.Context, serverID, userID uuid.UUID) error {
	tag, err := db.pool.Exec(ctx,
		`DELETE FROM server_members WHERE server_id = $1 AND user_id = $2`, serverID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (db *DB) TransferOwnership(ctx context.Context, serverID, fromUserID, toUserID uuid.UUID) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE servers SET owner_id = $1 WHERE id = $2 AND owner_id = $3`, toUserID, serverID, fromUserID)
	if err != nil {
		return fmt.Errorf("transfer server: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE server_members SET role = 'admin' WHERE server_id = $1 AND user_id = $2`, serverID, fromUserID)
	if err != nil {
		return fmt.Errorf("demote old owner: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE server_members SET role = 'owner' WHERE server_id = $1 AND user_id = $2`, serverID, toUserID)
	if err != nil {
		return fmt.Errorf("promote new owner: %w", err)
	}
	return tx.Commit(ctx)
}
```

- [ ] **Step 4: Add channel methods**

```go
func (db *DB) CreateChannel(ctx context.Context, serverID, createdBy uuid.UUID, name string) (*Channel, error) {
	var ch Channel
	var maxPos int
	_ = db.pool.QueryRow(ctx, `SELECT COALESCE(MAX(position), -1) FROM channels WHERE server_id = $1`, serverID).Scan(&maxPos)
	err := db.pool.QueryRow(ctx,
		`INSERT INTO channels (server_id, name, type, position, created_by)
		 VALUES ($1, $2, 'voice', $3, $4)
		 RETURNING id, server_id, name, type, position, created_by, created_at`,
		serverID, name, maxPos+1, createdBy,
	).Scan(&ch.ID, &ch.ServerID, &ch.Name, &ch.Type, &ch.Position, &ch.CreatedBy, &ch.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}
	return &ch, nil
}

func (db *DB) GetChannel(ctx context.Context, id uuid.UUID) (*Channel, error) {
	var ch Channel
	err := db.pool.QueryRow(ctx,
		`SELECT id, server_id, name, type, position, created_by, created_at FROM channels WHERE id = $1`, id,
	).Scan(&ch.ID, &ch.ServerID, &ch.Name, &ch.Type, &ch.Position, &ch.CreatedBy, &ch.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	return &ch, nil
}

func (db *DB) ListChannels(ctx context.Context, serverID uuid.UUID) ([]Channel, error) {
	rows, err := db.pool.Query(ctx,
		`SELECT id, server_id, name, type, position, created_by, created_at
		 FROM channels WHERE server_id = $1 ORDER BY position`, serverID)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()
	var channels []Channel
	for rows.Next() {
		var ch Channel
		if err := rows.Scan(&ch.ID, &ch.ServerID, &ch.Name, &ch.Type, &ch.Position, &ch.CreatedBy, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

func (db *DB) UpdateChannel(ctx context.Context, id uuid.UUID, name *string, position *int) (*Channel, error) {
	var ch Channel
	err := db.pool.QueryRow(ctx,
		`UPDATE channels SET name = COALESCE($1, name), position = COALESCE($2, position)
		 WHERE id = $3
		 RETURNING id, server_id, name, type, position, created_by, created_at`,
		name, position, id,
	).Scan(&ch.ID, &ch.ServerID, &ch.Name, &ch.Type, &ch.Position, &ch.CreatedBy, &ch.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
	}
	return &ch, nil
}

func (db *DB) DeleteChannel(ctx context.Context, id uuid.UUID) error {
	tag, err := db.pool.Exec(ctx, `DELETE FROM channels WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sql.ErrNoRows
	}
	return nil
}
```

- [ ] **Step 5: Build check**

```bash
cd /Users/n.shchugorev/zvonilka/backend/servers && go mod tidy && go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/repo/repo.go go.mod go.sum
git commit -m "feat(servers): repo layer — servers, members, channels CRUD"
```

---

### Task 4: Service layer — HTTP handlers

**Files:**
- Create: `backend/servers/internal/service/server.go`

- [ ] **Step 1: Create server.go with all handlers**

Write to `backend/servers/internal/service/server.go`:

```go
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/zvonilkaRU/api-schema/pkg/auth"
	"github.com/zvonilkaRU/servers/internal/repo"
)

var (
	ErrNotMember    = errors.New("not a member of this server")
	ErrNotAdmin     = errors.New("admin role required")
	ErrNotOwner     = errors.New("owner role required")
	ErrCannotDemoteOwner = errors.New("cannot demote the server owner")
	ErrCannotKickOwner   = errors.New("cannot kick the server owner")
	ErrCannotKickAdmin   = errors.New("only the owner can kick admins")
)

type Server struct {
	db       *repo.DB
	lkHost   string
	lkKey    string
	lkSecret string
}

func NewServer(db *repo.DB, lkHost, lkKey, lkSecret string) *Server {
	return &Server{db: db, lkHost: lkHost, lkKey: lkKey, lkSecret: lkSecret}
}
```

- [ ] **Step 2: Add Server CRUD handlers**

After the Server struct definition:

```go
func (s *Server) CreateServer(ctx context.Context, req *apiclient.CreateServerRequest) (*apiclient.CreateServerResponse, error) {
	uid := auth.UserID(ctx)
	userID, _ := uuid.Parse(uid)
	server, err := s.db.CreateServer(ctx, req.Body.Name, req.Body.IconURL, userID)
	if err != nil {
		return nil, fmt.Errorf("create server: %w", err)
	}
	_ = s.db.AddMember(ctx, server.ID, userID, "owner")
	return &apiclient.CreateServerResponse{
		Code: 201,
		Response201: &models.ServerResponse{
			ID: server.ID.String(), Name: server.Name, IconURL: server.IconURL,
			OwnerID: server.OwnerID.String(), CreatedAt: server.CreatedAt,
		},
	}, nil
}

func (s *Server) GetServer(ctx context.Context, req *apiclient.GetServerRequest) (*apiclient.GetServerResponse, error) {
	id, _ := uuid.Parse(req.ID)
	server, err := s.db.GetServer(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return &apiclient.GetServerResponse{Code: 404}, nil
	}
	if err != nil {
		return nil, err
	}
	channels, _ := s.db.ListChannels(ctx, id)
	channelItems := make([]any, len(channels))
	for i, ch := range channels {
		channelItems[i] = map[string]any{
			"id": ch.ID.String(), "name": ch.Name, "type": ch.Type,
			"position": ch.Position, "active": false, "participants": 0,
		}
	}
	resp := any(map[string]any{
		"id": server.ID.String(), "name": server.Name, "iconUrl": server.IconURL,
		"ownerId": server.OwnerID.String(), "createdAt": server.CreatedAt,
		"channels": channelItems,
	})
	return &apiclient.GetServerResponse{Code: 200, Response200: &resp}, nil
}

func (s *Server) ListServers(ctx context.Context, _ *apiclient.ListServersRequest) (*apiclient.ListServersResponse, error) {
	uid := auth.UserID(ctx)
	userID, _ := uuid.Parse(uid)
	serversList, err := s.db.ListUserServers(ctx, userID)
	if err != nil {
		return nil, err
	}
	items := make([]any, len(serversList))
	for i, sv := range serversList {
		items[i] = map[string]any{
			"id": sv.ID.String(), "name": sv.Name, "iconUrl": sv.IconURL,
			"ownerId": sv.OwnerID.String(), "createdAt": sv.CreatedAt,
		}
	}
	resp := any(items)
	return &apiclient.ListServersResponse{Code: 200, Response200: &resp}, nil
}
```

- [ ] **Step 3: Add member management handlers**

```go
func (s *Server) ListMembers(ctx context.Context, req *apiclient.ListMembersRequest) (*apiclient.ListMembersResponse, error) {
	id, _ := uuid.Parse(req.ID)
	members, err := s.db.ListMembers(ctx, id)
	if err != nil {
		return nil, err
	}
	items := make([]any, len(members))
	for i, m := range members {
		items[i] = map[string]any{
			"userId": m.UserID.String(), "nickname": m.Nickname, "tag": m.Tag,
			"role": m.Role, "joinedAt": m.JoinedAt,
		}
	}
	resp := any(items)
	return &apiclient.ListMembersResponse{Code: 200, Response200: &resp}, nil
}

func (s *Server) UpdateMember(ctx context.Context, req *apiclient.UpdateMemberRequest) (*apiclient.UpdateMemberResponse, error) {
	serverID, _ := uuid.Parse(req.ServerID)
	targetID, _ := uuid.Parse(req.UserID)
	uid := auth.UserID(ctx)
	actorID, _ := uuid.Parse(uid)

	actor, _ := s.db.GetMember(ctx, serverID, actorID)
	target, _ := s.db.GetMember(ctx, serverID, targetID)

	if actor == nil || target == nil {
		return &apiclient.UpdateMemberResponse{Code: 404}, nil
	}
	if actor.Role != "owner" && actor.Role != "admin" {
		return &apiclient.UpdateMemberResponse{Code: 403}, nil
	}
	if target.Role == "owner" {
		return &apiclient.UpdateMemberResponse{Code: 403}, nil
	}
	// Only owner can demote admins
	if target.Role == "admin" && actor.Role != "owner" {
		return &apiclient.UpdateMemberResponse{Code: 403}, nil
	}
	if err := s.db.UpdateMemberRole(ctx, serverID, targetID, req.Body.Role); err != nil {
		return nil, err
	}
	return &apiclient.UpdateMemberResponse{Code: 204, Response204: true}, nil
}

func (s *Server) KickMember(ctx context.Context, req *apiclient.KickMemberRequest) (*apiclient.KickMemberResponse, error) {
	serverID, _ := uuid.Parse(req.ServerID)
	targetID, _ := uuid.Parse(req.UserID)
	uid := auth.UserID(ctx)
	actorID, _ := uuid.Parse(uid)

	actor, _ := s.db.GetMember(ctx, serverID, actorID)
	target, _ := s.db.GetMember(ctx, serverID, targetID)

	if target == nil {
		return &apiclient.KickMemberResponse{Code: 404}, nil
	}
	if actor == nil || (actor.Role != "owner" && actor.Role != "admin") {
		return &apiclient.KickMemberResponse{Code: 403}, nil
	}
	if target.Role == "owner" {
		return &apiclient.KickMemberResponse{Code: 403}, nil
	}
	if target.Role == "admin" && actor.Role != "owner" {
		return &apiclient.KickMemberResponse{Code: 403}, nil
	}
	if err := s.db.RemoveMember(ctx, serverID, targetID); err != nil {
		return nil, err
	}
	return &apiclient.KickMemberResponse{Code: 204, Response204: true}, nil
}

func (s *Server) TransferOwnership(ctx context.Context, req *apiclient.TransferOwnershipRequest) (*apiclient.TransferOwnershipResponse, error) {
	serverID, _ := uuid.Parse(req.ServerID)
	uid := auth.UserID(ctx)
	ownerID, _ := uuid.Parse(uid)
	toUserID, _ := uuid.Parse(req.Body.UserID)

	target, _ := s.db.GetMember(ctx, serverID, toUserID)
	if target == nil {
		return &apiclient.TransferOwnershipResponse{Code: 404}, nil
	}
	if err := s.db.TransferOwnership(ctx, serverID, ownerID, toUserID); err != nil {
		return nil, err
	}
	return &apiclient.TransferOwnershipResponse{Code: 204, Response204: true}, nil
}
```

- [ ] **Step 4: Add channel handlers**

```go
func (s *Server) CreateChannel(ctx context.Context, req *apiclient.CreateChannelRequest) (*apiclient.CreateChannelResponse, error) {
	serverID, _ := uuid.Parse(req.ServerID)
	uid := auth.UserID(ctx)
	userID, _ := uuid.Parse(uid)
	actor, _ := s.db.GetMember(ctx, serverID, userID)
	if actor == nil || (actor.Role != "owner" && actor.Role != "admin") {
		return &apiclient.CreateChannelResponse{Code: 403}, nil
	}
	ch, err := s.db.CreateChannel(ctx, serverID, userID, req.Body.Name)
	if err != nil {
		return nil, err
	}
	resp := any(map[string]any{"id": ch.ID.String(), "name": ch.Name, "type": ch.Type, "position": ch.Position})
	return &apiclient.CreateChannelResponse{Code: 201, Response201: &resp}, nil
}

func (s *Server) JoinChannel(ctx context.Context, req *apiclient.JoinChannelRequest) (*apiclient.JoinChannelResponse, error) {
	channelID, _ := uuid.Parse(req.ID)
	ch, err := s.db.GetChannel(ctx, channelID)
	if errors.Is(err, sql.ErrNoRows) {
		return &apiclient.JoinChannelResponse{Code: 404}, nil
	}
	if err != nil {
		return nil, err
	}
	// Verify server membership
	uid := auth.UserID(ctx)
	userID, _ := uuid.Parse(uid)
	_, err = s.db.GetMember(ctx, ch.ServerID, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return &apiclient.JoinChannelResponse{Code: 403}, nil
	}
	// Generate LiveKit token
	roomName := "channel:" + channelID.String()
	token, _ := createLiveKitToken(s.lkSecret, userID, roomName)
	resp := any(map[string]string{"token": token, "url": "wss://" + s.lkHost, "room": roomName})
	return &apiclient.JoinChannelResponse{Code: 200, Response200: &resp}, nil
}

func createLiveKitToken(secret string, userID uuid.UUID, roomName string) (string, error) {
	// Same JWT HS256 pattern as rooms/internal/rooms/server.go:JoinRoom
	import "github.com/golang-jwt/jwt/v5"
	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":   "zvonilka-servers",
		"sub":   userID.String(),
		"nbf":   now.Unix(),
		"exp":   now.Add(24 * time.Hour).Unix(),
		"video": map[string]any{"room": roomName, "roomJoin": true},
	}).SignedString([]byte(secret))
	return token, err
}
```

- [ ] **Step 5: Add HealthCheck handler**

```go
func (s *Server) HealthCheck(ctx context.Context, _ *apiclient.HealthCheckRequest) (*apiclient.HealthCheckResponse, error) {
	if err := s.db.Ping(ctx); err != nil {
		return &apiclient.HealthCheckResponse{Code: http.StatusServiceUnavailable}, nil
	}
	return &apiclient.HealthCheckResponse{Code: http.StatusOK}, nil
}
```

- [ ] **Step 6: Build check**

```bash
cd /Users/n.shchugorev/zvonilka/backend/servers && go build ./...
```

Will fail until OpenAPI specs and generated code exist (Task 5). Expected. Fix generated types in next task.

- [ ] **Step 7: Commit**

```bash
git add internal/service/server.go
git commit -m "feat(servers): HTTP handlers — servers, members, channels"
```

---

### Task 5: OpenAPI specs

**Files:**
- Create: `backend/api-schema/zvonilkaRU/servers/` (entire spec tree)
- Follow existing pattern from `zvonilkaRU/rooms/`

- [ ] **Step 1: Create servers spec structure**

Follow the oapigen conventions from rooms/users:
- `zvonilkaRU/servers/generation_flags.yaml`
- `zvonilkaRU/servers/src/openapi/openapi.yaml`
- `zvonilkaRU/servers/src/openapi/schemas/models/Server.yaml`
- `zvonilkaRU/servers/src/openapi/schemas/models/ServerMember.yaml`
- `zvonilkaRU/servers/src/openapi/schemas/models/Channel.yaml`
- `zvonilkaRU/servers/src/openapi/resources/v1/servers/servers.yaml`
- `zvonilkaRU/servers/src/openapi/resources/v1/servers/{id}.yaml`
- `zvonilkaRU/servers/src/openapi/resources/v1/servers/{id}/members.yaml`
- `zvonilkaRU/servers/src/openapi/resources/v1/servers/{id}/channels.yaml`
- `zvonilkaRU/servers/src/openapi/resources/v1/channels/{id}/join.yaml`
- `zvonilkaRU/servers/src/openapi/resources/v1/service/health.yaml`

- [ ] **Step 2: Run oapigen to generate code**

```bash
cd /Users/n.shchugorev/zvonilka/backend/api-schema
# Ask user for exact oapigen command
```

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "feat(api-schema): add servers API specs and generated code"
```

---

### Task 6: Wire generated code → service, finalize

- [ ] **Step 1: Register generated server interface**

In `backend/servers/internal/service/server.go`, add compile-time check:
```go
var _ apiserver.Server = (*Server)(nil)
```

- [ ] **Step 2: Full build**

```bash
cd /Users/n.shchugorev/zvonilka/backend/servers && go mod tidy && go build ./...
```

- [ ] **Step 3: K8s deployment manifests**

Create `infra/k8s/base/backend/servers/` with deployment + service + kustomization. Follow existing rooms pattern. Add to `backend/kustomization.yaml`.

- [ ] **Step 4: Final commit**

```bash
git add -A && git commit -m "feat(servers): servers+channels service complete"
```
