package services

import (
	"log"
	"slack-review-notify/models"

	"gorm.io/gorm"
)

// LooksLikeResolvedSlackUserID reports whether s is a bare Slack user id of
// the form `U…` / `W…` followed by uppercase alphanumerics. This is the
// canonical form the modal user picker delivers; legacy plain @handles fail
// here and need to be re-mapped before PR-author exclusion can work.
func LooksLikeResolvedSlackUserID(s string) bool {
	if len(s) < 2 {
		return false
	}
	if s[0] != 'U' && s[0] != 'W' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// FindLegacyUserMappings returns every UserMapping row whose SlackUserID is
// not a resolved user id. Operators can use this to spot rows that need to be
// re-mapped via the modal picker.
func FindLegacyUserMappings(db *gorm.DB) []models.UserMapping {
	var mappings []models.UserMapping
	if err := db.Find(&mappings).Error; err != nil {
		log.Printf("FindLegacyUserMappings: query failed: %v", err)
		return nil
	}
	var legacy []models.UserMapping
	for _, m := range mappings {
		if !LooksLikeResolvedSlackUserID(m.SlackUserID) {
			legacy = append(legacy, m)
		}
	}
	return legacy
}

// LogLegacyUserMappings emits a WARN line per legacy row so that the breakage
// (the PR author silently entering the candidate pool) is visible in app logs
// without requiring an operator to manually inspect the DB. Called once at
// startup; no-op when every mapping is well-formed.
func LogLegacyUserMappings(db *gorm.DB) {
	legacy := FindLegacyUserMappings(db)
	if len(legacy) == 0 {
		return
	}
	log.Printf("WARN: %d user_mapping row(s) have a non-resolved slack_user_id. "+
		"These rows cannot exclude the PR author from the reviewer pool. "+
		"Re-register them via the user-mapping modal: /slack-review-notify help.",
		len(legacy))
	for _, m := range legacy {
		log.Printf("WARN: legacy user_mapping: github=%s slack_user_id=%q", m.GithubUsername, m.SlackUserID)
	}
}
