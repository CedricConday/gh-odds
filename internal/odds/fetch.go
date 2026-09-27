package odds

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// Client wraps the go-gh clients so the rest of the package stays testable.
type Client struct {
	gql  *api.GraphQLClient
	rest *api.RESTClient
}

// NewClient uses the caller's gh authentication.
func NewClient() (*Client, error) {
	gql, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, err
	}
	return &Client{gql: gql, rest: rest}, nil
}

var botLogins = map[string]bool{
	"dependabot": true, "renovate": true, "github-actions": true, "pre-commit-ci": true, "codecov": true,
	"allcontributors": true, "copilot": true, "claassistant": true, "cla-bot": true, "netlify": true,
	"vercel": true, "sonarcloud": true, "codspeed": true, "azure-pipelines": true, "circleci": true,
	"coderabbit": true, "coderabbitai": true, "sourcery-ai": true, "greptile-apps": true, "mergify": true,
	"bors": true, "lumberbot": true, "meeseeksdev": true, "review-notebook-app": true, "gitguardian": true,
	"cubic-dev-ai": true, "welcome": true, "stale": true, "imgbot": true, "codacy-production": true,
}

// IsBot: GitHub's Bot type, the [bot] suffix, a known bot login, or a -bot suffix.
func IsBot(login, typename string) bool {
	if typename == "Bot" {
		return true
	}
	l := strings.ToLower(login)
	if strings.HasSuffix(l, "[bot]") || strings.HasSuffix(l, "-bot") || strings.HasSuffix(l, "_bot") {
		return true
	}
	return botLogins[l]
}

type actor struct {
	Login    string `json:"login"`
	Typename string `json:"__typename"`
}

type prNode struct {
	Number            int    `json:"number"`
	Title             string `json:"title"`
	CreatedAt         time.Time
	ClosedAt          *time.Time
	MergedAt          *time.Time
	Merged            bool
	IsDraft           bool
	Mergeable         string
	ReviewDecision    string
	AuthorAssociation string
	Author            *actor
	MergedBy          *actor
	Reviews           struct {
		Nodes []struct {
			SubmittedAt *time.Time
			CreatedAt   time.Time
			Author      *actor
		}
	}
	Comments struct {
		Nodes []struct {
			CreatedAt time.Time
			Author    *actor
		}
	}
	Commits struct {
		Nodes []struct {
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
		}
	}
}

const prFields = `
  number title createdAt closedAt mergedAt merged isDraft mergeable reviewDecision authorAssociation
  author { login __typename }
  mergedBy { login __typename }
  reviews(first: 20) { nodes { submittedAt createdAt author { login __typename } } }
  comments(first: 20) { nodes { createdAt author { login __typename } } }
  commits(last: 1) { nodes { commit { committedDate parents { totalCount } author { user { login } } } } }`

const prFieldsFull = `
  number title createdAt closedAt mergedAt merged isDraft mergeable reviewDecision authorAssociation
  author { login __typename }
  mergedBy { login __typename }
  reviews(last: 30) { nodes { submittedAt createdAt author { login __typename } } }
  comments(last: 30) { nodes { createdAt author { login __typename } } }
  commits(last: 30) { nodes { commit { committedDate parents { totalCount } author { user { login } }
    statusCheckRollup { contexts(first: 60) { nodes { __typename
      ... on CheckRun { name conclusion status detailsUrl }
      ... on StatusContext { context state targetUrl } } } } } } }`

