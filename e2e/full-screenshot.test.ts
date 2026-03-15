import { test, Page } from "@playwright/test";
import { createHmac } from "crypto";

const SLACK_SECRET = "test-signing-secret";
const GITHUB_SECRET = "test-webhook-secret";
const BASE = "http://localhost:8080";
const SLACKHOG = "http://localhost:4112";
const JA = "C12345";
const EN = "C67890";
const JA_OFF = "C1234567890";
const EN_OFF = "C9876543210";

function slackSig(body: string) {
  const ts = Math.floor(Date.now() / 1000).toString();
  const sig = "v0=" + createHmac("sha256", SLACK_SECRET).update(`v0:${ts}:${body}`).digest("hex");
  return { ts, sig };
}

async function cmd(text: string, channel = JA) {
  const body = `command=%2Fslack-review-notify&text=${encodeURIComponent(text)}&channel_id=${channel}&user_id=U12345`;
  const { ts, sig } = slackSig(body);
  await fetch(`${BASE}/slack/command`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded", "X-Slack-Request-Timestamp": ts, "X-Slack-Signature": sig },
    body,
  });
}

async function wh(event: string, payload: object) {
  const body = JSON.stringify(payload);
  const sig = "sha256=" + createHmac("sha256", GITHUB_SECRET).update(body).digest("hex");
  await fetch(`${BASE}/webhook`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-GitHub-Event": event, "X-Hub-Signature-256": sig },
    body,
  });
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function selectCh(page: Page, ch: string) {
  const items = page.locator("#channel-list .channel-item");
  for (let i = 0; i < await items.count(); i++) {
    if ((await items.nth(i).textContent())?.includes(ch)) {
      await items.nth(i).click();
      await page.waitForTimeout(500);
      return;
    }
  }
}

async function openThread(page: Page, index = 0) {
  const badges = page.locator(".reply-badge");
  const cnt = await badges.count();
  if (index < cnt) {
    await badges.nth(index).click({ force: true });
    await page.waitForTimeout(800);
  }
}

async function closeThread(page: Page) {
  const closeBtn = page.locator("#thread-panel .thread-close, #thread-panel button");
  if (await closeBtn.count() > 0) {
    await closeBtn.first().click({ force: true });
    await page.waitForTimeout(300);
  }
}

async function shot(page: Page, name: string) {
  await page.screenshot({ path: `../docs/screenshots/${name}.png`, fullPage: true });
}

async function gotoSlackHog(page: Page) {
  await page.goto(SLACKHOG);
  await page.waitForTimeout(1200);
}

test.use({ viewport: { width: 1280, height: 900 } });

