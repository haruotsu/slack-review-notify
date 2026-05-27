package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

// OpenView posts to slack views.open with the supplied trigger_id and view payload.
// Honors SLACK_API_BASE_URL (for SlackHog) and IsTestMode (skips the call entirely).
func OpenView(triggerID string, view map[string]interface{}) error {
	if IsTestMode {
		log.Printf("test mode: would open view with trigger_id=%s", triggerID)
		return nil
	}

	body := map[string]interface{}{
		"trigger_id": triggerID,
		"view":       view,
	}
	return postViewsAPI("/views.open", body)
}

// UpdateView posts to slack views.update for re-rendering an existing modal.
func UpdateView(viewID string, view map[string]interface{}) error {
	if IsTestMode {
		log.Printf("test mode: would update view %s", viewID)
		return nil
	}
	body := map[string]interface{}{
		"view_id": viewID,
		"view":    view,
	}
	return postViewsAPI("/views.update", body)
}

func postViewsAPI(path string, body map[string]interface{}) error {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", SlackAPIBaseURL()+path, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBytes, _ := io.ReadAll(resp.Body)
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return fmt.Errorf("slack API response parse error: %v (body: %s)", err, string(respBytes))
	}
	if !result.OK {
		return fmt.Errorf("slack %s error: %s", path, result.Error)
	}
	return nil
}
