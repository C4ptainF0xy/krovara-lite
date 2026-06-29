package search

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseQuery_PlainText(t *testing.T) {
	pq := ParseQuery("hello world")
	require.Equal(t, "hello world", pq.Text)
	require.Empty(t, pq.From)
	require.Empty(t, pq.In)
	require.Zero(t, pq.Before)
	require.Zero(t, pq.After)
}

func TestParseQuery_Operators(t *testing.T) {
	pq := ParseQuery("from:alice in:general deploy after:2026-01-01 before:2026-02-01")
	require.Equal(t, "alice", pq.From)
	require.Equal(t, "general", pq.In)
	require.Equal(t, "deploy", pq.Text)
	require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix(), pq.After)
	require.Equal(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC).Unix(), pq.Before)
}

func TestParseQuery_OperatorsOnly(t *testing.T) {
	pq := ParseQuery("from:bob")
	require.Equal(t, "bob", pq.From)
	require.Empty(t, pq.Text)
}

func TestParseQuery_UnknownOperatorKept(t *testing.T) {
	pq := ParseQuery("foo:bar hello")
	require.Equal(t, "foo:bar hello", pq.Text)
}

func TestParseQuery_BadDateIgnored(t *testing.T) {
	pq := ParseQuery("before:nope hi")
	require.Zero(t, pq.Before)
	require.Equal(t, "hi", pq.Text)
}

func TestParseQuery_HasFilters(t *testing.T) {
	pq := ParseQuery("has:link rapport has:image")
	require.True(t, pq.HasLink)
	require.True(t, pq.HasMedia)
	require.Equal(t, "rapport", pq.Text)

	require.True(t, ParseQuery("has:file").HasMedia)
	require.True(t, ParseQuery("has:attachment").HasMedia)

	require.Equal(t, "has:banana", ParseQuery("has:banana").Text)
}

func TestParseQuery_CaseInsensitiveKeys(t *testing.T) {
	pq := ParseQuery("FROM:x IN:y")
	require.Equal(t, "x", pq.From)
	require.Equal(t, "y", pq.In)
}
