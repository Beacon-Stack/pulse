package downloadclient

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"

	db "github.com/beacon-stack/pulse/internal/db/generated"
	"github.com/beacon-stack/pulse/internal/events"
)

// Input is the data needed to create or update a download client.
type Input struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`      // qbittorrent, deluge, transmission, sabnzbd, nzbget
	Protocol  string `json:"protocol"`  // torrent, usenet
	Enabled   bool   `json:"enabled"`
	Priority  int    `json:"priority"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	UseSSL    bool   `json:"use_ssl"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Category  string `json:"category"`
	Directory string `json:"directory"`
	Settings  string `json:"settings"`
}

// DLClientTestResult holds the outcome of a download client connectivity test.
type DLClientTestResult struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	Duration string `json:"duration"`
}

// ServiceNotifier is called to notify services when download clients change.
type ServiceNotifier func(ctx context.Context, protocol string)

// Service manages download client configurations.
type Service struct {
	q        db.Querier
	bus      *events.Bus
	logger   *slog.Logger
	notifier ServiceNotifier
}

// NewService creates a new download client service.
func NewService(q db.Querier, bus *events.Bus, logger *slog.Logger) *Service {
	return &Service{q: q, bus: bus, logger: logger}
}

// SetNotifier sets the function called to notify services when download clients change.
func (s *Service) SetNotifier(fn ServiceNotifier) {
	s.notifier = fn
}

// Create adds a new download client.
func (s *Service) Create(ctx context.Context, input Input) (*db.DownloadClient, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if input.Settings == "" {
		input.Settings = "{}"
	}
	if input.Protocol == "" {
		input.Protocol = inferProtocol(input.Kind)
	}

	row, err := s.q.CreateDownloadClient(ctx, db.CreateDownloadClientParams{
		ID:        uuid.New().String(),
		Name:      input.Name,
		Kind:      input.Kind,
		Protocol:  input.Protocol,
		Enabled:   input.Enabled,
		Priority:  int32(input.Priority),
		Host:      input.Host,
		Port:      int32(input.Port),
		UseSsl:    input.UseSSL,
		Username:  input.Username,
		Password:  input.Password,
		Category:  input.Category,
		Directory: input.Directory,
		Settings:  input.Settings,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("creating download client: %w", err)
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.Type("download_client_created"),
		Data: map[string]any{"id": row.ID, "name": row.Name, "kind": row.Kind},
	})

	// Notify services that support this protocol.
	if s.notifier != nil {
		go s.notifier(context.WithoutCancel(ctx), row.Protocol)
	}

	return &row, nil
}

// Get returns a single download client.
func (s *Service) Get(ctx context.Context, id string) (*db.DownloadClient, error) {
	row, err := s.q.GetDownloadClient(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("download client not found: %w", err)
	}
	return &row, nil
}

// List returns all download clients.
func (s *Service) List(ctx context.Context) ([]db.DownloadClient, error) {
	return s.q.ListDownloadClients(ctx)
}

// Update modifies a download client.
func (s *Service) Update(ctx context.Context, id string, input Input) (*db.DownloadClient, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if input.Settings == "" {
		input.Settings = "{}"
	}
	if input.Protocol == "" {
		input.Protocol = inferProtocol(input.Kind)
	}

	row, err := s.q.UpdateDownloadClient(ctx, db.UpdateDownloadClientParams{
		Name:      input.Name,
		Kind:      input.Kind,
		Protocol:  input.Protocol,
		Enabled:   input.Enabled,
		Priority:  int32(input.Priority),
		Host:      input.Host,
		Port:      int32(input.Port),
		UseSsl:    input.UseSSL,
		Username:  input.Username,
		Password:  input.Password,
		Category:  input.Category,
		Directory: input.Directory,
		Settings:  input.Settings,
		UpdatedAt: now,
		ID:        id,
	})
	if err != nil {
		return nil, fmt.Errorf("updating download client: %w", err)
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.Type("download_client_updated"),
		Data: map[string]any{"id": row.ID, "name": row.Name},
	})

	if s.notifier != nil {
		go s.notifier(context.WithoutCancel(ctx), row.Protocol)
	}

	return &row, nil
}

// Delete removes a download client.
func (s *Service) Delete(ctx context.Context, id string) error {
	dc, err := s.q.GetDownloadClient(ctx, id)
	if err != nil {
		return fmt.Errorf("download client not found: %w", err)
	}
	if err := s.q.DeleteDownloadClient(ctx, id); err != nil {
		return fmt.Errorf("deleting download client: %w", err)
	}

	s.bus.Publish(ctx, events.Event{
		Type: events.Type("download_client_deleted"),
		Data: map[string]any{"id": id, "name": dc.Name},
	})

	if s.notifier != nil {
		go s.notifier(context.WithoutCancel(ctx), dc.Protocol)
	}

	return nil
}

// Test checks connectivity to a download client.
func (s *Service) Test(ctx context.Context, kind, host string, port int, useSSL bool) DLClientTestResult {
	start := time.Now()

	scheme := "http"
	if useSSL {
		scheme = "https"
	}

	// Build a test URL based on the client kind
	var testURL string
	switch kind {
	case "qbittorrent":
		testURL = fmt.Sprintf("%s://%s:%d/api/v2/app/version", scheme, host, port)
	case "deluge":
		testURL = fmt.Sprintf("%s://%s:%d", scheme, host, port)
	case "transmission":
		testURL = fmt.Sprintf("%s://%s:%d/transmission/rpc", scheme, host, port)
	case "sabnzbd":
		testURL = fmt.Sprintf("%s://%s:%d/api", scheme, host, port)
	case "nzbget":
		testURL = fmt.Sprintf("%s://%s:%d", scheme, host, port)
	default:
		testURL = fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}

	// First try a TCP connection to verify the host is reachable
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 5*time.Second)
	if err != nil {
		return DLClientTestResult{
			Success:  false,
			Message:  fmt.Sprintf("Connection refused — %s:%d is not reachable", host, port),
			Duration: since(start),
		}
	}
	conn.Close()

	// Try an HTTP request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(testURL)
	if err != nil {
		return DLClientTestResult{
			Success:  true, // TCP connected but HTTP failed — still reachable
			Message:  fmt.Sprintf("Reachable at %s:%d (HTTP error: %v)", host, port, err),
			Duration: since(start),
		}
	}
	defer resp.Body.Close()

	return DLClientTestResult{
		Success:  true,
		Message:  fmt.Sprintf("Connected to %s at %s:%d (HTTP %d)", kind, host, port, resp.StatusCode),
		Duration: since(start),
	}
}

func since(start time.Time) string {
	return time.Since(start).Truncate(time.Millisecond).String()
}

func inferProtocol(kind string) string {
	switch kind {
	case "sabnzbd", "nzbget":
		return "usenet"
	default:
		return "torrent"
	}
}
