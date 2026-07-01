// Command deckhouse-harness runs the Deckhouse MCP server.
//
// It supports two transports:
//   - stdio (default): newline-delimited JSON on stdin/stdout. Used by local
//     MCP clients such as Claude Desktop, Cursor, etc.
//   - SSE (HTTP): when LISTEN_ADDR is set or -listen flag is provided, serves
//     using the MCP SSE transport (mcp.NewSSEHandler) for remote / container
//     deployments.
//
// Kubernetes auth: in-cluster config (inside Pod) or ~/.kube/config / KUBECONFIG.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urfave/cli/v3"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/easyp-tech/deckhouse-harness/internal/handler"
	"github.com/easyp-tech/deckhouse-harness/internal/k8s"
	pb "github.com/easyp-tech/deckhouse-harness/proto/deckhouse/v1"
)

const (
	serverImplName        = "deckhouse-harness"
	serverImplVersion     = "0.3.1"
	serverImplDescription = "Deckhouse Kubernetes Platform MCP server"

	defaultListenAddr = ":8080"
	shutdownTimeout   = 10 * time.Second
	readHeaderTimeout = 5 * time.Second
)

func main() {
	cmd := &cli.Command{
		Name:    "deckhouse-harness",
		Version: serverImplVersion,
		Usage:   "Deckhouse Kubernetes Platform MCP server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "listen",
				Usage:   "Listen address for SSE HTTP transport (e.g. ':8080' or '0.0.0.0:8080'). If set, enables SSE mode.",
				Sources: cli.EnvVars("LISTEN_ADDR"),
			},
			&cli.StringFlag{
				Name:    "transport",
				Usage:   "Transport mode: 'stdio' or 'sse'. If empty, auto-selects based on presence of listen address.",
				Sources: cli.EnvVars("TRANSPORT", "MCP_TRANSPORT"),
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return run(c)
		},
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatalf("deckhouse-harness: %v", err)
	}
}

func run(c *cli.Command) error {
	// Values are already resolved by urfave/cli: CLI flag > env (Sources) > default.
	// This replaces the previous manual flag + os.Getenv fallback logic.
	listenAddr := c.String("listen")
	trans := strings.ToLower(strings.TrimSpace(c.String("transport")))

	useSSE := false
	switch trans {
	case "sse":
		useSSE = true
	case "stdio":
		useSSE = false
	default:
		useSSE = listenAddr != ""
	}

	logger := configureLogger()

	cfg, err := loadKubeConfig()
	if err != nil {
		return fmt.Errorf("loading kube config: %w", err)
	}

	client, err := k8s.New(cfg)
	if err != nil {
		return fmt.Errorf("creating k8s client: %w", err)
	}

	server, err := newServer(client, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if useSSE {
		if listenAddr == "" {
			listenAddr = defaultListenAddr
		}
		return serveSSE(ctx, server, listenAddr, logger)
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

// loadKubeConfig resolves a Kubernetes rest.Config for authenticating with the
// cluster. It tries in-cluster config first (when running inside a Pod), then
// falls back to the kubeconfig at KUBECONFIG or ~/.kube/config for local
// execution.
func loadKubeConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, configOverrides,
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	return cfg, nil
}

// configureLogger builds a *slog.Logger from LOG_LEVEL, LOG_OUTPUT, and LOG_FILE
// environment variables. Logs never go to stdout (that channel is reserved for
// the MCP protocol). Default output is stderr.
func configureLogger() *slog.Logger {
	var level slog.Level
	switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var w io.Writer = os.Stderr
	switch os.Getenv("LOG_OUTPUT") {
	case "file":
		path := os.Getenv("LOG_FILE")
		if path == "" {
			log.Println("LOG_OUTPUT=file but LOG_FILE is empty, falling back to stderr")
		} else {
			f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				log.Printf("failed to open log file %q: %v, falling back to stderr", path, err)
			} else {
				w = f
			}
		}
	case "discard":
		w = io.Discard
	default:
		w = os.Stderr
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	log.SetOutput(w)

	return logger
}

// newServer constructs an MCP server with all generated tool handlers
// registered against the provided Kubernetes client.
func newServer(client k8s.Client, logger *slog.Logger) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    serverImplName,
		Title:   serverImplDescription,
		Version: serverImplVersion,
	}, &mcp.ServerOptions{
		Logger: logger,
	})

	err := pb.RegisterDiagnosticsAPITools(server, handler.NewDiagnosticsHandler(client))
	if err != nil {
		return nil, fmt.Errorf("registering diagnostics tools: %w", err)
	}

	err = pb.RegisterModulesAPITools(server, handler.NewModulesHandler(client))
	if err != nil {
		return nil, fmt.Errorf("registering modules tools: %w", err)
	}

	err = pb.RegisterNodesAPITools(server, handler.NewNodesHandler(client))
	if err != nil {
		return nil, fmt.Errorf("registering nodes tools: %w", err)
	}

	err = pb.RegisterReleasesAPITools(server, handler.NewReleasesHandler(client))
	if err != nil {
		return nil, fmt.Errorf("registering releases tools: %w", err)
	}

	err = pb.RegisterConfigAPITools(server, handler.NewConfigHandler(client))
	if err != nil {
		return nil, fmt.Errorf("registering config tools: %w", err)
	}

	err = pb.RegisterSourcesAPITools(server, handler.NewSourcesHandler(client))
	if err != nil {
		return nil, fmt.Errorf("registering sources tools: %w", err)
	}

	return server, nil
}

// serveSSE runs the MCP server using the SSE transport (HTTP + Server-Sent Events).
// It reuses a single *mcp.Server instance to handle multiple concurrent sessions.
// The server is shut down gracefully when ctx is cancelled.
func serveSSE(ctx context.Context, srv *mcp.Server, addr string, logger *slog.Logger) error {
	handler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server {
		// Return the same server for all connections. The SDK's Server supports
		// multiple concurrent ServerSessions.
		return srv
	}, nil)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// Trigger shutdown when the context is cancelled (e.g. SIGINT/SIGTERM).
	go func() {
		<-ctx.Done()
		logger.Info("shutting down SSE server", "addr", addr)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown error", "error", err)
		}
	}()

	logger.Info("starting MCP SSE server", "addr", addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}
