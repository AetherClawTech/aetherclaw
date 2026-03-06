package config

import (
	"encoding/json"
	"testing"
)

func TestTeamConfig_ParseFull(t *testing.T) {
	raw := `{
		"agents": {"defaults": {"workspace": "/tmp"}},
		"channels": {},
		"gateway": {},
		"tools": {},
		"heartbeat": {},
		"devices": {},
		"teams": {
			"dev-team": {
				"name": "Development Team",
				"layers": {
					"intent": ["cos"],
					"execution": ["cto", "builder"],
					"maintenance": ["ko"]
				},
				"a2a": {
					"max_ping_pong_turns": 3,
					"max_iterations": 8,
					"timeout_seconds": 120,
					"wait_discipline": true
				}
			}
		}
	}`

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	team, ok := cfg.Teams["dev-team"]
	if !ok {
		t.Fatal("expected teams['dev-team'] to exist")
	}
	if team.Name != "Development Team" {
		t.Errorf("Name = %q, want 'Development Team'", team.Name)
	}
	if len(team.Layers.Intent) != 1 || team.Layers.Intent[0] != "cos" {
		t.Errorf("Layers.Intent = %v, want [cos]", team.Layers.Intent)
	}
	if len(team.Layers.Execution) != 2 {
		t.Errorf("Layers.Execution = %v, want [cto builder]", team.Layers.Execution)
	}
	if len(team.Layers.Maintenance) != 1 || team.Layers.Maintenance[0] != "ko" {
		t.Errorf("Layers.Maintenance = %v, want [ko]", team.Layers.Maintenance)
	}
	if team.A2A.MaxPingPongTurns != 3 {
		t.Errorf("A2A.MaxPingPongTurns = %d, want 3", team.A2A.MaxPingPongTurns)
	}
	if team.A2A.MaxIterations != 8 {
		t.Errorf("A2A.MaxIterations = %d, want 8", team.A2A.MaxIterations)
	}
	if team.A2A.TimeoutSeconds != 120 {
		t.Errorf("A2A.TimeoutSeconds = %d, want 120", team.A2A.TimeoutSeconds)
	}
	if !team.A2A.WaitDiscipline {
		t.Error("A2A.WaitDiscipline should be true")
	}
}

func TestTeamConfig_ParseMinimal(t *testing.T) {
	raw := `{
		"agents": {"defaults": {"workspace": "/tmp"}},
		"channels": {},
		"gateway": {},
		"tools": {},
		"heartbeat": {},
		"devices": {},
		"teams": {
			"small": {
				"layers": {
					"intent": ["lead"],
					"execution": ["worker"]
				}
			}
		}
	}`

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	team := cfg.Teams["small"]
	if len(team.Layers.Intent) != 1 {
		t.Errorf("Intent = %v, want [lead]", team.Layers.Intent)
	}
	if len(team.Layers.Execution) != 1 {
		t.Errorf("Execution = %v, want [worker]", team.Layers.Execution)
	}
	if team.Layers.Maintenance != nil {
		t.Errorf("Maintenance = %v, want nil", team.Layers.Maintenance)
	}
	// A2A defaults to zero values
	if team.A2A.MaxPingPongTurns != 0 {
		t.Errorf("A2A.MaxPingPongTurns = %d, want 0 (not set)", team.A2A.MaxPingPongTurns)
	}
}

func TestTeamConfig_NoTeamsSection(t *testing.T) {
	raw := `{
		"agents": {"defaults": {"workspace": "/tmp"}},
		"channels": {},
		"gateway": {},
		"tools": {},
		"heartbeat": {},
		"devices": {}
	}`

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if cfg.Teams != nil {
		t.Errorf("Teams = %v, want nil", cfg.Teams)
	}
}

func TestDefaultA2AConfig(t *testing.T) {
	d := DefaultA2AConfig()
	if d.MaxPingPongTurns != 5 {
		t.Errorf("MaxPingPongTurns = %d, want 5", d.MaxPingPongTurns)
	}
	if d.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", d.MaxIterations)
	}
	if d.TimeoutSeconds != 300 {
		t.Errorf("TimeoutSeconds = %d, want 300", d.TimeoutSeconds)
	}
	if !d.WaitDiscipline {
		t.Error("WaitDiscipline should be true")
	}
}

func TestTeamConfig_RoundTrip(t *testing.T) {
	cfg := Config{
		Teams: map[string]TeamConfig{
			"test": {
				Name: "Test Team",
				Layers: TeamLayers{
					Intent:    []string{"cos"},
					Execution: []string{"builder"},
				},
				A2A: DefaultA2AConfig(),
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var cfg2 Config
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	team := cfg2.Teams["test"]
	if team.Name != "Test Team" {
		t.Errorf("Name = %q after round-trip", team.Name)
	}
	if len(team.Layers.Intent) != 1 || team.Layers.Intent[0] != "cos" {
		t.Errorf("Intent = %v after round-trip", team.Layers.Intent)
	}
}
