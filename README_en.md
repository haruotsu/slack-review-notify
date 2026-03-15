# slack-review-notify

**Automatically notify Slack of GitHub PR review requests via labels, assign reviewers, and send reminders.**

> **[English](./README_en.md)** | **[日本語](./README_ja.md)**

## Features
- **Slack command configuration**: Manage all settings directly from Slack
- **Automatic notifications**: Notify configured Slack channels when a PR is labeled
- **PR author mentions**: PR authors are mentioned so they receive thread notifications
- **Customizable business hours**: Per-channel business hours with overnight support
- **Timezone support**: Supports global teams (JST, UTC, and many more)
- **Off-hours queuing**: Labels added outside business hours are held and notified during business hours
- **Random reviewer selection**: Randomly picks from a configured reviewer list
- **Periodic reminders**: Sends reminders at a configurable interval until review is complete
- **Off-hours reminder control**: One reminder outside business hours, then waits until the next business day
- **Pause reminders**: Pause reminders for preset durations
- **Auto review detection & thanks**: Automatically posts a thank-you message when a review is submitted on GitHub
- **Reviewer reassignment**: One-click "Change Reviewer!" button to re-draw a reviewer
- **i18n support**: Per-channel language setting (Japanese / English)

## Demo

### Review Flow
![EN Review Flow](docs/gifs/en-review-flow.gif)

### Other Features
![Features Overview EN](docs/gifs/en-features-overview.gif)

> Per-channel language setting via `/slack-review-notify set-language en|ja`

<details>
<summary>All screenshots</summary>

| # | Feature | Screenshot |
|---|---------|------------|
| 1 | PR notification + reviewer (JA) | ![](docs/screenshots/01-ja-pr-labeled-with-reviewer.png) |
| 2 | PR notification + reviewer (EN) | ![](docs/screenshots/02-en-pr-labeled-with-reviewer.png) |
| 3 | Off-hours notification (JA) | ![](docs/screenshots/03-ja-off-hours.png) |
| 4 | Review approved 1/2 (JA thread) | ![](docs/screenshots/04-ja-review-approved-partial.png) |
| 5 | Fully approved 2/2 (JA thread) | ![](docs/screenshots/05-ja-review-fully-approved.png) |
| 6 | Changes requested (EN thread) | ![](docs/screenshots/06-en-changes-requested.png) |
| 7 | Review comment (EN thread) | ![](docs/screenshots/07-en-commented.png) |
| 8 | Re-review requested (EN thread) | ![](docs/screenshots/08-en-re-review.png) |
| 9 | PR merged (JA thread) | ![](docs/screenshots/09-ja-pr-merged.png) |
| 10 | PR closed (EN thread) | ![](docs/screenshots/10-en-pr-closed.png) |
| 11 | Label removed (JA thread) | ![](docs/screenshots/11-ja-label-removed.png) |
| 12 | Reminder + pause dropdown (JA) | ![](docs/screenshots/12-ja-reminder.png) |
| 13 | All channels overview | ![](docs/screenshots/13-overview.png) |
| 14 | Off-hours notification (EN) | ![](docs/screenshots/14-en-off-hours.png) |
| 15 | Label removed (EN thread) | ![](docs/screenshots/15-en-label-removed.png) |

</details>

## Setup
### GitHub Configuration
In your repository's Settings > Webhooks, configure the following:
- Payload URL: `https://<your-domain>/webhook`
- Content type: `application/json`
- Secret: empty or any string of your choice
- Enable SSL verification: checked
- Select individual events and enable:
  - **Pull requests**: Detects label add/remove on PRs
  - **Pull request reviews**: Detects approval/changes requested/comments (for auto-completion)

### Environment Variables
Create a `.env` file with the following:
```
SLACK_BOT_TOKEN=xoxb-your-slack-bot-token
SLACK_SIGNING_SECRET=your-slack-signing-secret
GITHUB_WEBHOOK_SECRET=your-github-webhook-secret
DB_PATH=review_tasks.db  # Default: review_tasks.db (optional)
```