test("full feature screenshots with threads", async ({ page }) => {
  // ===================== SETUP =====================

  // JA channel: business hours
  await cmd("set-mention <@UTEAM>", JA);
  await cmd("add-repo org/backend", JA);
  await cmd("add-reviewer <@UREV_TARO>,<@UREV_HANAKO>,<@UREV_JIRO>", JA);
  await cmd("set-business-hours-start 00:00", JA);
  await cmd("set-business-hours-end 23:59", JA);
  await cmd("set-timezone UTC", JA);
  await cmd("set-required-approvals 2", JA);
  await cmd("map-user alice <@UALICE>", JA);
  await cmd("map-user bob <@UBOB>", JA);

  // EN channel: business hours
  await cmd("set-mention <@UTEAMEN>", EN);
  await cmd("add-repo org/frontend", EN);
  await cmd("add-reviewer <@UREV_JOHN>,<@UREV_JANE>,<@UREV_DOE>", EN);
  await cmd("set-language en", EN);
  await cmd("set-business-hours-start 00:00", EN);
  await cmd("set-business-hours-end 23:59", EN);
  await cmd("set-timezone UTC", EN);
  await cmd("set-required-approvals 2", EN);

  // JA OFF channel: very narrow business hours to guarantee off-hours
  await cmd("set-mention <@UTEAMOFF>", JA_OFF);
  await cmd("add-repo org/api", JA_OFF);
  await cmd("add-reviewer <@UREV_OFF1>,<@UREV_OFF2>", JA_OFF);
  await cmd("set-business-hours-start 03:00", JA_OFF);
  await cmd("set-business-hours-end 03:01", JA_OFF);
  await cmd("set-timezone UTC", JA_OFF);

  // EN channel: add off-hours label config (separate label with narrow business hours)
  await cmd("en-off-hours set-mention <@UTEAMENOFF>", EN);
  await cmd("en-off-hours add-repo org/infra", EN);
  await cmd("en-off-hours add-reviewer <@UREV_ENOFF1>,<@UREV_ENOFF2>", EN);
  await cmd("en-off-hours set-language en", EN);
  await cmd("en-off-hours set-business-hours-start 03:00", EN);
  await cmd("en-off-hours set-business-hours-end 03:01", EN);
  await cmd("en-off-hours set-timezone UTC", EN);

  // ===================== SCENARIO 1: JA PR labeled =====================
  await wh("pull_request", {
    action: "labeled", label: { name: "needs-review" },
    pull_request: { number: 1, title: "feat: ユーザー認証APIの実装", html_url: "https://github.com/org/backend/pull/1", state: "open", labels: [{ name: "needs-review" }], user: { login: "alice" } },
    repository: { name: "backend", owner: { login: "org" } },
  });
  await sleep(2000);

  await gotoSlackHog(page);
  await selectCh(page, JA);
  await openThread(page, 0);
  await shot(page, "01-ja-pr-labeled-with-reviewer");

  // ===================== SCENARIO 2: EN PR labeled =====================
  await wh("pull_request", {
    action: "labeled", label: { name: "needs-review" },
    pull_request: { number: 10, title: "fix: resolve auth token expiry", html_url: "https://github.com/org/frontend/pull/10", state: "open", labels: [{ name: "needs-review" }], user: { login: "bob" } },
    repository: { name: "frontend", owner: { login: "org" } },
  });
  await sleep(2000);

  await gotoSlackHog(page);
  await selectCh(page, EN);
  await openThread(page, 0);
  await shot(page, "02-en-pr-labeled-with-reviewer");

  // ===================== SCENARIO 3: JA OFF hours =====================
  await wh("pull_request", {
    action: "labeled", label: { name: "needs-review" },
    pull_request: { number: 20, title: "chore: CIパイプラインの改善", html_url: "https://github.com/org/api/pull/20", state: "open", labels: [{ name: "needs-review" }], user: { login: "alice" } },
    repository: { name: "api", owner: { login: "org" } },
  });
  await sleep(2000);

  await gotoSlackHog(page);
  await selectCh(page, JA_OFF);
  await shot(page, "03-ja-off-hours");

  // ===================== SCENARIO 4: JA review approved (1/2) =====================
  await wh("pull_request_review", {
    action: "submitted", review: { state: "approved", user: { login: "bob" } },
    pull_request: { number: 1, title: "feat: ユーザー認証APIの実装", html_url: "https://github.com/org/backend/pull/1", state: "open", labels: [{ name: "needs-review" }], user: { login: "alice" } },
    repository: { name: "backend", owner: { login: "org" } },
  });
  await sleep(500);

  await gotoSlackHog(page);
  await selectCh(page, JA);
  await openThread(page, 0);
  await shot(page, "04-ja-review-approved-partial");

  // ===================== SCENARIO 5: JA review fully approved (2/2) =====================
  await wh("pull_request_review", {
    action: "submitted", review: { state: "approved", user: { login: "charlie" } },
    pull_request: { number: 1, title: "feat: ユーザー認証APIの実装", html_url: "https://github.com/org/backend/pull/1", state: "open", labels: [{ name: "needs-review" }], user: { login: "alice" } },
    repository: { name: "backend", owner: { login: "org" } },
  });
  await sleep(500);

  await gotoSlackHog(page);
  await selectCh(page, JA);
  await openThread(page, 0);
  await shot(page, "05-ja-review-fully-approved");

  // ===================== SCENARIO 6: EN changes_requested =====================
  await wh("pull_request_review", {
    action: "submitted", review: { state: "changes_requested", user: { login: "reviewer_x" } },
    pull_request: { number: 10, title: "fix: resolve auth token expiry", html_url: "https://github.com/org/frontend/pull/10", state: "open", labels: [{ name: "needs-review" }], user: { login: "bob" } },
    repository: { name: "frontend", owner: { login: "org" } },
  });
  await sleep(500);

  await gotoSlackHog(page);
  await selectCh(page, EN);
  await openThread(page, 0);
  await shot(page, "06-en-changes-requested");

  // ===================== SCENARIO 7: EN commented =====================
  await wh("pull_request_review", {
    action: "submitted", review: { state: "commented", user: { login: "reviewer_y" } },
    pull_request: { number: 10, title: "fix: resolve auth token expiry", html_url: "https://github.com/org/frontend/pull/10", state: "open", labels: [{ name: "needs-review" }], user: { login: "bob" } },
    repository: { name: "frontend", owner: { login: "org" } },
  });
  await sleep(500);

  await gotoSlackHog(page);
  await selectCh(page, EN);
  await openThread(page, 0);
  await shot(page, "07-en-commented");

  // ===================== SCENARIO 8: EN re-review =====================
  await wh("pull_request", {
    action: "review_requested", requested_reviewer: { login: "reviewer_x" }, sender: { login: "bob" },
    pull_request: { number: 10, title: "fix: resolve auth token expiry", html_url: "https://github.com/org/frontend/pull/10", state: "open", labels: [{ name: "needs-review" }], user: { login: "bob" } },
    repository: { name: "frontend", owner: { login: "org" } },
  });
  await sleep(500);

  await gotoSlackHog(page);
  await selectCh(page, EN);
  await openThread(page, 0);
  await shot(page, "08-en-re-review");

  // ===================== SCENARIO 9: JA PR merged =====================
  await wh("pull_request", {
    action: "labeled", label: { name: "needs-review" },
    pull_request: { number: 2, title: "fix: パスワードリセットのバグ修正", html_url: "https://github.com/org/backend/pull/2", state: "open", labels: [{ name: "needs-review" }], user: { login: "alice" } },
    repository: { name: "backend", owner: { login: "org" } },
  });
  await sleep(2000);
  await wh("pull_request", {
    action: "closed",
    pull_request: { number: 2, title: "fix: パスワードリセットのバグ修正", html_url: "https://github.com/org/backend/pull/2", state: "closed", merged: true, labels: [{ name: "needs-review" }], user: { login: "alice" } },
    repository: { name: "backend", owner: { login: "org" } },
  });
  await sleep(500);

  await gotoSlackHog(page);
  await selectCh(page, JA);
  await openThread(page, 1); // second message
  await shot(page, "09-ja-pr-merged");

  // ===================== SCENARIO 10: EN PR closed =====================
  await wh("pull_request", {
    action: "labeled", label: { name: "needs-review" },
    pull_request: { number: 11, title: "feat: add dark mode", html_url: "https://github.com/org/frontend/pull/11", state: "open", labels: [{ name: "needs-review" }], user: { login: "bob" } },
    repository: { name: "frontend", owner: { login: "org" } },
  });
  await sleep(2000);
  await wh("pull_request", {
    action: "closed",
    pull_request: { number: 11, title: "feat: add dark mode", html_url: "https://github.com/org/frontend/pull/11", state: "closed", merged: false, labels: [{ name: "needs-review" }], user: { login: "bob" } },
    repository: { name: "frontend", owner: { login: "org" } },
  });
  await sleep(500);

  await gotoSlackHog(page);
  await selectCh(page, EN);
  await openThread(page, 1); // second message
  await shot(page, "10-en-pr-closed");

  // ===================== SCENARIO 11a: EN OFF hours =====================
  await wh("pull_request", {
    action: "labeled", label: { name: "en-off-hours" },
    pull_request: { number: 30, title: "chore: update CI pipeline", html_url: "https://github.com/org/infra/pull/30", state: "open", labels: [{ name: "en-off-hours" }], user: { login: "bob" } },
    repository: { name: "infra", owner: { login: "org" } },
  });
  await sleep(2000);

  await gotoSlackHog(page);
  await selectCh(page, EN);
  await shot(page, "14-en-off-hours");

  // ===================== SCENARIO 11: JA label removed =====================
  await wh("pull_request", {
    action: "labeled", label: { name: "needs-review" },
    pull_request: { number: 3, title: "refactor: DBクエリの最適化", html_url: "https://github.com/org/backend/pull/3", state: "open", labels: [{ name: "needs-review" }], user: { login: "alice" } },
    repository: { name: "backend", owner: { login: "org" } },
  });
  await sleep(2000);
  await wh("pull_request", {
    action: "unlabeled", label: { name: "needs-review" },
    pull_request: { number: 3, title: "refactor: DBクエリの最適化", html_url: "https://github.com/org/backend/pull/3", state: "open", labels: [], user: { login: "alice" } },
    repository: { name: "backend", owner: { login: "org" } },
  });
  await sleep(500);

  await gotoSlackHog(page);
  await selectCh(page, JA);
  // The label-removed message updates the original message and posts thread
  // Find the updated message (3rd message in JA channel)
  await openThread(page, 2);
  await shot(page, "11-ja-label-removed");

  // ===================== SCENARIO 11c: EN label removed =====================
  await wh("pull_request", {
    action: "labeled", label: { name: "needs-review" },
    pull_request: { number: 12, title: "refactor: extract API client", html_url: "https://github.com/org/frontend/pull/12", state: "open", labels: [{ name: "needs-review" }], user: { login: "bob" } },
    repository: { name: "frontend", owner: { login: "org" } },
  });
  await sleep(2000);
  await wh("pull_request", {
    action: "unlabeled", label: { name: "needs-review" },
    pull_request: { number: 12, title: "refactor: extract API client", html_url: "https://github.com/org/frontend/pull/12", state: "open", labels: [], user: { login: "bob" } },
    repository: { name: "frontend", owner: { login: "org" } },
  });
  await sleep(500);

  await gotoSlackHog(page);
  await selectCh(page, EN);
  // Find the label-removed message in EN channel (should be the latest with thread)
  const enBadges = page.locator(".reply-badge");
  const enBadgeCnt = await enBadges.count();
  await openThread(page, enBadgeCnt - 1);
  await shot(page, "15-en-label-removed");

  // ===================== SCENARIO 12: Reminder (JA + EN) =====================
  // Create both JA and EN PRs before waiting, so they share the 130s wait
  await wh("pull_request", {
    action: "labeled", label: { name: "needs-review" },
    pull_request: { number: 4, title: "docs: READMEの更新", html_url: "https://github.com/org/backend/pull/4", state: "open", labels: [{ name: "needs-review" }], user: { login: "alice" } },
    repository: { name: "backend", owner: { login: "org" } },
  });
  await sleep(500);
  await wh("pull_request", {
    action: "labeled", label: { name: "needs-review" },
    pull_request: { number: 99, title: "docs: update API reference", html_url: "https://github.com/org/frontend/pull/99", state: "open", labels: [{ name: "needs-review" }], user: { login: "bob" } },
    repository: { name: "frontend", owner: { login: "org" } },
  });
  await sleep(2000);
  console.log("Waiting 130s for reminder...");
  await sleep(130000);

  // JA reminder screenshot
  await gotoSlackHog(page);
  await selectCh(page, JA);
  const badges = page.locator(".reply-badge");
  const cnt = await badges.count();
  await openThread(page, cnt - 1);
  await shot(page, "12-ja-reminder");

  // EN reminder screenshot
  await gotoSlackHog(page);
  await selectCh(page, EN);
  const enReminderBadges = page.locator(".reply-badge");
  const enReminderCnt = await enReminderBadges.count();
  await openThread(page, enReminderCnt - 1);
  await shot(page, "16-en-reminder");

  // ===================== SCENARIO 13: Overview =====================
  await closeThread(page);
  await gotoSlackHog(page);
  await shot(page, "13-overview");
});
