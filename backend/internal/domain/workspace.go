package domain

type WorkspacePreferences struct {
	Features            []string `json:"features"`
	OnboardingCompleted bool     `json:"onboardingCompleted"`
	TimerIntroSeen      bool     `json:"timerIntroSeen"`
}
