import { test, expect, APIRequestContext } from "@playwright/test";
import { createHmac } from "crypto";

const SLACK_SECRET = "test-signing-secret";
const GITHUB_SECRET = "test-webhook-secret";
const CHANNEL = "C12345";
const USER = "U12345";

// ── helpers ──────────────────────────────────────────────────────

function slackSignature(body: string): { ts: string; sig: string } {
  const ts = Math.floor(Date.now() / 1000).toString();
  const base = `v0:${ts}:${body}`;
  const sig =
    "v0=" + createHmac("sha256", SLACK_SECRET).update(base).digest("hex");
  return { ts, sig };
}

async function slackCmd(
  request: APIRequestContext,
  text: string,
  channel = CHANNEL
): Promise<string> {
  const body = `command=%2Fslack-review-notify&text=${encodeURIComponent(text)}&channel_id=${channel}&user_id=${USER}`;
  const { ts, sig } = slackSignature(body);
  const res = await request.post("/slack/command", {
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-Slack-Request-Timestamp": ts,
      "X-Slack-Signature": sig,
    },
    data: body,
  });
  expect(res.status()).toBe(200);
  return res.text();
}

async function githubWebhook(
  request: APIRequestContext,
  event: string,
  payload: object
): Promise<number> {
  const body = JSON.stringify(payload);
  const sig =
    "sha256=" + createHmac("sha256", GITHUB_SECRET).update(body).digest("hex");
  const res = await request.post("/webhook", {
    headers: {
      "Content-Type": "application/json",
      "X-GitHub-Event": event,
      "X-Hub-Signature-256": sig,
    },
    data: body,
  });
  return res.status();
}

async function slackAction(
  request: APIRequestContext,
  payload: object
): Promise<number> {
  const payloadStr = JSON.stringify(payload);
  const body = `payload=${encodeURIComponent(payloadStr)}`;
  const { ts, sig } = slackSignature(body);
  const res = await request.post("/slack/actions", {
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-Slack-Request-Timestamp": ts,
      "X-Slack-Signature": sig,
    },
    data: body,
  });
  return res.status();
}

// ── 1. Help ──────────────────────────────────────────────────────

test.describe("1. Help command", () => {
  test("1-1. help returns Japanese by default", async ({ request }) => {
    const res = await slackCmd(request, "help");
    expect(res).toContain("Review通知Bot設定コマンド");
    expect(res).toContain("set-language ja|en");
  });
});

// ── 2. set-mention ───────────────────────────────────────────────

test.describe("2. set-mention", () => {
  test("2-1. create new config with set-mention (ja)", async ({ request }) => {
    const res = await slackCmd(request, "set-mention <@UMENTIONTEST>");
    expect(res).toContain("メンション先を");
    expect(res).toMatch(/設定しました|更新しました/);
  });

  test("2-2. update existing mention (ja)", async ({ request }) => {
    await slackCmd(request, "test-label set-mention <@U111>");
    const res = await slackCmd(request, "test-label set-mention <@U222>");
    expect(res).toContain("更新しました");
  });

  test("2-3. missing params shows usage", async ({ request }) => {
    const res = await slackCmd(request, "set-mention");
    expect(res).toContain("メンション先のユーザーID");
    expect(res).toContain("例:");
  });
});

// ── 3. add-reviewer ──────────────────────────────────────────────

test.describe("3. add-reviewer", () => {
  test("3-1. add reviewer creates list (ja)", async ({ request }) => {
    const res = await slackCmd(request, "add-reviewer <@UREV1>,<@UREV2>");
    expect(res).toContain("レビュワーリスト");
  });

  test("3-2. add more reviewers updates list", async ({ request }) => {
    const res = await slackCmd(request, "add-reviewer <@UREV3>");
    expect(res).toContain("更新しました");
  });

  test("3-3. missing params shows usage", async ({ request }) => {
    const res = await slackCmd(request, "add-reviewer");
    expect(res).toContain("カンマ区切り");
  });
});

