// Package odds computes contributor-side merge odds for a repository and a
// triage board for the caller's own open pull requests.
package odds

import "time"

// PR is the slice of a pull request the odds need. Fetched from GraphQL,
// or built by tests from fixtures.
type PR struct {
	Number            int
	Title             string
	Author            string
	AuthorIsBot       bool
	Association       string // OWNER, MEMBER, COLLABORATOR, CONTRIBUTOR, FIRST_TIME_CONTRIBUTOR, FIRST_TIMER, NONE
	IsDraft           bool
	CreatedAt         time.Time
	ClosedAt          time.Time // zero when open
	MergedAt          time.Time // zero unless merged
	Merged            bool
	MergedBy          string
	FirstResponse     time.Time // earliest review or comment by someone other than the author (zero if none)
	LastResponse      time.Time // latest review or comment by a non-author human
	LastResponder     string
	LastAuthorPush    time.Time // latest non-merge commit on the PR authored by the PR author
	LastAuthorComment time.Time // latest comment or review by the PR author
	Mergeable         string    // MERGEABLE, CONFLICTING, UNKNOWN
	ReviewDecision    string    // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, ""
	Checks            []Check
}

// Check is one entry of the head commit's status check rollup.
type Check struct {
	Name       string
	Conclusion string // SUCCESS, FAILURE, ERROR, TIMED_OUT, CANCELLED, NEUTRAL, SKIPPED, PENDING, ...
	URL        string
}

// Repo is what the fetcher returns for one repository.
type Repo struct {
	NameWithOwner  string
	Stars          int
	Archived       bool
	DefaultBranch  string
	Closed         []PR // recently updated merged or closed PRs, newest update first
	Open           []PR
	FetchedAt      time.Time
	Merges30d      int             // merged PRs in the last 30 days, from a search count, not the sample
	Merges30dKnown bool            // false when the search count could not be fetched
	BaseFailing    map[string]bool // check names failing on recent default-branch commits
}

// Outsider is true for authors without write access as GitHub reports it.
func Outsider(association string) bool {
	switch association {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return false
	}
	return true
}

func (p PR) IsOpen() bool { return p.ClosedAt.IsZero() }

// CheckFailed reports whether a rollup entry is a failure state. CANCELLED is a
// superseded run and ACTION_REQUIRED a workflow awaiting approval; neither is a
// failure of the change.
func CheckFailed(c Check) bool {
	switch c.Conclusion {
	case "FAILURE", "ERROR", "TIMED_OUT", "STARTUP_FAILURE":
		return true
	}
	return false
}