func toPR(n prNode) PR {
	p := PR{Number: n.Number, Title: n.Title, CreatedAt: n.CreatedAt, Merged: n.Merged, IsDraft: n.IsDraft,
		Mergeable: n.Mergeable, ReviewDecision: n.ReviewDecision, Association: n.AuthorAssociation}
	if n.Author != nil {
		p.Author = n.Author.Login
		p.AuthorIsBot = IsBot(n.Author.Login, n.Author.Typename)
	}
	if n.ClosedAt != nil {
		p.ClosedAt = *n.ClosedAt
	}
	if n.MergedAt != nil {
		p.MergedAt = *n.MergedAt
	}
	if n.MergedBy != nil {
		p.MergedBy = n.MergedBy.Login
	}
	consider := func(at time.Time, a *actor) {
		if a == nil || at.IsZero() || IsBot(a.Login, a.Typename) {
			return
		}
		if a.Login == p.Author {
			if at.After(p.LastAuthorComment) {
				p.LastAuthorComment = at
			}
			return
		}
		if p.FirstResponse.IsZero() || at.Before(p.FirstResponse) {
			p.FirstResponse = at
		}
		if at.After(p.LastResponse) {
			p.LastResponse = at
			p.LastResponder = a.Login
		}
	}
	for _, r := range n.Reviews.Nodes {
		at := r.CreatedAt
		if r.SubmittedAt != nil {
			at = *r.SubmittedAt
		}
		consider(at, r.Author)
	}
	for _, c := range n.Comments.Nodes {
		consider(c.CreatedAt, c.Author)
	}
	for i, c := range n.Commits.Nodes {
		// A merge commit ("update branch") is not an answer to a review.
		if c.Commit.Parents.TotalCount <= 1 && c.Commit.Author.User != nil && c.Commit.Author.User.Login == p.Author && c.Commit.CommittedDate.After(p.LastAuthorPush) {
			p.LastAuthorPush = c.Commit.CommittedDate
		}
		if i == len(n.Commits.Nodes)-1 && c.Commit.StatusCheckRollup != nil {
			for _, x := range c.Commit.StatusCheckRollup.Contexts.Nodes {
				if x.Typename == "StatusContext" {
					p.Checks = append(p.Checks, Check{Name: x.Context, Conclusion: x.State, URL: x.TargetURL})
				} else {
					concl := x.Conclusion
					if concl == "" {
						concl = x.Status
					}
					p.Checks = append(p.Checks, Check{Name: x.Name, Conclusion: concl, URL: x.DetailsURL})
				}
			}
		}
	}
	return p
}

// do runs a GraphQL query, retrying gateway errors and secondary rate limits.
func (c *Client) do(query string, vars map[string]interface{}, resp interface{}) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if err = c.gql.Do(query, vars, resp); err == nil {
			return nil
		}
		msg := err.Error()
		switch {
		case strings.Contains(msg, "502"), strings.Contains(msg, "503"), strings.Contains(msg, "504"):
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
		case strings.Contains(msg, "403"), strings.Contains(msg, "429"), strings.Contains(strings.ToLower(msg), "rate limit"):
			time.Sleep(time.Duration(attempt+1) * 15 * time.Second)
		default:
			return err
		}
	}
	return err
}

// FetchRepo samples the most recently updated closed PRs and the open PRs.
func (c *Client) FetchRepo(owner, name string, sample int) (Repo, error) {
	var meta struct {
		Repository struct {
			NameWithOwner    string
			StargazerCount   int
			IsArchived       bool
			DefaultBranchRef *struct{ Name string }
		}
	}
	if err := c.do(`query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) {
  nameWithOwner stargazerCount isArchived defaultBranchRef { name } } }`,
		map[string]interface{}{"owner": owner, "name": name}, &meta); err != nil {
		return Repo{}, err
	}
	r := Repo{NameWithOwner: meta.Repository.NameWithOwner, Stars: meta.Repository.StargazerCount,
		Archived: meta.Repository.IsArchived, FetchedAt: time.Now().UTC()}
	if meta.Repository.DefaultBranchRef != nil {
		r.DefaultBranch = meta.Repository.DefaultBranchRef.Name
	}
	fetch := func(states string) ([]PR, error) {
		var resp struct {
			Repository struct {
				PullRequests struct{ Nodes []prNode }
			}
		}
		query := fmt.Sprintf(`query($owner: String!, $name: String!, $n: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequests(last: $n, states: [%s], orderBy: {field: UPDATED_AT, direction: ASC}) { nodes { %s } }
  }
}`, states, prFields)
		if err := c.do(query, map[string]interface{}{"owner": owner, "name": name, "n": sample}, &resp); err != nil {
			return nil, err
		}
		var out []PR
		for _, n := range resp.Repository.PullRequests.Nodes {
			out = append(out, toPR(n))
		}
		return out, nil
	}
	var err error
	if r.Closed, err = fetch("MERGED, CLOSED"); err != nil {
		return Repo{}, err
	}
	if r.Open, err = fetch("OPEN"); err != nil {
		return Repo{}, err
	}
	sort.Slice(r.Closed, func(i, j int) bool { return r.Closed[i].ClosedAt.After(r.Closed[j].ClosedAt) })
	// Exact count of merges in the last 30 days; the sample is bounded by 100 and by update time.
	var count struct {
		Search struct{ IssueCount int }
	}
	since := time.Now().UTC().Add(-30 * 24 * time.Hour).Format("2006-01-02")
	q := fmt.Sprintf("repo:%s/%s is:pr is:merged merged:>=%s", owner, name, since)
	if err := c.do(`query($q: String!) { search(query: $q, type: ISSUE, first: 1) { issueCount } }`,
		map[string]interface{}{"q": q}, &count); err == nil {
		r.Merges30d, r.Merges30dKnown = count.Search.IssueCount, true
	}
	return r, nil
}