// ── 4. show-reviewers ────────────────────────────────────────────

test.describe("4. show-reviewers", () => {
  test("4-1. show-reviewers with reviewers", async ({ request }) => {
    const res = await slackCmd(request, "show-reviewers");
    expect(res).toContain("レビュワーリスト");
    expect(res).toContain("<@UREV1>");
  });

  test("4-2. show-reviewers for unconfigured label", async ({ request }) => {
    const res = await slackCmd(request, "nonexist-label show-reviewers");
    expect(res).toContain("設定はまだありません");
  });
});

// ── 5. clear-reviewers ───────────────────────────────────────────

test.describe("5. clear-reviewers", () => {
  test("5-1. clear-reviewers success", async ({ request }) => {
    await slackCmd(request, "clear-test set-mention <@U000>");
    await slackCmd(request, "clear-test add-reviewer <@UREVX>");
    const res = await slackCmd(request, "clear-test clear-reviewers");
    expect(res).toContain("クリアしました");
  });
});

// ── 6. add-repo ──────────────────────────────────────────────────

test.describe("6. add-repo", () => {
  test("6-1. add repo success", async ({ request }) => {
    const res = await slackCmd(request, "add-repo owner/test-repo");
    expect(res).toContain("追加しました");
  });

  test("6-2. add duplicate repo", async ({ request }) => {
    const res = await slackCmd(request, "add-repo owner/test-repo");
    expect(res).toContain("既に通知対象");
  });

  test("6-3. missing params shows usage", async ({ request }) => {
    const res = await slackCmd(request, "add-repo");
    expect(res).toContain("リポジトリ名をカンマ区切り");
  });
});

// ── 7. remove-repo ───────────────────────────────────────────────

test.describe("7. remove-repo", () => {
  test("7-1. remove-repo success", async ({ request }) => {
    await slackCmd(request, "rm-test set-mention <@U000>");
    await slackCmd(request, "rm-test add-repo owner/to-remove");
    const res = await slackCmd(request, "rm-test remove-repo owner/to-remove");
    expect(res).toContain("削除しました");
  });

  test("7-2. remove-repo not found or empty", async ({ request }) => {
    const res = await slackCmd(request, "rm-test remove-repo owner/nonexist");
    expect(res).toMatch(/通知対象ではありません|設定されていません/);
  });
});

// ── 8. set-label (rename) ────────────────────────────────────────

test.describe("8. set-label", () => {
  test("8-1. rename label success", async ({ request }) => {
    await slackCmd(request, "rename-me set-mention <@U000>");
    const res = await slackCmd(request, "rename-me set-label renamed-label");
    expect(res).toContain("変更しました");
    expect(res).toContain("rename-me");
    expect(res).toContain("renamed-label");
  });

  test("8-2. missing params shows usage", async ({ request }) => {
    const res = await slackCmd(request, "needs-review set-label");
    expect(res).toContain("新しいラベル名");
  });
});

// ── 9. activate / deactivate ─────────────────────────────────────

test.describe("9. activate / deactivate", () => {
  test("9-1. deactivate success", async ({ request }) => {
    const res = await slackCmd(request, "deactivate");
    expect(res).toContain("無効化しました");
  });

  test("9-2. activate success", async ({ request }) => {
    const res = await slackCmd(request, "activate");
    expect(res).toContain("有効化しました");
  });
});

// ── 10. set-reviewer-reminder-interval ───────────────────────────

test.describe("10. set-reviewer-reminder-interval", () => {
  test("10-1. set interval success", async ({ request }) => {
    const res = await slackCmd(request, "set-reviewer-reminder-interval 60");
    expect(res).toContain("リマインド頻度");
    expect(res).toContain("60");
  });

  test("10-2. invalid interval", async ({ request }) => {
    const res = await slackCmd(request, "set-reviewer-reminder-interval abc");
    expect(res).toContain("1以上の整数");
  });
});

