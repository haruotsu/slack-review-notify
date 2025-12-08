#!/bin/bash

# 統合テスト実行スクリプト
#
# 環境変数のチェック、テスト用DBのセットアップ、統合テストの実行、
# テスト後のクリーンアップ、実行結果のサマリー表示を行います。

set -e  # エラー時に即座に終了

# カラー定義
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ログ出力関数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# セクション区切り
print_separator() {
    echo ""
    echo "=================================================================="
    echo "$1"
    echo "=================================================================="
    echo ""
}

# クリーンアップ処理
cleanup() {
    local exit_code=$?

    print_separator "クリーンアップ中..."

    # Docker Composeサービスの停止と削除
    if [ -f docker-compose.test.yml ]; then
        log_info "Docker Composeサービスを停止しています..."
        docker-compose -f docker-compose.test.yml down -v 2>/dev/null || true
    fi

    # テスト用DBファイルの削除
    if [ -f "${TEST_DB_PATH}" ]; then
        log_info "テスト用DBファイルを削除しています: ${TEST_DB_PATH}"
        rm -f "${TEST_DB_PATH}"
    fi

    # 一時ファイルの削除
    rm -f coverage.*.out coverage.*.txt 2>/dev/null || true

    if [ $exit_code -eq 0 ]; then
        log_success "クリーンアップが完了しました"
    else
        log_warning "クリーンアップが完了しました（テスト失敗）"
    fi

    exit $exit_code
}

# エラー時のクリーンアップ設定
trap cleanup EXIT INT TERM

# スクリプトのディレクトリに移動
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}"

print_separator "統合テスト実行開始"
log_info "プロジェクトディレクトリ: ${PROJECT_ROOT}"

# ================================================================
# 1. 環境変数のチェック
# ================================================================
print_separator "1. 環境変数のチェック"

# デフォルト値の設定
export SLACK_BOT_TOKEN="${SLACK_BOT_TOKEN:-xoxb-test-token}"
export SLACK_SIGNING_SECRET="${SLACK_SIGNING_SECRET:-test-signing-secret}"
# テストモードではGITHUB_WEBHOOK_SECRETを空にして署名検証をスキップ
export GITHUB_WEBHOOK_SECRET=""
export TEST_DB_PATH="${DB_PATH:-test_integration.db}"
export TEST_MODE="${TEST_MODE:-true}"

# 環境変数の表示（トークンは一部マスク）
log_info "SLACK_BOT_TOKEN: ${SLACK_BOT_TOKEN:0:10}..."
log_info "SLACK_SIGNING_SECRET: ${SLACK_SIGNING_SECRET:0:10}..."
log_info "GITHUB_WEBHOOK_SECRET: (empty - signature validation disabled for tests)"
log_info "TEST_DB_PATH: ${TEST_DB_PATH}"
log_info "TEST_MODE: ${TEST_MODE}"

# 必須コマンドの確認
command -v docker >/dev/null 2>&1 || { log_error "dockerコマンドが見つかりません"; exit 1; }
command -v docker-compose >/dev/null 2>&1 || { log_error "docker-composeコマンドが見つかりません"; exit 1; }
command -v go >/dev/null 2>&1 || { log_error "goコマンドが見つかりません"; exit 1; }

log_success "環境変数とコマンドのチェックが完了しました"

# ================================================================
# 2. テスト用DBのセットアップ
# ================================================================
print_separator "2. テスト用DBのセットアップ"

# 既存のテスト用DBファイルを削除
if [ -f "${TEST_DB_PATH}" ]; then
    log_warning "既存のテスト用DBファイルを削除します: ${TEST_DB_PATH}"
    rm -f "${TEST_DB_PATH}"
fi

# テスト用ディレクトリの作成
log_info "テスト用ディレクトリを作成しています..."
mkdir -p test/mock
mkdir -p test/integration

# モックサーバー初期化ファイルの作成（存在しない場合）
if [ ! -f test/mock/initializer.json ]; then
    log_info "モックサーバー初期化ファイルを作成しています..."
    echo '{}' > test/mock/initializer.json
fi

log_success "テスト用DBのセットアップが完了しました"

# ================================================================
# 3. Docker Composeサービスの起動
# ================================================================
print_separator "3. Docker Composeサービスの起動"

if [ -f docker-compose.test.yml ]; then
    log_info "Docker Composeサービスを起動しています..."
    docker-compose -f docker-compose.test.yml up -d

    log_info "サービスが起動するまで待機中..."
    sleep 10

    # アプリケーションのヘルスチェック
    log_info "アプリケーションのヘルスチェックを実行しています..."
    MAX_ATTEMPTS=30
    ATTEMPT=0
    HEALTH_CHECK_URL="http://localhost:8080/health"

    until curl -f "${HEALTH_CHECK_URL}" >/dev/null 2>&1 || [ $ATTEMPT -eq $MAX_ATTEMPTS ]; do
        ATTEMPT=$((ATTEMPT + 1))
        log_info "アプリケーションの起動を待機中... (試行 ${ATTEMPT}/${MAX_ATTEMPTS})"
        sleep 2
    done

    if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
        log_error "アプリケーションが正常に起動しませんでした"
        log_info "Docker Composeログを表示します:"
        docker-compose -f docker-compose.test.yml logs
        exit 1
    fi

    log_success "アプリケーションが正常に起動しました"
