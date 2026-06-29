package joingate

import (
	"encoding/json"
	"strings"
	"testing"
)

func qsJSON(t *testing.T, qs []question) []byte {
	t.Helper()
	b, err := json.Marshal(qs)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestValidateAnswers(t *testing.T) {
	form := qsJSON(t, []question{
		{ID: "why", Label: "Why?", Required: true},
		{ID: "ref", Label: "Referral", Required: false},
	})

	if _, _, ok := validateAnswers(form, []answer{{QuestionID: "ref", Answer: "bob"}}); ok {
		t.Fatal("expected failure when required answer is missing")
	}

	norm, _, ok := validateAnswers(form, []answer{
		{QuestionID: "why", Answer: " I like it "},
		{QuestionID: "evil", Answer: "junk"},
	})
	if !ok {
		t.Fatal("expected success")
	}
	if len(norm) != 1 || norm[0].QuestionID != "why" || norm[0].Answer != "I like it" {
		t.Fatalf("expected one trimmed known answer, got %+v", norm)
	}

	if _, _, ok := validateAnswers(form, []answer{{QuestionID: "why", Answer: strings.Repeat("x", 2001)}}); ok {
		t.Fatal("expected failure on over-long answer")
	}

	norm, _, ok = validateAnswers(form, []answer{
		{QuestionID: "why", Answer: "yes"}, {QuestionID: "ref", Answer: "alice"},
	})
	if !ok || len(norm) != 2 {
		t.Fatalf("expected both answers, got %+v ok=%v", norm, ok)
	}
}
