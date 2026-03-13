---
name: fetch-github-issue
description: Fetches GitHub issue content by ID and saves it as a structured markdown file for the development pipeline. Use this skill when users want to (1) fetch a GitHub issue by number/ID, (2) retrieve issue details from a repository, (3) convert an issue into a requirements document, (4) integrate GitHub issues into the idea-to-software pipeline, or (5) start a development workflow from a GitHub issue.
---

# Fetch GitHub Issue

Fetch a GitHub issue by ID and convert it into a structured requirements document for the idea-to-software pipeline.

**CRITICAL:** Run this skill inside a subAgent to keep the fetch operation isolated and avoid pulling unrelated workspace context into the issue document.

## Prerequisites

- **GitHub CLI (`gh`)** must be installed and authenticated
- Repository access (public or authenticated for private repos)

## Usage

### Via Script (Recommended)

Use the bundled Python script for automated fetching:

```bash
./skills/fetch-github-issue/scripts/fetch-issue.py <issue-id> [repo] [--preview] [--start-pipeline]
```

### Via GitHub CLI

For manual fetching or custom workflows:

```bash
gh issue view {ISSUE_ID} --repo {REPO} --json title,body,labels,assignees,milestone,createdAt,number,url,state
```

## Output Format

Creates markdown file at `.tot-docs/requirements/issues/{issue-id}.md` with this structure:

```markdown
# GitHub Issue #{number}: {title}

**Repository:** {repo}
**URL:** {url}
**State:** {state}
**Labels:** {labels as comma-separated list, or "None"}
**Created:** {createdAt formatted as YYYY-MM-DD}

---

## Description

{issue body content - preserve original markdown formatting}

---

## Metadata

- **Assignees:** {assignees as comma-separated list, or "Unassigned"}
- **Milestone:** {milestone name, or "None"}

---

_Fetched on {current timestamp} using fetch-github-issue skill_
```

## Parameters

| Parameter      | Required | Default     | Description                            |
| -------------- | -------- | ----------- | -------------------------------------- |
| Issue ID       | Yes      | -           | GitHub issue number (positive integer) |
| Repository     | No       | auto-detect | Repository in `owner/repo` format      |
| Preview        | No       | false       | Show output without writing file       |
| Start Pipeline | No       | false       | Auto-start orchestrator after fetch    |

## Common Errors

| Error             | Solution                                           |
| ----------------- | -------------------------------------------------- |
| gh not installed  | Install: `brew install gh` (macOS) or see CLI docs |
| Not authenticated | Run `gh auth login`                                |
| Issue not found   | Verify issue number and repository access          |
| Permission denied | Check repo permissions, try `gh auth refresh`      |

## Output Location

All fetched issues are saved to: `.tot-docs/requirements/issues/{issue-id}.md`

## Pipeline Integration

The output file is compatible with:

- **feature-explorer** — Use as initial requirement input
- **create-prd** — Process directly into a PRD
- **orchestrator** — Pass as `$INPUT` parameter

## When to Use This Skill

- User asks to "fetch issue #123" or "get GitHub issue"
- User wants to start development from a GitHub issue
- User needs to convert an issue into a requirements document
- Pipeline needs GitHub issue content as input

## Implementation

The complete implementation is in [scripts/fetch-issue.py](scripts/fetch-issue.py), which handles:

- Input validation (issue ID, repository format)
- GitHub CLI availability and authentication checks
- Repository auto-detection
- Issue fetching and formatting
- File writing and error handling
