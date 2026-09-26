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
mne-tools/mne-python   stars 3528   default main
sample: 97 closed PRs, 2026-06-30 to 2026-09-26, plus the open ones
outsiders: 60 closed, 58 merged (97%)   median first response 10h   median time to merge 16h
members:   37 closed, 37 merged (100%)   median time to merge 4h
merges in the last 30 days: 88   who merges: larsoner 80, drammock 5, mne-bot 4, wmvanvliet 3
open outsider PRs: 60, 52 older than 60 days, oldest 1882d
verdict: OPEN LANE: 97% of outsider PRs merge, median 16h, 88 merges in 30 days; 52 open outsider PRs are older than 60 days, so expect yours to need a nudge
```

```
joke2k/faker   stars 19412   default master
sample: 95 closed PRs, 2025-08-26 to 2026-09-20, plus the open ones
outsiders: 94 closed, 69 merged (73%)   median first response 10.2d   median time to merge 19.6d
members:   1 closed, 1 merged (100%)   median time to merge 4h
merges in the last 30 days: 4   who merges: fcurella 69, joke2k 1
open outsider PRs: 20, 12 older than 60 days, oldest 305d
verdict: SLOW LANE: 73% of outsider PRs merge, but median 19.6d and 4 merges in 30 days; 12 open outsider PRs are older than 60 days, so expect yours to need a nudge
```

Two repositories with a high historical merge rate and opposite prospects this month.
That is the number a star count cannot give you.

**Outsider** means what GitHub reports: an author association of CONTRIBUTOR,
FIRST_TIME_CONTRIBUTOR, FIRST_TIMER or NONE. OWNER, MEMBER and COLLABORATOR are
members. Bots are excluded on both sides.

**Sample**: the 100 most recently updated merged or closed PRs and the 100 most recently
updated open ones (`--sample N`, 100 is the API's page limit). The dates in the card say
what period that covers; on a busy repository it is a few months, on a quiet one years,
which is itself information.

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
traceroot-ai/traceroot                 1368   89   89 83% SLOW LANE  18p/0f/0~   OWES-REPLY  fix(api): use exclusive upper bound
mne-tools/mne-python                  14003   88    0 97% OPEN LANE  25p/0f/0~   ANSWERED-BY-PUSH  Fix psd_array_welch for good data
nilearn/nilearn                        6530   30    5 86% OPEN LANE  43p/4f/0~   CI-FAIL,CONFLICT  [ENH] add ensure_finite to smooth_img
joke2k/django-environ                   668    0    0 72% DORMANT    1p/0f/0~    REPO-DORMANT  Use the default in db_url() when the
...
23 open PRs by CedricConday  |  2 owe a reply  |  1 with a failure that is not on the base branch  |  2 conflicted
```

Flags, and the rule behind each:

* **OWES-REPLY**: a human other than you spoke last (review, review comment or comment)
  and you have not pushed since. A push counts as an answer.
* **ANSWERED-BY-PUSH**: changes were requested and you pushed after the request.
* **CHANGES-REQ**: changes were requested, nobody has spoken since, and you have not pushed.
* **CI-FAIL**: a check fails on your PR and the same check is not failing on the base
  branch head. **CI-RED-ON-BASE**: every failing check also fails on the base branch;
  not yours to fix.
* **CONFLICT**: GitHub reports the PR as conflicting.
* **REPO-DORMANT / REPO-ARCHIVED**: the repository itself is the problem.

Rows that need you come first, oldest first; the rest is parked on their side.

## Cost and speed

One repository card is three GraphQL requests; the `mine` board is three per repository
plus one REST call for the base branch, run six repositories at a time. About 20 seconds
for one repository, 30 to 60 seconds for a board of two dozen PRs across fifteen
repositories. Well inside the 5,000-point hourly GraphQL budget.

## What it does not know

* Why a PR was closed. A closed-unmerged PR counts against the odds whether the author
  abandoned it or the maintainer declined it.
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
