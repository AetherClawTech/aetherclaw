// AetherClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 AetherClaw contributors

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/AetherClawTech/aetherclaw/pkg/bus"
	"github.com/AetherClawTech/aetherclaw/pkg/channels"
	"github.com/AetherClawTech/aetherclaw/pkg/config"
	"github.com/AetherClawTech/aetherclaw/pkg/constants"
	"github.com/AetherClawTech/aetherclaw/pkg/cron"
	"github.com/AetherClawTech/aetherclaw/pkg/logger"
	"github.com/AetherClawTech/aetherclaw/pkg/media"
	mcptypes "github.com/AetherClawTech/aetherclaw/pkg/mcp"
	mcpclient "github.com/AetherClawTech/aetherclaw/pkg/mcp/client"
	"github.com/AetherClawTech/aetherclaw/pkg/memory"
	"github.com/AetherClawTech/aetherclaw/pkg/multiagent"
	"github.com/AetherClawTech/aetherclaw/pkg/pairing"
	"github.com/AetherClawTech/aetherclaw/pkg/providers"
	"github.com/AetherClawTech/aetherclaw/pkg/routing"
	"github.com/AetherClawTech/aetherclaw/pkg/session"
	"github.com/AetherClawTech/aetherclaw/pkg/skills"
	"github.com/AetherClawTech/aetherclaw/pkg/state"
	"github.com/AetherClawTech/aetherclaw/pkg/tools"
	"github.com/AetherClawTech/aetherclaw/pkg/tts"
	"github.com/AetherClawTech/aetherclaw/pkg/usage"
	"github.com/AetherClawTech/aetherclaw/pkg/utils"
)

type AgentLoop struct {
	bus            *bus.MessageBus
	cfg            *config.Config
	registry       *AgentRegistry
	state          *state.Manager
	running        atomic.Bool
	summarizing    sync.Map
	blackboards    *blackboardStore
	fallback       *providers.FallbackChain
	channelManager *channels.Manager
	mediaStore     media.MediaStore
	cronService    *cron.CronService
	mcpManager     *mcpclient.MCPClientManager
	runRegistry    *multiagent.RunRegistry
	announcer      *multiagent.Announcer
	spawnManager   *multiagent.SpawnManager
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey      string   // Session identifier for history/context
	Channel         string   // Target channel for tool execution
	ChatID          string   // Target chat ID for tool execution
	UserMessage     string   // User message content (may include prefix)
	Media           []string // Media file paths (images, etc.)
	DefaultResponse string   // Response when LLM returns empty
	EnableSummary   bool     // Whether to trigger summarization
	SendResponse    bool     // Whether to send response via bus
	NoHistory       bool     // If true, don't load session history (for heartbeat)
}

const defaultResponse = "I've completed processing but have no response to give. Increase `max_tool_iterations` in config.json."

func NewAgentLoop(cfg *config.Config, msgBus *bus.MessageBus, provider providers.LLMProvider) *AgentLoop {
	registry := NewAgentRegistry(cfg, provider)

	// Initialize multi-agent infrastructure
	runRegistry := multiagent.NewRunRegistry()
	announcer := multiagent.NewAnnouncer(32)
	spawnManager := multiagent.NewSpawnManager(runRegistry, announcer, 5, 5*time.Minute)

	// Register shared tools to all agents
	registerSharedTools(cfg, msgBus, registry, provider, spawnManager, runRegistry)

	// Set up shared fallback chain
	cooldown := providers.NewCooldownTracker()
	fallbackChain := providers.NewFallbackChain(cooldown)

	// Create state manager using default agent's workspace for channel recording
	defaultAgent := registry.GetDefaultAgent()
	var stateManager *state.Manager
	if defaultAgent != nil {
		stateManager = state.NewManager(defaultAgent.Workspace)
	}

	// Create cron service
	workspacePath := cfg.WorkspacePath()
	cronStorePath := filepath.Join(workspacePath, "cron", "store.json")
	os.MkdirAll(filepath.Dir(cronStorePath), 0o755)
	cronService := cron.NewCronService(cronStorePath, nil) // handler wired in Run()

	al := &AgentLoop{
		bus:          msgBus,
		cfg:          cfg,
		registry:     registry,
		state:        stateManager,
		summarizing:  sync.Map{},
		blackboards:  newBlackboardStore(defaultBlackboardCacheSize),
		fallback:     fallbackChain,
		cronService:  cronService,
		runRegistry:  runRegistry,
		announcer:    announcer,
		spawnManager: spawnManager,
	}

	// Register extended tools that need AgentLoop reference (cron)
	registerExtendedTools(cfg, msgBus, registry, al)

	// Start MCP clients and register their tools to agents (respecting per-agent filtering)
	if len(cfg.MCP.Servers) > 0 {
		mcpConfigs := configToMCPClientConfigs(cfg.MCP.Servers)
		al.mcpManager = mcpclient.NewMCPClientManager()
		al.mcpManager.StartFromConfig(context.Background(), mcpConfigs)

		for _, id := range registry.ListAgentIDs() {
			if a, ok := registry.GetAgent(id); ok {
				count := al.mcpManager.RegisterToolsToAgent(id, a.Tools)
				if count > 0 {
					logger.InfoCF("agent", "MCP tools registered",
						map[string]any{"agent_id": id, "tools": count})
				}
			}
		}

		al.mcpManager.StartHealthMonitor(registry)

		// Inject MCP configs into CLI providers for native passthrough.
		// Takes the union of all stdio MCP servers — CLI providers get all of them
		// since per-agent filtering isn't possible when agents share a provider instance.
		allMCPConfigs := buildCLIMCPProviderConfigs(cfg.MCP.Servers)
		if len(allMCPConfigs) > 0 {
			injected := make(map[providers.MCPConfigurable]bool)
			for _, id := range registry.ListAgentIDs() {
				agent, ok := registry.GetAgent(id)
				if !ok {
					continue
				}
				if configurable, ok := agent.Provider.(providers.MCPConfigurable); ok && !injected[configurable] {
					configurable.SetMCPConfigs(allMCPConfigs)
					injected[configurable] = true
					logger.InfoCF("agent", "MCP configs injected into CLI provider",
						map[string]any{"agent_id": id, "mcp_servers": len(allMCPConfigs)})
				}
			}
		}
	}

	return al
}

