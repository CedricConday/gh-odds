package odds

import (
	"strings"
	"testing"
	"time"
)

func TestSortOrgRowsPutsTestedOpenLanesFirstAndFailuresLast(t *testing.T) {
	rows := []OrgRow{
		{Repo: "untested", Odds: &RepoOdds{Repo: "o/untested", Outsiders: Group{Closed: 2, Merged: 2}, Merges30d: 90}},
		{Repo: "broken", Err: "boom"},
		{Repo: "selective", Odds: &RepoOdds{Repo: "o/selective", Outsiders: Group{Closed: 20, Merged: 10}, Merges30d: 5}},
		{Repo: "open", Odds: &RepoOdds{Repo: "o/open", Outsiders: Group{Closed: 10, Merged: 9}, Merges30d: 40}},
		{Repo: "open-quiet", Odds: &RepoOdds{Repo: "o/open-quiet", Outsiders: Group{Closed: 10, Merged: 9}, Merges30d: 2}},
	}
	SortOrgRows(rows)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Repo)
	}
	want := "open open-quiet selective untested broken"
	if strings.Join(got, " ") != want {
		t.Fatalf("order = %q, want %q", strings.Join(got, " "), want)
	}
}

func TestRenderOrgShowsNumbersAndFailures(t *testing.T) {
	rows := []OrgRow{
		{Repo: "a", Odds: &RepoOdds{Repo: "o/a", Verdict: "OPEN LANE", Stars: 12, Merges30d: 7, OpenOutsider: 1,
			Outsiders: Group{Closed: 11, Merged: 9, MedianFirstResp: 3 * time.Hour, MedianToMerge: 2 * 24 * time.Hour}}},
		{Repo: "b", Err: "404"},
	}
	out := RenderOrg("o", rows)
	for _, want := range []string{"o/a", "OPEN LANE", "9/11", "3h", "2.0d", "FETCH-FAILED 404"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
}
