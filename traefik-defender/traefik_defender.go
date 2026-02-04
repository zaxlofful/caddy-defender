// Package traefik_defender implements a Traefik middleware plugin that blocks or manipulates
// requests based on the client's IP address, particularly useful for preventing unwanted
// AI scraping traffic or polluting AI training data.
package traefik_defender

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
	"pkg.jsn.cam/caddy-defender/matchers/ip"
	"pkg.jsn.cam/caddy-defender/responders"
	"pkg.jsn.cam/caddy-defender/responders/tarpit"
)

// Config holds the plugin configuration.
type Config struct {
	// IPRanges specifies IP ranges to block, which can be either:
	// - CIDR notations (e.g., "192.168.1.0/24")
	// - Predefined service keys (e.g., "openai", "aws", "gcloud", "azurepubliccloud", "deepseek", "githubcopilot")
	// Default: ["aws", "gcloud", "azurepubliccloud", "openai", "deepseek", "githubcopilot"]
	IPRanges []string `json:"ipRanges,omitempty" yaml:"ipRanges,omitempty"`

	// Responder defines the response strategy for blocked requests.
	// Supported values: "block", "custom", "drop", "garbage", "redirect", "tarpit"
	// Default: "block"
	Responder string `json:"responder,omitempty" yaml:"responder,omitempty"`

	// Message specifies the custom response message for 'custom' responder type.
	// Required when using 'custom' responder.
	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	// StatusCode specifies the HTTP status code for 'custom' responder type.
	// Optional. Default: 200
	StatusCode int `json:"statusCode,omitempty" yaml:"statusCode,omitempty"`

	// URL specifies the custom URL to redirect clients to for 'redirect' responder type.
	// Required when using 'redirect' responder.
	URL string `json:"url,omitempty" yaml:"url,omitempty"`

	// Whitelist specifies IP addresses to exclude from blocking.
	// NOTE: this only supports exact IP addresses, not ranges.
	// Default: []
	Whitelist []string `json:"whitelist,omitempty" yaml:"whitelist,omitempty"`

	// TarpitConfig provides optional configuration for the 'tarpit' responder.
	// Only used when Responder is "tarpit".
	TarpitConfig *TarpitConfig `json:"tarpitConfig,omitempty" yaml:"tarpitConfig,omitempty"`

	// ServeIgnore specifies whether to serve a robots.txt file with a "Disallow: /" directive
	// Default: false
	ServeIgnore bool `json:"serveIgnore,omitempty" yaml:"serveIgnore,omitempty"`
}

