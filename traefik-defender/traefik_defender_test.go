package traefik_defender

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid default config",
			config: &Config{
				Responder: "block",
			},
			wantErr: false,
		},
		{
			name: "valid custom responder",
			config: &Config{
				Responder:  "custom",
				Message:    "Custom message",
				StatusCode: 403,
			},
			wantErr: false,
		},
		{
			name: "valid redirect responder",
			config: &Config{
				Responder: "redirect",
				URL:       "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "invalid redirect without URL",
			config: &Config{
				Responder: "redirect",
			},
			wantErr: true,
		},
		{
			name: "invalid responder type",
			config: &Config{
				Responder: "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			_, err := New(ctx, next, tt.config, "test")
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateConfig(t *testing.T) {
	config := CreateConfig()
	
	if config == nil {
		t.Fatal("CreateConfig() returned nil")
	}

	if config.Responder != "block" {
		t.Errorf("Expected default responder to be 'block', got '%s'", config.Responder)
	}

	if len(config.IPRanges) == 0 {
		t.Error("Expected default IP ranges to be set")
	}
}

func TestServeHTTP_AllowedIP(t *testing.T) {
	config := &Config{
		IPRanges:  []string{"192.168.0.0/16"}, // Block local network
		Responder: "block",
	}

	ctx := context.Background()
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler, err := New(ctx, next, config, "test")
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Test with an IP that should be allowed (not in 192.168.0.0/16)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Error("Expected next handler to be called for allowed IP")
	}
}

func TestServeHTTP_BlockedIP(t *testing.T) {
	config := &Config{
		IPRanges:  []string{"192.168.0.0/16"},
		Responder: "block",
	}

	ctx := context.Background()
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler, err := New(ctx, next, config, "test")
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Test with an IP in the blocked range
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if nextCalled {
		t.Error("Expected next handler NOT to be called for blocked IP")
	}

	if rr.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", rr.Code)
	}
}

func TestServeHTTP_Whitelist(t *testing.T) {
	config := &Config{
		IPRanges:  []string{"192.168.0.0/16"},
		Whitelist: []string{"192.168.1.1"},
		Responder: "block",
	}

	ctx := context.Background()
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler, err := New(ctx, next, config, "test")
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Test with a whitelisted IP that's in the blocked range
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Error("Expected next handler to be called for whitelisted IP")
	}
}

func TestServeHTTP_RobotsTxt(t *testing.T) {
	config := &Config{
		IPRanges:    []string{"192.168.0.0/16"},
		Responder:   "block",
		ServeIgnore: true,
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler, err := New(ctx, next, config, "test")
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Test robots.txt request
	req := httptest.NewRequest(http.MethodGet, "http://example.com/robots.txt", nil)
	req.RemoteAddr = "192.168.1.1:12345" // Blocked IP
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 for robots.txt, got %d", rr.Code)
	}

	body := rr.Body.String()
	if body == "" {
		t.Error("Expected robots.txt content, got empty body")
	}
}

func TestGetClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	req.RemoteAddr = "10.0.0.1:12345"

	ip, err := getClientIP(req)
	if err != nil {
		t.Fatalf("getClientIP() error = %v", err)
	}

	if ip.String() != "1.2.3.4" {
		t.Errorf("Expected IP 1.2.3.4, got %s", ip.String())
	}
}

func TestGetClientIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("X-Real-IP", "1.2.3.4")
	req.RemoteAddr = "10.0.0.1:12345"

	ip, err := getClientIP(req)
	if err != nil {
		t.Fatalf("getClientIP() error = %v", err)
	}

	if ip.String() != "1.2.3.4" {
		t.Errorf("Expected IP 1.2.3.4, got %s", ip.String())
	}
}

func TestGetClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.RemoteAddr = "1.2.3.4:12345"

	ip, err := getClientIP(req)
	if err != nil {
		t.Fatalf("getClientIP() error = %v", err)
	}

	if ip.String() != "1.2.3.4" {
		t.Errorf("Expected IP 1.2.3.4, got %s", ip.String())
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"seconds", "30s", 30000000000, false},
		{"milliseconds", "500ms", 500000000, false},
		{"minutes", "2m", 120000000000, false},
		{"hours", "1h", 3600000000000, false},
		{"invalid", "30x", 0, true},
		{"empty", "", 0, true},
		{"no number", "s", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseContent(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"file", "file:///path/to/file", false},
		{"http", "http://example.com/data", false},
		{"https", "https://example.com/data", false},
		{"invalid no protocol", "/path/to/file", true},
		{"invalid format", "file/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseContent(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid config",
			config: &Config{
				Responder: "block",
			},
			wantErr: false,
		},
		{
			name: "invalid responder",
			config: &Config{
				Responder: "invalid",
			},
			wantErr: true,
		},
		{
			name: "redirect without URL",
			config: &Config{
				Responder: "redirect",
			},
			wantErr: true,
		},
		{
			name: "valid redirect",
			config: &Config{
				Responder: "redirect",
				URL:       "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "invalid whitelist IP",
			config: &Config{
				Responder: "block",
				Whitelist: []string{"invalid-ip"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