// registerSharedTools registers tools that are shared across all agents (web, message, spawn, sessions, etc.).
func registerSharedTools(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	registry *AgentRegistry,
	provider providers.LLMProvider,
	spawnManager *multiagent.SpawnManager,
	runRegistry *multiagent.RunRegistry,
) {
	workspacePath := cfg.WorkspacePath()
	openAIKey := cfg.Providers.OpenAI.APIKey
	geminiKey := cfg.Providers.Gemini.APIKey

	// Shared managers (created once, shared across all agents)
	approvalMgr := tools.NewApprovalManager()
	autoReplyMgr := tools.NewAutoReplyManager()
	usageTracker := usage.NewTracker(workspacePath)
	pairingStore := pairing.NewStore(workspacePath)

	// TTS manager (OpenAI + ElevenLabs + Edge)
	var ttsProviders []tts.Provider
	if openAIKey != "" {
		if p := tts.NewOpenAITTS(openAIKey); p != nil {
			ttsProviders = append(ttsProviders, p)
		}
	}
	elevenLabsKey := os.Getenv("ELEVENLABS_API_KEY")
	if elevenLabsKey != "" {
		if p := tts.NewElevenLabsTTS(elevenLabsKey); p != nil {
			ttsProviders = append(ttsProviders, p)
		}
	}
	// Edge TTS is always available (free, no API key required)
	ttsProviders = append(ttsProviders, tts.NewEdgeTTS())
	ttsOutputDir := filepath.Join(workspacePath, "tts")
	os.MkdirAll(ttsOutputDir, 0o755)
	ttsManager := tts.NewManager(ttsOutputDir, ttsProviders...)

	// Memory embedding provider (prefer OpenAI, fallback to Gemini)
	var embeddingProvider memory.EmbeddingProvider
	if openAIKey != "" {
		embeddingProvider = memory.NewOpenAIEmbedding(openAIKey)
	} else if geminiKey != "" {
		embeddingProvider = memory.NewGeminiEmbedding(geminiKey)
	}

	// Sessions tool deps (shared access to all agent sessions)
	sessionsDeps := &tools.SessionsToolDeps{
		GetSessions: func(agentID string) *session.SessionManager {
			if a, ok := registry.GetAgent(agentID); ok {
				return a.Sessions
			}
			return nil
		},
		CanAccess: func(currentAgentID, targetAgentID string) bool {
			return registry.CanSpawnSubagent(currentAgentID, targetAgentID)
		},
		ListAgentIDs: func() []string {
			return registry.ListAgentIDs()
		},
		Bus: msgBus,
	}

	// Agents list deps
	agentsListDeps := &tools.AgentsListDeps{
		ListAgentIDs: func() []string {
			return registry.ListAgentIDs()
		},
		GetAgentInfo: func(agentID string) *tools.AgentInfo {
			a, ok := registry.GetAgent(agentID)
			if !ok {
				return nil
			}
			return &tools.AgentInfo{
				ID:        a.ID,
				Model:     a.Model,
				Workspace: a.Workspace,
				Status:    "running",
			}
		},
	}

	// Loop detector hook (shared across agents)
	loopDetector := tools.NewLoopDetector(tools.DefaultLoopDetectorConfig())
	multiagentResolver := newMultiagentResolver(registry)

	for _, agentID := range registry.ListAgentIDs() {
		agent, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}
		currentAgentID := agentID

		// Web tools
		if searchTool := tools.NewWebSearchTool(tools.WebSearchToolOptions{
			BraveAPIKey:          cfg.Tools.Web.Brave.APIKey,
			BraveMaxResults:      cfg.Tools.Web.Brave.MaxResults,
			BraveEnabled:         cfg.Tools.Web.Brave.Enabled,
			TavilyAPIKey:         cfg.Tools.Web.Tavily.APIKey,
			TavilyBaseURL:        cfg.Tools.Web.Tavily.BaseURL,
			TavilyMaxResults:     cfg.Tools.Web.Tavily.MaxResults,
			TavilyEnabled:        cfg.Tools.Web.Tavily.Enabled,
			DuckDuckGoMaxResults: cfg.Tools.Web.DuckDuckGo.MaxResults,
			DuckDuckGoEnabled:    cfg.Tools.Web.DuckDuckGo.Enabled,
			PerplexityAPIKey:     cfg.Tools.Web.Perplexity.APIKey,
			PerplexityMaxResults: cfg.Tools.Web.Perplexity.MaxResults,
			PerplexityEnabled:    cfg.Tools.Web.Perplexity.Enabled,
			Proxy:                cfg.Tools.Web.Proxy,
		}); searchTool != nil {
			agent.Tools.Register(searchTool)
		}
		agent.Tools.Register(tools.NewWebFetchToolWithProxy(50000, cfg.Tools.Web.Proxy))

		// Hardware tools (I2C, SPI) - Linux only, returns error on other platforms
		agent.Tools.Register(tools.NewI2CTool())
		agent.Tools.Register(tools.NewSPITool())

		// Message tool
		messageTool := tools.NewMessageTool()
		messageTool.SetSendCallback(func(channel, chatID, content string) error {
			pubCtx, pubCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer pubCancel()
			return msgBus.PublishOutbound(pubCtx, bus.OutboundMessage{
				Channel: channel,
				ChatID:  chatID,
				Content: content,
			})
		})
		agent.Tools.Register(messageTool)

		// Skill discovery and installation tools
		registryMgr := skills.NewRegistryManagerFromConfig(skills.RegistryConfig{
			MaxConcurrentSearches: cfg.Tools.Skills.MaxConcurrentSearches,
			ClawHub:               skills.ClawHubConfig(cfg.Tools.Skills.Registries.ClawHub),
		})
		searchCache := skills.NewSearchCache(
			cfg.Tools.Skills.SearchCache.MaxSize,
			time.Duration(cfg.Tools.Skills.SearchCache.TTLSeconds)*time.Second,
		)
		agent.Tools.Register(tools.NewFindSkillsTool(registryMgr, searchCache))
		agent.Tools.Register(tools.NewInstallSkillTool(registryMgr, agent.Workspace))

		// Multi-agent tools (blackboard + handoff + spawn + list_agents)
		agent.Tools.Register(multiagent.NewBlackboardTool(nil, currentAgentID))
		handoffTool := multiagent.NewHandoffTool(multiagentResolver, nil, currentAgentID)
		handoffTool.SetAllowlistChecker(multiagent.AllowlistCheckerFunc(func(fromAgentID, toAgentID string) bool {
			return registry.CanSpawnSubagent(fromAgentID, toAgentID)
		}))
		handoffTool.SetRunRegistry(runRegistry, "")
		agent.Tools.Register(handoffTool)

		// Async spawn with RunRegistry/Announcer/cascade cancellation
		spawnTool := multiagent.NewSpawnTool(multiagentResolver, nil, spawnManager, currentAgentID)
		spawnTool.SetAllowlistChecker(multiagent.AllowlistCheckerFunc(func(from, to string) bool {
			return registry.CanSpawnSubagent(from, to)
		}))
		spawnTool.SetRunRegistry(runRegistry, "")
		agent.Tools.Register(spawnTool)

		// Agent discovery with role + capabilities
		agent.Tools.Register(multiagent.NewListAgentsTool(multiagentResolver))

		// --- New tools ---

		// Sessions tools (cross-agent communication)
		agent.Tools.Register(tools.NewSessionsListTool(sessionsDeps, currentAgentID))
		agent.Tools.Register(tools.NewSessionsHistoryTool(sessionsDeps, currentAgentID))
		agent.Tools.Register(tools.NewSessionsSendTool(sessionsDeps, currentAgentID))

		// Agents list
		agent.Tools.Register(tools.NewAgentsListTool(agentsListDeps))

		// Image generation (DALL-E 3, requires OpenAI key)
		if imgTool := tools.NewImageGenTool(openAIKey); imgTool != nil {
			agent.Tools.Register(imgTool)
		}

		// TTS (text-to-speech)
		if ttsTool := tools.NewTTSTool(ttsManager); ttsTool != nil {
			agent.Tools.Register(ttsTool)
		}

		// Per-agent memory (hybrid BM25 + vector search)
		memDir := filepath.Join(workspacePath, "memory", currentAgentID)
		os.MkdirAll(memDir, 0o755)
		memIndex := memory.NewIndex(memDir, embeddingProvider)
		if memTool := tools.NewMemoryTool(memIndex); memTool != nil {
			agent.Tools.Register(memTool)
		}

		// Usage tracking
		if usageTool := tools.NewUsageTool(usageTracker); usageTool != nil {
			agent.Tools.Register(usageTool)
		}

		// Approval workflow
		if approvalTool := tools.NewApprovalTool(approvalMgr); approvalTool != nil {
			agent.Tools.Register(approvalTool)
		}

		// Auto-reply rules
		if autoReplyTool := tools.NewAutoReplyTool(autoReplyMgr); autoReplyTool != nil {
			agent.Tools.Register(autoReplyTool)
		}

		// Channel actions (pin, delete, react, etc.)
		agent.Tools.Register(tools.NewChannelActionsTool())

		// Device pairing
		if pairingTool := tools.NewPairingTool(pairingStore); pairingTool != nil {
			agent.Tools.Register(pairingTool)
		}

		// Loop detector hook (prevents infinite tool loops)
		agent.Tools.AddHook(loopDetector)

		// Autonomy hook (enforces per-agent autonomy levels)
		autonomyLevel := tools.L1_Reversible // default
		for _, ac := range cfg.Agents.List {
			if ac.ID == currentAgentID && ac.Autonomy != nil {
				autonomyLevel = tools.AutonomyLevel(*ac.Autonomy)
				break
			}
		}
		agent.Tools.AddHook(tools.NewAutonomyHook(currentAgentID, autonomyLevel, nil))
	}
}

