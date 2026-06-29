package searchingest

import (
	"errors"
	"testing"
)

const sampleStanza = `<message from='de076041-3387-4150-8878-0d918ad3447f@conference.krovara.local/TestUser' type='groupchat'><body>test</body><x xmlns='http://jabber.org/protocol/muc#user'><item jid='1664f750-fa1d-4142-aacc-794373897ab3@krovara.local' role='moderator' affiliation='owner'/></x></message>`

func TestParseArchiveRow_extractsBodyAuthorChannel(t *testing.T) {
	user := "de076041-3387-4150-8878-0d918ad3447f"
	got, err := ParseArchiveRow(user, sampleStanza)
	if err != nil {
		t.Fatalf("ParseArchiveRow: %v", err)
	}
	if got.ChannelID != user {
		t.Errorf("ChannelID = %q, want %q", got.ChannelID, user)
	}
	if got.AuthorID != "1664f750-fa1d-4142-aacc-794373897ab3" {
		t.Errorf("AuthorID = %q", got.AuthorID)
	}
	if got.Body != "test" {
		t.Errorf("Body = %q", got.Body)
	}
}

func TestParseArchiveRow_skipsBodylessStanza(t *testing.T) {

	stanza := `<message from='room@conference.krovara.local/nick' type='groupchat'><subject>topic</subject></message>`
	_, err := ParseArchiveRow("room", stanza)
	if !errors.Is(err, ErrNoBody) {
		t.Errorf("err = %v, want ErrNoBody", err)
	}
}

func TestParseArchiveRow_fallsBackToResourceWhenNoMucItem(t *testing.T) {

	stanza := `<message from='room@conference.krovara.local/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa' type='groupchat'><body>hi</body></message>`
	got, err := ParseArchiveRow("room", stanza)
	if err != nil {
		t.Fatalf("ParseArchiveRow: %v", err)
	}
	if got.AuthorID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Errorf("AuthorID = %q", got.AuthorID)
	}
}

func TestParseArchiveRow_extractsOriginAndReplace(t *testing.T) {

	stanza := `<message from='room@conference.krovara.local/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa' type='groupchat'>` +
		`<body>v2</body>` +
		`<origin-id xmlns='urn:xmpp:sid:0' id='o2'/>` +
		`<replace xmlns='urn:xmpp:message-correct:0' id='o1'/></message>`
	got, err := ParseArchiveRow("room", stanza)
	if err != nil {
		t.Fatalf("ParseArchiveRow: %v", err)
	}
	if got.OriginID != "o2" {
		t.Errorf("OriginID = %q, want o2", got.OriginID)
	}
	if got.ReplaceID != "o1" {
		t.Errorf("ReplaceID = %q, want o1", got.ReplaceID)
	}
}

func TestParseArchiveRow_noCorrectionLeavesReplaceEmpty(t *testing.T) {
	got, err := ParseArchiveRow("room", sampleStanza)
	if err != nil {
		t.Fatalf("ParseArchiveRow: %v", err)
	}
	if got.ReplaceID != "" {
		t.Errorf("ReplaceID = %q, want empty", got.ReplaceID)
	}
}

func TestParseArchiveRow_emptyUser(t *testing.T) {
	if _, err := ParseArchiveRow("", sampleStanza); err == nil {
		t.Error("expected error on empty user")
	}
}

func TestParseArchiveRow_malformedXML(t *testing.T) {
	if _, err := ParseArchiveRow("room", "<not xml"); err == nil {
		t.Error("expected error on malformed XML")
	}
}
