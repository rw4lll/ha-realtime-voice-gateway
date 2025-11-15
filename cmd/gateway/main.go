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
	"github.com/rw4lll/ha-realtime-voice-gateway/internal/protocol/wyoming"
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
		zap.String("wyoming_addr", cfg.Wyoming.Address),
		zap.Bool("ha_enabled", cfg.HasHomeAssistant()))

	// Initialize Home Assistant integration (if configured)
	var toolExecutor pipeline.ToolExecutor
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

		// Autodiscovery (always enabled, just configure filtering)
		logger.Info("Starting Home Assistant autodiscovery",
			zap.Int("allowed_domains", len(cfg.HomeAssistant.AutoDiscoveryDomains)),
			zap.Int("denied_domains", len(cfg.HomeAssistant.AutoDiscoveryDenied)))

		// Create autodiscovery instance
		autoDiscovery, err := ha.NewAutoDiscovery(ha.AutoDiscoveryConfig{
			Client:            haClient,
			AllowedDomains:    cfg.HomeAssistant.AutoDiscoveryDomains,
			DeniedDomains:     cfg.HomeAssistant.AutoDiscoveryDenied,
			AllowedServices:   cfg.HomeAssistant.AllowList, // Service patterns filter
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

		// Create tool executor with allow list from config
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
			zap.Int("allowed_services", len(toolExecutor.(*ha.Executor).GetAllowList())))
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
			SystemPrompt: cfg.Session.SystemPrompt,
			Extra:        extra,
		}); err != nil {
			logger.Fatal("Failed to initialize Gemini backend", zap.Error(err))
		}

		logger.Info("Gemini backend initialized successfully")
		llmBackend = geminiBackend

	case "openai":
		logger.Fatal("OpenAI backend not yet implemented",
			zap.String("note", "Coming in Phase 2"))

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

	// Create pipeline with autodiscovered system prompt
	pipe, err := pipeline.NewPipeline(pipeline.Config{
		Backend:                llmBackend,
		Logger:                 logger,
		SystemPrompt:           systemPrompt, // Use autodiscovered prompt if available
		ToolExecutor:           toolExecutor,
		AudioStartBufferMs:     cfg.Performance.AudioStartBufferMs,
		EnableMetrics:          cfg.Performance.EnableMetrics,
		SessionMode:            cfg.Session.Mode,
		AutoCloseAfterResponse: cfg.Session.AutoCloseAfterResponse,
		SessionResponseTimeout: time.Duration(cfg.Session.ResponseTimeoutMs) * time.Millisecond,
	})
	if err != nil {
		logger.Fatal("Failed to create pipeline", zap.Error(err))
	}
	defer pipe.Close()

	logger.Info("Pipeline initialized",
		zap.Bool("ha_enabled", toolExecutor != nil),
		zap.Int("audio_buffer", cfg.Performance.AudioBufferSize),
		zap.Int("event_buffer", cfg.Performance.EventBufferSize),
		zap.Int("audio_start_buffer_ms", cfg.Performance.AudioStartBufferMs),
		zap.Bool("metrics_enabled", cfg.Performance.EnableMetrics),
		zap.String("session_mode", cfg.Session.Mode),
		zap.Bool("auto_close", cfg.Session.AutoCloseAfterResponse))

	// Create Wyoming server
	sttServer := wyoming.NewServer(wyoming.ServerConfig{
		Address: cfg.Wyoming.Address,
		Logger:  logger.With(zap.String("server", "Wyoming")),
		OnSession: func(s *wyoming.Session) {
			logger.Info("New Wyoming session connected",
				zap.String("session_id", s.ID))
			// Pass session to pipeline for handling
			go pipe.HandleWyomingSession(s)
		},
	})

	if err := sttServer.Start(); err != nil {
		logger.Fatal("Failed to start Wyoming server", zap.Error(err))
	}
	defer sttServer.Close()

	// Log actual backend capabilities (not config file values)
	backendCaps := llmBackend.Capabilities()
	outputFormat := backendCaps.SupportedAudioFormats[0] // Primary format

	logger.Info("Gateway ready",
		zap.String("wyoming_addr", sttServer.Addr()),
		zap.Int("active_sessions", pipe.GetSessionCount()),
		zap.String("backend", cfg.Backend.Type),
		zap.Int("backend_output_rate", outputFormat.SampleRate),
		zap.Int("backend_output_channels", outputFormat.Channels),
		zap.Int("backend_output_bits", outputFormat.BitsPerSample))

	// Start periodic metrics logger if metrics enabled
	var metricsTicker *time.Ticker
	var metricsStop chan struct{}
	if cfg.Performance.EnableMetrics {
		metricsTicker = time.NewTicker(60 * time.Second) // Log metrics every minute
		metricsStop = make(chan struct{})
		go func() {
			for {
				select {
				case <-metricsTicker.C:
					pipe.LogMetrics()
				case <-metricsStop:
					return
				}
			}
		}()
		logger.Info("Periodic metrics logging enabled (every 60 seconds)")
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	logger.Info("Shutting down gracefully...")

	// Stop metrics logger if running
	if metricsTicker != nil {
		metricsTicker.Stop()
		close(metricsStop)
	}
}