// registerExtendedTools registers tools that need a reference to the AgentLoop (e.g., cron).
func registerExtendedTools(
	cfg *config.Config,
	msgBus *bus.MessageBus,
	registry *AgentRegistry,
	al *AgentLoop,
) {
	workspacePath := cfg.WorkspacePath()
	restrict := cfg.Agents.Defaults.RestrictToWorkspace
	execTimeout := time.Duration(cfg.Tools.Cron.ExecTimeoutMinutes) * time.Minute

	for _, agentID := range registry.ListAgentIDs() {
		agent, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}

		// Cron tool (needs AgentLoop as JobExecutor)
		cronTool, err := tools.NewCronTool(al.cronService, al, msgBus, workspacePath, restrict, execTimeout, cfg)
		if err != nil {
			logger.ErrorCF("agent", "Failed to create cron tool",
				map[string]any{"agent": agentID, "error": err.Error()})
			continue
		}
		agent.Tools.Register(cronTool)
	}
}

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	// Start cron service with job handler
	if al.cronService != nil {
		al.cronService.SetOnJob(func(job *cron.CronJob) (string, error) {
			// Find the cron tool on the default agent to execute the job
			defaultAgent := al.registry.GetDefaultAgent()
			if defaultAgent == nil {
				return "", fmt.Errorf("no default agent for cron execution")
			}
			if tool, ok := defaultAgent.Tools.Get("cron"); ok {
				if cronTool, ok := tool.(*tools.CronTool); ok {
					result := cronTool.ExecuteJob(ctx, job)
					return result, nil
				}
			}
			return "", fmt.Errorf("cron tool not available")
		})
		if err := al.cronService.Start(); err != nil {
			logger.ErrorCF("agent", "Failed to start cron service",
				map[string]any{"error": err.Error()})
		}
		defer al.cronService.Stop()
	}


	for al.running.Load() {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, ok := al.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			// Process message
			func() {
				// Media cleanup: safe to run after processMessage returns because
				// BuildMessages now base64-encodes images synchronously into ContentParts
				// before any async work begins.
				defer func() {
					if al.mediaStore != nil && msg.MediaScope != "" {
						if releaseErr := al.mediaStore.ReleaseAll(msg.MediaScope); releaseErr != nil {
							logger.WarnCF("agent", "Failed to release media", map[string]any{
								"scope": msg.MediaScope,
								"error": releaseErr.Error(),
							})
						}
					}
				}()

				response, err := al.processMessage(ctx, msg)
				if err != nil {
					response = fmt.Sprintf("Error processing message: %v", err)
				}

				if response != "" {
					// Check if the message tool already sent a response during this round.
					// If so, skip publishing to avoid duplicate messages to the user.
					// Use default agent's tools to check (message tool is shared).
					alreadySent := false
					defaultAgent := al.registry.GetDefaultAgent()
					if defaultAgent != nil {
						if tool, ok := defaultAgent.Tools.Get("message"); ok {
							if mt, ok := tool.(*tools.MessageTool); ok {
								alreadySent = mt.HasSentInRound()
							}
						}
					}

					if !alreadySent {
						al.bus.PublishOutbound(ctx, bus.OutboundMessage{
							Channel: msg.Channel,
							ChatID:  msg.ChatID,
							Content: response,
						})
						logger.InfoCF("agent", "Published outbound response",
							map[string]any{
								"channel":     msg.Channel,
								"chat_id":     msg.ChatID,
								"content_len": len(response),
							})
					} else {
						logger.DebugCF(
							"agent",
							"Skipped outbound (message tool already sent)",
							map[string]any{"channel": msg.Channel},
						)
					}
				}
			}()
		}
	}

	return nil
}

