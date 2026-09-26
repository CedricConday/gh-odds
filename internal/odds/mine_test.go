package odds

import (
	"strings"
	"testing"
	"time"
)

func minePR(created time.Time) MinePR {
	return MinePR{Owner: "o", Name: "r", PR: PR{Number: 7, Title: "t", Author: "me", CreatedAt: created, Mergeable: "MERGEABLE"}}
}

func flags(r Row) string { return strings.Join(r.Flags, ",") }

func TestOwedVersusAnsweredByPush(t *testing.T) {
	created := now.Add(-10 * 24 * time.Hour)
	m := minePR(created)
	m.PR.LastResponse = now.Add(-3 * 24 * time.Hour)
	m.PR.LastResponder = "reviewer"
	m.PR.ReviewDecision = "CHANGES_REQUESTED"
	r := Triage(m, nil, nil, now)
	if !r.Owed || !strings.Contains(flags(r), "OWES-REPLY") || r.IdleDays != 3 {
		t.Fatalf("owed: %+v", r)
	}
	m.PR.LastAuthorPush = now.Add(-2 * 24 * time.Hour) // pushed after the review
	r = Triage(m, nil, nil, now)
	if r.Owed || !strings.Contains(flags(r), "ANSWERED-BY-PUSH") || strings.Contains(flags(r), "CHANGES-REQ") {
		t.Fatalf("answered: %+v", r)
	}
	m.PR.LastAuthorPush = time.Time{}
	m.PR.LastAuthorComment = now.Add(-2 * 24 * time.Hour) // answered in words instead
	r = Triage(m, nil, nil, now)
	if r.Owed || r.IdleDays != 2 {
		t.Fatalf("comment should clear the debt: %+v", r)
	}
	if !strings.Contains(flags(r), "CHANGES-REQ") { // still requested, answered by comment not code
		t.Fatalf("flags %s", flags(r))
	}
}

func TestMergeCommitsAndCancelledRuns(t *testing.T) {
	var n prNode
	n.Number = 1
	n.Author = &actor{Login: "me", Typename: "User"}
	n.Commits.Nodes = append(n.Commits.Nodes, struct {
		Commit struct {
			CommittedDate time.Time
			Parents       struct{ TotalCount int }
			Author        struct {
				User *struct{ Login string }
			}
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []struct {
						Typename   string `json:"__typename"`
						Name       string
						Conclusion string
						Status     string
						DetailsURL string `json:"detailsUrl"`
						Context    string
						State      string
						TargetURL  string `json:"targetUrl"`
					}
				}
			}
		}
	}{})
	c := &n.Commits.Nodes[0].Commit
	c.CommittedDate = now
	c.Parents.TotalCount = 2
	c.Author.User = &struct{ Login string }{"me"}
	p := toPR(n)
	if !p.LastAuthorPush.IsZero() {
		t.Fatal("a merge commit must not count as an answering push")
	}
	if CheckFailed(Check{Conclusion: "CANCELLED"}) || CheckFailed(Check{Conclusion: "ACTION_REQUIRED"}) || !CheckFailed(Check{Conclusion: "FAILURE"}) {
		t.Fatal("failure set")
	}
}

func TestBaseRedIsNotMyFailure(t *testing.T) {
	m := minePR(now.Add(-24 * time.Hour))
	m.PR.Checks = []Check{{Name: "flaky", Conclusion: "FAILURE"}, {Name: "lint", Conclusion: "FAILURE"}, {Name: "docs", Conclusion: "SUCCESS"}, {Name: "slow", Conclusion: "PENDING"}}
	r := Triage(m, map[string]bool{"flaky": true}, nil, now)
	if len(r.Failing) != 1 || r.Failing[0] != "lint" || len(r.BaseRed) != 1 || r.Passing != 1 || r.Pending != 1 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(flags(r), "CI-FAIL") || strings.Contains(flags(r), "CI-RED-ON-BASE") {
		t.Fatalf("flags %s", flags(r))
	}
	m.PR.Checks = []Check{{Name: "flaky", Conclusion: "FAILURE"}}
	r = Triage(m, map[string]bool{"flaky": true}, nil, now)
	if flags(r) != "CI-RED-ON-BASE" {
		t.Fatalf("flags %s", flags(r))
	}
}

func TestConflictDraftAndDormantRepo(t *testing.T) {
	m := minePR(now.Add(-24 * time.Hour))
	m.PR.Mergeable = "CONFLICTING"
	m.PR.IsDraft = true
	o := &RepoOdds{Verdict: "DORMANT"}
	r := Triage(m, nil, o, now)
	if flags(r) != "CONFLICT,DRAFT,REPO-DORMANT" {
		t.Fatalf("flags %s", flags(r))
	}
}

func TestSortRowsPutsActionableFirstThenOldest(t *testing.T) {
	rows := []Row{{AgeDays: 5}, {AgeDays: 50, Flags: []string{"REPO-DORMANT"}}, {AgeDays: 1, Flags: []string{"OWES-REPLY"}}, {AgeDays: 9, Flags: []string{"CONFLICT"}}}
	SortRows(rows)
	if rows[0].AgeDays != 9 || rows[1].AgeDays != 1 || rows[2].AgeDays != 50 || rows[3].AgeDays != 5 {
		t.Fatalf("%+v", rows)
	}
}

func TestTruncIsRuneSafe(t *testing.T) {
	if got := trunc("Fix Umlaute fuer Ärzte", 19); got != "Fix Umlaute fuer Är" {
		t.Fatalf("%q", got)
	}
}