// ── 11. business hours ───────────────────────────────────────────

test.describe("11. Business hours", () => {
  test("11-1. set-business-hours-start", async ({ request }) => {
    const res = await slackCmd(request, "set-business-hours-start 08:30");
    expect(res).toContain("営業開始時間");
    expect(res).toContain("08:30");
  });

  test("11-2. set-business-hours-end", async ({ request }) => {
    const res = await slackCmd(request, "set-business-hours-end 17:30");
    expect(res).toContain("営業終了時間");
    expect(res).toContain("17:30");
  });

  test("11-3. invalid time format", async ({ request }) => {
    const res = await slackCmd(request, "set-business-hours-start 25:00");
    expect(res).toContain("HH:MM");
  });
});

// ── 12. set-timezone ─────────────────────────────────────────────

test.describe("12. set-timezone", () => {
  test("12-1. set timezone success", async ({ request }) => {
    const res = await slackCmd(request, "set-timezone America/New_York");
    expect(res).toContain("タイムゾーン");
    expect(res).toContain("America/New_York");
  });

  test("12-2. invalid timezone", async ({ request }) => {
    const res = await slackCmd(request, "set-timezone Invalid/Zone");
    expect(res).toContain("無効なタイムゾーン");
  });
});

// ── 13. set-required-approvals ───────────────────────────────────

test.describe("13. set-required-approvals", () => {
  test("13-1. set required approvals", async ({ request }) => {
    const res = await slackCmd(request, "set-required-approvals 3");
    expect(res).toContain("approve数");
    expect(res).toContain("3");
  });

  test("13-2. invalid value", async ({ request }) => {
    const res = await slackCmd(request, "set-required-approvals 99");
    expect(res).toContain("1〜10");
  });
});

// ── 14. show (all labels / specific label) ───────────────────────

test.describe("14. show", () => {
  test("14-1. show all labels", async ({ request }) => {
    const res = await slackCmd(request, "show");
    expect(res).toContain("設定済みのラベル");
    expect(res).toContain("needs-review");
  });

  test("14-2. show specific label", async ({ request }) => {
    const res = await slackCmd(request, "needs-review show");
    expect(res).toContain("レビュー通知設定");
    expect(res).toContain("ステータス");
    expect(res).toContain("言語");
  });
});

// ── 15. map-user ─────────────────────────────────────────────────

test.describe("15. map-user", () => {
  test("15-1. create mapping", async ({ request }) => {
    const res = await slackCmd(request, "map-user testuser <@USLACK1>");
    expect(res).toContain("マッピングしました");
  });

  test("15-2. update mapping", async ({ request }) => {
    const res = await slackCmd(request, "map-user testuser <@USLACK2>");
    expect(res).toContain("更新しました");
  });

  test("15-3. missing params", async ({ request }) => {
    const res = await slackCmd(request, "map-user");
    expect(res).toContain("使用方法");
  });
});

// ── 16. show-user-mappings ───────────────────────────────────────

test.describe("16. show-user-mappings", () => {
  test("16-1. show mappings with data", async ({ request }) => {
    const res = await slackCmd(request, "show-user-mappings");
    expect(res).toContain("ユーザーマッピング");
    expect(res).toContain("testuser");
  });
});

// ── 17. remove-user-mapping ──────────────────────────────────────

test.describe("17. remove-user-mapping", () => {
  test("17-1. remove existing mapping", async ({ request }) => {
    const res = await slackCmd(request, "remove-user-mapping testuser");
    expect(res).toContain("削除しました");
  });

  test("17-2. remove non-existent mapping", async ({ request }) => {
    const res = await slackCmd(request, "remove-user-mapping nobody");
    expect(res).toContain("存在しません");
  });
});

// ── 18. set-away ─────────────────────────────────────────────────

