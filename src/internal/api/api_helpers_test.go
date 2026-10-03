package api

import (
	"strings"
	"testing"
	"time"
)

func TestValidLang(t *testing.T) {
	cases := map[string]string{"it": "it", "en": "en", "nl": "nl", "fr": "it", "": "it", "EN": "it"}
	for in, want := range cases {
		if got := validLang(in); got != want {
			t.Errorf("validLang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	if got := splitCSV(""); got != nil {
		t.Errorf("empty: want nil, got %v", got)
	}
	if got := splitCSV("   "); got != nil {
		t.Errorf("blank: want nil, got %v", got)
	}
	got := splitCSV("a, b ,,c")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len: got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestCleanName(t *testing.T) {
	ok := []string{"Fox", "José", "Anne-Marie", "O'Brien", "Red Panda", "L'Hoest (monkey)", "a"}
	for _, s := range ok {
		if _, valid := cleanName(s); !valid {
			t.Errorf("cleanName(%q): want valid", s)
		}
	}
	if got, _ := cleanName("  Fox  "); got != "Fox" {
		t.Errorf("cleanName trims: got %q", got)
	}
	bad := []string{"", "   ", "Fox123", "<script>", "a>b", "a\tb", strings.Repeat("x", 61)}
	for _, s := range bad {
		if _, valid := cleanName(s); valid {
			t.Errorf("cleanName(%q): want invalid", s)
		}
	}
	// 60 runes is the inclusive upper bound.
	if _, valid := cleanName(strings.Repeat("x", 60)); !valid {
		t.Error("cleanName(60 runes): want valid")
	}
}

func TestCleanNote(t *testing.T) {
	// An empty note is allowed (optional field) and comes back empty.
	if got, valid := cleanNote(""); !valid || got != "" {
		t.Errorf("empty note: got %q valid=%v", got, valid)
	}
	if _, valid := cleanNote("perfectly ordinary note, with punctuation!"); !valid {
		t.Error("ordinary note: want valid")
	}
	bad := []string{"a<b", "a>b", "a\x00b", strings.Repeat("x", 281)}
	for _, s := range bad {
		if _, valid := cleanNote(s); valid {
			t.Errorf("cleanNote(%q): want invalid", s)
		}
	}
	if _, valid := cleanNote(strings.Repeat("x", 280)); !valid {
		t.Error("cleanNote(280 runes): want valid")
	}
}

func TestCleanNick(t *testing.T) {
	ok := []string{"Niklas", "Niklas2", "José", "Anne-Marie", "O'Brien", "a b"}
	for _, s := range ok {
		if _, valid := cleanNick(s); !valid {
			t.Errorf("cleanNick(%q): want valid", s)
		}
	}
	bad := []string{"", "   ", "<x>", "a\x00b", strings.Repeat("x", NickMaxLen+1)}
	for _, s := range bad {
		if _, valid := cleanNick(s); valid {
			t.Errorf("cleanNick(%q): want invalid", s)
		}
	}
	if _, valid := cleanNick(strings.Repeat("x", NickMaxLen)); !valid {
		t.Errorf("cleanNick(%d runes): want valid", NickMaxLen)
	}
}

func TestRateLimiterWindow(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	base := time.Now()
	if !rl.allow("1.2.3.4", base) || !rl.allow("1.2.3.4", base) {
		t.Fatal("first two requests in the window should pass")
	}
	if rl.allow("1.2.3.4", base) {
		t.Error("third request in the same window should be refused")
	}
	// A different IP has its own budget.
	if !rl.allow("5.6.7.8", base) {
		t.Error("a different IP should not be rate limited")
	}
	// After the window elapses, the first IP is allowed again.
	if !rl.allow("1.2.3.4", base.Add(time.Minute+time.Second)) {
		t.Error("the window should reset after it elapses")
	}
}
