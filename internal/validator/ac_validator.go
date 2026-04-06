package validator

import (
	"regexp"
	"strings"
)

// ValidateAC returns a list of warnings if the description lacks required AC for the given issue type.
func ValidateAC(issueType, description string) []string {
	var warnings []string

	// Look for AC section
	acRegex := regexp.MustCompile(`(?i)(##+\s*Acceptance Criteria|##+\s*AC)`)
	acMatch := acRegex.FindStringIndex(description)

	if acMatch == nil {
		if issueType == "api" || issueType == "backend" {
			warnings = append(warnings, "Issue is missing an 'Acceptance Criteria' or 'AC' section.")
		}
		return warnings
	}

	acContent := description[acMatch[1]:]

	switch issueType {
	case "api":
		requiredKeywords := []struct {
			keywords []string
			warning  string
		}{
			{[]string{"path", "method", "endpoint"}, "Type 'api' usually requires an endpoint path and method."},
			{[]string{"request", "body", "param"}, "Type 'api' should specify a request schema."},
			{[]string{"response", "schema", "success"}, "Type 'api' should specify a response schema."},
			{[]string{"status code", "200", "400", "404", "500"}, "Type 'api' should specify expected status codes."},
		}

		for _, req := range requiredKeywords {
			found := false
			for _, kw := range req.keywords {
				if strings.Contains(strings.ToLower(acContent), kw) {
					found = true
					break
				}
			}
			if !found {
				warnings = append(warnings, req.warning)
			}
		}

	case "backend":
		requiredKeywords := []struct {
			keywords []string
			warning  string
		}{
			{[]string{"logic", "algorithm", "rule"}, "Type 'backend' should describe core business logic or rules."},
			{[]string{"database", "table", "column", "persistence", "state"}, "Type 'backend' should specify database or persistence changes."},
			{[]string{"service", "dependency", "api"}, "Type 'backend' should list external or internal dependencies."},
		}

		for _, req := range requiredKeywords {
			found := false
			for _, kw := range req.keywords {
				if strings.Contains(strings.ToLower(acContent), kw) {
					found = true
					break
				}
			}
			if !found {
				warnings = append(warnings, req.warning)
			}
		}

	default:
		// Generic validation: check for at least one bullet point or numbered list
		listRegex := regexp.MustCompile(`(?m)^\s*([-*+]|\d+\.)\s+.+`)
		if !listRegex.MatchString(acContent) {
			warnings = append(warnings, "Acceptance criteria should include at least one bullet point or numbered item.")
		}
	}

	return warnings
}
