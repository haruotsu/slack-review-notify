package services

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slack-review-notify/models"
	"strings"

	"github.com/google/go-github/v71/github"
	"golang.org/x/oauth2"
)

// GitHubクライアントを作成する関数（PRのURLからベースURLを自動検出）
func NewGitHubClientForPR(prURL string) *github.Client {
	// GITHUB_WEBHOOK_SECRETを認証トークンとして使用
	token := os.Getenv("GITHUB_WEBHOOK_SECRET")
	
	ctx := context.Background()
	var httpClient *http.Client
	
	if token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		httpClient = oauth2.NewClient(ctx, ts)
	} else {
		httpClient = http.DefaultClient
	}
	
	client := github.NewClient(httpClient)
	
	// PRのURLからGitHubのベースURLを検出
	if prURL != "" {
		parsedURL, err := url.Parse(prURL)
		if err == nil && parsedURL.Host != "github.com" {
			// GitHub Enterpriseの場合
			baseURL := fmt.Sprintf("https://%s/api/v3/", parsedURL.Host)
			uploadURL := fmt.Sprintf("https://%s/api/uploads/", parsedURL.Host)
			
			enterpriseClient, err := client.WithEnterpriseURLs(baseURL, uploadURL)
			if err != nil {
				log.Printf("failed to set enterprise URLs: %v", err)
			} else {
				log.Printf("using GitHub Enterprise URL: %s", baseURL)
				client = enterpriseClient
			}
		}
	}
	
	return client
}

// PRのURLからオーナー、リポジトリ名、PR番号を抽出する関数
func ParseRepoAndPRNumber(prURL string) (owner string, repo string, prNumber int, err error) {
	// GitHub EnterpriseとGitHub.comの両方に対応するパターン
	re := regexp.MustCompile(`https?://[^/]+/([^/]+)/([^/]+)/pull/(\d+)`)
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
	
	log.Printf("removing label %s from PR %s/%s#%d", labelName, owner, repo, prNumber)
	
	// PRのURLに基づいてGitHubクライアントを作成
	client := NewGitHubClientForPR(task.PRURL)
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
