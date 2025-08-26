package services

import "github.com/google/go-github/v71/github"

// GetDisplayName は GitHub User から表示名を取得する
// Name フィールドが設定されている場合は Name を返し、
// そうでなければ Login を返す
func GetDisplayName(user *github.User) string {
	if user == nil {
		return ""
	}
	
	// Name フィールドが設定されていて、空文字でない場合はそれを使用
	if user.Name != nil && *user.Name != "" {
		return *user.Name
	}
	
	// Name が設定されていない、または空文字の場合は Login を使用
	if user.Login != nil {
		return *user.Login
	}
	
	// すべて nil の場合は空文字を返す
	return ""
}