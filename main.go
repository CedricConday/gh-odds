// gh-odds: will they merge my PR?
//
//	gh odds repo owner/name     merge odds for outsiders, from the last 100 closed PRs
//	gh odds mine                your open PRs: owed replies, failures not on base, conflicts
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/CedricConday/gh-odds/internal/odds"
)

const version = "0.2.0"

func usage() {
	fmt.Fprintln(os.Stderr, "usage:\n  gh odds repo <owner/name> [--sample N] [--json]\n  gh odds org <owner> [--limit N] [--sample N] [--json]\n  gh odds mine [--json]\n  gh odds <owner/name>          (same as repo)")
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "--version", "version":
		fmt.Println("gh-odds", version)
		return
	case "-h", "--help", "help":
		usage()
	case "repo", "mine", "org":
	default:
		if strings.Contains(cmd, "/") {
			args = append([]string{cmd}, args...)
			cmd = "repo"
		} else {
			usage()
		}
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	sample := fs.Int("sample", 100, "closed PRs to sample per repository (max 100 per page)")
	asJSON := fs.Bool("json", false, "machine-readable output")
	limit := fs.Int("limit", 10, "org: repositories to examine, most recently pushed first (max 50)")
	var target string
	if cmd == "repo" || cmd == "org" {
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			usage()
		}
		target = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		usage()
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(os.Stderr, "gh-odds: unexpected argument %q\n", fs.Args()[0])
		usage()
	}
	if *sample < 1 || *sample > 100 {
		fmt.Fprintln(os.Stderr, "gh-odds: --sample must be between 1 and 100")
		os.Exit(2)
	}
	if *limit < 1 || *limit > 50 {
		fmt.Fprintln(os.Stderr, "gh-odds: --limit must be between 1 and 50")
		os.Exit(2)
	}
	client, err := odds.NewClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gh-odds:", err)
		os.Exit(1)
	}
	now := time.Now().UTC()
	switch cmd {
	case "repo":
		o, err := repoOdds(client, target, *sample, now)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gh-odds:", err)
			os.Exit(1)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(o)
			return
		}
		fmt.Print(odds.RenderRepo(o))
	case "org":
		if strings.Contains(target, "/") {
			fmt.Fprintln(os.Stderr, "gh-odds: org takes an owner, not owner/name")
			os.Exit(2)
		}
		names, err := client.FetchOwnerRepos(target, *limit)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gh-odds:", err)
			os.Exit(1)
		}
		rows := orgOdds(client, target, names, *sample, now)
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]interface{}{"owner": target, "repos": rows})
			return
		}
		fmt.Print(odds.RenderOrg(target, rows))
	case "mine":
		login, err := client.Login()
		if err != nil {
			fmt.Fprintln(os.Stderr, "gh-odds:", err)
			os.Exit(1)
		}
		mine, err := client.FetchMine(login)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gh-odds:", err)
			os.Exit(1)
		}
		rows, repos := triageAll(client, mine, *sample, now)
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]interface{}{"login": login, "rows": rows, "repos": repos})
			return
		}
		fmt.Print(odds.RenderMine(rows, login))
	}
}

// orgOdds computes the card for every repository, three at a time, and sorts open lanes first.
func orgOdds(client *odds.Client, owner string, names []string, sample int, now time.Time) []odds.OrgRow {
	rows := make([]odds.OrgRow, len(names))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r, err := client.FetchRepo(owner, name, sample)
			if err != nil {
				rows[i] = odds.OrgRow{Repo: name, Err: err.Error()}
				return
			}
			o := odds.Compute(r, now)
			rows[i] = odds.OrgRow{Repo: name, Odds: &o}
		}(i, name)
	}
	wg.Wait()
	odds.SortOrgRows(rows)
	return rows
}

func repoOdds(client *odds.Client, target string, sample int, now time.Time) (odds.RepoOdds, error) {
	parts := strings.SplitN(target, "/", 2)
	if len(parts) != 2 {
		return odds.RepoOdds{}, fmt.Errorf("expected owner/name, got %q", target)
	}
	r, err := client.FetchRepo(parts[0], parts[1], sample)
	if err != nil {
		return odds.RepoOdds{}, err
	}
	return odds.Compute(r, now), nil
}

type repoState struct {
	odds *odds.RepoOdds
	base map[string]bool
	err  string
}

func triageAll(client *odds.Client, mine []odds.MinePR, sample int, now time.Time) ([]odds.Row, map[string]*odds.RepoOdds) {
	states := map[string]*repoState{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3) // gentle: three repositories at a time keeps clear of secondary limits
	for _, m := range mine {
		key := m.Owner + "/" + m.Name
		mu.Lock()
		if _, seen := states[key]; seen {
			mu.Unlock()
			continue
		}
		states[key] = &repoState{}
		mu.Unlock()
		wg.Add(1)
		go func(owner, name, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			st := &repoState{}
			r, err := client.FetchRepo(owner, name, sample)
			if err != nil {
				st.err = "odds: " + err.Error()
			} else {
				o := odds.Compute(r, now)
				st.odds = &o
				if base, err := client.FetchBaseFailing(owner, name, r.DefaultBranch); err == nil {
					st.base = base
				} else {
					st.err = "base branch checks: " + err.Error()
				}
			}
			mu.Lock()
			states[key] = st
			mu.Unlock()
		}(m.Owner, m.Name, key)
	}
	wg.Wait()
	rows := []odds.Row{}
	repos := map[string]*odds.RepoOdds{}
	for _, m := range mine {
		key := m.Owner + "/" + m.Name
		st := states[key]
		row := odds.Triage(m, st.base, st.odds, now)
		row.Error = st.err
		if st.err != "" {
			fmt.Fprintf(os.Stderr, "gh-odds: %s: %s\n", key, st.err)
		}
		rows = append(rows, row)
		if st.odds != nil {
			repos[key] = st.odds
		}
	}
	odds.SortRows(rows)
	return rows, repos
}
