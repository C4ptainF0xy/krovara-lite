package searchingest

import (
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Parsed struct {
	ChannelID string
	AuthorID  string
	Body      string
	OriginID  string
	ReplaceID string
	HasLink   bool
	HasMedia  bool
}

func ParseArchiveRow(archiveUser, value string) (*Parsed, error) {
	if archiveUser == "" {
		return nil, errors.New("empty archive user")
	}
	var msg messageEnvelope
	if err := xml.Unmarshal([]byte(value), &msg); err != nil {
		return nil, fmt.Errorf("decode stanza: %w", err)
	}
	body := strings.TrimSpace(msg.Body)
	if body == "" {
		return nil, ErrNoBody
	}
	author := ""
	for _, it := range msg.X.Items {
		if it.JID != "" {
			author = bareLocal(it.JID)
			break
		}
	}
	if author == "" {

		author = resourceOf(msg.From)
	}
	if author == "" {
		return nil, errors.New("no author")
	}
	return &Parsed{
		ChannelID: archiveUser,
		AuthorID:  author,
		Body:      body,
		OriginID:  msg.OriginID.ID,
		ReplaceID: msg.Replace.ID,
		HasLink:   urlRE.MatchString(body),

		HasMedia: strings.Contains(value, "jabber:x:oob"),
	}, nil
}

var urlRE = regexp.MustCompile(`https?://[^\s]+`)

var ErrNoBody = errors.New("stanza has no body")

type messageEnvelope struct {
	XMLName  xml.Name `xml:"message"`
	From     string   `xml:"from,attr"`
	Body     string   `xml:"body"`
	X        mucX     `xml:"x"`
	OriginID idAttr   `xml:"origin-id"`
	Replace  idAttr   `xml:"replace"`
}

type mucX struct {
	Items []mucItem `xml:"item"`
}

type mucItem struct {
	JID string `xml:"jid,attr"`
}

type idAttr struct {
	ID string `xml:"id,attr"`
}

func bareLocal(jid string) string {
	at := strings.IndexByte(jid, '@')
	if at < 0 {
		return ""
	}
	return jid[:at]
}

func resourceOf(jid string) string {
	slash := strings.IndexByte(jid, '/')
	if slash < 0 {
		return ""
	}
	return jid[slash+1:]
}