test.describe("18. set-away", () => {
  test("18-1. set away indefinite", async ({ request }) => {
    const res = await slackCmd(request, "set-away <@UAWAY1>");
    expect(res).toContain("休暇に設定しました");
    expect(res).toContain("無期限");
  });

  test("18-2. set away with date and reason", async ({ request }) => {
    const res = await slackCmd(
      request,
      "set-away <@UAWAY2> until 2026-12-31 reason vacation"
    );
    expect(res).toContain("休暇に設定しました");
    expect(res).toContain("2026-12-31");
    expect(res).toContain("vacation");
  });

  test("18-3. past date rejected", async ({ request }) => {
    const res = await slackCmd(
      request,
      "set-away <@UAWAY3> until 2020-01-01"
    );
    expect(res).toContain("過去の日付");
  });

  test("18-4. missing params", async ({ request }) => {
    const res = await slackCmd(request, "set-away");
    expect(res).toContain("休暇に設定するユーザー");
  });
});

// ── 19. show-availability ────────────────────────────────────────

test.describe("19. show-availability", () => {
  test("19-1. show users on leave", async ({ request }) => {
    const res = await slackCmd(request, "show-availability");
    expect(res).toContain("休暇中のユーザー");
    expect(res).toContain("<@UAWAY1>");
  });
});

// ── 20. unset-away ───────────────────────────────────────────────

test.describe("20. unset-away", () => {
  test("20-1. unset-away success", async ({ request }) => {
    const res = await slackCmd(request, "unset-away <@UAWAY1>");
    expect(res).toContain("休暇を解除しました");
  });

  test("20-2. unset-away not set", async ({ request }) => {
    const res = await slackCmd(request, "unset-away <@UNOTAWAY>");
    expect(res).toContain("休暇に設定されていません");
  });
});

// ── 21. unknown command ──────────────────────────────────────────

test.describe("21. Unknown command", () => {
  test("21-1. completely unknown command", async ({ request }) => {
    // This creates a config lookup for a label named "xyz" with subcommand "badcmd"
    await slackCmd(request, "needs-review set-mention <@U000>"); // ensure config exists
    const res = await slackCmd(request, "needs-review badcmd");
    expect(res).toContain("不明なコマンド");
  });
});

// ── 22. set-language & i18n ──────────────────────────────────────

test.describe("22. set-language & i18n switching", () => {
  const CH = "C_I18N";

  test("22-1. default is Japanese", async ({ request }) => {
    const res = await slackCmd(request, "set-mention <@UI18N>", CH);
    expect(res).toContain("メンション先を");
  });

  test("22-2. switch to English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    const res = await slackCmd(request, "set-language en", CH);
    expect(res).toContain("Updated language");
    expect(res).toContain("en");
  });

  test("22-3. help in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "help", CH);
    expect(res).toContain("Review Notification Bot Configuration");
    expect(res).not.toContain("Review通知Bot");
  });

  test("22-4. show in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "show", CH);
    expect(res).toContain("Configured labels");
    expect(res).toContain("Active");
  });

  test("22-5. show config in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "needs-review show", CH);
    expect(res).toContain("Review notification settings");
    expect(res).toContain("Language: en");
  });

  test("22-6. add-reviewer in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "add-reviewer <@UENREV>", CH);
    expect(res).toContain("reviewer list");
  });

  test("22-7. add-repo in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "add-repo owner/en-repo", CH);
    expect(res).toContain("Added");
    expect(res).toContain("notification target");
  });

  test("22-8. activate in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    await slackCmd(request, "deactivate", CH);
    const res = await slackCmd(request, "activate", CH);
    expect(res).toContain("Enabled");
    expect(res).toContain("notifications");
  });

  test("22-9. set-timezone in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "set-timezone UTC", CH);
    expect(res).toContain("timezone");
    expect(res).toContain("UTC");
  });

  test("22-10. set-away in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "set-away <@UENAWAY>", CH);
    expect(res).toContain("Set");
    expect(res).toContain("as away");
    expect(res).toContain("Indefinite");
  });

  test("22-11. show-availability in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "show-availability", CH);
    expect(res).toContain("Currently on Leave");
  });

  test("22-12. unset-away in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    await slackCmd(request, "set-away <@UENAWAY2>", CH);
    const res = await slackCmd(request, "unset-away <@UENAWAY2>", CH);
    expect(res).toContain("Removed away status");
  });

  test("22-13. map-user in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "map-user enuser <@UENMAP>", CH);
    expect(res).toContain("Mapped");
  });

  test("22-14. show-user-mappings in English", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const res = await slackCmd(request, "show-user-mappings", CH);
    expect(res).toContain("User Mappings");
  });

  test("22-15. invalid language rejected", async ({ request }) => {
    const res = await slackCmd(request, "set-language fr", CH);
    expect(res).toMatch(/対応していない言語|Unsupported language/);
  });

  test("22-16. switch back to Japanese", async ({ request }) => {
    await slackCmd(request, "set-mention <@UI18N>", CH);
    await slackCmd(request, "set-language en", CH);
    const resEn = await slackCmd(request, "set-language ja", CH);
    expect(resEn).toContain("言語を ja に更新しました");
    const resJa = await slackCmd(request, "show", CH);
    expect(resJa).toContain("設定済みのラベル");
  });
});

