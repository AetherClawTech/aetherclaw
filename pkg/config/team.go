package config

// TeamConfig defines a synthetic team of agents organized in layers.
type TeamConfig struct {
	Name   string     `json:"name,omitempty"`
	Layers TeamLayers `json:"layers"`
	A2A    A2AConfig  `json:"a2a,omitempty"`
}

// TeamLayers organizes agents into the 3-layer OpenCrew-style hierarchy.
type TeamLayers struct {
	Intent      []string `json:"intent"`                // strategic layer (e.g., CoS)
	Execution   []string `json:"execution"`             // implementation layer (e.g., CTO, Builder)
	Maintenance []string `json:"maintenance,omitempty"` // ongoing operations (e.g., KO, Ops)
}

// A2AConfig controls agent-to-agent communication parameters.
type A2AConfig struct {
	MaxPingPongTurns int  `json:"max_ping_pong_turns,omitempty"`
	MaxIterations    int  `json:"max_iterations,omitempty"`
	TimeoutSeconds   int  `json:"timeout_seconds,omitempty"`
	WaitDiscipline   bool `json:"wait_discipline,omitempty"`
}

// DefaultA2AConfig returns safe defaults for agent-to-agent communication.
func DefaultA2AConfig() A2AConfig {
	return A2AConfig{
		MaxPingPongTurns: 5,
		MaxIterations:    10,
		TimeoutSeconds:   300,
		WaitDiscipline:   true,
	}
}
