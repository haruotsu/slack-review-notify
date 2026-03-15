#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCREENSHOTS="$SCRIPT_DIR/../screenshots"
OUT="$SCRIPT_DIR"
TMP="$SCRIPT_DIR/tmp"
FONT=".Hiragino-Kaku-Gothic-Interface-W6"
WIDTH=800

mkdir -p "$TMP"
rm -f "$TMP"/*

# Add caption banner on top of image, resize to target width
add_caption() {
  local input="$1" output="$2" caption="$3" bg_color="${4:-#1a1a2e}" text_color="${5:-#ffffff}"
  # Resize source image first
  magick "$input" -resize "${WIDTH}x" "$TMP/_resized.png"
  # Create banner matching resized width
  local h=44
  magick -size "${WIDTH}x${h}" "xc:${bg_color}" \
    -font "$FONT" -pointsize 20 -fill "$text_color" \
    -gravity center -annotate +0+0 "$caption" \
    "$TMP/_banner.png"
  # Stack vertically
  magick "$TMP/_banner.png" "$TMP/_resized.png" -append "$output"
}

# Build GIF from frames with specified delay (in 1/100s)
build_gif() {
  local output="$1" delay="$2"
  shift 2
  # Use coalesce-safe approach: no -layers Optimize
  magick -delay "$delay" -loop 0 -dispose Background "$@" -colors 128 "$output"
}

echo "=== GIF 1: JA Review Flow ==="
add_caption "$SCREENSHOTS/01-ja-pr-labeled-with-reviewer.png" "$TMP/ja-01.png" \
  "Step 1: PR通知 + レビュワー自動アサイン" "#2d1b69" "#e0d4ff"
add_caption "$SCREENSHOTS/04-ja-review-approved-partial.png" "$TMP/ja-02.png" \
  "Step 2: レビュー承認 (1/2 approved)" "#1b3d69" "#d4e8ff"
add_caption "$SCREENSHOTS/05-ja-review-fully-approved.png" "$TMP/ja-03.png" \
  "Step 3: 全員承認完了! (2/2 approved)" "#1b6935" "#d4ffd8"
add_caption "$SCREENSHOTS/09-ja-pr-merged.png" "$TMP/ja-04.png" \
  "Step 4: PRマージ完了!" "#695b1b" "#fff8d4"

build_gif "$OUT/ja-review-flow.gif" 300 \
  "$TMP/ja-01.png" "$TMP/ja-02.png" "$TMP/ja-03.png" "$TMP/ja-04.png"
echo "  -> ja-review-flow.gif"

echo "=== GIF 2: EN Review Flow ==="
add_caption "$SCREENSHOTS/02-en-pr-labeled-with-reviewer.png" "$TMP/en-01.png" \
  "Step 1: PR Notification + Auto-Assign Reviewer" "#2d1b69" "#e0d4ff"
add_caption "$SCREENSHOTS/06-en-changes-requested.png" "$TMP/en-02.png" \
  "Step 2: Changes Requested" "#691b1b" "#ffd4d4"
add_caption "$SCREENSHOTS/07-en-commented.png" "$TMP/en-03.png" \
  "Step 3: Review Comment" "#1b3d69" "#d4e8ff"
add_caption "$SCREENSHOTS/08-en-re-review.png" "$TMP/en-04.png" \
  "Step 4: Re-review Requested" "#69531b" "#fff0d4"
add_caption "$SCREENSHOTS/10-en-pr-closed.png" "$TMP/en-05.png" \
  "Step 5: PR Closed" "#3d3d3d" "#e0e0e0"

build_gif "$OUT/en-review-flow.gif" 300 \
  "$TMP/en-01.png" "$TMP/en-02.png" "$TMP/en-03.png" "$TMP/en-04.png" "$TMP/en-05.png"
echo "  -> en-review-flow.gif"

echo "=== GIF 3: JA Features ==="
add_caption "$SCREENSHOTS/03-ja-off-hours.png" "$TMP/ft-ja-01.png" \
  "営業時間外通知: 翌営業日の朝にメンション" "#1b3d69" "#d4e8ff"
add_caption "$SCREENSHOTS/12-ja-reminder.png" "$TMP/ft-ja-02.png" \
  "リマインダー: 定期的にレビューを催促" "#69531b" "#fff0d4"
add_caption "$SCREENSHOTS/11-ja-label-removed.png" "$TMP/ft-ja-03.png" \
  "ラベル削除: 自動でタスク完了" "#1b6935" "#d4ffd8"
add_caption "$SCREENSHOTS/13-overview.png" "$TMP/ft-ja-04.png" \
  "全チャンネル俯瞰: JA / EN 両対応" "#2d1b69" "#e0d4ff"

build_gif "$OUT/features-overview.gif" 300 \
  "$TMP/ft-ja-01.png" "$TMP/ft-ja-02.png" "$TMP/ft-ja-03.png" "$TMP/ft-ja-04.png"
echo "  -> features-overview.gif"

echo "=== GIF 4: EN Features ==="
add_caption "$SCREENSHOTS/14-en-off-hours.png" "$TMP/ft-en-01.png" \
  "Off-Hours: Mentions sent next business day morning" "#1b3d69" "#d4e8ff"
add_caption "$SCREENSHOTS/15-en-label-removed.png" "$TMP/ft-en-02.png" \
  "Label Removed: Auto-complete task" "#1b6935" "#d4ffd8"
# 16-en-reminder.png is optional (requires weekday to capture)
if [ -f "$SCREENSHOTS/16-en-reminder.png" ]; then
  EN_REMINDER_VALID=$(magick identify -format "%w" "$SCREENSHOTS/16-en-reminder.png" 2>/dev/null || echo "0")
  # Check if the screenshot actually shows a reminder thread (file exists and is recent)
fi
add_caption "$SCREENSHOTS/13-overview.png" "$TMP/ft-en-03.png" \
  "All Channels Overview: JA / EN Support" "#2d1b69" "#e0d4ff"

build_gif "$OUT/en-features-overview.gif" 300 \
  "$TMP/ft-en-01.png" "$TMP/ft-en-02.png" "$TMP/ft-en-03.png"
echo "  -> en-features-overview.gif"

# Cleanup
rm -rf "$TMP"
echo ""
echo "Done! Generated GIFs:"
ls -lh "$OUT"/*.gif