// ── 23. GitHub Webhook - labeled event ───────────────────────────

test.describe("23. GitHub Webhook - labeled", () => {
  test("23-1. labeled event creates task and sends notification", async ({
    request,
  }) => {
    // Setup channel
    await slackCmd(request, "set-mention <@UWHMENTION>", "C67890");
    await slackCmd(request, "add-repo owner/webhook-repo", "C67890");
    await slackCmd(request, "add-reviewer <@UWHREV1>", "C67890");

    const status = await githubWebhook(request, "pull_request", {
      action: "labeled",
      label: { name: "needs-review" },
      pull_request: {
        number: 200,
        title: "Webhook test PR",
        html_url: "https://github.com/owner/webhook-repo/pull/200",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "prauthor" },
      },
      repository: { name: "webhook-repo", owner: { login: "owner" } },
    });
    expect(status).toBe(200);
  });

  test("23-2. labeled event with English channel", async ({ request }) => {
    await slackCmd(request, "set-mention <@UWHEN>", "C1234567890");
    await slackCmd(request, "add-repo owner/en-webhook-repo", "C1234567890");
    await slackCmd(request, "set-language en", "C1234567890");

    const status = await githubWebhook(request, "pull_request", {
      action: "labeled",
      label: { name: "needs-review" },
      pull_request: {
        number: 201,
        title: "English webhook test",
        html_url: "https://github.com/owner/en-webhook-repo/pull/201",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "enauthor" },
      },
      repository: { name: "en-webhook-repo", owner: { login: "owner" } },
    });
    expect(status).toBe(200);
  });

  test("23-3. labeled event - repo not watched", async ({ request }) => {
    const status = await githubWebhook(request, "pull_request", {
      action: "labeled",
      label: { name: "needs-review" },
      pull_request: {
        number: 300,
        title: "Unwatched PR",
        html_url: "https://github.com/other/unwatched/pull/300",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "someone" },
      },
      repository: { name: "unwatched", owner: { login: "other" } },
    });
    expect(status).toBe(200);
  });
});

// ── 24. GitHub Webhook - review submitted ────────────────────────

