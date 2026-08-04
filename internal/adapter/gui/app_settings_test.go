package main

import "testing"

func TestApp_GetSettings_Defaults(t *testing.T) {
	a := newTestApp(t)
	s, err := a.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.AlertThresholdDays != 30 {
		t.Fatalf("esperava default AlertThresholdDays=30, obteve %d", s.AlertThresholdDays)
	}
	if s.DefaultAlgorithm != "ed25519" {
		t.Fatalf("esperava default ed25519, obteve %q", s.DefaultAlgorithm)
	}
}

func TestApp_UpdateSettings_PersistsChanges(t *testing.T) {
	a := newTestApp(t)
	if err := a.UpdateSettings(AppSettingsDTO{AlertThresholdDays: 45, DefaultAlgorithm: "rsa"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	s, err := a.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.AlertThresholdDays != 45 || s.DefaultAlgorithm != "rsa" {
		t.Fatalf("esperava settings persistidas, obteve %+v", s)
	}
}

func TestApp_UpdateSettings_AffectsGenerateKeyDefault(t *testing.T) {
	a := newTestApp(t)
	if err := a.UpdateSettings(AppSettingsDTO{AlertThresholdDays: 30, DefaultAlgorithm: "rsa"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	key, err := a.GenerateKey(GenerateKeyInput{RSABits: 4096})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if key.Algorithm != "rsa" {
		t.Fatalf("esperava algoritmo default rsa (das settings), obteve %q", key.Algorithm)
	}
}
