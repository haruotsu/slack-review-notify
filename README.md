# slack-review-notify

**Automatically notify Slack of GitHub PR review requests via labels, assign reviewers, and send reminders.**

> **[日本語](./README_ja.md)** | **[English](./README_en.md)**

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

### PR Notification & Review Thread
| Japanese (default) | English |
|---|---|
| ![JA](docs/screenshots/05-ja-review-fully-approved.png) | ![EN](docs/screenshots/08-en-re-review.png) |

> Per-channel language setting via `/slack-review-notify set-language en|ja`

<details>
<summary>More screenshots</summary>

#### Off-hours notification (JA)
![](docs/screenshots/03-ja-off-hours.png)

#### PR merged (JA thread)
![](docs/screenshots/09-ja-pr-merged.png)

#### PR closed (EN thread)
![](docs/screenshots/10-en-pr-closed.png)

#### Label removed (JA thread)
![](docs/screenshots/11-ja-label-removed.png)

#### Reminder with pause dropdown (JA thread)
![](docs/screenshots/12-ja-reminder.png)

</details>

## Quick Start

See **[English README](./README_en.md)** or **[日本語 README](./README_ja.md)** for full setup and usage instructions.

## Development

```bash
make deps          # Install dependencies
make dev           # Dev server with hot reload
make build         # Build binary
make test          # Run tests
make lint          # Run linter
```

Port: 8080

## Contributing
Stars & PRs welcome. For large changes, please open an issue first to discuss.

## License
Apache License Version 2.0, January 2004
