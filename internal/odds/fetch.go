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

var bots = []string{"[bot]", "dependabot", "renovate", "github-actions", "pre-commit-ci", "codecov",
	"allcontributors", "copilot", "claassistant", "cla-bot", "netlify", "vercel", "sonarcloud", "codspeed",
	"azure-pipelines", "circleci", "coderabbit", "sourcery", "greptile", "welcome", "stale", "mergify",
	"lumberbot", "meeseeks", "review-notebook-app", "gitguardian", "licensebot", "cubic-dev-ai", "-bot"}

// IsBot guesses from the login; GitHub's __typename catches the rest.
func IsBot(login, typename string) bool {
	if typename == "Bot" {
		return true
	}
	l := strings.ToLower(login)
	for _, b := range bots {
		if strings.Contains(l, b) {
			return true
		}
	}
	return false
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
  reviews(first: 5) { nodes { submittedAt createdAt author { login __typename } } }
  comments(first: 5) { nodes { createdAt author { login __typename } } }
  commits(last: 1) { nodes { commit { committedDate author { user { login } } } } }`

const prFieldsFull = `
  number title createdAt closedAt mergedAt merged isDraft mergeable reviewDecision authorAssociation
  author { login __typename }
  mergedBy { login __typename }
  reviews(last: 30) { nodes { submittedAt createdAt author { login __typename } } }
  comments(last: 30) { nodes { createdAt author { login __typename } } }
  commits(last: 30) { nodes { commit { committedDate author { user { login } }
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
		if a == nil || a.Login == p.Author || IsBot(a.Login, a.Typename) || at.IsZero() {
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
		if c.Commit.Author.User != nil && c.Commit.Author.User.Login == p.Author && c.Commit.CommittedDate.After(p.LastAuthorPush) {
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

// do runs a GraphQL query with two retries on gateway errors.
func (c *Client) do(query string, vars map[string]interface{}, resp interface{}) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = c.gql.Do(query, vars, resp); err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "502") && !strings.Contains(err.Error(), "503") && !strings.Contains(err.Error(), "504") {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
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
	return r, nil
}

// FetchBaseFailing lists check names failing on the default branch head, so a
// red check on a PR can be told apart from a red base.
func (c *Client) FetchBaseFailing(owner, name, branch string) (map[string]bool, error) {
	if branch == "" {
		return nil, nil
	}
	var resp struct {
		CheckRuns []struct {
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	path := fmt.Sprintf("repos/%s/%s/commits/%s/check-runs?per_page=100", owner, name, branch)
	if err := c.rest.Get(path, &resp); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, cr := range resp.CheckRuns {
		if CheckFailed(Check{Conclusion: strings.ToUpper(cr.Conclusion)}) {
			out[cr.Name] = true
		}
	}
	var statuses struct {
		Statuses []struct {
			Context string `json:"context"`
			State   string `json:"state"`
		} `json:"statuses"`
	}
	if err := c.rest.Get(fmt.Sprintf("repos/%s/%s/commits/%s/status", owner, name, branch), &statuses); err == nil {
		for _, s := range statuses.Statuses {
			if strings.ToUpper(s.State) == "FAILURE" || strings.ToUpper(s.State) == "ERROR" {
				out[s.Context] = true
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
	q := fmt.Sprintf("is:pr is:open author:%s archived:false", login)
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
