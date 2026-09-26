package odds

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func closed(assoc string, merged bool, daysAgo int, hoursToMerge float64, firstRespHours float64, mergedBy string) PR {
	created := now.Add(-time.Duration(daysAgo) * 24 * time.Hour)
	p := PR{Number: 1, Author: "someone", Association: assoc, CreatedAt: created, Merged: merged, MergedBy: mergedBy}
	p.ClosedAt = created.Add(time.Duration(hoursToMerge * float64(time.Hour)))
	if merged {
		p.MergedAt = p.ClosedAt
	}
	if firstRespHours > 0 {
		p.FirstResponse = created.Add(time.Duration(firstRespHours * float64(time.Hour)))
	}
	return p
}

func open(assoc string, daysAgo int, draft bool) PR {
	return PR{Number: 2, Author: "someone", Association: assoc, CreatedAt: now.Add(-time.Duration(daysAgo) * 24 * time.Hour), IsDraft: draft}
}

func TestOpenLane(t *testing.T) {
	var r Repo
	for i := 0; i < 20; i++ {
		r.Closed = append(r.Closed, closed("CONTRIBUTOR", i%10 != 0, 5+i, 20, 4, "maintainer"))
	}
	r.Closed = append(r.Closed, closed("MEMBER", true, 3, 2, 1, "maintainer"))
	r.Open = append(r.Open, open("NONE", 3, false), open("NONE", 200, false), open("MEMBER", 100, false), open("NONE", 100, true))
	o := Compute(r, now)
	if o.Outsiders.Closed != 20 || o.Outsiders.Merged != 18 {
		t.Fatalf("outsiders %+v", o.Outsiders)
	}
	if o.Verdict != "OPEN LANE" {
		t.Fatalf("verdict %s: %s", o.Verdict, o.VerdictWhy)
	}
	if o.Merges30d != 19 { // 18 outsider merges within 25 days plus the member's
		t.Fatalf("merges30d %d", o.Merges30d)
	}
	if o.Mergers[0].Login != "maintainer" || o.Mergers[0].Count != 19 {
		t.Fatalf("mergers %+v", o.Mergers)
	}
	if o.OpenOutsider != 2 || o.OpenOutsiderOld != 1 || o.OldestOpenDays != 200 {
		t.Fatalf("open %d old %d oldest %d", o.OpenOutsider, o.OpenOutsiderOld, o.OldestOpenDays)
	}
	if o.Outsiders.MedianToMerge != 20*time.Hour || o.Outsiders.MedianFirstResp != 4*time.Hour {
		t.Fatalf("medians %v %v", o.Outsiders.MedianToMerge, o.Outsiders.MedianFirstResp)
	}
}

func TestBacklogIsAFactNotTheVerdict(t *testing.T) {
	var r Repo
	for i := 0; i < 10; i++ {
		r.Closed = append(r.Closed, closed("CONTRIBUTOR", true, 2+i, 16, 3, "m"))
	}
	for i := 0; i < 30; i++ {
		r.Open = append(r.Open, open("NONE", 100+i, false))
	}
	o := Compute(r, now)
	if o.Verdict != "OPEN LANE" || !strings.Contains(o.VerdictWhy, "30 open outsider PRs are older than 60 days") {
		t.Fatalf("%s: %s", o.Verdict, o.VerdictWhy)
	}
}

func TestSlowLaneDormantLongShotUntestedArchived(t *testing.T) {
	var slow Repo
	for i := 0; i < 10; i++ {
		slow.Closed = append(slow.Closed, closed("CONTRIBUTOR", true, 200+i, 24*30, 24*10, "m"))
	}
	slow.Closed = append(slow.Closed, closed("CONTRIBUTOR", true, 3, 24*30, 24, "m"))
	if o := Compute(slow, now); o.Verdict != "SLOW LANE" {
		t.Fatalf("slow: %s %s", o.Verdict, o.VerdictWhy)
	}
	var dormant Repo
	for i := 0; i < 10; i++ {
		dormant.Closed = append(dormant.Closed, closed("CONTRIBUTOR", true, 100+i, 24, 2, "m"))
	}
	if o := Compute(dormant, now); o.Verdict != "DORMANT" {
		t.Fatalf("dormant: %s", o.Verdict)
	}
	var longshot Repo
	for i := 0; i < 10; i++ {
		longshot.Closed = append(longshot.Closed, closed("CONTRIBUTOR", i == 0, 5+i, 24, 2, "m"))
	}
	longshot.Closed = append(longshot.Closed, closed("MEMBER", true, 2, 2, 1, "m"))
	if o := Compute(longshot, now); o.Verdict != "LONG SHOT" {
		t.Fatalf("longshot: %s %s", o.Verdict, o.VerdictWhy)
	}
	var untested Repo
	for i := 0; i < 8; i++ {
		untested.Closed = append(untested.Closed, closed("MEMBER", true, 2+i, 2, 1, "m"))
	}
	untested.Closed = append(untested.Closed, closed("NONE", true, 4, 5, 1, "m"))
	if o := Compute(untested, now); o.Verdict != "UNTESTED" {
		t.Fatalf("untested: %s", o.Verdict)
	}
	if o := Compute(Repo{Archived: true, Closed: untested.Closed}, now); o.Verdict != "ARCHIVED" {
		t.Fatalf("archived: %s", o.Verdict)
	}
}

func TestBotsAreIgnored(t *testing.T) {
	var r Repo
	for i := 0; i < 6; i++ {
		r.Closed = append(r.Closed, closed("CONTRIBUTOR", true, 2+i, 5, 1, "m"))
	}
	bot := closed("NONE", true, 1, 1, 0, "m")
	bot.Author, bot.AuthorIsBot = "dependabot[bot]", true
	r.Closed = append(r.Closed, bot, bot, bot, bot)
	o := Compute(r, now)
	if o.SampleClosed != 6 || o.Outsiders.Closed != 6 {
		t.Fatalf("bots counted: %+v", o.Outsiders)
	}
	if !IsBot("CLAassistant", "User") || !IsBot("x", "Bot") || IsBot("larsoner", "User") {
		t.Fatal("bot detection")
	}
}

func TestHumanize(t *testing.T) {
	cases := map[time.Duration]string{0: "-", 30 * time.Minute: "<1h", 16 * time.Hour: "16h", 47 * time.Hour: "47h", 72 * time.Hour: "3.0d", 19*24*time.Hour + 14*time.Hour: "19.6d"}
	for d, want := range cases {
		if got := Humanize(d); got != want {
			t.Errorf("%v: got %s want %s", d, got, want)
		}
	}
}
