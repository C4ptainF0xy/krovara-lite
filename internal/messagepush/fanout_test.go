package messagepush

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/krovara/krovara/internal/db"
)

const mucHost = "conference.krovara.local"

type capturedPush struct {
	user      uuid.UUID
	isMention bool
}

type fakeEmitter struct {
	mu   sync.Mutex
	hits []capturedPush
}

func (f *fakeEmitter) Emit(_ context.Context, userID, _ uuid.UUID, isMention bool, _, _ string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hits = append(f.hits, capturedPush{userID, isMention})
}

func (f *fakeEmitter) pushedTo(u uuid.UUID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, h := range f.hits {
		if h.user == u {
			return true
		}
	}
	return false
}

func pgU(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	pg, err := tcpg.Run(ctx, "postgres:16-alpine",
		tcpg.WithDatabase("krovara"), tcpg.WithUsername("krovara"), tcpg.WithPassword("krovara"),
		tcpg.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(pg) })
	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	migDir, _ := filepath.Abs(filepath.Join("..", "..", "migrations"))
	m, err := migrate.New("file://"+filepath.ToSlash(migDir), "pgx5://"+dsn[len("postgres://"):])
	require.NoError(t, err)
	require.NoError(t, m.Up())
	_, _ = m.Close()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	_, err = pool.Exec(ctx, `
CREATE TABLE prosodyarchive (
    sort_id BIGSERIAL PRIMARY KEY,
    host    TEXT NOT NULL,
    "user"  TEXT NOT NULL,
    store   TEXT NOT NULL,
    "key"   TEXT NOT NULL,
    "when"  BIGINT,
    "with"  TEXT,
    type    TEXT,
    value   TEXT
)`)
	require.NoError(t, err)
	return pool
}