// TarpitConfig holds configuration for the tarpit responder.
type TarpitConfig struct {
	// Timeout specifies how long to keep the connection open (e.g., "30s")
	// Default: 30s
	Timeout string `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// BytesPerSecond specifies the rate at which to stream data
	// Default: 24
	BytesPerSecond int `json:"bytesPerSecond,omitempty" yaml:"bytesPerSecond,omitempty"`

	// ResponseCode specifies the HTTP status code to return
	// Default: 200
	ResponseCode int `json:"responseCode,omitempty" yaml:"responseCode,omitempty"`

	// Headers specifies custom HTTP headers to include in the response
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`

	// Content specifies the content source for tarpit (e.g., "file:///path/to/file" or "http://example.com")
	Content string `json:"content,omitempty" yaml:"content,omitempty"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		IPRanges:  []string{"aws", "gcloud", "azurepubliccloud", "openai", "deepseek", "githubcopilot"},
		Responder: "block",
	}
}

// TraefikDefender is the middleware handler.
type TraefikDefender struct {
	next        http.Handler
	name        string
	ipChecker   *ip.IPChecker
	responder   responders.Responder
	log         *zap.Logger
	config      *Config
	serveIgnore bool
}

// New creates a new instance of the Traefik Defender middleware.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// Set up logger
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	// Validate config
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Use default ranges if none specified
	if len(config.IPRanges) == 0 {
		config.IPRanges = []string{"aws", "gcloud", "azurepubliccloud", "openai", "deepseek", "githubcopilot"}
	}

	// Create IP checker
	ipChecker := ip.NewIPChecker(config.IPRanges, config.Whitelist, logger)

	// Create responder based on config
	responder, err := createResponder(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create responder: %w", err)
	}

	return &TraefikDefender{
		next:        next,
		name:        name,
		ipChecker:   ipChecker,
		responder:   responder,
		log:         logger,
		config:      config,
		serveIgnore: config.ServeIgnore,
	}, nil
}

// ServeHTTP implements the http.Handler interface.
func (td *TraefikDefender) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle robots.txt if enabled
	if td.serveIgnore && r.URL.Path == "/robots.txt" && r.Method == http.MethodGet {
		td.serveRobotsTxt(w)
		return
	}

	// Extract client IP
	clientIP, err := getClientIP(r)
	if err != nil {
		td.log.Error("Failed to parse client IP", zap.Error(err))
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	td.log.Debug("Processing request", zap.String("ip", clientIP.String()))

	// Check if request should be allowed
	if td.ipChecker.ReqAllowed(r.Context(), clientIP) {
		td.log.Debug("Request allowed", zap.String("ip", clientIP.String()))
		td.next.ServeHTTP(w, r)
		return
	}

	// Request is blocked - use responder
	td.log.Debug("Request blocked", zap.String("ip", clientIP.String()))
	
	// Traefik responders need to be adapted since they don't use caddyhttp.Handler
	// We'll create a simple adapter
	if err := td.responder.ServeHTTP(w, r, nil); err != nil {
		td.log.Error("Responder error", zap.Error(err))
	}
}

// serveRobotsTxt serves a robots.txt file that blocks most user agents.
func (td *TraefikDefender) serveRobotsTxt(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	robotsTxt := `
User-agent: Googlebot
Disallow:

User-agent: Bingbot
Disallow:

User-agent: DuckDuckBot
Disallow:

User-agent: *
Disallow: /
`
	_, _ = w.Write([]byte(robotsTxt))
}

// getClientIP extracts the client IP address from the request.
func getClientIP(r *http.Request) (net.IP, error) {
	// Try X-Forwarded-For first (common in reverse proxy setups)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		ips := parseXFF(xff)
		if len(ips) > 0 {
			if ip := net.ParseIP(ips[0]); ip != nil {
				return ip, nil
			}
		}
	}

	// Try X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return ip, nil
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid remote address: %w", err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse IP: %s", host)
	}

	return ip, nil
}

// parseXFF parses the X-Forwarded-For header.
func parseXFF(xff string) []string {
	var ips []string
	for i := 0; i < len(xff); {
		// Skip whitespace
		for i < len(xff) && (xff[i] == ' ' || xff[i] == '\t') {
			i++
		}
		if i >= len(xff) {
			break
		}

		// Find end of IP (comma or end of string)
		start := i
		for i < len(xff) && xff[i] != ',' {
			i++
		}

		// Trim whitespace and add IP
		ip := xff[start:i]
		for len(ip) > 0 && (ip[len(ip)-1] == ' ' || ip[len(ip)-1] == '\t') {
			ip = ip[:len(ip)-1]
		}
		if len(ip) > 0 {
			ips = append(ips, ip)
		}

		// Skip comma
		if i < len(xff) && xff[i] == ',' {
			i++
		}
	}
	return ips
}

// validateConfig validates the plugin configuration.
func validateConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}

	// Validate responder type
	validResponders := map[string]bool{
		"block":     true,
		"custom":    true,
		"drop":      true,
		"garbage":   true,
		"redirect":  true,
		"tarpit":    true,
		"ratelimit": true,
	}

	if config.Responder != "" && !validResponders[config.Responder] {
		return fmt.Errorf("invalid responder: %s", config.Responder)
	}

	// Validate responder-specific requirements
	if config.Responder == "redirect" && config.URL == "" {
		return fmt.Errorf("redirect responder requires URL to be set")
	}

	// Validate whitelist IPs
	for _, ipStr := range config.Whitelist {
		if net.ParseIP(ipStr) == nil {
			return fmt.Errorf("invalid whitelist IP: %s", ipStr)
		}
	}

	return nil
}

// createResponder creates the appropriate responder based on configuration.
func createResponder(config *Config) (responders.Responder, error) {
	switch config.Responder {
	case "", "block":
		return &responders.BlockResponder{}, nil

	case "custom":
		statusCode := config.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		return &responders.CustomResponder{
			Message:    config.Message,
			StatusCode: statusCode,
		}, nil

	case "drop":
		return &responders.DropResponder{}, nil

	case "garbage":
		return &responders.GarbageResponder{}, nil

	case "ratelimit":
		return &responders.RateLimitResponder{}, nil

	case "redirect":
		return &responders.RedirectResponder{
			URL: config.URL,
		}, nil

	case "tarpit":
		tarpitConfig, err := buildTarpitConfig(config.TarpitConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build tarpit config: %w", err)
		}
		responder := &tarpit.Responder{
			Config: tarpitConfig,
		}
		if err := responder.ConfigureContentReader(); err != nil {
			return nil, fmt.Errorf("failed to configure tarpit content reader: %w", err)
		}
		return responder, nil

	default:
		return nil, fmt.Errorf("unknown responder: %s", config.Responder)
	}
}

// buildTarpitConfig converts the Traefik config to internal tarpit config.
func buildTarpitConfig(tc *TarpitConfig) (*tarpit.Config, error) {
	config := &tarpit.Config{
		Timeout:        30 * time.Second,
		BytesPerSecond: 24,
		ResponseCode:   http.StatusOK,
		Headers:        make(map[string]string),
	}

	if tc == nil {
		return config, nil
	}

	// Parse timeout
	if tc.Timeout != "" {
		// Simple duration parser for common formats
		timeout, err := parseDuration(tc.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
		config.Timeout = time.Duration(timeout)
	}

	// Set bytes per second
	if tc.BytesPerSecond > 0 {
		config.BytesPerSecond = tc.BytesPerSecond
	}

	// Set response code
	if tc.ResponseCode > 0 {
		config.ResponseCode = tc.ResponseCode
	}

	// Set headers
	if tc.Headers != nil {
		config.Headers = tc.Headers
	}

	// Parse content source
	if tc.Content != "" {
		content, err := parseContent(tc.Content)
		if err != nil {
			return nil, fmt.Errorf("invalid content: %w", err)
		}
		config.Content = content
	}

	return config, nil
}

// parseDuration is a simple duration parser for common formats.
func parseDuration(s string) (int64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty duration")
	}

	// Extract numeric part and unit
	var num int64
	var unit string
	var i int

	for i = 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		num = num*10 + int64(s[i]-'0')
	}

	if i == 0 {
		return 0, fmt.Errorf("no numeric value")
	}

	unit = s[i:]

	// Convert to nanoseconds
	switch unit {
	case "ns":
		return num, nil
	case "us", "µs":
		return num * 1000, nil
	case "ms":
		return num * 1000000, nil
	case "s":
		return num * 1000000000, nil
	case "m":
		return num * 60 * 1000000000, nil
	case "h":
		return num * 60 * 60 * 1000000000, nil
	default:
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}
}

// parseContent parses the content source string (e.g., "file:///path" or "http://url").
func parseContent(s string) (tarpit.Content, error) {
	// Split on ://
	parts := []string{}
	colonIdx := -1
	for i := 0; i < len(s)-2; i++ {
		if s[i] == ':' && s[i+1] == '/' && s[i+2] == '/' {
			colonIdx = i
			break
		}
	}

	if colonIdx == -1 {
		return tarpit.Content{}, fmt.Errorf("invalid content format, expected protocol://path")
	}

	parts = append(parts, s[:colonIdx])
	parts = append(parts, s[colonIdx+3:])

	if len(parts) != 2 {
		return tarpit.Content{}, fmt.Errorf("invalid content format, expected protocol://path")
	}

	return tarpit.Content{
		Protocol: parts[0],
		Path:     parts[1],
	}, nil
}