## Testing Locally
See [/docs/example_usage.md](./docs/example_usage.md) for instructions on setting up a local server with ngrok.

You can also deploy to Kubernetes, AWS EC2, or any environment you prefer. A sample `slack-app-manifest_sample.json` is included.

## Usage
### Add the Bot to a Channel
```
/invite @review-notify-bot
```

### Notification Settings
Full command list (also available via `/slack-review-notify help`)

Command format: `/slack-review-notify [label-name] subcommand [args]`

- `/slack-review-notify help`: Show command help
- `/slack-review-notify show`: Show all label configurations for the channel

**Omitting [label-name] uses the default label "needs-review"**

- `/slack-review-notify [label-name] show`: Show settings for the specified label
- `/slack-review-notify [label-name] set-mention @user`: Set mention target
- `/slack-review-notify [label-name] add-reviewer @user1,@user2`: Add reviewers
- `/slack-review-notify [label-name] show-reviewers`: Show registered reviewer list
- `/slack-review-notify [label-name] clear-reviewers`: Clear reviewer list
- `/slack-review-notify [label-name] add-repo owner/repo`: Add target repository
- `/slack-review-notify [label-name] remove-repo owner/repo`: Remove target repository
- `/slack-review-notify [label-name] set-label new-label-name`: Rename the label
- `/slack-review-notify [label-name] set-reviewer-reminder-interval 30`: Set reminder interval after reviewer assignment (minutes)
- `/slack-review-notify [label-name] set-business-hours-start 09:00`: Set business hours start (HH:MM)
- `/slack-review-notify [label-name] set-business-hours-end 18:00`: Set business hours end (HH:MM)
- `/slack-review-notify [label-name] set-timezone Asia/Tokyo`: Set timezone (e.g., `Asia/Tokyo`, `UTC`, `America/New_York`)
- `/slack-review-notify [label-name] set-required-approvals N`: Set required number of approvals (1-10)
- `/slack-review-notify [label-name] set-language ja|en`: Set message language
- `/slack-review-notify [label-name] activate`: Enable notifications
- `/slack-review-notify [label-name] deactivate`: Disable notifications

### User Mapping (PR Author Notifications)
Link GitHub users to Slack users so PR authors receive thread notifications.

- `/slack-review-notify map-user <github-username> @slack-user`: Link GitHub and Slack users
- `/slack-review-notify show-user-mappings`: Show registered mappings
- `/slack-review-notify remove-user-mapping <github-username>`: Remove a mapping

**Example:**
```bash
# Link your GitHub username to your Slack account
/slack-review-notify map-user octocat @john

# Check mappings
/slack-review-notify show-user-mappings

# Remove a mapping
/slack-review-notify remove-user-mapping octocat
```

When a mapping is set, notifications appear like:
```
@team Review request from @john

*PR Title*: Implement hogehoge API
*URL*: https://github.com/owner/repo/pull/123
```

### Leave Management
- `/slack-review-notify set-away @user [until YYYY-MM-DD] [reason description]`: Set user as away
- `/slack-review-notify unset-away @user`: Remove away status
- `/slack-review-notify show-availability`: Show users currently on leave

### Review Management
Various actions are available from notification messages:

#### Auto Review Detection
When a review is submitted on GitHub, a thank-you message is automatically posted to the thread:
- Approved: `✅ reviewer approved the review! Thanks! 👏`
- Changes requested: `🔄 reviewer requested changes. Thanks! 👏`
- Commented: `💬 reviewer left a review comment. Thanks! 👏`

#### Off-Hours Queuing
When a PR is labeled outside business hours:
- Notification is held until business hours
- Reviewers are automatically assigned and notified when business hours begin
- No disturbance to team members at inappropriate times

**Business Hours Settings**
- Default: Weekdays 9:00-18:00 (JST)
- Configurable per channel
- Overnight hours supported (e.g., 22:00-06:00)
- Timezone settings for global team support

#### Manual Actions
- "Review Done" button: Manually mark review as complete
- "Change Reviewer!" button: Re-draw a reviewer
- Pause reminders at assignment: Pause reminders when a reviewer is assigned
- Pause options: 1 hour, 2 hours, 4 hours, no more today (until next business day), stop completely

