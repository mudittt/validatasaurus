<div>
<img display="inline-block" src="validatasaurus_logo.svg" alt="Validatasaurus Logo" width="200" />
</div>

# validatasaurus



A small tool that checks SQL files attached to your tickets and tells you if they look broken.

## What it does, in plain English

1. You give it a Jira or GitHub ticket link.
2. It downloads every `.sql` file someone has attached to that ticket.
3. It checks each file for common mistakes (missing semicolons, broken brackets, unmatched quotes, missing WHERE clauses, etc.).
4. It posts a clean report back to the ticket as a comment.

That's the whole thing.

---

## Before you start, you need three things

1. **Go** — the programming language used to build the tool. Version 1.22 or newer.
   - On Mac: `brew install go`
   - Or download from https://go.dev/dl/
   - Check it works by running: `go version`
2. **Python 3** — most computers already have it.
   - Check by running: `python3 --version`. If you see a version, you're good.
   - If not, install it from https://www.python.org/downloads/
3. **A terminal** — the Terminal app on Mac, or any terminal on Linux/Windows.

Everything below is free.

---

## Step 1 — Get the code onto your computer

If you already have the project folder on your machine, just open a terminal and `cd` into it:

```sh
cd path/to/validatasaurus
```

If you don't have it yet, clone it:

```sh
git clone https://github.com/mudittt/validatasaurus.git
cd validatasaurus
```

---

## Step 2 — Build the tool

Inside the folder, run these two commands:

```sh
make tidy
make build
```

- `make tidy` downloads the small libraries the tool needs.
- `make build` turns the code into a single runnable file called `validatasaurus`.

When it finishes you'll see a file named `validatasaurus` in the folder. That file *is* the tool.

---

## Step 3 — Tell the tool who you are

The tool needs your login details so it can read tickets and post comments. You only need to set up the side you actually use — **Jira** or **GitHub**. You can set up both if you want.

Create a file called `.env` in this folder (note the dot at the start). You can do this in your editor, or run:

```sh
touch .env
```

Open `.env` and paste whichever block you need from below.

### If you use Jira

```
JIRA_BASE_URL=https://YOUR-COMPANY.atlassian.net
JIRA_EMAIL=you@yourcompany.com
JIRA_API_TOKEN=paste-your-token-here
```

**How to find each value:**

- `JIRA_BASE_URL` — open any Jira ticket in your browser. Copy everything up to and including `.atlassian.net`. Example: if your ticket is `https://acme.atlassian.net/browse/PROJ-123`, your base URL is `https://acme.atlassian.net`.
- `JIRA_EMAIL` — the email address you use to log into Jira.
- `JIRA_API_TOKEN` — get this from Atlassian:
   1. Go to https://id.atlassian.com/manage-profile/security/api-tokens
   2. Click **Create API token**.
   3. Give it any name (e.g. "validatasaurus").
   4. Click **Create**, then **Copy**.
   5. Paste it into `.env`.

### If you use GitHub

```
GITHUB_TOKEN=paste-your-token-here
```

**How to get the token:**

1. Go to https://github.com/settings/tokens
2. Click **Generate new token** → **Generate new token (classic)**.
3. Give it a name (e.g. "validatasaurus") and an expiry date.
4. Tick the **`repo`** checkbox. (This lets the tool read issues and post comments.)
5. Scroll to the bottom and click **Generate token**.
6. **Copy the token straight away** — GitHub only shows it once.
7. Paste it into `.env`.

> **Don't commit `.env` to git.** It contains your secrets. The project's `.gitignore` already excludes it.
>
> **You don't have to use a `.env` file.** You can also set these as normal environment variables in your shell. `.env` is just easier.
>
> **Skipped this step?** No problem — the tool will pop up a screen asking for your credentials when you run it. They'll be remembered for that one session.

---

## Step 4 — Run it

You're ready. Run the tool with a ticket URL:

```sh
./validatasaurus https://your-company.atlassian.net/browse/PROJ-123
./validatasaurus https://github.com/your-org/your-repo/issues/42
```

Or run it without anything and paste the URL on screen:

