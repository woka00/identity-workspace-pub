ALTER TABLE user_profiles
    ADD COLUMN workspace_preferences JSONB NOT NULL DEFAULT
    '{"features":["tracker","calories","tasks","projects","widgets"],"onboardingCompleted":false,"timerIntroSeen":false}'::jsonb;
