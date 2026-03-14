package i18n

var messagesEn = map[string]string{
	// ==================== Command: General ====================
	"cmd.unknown":           "Unknown command.",
	"cmd.unknown_with_help": "Unknown command. Use /slack-review-notify help to see usage.",

	// ==================== Command: Help ====================
	"cmd.help": `*Review Notification Bot Configuration*
Command format: /slack-review-notify [label-name] subcommand [args]

*Label names with spaces*
If your label name contains spaces, wrap it in quotes (" or '):
- Example: /slack-review-notify "needs review" set-mention @team
- Example: /slack-review-notify 'security review' add-reviewer @security

*Initial Setup (Required)*
At minimum, the following settings are required to receive notifications:
:point_right: *1. Set mention target*
   /slack-review-notify [label-name] set-mention @user

:point_right: *2. Add target repository (important!)*
   /slack-review-notify [label-name] add-repo owner/repo

:information_source: Notifications will not be sent without both of these settings

*Multiple Label Configuration*
You can have independent settings for different labels in this channel:
- Example 1: /slack-review-notify bug set-mention @bug-team - settings for "bug" label
- Example 2: /slack-review-notify feature set-mention @dev-team - settings for "feature" label
- Example 3: /slack-review-notify security-review set-mention @security-team - settings for "security-review" label

*Multiple Label AND Conditions*
Specify multiple labels separated by commas to notify only when all labels are present:
- Example: /slack-review-notify "hoge-project,needs-review" set-mention @team
  - Notifies only when the PR has both "hoge-project" and "needs-review" labels
- Example: /slack-review-notify "frontend,urgent,needs-review" add-repo owner/app
  - All 3 labels are required

*All Commands*
*Basic Operations:*
• /slack-review-notify show - Show all label settings for this channel
• /slack-review-notify [label-name] show - Show detailed settings for specified label

*Required Settings:*
• /slack-review-notify [label-name] set-mention @user - Set mention target
• /slack-review-notify [label-name] add-repo owner/repo1,owner/repo2 - Add target repositories

*Reviewer Management:*
• /slack-review-notify [label-name] add-reviewer @user1,@user2 - Add reviewers
• /slack-review-notify [label-name] show-reviewers - Show reviewer list
• /slack-review-notify [label-name] clear-reviewers - Clear reviewers

*Advanced Settings:*
• /slack-review-notify [label-name] remove-repo owner/repo - Remove repository
• /slack-review-notify [label-name] set-label new-label-name - Rename label
• /slack-review-notify [label-name] set-reviewer-reminder-interval 30 - Set reminder interval (minutes)
• /slack-review-notify [label-name] set-business-hours-start 09:00 - Set business hours start
• /slack-review-notify [label-name] set-business-hours-end 18:00 - Set business hours end
• /slack-review-notify [label-name] set-timezone Asia/Tokyo - Set timezone
• /slack-review-notify [label-name] set-required-approvals N - Set required approvals (1-10)
• /slack-review-notify [label-name] set-language ja|en - Set message language
• /slack-review-notify [label-name] activate - Enable notifications
• /slack-review-notify [label-name] deactivate - Disable notifications

*User Mapping (for PR author notifications):*
• /slack-review-notify map-user <github-username> @slack-user - Link GitHub user to Slack user
• /slack-review-notify show-user-mappings - Show registered user mappings
• /slack-review-notify remove-user-mapping <github-username> - Remove user mapping

*Leave Management:*
• /slack-review-notify set-away @user [until YYYY-MM-DD] [reason description] - Set user as away
• /slack-review-notify unset-away @user - Remove user's away status
• /slack-review-notify show-availability - Show users currently on leave

Omitting [label-name] uses the default label "needs-review"`,

	// ==================== Command: set-language ====================
	"cmd.set_language.usage":   "Please specify a language. Supported: ja (Japanese), en (English)\nExample: /slack-review-notify %s set-language en",
	"cmd.set_language.invalid": "Unsupported language. Supported: ja (Japanese), en (English)",
	"cmd.set_language.set":     "Set language for label \"%s\" to %s.",
	"cmd.set_language.updated": "Updated language for label \"%s\" to %s.",

	// ==================== Command: set-mention ====================
	"cmd.set_mention.usage":   "Please specify a user ID for mention target. Example: /slack-review-notify %s set-mention @user",
	"cmd.set_mention.created": "Set mention target for label \"%s\" to %s.",
	"cmd.set_mention.updated": "Updated mention target for label \"%s\" to %s.",

	// ==================== Command: add-reviewer ====================
	"cmd.add_reviewer.usage":   "Please specify reviewer user IDs separated by commas. Example: /slack-review-notify %s add-reviewer @user1,@user2",
	"cmd.add_reviewer.created": "Set reviewer list for label \"%s\": %s",
	"cmd.add_reviewer.updated": "Updated reviewer list for label \"%s\": %s",

	// ==================== Command: show-reviewers ====================
	"cmd.show_reviewers.no_config": "No configuration found for label \"%s\" in this channel. Use /slack-review-notify %s add-reviewer to get started.",
	"cmd.show_reviewers.empty":     "No reviewers are registered for label \"%s\". Use /slack-review-notify %s add-reviewer to add reviewers.",
	"cmd.show_reviewers.header":    "*Reviewer list for label \"%s\" in this channel*\n%s",

	// ==================== Command: clear-reviewers ====================
	"cmd.clear_reviewers.no_config": "No configuration found for label \"%s\" in this channel.",
	"cmd.clear_reviewers.success":   "Cleared reviewer list for label \"%s\".",

	// ==================== Command: add-repo ====================
	"cmd.add_repo.usage":          "Please specify repository names separated by commas. Example: /slack-review-notify %s add-repo owner/repo1,owner/repo2",
	"cmd.add_repo.created":        "Added `%[2]s` to notification target repositories for label \"%[1]s\".",
	"cmd.add_repo.added":          "Added the following to notification target repositories for label \"%s\":\n`%s`",
	"cmd.add_repo.already_exists": "The following repositories were already notification targets:\n`%s`",
	"cmd.add_repo.no_valid":       "No valid repository names were specified.",

	// ==================== Command: remove-repo ====================
	"cmd.remove_repo.usage":     "Please specify a repository name. Example: /slack-review-notify %s remove-repo owner/repo",
	"cmd.remove_repo.no_config": "No configuration found for label \"%s\" in this channel.",
	"cmd.remove_repo.empty":     "No notification target repositories are set for label \"%s\".",
	"cmd.remove_repo.not_found": "Repository `%s` is not a notification target for label \"%s\".",
	"cmd.remove_repo.success":   "Removed `%[2]s` from notification target repositories for label \"%[1]s\".",

	// ==================== Command: set-label ====================
	"cmd.set_label.usage":          "Please specify a new label name. Example: /slack-review-notify %s set-label new-label-name",
	"cmd.set_label.no_config":      "No configuration for label \"%s\" exists in this channel.",
	"cmd.set_label.already_exists": "Configuration for label \"%s\" already exists. Please specify a different label name.",
	"cmd.set_label.success":        "Renamed label from \"%s\" to \"%s\".",

	// ==================== Command: activate/deactivate ====================
	"cmd.activate.no_config":   "No configuration found for label \"%s\" in this channel. Use /slack-review-notify %s set-mention to get started.",
	"cmd.deactivate.no_config": "No configuration found for label \"%s\" in this channel.",
	"cmd.activate.success":     "Enabled review notifications for label \"%s\" in this channel.",
	"cmd.deactivate.success":   "Disabled review notifications for label \"%s\" in this channel.",

	// ==================== Command: set-reviewer-reminder-interval ====================
	"cmd.set_reminder_interval.usage":        "Please specify the reminder interval in minutes after reviewer assignment. Example: /slack-review-notify %s set-reviewer-reminder-interval 30",
	"cmd.set_reminder_interval.invalid":      "Reminder interval must be a positive integer.",
	"cmd.set_reminder_interval.reviewer_set": "Set post-assignment reminder interval to %d minutes.",
	"cmd.set_reminder_interval.pending_set":  "Set pending reviewer reminder interval to %d minutes.",

	// ==================== Command: business hours ====================
	"cmd.time_format_invalid":            "Invalid time format. Please use HH:MM format (e.g., %s).",
	"cmd.set_business_hours_start.usage": "Please specify business hours start time. Example: /slack-review-notify %s set-business-hours-start 09:00",
	"cmd.set_business_hours_start.set":   "Set business hours start for label \"%s\" to %s.",
	"cmd.set_business_hours_start.updated": "Updated business hours start for label \"%s\" to %s.",
	"cmd.set_business_hours_end.usage":   "Please specify business hours end time. Example: /slack-review-notify %s set-business-hours-end 18:00",
	"cmd.set_business_hours_end.set":     "Set business hours end for label \"%s\" to %s.",
	"cmd.set_business_hours_end.updated": "Updated business hours end for label \"%s\" to %s.",

	// ==================== Command: timezone ====================
	"cmd.set_timezone.usage":   "Please specify a timezone. Example: /slack-review-notify %s set-timezone Asia/Tokyo",
	"cmd.set_timezone.invalid": "Invalid timezone. Examples: Asia/Tokyo, UTC, America/New_York",
	"cmd.set_timezone.set":     "Set timezone for label \"%s\" to %s.",
	"cmd.set_timezone.updated": "Updated timezone for label \"%s\" to %s.",

	// ==================== Command: required-approvals ====================
	"cmd.set_required_approvals.usage":   "Please specify the required number of approvals. Example: /slack-review-notify %s set-required-approvals 2",
	"cmd.set_required_approvals.invalid": "Required approvals must be an integer between 1 and 10.",
	"cmd.set_required_approvals.set":     "Set required approvals for label \"%s\" to %d.",
	"cmd.set_required_approvals.updated": "Updated required approvals for label \"%s\" to %d.",

	// ==================== Command: show ====================
	"cmd.show.error":     "An error occurred while retrieving settings.",
	"cmd.show.no_config": "No configuration found for this channel. Use /slack-review-notify [label-name] set-mention to get started.",
	"cmd.show.header":    "*Configured labels for this channel*\n",
	"cmd.show.footer":    "\nTo view detailed settings for a specific label: `/slack-review-notify [label-name] show`",

	// ==================== Command: show config ====================
	"cmd.show_config.no_config": "No configuration found for label \"%s\" in this channel. Use /slack-review-notify %s set-mention to get started.",
	"cmd.show_config.response": `*Review notification settings for label "%s" in this channel*
- Status: %s
- Mention target: <@%s>
- Reviewer list: %s
- Notification target repositories: %s
- Post-assignment reminder interval: %d min
- Business hours: %s - %s (%s)
- Required approvals: %d
- Language: %s`,

	// ==================== Command: map-user ====================
	"cmd.map_user.usage":        "Usage: /slack-review-notify map-user <github-username> @slack-user\nExample: /slack-review-notify map-user octocat @user",
	"cmd.map_user.invalid":      "Please specify both a GitHub username and a Slack user ID.",
	"cmd.map_user.updated":      "Updated mapping for GitHub user `%s` to <@%s>.",
	"cmd.map_user.create_error": "Failed to create user mapping.",
	"cmd.map_user.created":      "Mapped GitHub user `%s` to <@%s>.",

	// ==================== Command: show-user-mappings ====================
	"cmd.show_user_mappings.error":  "Failed to retrieve user mappings.",
	"cmd.show_user_mappings.empty":  "No user mappings registered yet.\nUse /slack-review-notify map-user to register.",
	"cmd.show_user_mappings.header": "*Registered User Mappings*\n",

	// ==================== Command: remove-user-mapping ====================
	"cmd.remove_user_mapping.usage":     "Usage: /slack-review-notify remove-user-mapping <github-username>\nExample: /slack-review-notify remove-user-mapping octocat",
	"cmd.remove_user_mapping.not_found": "No mapping found for GitHub user `%s`.",
	"cmd.remove_user_mapping.error":     "Failed to delete user mapping.",
	"cmd.remove_user_mapping.success":   "Deleted mapping for GitHub user `%s`.",

	// ==================== Command: set-away ====================
	"cmd.set_away.usage":        "Please specify a user to set as away. Example: /slack-review-notify set-away @user [until YYYY-MM-DD] [reason description]",
	"cmd.set_away.no_user":      "Please specify a user to set as away.",
	"cmd.set_away.invalid_date": "Invalid date format. Please use YYYY-MM-DD format (e.g., 2025-06-01).",
	"cmd.set_away.past_date":    "Cannot specify a past date. Please specify today or a future date.",
	"cmd.set_away.update_error": "Failed to update away status.",
	"cmd.set_away.create_error": "Failed to create away status.",
	"cmd.set_away.success":      "Set <@%s> as away",

	// ==================== Command: unset-away ====================
	"cmd.unset_away.usage":   "Please specify a user to remove away status. Example: /slack-review-notify unset-away @user",
	"cmd.unset_away.not_set": "<@%s> is not set as away.",
	"cmd.unset_away.success": "Removed away status for <@%s>",

	// ==================== Command: show-availability ====================
	"cmd.show_availability.empty":  "No users are currently on leave.",
	"cmd.show_availability.header": "*Users Currently on Leave*\n",

	// ==================== Common ====================
	"common.active":             "Active",
	"common.inactive":           "Inactive",
	"common.not_set":            "Not set",
	"common.invalid_user_id":    "Please specify a valid user ID.",
	"common.select_placeholder": "Select an option",
	"common.until":              "until %s",
	"common.indefinite":         "Indefinite",
	"common.reason":             ", reason: %s",
	"common.reason_paren":       " (%s)",

	// ==================== Pause Options ====================
	"pause.1h":    "Pause 1 hour",
	"pause.2h":    "Pause 2 hours",
	"pause.4h":    "Pause 4 hours",
	"pause.today": "No more today (pause until next business day)",
	"pause.stop":  "Stop reminders completely",

	// ==================== Button Labels ====================
	"button.review_done":     "Review Done",
	"button.pause_reminder":  "Pause Reminder",
	"button.stop_reminder":   "Stop Reminder",
	"button.change_reviewer": "Change Reviewer!",

	// ==================== Notifications ====================
	"notify.off_hours.with_creator":    "Review request from <@%s> has been registered\n\n*PR Title*: %s\n*URL*: <%s>\n\n📝 Mentions will be sent on the next business day morning",
	"notify.off_hours.without_creator": "📝 *A PR has been registered for review*\n\n*PR Title*: %s\n*URL*: <%s>\n\n(Mentions will be sent on the next business day morning)",

	"notify.review_request.with_creator":    "%s Review request from <@%s>\n\n*PR Title*: %s\n*URL*: <%s>",
	"notify.review_request.without_creator": "%s *There is a PR to review!*\n\n*PR Title*: %s\n*URL*: <%s>",

	"notify.business_hours_morning":  "🌅 *Good morning!* %s\n\n📋 Please review this PR. %s",
	"notify.reviewer_in_morning":     "\n\n🎯 *Reviewer*: @%s, please take a look!",
	"notify.reminder":                "%s Would love a review! 👀",
	"notify.out_of_hours_reminder":   "@%s Would love a review! 👀\n\nOutside business hours - next reminder will be sent on the next business day.",

	"notify.reminder_paused.1h":      "Got it! Pausing reminders for 1 hour!",
	"notify.reminder_paused.2h":      "Got it! Pausing reminders for 2 hours!",
	"notify.reminder_paused.4h":      "Got it! Pausing reminders for 4 hours!",
	"notify.reminder_paused.today":   "No more reminders today. Will resume on the next business day morning!",
	"notify.reminder_paused.stop":    "Reminders completely stopped. No notifications until a reviewer is assigned.",
	"notify.reminder_paused.default": "Reminders paused!",

	"notify.reviewer_auto_assigned":  "Reviewer auto-assigned: %s Please review!",
	"notify.reviewer_changed":        "Reviewer changed: %s → %s, please take a look!",
	"notify.cannot_change_reviewer":  "Cannot change reviewer - only one reviewer is registered. Please add more reviewers.",

	"notify.task_completed":   "✅ *%s*\n🔗 %s\n\n*Review Complete*: The review task has been closed because the PR label was removed.",
	"notify.review_done_button": "✅ <@%s> has completed the review! Thanks! 👏",

	"notify.review_approved":          "%s %s approved the review! Thanks! 👏",
	"notify.review_changes_requested": "%s %s requested changes. Thanks! 👏",
	"notify.review_commented":         "%s %s left a review comment. Thanks! 👏",
	"notify.review_default":           "%s %s reviewed. Thanks! 👏",

	"notify.label_removed_single":   "🏷️ Review task completed because the `%s` label was removed.",
	"notify.label_removed_multiple": "🏷️ Review task completed because one of the `%s` labels was removed.",

	"notify.pr_merged": "🎉 PR has been merged! Great work!",
	"notify.pr_closed": "🔒 PR has been closed.",

	"notify.re_review_requested": "🔄 %s requested a re-review from %s. Please take a look!",
	"notify.fully_approved":      "🎉 %d/%d approved - Review complete!",
}
