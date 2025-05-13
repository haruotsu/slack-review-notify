package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"slack-review-notify/models"
	"strings"

	"github.com/google/go-github/v71/github"
	"golang.org/x/oauth2"
)

// GitHubクライアントを作成する関数
func NewGitHubClient() *github.Client {
	// GITHUB_WEBHOOK_SECRETを認証トークンとして使用
	token := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if token == "" {
		log.Println("GITHUB_WEBHOOK_SECRET is not set")
		return github.NewClient(nil) // 認証なしのクライアント
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

// PRのURLからオーナー、リポジトリ名、PR番号を抽出する関数
func ParseRepoAndPRNumber(prURL string) (owner string, repo string, prNumber int, err error) {
	// https://github.com/owner/repo/pull/123 の形式を想定
	re := regexp.MustCompile(`https://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
	matches := re.FindStringSubmatch(prURL)
	
	if len(matches) != 4 {
		return "", "", 0, fmt.Errorf("invalid PR URL format: %s", prURL)
	}
	
	owner = matches[1]
	repo = matches[2]
	
	var prNum int
	_, err = fmt.Sscanf(matches[3], "%d", &prNum)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to parse PR number: %v", err)
	}
	
	return owner, repo, prNum, nil
}

// PRからラベルを削除する関数
func RemoveLabelFromPR(task models.ReviewTask, labelName string) error {
	if task.PRURL == "" {
		return fmt.Errorf("PR URL is empty")
	}
	
	// PRのURLからオーナー、リポジトリ名、PR番号を抽出
	owner, repo, prNumber, err := ParseRepoAndPRNumber(task.PRURL)
	if err != nil {
		return fmt.Errorf("failed to parse PR URL: %v", err)
	}
	
	// GitHubクライアントを作成
	client := NewGitHubClient()
	ctx := context.Background()
	
	// ラベルを削除
	_, err = client.Issues.RemoveLabelForIssue(ctx, owner, repo, prNumber, labelName)
	if err != nil {
		// ラベルが既に削除されている場合は404エラーになるが、それは無視する
		if strings.Contains(err.Error(), "404") {
			log.Printf("label %s already removed from PR %s", labelName, task.PRURL)
			return nil
		}
		return fmt.Errorf("failed to remove label: %v", err)
	}
	
	log.Printf("label %s removed from PR %s", labelName, task.PRURL)
	return nil
}