else
    log_warning "docker-compose.test.ymlが見つかりません。Docker Composeをスキップします"
fi

# ================================================================
# 4. 統合テストの実行
# ================================================================
print_separator "4. 統合テストの実行"

TEST_FAILED=0

# ユニットテストの実行
log_info "ユニットテストを実行しています..."
if go test -v -race -coverprofile=coverage.unit.out ./... ; then
    log_success "ユニットテストが成功しました"
else
    log_error "ユニットテストが失敗しました"
    TEST_FAILED=1
fi

# 統合テストの実行（test/integrationディレクトリがある場合）
if [ -d "test/integration" ] && [ -n "$(ls -A test/integration 2>/dev/null)" ]; then
    log_info "統合テストを実行しています..."
    export DB_PATH="${TEST_DB_PATH}"

    if go test -v -race -tags=integration -coverprofile=coverage.integration.out ./test/integration/... ; then
        log_success "統合テストが成功しました"
    else
        log_error "統合テストが失敗しました"
        TEST_FAILED=1
    fi
else
    log_warning "統合テストファイルが見つかりません（test/integrationディレクトリ）"
fi

# servicesパッケージの統合テスト実行
if ls services/*_integration_test.go >/dev/null 2>&1; then
    log_info "servicesパッケージの統合テストを実行しています..."
    export DB_PATH="${TEST_DB_PATH}"

    if go test -v -race ./services/... -run Integration ; then
        log_success "servicesパッケージの統合テストが成功しました"
    else
        log_error "servicesパッケージの統合テストが失敗しました"
        TEST_FAILED=1
    fi
else
    log_warning "servicesパッケージの統合テストファイルが見つかりません"
fi

# ================================================================
# 5. カバレッジレポートの生成
# ================================================================
print_separator "5. カバレッジレポートの生成"

if [ -f coverage.unit.out ]; then
    log_info "ユニットテストのカバレッジレポートを生成しています..."
    go tool cover -func=coverage.unit.out -o=coverage.unit.txt

    # 総合カバレッジの表示
    UNIT_COVERAGE=$(tail -n 1 coverage.unit.txt | awk '{print $3}')
    log_info "ユニットテストカバレッジ: ${UNIT_COVERAGE}"
fi

if [ -f coverage.integration.out ]; then
    log_info "統合テストのカバレッジレポートを生成しています..."
    go tool cover -func=coverage.integration.out -o=coverage.integration.txt

    # 総合カバレッジの表示
    INTEGRATION_COVERAGE=$(tail -n 1 coverage.integration.txt | awk '{print $3}')
    log_info "統合テストカバレッジ: ${INTEGRATION_COVERAGE}"
fi

# ================================================================
# 6. 実行結果のサマリー表示
# ================================================================
print_separator "6. 実行結果のサマリー"

echo "テスト実行サマリー:"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ -f coverage.unit.out ]; then
    echo "  ユニットテスト:     ${GREEN}成功${NC}"
    echo "  カバレッジ:         ${UNIT_COVERAGE:-N/A}"
else
    echo "  ユニットテスト:     ${RED}失敗 または スキップ${NC}"
fi

echo ""

if [ -f coverage.integration.out ]; then
    echo "  統合テスト:         ${GREEN}成功${NC}"
    echo "  カバレッジ:         ${INTEGRATION_COVERAGE:-N/A}"
else
    echo "  統合テスト:         ${YELLOW}スキップ${NC}"
fi

echo ""

if [ -f docker-compose.test.yml ]; then
    echo "  Docker Compose:     ${GREEN}起動完了${NC}"
else
    echo "  Docker Compose:     ${YELLOW}スキップ${NC}"
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Docker Composeログの表示（失敗時）
if [ $TEST_FAILED -eq 1 ] && [ -f docker-compose.test.yml ]; then
    print_separator "Docker Composeログ（テスト失敗時）"

    log_info "=== アプリケーションログ ==="
    docker-compose -f docker-compose.test.yml logs app

    log_info "=== モックサーバーログ ==="
    docker-compose -f docker-compose.test.yml logs mock-server
fi

# 最終結果
echo ""
if [ $TEST_FAILED -eq 0 ]; then
    log_success "すべてのテストが成功しました！"
    echo ""
    exit 0
else
    log_error "一部のテストが失敗しました"
    echo ""
    exit 1
fi
