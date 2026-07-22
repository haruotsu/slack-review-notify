# Assigned Review List Command Design

## Context

Review notifications arrive in multiple Slack channels, but the application has no command that consolidates outstanding requests. Existing `show` and `show-reviewers` commands display configuration and reviewer-pool data, not review tasks.

## Goal

Add `/slack-review-notify reviews` so a Slack user can see every unfinished review request assigned to them across all channels. The response must be visible only to the command sender and make each pull request easy to open.

## Non-goals

- Do not add a dashboard, Slack App Home, direct messages, Block Kit actions, or pagination.
- Do not change the database schema or task lifecycle.
- Do not list tasks assigned only to other users.
- Do not add filtering by channel, label, repository, or status.
- Do not change existing review notifications or reminder behavior.

## Command and visibility

- Command: `/slack-review-notify reviews`
- Scope: all `ReviewTask` records in every Slack channel stored by this application.
- Viewer: only the Slack user whose `user_id` is included in the slash-command request.
- Response: HTTP 200 JSON with `response_type: "ephemeral"` and a Slack mrkdwn `text` value for success, empty results, and database errors.
- The command does not call the Slack Web API and does not post to a channel or direct message.
- Language follows the existing command-handler behavior: use the `needs-review` configuration for the channel where the command is invoked and otherwise default to Japanese.

## Task selection

A task is included only when both conditions below are true.

1. Its status is one of:
   - `pending`
   - `in_review`
   - `waiting_business_hours`
   - `paused`
   - `snoozed`
2. The command sender's Slack user ID exactly matches:
   - one comma-separated entry in `Reviewers`, after trimming surrounding whitespace; or
   - the legacy `Reviewer` field, after trimming surrounding whitespace.

Exact comparison is required so a user such as `U123` does not match `U1234`. A task that matches both fields is displayed once. `completed`, `done`, `archived`, deleted tasks, and tasks assigned only to other users are excluded.

## Ordering and limits

- Sort matching tasks by `CreatedAt` ascending so the oldest outstanding request appears first.
- Use `ID` ascending as the deterministic tie-breaker.
- Display at most 50 tasks.
- The header count is the total number of matching tasks, including tasks beyond the display limit.
- When more than 50 tasks match, append `:information_source:` text stating how many additional tasks exist.

## Slack presentation

Use Slack ASCII emoji aliases in source text. Do not embed Unicode emoji.

Japanese example:

```text
:clipboard: *あなたへの未完了レビュー依頼 (3件)*
_古い依頼から表示しています_

:large_blue_circle: <https://github.com/example/api/pull/123|example/api #123>
認証APIを追加
<#C_BACKEND> | レビュー中

:crescent_moon: <https://github.com/example/web/pull/456|example/web #456>
ログイン画面を修正
<#C_FRONTEND> | 営業時間待ち

:double_vertical_bar: <https://github.com/example/app/pull/789|example/app #789>
依存ライブラリを更新
<#C_APP> | リマインド停止中
```

English uses the same layout with translated header, empty, truncation, error, and status text.

Each task entry contains, in order:

1. A status-specific ASCII emoji.
2. A clickable PR link whose label is `<repo> #<PR number>`.
3. The PR title on its own line.
4. A clickable Slack channel mention and localized status label.

Status presentation:

| Status | ASCII emoji | Japanese label | English label |
| --- | --- | --- | --- |
| `pending` | `:hourglass_flowing_sand:` | 登録処理中 | Processing |
| `in_review` | `:large_blue_circle:` | レビュー中 | In review |
| `waiting_business_hours` | `:crescent_moon:` | 営業時間待ち | Waiting for business hours |
| `paused` | `:double_vertical_bar:` | リマインド停止中 | Reminders stopped |
| `snoozed` | `:zzz:` | スヌーズ中 | Snoozed |

Before interpolation, collapse title whitespace to single spaces and escape Slack-sensitive `&`, `<`, and `>` characters. Repository, PR number, channel ID, and PR URL come from existing task fields. Entries are separated by one blank line for scanability.

Empty-result presentation:

```text
:white_check_mark: あなたへの未完了レビュー依頼はありません。
```

Database-error presentation:

```text
:warning: レビュー依頼の取得に失敗しました。
```

## Implementation shape

Keep the change within the existing slash-command architecture:

- Register `reviews` in `potentialSubCommands` and the command switch in `handlers/command.go`.
- Add small private helpers in `handlers/command.go` for assignment matching, Slack-safe title formatting, status presentation, and response construction.
- Add Japanese and English command/help strings in the existing i18n message maps.
- Add handler-level tests using the existing in-memory SQLite and Slack test-mode setup.
- Update the built-in Japanese and English help output so users can discover the command.

Do not introduce a new package, dependency, database migration, public interface, or Slack API call.

## Error handling

- A database query failure returns the localized `:warning:` ephemeral response with HTTP 200 so Slack can render the message to the sender.
- An empty result returns the localized `:white_check_mark:` ephemeral response with HTTP 200.
- A task with an unexpected status cannot appear because the query only selects the five specified active statuses.

## TDD coverage

Tests must be written and observed failing before production code is added. Handler-level tests cover:

1. Assigned active tasks from multiple channels are returned in oldest-first order.
2. `Reviewers` CSV matching is exact and whitespace-tolerant.
3. Legacy `Reviewer` matching works.
4. Tasks assigned only to other users are excluded.
5. `completed`, `done`, `archived`, and deleted tasks are excluded.
6. The response is explicitly ephemeral and contains clickable PR and channel links plus ASCII emoji status markers.
7. PR title whitespace and Slack-sensitive characters are rendered safely.
8. An empty result returns the localized success message.
9. More than 50 matches displays the oldest 50, reports the total count, and states the remaining count.
10. A database failure returns the localized warning response.

After the focused tests pass, run `go test ./...`, `make lint`, and `make build` before review and PR creation.

## Acceptance criteria

- `/slack-review-notify reviews` works without a channel configuration.
- Only unfinished tasks assigned to the command sender are included.
- Matching tasks from all stored Slack channels can appear.
- Only the command sender can see the response.
- Every displayed task has a clickable GitHub PR link and clickable Slack channel mention.
- The response uses ASCII emoji aliases and remains readable as a compact Slack command result.
- At most 50 task entries are rendered, with an accurate remaining count.
- Existing commands, notifications, database schema, and dependencies remain unchanged.