func (al *AgentLoop) Stop() {
	al.running.Store(false)
	if al.runRegistry != nil {
		al.runRegistry.StopAll()
	}
	if al.spawnManager != nil {
		al.spawnManager.Stop()
	}
	if al.mcpManager != nil {
		al.mcpManager.StopAll()
	}
}

func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	for _, agentID := range al.registry.ListAgentIDs() {
		if agent, ok := al.registry.GetAgent(agentID); ok {
			agent.Tools.Register(tool)
		}
	}
}

func (al *AgentLoop) SetChannelManager(cm *channels.Manager) {
	al.channelManager = cm
}

// SetMediaStore injects a MediaStore for media lifecycle management.
func (al *AgentLoop) SetMediaStore(s media.MediaStore) {
	al.mediaStore = s
}

// inferMediaType determines the media type ("image", "audio", "video", "file")
// from a filename and MIME content type.
func inferMediaType(filename, contentType string) string {
	ct := strings.ToLower(contentType)
	fn := strings.ToLower(filename)

	if strings.HasPrefix(ct, "image/") {
		return "image"
	}
	if strings.HasPrefix(ct, "audio/") || ct == "application/ogg" {
		return "audio"
	}
	if strings.HasPrefix(ct, "video/") {
		return "video"
	}

	// Fallback: infer from extension
	ext := filepath.Ext(fn)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg":
		return "image"
	case ".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac", ".wma", ".opus":
		return "audio"
	case ".mp4", ".avi", ".mov", ".webm", ".mkv":
		return "video"
	}

	return "file"
}

// RecordLastChannel records the last active channel for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChannel(channel string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChannel(channel)
}

// RecordLastChatID records the last active chat ID for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChatID(chatID string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChatID(chatID)
}

func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

func (al *AgentLoop) ProcessDirectWithChannel(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

// ProcessHeartbeat processes a heartbeat request without session history.
// Each heartbeat is independent and doesn't accumulate context.
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, content, channel, chatID string) (string, error) {
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for heartbeat")
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      "heartbeat",
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: defaultResponse,
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true, // Don't load session history for heartbeat
	})
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Add message preview to log (show full content for error messages)
	var logContent string
	if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content // Full content for errors
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF("agent", fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]any{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		})

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	// Check for commands
	if response, handled := al.handleCommand(ctx, msg); handled {
		return response, nil
	}

	// Route to determine agent and session key
	route := al.registry.ResolveRoute(routing.RouteInput{
		Channel:    msg.Channel,
		AccountID:  msg.Metadata["account_id"],
		Peer:       extractPeer(msg),
		ParentPeer: extractParentPeer(msg),
		GuildID:    msg.Metadata["guild_id"],
		TeamID:     msg.Metadata["team_id"],
	})

	agent, ok := al.registry.GetAgent(route.AgentID)
	if !ok {
		agent = al.registry.GetDefaultAgent()
	}
	if agent == nil {
		return "", fmt.Errorf("no agent available for route (agent_id=%s)", route.AgentID)
	}

	// Reset message-tool state for this round so we don't skip publishing due to a previous round.
	if tool, ok := agent.Tools.Get("message"); ok {
		if mt, ok := tool.(tools.ContextualTool); ok {
			mt.SetContext(msg.Channel, msg.ChatID)
		}
	}

	// Use routed session key, but honor pre-set agent-scoped keys (for ProcessDirect/cron)
	sessionKey := route.SessionKey
	if msg.SessionKey != "" && strings.HasPrefix(msg.SessionKey, "agent:") {
		sessionKey = msg.SessionKey
	}

	logger.InfoCF("agent", "Routed message",
		map[string]any{
			"agent_id":    agent.ID,
			"session_key": sessionKey,
			"matched_by":  route.MatchedBy,
		})

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     msg.Content,
		Media:           msg.Media,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
	})
}

func (al *AgentLoop) processSystemMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	if msg.Channel != "system" {
		return "", fmt.Errorf("processSystemMessage called with non-system message channel: %s", msg.Channel)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]any{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel, originChatID string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	} else {
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	// Extract subagent result from message content
	// Format: "Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // Extract just the result part
	}

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]any{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// Use default agent for system messages
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent for system message")
	}

	// Use the origin session for context
	sessionKey := routing.BuildAgentMainSessionKey(agent.ID)

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     fmt.Sprintf("[System: %s] %s", msg.SenderID, msg.Content),
		DefaultResponse: "Background task completed.",
		EnableSummary:   false,
		SendResponse:    true,
	})
}

// runAgentLoop is the core message processing logic.
func (al *AgentLoop) runAgentLoop(ctx context.Context, agent *AgentInstance, opts processOptions) (string, error) {
	// 0. Record last channel for heartbeat notifications (skip internal channels)
	if opts.Channel != "" && opts.ChatID != "" {
		// Don't record internal channels (cli, system, subagent)
		if !constants.IsInternalChannel(opts.Channel) {
			channelKey := fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID)
			if err := al.RecordLastChannel(channelKey); err != nil {
				logger.WarnCF("agent", "Failed to record last channel", map[string]any{"error": err.Error()})
			}
		}
	}

	// 1. Update tool contexts (channel, chatID, sessionKey, board)
	al.updateToolContexts(agent, opts.Channel, opts.ChatID, opts.SessionKey)

	// 2. Enrich user message with link content (if configured)
	userMessage := opts.UserMessage
	if al.cfg.Tools.Web.LinkEnrichment.Enabled {
		maxLinks := al.cfg.Tools.Web.LinkEnrichment.MaxPerMessage
		if maxLinks <= 0 {
			maxLinks = 3
		}
		userMessage = tools.EnrichMessageWithLinks(userMessage, maxLinks)
	}

	// 3. Build messages (skip history for heartbeat)
	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = agent.Sessions.GetHistory(opts.SessionKey)
		summary = agent.Sessions.GetSummary(opts.SessionKey)
	}
	messages := agent.ContextBuilder.BuildMessages(
		history,
		summary,
		userMessage,
		opts.Media,
		opts.Channel,
		opts.ChatID,
	)

	// 4. Save user message to session
	agent.Sessions.AddMessage(opts.SessionKey, "user", userMessage)

	// 5. Run LLM iteration loop
	finalContent, iteration, err := al.runLLMIteration(ctx, agent, messages, opts)
	if err != nil {
		return "", err
	}

	// If last tool had ForUser content and we already sent it, we might not need to send final response
	// This is controlled by the tool's Silent flag and ForUser content

	// 6. Handle empty response
	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}

	// 7. Save final assistant message to session
	agent.Sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	agent.Sessions.Save(opts.SessionKey)

	// 8. Optional: summarization
	if opts.EnableSummary {
		al.maybeSummarize(agent, opts.SessionKey, opts.Channel, opts.ChatID)
	}

	// 8. Optional: send response via bus
	if opts.SendResponse {
		al.bus.PublishOutbound(ctx, bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Content: finalContent,
		})
	}

	// 9. Log response
	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF("agent", fmt.Sprintf("Response: %s", responsePreview),
		map[string]any{
			"agent_id":     agent.ID,
			"session_key":  opts.SessionKey,
			"iterations":   iteration,
			"final_length": len(finalContent),
		})

	return finalContent, nil
}