test.describe("24. GitHub Webhook - review submitted", () => {
  test("24-1. approved review", async ({ request }) => {
    // Wait for task creation from test 23-1
    await new Promise((r) => setTimeout(r, 1000));

    const status = await githubWebhook(request, "pull_request_review", {
      action: "submitted",
      review: { state: "approved", user: { login: "reviewer1" } },
      pull_request: {
        number: 200,
        title: "Webhook test PR",
        html_url: "https://github.com/owner/webhook-repo/pull/200",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "prauthor" },
      },
      repository: { name: "webhook-repo", owner: { login: "owner" } },
    });
    expect(status).toBe(200);
  });

  test("24-2. changes_requested review", async ({ request }) => {
    const status = await githubWebhook(request, "pull_request_review", {
      action: "submitted",
      review: { state: "changes_requested", user: { login: "reviewer2" } },
      pull_request: {
        number: 201,
        title: "English webhook test",
        html_url: "https://github.com/owner/en-webhook-repo/pull/201",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "enauthor" },
      },
      repository: { name: "en-webhook-repo", owner: { login: "owner" } },
    });
    expect(status).toBe(200);
  });

  test("24-3. commented review", async ({ request }) => {
    const status = await githubWebhook(request, "pull_request_review", {
      action: "submitted",
      review: { state: "commented", user: { login: "reviewer3" } },
      pull_request: {
        number: 201,
        title: "English webhook test",
        html_url: "https://github.com/owner/en-webhook-repo/pull/201",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "enauthor" },
      },
      repository: { name: "en-webhook-repo", owner: { login: "owner" } },
    });
    expect(status).toBe(200);
  });
});

// ── 25. GitHub Webhook - unlabeled ───────────────────────────────

test.describe("25. GitHub Webhook - unlabeled", () => {
  test("25-1. unlabeled completes task", async ({ request }) => {
    // Create a new task first
    await slackCmd(request, "set-mention <@UUNLABEL>", "C67890");
    await githubWebhook(request, "pull_request", {
      action: "labeled",
      label: { name: "needs-review" },
      pull_request: {
        number: 400,
        title: "Unlabel test",
        html_url: "https://github.com/owner/webhook-repo/pull/400",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "dev" },
      },
      repository: { name: "webhook-repo", owner: { login: "owner" } },
    });
    await new Promise((r) => setTimeout(r, 1000));

    const status = await githubWebhook(request, "pull_request", {
      action: "unlabeled",
      label: { name: "needs-review" },
      pull_request: {
        number: 400,
        title: "Unlabel test",
        html_url: "https://github.com/owner/webhook-repo/pull/400",
        state: "open",
        labels: [],
        user: { login: "dev" },
      },
      repository: { name: "webhook-repo", owner: { login: "owner" } },
    });
    expect(status).toBe(200);
  });
});

// ── 26. GitHub Webhook - closed / merged ─────────────────────────

test.describe("26. GitHub Webhook - closed", () => {
  test("26-1. PR closed (not merged)", async ({ request }) => {
    await githubWebhook(request, "pull_request", {
      action: "labeled",
      label: { name: "needs-review" },
      pull_request: {
        number: 500,
        title: "Close test",
        html_url: "https://github.com/owner/webhook-repo/pull/500",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "dev" },
      },
      repository: { name: "webhook-repo", owner: { login: "owner" } },
    });
    await new Promise((r) => setTimeout(r, 1000));

    const status = await githubWebhook(request, "pull_request", {
      action: "closed",
      pull_request: {
        number: 500,
        title: "Close test",
        html_url: "https://github.com/owner/webhook-repo/pull/500",
        state: "closed",
        merged: false,
        labels: [{ name: "needs-review" }],
        user: { login: "dev" },
      },
      repository: { name: "webhook-repo", owner: { login: "owner" } },
    });
    expect(status).toBe(200);
  });

  test("26-2. PR merged", async ({ request }) => {
    await githubWebhook(request, "pull_request", {
      action: "labeled",
      label: { name: "needs-review" },
      pull_request: {
        number: 501,
        title: "Merge test",
        html_url: "https://github.com/owner/webhook-repo/pull/501",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "dev" },
      },
      repository: { name: "webhook-repo", owner: { login: "owner" } },
    });
    await new Promise((r) => setTimeout(r, 1000));

    const status = await githubWebhook(request, "pull_request", {
      action: "closed",
      pull_request: {
        number: 501,
        title: "Merge test",
        html_url: "https://github.com/owner/webhook-repo/pull/501",
        state: "closed",
        merged: true,
        labels: [{ name: "needs-review" }],
        user: { login: "dev" },
      },
      repository: { name: "webhook-repo", owner: { login: "owner" } },
    });
    expect(status).toBe(200);
  });
});

