package agent

import (
	"github.com/AetherClawTech/aetherclaw/pkg/multiagent"
)

type multiagentResolver struct {
	registry *AgentRegistry
}

// NOTE: This adapter lives in pkg/agent to avoid import cycles.
// The multiagent package must not depend on pkg/agent.
func newMultiagentResolver(registry *AgentRegistry) *multiagentResolver {
	return &multiagentResolver{registry: registry}
}

func (r *multiagentResolver) GetAgentInfo(agentID string) *multiagent.AgentInfo {
	agent, ok := r.registry.GetAgent(agentID)
	if !ok || agent == nil {
		return nil
	}

	return &multiagent.AgentInfo{
		ID:   agent.ID,
		Name: agent.Name,
		SystemPromptFn: func() string {
			return agent.ContextBuilder.BuildSystemPromptWithCache()
		},
		Model:    agent.Model,
		Provider: agent.Provider,
		Tools:    agent.Tools,
		MaxIter:  agent.MaxIterations,
	}
}

func (r *multiagentResolver) ListAgents() []multiagent.AgentInfo {
	ids := r.registry.ListAgentIDs()
	out := make([]multiagent.AgentInfo, 0, len(ids))
	for _, id := range ids {
		info := r.GetAgentInfo(id)
		if info != nil {
			out = append(out, *info)
		}
	}
	return out
}
