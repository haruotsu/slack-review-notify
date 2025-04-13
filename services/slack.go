package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slack-review-notify/models"
)

type SlackMessage struct {
    Channel string       `json:"channel"`
    Blocks  []Block      `json:"blocks"`
}

type Block struct {
    Type string          `json:"type"`
    Text *TextObject     `json:"text,omitempty"`
    Elements []Button    `json:"elements,omitempty"`
}

type TextObject struct {
    Type string `json:"type"`
    Text string `json:"text"`
}

type Button struct {
    Type     string `json:"type"`
    Text     TextObject `json:"text"`
    ActionID string `json:"action_id"`
    Style    string `json:"style,omitempty"`
}

type SlackPostResponse struct {
    OK      bool   `json:"ok"`
    Channel string `json:"channel"`
    Ts      string `json:"ts"`
    Error   string `json:"error,omitempty"`
}

func SendSlackMessage(prURL, title, channel string) (string, string, error) {
    blocks := []Block{
        {
            Type: "section",
            Text: &TextObject{
                Type: "mrkdwn",
                Text: fmt.Sprintf("<@U08MRE10GS2> *🔍 新しいレビュー対象のPRがあります！*\n\n*タイトル*: %s\n*リンク*: <%s>", title, prURL),
            },
        },
        {
            Type: "actions",
            Elements: []Button{
                {
                    Type: "button",
                    Text: TextObject{
                        Type: "plain_text",
                        Text: "レビューします！",
                    },
                    ActionID: "review_take",
                    Style: "primary",
                },
                {
                    Type: "button",
                    Text: TextObject{
                        Type: "plain_text",
                        Text: "今みてる！",
                    },
                    ActionID: "review_watch",
                },
            },
        },
    }

    message := SlackMessage{
        Channel: channel,
        Blocks:  blocks,
    }

    jsonData, _ := json.Marshal(message)
    req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(jsonData))
    if err != nil {
        return "", "", err
    }

    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	// ✅ レスポンスボディを一度全部読む
    bodyBytes, _ := io.ReadAll(resp.Body)
    fmt.Println("🔍 Slackレスポンス:", string(bodyBytes))

    // ✅ ここで読み取ったbodyをJSONパース
    var slackResp SlackPostResponse
    if err := json.Unmarshal(bodyBytes, &slackResp); err != nil {
        return "", "", err
    }

    if !slackResp.OK {
        return "", "", fmt.Errorf("Slack error: %s", slackResp.Error)
    }


	return slackResp.Ts, slackResp.Channel, nil
}

func UpdateSlackMessage(channel, ts string, task models.ReviewTask) error {
    status := "❓未割り当て"
    if task.Status == "pending" && task.Reviewer != "" {
        status = fmt.Sprintf("✅ <@%s> さんがレビュー担当です！", task.Reviewer)
    } else if task.Status == "watching" {
        status = fmt.Sprintf("👀 <@%s> さんが見てるところです", task.Reviewer)
    }

    // まず元のメッセージを更新
    body := map[string]interface{}{
        "channel": channel,
        "ts":      ts,
        "blocks": []map[string]interface{}{
            {
                "type": "section",
                "text": map[string]string{
                    "type": "mrkdwn",
                    "text": fmt.Sprintf("*%s*\n\n*タイトル*: %s\n*リンク*: <%s>", status, task.Title, task.PRURL),
                },
            },
        },
    }

    jsonData, _ := json.Marshal(body)
    req, _ := http.NewRequest("POST", "https://slack.com/api/chat.update", bytes.NewBuffer(jsonData))
    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    // スレッドにメッセージを投稿
    if task.Status == "pending" && task.Reviewer != "" {
        postToThread(channel, ts, fmt.Sprintf("<@%s> さんがレビュー担当になりました", task.Reviewer))
    } else if task.Status == "watching" {
        postToThread(channel, ts, fmt.Sprintf("<@%s> さんが確認中です（2時間）", task.Reviewer))
    }

    return nil
}

// スレッドにメッセージを投稿する関数
func postToThread(channel, ts, message string) error {
    body := map[string]interface{}{
        "channel": channel,
        "thread_ts": ts,  // これがスレッド投稿の重要なパラメータ
        "text": message,
    }

    jsonData, _ := json.Marshal(body)
    req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }

    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    // レスポンスをログに記録
    bodyBytes, _ := io.ReadAll(resp.Body)
    fmt.Println("🧵 スレッド投稿レスポンス:", string(bodyBytes))

    return nil
}
