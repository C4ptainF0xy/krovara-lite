package search

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/krovara/krovara/internal/db"
)

type stubQuerier struct {
	db.Querier
	channels []db.Channel
	err      error
}

func (s stubQuerier) ListSpaceChannels(_ context.Context, _ pgtype.UUID) ([]db.Channel, error) {
	return s.channels, s.err
}

func mkChannel(id uuid.UUID) db.Channel {
	return db.Channel{ID: pgtype.UUID{Bytes: id, Valid: true}}
}

func TestAllowedFilter_PinsSpaceAndChannels(t *testing.T) {
	space := uuid.New()
	c1, c2 := uuid.New(), uuid.New()
	s := &Service{q: stubQuerier{channels: []db.Channel{mkChannel(c1), mkChannel(c2)}}}

	out, err := s.allowedFilter(context.Background(), uuid.Nil, space, "", "")
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Equal(t, `space_id = "`+space.String()+`"`, out[0])
	require.Contains(t, out[1], c1.String())
	require.Contains(t, out[1], c2.String())
}

func TestAllowedFilter_NarrowsToChannelHint(t *testing.T) {
	space := uuid.New()
	c1, c2 := uuid.New(), uuid.New()
	s := &Service{q: stubQuerier{channels: []db.Channel{mkChannel(c1), mkChannel(c2)}}}

	out, err := s.allowedFilter(context.Background(), uuid.Nil, space, c1.String(), "")
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, `channel_id = "`+c1.String()+`"`, out[2])
}

func TestAllowedFilter_RejectsForeignChannel(t *testing.T) {
	space := uuid.New()
	c1 := uuid.New()
	other := uuid.New()
	s := &Service{q: stubQuerier{channels: []db.Channel{mkChannel(c1)}}}

	_, err := s.allowedFilter(context.Background(), uuid.Nil, space, other.String(), "")
	require.ErrorContains(t, err, "channel not visible")
}

func TestAllowedFilter_RejectsBadAuthor(t *testing.T) {
	space := uuid.New()
	c1 := uuid.New()
	s := &Service{q: stubQuerier{channels: []db.Channel{mkChannel(c1)}}}

	_, err := s.allowedFilter(context.Background(), uuid.Nil, space, "", "not-a-uuid")
	require.ErrorContains(t, err, "invalid author")
}

func TestAllowedFilter_EmptyChannelList(t *testing.T) {
	s := &Service{q: stubQuerier{channels: nil}}
	_, err := s.allowedFilter(context.Background(), uuid.Nil, uuid.New(), "", "")
	require.ErrorContains(t, err, "no visible channels")
}

func TestAllowedFilter_AddsAuthor(t *testing.T) {
	space := uuid.New()
	c1 := uuid.New()
	author := uuid.New()
	s := &Service{q: stubQuerier{channels: []db.Channel{mkChannel(c1)}}}

	out, err := s.allowedFilter(context.Background(), uuid.Nil, space, "", author.String())
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, `author_id = "`+author.String()+`"`, out[2])
}