func (al *AgentLoop) targetReasoningChannelID(channelName string) (chatID string) {
	if al.channelManager == nil {
		return ""
	}
	if ch, ok := al.channelManager.GetChannel(channelName); ok {
		return ch.ReasoningChannelID()
	}
	return ""
}

func (al *AgentLoop) handleReasoning(ctx context.Context, reasoningContent, channelName, channelID string) {
	if reasoningContent == "" || channelName == "" || channelID == "" {
		return
	}

	// Check context cancellation before attempting to publish,
	// since PublishOutbound's select may race between send and ctx.Done().
	if ctx.Err() != nil {
		return
	}

	// Use a short timeout so the goroutine does not block indefinitely when
	// the outbound bus is full.  Reasoning output is best-effort; dropping it
	// is acceptable to avoid goroutine accumulation.
	pubCtx, pubCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pubCancel()

	if err := al.bus.PublishOutbound(pubCtx, bus.OutboundMessage{
		Channel: channelName,
		ChatID:  channelID,
		Content: reasoningContent,
	}); err != nil {
		// Treat context.DeadlineExceeded / context.Canceled as expected
		// (bus full under load, or parent canceled).  Check the error
		// itself rather than ctx.Err(), because pubCtx may time out
		// (5 s) while the parent ctx is still active.
		// Also treat ErrBusClosed as expected — it occurs during normal
		// shutdown when the bus is closed before all goroutines finish.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
			errors.Is(err, bus.ErrBusClosed) {
			logger.DebugCF("agent", "Reasoning publish skipped (timeout/cancel)", map[string]any{
				"channel": channelName,
				"error":   err.Error(),
			})
		} else {
			logger.WarnCF("agent", "Failed to publish reasoning (best-effort)", map[string]any{
				"channel": channelName,
				"error":   err.Error(),
			})
		}
	}
}

