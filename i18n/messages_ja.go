package i18n

var messagesJa = map[string]string{
	// ==================== Command: General ====================
	"cmd.unknown":           "不明なコマンドです。",
	"cmd.unknown_with_help": "不明なコマンドです。/slack-review-notify help で使い方を確認してください。",

	// ==================== Command: Help ====================
	"cmd.help": `*Review通知Bot設定コマンド*
コマンド形式: /slack-review-notify [ラベル名] サブコマンド [引数]

*スペースを含むラベル名について*
スペースを含むラベル名を使用する場合は、クォート（"または'）で囲んでください:
- 例: /slack-review-notify "needs review" set-mention @team
- 例: /slack-review-notify 'security review' add-reviewer @security

*初期設定（必須）*
通知を受けるには最低限以下の設定が必要です：
:point_right: *1. メンション先の設定*
   /slack-review-notify [ラベル名] set-mention @user

:point_right: *2. 対象リポジトリの追加（重要！）*
   /slack-review-notify [ラベル名] add-repo owner/repo

:information_source: この2つを設定しないと通知は送信されません

*複数ラベル設定の使い方*
このチャンネル内で複数の異なるラベルごとに独立した設定を持つことができます:
- 例1: /slack-review-notify bug set-mention @バグチーム → bugラベル用の設定
- 例2: /slack-review-notify feature set-mention @開発チーム → featureラベル用の設定
- 例3: /slack-review-notify security-review set-mention @セキュリティチーム → security-reviewラベル用の設定

*複数ラベルAND条件の設定*
カンマ区切りで複数のラベルを指定することで、全てのラベルが付いている場合のみ通知します:
- 例: /slack-review-notify "hoge-project,needs-review" set-mention @team
  → PRに「hoge-project」と「needs-review」の両方のラベルがある場合のみ通知
- 例: /slack-review-notify "frontend,urgent,needs-review" add-repo owner/app
  → 3つのラベル全てが必要

*全コマンド一覧*
*基本操作:*
• /slack-review-notify show - このチャンネルの全ラベル設定を表示
• /slack-review-notify [ラベル名] show - 指定ラベルの詳細設定を表示

*必須設定:*
• /slack-review-notify [ラベル名] set-mention @user - メンション先を設定
• /slack-review-notify [ラベル名] add-repo owner/repo1,owner/repo2 - 対象リポジトリを追加

*レビュワー管理:*
• /slack-review-notify [ラベル名] add-reviewer @user1,@user2 - レビュワーを追加
• /slack-review-notify [ラベル名] show-reviewers - レビュワー一覧を表示
• /slack-review-notify [ラベル名] clear-reviewers - レビュワーをクリア

*高度な設定:*
• /slack-review-notify [ラベル名] remove-repo owner/repo - リポジトリを削除
• /slack-review-notify [ラベル名] set-label 新ラベル名 - ラベル名を変更
• /slack-review-notify [ラベル名] set-reviewer-reminder-interval 30 - リマインド頻度設定（分）
• /slack-review-notify [ラベル名] set-business-hours-start 09:00 - 営業開始時間を設定
• /slack-review-notify [ラベル名] set-business-hours-end 18:00 - 営業終了時間を設定
• /slack-review-notify [ラベル名] set-timezone Asia/Tokyo - タイムゾーンを設定
• /slack-review-notify [ラベル名] set-required-approvals N - 必要なapprove数を設定（1〜10）
• /slack-review-notify [ラベル名] set-language ja|en - メッセージの言語を設定
• /slack-review-notify [ラベル名] activate - 通知を有効化
• /slack-review-notify [ラベル名] deactivate - 通知を無効化

*ユーザーマッピング（PR作成者の通知用）:*
• /slack-review-notify map-user <github-username> @slack-user - GitHubユーザーとSlackユーザーを紐付け
• /slack-review-notify show-user-mappings - 登録済みのユーザーマッピング一覧を表示
• /slack-review-notify remove-user-mapping <github-username> - ユーザーマッピングを削除

*休暇管理:*
• /slack-review-notify set-away @user [until YYYY-MM-DD] [reason 理由] - ユーザーを休暇に設定
• /slack-review-notify unset-away @user - ユーザーの休暇を解除
• /slack-review-notify show-availability - 休暇中のユーザー一覧を表示

[ラベル名]を省略すると「needs-review」というデフォルトのラベルを使用します`,

	// ==================== Command: set-language ====================
	"cmd.set_language.usage":   "言語を指定してください。対応言語: ja (日本語), en (English)\n例: /slack-review-notify %s set-language en",
	"cmd.set_language.invalid": "対応していない言語です。対応言語: ja (日本語), en (English)",
	"cmd.set_language.set":     "ラベル「%s」の言語を %s に設定しました。",
	"cmd.set_language.updated": "ラベル「%s」の言語を %s に更新しました。",

	// ==================== Command: set-mention ====================
	"cmd.set_mention.usage":   "メンション先のユーザーIDを指定してください。例: /slack-review-notify %s set-mention @user",
	"cmd.set_mention.created": "ラベル「%s」のメンション先を %s に設定しました。",
	"cmd.set_mention.updated": "ラベル「%s」のメンション先を %s に更新しました。",

	// ==================== Command: add-reviewer ====================
	"cmd.add_reviewer.usage":   "レビュワーのユーザーIDをカンマ区切りで指定してください。例: /slack-review-notify %s add-reviewer @user1,@user2",
	"cmd.add_reviewer.created": "ラベル「%s」のレビュワーリストを設定しました: %s",
	"cmd.add_reviewer.updated": "ラベル「%s」のレビュワーリストを更新しました: %s",

	// ==================== Command: show-reviewers ====================
	"cmd.show_reviewers.no_config": "このチャンネルのラベル「%s」の設定はまだありません。/slack-review-notify %s add-reviewer コマンドで設定を開始してください。",
	"cmd.show_reviewers.empty":     "現在レビュワーはラベル「%s」に登録されていません。/slack-review-notify %s add-reviewer コマンドで追加してください。",
	"cmd.show_reviewers.header":    "*このチャンネルのラベル「%s」のレビュワーリスト*\n%s",

	// ==================== Command: clear-reviewers ====================
	"cmd.clear_reviewers.no_config": "このチャンネルのラベル「%s」の設定はまだありません。",
	"cmd.clear_reviewers.success":   "ラベル「%s」のレビュワーリストをクリアしました。",

	// ==================== Command: add-repo ====================
	"cmd.add_repo.usage":          "リポジトリ名をカンマ区切りで指定してください。例: /slack-review-notify %s add-repo owner/repo1,owner/repo2",
	"cmd.add_repo.created":        "ラベル「%s」の通知対象リポジトリに `%s` を追加しました。",
	"cmd.add_repo.added":          "ラベル「%s」の通知対象リポジトリに以下を追加しました:\n`%s`",
	"cmd.add_repo.already_exists": "以下のリポジトリは既に通知対象でした:\n`%s`",
	"cmd.add_repo.no_valid":       "有効なリポジトリ名が指定されませんでした。",

	// ==================== Command: remove-repo ====================
	"cmd.remove_repo.usage":     "リポジトリ名を指定してください。例: /slack-review-notify %s remove-repo owner/repo",
	"cmd.remove_repo.no_config": "このチャンネルのラベル「%s」の設定はまだありません。",
	"cmd.remove_repo.empty":     "ラベル「%s」の通知対象リポジトリは設定されていません。",
	"cmd.remove_repo.not_found": "リポジトリ `%s` はラベル「%s」の通知対象ではありません。",
	"cmd.remove_repo.success":   "ラベル「%s」の通知対象リポジトリから `%s` を削除しました。",

	// ==================== Command: set-label ====================
	"cmd.set_label.usage":          "新しいラベル名を指定してください。例: /slack-review-notify %s set-label new-label-name",
	"cmd.set_label.no_config":      "ラベル「%s」の設定はこのチャンネルに存在しません。",
	"cmd.set_label.already_exists": "ラベル「%s」の設定は既に存在します。別のラベル名を指定してください。",
	"cmd.set_label.success":        "ラベル名を「%s」から「%s」に変更しました。",

	// ==================== Command: activate/deactivate ====================
	"cmd.activate.no_config":   "このチャンネルのラベル「%s」の設定はまだありません。/slack-review-notify %s set-mention コマンドで設定を開始してください。",
	"cmd.deactivate.no_config": "このチャンネルのラベル「%s」の設定はまだありません。",
	"cmd.activate.success":     "このチャンネルでのラベル「%s」のレビュー通知を有効化しました。",
	"cmd.deactivate.success":   "このチャンネルでのラベル「%s」のレビュー通知を無効化しました。",

	// ==================== Command: set-reviewer-reminder-interval ====================
	"cmd.set_reminder_interval.usage":        "レビュワー割り当て後のリマインド頻度を分単位で指定してください。例: /slack-review-notify %s set-reviewer-reminder-interval 30",
	"cmd.set_reminder_interval.invalid":      "リマインド頻度は1以上の整数で指定してください。",
	"cmd.set_reminder_interval.reviewer_set": "レビュワー割り当て後のリマインド頻度を %d分 に設定しました。",
	"cmd.set_reminder_interval.pending_set":  "レビュワー募集中のリマインド頻度を %d分 に設定しました。",

	// ==================== Command: business hours ====================
	"cmd.time_format_invalid":            "時間形式が無効です。HH:MM形式で指定してください（例: %s）",
	"cmd.set_business_hours_start.usage": "営業開始時間を指定してください。例: /slack-review-notify %s set-business-hours-start 09:00",
	"cmd.set_business_hours_start.set":   "ラベル「%s」の営業開始時間を %s に設定しました。",
	"cmd.set_business_hours_start.updated": "ラベル「%s」の営業開始時間を %s に更新しました。",
	"cmd.set_business_hours_end.usage":   "営業終了時間を指定してください。例: /slack-review-notify %s set-business-hours-end 18:00",
	"cmd.set_business_hours_end.set":     "ラベル「%s」の営業終了時間を %s に設定しました。",
	"cmd.set_business_hours_end.updated": "ラベル「%s」の営業終了時間を %s に更新しました。",

	// ==================== Command: timezone ====================
	"cmd.set_timezone.usage":   "タイムゾーンを指定してください。例: /slack-review-notify %s set-timezone Asia/Tokyo",
	"cmd.set_timezone.invalid": "無効なタイムゾーンです。例: Asia/Tokyo, UTC, America/New_York",
	"cmd.set_timezone.set":     "ラベル「%s」のタイムゾーンを %s に設定しました。",
	"cmd.set_timezone.updated": "ラベル「%s」のタイムゾーンを %s に更新しました。",

	// ==================== Command: required-approvals ====================
	"cmd.set_required_approvals.usage":   "必要なapprove数を指定してください。例: /slack-review-notify %s set-required-approvals 2",
	"cmd.set_required_approvals.invalid": "必要なapprove数は1〜10の整数で指定してください。",
	"cmd.set_required_approvals.set":     "ラベル「%s」の必要なapprove数を %d に設定しました。",
	"cmd.set_required_approvals.updated": "ラベル「%s」の必要なapprove数を %d に更新しました。",

	// ==================== Command: show ====================
	"cmd.show.error":     "設定の取得中にエラーが発生しました。",
	"cmd.show.no_config": "このチャンネルにはまだ設定がありません。/slack-review-notify [ラベル名] set-mention コマンドで設定を開始してください。",
	"cmd.show.header":    "*このチャンネルで設定済みのラベル*\n",
	"cmd.show.footer":    "\n特定のラベルの詳細設定を確認するには: `/slack-review-notify [ラベル名] show`",

	// ==================== Command: show config ====================
	"cmd.show_config.no_config": "このチャンネルのラベル「%s」の設定はまだありません。/slack-review-notify %s set-mention コマンドで設定を開始してください。",
	"cmd.show_config.response": `*このチャンネルのラベル「%s」のレビュー通知設定*
- ステータス: %s
- メンション先: <@%s>
- レビュワーリスト: %s
- 通知対象リポジトリ: %s
- レビュワー割り当て後のリマインド頻度: %d分
- 営業時間: %s - %s (%s)
- 必要なapprove数: %d
- 言語: %s`,

	// ==================== Command: map-user ====================
	"cmd.map_user.usage":        "使用方法: /slack-review-notify map-user <github-username> @slack-user\n例: /slack-review-notify map-user octocat @user",
	"cmd.map_user.invalid":      "GitHubユーザー名とSlackユーザーIDの両方を指定してください。",
	"cmd.map_user.updated":      "GitHubユーザー `%s` のマッピングを <@%s> に更新しました。",
	"cmd.map_user.create_error": "ユーザーマッピングの作成に失敗しました。",
	"cmd.map_user.created":      "GitHubユーザー `%s` を <@%s> にマッピングしました。",

	// ==================== Command: show-user-mappings ====================
	"cmd.show_user_mappings.error":  "ユーザーマッピングの取得に失敗しました。",
	"cmd.show_user_mappings.empty":  "まだユーザーマッピングが登録されていません。\n/slack-review-notify map-user コマンドで登録してください。",
	"cmd.show_user_mappings.header": "*登録済みのユーザーマッピング*\n",

	// ==================== Command: remove-user-mapping ====================
	"cmd.remove_user_mapping.usage":     "使用方法: /slack-review-notify remove-user-mapping <github-username>\n例: /slack-review-notify remove-user-mapping octocat",
	"cmd.remove_user_mapping.not_found": "GitHubユーザー `%s` のマッピングは存在しません。",
	"cmd.remove_user_mapping.error":     "ユーザーマッピングの削除に失敗しました。",
	"cmd.remove_user_mapping.success":   "GitHubユーザー `%s` のマッピングを削除しました。",

	// ==================== Command: set-away ====================
	"cmd.set_away.usage":        "休暇に設定するユーザーを指定してください。例: /slack-review-notify set-away @user [until YYYY-MM-DD] [reason 理由]",
	"cmd.set_away.no_user":      "休暇に設定するユーザーを指定してください。",
	"cmd.set_away.invalid_date": "日付形式が無効です。YYYY-MM-DD形式で指定してください（例: 2025-06-01）",
	"cmd.set_away.past_date":    "過去の日付は指定できません。今日以降の日付を指定してください。",
	"cmd.set_away.update_error": "休暇設定の更新に失敗しました。",
	"cmd.set_away.create_error": "休暇設定の作成に失敗しました。",
	"cmd.set_away.success":      "<@%s> を休暇に設定しました",

	// ==================== Command: unset-away ====================
	"cmd.unset_away.usage":   "休暇を解除するユーザーを指定してください。例: /slack-review-notify unset-away @user",
	"cmd.unset_away.not_set": "<@%s> は休暇に設定されていません。",
	"cmd.unset_away.success": "<@%s> の休暇を解除しました",

	// ==================== Command: show-availability ====================
	"cmd.show_availability.empty":  "現在休暇中のユーザーはいません",
	"cmd.show_availability.header": "*現在休暇中のユーザー*\n",

	// ==================== Common ====================
	"common.active":             "有効",
	"common.inactive":           "無効",
	"common.not_set":            "未設定",
	"common.invalid_user_id":    "有効なユーザーIDを指定してください。",
	"common.select_placeholder": "選択してください",
	"common.until":              "%s まで",
	"common.indefinite":         "無期限",
	"common.reason":             "、理由: %s",
	"common.reason_paren":       "（%s）",

	// ==================== Pause Options ====================
	"pause.1h":    "1時間停止",
	"pause.2h":    "2時間停止",
	"pause.4h":    "4時間停止",
	"pause.today": "今日は通知しない (翌営業日の朝まで停止)",
	"pause.stop":  "リマインダーを完全に停止",

	// ==================== Button Labels ====================
	"button.review_done":     "レビュー完了",
	"button.pause_reminder":  "リマインドを一時停止",
	"button.stop_reminder":   "リマインダーを停止",
	"button.change_reviewer": "変わってほしい！",

	// ==================== Notifications ====================
	"notify.off_hours.with_creator":    "<@%s> からのレビュー依頼が登録されました\n\n*PRタイトル*: %s\n*URL*: <%s>\n\n📝 レビューのメンションは翌営業日の朝にお送りします",
	"notify.off_hours.without_creator": "📝 *レビュー対象のPRが登録されました*\n\n*PRタイトル*: %s\n*URL*: <%s>\n\n (レビューのメンションは翌営業日の朝にお送りします)",

	"notify.review_request.with_creator":    "%s <@%s> からのレビュー依頼があります\n\n*PRタイトル*: %s\n*URL*: <%s>",
	"notify.review_request.without_creator": "%s *レビュー対象のPRがあります！*\n\n*PRタイトル*: %s\n*URL*: <%s>",

	"notify.business_hours_morning":  "🌅 *おはようございます！* %s\n\n📋 こちらのPRのレビューをお願いします。%s",
	"notify.reviewer_in_morning":     "\n\n🎯 *レビュワー*: @%s さん、よろしくお願いします！",
	"notify.reminder":                "%s レビューしてくれたら嬉しいです...👀",
	"notify.out_of_hours_reminder":   "@%s レビューしてくれたら嬉しいです...👀\n\n営業時間外のため、次回のリマインドは翌営業日に送信します。",

	"notify.reminder_paused.1h":      "はい！1時間リマインドをストップします！",
	"notify.reminder_paused.2h":      "はい！2時間リマインドをストップします！",
	"notify.reminder_paused.4h":      "はい！4時間リマインドをストップします！",
	"notify.reminder_paused.today":   "今日はもうリマインドしません。翌営業日の朝に再開します！",
	"notify.reminder_paused.stop":    "リマインダーを完全に停止しました。レビュー担当者が決まるまで通知しません。",
	"notify.reminder_paused.default": "リマインドをストップします！",

	"notify.reviewer_auto_assigned":  "自動でレビュワーが割り当てられました: %s レビューをお願いします！",
	"notify.reviewer_changed":        "レビュワーを変更しました: %s → %s さん、よろしくお願いします！",
	"notify.cannot_change_reviewer":  "レビュワーが1人しか登録されていないため、変更できません。他のレビュワーを登録してください。",

	"notify.task_completed":   "✅ *%s*\n🔗 %s\n\n*レビュー完了*: このPRのラベルが外れたため、レビュータスクを終了しました。",
	"notify.review_done_button": "✅ <@%s> さんがレビューを完了しました！感謝！👏",

	"notify.review_approved":          "%s %sさんがレビューを承認しました！感謝！👏",
	"notify.review_changes_requested": "%s %sさんが変更を要求しました 感謝！👏",
	"notify.review_commented":         "%s %sさんがレビューコメントを残しました 感謝！👏",
	"notify.review_default":           "%s %sさんがレビューしました 感謝！👏",

	"notify.label_removed_single":   "🏷️ `%s`ラベルが削除されたため、レビュータスクを完了しました。",
	"notify.label_removed_multiple": "🏷️ `%s`ラベルのいずれかが削除されたため、レビュータスクを完了しました。",

	"notify.pr_merged": "🎉 PRがマージされました！お疲れさまでした！",
	"notify.pr_closed": "🔒 PRがクローズされました。",

	"notify.re_review_requested": "🔄 %s さんが %s に再レビューを依頼しました。対応をお願いします！",
	"notify.re_review_deferred":  "🔄 %s が再レビューをリクエストしました。営業時間外のため、翌営業日の朝に通知します。",
	"notify.fully_approved":      "🎉 %d/%d approved - レビュー完了！",
}