### Configuration Examples
#### User Mapping Setup (Recommended)
Set up user mappings first so PR authors receive thread notifications.

```bash
# Link team members' GitHub and Slack accounts
/slack-review-notify map-user alice @alice
/slack-review-notify map-user bob @bob
/slack-review-notify map-user charlie @charlie

# Verify mappings
/slack-review-notify show-user-mappings
```

#### Business Hours and Timezone
```bash
# Set business hours to 9:00-18:00
/slack-review-notify set-business-hours-start 09:00
/slack-review-notify set-business-hours-end 18:00

# Set timezone to Japan
/slack-review-notify set-timezone Asia/Tokyo

# Night shift team (22:00-06:00)
/slack-review-notify night-shift set-business-hours-start 22:00
/slack-review-notify night-shift set-business-hours-end 06:00
/slack-review-notify night-shift set-timezone Asia/Tokyo

# US team
/slack-review-notify us-team set-business-hours-start 09:00
/slack-review-notify us-team set-business-hours-end 17:00
/slack-review-notify us-team set-timezone America/New_York

# Check settings
/slack-review-notify show
```

#### Basic Notification Setup
```bash
# Settings for needs-review label
/slack-review-notify add-repo owner/repository
/slack-review-notify set-mention @team-lead
/slack-review-notify add-reviewer @reviewer1,@reviewer2,@reviewer3

# Settings for security label
/slack-review-notify security add-repo owner/repository
/slack-review-notify security set-mention @security-team
/slack-review-notify security add-reviewer @security-expert1,@security-expert2
```

#### Language Setting
```bash
# Set channel language to English
/slack-review-notify set-language en

# Set channel language to Japanese
/slack-review-notify set-language ja
```

## Development

### Local Development
```bash
# Install dependencies
make deps

# Dev server with hot reload
make dev

# Build binary
make build

# Run the application (after build)
make run

# Run tests
make test

# Run tests with coverage
make test-coverage

# Run linter
make lint

# Install golangci-lint (first time only)
make lint-install

# Cleanup (remove build artifacts and DB files)
make clean
```

Port: 8080

### Development with SlackHog (Slack API Mock)

[SlackHog](https://github.com/harakeishi/slackhog) is a Slack API mock server that lets you develop and test without a real Slack workspace.

#### Docker Compose (recommended)

```bash
# Start app + SlackHog together
make up

# Stop
make down
```

This starts:
- **App** on `http://localhost:8080` — automatically sends Slack API calls to SlackHog
- **SlackHog** on `http://localhost:4112` — web UI to view messages and threads

The `SLACK_API_BASE_URL` is automatically set to `http://slackhog:4112/api` via `docker-compose.yml`.

#### SlackHog standalone (for `make dev`)

```bash
# Terminal 1: Start SlackHog
make slackhog

# Terminal 2: Start the app pointing to SlackHog
SLACK_API_BASE_URL=http://localhost:4112/api make dev
```

#### Viewing messages

Open `http://localhost:4112` in your browser. Click a channel in the sidebar to view messages. Click the reply badge (e.g. "3件の返信") to open the thread panel.

#### E2E Tests

Playwright-based E2E tests are available in the `e2e/` directory:

```bash
cd e2e
npm install
npx playwright install chromium

# Start Docker environment first
make up

# Run API tests (78 tests)
npx playwright test i18n-e2e.test.ts

# Run screenshot capture (takes ~3 min, waits for reminder)
npx playwright test full-screenshot.test.ts --timeout=300000
```

### Deployment
Run the application on Kubernetes, AWS EC2, or any environment you prefer.

#### Release Workflow (triggered by tag creation)
Creating a tag in the `v*` format automatically builds and releases binaries for:
- Linux (amd64)
- macOS (amd64, arm64)
- Windows (amd64)

## Contributors

<a href="https://github.com/haruotsu/slack-review-notify/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=haruotsu/slack-review-notify" />
</a>

Stars & PRs welcome. For large changes, please open an issue first to discuss.

## License
Apache License Version 2.0, January 2004