// runLLMIteration executes the LLM call loop with tool handling.
func (al *AgentLoop) runLLMIteration(
	ctx context.Context,
	agent *AgentInstance,
	messages []providers.Message,
	opts processOptions,
) (string, int, error) {
	iteration := 0
	var finalContent string
	toolCtx := al.toolContext(ctx, opts.SessionKey, opts.NoHistory)

	for iteration < agent.MaxIterations {
		iteration++

		logger.DebugCF("agent", "LLM iteration",
			map[string]any{
				"agent_id":  agent.ID,
				"iteration": iteration,
				"max":       agent.MaxIterations,
			})

		// Build tool definitions
		providerToolDefs := agent.Tools.ToProviderDefs()

		// Log LLM request details
		logger.DebugCF("agent", "LLM request",
			map[string]any{
				"agent_id":          agent.ID,
				"iteration":         iteration,
				"model":             agent.Model,
				"messages_count":    len(messages),
				"tools_count":       len(providerToolDefs),
				"max_tokens":        agent.MaxTokens,
				"temperature":       agent.Temperature,
				"system_prompt_len": len(messages[0].Content),
			})

		// Log full messages (detailed)
		logger.DebugCF("agent", "Full LLM request",
			map[string]any{
				"iteration":     iteration,
				"messages_json": formatMessagesForLog(messages),
				"tools_json":    formatToolsForLog(providerToolDefs),
			})

		// Call LLM with fallback chain if candidates are configured.
		var response *providers.LLMResponse
		var err error

		callLLM := func() (*providers.LLMResponse, error) {
			if len(agent.Candidates) > 1 && al.fallback != nil {
				fbResult, fbErr := al.fallback.Execute(ctx, agent.Candidates,
					func(ctx context.Context, provider, model string) (*providers.LLMResponse, error) {
						return agent.Provider.Chat(ctx, messages, providerToolDefs, model, map[string]any{
							"max_tokens":       agent.MaxTokens,
							"temperature":      agent.Temperature,
							"prompt_cache_key": agent.ID,
						})
					},
				)
				if fbErr != nil {
					return nil, fbErr
				}
				if fbResult.Provider != "" && len(fbResult.Attempts) > 0 {
					logger.InfoCF("agent", fmt.Sprintf("Fallback: succeeded with %s/%s after %d attempts",
						fbResult.Provider, fbResult.Model, len(fbResult.Attempts)+1),
						map[string]any{"agent_id": agent.ID, "iteration": iteration})
				}
				return fbResult.Response, nil
			}
			return agent.Provider.Chat(ctx, messages, providerToolDefs, agent.Model, map[string]any{
				"max_tokens":       agent.MaxTokens,
				"temperature":      agent.Temperature,
				"prompt_cache_key": agent.ID,
			})
		}

		// Retry loop for context/token errors
		maxRetries := 2
		for retry := 0; retry <= maxRetries; retry++ {
			response, err = callLLM()
			if err == nil {
				break
			}

			errMsg := strings.ToLower(err.Error())

			// Check if this is a network/HTTP timeout — not a context window error.
			isTimeoutError := errors.Is(err, context.DeadlineExceeded) ||
				strings.Contains(errMsg, "deadline exceeded") ||
				strings.Contains(errMsg, "client.timeout") ||
				strings.Contains(errMsg, "timed out") ||
				strings.Contains(errMsg, "timeout exceeded")

			// Detect real context window / token limit errors, excluding network timeouts.
			isContextError := !isTimeoutError && (strings.Contains(errMsg, "context_length_exceeded") ||
				strings.Contains(errMsg, "context window") ||
				strings.Contains(errMsg, "maximum context length") ||
				strings.Contains(errMsg, "token limit") ||
				strings.Contains(errMsg, "too many tokens") ||
				strings.Contains(errMsg, "max_tokens") ||
				strings.Contains(errMsg, "invalidparameter") ||
				strings.Contains(errMsg, "prompt is too long") ||
				strings.Contains(errMsg, "request too large"))

			if isTimeoutError && retry < maxRetries {
				backoff := time.Duration(retry+1) * 5 * time.Second
				logger.WarnCF("agent", "Timeout error, retrying after backoff", map[string]any{
					"error":   err.Error(),
					"retry":   retry,
					"backoff": backoff.String(),
				})
				time.Sleep(backoff)
				continue
			}

			if isContextError && retry < maxRetries {
				logger.WarnCF("agent", "Context window error detected, attempting compression", map[string]any{
					"error": err.Error(),
					"retry": retry,
				})

				if retry == 0 && !constants.IsInternalChannel(opts.Channel) {
					al.bus.PublishOutbound(ctx, bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: "Context window exceeded. Compressing history and retrying...",
					})
				}

				al.forceCompression(agent, opts.SessionKey)
				newHistory := agent.Sessions.GetHistory(opts.SessionKey)
				newSummary := agent.Sessions.GetSummary(opts.SessionKey)
				messages = agent.ContextBuilder.BuildMessages(
					newHistory, newSummary, "",
					nil, opts.Channel, opts.ChatID,
				)
				continue
			}
			break
		}

		if err != nil {
			logger.ErrorCF("agent", "LLM call failed",
				map[string]any{
					"agent_id":  agent.ID,
					"iteration": iteration,
					"error":     err.Error(),
				})
			return "", iteration, fmt.Errorf("LLM call failed after retries: %w", err)
		}

		go al.handleReasoning(ctx, response.Reasoning, opts.Channel, al.targetReasoningChannelID(opts.Channel))

		logger.DebugCF("agent", "LLM response",
			map[string]any{
				"agent_id":       agent.ID,
				"iteration":      iteration,
				"content_chars":  len(response.Content),
				"tool_calls":     len(response.ToolCalls),
				"reasoning":      response.Reasoning,
				"target_channel": al.targetReasoningChannelID(opts.Channel),
				"channel":        opts.Channel,
			})
		// Check if no tool calls - we're done
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			logger.InfoCF("agent", "LLM response without tool calls (direct answer)",
				map[string]any{
					"agent_id":      agent.ID,
					"iteration":     iteration,
					"content_chars": len(finalContent),
				})
			break
		}

		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, providers.NormalizeToolCall(tc))
		}

		// Log tool calls
		toolNames := make([]string, 0, len(normalizedToolCalls))
		for _, tc := range normalizedToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF("agent", "LLM requested tool calls",
			map[string]any{
				"agent_id":  agent.ID,
				"tools":     toolNames,
				"count":     len(normalizedToolCalls),
				"iteration": iteration,
			})

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:             "assistant",
			Content:          response.Content,
			ReasoningContent: response.ReasoningContent,
		}
		for _, tc := range normalizedToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			// Copy ExtraContent to ensure thought_signature is persisted for Gemini 3
			extraContent := tc.ExtraContent
			thoughtSignature := ""
			if tc.Function != nil {
				thoughtSignature = tc.Function.ThoughtSignature
			}

			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Name: tc.Name,
				Function: &providers.FunctionCall{
					Name:             tc.Name,
					Arguments:        string(argumentsJSON),
					ThoughtSignature: thoughtSignature,
				},
				ExtraContent:     extraContent,
				ThoughtSignature: thoughtSignature,
			})
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with tool calls to session
		agent.Sessions.AddFullMessage(opts.SessionKey, assistantMsg)

		// Execute tool calls
		for _, tc := range normalizedToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments)
			argsPreview := utils.Truncate(string(argsJSON), 200)
			logger.InfoCF("agent", fmt.Sprintf("Tool call: %s(%s)", tc.Name, argsPreview),
				map[string]any{
					"agent_id":  agent.ID,
					"tool":      tc.Name,
					"iteration": iteration,
				})

			// Create async callback for tools that implement AsyncTool
			// NOTE: Following openclaw's design, async tools do NOT send results directly to users.
			// Instead, they notify the agent via PublishInbound, and the agent decides
			// whether to forward the result to the user (in processSystemMessage).
			asyncCallback := func(callbackCtx context.Context, result *tools.ToolResult) {
				// Log the async completion but don't send directly to user
				// The agent will handle user notification via processSystemMessage
				if !result.Silent && result.ForUser != "" {
					logger.InfoCF("agent", "Async tool completed, agent will handle notification",
						map[string]any{
							"tool":        tc.Name,
							"content_len": len(result.ForUser),
						})
				}
			}

			toolResult := agent.Tools.ExecuteWithContext(
				toolCtx,
				tc.Name,
				tc.Arguments,
				opts.Channel,
				opts.ChatID,
				asyncCallback,
			)

			// Send ForUser content to user immediately if not Silent
			if !toolResult.Silent && toolResult.ForUser != "" && opts.SendResponse {
				al.bus.PublishOutbound(ctx, bus.OutboundMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Content: toolResult.ForUser,
				})
				logger.DebugCF("agent", "Sent tool result to user",
					map[string]any{
						"tool":        tc.Name,
						"content_len": len(toolResult.ForUser),
					})
			}

			// If tool returned media refs, publish them as outbound media
			if len(toolResult.Media) > 0 && opts.SendResponse {
				parts := make([]bus.MediaPart, 0, len(toolResult.Media))
				for _, ref := range toolResult.Media {
					part := bus.MediaPart{Ref: ref}
					// Populate metadata from MediaStore when available
					if al.mediaStore != nil {
						if _, meta, err := al.mediaStore.ResolveWithMeta(ref); err == nil {
							part.Filename = meta.Filename
							part.ContentType = meta.ContentType
							part.Type = inferMediaType(meta.Filename, meta.ContentType)
						}
					}
					parts = append(parts, part)
				}
				al.bus.PublishOutboundMedia(ctx, bus.OutboundMediaMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Parts:   parts,
				})
			}

			// Determine content for LLM based on tool result
			contentForLLM := toolResult.ForLLM
			if contentForLLM == "" && toolResult.Err != nil {
				contentForLLM = toolResult.Err.Error()
			}

			toolResultMsg := providers.Message{
				Role:       "tool",
				Content:    contentForLLM,
				ToolCallID: tc.ID,
			}
			messages = append(messages, toolResultMsg)

			// Save tool result message to session
			agent.Sessions.AddFullMessage(opts.SessionKey, toolResultMsg)
		}

		// Drain completed subagent announcements into the conversation
		if al.announcer != nil {
			for _, ann := range al.announcer.Drain(opts.SessionKey) {
				messages = append(messages, providers.Message{
					Role:    "user",
					Content: fmt.Sprintf("[Subagent completed] %s", ann.Content),
				})
			}
		}
	}

	return finalContent, iteration, nil
}

