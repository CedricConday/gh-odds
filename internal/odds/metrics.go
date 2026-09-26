package odds

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Group holds the odds for one class of authors.
type Group struct {
	Closed          int
	Merged          int
	MedianFirstResp time.Duration // zero when unknown
	MedianToMerge   time.Duration
	FirstRespKnown  int
}

// MergeRate is merged over closed, or -1 with no data.
func (g Group) MergeRate() float64 {
	if g.Closed == 0 {
		return -1
	}
	return float64(g.Merged) / float64(g.Closed)
}

// RepoOdds is the computed verdict for one repository.
type RepoOdds struct {
	Repo            string
	Stars           int
	Archived        bool
	DefaultBranch   string
	SampleClosed    int
	SampleFrom      time.Time
	SampleTo        time.Time
	Outsiders       Group
	Members         Group
	Merges30d       int      // from a search count when available, else from the sample
	Merges30dExact  bool     // true when Merges30d came from the search count
	InferredMembers []string // authors reported as CONTRIBUTOR who merge PRs, treated as members
	Mergers         []Merger
	OpenOutsider    int
	OpenOutsiderOld int // older than StaleAfter
	OldestOpenDays  int
	Verdict         string
	VerdictWhy      string
}

// Merger is who merged how many PRs in the sample.
type Merger struct {
	Login string
	Count int
}

// StaleAfter is the age at which an open outsider PR counts as backlog.
const StaleAfter = 60 * 24 * time.Hour

func median(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return ds[len(ds)/2]
}

// MarshalJSON renders durations as seconds and a human string.
func (g Group) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Closed              int     `json:"closed"`
		Merged              int     `json:"merged"`
		MergeRate           float64 `json:"merge_rate"`
		FirstRespKnown      int     `json:"first_response_known"`
		MedianFirstRespSecs float64 `json:"median_first_response_seconds"`
		MedianFirstResp     string  `json:"median_first_response"`
		MedianToMergeSecs   float64 `json:"median_to_merge_seconds"`
		MedianToMerge       string  `json:"median_to_merge"`
	}{g.Closed, g.Merged, g.MergeRate(), g.FirstRespKnown, g.MedianFirstResp.Seconds(), Humanize(g.MedianFirstResp), g.MedianToMerge.Seconds(), Humanize(g.MedianToMerge)})
}

func groupOf(prs []PR) Group {
	var g Group
	var first, merge []time.Duration
	for _, p := range prs {
		if p.IsOpen() {
			continue
		}
		g.Closed++
		if p.Merged {
			g.Merged++
			merge = append(merge, p.MergedAt.Sub(p.CreatedAt))
		}
		if !p.FirstResponse.IsZero() {
			first = append(first, p.FirstResponse.Sub(p.CreatedAt))
		}
	}
	g.FirstRespKnown = len(first)
	g.MedianFirstResp = median(first)
	g.MedianToMerge = median(merge)
	return g
}

