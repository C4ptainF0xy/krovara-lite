package messagepush

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/eventsfeed"
	"github.com/krovara/krovara/internal/push"
	"github.com/krovara/krovara/internal/searchingest"
)

var uuidRE = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

type Emitter interface {
	Emit(ctx context.Context, userID, spaceID uuid.UUID, isMention bool, title, body string)
}

func Listen(ctx context.Context, pool *pgxpool.Pool, emitter Emitter, log *slog.Logger) error {
	const channel = "search_ingest"
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for ctx.Err() == nil {
		err := listenOnce(ctx, pool, emitter, channel, log)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Warn("message push listener disconnected", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	return ctx.Err()
}

func listenOnce(ctx context.Context, pool *pgxpool.Pool, emitter Emitter, channel string, log *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}
	log.Info("message push listener online", "channel", channel)

	q := db.New(pool)
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		sortID, perr := strconv.ParseInt(n.Payload, 10, 64)
		if perr != nil {
			continue
		}
		if err := fanout(ctx, pool, q, emitter, sortID); err != nil {
			log.Warn("push fanout failed", "sort_id", sortID, "err", err)
		}
	}
}

func fanout(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, emitter Emitter, sortID int64) error {
	var user, value, archiveKey string
	err := pool.QueryRow(ctx, `
SELECT "user", value, "key" FROM prosodyarchive
WHERE sort_id = $1 AND host = 'conference.krovara.local' AND store = 'muc_log' AND type = 'xml'
`, sortID).Scan(&user, &value, &archiveKey)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	parsed, err := searchingest.ParseArchiveRow(user, value)
	if err != nil {

		return nil
	}
	channelUUID, err := uuid.Parse(parsed.ChannelID)
	if err != nil {
		return nil
	}
	authorUUID, err := uuid.Parse(parsed.AuthorID)
	if err != nil {
		return nil
	}
	channel, err := q.GetChannel(ctx, pgtype.UUID{Bytes: channelUUID, Valid: true})
	if err != nil {
		return nil
	}
	members, err := q.ListSpaceMembers(ctx, channel.SpaceID)
	if err != nil {
		return fmt.Errorf("list members: %w", err)
	}

	byUsername := make(map[string]uuid.UUID, len(members))
	for _, m := range members {
		byUsername[strings.ToLower(m.Username)] = uuid.UUID(m.UserID.Bytes)
	}
	roleMembers := map[string][]uuid.UUID{}
	if rows, rerr := q.ListMentionableRoleMembers(ctx, channel.SpaceID); rerr == nil {
		for _, row := range rows {
			roleMembers[row.RoleName] = append(roleMembers[row.RoleName], uuid.UUID(row.UserID.Bytes))
		}
	}
	mentioned := resolveMentions(parsed.Body, byUsername, roleMembers)
	isEveryone := mentionsEveryone(parsed.Body)
	title := "#" + channel.Name
	preview := truncate(parsed.Body, 120)
	spaceUUID := uuid.UUID(channel.SpaceID.Bytes)

	for _, m := range members {
		recipient := uuid.UUID(m.UserID.Bytes)
		if recipient == authorUUID {
			continue
		}
		_, directMention := mentioned[recipient]

		level, mutedUntil, suppressEveryone := effectiveSettings(ctx, q, recipient, spaceUUID, channelUUID)

		everyoneMention := isEveryone && !suppressEveryone
		isMention := directMention || everyoneMention

		if isMention {
			kind := "everyone"
			if directMention {
				kind = "mention"
			}
			_ = q.CreateInboxItem(ctx, db.CreateInboxItemParams{
				UserID:    pgtype.UUID{Bytes: recipient, Valid: true},
				Kind:      kind,
				SpaceID:   pgtype.UUID{Bytes: spaceUUID, Valid: true},
				ChannelID: pgtype.UUID{Bytes: channelUUID, Valid: true},
				ArchiveID: archiveKey,
				AuthorID:  pgtype.UUID{Bytes: authorUUID, Valid: true},
				Preview:   ptr(preview),
			})
			_ = eventsfeed.Emit(ctx, pool, recipient, "inbox_update", map[string]any{})
		}

		muted := mutedUntil != nil && mutedUntil.After(time.Now())
		if muted || level == "nothing" {
			continue
		}
		if level == "mentions" && !isMention {
			continue
		}
		emitter.Emit(ctx, recipient, spaceUUID, isMention, title, preview)
	}

	if parentID := replyParentID(value); parentID != "" {
		followers, ferr := q.ListMessageFollowers(ctx, db.ListMessageFollowersParams{
			ChannelID: channel.ID, ArchiveID: parentID,
		})
		if ferr == nil {
			for _, f := range followers {
				recipient := uuid.UUID(f.Bytes)
				if recipient == authorUUID {
					continue
				}
				_ = q.CreateInboxItem(ctx, db.CreateInboxItemParams{
					UserID:    pgtype.UUID{Bytes: recipient, Valid: true},
					Kind:      "reply",
					SpaceID:   pgtype.UUID{Bytes: spaceUUID, Valid: true},
					ChannelID: channel.ID,
					ArchiveID: archiveKey,
					AuthorID:  pgtype.UUID{Bytes: authorUUID, Valid: true},
					Preview:   ptr(preview),
				})
				_ = eventsfeed.Emit(ctx, pool, recipient, "inbox_update", map[string]any{})
				level, mutedUntil, _ := effectiveSettings(ctx, q, recipient, spaceUUID, channelUUID)
				if level == "nothing" || (mutedUntil != nil && mutedUntil.After(time.Now())) {
					continue
				}

				emitter.Emit(ctx, recipient, spaceUUID, true, title, preview)
			}
		}
	}
	return nil
}

