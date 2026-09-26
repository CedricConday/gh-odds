package odds

import (
	"sort"
	"time"
)

// Row is one of the caller's open PRs, triaged.
type Row struct {
	Repo      string
	Number    int
	Title     string
	AgeDays   int
	IdleDays  int
	Flags     []string
	Owed      bool   // they spoke last and no push since
	Responder string // who spoke last (non-author human)
	LastAt    time.Time
	Failing   []string // checks failing on the PR but not on the base branch
	BaseRed   []string // checks failing on the PR and on the base branch
	Pending   int
	Passing   int
	Odds      *RepoOdds `json:"-"`
	OddsRepo  string    // key into the repos map of the JSON output
	Error     string    // why the repository's odds or base state could not be fetched
}

// Triage classifies one PR against its repository's base-branch state.
// IDLE is days since the last activity by anyone: a human response, the
// author's own comment, or the author's push.
func Triage(m MinePR, baseFailing map[string]bool, odds *RepoOdds, now time.Time) Row {
	p := m.PR
	r := Row{Repo: m.Owner + "/" + m.Name, Number: p.Number, Title: p.Title, Odds: odds, OddsRepo: m.Owner + "/" + m.Name, Flags: []string{}}
	r.AgeDays = int(now.Sub(p.CreatedAt).Hours() / 24)
	last := p.CreatedAt
	for _, t := range []time.Time{p.LastResponse, p.LastAuthorPush, p.LastAuthorComment} {
		if t.After(last) {
			last = t
		}
	}
	r.IdleDays = int(now.Sub(last).Hours() / 24)
	answered := p.LastAuthorPush
	if p.LastAuthorComment.After(answered) {
		answered = p.LastAuthorComment
	}
	for _, c := range p.Checks {
		switch {
		case CheckFailed(c):
			if baseFailing[c.Name] {
				r.BaseRed = append(r.BaseRed, c.Name)
			} else {
				r.Failing = append(r.Failing, c.Name)
			}
		case c.Conclusion == "SUCCESS" || c.Conclusion == "NEUTRAL" || c.Conclusion == "SKIPPED":
			r.Passing++
		default:
			r.Pending++
		}
	}
	r.Responder = p.LastResponder
	r.LastAt = p.LastResponse
	// They spoke last, so a reply is owed, unless the author pushed or commented after.
	r.Owed = !p.LastResponse.IsZero() && !answered.After(p.LastResponse)
	if len(r.Failing) > 0 {
		r.Flags = append(r.Flags, "CI-FAIL")
	}
	if len(r.BaseRed) > 0 && len(r.Failing) == 0 {
		r.Flags = append(r.Flags, "CI-RED-ON-BASE")
	}
	if p.Mergeable == "CONFLICTING" {
		r.Flags = append(r.Flags, "CONFLICT")
	}
	if r.Owed {
		r.Flags = append(r.Flags, "OWES-REPLY")
	} else if p.ReviewDecision == "CHANGES_REQUESTED" && !p.LastResponse.IsZero() && p.LastAuthorPush.After(p.LastResponse) {
		r.Flags = append(r.Flags, "ANSWERED-BY-PUSH")
	} else if p.ReviewDecision == "CHANGES_REQUESTED" && !p.LastResponse.IsZero() {
		r.Flags = append(r.Flags, "CHANGES-REQ")
	}
	if p.IsDraft {
		r.Flags = append(r.Flags, "DRAFT")
	}
	if odds != nil && (odds.Verdict == "DORMANT" || odds.Verdict == "ARCHIVED") {
		r.Flags = append(r.Flags, "REPO-"+odds.Verdict)
	}
	return r
}

// NeedsMe is true for the flags that ask something of the caller.
func (r Row) NeedsMe() bool {
	for _, f := range r.Flags {
		switch f {
		case "OWES-REPLY", "CI-FAIL", "CONFLICT", "CHANGES-REQ":
			return true
		}
	}
	return false
}

// SortRows puts what needs the caller first, then the oldest.
func SortRows(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		ai, aj := rows[i].NeedsMe(), rows[j].NeedsMe()
		if ai != aj {
			return ai
		}
		return rows[i].AgeDays > rows[j].AgeDays
	})
}