```sh
./validatasaurus
```

---

## What you'll see, screen by screen

1. **URL screen** — type or paste the ticket link, press `Enter`.
2. **Auth screen** *(only if your credentials are missing)* — fill in the fields, press `Enter` on the last one.
3. **Fetching** — a spinner spins while the tool downloads the SQL files from the ticket.
4. **Validating** — each file is checked one by one. You'll see a log of what's happening.
5. **Results screen** — a table appears with one row per file. Each row has one of three colours:
   - ✅ **PASSED** (green) — no problems
   - ⚠️ **PASSED with warnings** (orange) — small things to look at, but not broken
   - ❌ **FAILED** (red) — real errors that need fixing
6. **Post this report?** — press `y` to add the report as a comment on the ticket, `n` to skip.
7. **Done** — press `Enter` to exit.

---

## Keys you can press

| Where | Key | What it does |
|---|---|---|
| URL screen | `Enter` | Submit the URL |
| URL screen | `Esc` | Quit |
| Auth screen | `Tab` or `↓` | Move to next field |
| Auth screen | `Shift+Tab` or `↑` | Move to previous field |
| Auth screen | `Enter` | Submit (when on last field) |
| Results screen | `y` | Post the comment |
| Results screen | `n` | Skip posting |
| Results screen | `q` | Quit |
| Done / Error screen | `Enter` or `q` | Exit |
| Anywhere | `Ctrl+C` | Force quit |

---

## Handy extra commands (for testing)

These are useful when you want to try things without involving a real ticket:

```sh
# Check a SQL file already on your computer
./validatasaurus --validate-local path/to/your/file.sql

# Check that a URL is recognised correctly
./validatasaurus --detect https://github.com/owner/repo/issues/42

# Fetch a ticket and validate, but DO NOT post the comment
./validatasaurus --dry-run https://your-company.atlassian.net/browse/PROJ-123
```

---

## When something goes wrong

**"command not found: ./validatasaurus"**
You're not inside the project folder, or you haven't built it yet. Run `cd` into the folder, then `make build`.

**"jira credentials not configured"**
Your `.env` file is missing one of the three Jira variables, or one is misspelled. Either fix `.env`, or just run the tool — it'll pop up the Auth screen for you to fill in.

**"github comment failed: 403" or "404"**
Your GitHub token doesn't have permission for that repo. Re-create the token (Step 3) and make sure the `repo` scope is ticked.

**"python3: not found"**
Install Python 3 from https://www.python.org/downloads/, or if Python is somewhere unusual on your machine, set `PYTHON_PATH` in `.env` (e.g. `PYTHON_PATH=/usr/bin/python3.11`).

**The screen looks garbled or cut off**
Make your terminal window wider. The tool wants at least about 80 characters of width.

**"No .sql files attached to this ticket"**
The tool only picks up files whose name ends in `.sql`. Check the ticket has those attached.

---

## All settings in one table

| Variable | What it is | Needed? |
|---|---|---|
| `JIRA_BASE_URL` | Your Jira site URL (e.g. `https://acme.atlassian.net`) | Only if using Jira |
| `JIRA_EMAIL` | Your Jira login email | Only if using Jira |
| `JIRA_API_TOKEN` | Your Jira API token (see Step 3) | Only if using Jira |
| `GITHUB_TOKEN` | Your GitHub personal access token (see Step 3) | Only if using GitHub |
| `PYTHON_PATH` | Where Python 3 lives on your machine | No (defaults to `python3`) |

---

## What's inside the project

```
cmd/main.go              the program's starting point
internal/config/         reads your settings from .env or environment
internal/detect/         figures out if a URL belongs to Jira or GitHub
internal/platform/       talks to the Jira and GitHub APIs
internal/validator/      runs the SQL syntax checker and formats reports
internal/tui/            draws the screens you see
```

The Python syntax-checking script is **baked into** the `validatasaurus` binary at build time. You don't need to install or carry it separately — the single `validatasaurus` file is everything.

---

If you ever get stuck, run `./validatasaurus` with no arguments. It prints whether your credentials are detected and where Python is — usually that's enough to spot the problem.
