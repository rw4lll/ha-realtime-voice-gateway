package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/backend/gemini"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/config"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/ha"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/pipeline"
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/websocket"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.LoadFromFile(".env")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Create logger from config
	logger, err := cfg.CreateLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting Home Assistant Realtime Voice Gateway",
		zap.String("backend", cfg.Backend.Type),
		zap.Bool("ha_enabled", cfg.HasHomeAssistant()))

	// Initialize Home Assistant integration (if configured)
	var toolExecutor *ha.Executor
	var discoveredTools []backend.Tool
	var systemPrompt string

	if cfg.HasHomeAssistant() {
		logger.Info("Initializing Home Assistant integration",
			zap.String("ha_url", cfg.HomeAssistant.URL))

		// Create HA client
		haClient, err := ha.NewClient(ha.ClientConfig{
			BaseURL: cfg.HomeAssistant.URL,
			Token:   cfg.HomeAssistant.Token,
			Timeout: cfg.HomeAssistant.Timeout,
			Logger:  logger,
		})
		if err != nil {
			logger.Fatal("Failed to create HA client", zap.Error(err))
		}

		// Test HA connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := haClient.Ping(ctx); err != nil {
			logger.Warn("Failed to ping Home Assistant", zap.Error(err))
		} else {
			logger.Info("Successfully connected to Home Assistant")
		}

		// Autodiscovery
		logger.Info("Starting Home Assistant autodiscovery",
			zap.Int("allowed_domains", len(cfg.HomeAssistant.AutoDiscoveryDomains)),
			zap.Int("denied_domains", len(cfg.HomeAssistant.AutoDiscoveryDenied)))

		// Create autodiscovery instance
		autoDiscovery, err := ha.NewAutoDiscovery(ha.AutoDiscoveryConfig{
			Client:            haClient,
			AllowedDomains:    cfg.HomeAssistant.AutoDiscoveryDomains,
			DeniedDomains:     cfg.HomeAssistant.AutoDiscoveryDenied,
			AllowedServices:   cfg.HomeAssistant.AllowList,
			IncludeAttributes: cfg.HomeAssistant.AutoDiscoveryAttributes,
			Logger:            logger,
		})
		if err != nil {
			logger.Fatal("Failed to create autodiscovery", zap.Error(err))
		}

		// Discover tools
		discoveryCtx, discoveryCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer discoveryCancel()

		discoveredTools, err = autoDiscovery.DiscoverTools(discoveryCtx)
		if err != nil {
			logger.Error("Failed to discover tools, falling back to default tools", zap.Error(err))
			discoveredTools = ha.GetDefaultTools()
		} else {
			logger.Info("Autodiscovery completed",
				zap.Int("tools_discovered", len(discoveredTools)))
		}

		// Generate system prompt with entity list if enabled
		if cfg.HomeAssistant.AutoDiscoveryPrompt {
			promptCtx, promptCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer promptCancel()

			systemPrompt, err = autoDiscovery.GenerateSystemPrompt(promptCtx, cfg.Session.SystemPrompt)
			if err != nil {
				logger.Warn("Failed to generate system prompt with entities", zap.Error(err))
				systemPrompt = cfg.Session.SystemPrompt
			} else {
				logger.Info("Generated system prompt with entity list",
					zap.Int("prompt_length", len(systemPrompt)))
			}
		} else {
			systemPrompt = cfg.Session.SystemPrompt
		}

		// Create tool executor
		toolExecutor, err = ha.NewExecutor(ha.ExecutorConfig{
			Client:    haClient,
			AllowList: cfg.HomeAssistant.AllowList,
			Logger:    logger,
		})
		if err != nil {
			logger.Fatal("Failed to create HA executor", zap.Error(err))
		}

		logger.Info("Home Assistant integration ready",
			zap.Int("tools_available", len(discoveredTools)),
			zap.Int("allowed_services", len(toolExecutor.GetAllowList())))
	} else {
		logger.Warn("Home Assistant not configured. Tool execution will be disabled.")
		systemPrompt = cfg.Session.SystemPrompt
	}

	// Create backend based on config
	var llmBackend backend.Backend
	switch cfg.Backend.Type {
	case "mock":
		logger.Info("Using mock backend for testing")
		llmBackend = backend.NewMockBackend()

	case "gemini":
		logger.Info("Using Gemini Live backend",
			zap.String("model", cfg.Backend.GeminiModel))

		geminiBackend := gemini.NewGeminiBackend(logger)

		extra := map[string]any{
			"connect_timeout": cfg.Backend.GeminiConnectTimeout,
			"receive_timeout": cfg.Backend.GeminiReceiveTimeout,
			"send_timeout":    cfg.Backend.GeminiSendTimeout,
			"max_retries":     cfg.Backend.GeminiMaxRetries,
			"retry_backoff":   cfg.Backend.GeminiRetryBackoff,
			"max_sessions":    cfg.Backend.GeminiMaxSessions,
		}
		if cfg.Backend.GeminiVADDisabled {
			extra["vad_disabled"] = true
		}
		if cfg.Backend.GeminiVADStartSensitivity != "" {
			extra["vad_start_sensitivity"] = cfg.Backend.GeminiVADStartSensitivity
		}
		if cfg.Backend.GeminiVADEndSensitivity != "" {
			extra["vad_end_sensitivity"] = cfg.Backend.GeminiVADEndSensitivity
		}
		if cfg.Backend.GeminiVADPrefixPaddingMs > 0 {
			extra["vad_prefix_padding_ms"] = cfg.Backend.GeminiVADPrefixPaddingMs
		}
		if cfg.Backend.GeminiVADSilenceDurationMs > 0 {
			extra["vad_silence_duration_ms"] = cfg.Backend.GeminiVADSilenceDurationMs
		}
		if cfg.Backend.GeminiActivityHandling != "" {
			extra["activity_handling"] = cfg.Backend.GeminiActivityHandling
		}
		if cfg.Backend.GeminiTurnCoverage != "" {
			extra["turn_coverage"] = cfg.Backend.GeminiTurnCoverage
		}

		// Initialize Gemini
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := geminiBackend.Init(ctx, backend.Config{
			APIKey:       cfg.Backend.GeminiAPIKey,
			Model:        cfg.Backend.GeminiModel,
			SystemPrompt: systemPrompt,
			Extra:        extra,
		}); err != nil {
			logger.Fatal("Failed to initialize Gemini backend", zap.Error(err))
		}

		logger.Info("Gemini backend initialized successfully")
		llmBackend = geminiBackend

	case "openai":
		logger.Fatal("OpenAI backend not yet implemented",
			zap.String("note", "Coming soon"))

	default:
		logger.Fatal("Unknown backend type",
			zap.String("type", cfg.Backend.Type),
			zap.Strings("valid_types", []string{"mock", "gemini", "openai"}))
	}

	// Register tools with backend
	if toolExecutor != nil && len(discoveredTools) > 0 {
		if err := llmBackend.RegisterTools(discoveredTools); err != nil {
			logger.Error("Failed to register tools", zap.Error(err))
		} else {
			logger.Info("Registered HA tools with backend", zap.Int("count", len(discoveredTools)))
		}
	}

	// Create pipeline
	pipe, err := pipeline.NewPipeline(pipeline.Config{
		Backend:        llmBackend,
		Logger:         logger,
		ToolExecutor:   toolExecutor,
		SystemPrompt:   systemPrompt,
		AudioBufferMs:  cfg.Session.AudioBufferMs,
		EnableMetrics:  cfg.Performance.EnableMetrics,
		SafetyTimeout:  cfg.Session.SafetyTimeout,
		SilenceTimeout: cfg.Session.SilenceTimeout,
	})
	if err != nil {
		logger.Fatal("Failed to create pipeline", zap.Error(err))
	}
	defer pipe.Close()

	logger.Info("Pipeline initialized",
		zap.Bool("ha_enabled", toolExecutor != nil),
		zap.Int("tools", len(discoveredTools)),
		zap.Int("audio_buffer_ms", cfg.Session.AudioBufferMs),
		zap.Duration("safety_timeout", cfg.Session.SafetyTimeout))

	// Create WebSocket server
	wsServer := websocket.NewServer(websocket.ServerConfig{
		Address:       cfg.WebSocket.Address,
		Path:          cfg.WebSocket.Path,
		ReadTimeout:   time.Duration(cfg.WebSocket.ReadTimeout) * time.Second,
		WriteTimeout:  time.Duration(cfg.WebSocket.WriteTimeout) * time.Second,
		MaxBufferSize: cfg.WebSocket.MaxBufferSize,
		Logger:        logger.With(zap.String("server", "WebSocket")),
		OnSession: func(s *websocket.Session) {
			logger.Info("New WebSocket session connected",
				zap.String("session_id", s.ID))

			// Handle session in pipeline
			go pipe.HandleWebSocketSession(s)
		},
	})

	if err := wsServer.Start(); err != nil {
		logger.Fatal("Failed to start WebSocket server", zap.Error(err))
	}
	defer wsServer.Close()

	logger.Info("Gateway ready",
		zap.String("websocket_addr", cfg.WebSocket.Address),
		zap.String("websocket_path", cfg.WebSocket.Path),
		zap.Int("active_sessions", pipe.GetSessionCount()),
		zap.String("backend", cfg.Backend.Type))

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("WebSocket server listening. Waiting for device connections...")
	<-sigChan

	logger.Info("Shutting down gracefully...")
}
