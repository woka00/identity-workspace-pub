package application

import (
	"context"

	"avatar-id/internal/domain"
)

func (s *Service) UpdateWorkspacePreferences(ctx context.Context, preferences domain.WorkspacePreferences) error {
	if !preferences.OnboardingCompleted {
		return invalidf("complete workspace onboarding before saving")
	}
	if len(preferences.Features) == 0 || len(preferences.Features) > 5 {
		return invalidf("select at least one workspace feature")
	}
	seen := make(map[string]bool)
	for _, feature := range preferences.Features {
		switch feature {
		case "tracker", "calories", "tasks", "projects", "widgets":
		default:
			return invalidf("unknown workspace feature %q", feature)
		}
		if seen[feature] {
			return invalidf("duplicate workspace feature %q", feature)
		}
		seen[feature] = true
	}
	return s.repo.UpdateWorkspacePreferences(ctx, preferences)
}
