package odds

import (
	"fmt"
	"strings"
)

func pct(f float64) string {
	if f < 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", f*100)
}

// RenderRepo prints the odds card for one repository.
func RenderRepo(o RepoOdds) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s   stars %d   default %s\n", o.Repo, o.Stars, o.DefaultBranch)
	if o.SampleClosed > 0 {
		fmt.Fprintf(&b, "sample: %d closed PRs, %s to %s, plus the open ones\n", o.SampleClosed,
			o.SampleFrom.Format("2006-01-02"), o.SampleTo.Format("2006-01-02"))
	}
	fmt.Fprintf(&b, "outsiders: %d closed, %d merged (%s)   median first response %s   median time to merge %s\n",
		o.Outsiders.Closed, o.Outsiders.Merged, pct(o.Outsiders.MergeRate()),
		Humanize(o.Outsiders.MedianFirstResp), Humanize(o.Outsiders.MedianToMerge))
	fmt.Fprintf(&b, "members:   %d closed, %d merged (%s)   median time to merge %s\n",
		o.Members.Closed, o.Members.Merged, pct(o.Members.MergeRate()), Humanize(o.Members.MedianToMerge))
	var who []string
	for i, m := range o.Mergers {
		if i == 4 {
			break
		}
		who = append(who, fmt.Sprintf("%s %d", m.Login, m.Count))
	}
	fmt.Fprintf(&b, "merges in the last 30 days: %d   who merges: %s\n", o.Merges30d, strings.Join(who, ", "))
	fmt.Fprintf(&b, "open outsider PRs: %d, %d older than 60 days, oldest %dd\n", o.OpenOutsider, o.OpenOutsiderOld, o.OldestOpenDays)
	fmt.Fprintf(&b, "verdict: %s: %s\n", o.Verdict, o.VerdictWhy)
	return b.String()
}

// RenderMine prints the triage board.
func RenderMine(rows []Row, login string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-36s %6s %4s %4s %-14s %-11s %s\n", "REPO", "#", "AGE", "IDLE", "REPO ODDS", "CHECKS", "FLAGS / TITLE")
	owed, fail, conf := 0, 0, 0
	for _, r := range rows {
		odds := "-"
		if r.Odds != nil {
			odds = r.Odds.Verdict
			if rate := r.Odds.Outsiders.MergeRate(); rate >= 0 {
				odds = fmt.Sprintf("%s %s", pct(rate), r.Odds.Verdict)
			}
		}
		checks := fmt.Sprintf("%dp/%df/%d~", r.Passing, len(r.Failing)+len(r.BaseRed), r.Pending)
		flags := strings.Join(r.Flags, ",")
		if flags == "" {
			flags = "-"
		}
		title := r.Title
		if len(title) > 44 {
			title = title[:44]
		}
		fmt.Fprintf(&b, "%-36s %6d %4d %4d %-14s %-11s %s  %s\n", trunc(r.Repo, 36), r.Number, r.AgeDays, r.IdleDays, trunc(odds, 14), checks, flags, title)
		if r.Owed {
			owed++
		}
		if len(r.Failing) > 0 {
			fail++
		}
		for _, f := range r.Flags {
			if f == "CONFLICT" {
				conf++
			}
		}
	}
	fmt.Fprintf(&b, "\n%d open PRs by %s  |  %d owe a reply  |  %d with a failure that is not on the base branch  |  %d conflicted\n", len(rows), login, owed, fail, conf)
	for _, r := range rows {
		if r.Owed {
			fmt.Fprintf(&b, "  OWED   %s#%d  %s spoke last, %s\n", r.Repo, r.Number, r.Responder, r.LastAt.Format("2006-01-02"))
		}
	}
	for _, r := range rows {
		if len(r.Failing) > 0 {
			fmt.Fprintf(&b, "  FAIL   %s#%d  %s\n", r.Repo, r.Number, strings.Join(r.Failing, ", "))
		}
		if len(r.BaseRed) > 0 && len(r.Failing) == 0 {
			fmt.Fprintf(&b, "  BASE   %s#%d  also red on %s: %s\n", r.Repo, r.Number, "the base branch", strings.Join(r.BaseRed, ", "))
		}
	}
	if owed == 0 && fail == 0 && conf == 0 {
		b.WriteString("  board is clean; everything is parked on their side\n")
	}
	return b.String()
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
