# gh-odds

**Will they merge my PR?** A `gh` extension that measures a repository's odds for
outsiders before you spend a week on it, and triages your own open PRs into what needs
you and what is parked on their side.

```bash
gh extension install CedricConday/gh-odds
gh odds repo mne-tools/mne-python
gh odds mine
```

## `gh odds repo owner/name`

```
anthropics/claude-code   stars 148186   default main
sample: the 100 most recently updated closed PRs, closed 2025-02-24 to 2026-09-25, plus the open ones
outsiders: 69 closed, 1 merged (1%)   median first response 16h (known for 19)   median time to merge 47.9d
members:   31 closed, 30 merged (97%)   median time to merge 2h
merges in the last 30 days: 27   who merges: poteat 24, bcherny 2, notitatall 1, roy-ant 1
counted as members because they merge PRs: poteat
open outsider PRs: 92, 65 older than 60 days, oldest 291d
verdict: LONG SHOT: only 1% of outsider PRs merge; 65 open outsider PRs are older than 60 days, so expect yours to need a nudge
```

```
mne-tools/mne-python   stars 3528   default main
outsiders: 59 closed, 58 merged (98%)   median first response 10h   median time to merge 27h
members:   38 closed, 38 merged (100%)   median time to merge 4h
merges in the last 30 days: 100   who merges: larsoner 80, drammock 5, wmvanvliet 3
counted as members because they merge PRs: cbrnr, tsbinns, wmvanvliet
open outsider PRs: 59, 52 older than 60 days, oldest 1882d
verdict: OPEN LANE: 98% of outsider PRs merge, median 27h, 100 merges in 30 days; 52 open outsider PRs are older than 60 days, so expect yours to need a nudge
```

A repository with 148k stars where one outsider PR in seventy merges, and one with 3.5k
stars that merges outsiders within a day. That is the number a star count cannot give
you.

**Outsider** starts from what GitHub reports (an author association of CONTRIBUTOR,
FIRST_TIME_CONTRIBUTOR, FIRST_TIMER or NONE; OWNER, MEMBER and COLLABORATOR are members)
with one correction: anyone who merges PRs in the sample is a member whatever the
association says, because org members with private membership are reported as
CONTRIBUTOR. On anthropics/claude-code that correction moves the outsider merge rate from
28 % to under 2 %. The card names who was reclassified. Bot authors, bot responders and
bot mergers are excluded.

**Sample**: the 100 most recently *updated* merged or closed PRs and the 100 most recently
updated open ones (`--sample N`, at most 100, the API's page limit). Update time, not
close time: a years-old PR that got a comment last week is in the sample, so the dates on
the card can reach far back on a busy repository. The 30-day merge count does not depend
on the sample; it is a separate search count. Closed drafts are excluded from the rate,
open drafts from the backlog.

**Verdict** is a rule table, not a score. Each verdict names the fact it rests on:

| verdict | rule |
|---|---|
| ARCHIVED | the repository is archived |
| NO DATA | no closed PRs |
| DORMANT | nobody's PR merged in the last 30 days |
| UNTESTED | fewer than 5 outsider PRs closed in the sample |
| LONG SHOT | under 25 % of outsider PRs merge |
| OPEN LANE | 60 % or more merge, median time to merge 14 days or less, 5 or more merges in 30 days |
| SLOW LANE | 60 % or more merge, but slower or fewer than that |
| SELECTIVE | between 25 and 60 % merge |

A backlog of open outsider PRs older than 60 days is appended as a fact; it never
decides the verdict, because a repository that merges outsiders inside a day is an
open lane whatever its graveyard looks like.

## `gh odds mine`

```
REPO                                      #  AGE IDLE REPO ODDS      CHECKS      FLAGS / TITLE
nilearn/nilearn                        6530   30    5 92% OPEN LANE  43p/4f/0~   CI-RED-ON-BASE,CONFLICT  [ENH] add ensure_finite to smooth_img
actualbudget/actual                    8984    5    5 54% SELECTIVE  60p/0f/0~   CONFLICT  [WIP] [AI] Re-parse the selected file when i
mne-tools/mne-python                  14003   88    0 100% OPEN LANE 25p/0f/0~   ANSWERED-BY-PUSH  Fix psd_array_welch for good data spans
pydicom/pynetdicom                     1089   37   37 90% DORMANT    2p/0f/0~    REPO-DORMANT  [MRG] Document that the acceptor's transfer
anthropics/sandbox-runtime              520   20   20 20% LONG SHOT  0p/0f/0~    -  Bind the mux listeners exclusively so node c
...
24 open PRs by CedricConday  |  0 owe a reply  |  0 with a failure that is not on the base branch  |  2 conflicted
  BASE   nilearn/nilearn#6530  also red on the base branch: Test with 3.14 on ubuntu-latest: flaky tests, ...
```

Columns: AGE is days since the PR was opened; IDLE is days since anyone last did
anything on it (a human response, your comment, or your push). Flags, and the rule
behind each:

* **OWES-REPLY**: a human other than you spoke last (review, review comment or comment)
  and you have neither pushed nor commented since. A push or a comment counts as an
  answer; a merge commit ("update branch") does not.
* **ANSWERED-BY-PUSH**: changes were requested and you pushed after the request.
* **CHANGES-REQ**: changes were requested and you answered in words but not in code.
* **CI-FAIL**: a check fails on your PR and the same check has not failed on any of the
  last five commits of the base branch. **CI-RED-ON-BASE**: every failing check has also
  failed on the base branch recently; flaky or broken there, not yours to fix. Cancelled
  runs and workflows awaiting approval are not failures.
* **CONFLICT**: GitHub reports the PR as conflicting.
* **REPO-DORMANT / REPO-ARCHIVED**: the repository itself is the problem.
* **FETCH-FAILED**: the repository's odds could not be fetched; the reason is printed
  below the table and in the JSON `error` field. Nothing is silently omitted.

Rows that ask something of you (owed reply, failing check, conflict, changes requested)
come first, oldest first; the rest is parked on their side.

`--json` gives `{login, rows, repos}`: rows carry the flags and check names, `repos`
carries one odds card per repository keyed by name, durations as seconds plus a human
string.

## Cost and speed

One repository card is four GraphQL requests (metadata, closed sample, open sample, the
30-day merge count). The `mine` board adds, per repository, the check runs and statuses of
the last five base-branch commits over REST, three repositories at a time to stay clear of
GitHub's secondary rate limits; gateway errors and rate limits are retried with backoff.
About 20 seconds for one repository, one to two minutes for a board of two dozen PRs
across fifteen repositories. Well inside the 5,000-point hourly GraphQL budget.

## What it does not know

* Why a PR was closed. A closed-unmerged PR counts against the odds whether the author
  abandoned it or the maintainer declined it.
* The first response when the author or bots posted twenty times before anyone else
  answered; the card says for how many PRs a first response is known.
* Private context: a maintainer who merges only colleagues' work shows as a high member
  rate and a low outsider rate, which is the point.
* Anything about you. The odds are the repository's; whether your PR is good is still
  your job.

## Development

```bash
go build -o gh-odds . && ./gh-odds repo cli/cli
go test ./...
```

The tests build repositories from synthetic PR lists with known outcomes and check every
verdict rule, the bot filter, the owed-versus-answered logic and the base-branch red
comparison before anything is claimed about real repositories.

MIT. Written by Cedric Conday with Claude (Anthropic) as coding partner.
