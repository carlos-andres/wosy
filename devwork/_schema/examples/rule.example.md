---
kind: rule
role: "How code moves in the three repos: who commits, the subject, the branches, the flow."
root: /Users/you/work/umbrella
para: Que ningún agente haga commit, push ni PR, y que cuando el dueño lo pida el mensaje y la rama salgan siempre con la misma forma.
created: 2026-09-29
updated: 2026-09-29
source_ref: /Users/you/.devwork/tasks/brain-continuity-20260923/reference-20260929/CLAUDE.md
---

# Git

- The agent never commits, pushes or opens a PR. It drafts the subject and the what and why. The owner commits.
- The commit subject is one line, no body: `<TICKET> <type>(<scope>): <imperative summary>`, for example `TICKET-1234 feat(feed): map the IRV stock field`. The ticket lives in the subject and in the branch name only, never in a source comment.
- Branches are `feature/<TICKET>` toward `release` and `hotfix/<TICKET>` toward `production`. Cut a branch with `--no-track` and hand over a push that names the branch:

```
git switch -c hotfix/<TICKET> --no-track origin/production
git push -u origin hotfix/<TICKET>
```

  A bare `git push` after a tracking switch writes straight to `production`. That happened on 2026-09-23, with no branch on the remote and no PR possible.
- Three branches, one flow. `develop` takes the normal workload. `release` takes every task. The lifecycle is three weeks. At mid-cycle every developer sends their work from `develop` to `release`, and QA runs on staging only. `production` takes release candidates and hotfixes. `origin/HEAD` reads `develop` in all three repos, and `develop` is not the target.
- A patch by `scp` to production is an exception on `worker-service` only, run by the owner. It is never the default.
- `gh` does not work here. The remotes are Bitbucket: use `git` and `curl`. Read Jira with `acli jira workitem view <KEY>`. Two boards: `TICKET` for connectors, `BOARD` for `api-service` and `dashboard-app` PR work.