// updateToolContexts updates per-request context for tools that need it:
// channel/chatID (ContextualTool), sessionKey (SessionKeyAware), board (BoardAware).
func (al *AgentLoop) updateToolContexts(agent *AgentInstance, channel, chatID, sessionKey string) {
	var board *multiagent.Blackboard
	if al.blackboards != nil && sessionKey != "" {
		board = al.blackboards.Get(strings.TrimSpace(sessionKey))
	}

	for _, name := range agent.Tools.List() {
		tool, ok := agent.Tools.Get(name)
		if !ok {
			continue
		}
		if ct, ok := tool.(tools.ContextualTool); ok {
			ct.SetContext(channel, chatID)
		}
		if sk, ok := tool.(multiagent.SessionKeyAware); ok {
			sk.SetSessionKey(sessionKey)
		}
		if ba, ok := tool.(multiagent.BoardAware); ok && board != nil {
			ba.SetBoard(board)
		}
	}
}

func (al *AgentLoop) toolContext(ctx context.Context, sessionKey string, ephemeral bool) context.Context {
	if ephemeral {
		return multiagent.WithBlackboard(ctx, multiagent.NewBlackboard())
	}
	if al.blackboards == nil {
		return ctx
	}
	board := al.blackboards.Get(strings.TrimSpace(sessionKey))
	return multiagent.WithBlackboard(ctx, board)
}

// maybeSummarize triggers summarization if the session history exceeds thresholds.
func (al *AgentLoop) maybeSummarize(agent *AgentInstance, sessionKey, channel, chatID string) {
	newHistory := agent.Sessions.GetHistory(sessionKey)
	tokenEstimate := al.estimateTokens(newHistory)
	threshold := agent.ContextWindow * 75 / 100

	if len(newHistory) > 20 || tokenEstimate > threshold {
		summarizeKey := agent.ID + ":" + sessionKey
		if _, loading := al.summarizing.LoadOrStore(summarizeKey, true); !loading {
			go func() {
				defer al.summarizing.Delete(summarizeKey)
				logger.Debug("Memory threshold reached. Optimizing conversation history...")
				al.summarizeSession(agent, sessionKey)
			}()
		}
	}
}

// forceCompression aggressively reduces context when the limit is hit.
// It drops the oldest 50% of messages (keeping system prompt and last user message).
func (al *AgentLoop) forceCompression(agent *AgentInstance, sessionKey string) {
	history := agent.Sessions.GetHistory(sessionKey)
	if len(history) <= 4 {
		return
	}

	// Keep system prompt (usually [0]) and the very last message (user's trigger)
	// We want to drop the oldest half of the *conversation*
	// Assuming [0] is system, [1:] is conversation
	conversation := history[1 : len(history)-1]
	if len(conversation) == 0 {
		return
	}

	// Helper to find the mid-point of the conversation
	mid := len(conversation) / 2

	// New history structure:
	// 1. System Prompt (with compression note appended)
	// 2. Second half of conversation
	// 3. Last message

	droppedCount := mid
	keptConversation := conversation[mid:]

	newHistory := make([]providers.Message, 0, 1+len(keptConversation)+1)

	// Append compression note to the original system prompt instead of adding a new system message
	// This avoids having two consecutive system messages which some APIs (like Zhipu) reject
	compressionNote := fmt.Sprintf(
		"\n\n[System Note: Emergency compression dropped %d oldest messages due to context limit]",
		droppedCount,
	)
	enhancedSystemPrompt := history[0]
	enhancedSystemPrompt.Content = enhancedSystemPrompt.Content + compressionNote
	newHistory = append(newHistory, enhancedSystemPrompt)

	newHistory = append(newHistory, keptConversation...)
	newHistory = append(newHistory, history[len(history)-1]) // Last message

	// Update session
	agent.Sessions.SetHistory(sessionKey, newHistory)
	agent.Sessions.Save(sessionKey)

	logger.WarnCF("agent", "Forced compression executed", map[string]any{
		"session_key":  sessionKey,
		"dropped_msgs": droppedCount,
		"new_count":    len(newHistory),
	})
}

// GetStartupInfo returns information about loaded tools and skills for logging.
func (al *AgentLoop) GetStartupInfo() map[string]any {
	info := make(map[string]any)

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		return info
	}

	// Tools info
	toolsList := agent.Tools.List()
	info["tools"] = map[string]any{
		"count": len(toolsList),
		"names": toolsList,
	}

	// Skills info
	info["skills"] = agent.ContextBuilder.GetSkillsInfo()

	// Agents info
	info["agents"] = map[string]any{
		"count": len(al.registry.ListAgentIDs()),
		"ids":   al.registry.ListAgentIDs(),
	}

	return info
}

// formatMessagesForLog formats messages for logging
func formatMessagesForLog(messages []providers.Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, msg := range messages {
		fmt.Fprintf(&sb, "  [%d] Role: %s\n", i, msg.Role)
		if len(msg.ToolCalls) > 0 {
			sb.WriteString("  ToolCalls:\n")
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&sb, "    - ID: %s, Type: %s, Name: %s\n", tc.ID, tc.Type, tc.Name)
				if tc.Function != nil {
					fmt.Fprintf(&sb, "      Arguments: %s\n", utils.Truncate(tc.Function.Arguments, 200))
				}
			}
		}
		if msg.Content != "" {
			content := utils.Truncate(msg.Content, 200)
			fmt.Fprintf(&sb, "  Content: %s\n", content)
		}
		if msg.ToolCallID != "" {
			fmt.Fprintf(&sb, "  ToolCallID: %s\n", msg.ToolCallID)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("]")
	return sb.String()
}

// formatToolsForLog formats tool definitions for logging
func formatToolsForLog(toolDefs []providers.ToolDefinition) string {
	if len(toolDefs) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, tool := range toolDefs {
		fmt.Fprintf(&sb, "  [%d] Type: %s, Name: %s\n", i, tool.Type, tool.Function.Name)
		fmt.Fprintf(&sb, "      Description: %s\n", tool.Function.Description)
		if len(tool.Function.Parameters) > 0 {
			fmt.Fprintf(&sb, "      Parameters: %s\n", utils.Truncate(fmt.Sprintf("%v", tool.Function.Parameters), 200))
		}
	}
	sb.WriteString("]")
	return sb.String()
}