func seedMessage(t *testing.T, pool *pgxpool.Pool, channel, author uuid.UUID, key, body string) int64 {
	t.Helper()
	value := fmt.Sprintf(`<message from="%s@%s/%s"><body>%s</body></message>`,
		channel.String(), mucHost, author.String(), body)
	var sortID int64
	err := pool.QueryRow(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, $4, 'xml', $5) RETURNING sort_id`,
		mucHost, channel.String(), key, time.Now().Unix(), value).Scan(&sortID)
	require.NoError(t, err)
	return sortID
}

func seedReply(t *testing.T, pool *pgxpool.Pool, channel, author uuid.UUID, key, parentKey, body string) int64 {
	t.Helper()
	value := fmt.Sprintf(
		`<message from="%s@%s/%s"><body>%s</body><reply xmlns="urn:xmpp:reply:0" to="%s@%s" id="%s"/></message>`,
		channel.String(), mucHost, author.String(), body, channel.String(), mucHost, parentKey)
	var sortID int64
	err := pool.QueryRow(context.Background(), `
INSERT INTO prosodyarchive (host, "user", store, "key", "when", type, value)
VALUES ($1, $2, 'muc_log', $3, $4, 'xml', $5) RETURNING sort_id`,
		mucHost, channel.String(), key, time.Now().Unix(), value).Scan(&sortID)
	require.NoError(t, err)
	return sortID
}

func TestFanout_ReplyFollow(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()
	pool := newPool(t)
	q := db.New(pool)

	author, _ := q.CreateUser(ctx, db.CreateUserParams{Username: "alice", Email: "a@x.io"})
	bob, _ := q.CreateUser(ctx, db.CreateUserParams{Username: "bob", Email: "b@x.io"})
	space, _ := q.CreateSpace(ctx, db.CreateSpaceParams{OwnerID: author.ID, Name: "S"})
	typ, pos := "text", int32(0)
	channel, _ := q.CreateChannel(ctx, db.CreateChannelParams{SpaceID: space.ID, Name: "g", Type: &typ, Position: &pos})
	_, _ = q.CreateMember(ctx, db.CreateMemberParams{SpaceID: space.ID, UserID: author.ID})
	_, _ = q.CreateMember(ctx, db.CreateMemberParams{SpaceID: space.ID, UserID: bob.ID})
	channelID := uuid.UUID(channel.ID.Bytes)
	authorID := uuid.UUID(author.ID.Bytes)
	bobID := uuid.UUID(bob.ID.Bytes)

	require.NoError(t, q.FollowMessage(ctx, db.FollowMessageParams{
		UserID: pgU(bobID), ChannelID: channel.ID, ArchiveID: "p1",
	}))

	em := &fakeEmitter{}
	sid := seedReply(t, pool, channelID, authorID, "r1", "p1", "bonne idee")
	require.NoError(t, fanout(ctx, pool, q, em, sid))
	unread, _ := q.CountInboxUnread(ctx, pgU(bobID))
	require.EqualValues(t, 1, unread, "follower gets a reply inbox item")

	em2 := &fakeEmitter{}
	sid2 := seedReply(t, pool, channelID, authorID, "r2", "other", "autre")
	require.NoError(t, fanout(ctx, pool, q, em2, sid2))
	unread, _ = q.CountInboxUnread(ctx, pgU(bobID))
	require.EqualValues(t, 1, unread, "no inbox item for an unfollowed parent")

	require.NoError(t, q.FollowMessage(ctx, db.FollowMessageParams{
		UserID: pgU(authorID), ChannelID: channel.ID, ArchiveID: "p1",
	}))
	em3 := &fakeEmitter{}
	sid3 := seedReply(t, pool, channelID, authorID, "r3", "p1", "encore moi")
	require.NoError(t, fanout(ctx, pool, q, em3, sid3))
	authorUnread, _ := q.CountInboxUnread(ctx, pgU(authorID))
	require.EqualValues(t, 0, authorUnread, "author isn't notified of their own reply")
}

func TestFanout_MentionInboxAndPush(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()
	pool := newPool(t)
	q := db.New(pool)

	author, err := q.CreateUser(ctx, db.CreateUserParams{Username: "alice", Email: "a@x.io"})
	require.NoError(t, err)
	bob, err := q.CreateUser(ctx, db.CreateUserParams{Username: "bob", Email: "b@x.io"})
	require.NoError(t, err)
	authorID := uuid.UUID(author.ID.Bytes)
	bobID := uuid.UUID(bob.ID.Bytes)

	space, err := q.CreateSpace(ctx, db.CreateSpaceParams{OwnerID: author.ID, Name: "S"})
	require.NoError(t, err)
	typ, pos := "text", int32(0)
	channel, err := q.CreateChannel(ctx, db.CreateChannelParams{
		SpaceID: space.ID, Name: "general", Type: &typ, Position: &pos,
	})
	require.NoError(t, err)
	_, err = q.CreateMember(ctx, db.CreateMemberParams{SpaceID: space.ID, UserID: author.ID})
	require.NoError(t, err)
	_, err = q.CreateMember(ctx, db.CreateMemberParams{SpaceID: space.ID, UserID: bob.ID})
	require.NoError(t, err)

	channelID := uuid.UUID(channel.ID.Bytes)
	spaceID := uuid.UUID(space.ID.Bytes)

	em := &fakeEmitter{}
	sid := seedMessage(t, pool, channelID, authorID, "m1", "hey @"+bobID.String()+" look")
	require.NoError(t, fanout(ctx, pool, q, em, sid))
	require.True(t, em.pushedTo(bobID))
	unread, _ := q.CountInboxUnread(ctx, pgU(bobID))
	require.EqualValues(t, 1, unread)

	_, err = q.UpsertNotifSetting(ctx, db.UpsertNotifSettingParams{
		UserID: pgU(bobID), ScopeType: "space", ScopeID: space.ID, Level: "nothing",
	})
	require.NoError(t, err)
	em2 := &fakeEmitter{}
	sid2 := seedMessage(t, pool, channelID, authorID, "m2", "just chatting")
	require.NoError(t, fanout(ctx, pool, q, em2, sid2))
	require.False(t, em2.pushedTo(bobID))

	em3 := &fakeEmitter{}
	sid3 := seedMessage(t, pool, channelID, authorID, "m3", "ping @"+bobID.String())
	require.NoError(t, fanout(ctx, pool, q, em3, sid3))
	require.False(t, em3.pushedTo(bobID), "level=nothing suppresses push")
	unread, _ = q.CountInboxUnread(ctx, pgU(bobID))
	require.EqualValues(t, 2, unread, "mention still lands in inbox")

	_, err = q.UpsertNotifSetting(ctx, db.UpsertNotifSettingParams{
		UserID: pgU(bobID), ScopeType: "space", ScopeID: space.ID, Level: "mentions",
		MutedUntil: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})
	require.NoError(t, err)
	em4 := &fakeEmitter{}
	sid4 := seedMessage(t, pool, channelID, authorID, "m4", "yo @"+bobID.String())
	require.NoError(t, fanout(ctx, pool, q, em4, sid4))
	require.False(t, em4.pushedTo(bobID), "mute suppresses push")

	_ = spaceID
}

func TestFanout_EveryoneSuppression(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()
	pool := newPool(t)
	q := db.New(pool)

	author, _ := q.CreateUser(ctx, db.CreateUserParams{Username: "alice", Email: "a@x.io"})
	bob, _ := q.CreateUser(ctx, db.CreateUserParams{Username: "bob", Email: "b@x.io"})
	space, _ := q.CreateSpace(ctx, db.CreateSpaceParams{OwnerID: author.ID, Name: "S"})
	typ, pos := "text", int32(0)
	channel, _ := q.CreateChannel(ctx, db.CreateChannelParams{SpaceID: space.ID, Name: "g", Type: &typ, Position: &pos})
	_, _ = q.CreateMember(ctx, db.CreateMemberParams{SpaceID: space.ID, UserID: author.ID})
	_, _ = q.CreateMember(ctx, db.CreateMemberParams{SpaceID: space.ID, UserID: bob.ID})
	channelID := uuid.UUID(channel.ID.Bytes)
	bobID := uuid.UUID(bob.ID.Bytes)
	authorID := uuid.UUID(author.ID.Bytes)

	em := &fakeEmitter{}
	require.NoError(t, fanout(ctx, pool, q, em, seedMessage(t, pool, channelID, authorID, "e1", "@everyone hi")))
	require.True(t, em.pushedTo(bobID))

	_, err := q.UpsertNotifSetting(ctx, db.UpsertNotifSettingParams{
		UserID: pgU(bobID), ScopeType: "space", ScopeID: space.ID, Level: "mentions",
		SuppressEveryone: true,
	})
	require.NoError(t, err)
	before, _ := q.CountInboxUnread(ctx, pgU(bobID))
	em2 := &fakeEmitter{}
	require.NoError(t, fanout(ctx, pool, q, em2, seedMessage(t, pool, channelID, authorID, "e2", "@everyone again")))
	require.False(t, em2.pushedTo(bobID))
	after, _ := q.CountInboxUnread(ctx, pgU(bobID))
	require.Equal(t, before, after, "suppressed @everyone creates no inbox item")
}

func TestFanout_UsernameAndRoleMention(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	ctx := context.Background()
	pool := newPool(t)
	q := db.New(pool)

	author, _ := q.CreateUser(ctx, db.CreateUserParams{Username: "alice", Email: "a@x.io"})
	bob, _ := q.CreateUser(ctx, db.CreateUserParams{Username: "bob", Email: "b@x.io"})
	carol, _ := q.CreateUser(ctx, db.CreateUserParams{Username: "carol", Email: "c@x.io"})
	authorID := uuid.UUID(author.ID.Bytes)
	bobID := uuid.UUID(bob.ID.Bytes)
	carolID := uuid.UUID(carol.ID.Bytes)

	space, _ := q.CreateSpace(ctx, db.CreateSpaceParams{OwnerID: author.ID, Name: "S"})
	typ, pos := "text", int32(0)
	channel, _ := q.CreateChannel(ctx, db.CreateChannelParams{
		SpaceID: space.ID, Name: "general", Type: &typ, Position: &pos,
	})
	_, _ = q.CreateMember(ctx, db.CreateMemberParams{SpaceID: space.ID, UserID: author.ID})
	_, _ = q.CreateMember(ctx, db.CreateMemberParams{SpaceID: space.ID, UserID: bob.ID})
	carolMem, _ := q.CreateMember(ctx, db.CreateMemberParams{SpaceID: space.ID, UserID: carol.ID})

	var roleID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO roles (space_id, name, mentionable, is_everyone) VALUES ($1,'Mods',true,false) RETURNING id`,
		space.ID).Scan(&roleID))
	require.NoError(t, q.AssignMemberRole(ctx, db.AssignMemberRoleParams{
		MemberID: carolMem.ID, RoleID: pgU(roleID),
	}))

	channelID := uuid.UUID(channel.ID.Bytes)

	em := &fakeEmitter{}
	sid := seedMessage(t, pool, channelID, authorID, "n1", "hey @bob and @Mods ship it")
	require.NoError(t, fanout(ctx, pool, q, em, sid))

	require.True(t, em.pushedTo(bobID), "bob mentioned by username")
	require.True(t, em.pushedTo(carolID), "carol mentioned via @Mods role")
	ub, _ := q.CountInboxUnread(ctx, pgU(bobID))
	require.EqualValues(t, 1, ub)
	uc, _ := q.CountInboxUnread(ctx, pgU(carolID))
	require.EqualValues(t, 1, uc)
}