// Compute derives the odds from a fetched repository at time now.
//
// Membership is GitHub's author association, corrected by one observation:
// anyone who merges PRs in the sample is a member whatever the association
// says, because org members with private membership are reported as
// CONTRIBUTOR and would otherwise inflate the outsider merge rate.
func Compute(r Repo, now time.Time) RepoOdds {
	o := RepoOdds{Repo: r.NameWithOwner, Stars: r.Stars, Archived: r.Archived, DefaultBranch: r.DefaultBranch}
	mergers := map[string]int{}
	for _, p := range r.Closed {
		if p.Merged && p.MergedBy != "" && !IsBot(p.MergedBy, "") {
			mergers[p.MergedBy]++
		}
	}
	member := func(p PR) bool {
		if !Outsider(p.Association) {
			return true
		}
		return mergers[p.Author] > 0
	}
	inferred := map[string]bool{}
	var outs, mems []PR
	for _, p := range r.Closed {
		if p.AuthorIsBot || p.IsDraft {
			continue
		}
		o.SampleClosed++
		if o.SampleFrom.IsZero() || p.ClosedAt.Before(o.SampleFrom) {
			o.SampleFrom = p.ClosedAt
		}
		if p.ClosedAt.After(o.SampleTo) {
			o.SampleTo = p.ClosedAt
		}
		if member(p) {
			mems = append(mems, p)
			if Outsider(p.Association) {
				inferred[p.Author] = true
			}
		} else {
			outs = append(outs, p)
		}
		if p.Merged && now.Sub(p.MergedAt) <= 30*24*time.Hour {
			o.Merges30d++
		}
	}
	if r.Merges30d >= 0 && r.Merges30dKnown {
		o.Merges30d, o.Merges30dExact = r.Merges30d, true
	}
	for login := range inferred {
		o.InferredMembers = append(o.InferredMembers, login)
	}
	sort.Strings(o.InferredMembers)
	o.Outsiders = groupOf(outs)
	o.Members = groupOf(mems)
	for login, n := range mergers {
		o.Mergers = append(o.Mergers, Merger{login, n})
	}
	sort.Slice(o.Mergers, func(i, j int) bool {
		if o.Mergers[i].Count != o.Mergers[j].Count {
			return o.Mergers[i].Count > o.Mergers[j].Count
		}
		return o.Mergers[i].Login < o.Mergers[j].Login
	})
	for _, p := range r.Open {
		if p.AuthorIsBot || p.IsDraft || member(p) {
			continue
		}
		o.OpenOutsider++
		age := now.Sub(p.CreatedAt)
		if age > StaleAfter {
			o.OpenOutsiderOld++
		}
		if d := int(age.Hours() / 24); d > o.OldestOpenDays {
			o.OldestOpenDays = d
		}
	}
	o.Verdict, o.VerdictWhy = verdict(o)
	return o
}

// verdict is a rule table, not a score: each line names the fact it rests on.
// The backlog of old open PRs is reported as a fact, never as the verdict: a
// repository that merges outsiders within a day is an open lane whatever its
// graveyard looks like.
func verdict(o RepoOdds) (string, string) {
	backlog := ""
	if o.OpenOutsiderOld >= 10 {
		backlog = fmt.Sprintf("; %d open outsider PRs are older than 60 days, so expect yours to need a nudge", o.OpenOutsiderOld)
	}
	rate := o.Outsiders.MergeRate()
	fast := o.Outsiders.MedianToMerge > 0 && o.Outsiders.MedianToMerge <= 14*24*time.Hour
	switch {
	case o.Archived:
		return "ARCHIVED", "the repository is archived; nothing merges"
	case o.SampleClosed == 0:
		return "NO DATA", "no closed PRs to sample"
	case o.Merges30d == 0:
		return "DORMANT", "no PR from anyone merged in the last 30 days" + backlog
	case o.Outsiders.Closed < 5:
		return "UNTESTED", "fewer than 5 outsider PRs closed in the sample; the odds cannot be measured" + backlog
	case rate < 0.25:
		return "LONG SHOT", fmt.Sprintf("only %s of outsider PRs merge", pct(rate)) + backlog
	case rate >= 0.6 && o.Merges30d >= 5 && fast:
		return "OPEN LANE", fmt.Sprintf("%s of outsider PRs merge, median %s, %d merges in 30 days", pct(rate), Humanize(o.Outsiders.MedianToMerge), o.Merges30d) + backlog
	case rate >= 0.6:
		return "SLOW LANE", fmt.Sprintf("%s of outsider PRs merge, but median %s and %d merges in 30 days", pct(rate), Humanize(o.Outsiders.MedianToMerge), o.Merges30d) + backlog
	default:
		return "SELECTIVE", fmt.Sprintf("%s of outsider PRs merge; pick an issue a maintainer has asked for", pct(rate)) + backlog
	}
}

// Humanize renders a duration as hours or days for the tables.
func Humanize(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	h := d.Hours()
	switch {
	case h < 1:
		return "<1h"
	case h < 48:
		return fmt.Sprintf("%.0fh", h)
	default:
		return fmt.Sprintf("%.1fd", h/24)
	}
}