// ── 27. GitHub Webhook - review_requested ────────────────────────

test.describe("27. GitHub Webhook - review_requested", () => {
  test("27-1. re-review request", async ({ request }) => {
    const status = await githubWebhook(request, "pull_request", {
      action: "review_requested",
      requested_reviewer: { login: "reviewer1" },
      sender: { login: "prauthor" },
      pull_request: {
        number: 200,
        title: "Webhook test PR",
        html_url: "https://github.com/owner/webhook-repo/pull/200",
        state: "open",
        labels: [{ name: "needs-review" }],
        user: { login: "prauthor" },
      },
      repository: { name: "webhook-repo", owner: { login: "owner" } },
    });
    expect(status).toBe(200);
  });
});

// ── 28. Slack Events - url_verification ──────────────────────────

test.describe("28. Slack Events", () => {
  test("28-1. url_verification challenge", async ({ request }) => {
    const body = JSON.stringify({
      type: "url_verification",
      challenge: "test_challenge_token",
    });
    const { ts, sig } = slackSignature(body);
    const res = await request.post("/slack/events", {
      headers: {
        "Content-Type": "application/json",
        "X-Slack-Request-Timestamp": ts,
        "X-Slack-Signature": sig,
      },
      data: body,
    });
    expect(res.status()).toBe(200);
    const json = await res.json();
    expect(json.challenge).toBe("test_challenge_token");
  });
});

// ── 29. Multiple labels per channel ──────────────────────────────

test.describe("29. Multiple labels per channel", () => {
  test("29-1. configure multiple labels independently", async ({
    request,
  }) => {
    const ch = "C_MULTI";
    await slackCmd(request, "bug set-mention <@UBUG>", ch);
    await slackCmd(request, "feature set-mention <@UFEAT>", ch);

    const res = await slackCmd(request, "show", ch);
    expect(res).toContain("bug");
    expect(res).toContain("feature");
  });

  test("29-2. different language per label", async ({ request }) => {
    const ch = "C_MULTI";
    await slackCmd(request, "bug set-language en", ch);
    // bug label should respond in English
    const bugShow = await slackCmd(request, "bug show", ch);
    expect(bugShow).toContain("Review notification settings");
    // feature label should respond in Japanese (default)
    const featShow = await slackCmd(request, "feature show", ch);
    expect(featShow).toContain("レビュー通知設定");
  });
});

// ── 30. Edge cases ───────────────────────────────────────────────

test.describe("30. Edge cases", () => {
  test("30-1. empty command shows help", async ({ request }) => {
    const res = await slackCmd(request, "");
    expect(res).toContain("Bot設定コマンド");
  });

  test("30-2. quoted label name with spaces", async ({ request }) => {
    const res = await slackCmd(
      request,
      '"needs review" set-mention <@USPACE>'
    );
    expect(res).toContain("needs review");
  });

  test("30-3. set-required-approvals 0 rejected", async ({ request }) => {
    const res = await slackCmd(request, "set-required-approvals 0");
    expect(res).toContain("1〜10");
  });

  test("30-4. remove-user-mapping missing params", async ({ request }) => {
    const res = await slackCmd(request, "remove-user-mapping");
    expect(res).toContain("使用方法");
  });

  test("30-5. unset-away missing params", async ({ request }) => {
    const res = await slackCmd(request, "unset-away");
    expect(res).toContain("休暇を解除するユーザーを指定");
  });
});