func replyParentID(stanza string) string {
	m := replyRE.FindStringSubmatch(stanza)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

var replyRE = regexp.MustCompile(`<reply\b[^>]*\bid="([^"]*)"`)

func effectiveSettings(ctx context.Context, q *db.Queries, user, space, channel uuid.UUID) (level string, mutedUntil *time.Time, suppressEveryone bool) {
	level = "all"

	for _, sc := range []struct {
		t  string
		id uuid.UUID
	}{{"space", space}, {"channel", channel}} {
		row, err := q.GetNotifSetting(ctx, db.GetNotifSettingParams{
			UserID: pgtype.UUID{Bytes: user, Valid: true}, ScopeType: sc.t,
			ScopeID: pgtype.UUID{Bytes: sc.id, Valid: true},
		})
		if err != nil {
			continue
		}
		level = row.Level
		if row.SuppressEveryone {
			suppressEveryone = true
		}
		if row.MutedUntil.Valid {
			t := row.MutedUntil.Time
			mutedUntil = &t
		}
	}
	return level, mutedUntil, suppressEveryone
}

func ptr[T any](v T) *T { return &v }

func mentionsEveryone(body string) bool {
	return strings.Contains(body, "@everyone") || strings.Contains(body, "@here")
}

var nameMentionRE = regexp.MustCompile(`@([A-Za-z0-9_][A-Za-z0-9_-]{0,31})`)

func resolveMentions(body string, byUsername map[string]uuid.UUID, roleMembers map[string][]uuid.UUID) map[uuid.UUID]struct{} {
	out := map[uuid.UUID]struct{}{}

	for _, m := range uuidRE.FindAllString(body, -1) {
		if id, err := uuid.Parse(m); err == nil {
			out[id] = struct{}{}
		}
	}

	for _, m := range nameMentionRE.FindAllStringSubmatch(body, -1) {
		token := strings.ToLower(m[1])
		if id, ok := byUsername[token]; ok {
			out[id] = struct{}{}
		}
		for _, id := range roleMembers[token] {
			out[id] = struct{}{}
		}
	}
	return out
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

var _ Emitter = (*push.Service)(nil)