// summarizeSession summarizes the conversation history for a session.
func (al *AgentLoop) summarizeSession(agent *AgentInstance, sessionKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	history := agent.Sessions.GetHistory(sessionKey)
	summary := agent.Sessions.GetSummary(sessionKey)

	// Keep last 4 messages for continuity
	if len(history) <= 4 {
		return
	}

	toSummarize := history[:len(history)-4]

	// Oversized Message Guard
	maxMessageTokens := agent.ContextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		msgTokens := len(m.Content) / 2
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	// Multi-Part Summarization
	var finalSummary string
	if len(validMessages) > 10 {
		mid := len(validMessages) / 2
		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := al.summarizeBatch(ctx, agent, part1, "")
		s2, _ := al.summarizeBatch(ctx, agent, part2, "")

		mergePrompt := fmt.Sprintf(
			"Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s",
			s1,
			s2,
		)
		resp, err := agent.Provider.Chat(
			ctx,
			[]providers.Message{{Role: "user", Content: mergePrompt}},
			nil,
			agent.Model,
			map[string]any{
				"max_tokens":       1024,
				"temperature":      0.3,
				"prompt_cache_key": agent.ID,
			},
		)
		if err == nil {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = al.summarizeBatch(ctx, agent, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		agent.Sessions.SetSummary(sessionKey, finalSummary)
		agent.Sessions.TruncateHistory(sessionKey, 4)
		agent.Sessions.Save(sessionKey)
	}
}

// summarizeBatch summarizes a batch of messages.
func (al *AgentLoop) summarizeBatch(
	ctx context.Context,
	agent *AgentInstance,
	batch []providers.Message,
	existingSummary string,
) (string, error) {
	var sb strings.Builder
	sb.WriteString("Provide a concise summary of this conversation segment, preserving core context and key points.\n")
	if existingSummary != "" {
		sb.WriteString("Existing context: ")
		sb.WriteString(existingSummary)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCONVERSATION:\n")
	for _, m := range batch {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}
	prompt := sb.String()

	response, err := agent.Provider.Chat(
		ctx,
		[]providers.Message{{Role: "user", Content: prompt}},
		nil,
		agent.Model,
		map[string]any{
			"max_tokens":       1024,
			"temperature":      0.3,
			"prompt_cache_key": agent.ID,
		},
	)
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// estimateTokens estimates the number of tokens in a message list.
// Uses a safe heuristic of 2.5 characters per token to account for CJK and other
// overheads better than the previous 3 chars/token.
func (al *AgentLoop) estimateTokens(messages []providers.Message) int {
	totalChars := 0
	for _, m := range messages {
		totalChars += utf8.RuneCountInString(m.Content)
	}
	// 2.5 chars per token = totalChars * 2 / 5
	return totalChars * 2 / 5
}

func (al *AgentLoop) handleCommand(ctx context.Context, msg bus.InboundMessage) (string, bool) {
	content := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	parts := strings.Fields(content)
	if len(parts) == 0 {
		return "", false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/show":
		if len(args) < 1 {
			return "Usage: /show [model|channel|agents]", true
		}
		switch args[0] {
		case "model":
			defaultAgent := al.registry.GetDefaultAgent()
			if defaultAgent == nil {
				return "No default agent configured", true
			}
			return fmt.Sprintf("Current model: %s", defaultAgent.Model), true
		case "channel":
			return fmt.Sprintf("Current channel: %s", msg.Channel), true
		case "agents":
			agentIDs := al.registry.ListAgentIDs()
			return fmt.Sprintf("Registered agents: %s", strings.Join(agentIDs, ", ")), true
		default:
			return fmt.Sprintf("Unknown show target: %s", args[0]), true
		}

	case "/list":
		if len(args) < 1 {
			return "Usage: /list [models|channels|agents]", true
		}
		switch args[0] {
		case "models":
			return "Available models: configured in config.json per agent", true
		case "channels":
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			channels := al.channelManager.GetEnabledChannels()
			if len(channels) == 0 {
				return "No channels enabled", true
			}
			return fmt.Sprintf("Enabled channels: %s", strings.Join(channels, ", ")), true
		case "agents":
			agentIDs := al.registry.ListAgentIDs()
			return fmt.Sprintf("Registered agents: %s", strings.Join(agentIDs, ", ")), true
		default:
			return fmt.Sprintf("Unknown list target: %s", args[0]), true
		}

	case "/switch":
		if len(args) < 3 || args[1] != "to" {
			return "Usage: /switch [model|channel] to <name>", true
		}
		target := args[0]
		value := args[2]

		switch target {
		case "model":
			defaultAgent := al.registry.GetDefaultAgent()
			if defaultAgent == nil {
				return "No default agent configured", true
			}
			oldModel := defaultAgent.Model
			defaultAgent.Model = value
			return fmt.Sprintf("Switched model from %s to %s", oldModel, value), true
		case "channel":
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			if _, exists := al.channelManager.GetChannel(value); !exists && value != "cli" {
				return fmt.Sprintf("Channel '%s' not found or not enabled", value), true
			}
			return fmt.Sprintf("Switched target channel to %s", value), true
		default:
			return fmt.Sprintf("Unknown switch target: %s", target), true
		}
	}

	return "", false
}

// extractPeer extracts the routing peer from the inbound message's structured Peer field.
func extractPeer(msg bus.InboundMessage) *routing.RoutePeer {
	if msg.Peer.Kind == "" {
		return nil
	}
	peerID := msg.Peer.ID
	if peerID == "" {
		if msg.Peer.Kind == "direct" {
			peerID = msg.SenderID
		} else {
			peerID = msg.ChatID
		}
	}
	return &routing.RoutePeer{Kind: msg.Peer.Kind, ID: peerID}
}

// buildCLIMCPProviderConfigs extracts stdio MCP server entries for CLI provider passthrough.
func buildCLIMCPProviderConfigs(entries []config.MCPClientEntry) []providers.MCPProviderConfig {
	var configs []providers.MCPProviderConfig
	for _, e := range entries {
		if e.Transport != "stdio" {
			continue
		}
		configs = append(configs, providers.MCPProviderConfig{
			Name:    e.Name,
			Command: e.Command,
			Args:    e.Args,
			Env:     e.Env,
		})
	}
	return configs
}

// configToMCPClientConfigs converts config.MCPClientEntry to mcp.MCPClientConfig.
func configToMCPClientConfigs(entries []config.MCPClientEntry) []mcptypes.MCPClientConfig {
	configs := make([]mcptypes.MCPClientConfig, len(entries))
	for i, e := range entries {
		configs[i] = mcptypes.MCPClientConfig{
			Name:      e.Name,
			Transport: e.Transport,
			Command:   e.Command,
			Args:      e.Args,
			URL:       e.URL,
			Env:       e.Env,
			Agents:    e.Agents,
		}
	}
	return configs
}

// extractParentPeer extracts the parent peer (reply-to) from inbound message metadata.
func extractParentPeer(msg bus.InboundMessage) *routing.RoutePeer {
	parentKind := msg.Metadata["parent_peer_kind"]
	parentID := msg.Metadata["parent_peer_id"]
	if parentKind == "" || parentID == "" {
		return nil
	}
	return &routing.RoutePeer{Kind: parentKind, ID: parentID}
}