// FetchBaseFailing lists check names that failed on any of the last few
// default-branch commits, so a red check on a PR can be told apart from a
// check that is red or flaky on the base itself.
func (c *Client) FetchBaseFailing(owner, name, branch string) (map[string]bool, error) {
	if branch == "" {
		return nil, nil
	}
	var commits []struct {
		SHA string `json:"sha"`
	}
	if err := c.rest.Get(fmt.Sprintf("repos/%s/%s/commits?sha=%s&per_page=5", owner, name, branch), &commits); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, cm := range commits {
		for page := 1; page <= 3; page++ {
			var resp struct {
				TotalCount int `json:"total_count"`
				CheckRuns  []struct {
					Name       string `json:"name"`
					Conclusion string `json:"conclusion"`
				} `json:"check_runs"`
			}
			path := fmt.Sprintf("repos/%s/%s/commits/%s/check-runs?per_page=100&page=%d", owner, name, cm.SHA, page)
			if err := c.rest.Get(path, &resp); err != nil {
				break
			}
			for _, cr := range resp.CheckRuns {
				if CheckFailed(Check{Conclusion: strings.ToUpper(cr.Conclusion)}) {
					out[cr.Name] = true
				}
			}
			if page*100 >= resp.TotalCount {
				break
			}
		}
		var statuses struct {
			Statuses []struct {
				Context string `json:"context"`
				State   string `json:"state"`
			} `json:"statuses"`
		}
		if err := c.rest.Get(fmt.Sprintf("repos/%s/%s/commits/%s/status", owner, name, cm.SHA), &statuses); err == nil {
			for _, st := range statuses.Statuses {
				if u := strings.ToUpper(st.State); u == "FAILURE" || u == "ERROR" {
					out[st.Context] = true
				}
			}
		}
	}
	return out, nil
}

// MinePR is one of the caller's open PRs with its repository.
type MinePR struct {
	Owner string
	Name  string
	PR    PR
}

// FetchMine lists the caller's open PRs with the full activity fields.
func (c *Client) FetchMine(login string) ([]MinePR, error) {
	var resp struct {
		Search struct {
			Nodes []struct {
				Typename   string `json:"__typename"`
				Repository struct {
					Owner struct{ Login string }
					Name  string
				}
				prNode
			}
		}
	}
	query := fmt.Sprintf(`query($q: String!) {
  search(query: $q, type: ISSUE, first: 100) { nodes { __typename ... on PullRequest { repository { owner { login } name } %s } } }
}`, prFieldsFull)
	q := fmt.Sprintf("is:pr is:open author:%s", login)
	if err := c.do(query, map[string]interface{}{"q": q}, &resp); err != nil {
		return nil, err
	}
	var out []MinePR
	for _, n := range resp.Search.Nodes {
		if n.Typename != "PullRequest" {
			continue
		}
		out = append(out, MinePR{Owner: n.Repository.Owner.Login, Name: n.Repository.Name, PR: toPR(n.prNode)})
	}
	return out, nil
}

// Login returns the authenticated user.
func (c *Client) Login() (string, error) {
	var u struct{ Login string }
	if err := c.rest.Get("user", &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

// FetchOwnerRepos lists an organisation's (or user's) public, non-archived, non-fork
// repositories, most recently pushed first, at most limit of them.
func (c *Client) FetchOwnerRepos(owner string, limit int) ([]string, error) {
	type repo struct {
		Name     string `json:"name"`
		Archived bool   `json:"archived"`
		Fork     bool   `json:"fork"`
	}
	var names []string
	for _, base := range []string{"orgs/" + owner + "/repos", "users/" + owner + "/repos"} {
		page := 1
		found := false
		for len(names) < limit {
			var batch []repo
			path := fmt.Sprintf("%s?type=public&sort=pushed&per_page=100&page=%d", base, page)
			if err := c.rest.Get(path, &batch); err != nil {
				if page == 1 && !found {
					break // not an org (or not a user): try the other endpoint
				}
				return names, err
			}
			found = true
			for _, r := range batch {
				if r.Archived || r.Fork {
					continue
				}
				names = append(names, r.Name)
				if len(names) >= limit {
					break
				}
			}
			if len(batch) < 100 {
				break
			}
			page++
		}
		if found {
			return names, nil
		}
	}
	return nil, fmt.Errorf("no repositories found for %q", owner)
}
