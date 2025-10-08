package consul

import (
	"encoding/json"
	"fmt"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"nomad-mcp-builder/pkg/types"
)

// Client wraps Consul API client for job watching
type Client struct {
	client *consulapi.Client
}

// NewClient creates a new Consul client
// If consulAddr is empty, uses default Consul configuration (localhost:8500)
func NewClient(consulAddr string) (*Client, error) {
	config := consulapi.DefaultConfig()
	if consulAddr != "" {
		config.Address = consulAddr
	}

	client, err := consulapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	return &Client{client: client}, nil
}

// JobUpdate represents a job status update from Consul KV
type JobUpdate struct {
	JobID     string
	Status    types.JobStatus
	Phase     string
	Timestamp time.Time
	Error     string
}

// WatchJob watches a job's status in Consul KV and sends updates to the channel
// Returns when job reaches terminal state (SUCCEEDED or FAILED) or context is cancelled
func (c *Client) WatchJob(jobID string, updates chan<- JobUpdate, errors chan<- error) {
	defer close(updates)
	defer close(errors)

	// Watch the active job, not history (history is only written when job completes)
	key := fmt.Sprintf("nomad-build-service/jobs/%s", jobID)
	kv := c.client.KV()

	var lastIndex uint64

	for {
		// Use blocking query with 60s timeout
		opts := &consulapi.QueryOptions{
			WaitIndex: lastIndex,
			WaitTime:  60 * time.Second,
		}

		pair, meta, err := kv.Get(key, opts)
		if err != nil {
			errors <- fmt.Errorf("failed to query Consul KV: %w", err)
			time.Sleep(5 * time.Second) // Back off before retry
			continue
		}

		// Update index for next blocking query
		lastIndex = meta.LastIndex

		// Key doesn't exist yet
		if pair == nil {
			continue
		}

		// Parse job status from KV value
		var job types.Job
		if err := json.Unmarshal(pair.Value, &job); err != nil {
			errors <- fmt.Errorf("failed to parse job data: %w", err)
			continue
		}

		// Send update
		update := JobUpdate{
			JobID:     jobID,
			Status:    job.Status,
			Phase:     job.CurrentPhase,
			Timestamp: job.UpdatedAt,
			Error:     job.Error,
		}

		select {
		case updates <- update:
		default:
			// Channel full, skip update
		}

		// Exit if job reached terminal state
		if job.Status == types.StatusSucceeded || job.Status == types.StatusFailed {
			return
		}
	}
}

// GetConsulAddress discovers Consul address from environment or uses default
func GetConsulAddress() string {
	// Check environment variable
	if addr := consulapi.DefaultConfig().Address; addr != "" {
		return addr
	}
	return "localhost:8500"
}
