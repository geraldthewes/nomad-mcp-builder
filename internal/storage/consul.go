package storage

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/sirupsen/logrus"

	"nomad-mcp-builder/pkg/types"
)

// ConsulStorage implements job storage using Consul KV
type ConsulStorage struct {
	client    *consulapi.Client
	keyPrefix string
	logger    *logrus.Logger
}

// NewConsulStorage creates a new Consul-based storage backend
func NewConsulStorage(address, token, datacenter, keyPrefix string) (*ConsulStorage, error) {
	config := consulapi.DefaultConfig()
	config.Address = address
	config.Token = token
	config.Datacenter = datacenter
	
	client, err := consulapi.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}
	
	return &ConsulStorage{
		client:    client,
		keyPrefix: keyPrefix,
		logger:    logrus.New(),
	}, nil
}

// StoreJob stores a job in Consul KV
func (cs *ConsulStorage) StoreJob(job *types.Job) error {
	key := fmt.Sprintf("%s/jobs/%s", cs.keyPrefix, job.ID)
	
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}
	
	pair := &consulapi.KVPair{
		Key:   key,
		Value: data,
	}
	
	_, err = cs.client.KV().Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to store job in Consul: %w", err)
	}
	
	cs.logger.WithField("job_id", job.ID).Debug("Job stored in Consul")
	return nil
}

// GetJob retrieves a job from Consul KV
func (cs *ConsulStorage) GetJob(jobID string) (*types.Job, error) {
	key := fmt.Sprintf("%s/jobs/%s", cs.keyPrefix, jobID)
	
	pair, _, err := cs.client.KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get job from Consul: %w", err)
	}
	
	if pair == nil {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}
	
	var job types.Job
	if err := json.Unmarshal(pair.Value, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}
	
	return &job, nil
}

// UpdateJob updates an existing job in Consul KV
func (cs *ConsulStorage) UpdateJob(job *types.Job) error {
	job.UpdatedAt = time.Now()
	return cs.StoreJob(job)
}

// DeleteJob removes a job from Consul KV
func (cs *ConsulStorage) DeleteJob(jobID string) error {
	key := fmt.Sprintf("%s/jobs/%s", cs.keyPrefix, jobID)
	
	_, err := cs.client.KV().Delete(key, nil)
	if err != nil {
		return fmt.Errorf("failed to delete job from Consul: %w", err)
	}
	
	cs.logger.WithField("job_id", jobID).Debug("Job deleted from Consul")
	return nil
}

// ListJobs returns a list of all job IDs
func (cs *ConsulStorage) ListJobs() ([]string, error) {
	prefix := fmt.Sprintf("%s/jobs/", cs.keyPrefix)
	
	pairs, _, err := cs.client.KV().List(prefix, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs from Consul: %w", err)
	}
	
	var jobIDs []string
	for _, pair := range pairs {
		// Extract job ID from key (remove prefix)
		jobID := pair.Key[len(prefix):]
		jobIDs = append(jobIDs, jobID)
	}
	
	return jobIDs, nil
}

// StoreJobHistory stores job history for debugging purposes
func (cs *ConsulStorage) StoreJobHistory(history *types.JobHistory) error {
	key := fmt.Sprintf("%s/history/%s", cs.keyPrefix, history.ID)
	
	data, err := json.Marshal(history)
	if err != nil {
		return fmt.Errorf("failed to marshal job history: %w", err)
	}
	
	pair := &consulapi.KVPair{
		Key:   key,
		Value: data,
	}
	
	_, err = cs.client.KV().Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to store job history in Consul: %w", err)
	}
	
	cs.logger.WithField("job_id", history.ID).Debug("Job history stored in Consul")
	return nil
}

// GetJobHistory retrieves job history with pagination
func (cs *ConsulStorage) GetJobHistory(limit, offset int) ([]types.JobHistory, int, error) {
	prefix := fmt.Sprintf("%s/history/", cs.keyPrefix)
	
	pairs, _, err := cs.client.KV().List(prefix, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get job history from Consul: %w", err)
	}
	
	// Parse and sort by creation time (newest first)
	var histories []types.JobHistory
	for _, pair := range pairs {
		var history types.JobHistory
		if err := json.Unmarshal(pair.Value, &history); err != nil {
			cs.logger.WithError(err).Warn("Failed to unmarshal job history")
			continue
		}
		histories = append(histories, history)
	}
	
	// Sort by creation time (newest first)
	sort.Slice(histories, func(i, j int) bool {
		return histories[i].CreatedAt.After(histories[j].CreatedAt)
	})
	
	total := len(histories)
	
	// Apply pagination
	if offset >= total {
		return []types.JobHistory{}, total, nil
	}
	
	end := offset + limit
	if end > total {
		end = total
	}
	
	return histories[offset:end], total, nil
}

// CleanupOldHistory removes job history older than the specified duration
func (cs *ConsulStorage) CleanupOldHistory(maxAge time.Duration) error {
	prefix := fmt.Sprintf("%s/history/", cs.keyPrefix)
	
	pairs, _, err := cs.client.KV().List(prefix, nil)
	if err != nil {
		return fmt.Errorf("failed to list job history from Consul: %w", err)
	}
	
	cutoff := time.Now().Add(-maxAge)
	var deletedCount int
	
	for _, pair := range pairs {
		var history types.JobHistory
		if err := json.Unmarshal(pair.Value, &history); err != nil {
			cs.logger.WithError(err).Warn("Failed to unmarshal job history during cleanup")
			continue
		}
		
		if history.CreatedAt.Before(cutoff) {
			if _, err := cs.client.KV().Delete(pair.Key, nil); err != nil {
				cs.logger.WithError(err).WithField("key", pair.Key).Warn("Failed to delete old job history")
				continue
			}
			deletedCount++
		}
	}
	
	cs.logger.WithFields(logrus.Fields{
		"deleted_count": deletedCount,
		"max_age":       maxAge,
	}).Info("Cleaned up old job history")
	
	return nil
}

// GetConfiguration retrieves a configuration value from Consul
func (cs *ConsulStorage) GetConfiguration(key string) (string, error) {
	fullKey := fmt.Sprintf("%s/config/%s", cs.keyPrefix, key)
	
	pair, _, err := cs.client.KV().Get(fullKey, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get configuration from Consul: %w", err)
	}
	
	if pair == nil {
		return "", fmt.Errorf("configuration key not found: %s", key)
	}
	
	return string(pair.Value), nil
}

// SetConfiguration stores a configuration value in Consul
func (cs *ConsulStorage) SetConfiguration(key, value string) error {
	fullKey := fmt.Sprintf("%s/config/%s", cs.keyPrefix, key)
	
	pair := &consulapi.KVPair{
		Key:   fullKey,
		Value: []byte(value),
	}
	
	_, err := cs.client.KV().Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to set configuration in Consul: %w", err)
	}
	
	cs.logger.WithFields(logrus.Fields{
		"key":   key,
		"value": value,
	}).Debug("Configuration updated in Consul")
	
	return nil
}

// Health checks the health of the Consul connection
func (cs *ConsulStorage) Health() error {
	_, err := cs.client.Status().Leader()
	if err != nil {
		return fmt.Errorf("consul health check failed: %w", err)
	}
	return nil
}

// AcquireLock acquires a distributed lock for the given key
// Returns a session ID that must be used to release the lock
func (cs *ConsulStorage) AcquireLock(lockKey string, timeout time.Duration) (string, error) {
	cs.logger.WithField("lock_key", lockKey).Debug("Attempting to acquire lock")
	
	// Create a session for the lock
	session := &consulapi.SessionEntry{
		TTL:      timeout.String(),
		Behavior: consulapi.SessionBehaviorRelease,
		Name:     fmt.Sprintf("build-lock-%s", lockKey),
	}
	
	sessionID, _, err := cs.client.Session().Create(session, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create session for lock: %w", err)
	}
	
	cs.logger.WithFields(logrus.Fields{
		"lock_key":   lockKey,
		"session_id": sessionID,
	}).Debug("Session created for lock")
	
	// Try to acquire the lock
	fullKey := fmt.Sprintf("%s/locks/%s", cs.keyPrefix, lockKey)
	pair := &consulapi.KVPair{
		Key:     fullKey,
		Value:   []byte(sessionID),
		Session: sessionID,
	}
	
	// Use the Acquire method which is atomic
	acquired, _, err := cs.client.KV().Acquire(pair, nil)
	if err != nil {
		// Clean up session if acquire failed
		cs.client.Session().Destroy(sessionID, nil)
		return "", fmt.Errorf("failed to acquire lock: %w", err)
	}
	
	if !acquired {
		// Lock is held by someone else, clean up session
		cs.client.Session().Destroy(sessionID, nil)
		return "", fmt.Errorf("lock is already held by another process")
	}
	
	cs.logger.WithFields(logrus.Fields{
		"lock_key":   lockKey,
		"session_id": sessionID,
	}).Info("Lock acquired successfully")
	
	return sessionID, nil
}

// ReleaseLock releases a distributed lock using the session ID
func (cs *ConsulStorage) ReleaseLock(lockKey, sessionID string) error {
	cs.logger.WithFields(logrus.Fields{
		"lock_key":   lockKey,
		"session_id": sessionID,
	}).Debug("Releasing lock")
	
	// First, release the key from the session
	fullKey := fmt.Sprintf("%s/locks/%s", cs.keyPrefix, lockKey)
	pair := &consulapi.KVPair{
		Key:     fullKey,
		Session: sessionID,
	}
	
	// Use the Release method which is atomic
	released, _, err := cs.client.KV().Release(pair, nil)
	if err != nil {
		cs.logger.WithError(err).Warn("Failed to release lock key")
	} else if !released {
		cs.logger.Warn("Lock key was not held by this session")
	}
	
	// Always destroy the session to clean up
	_, err = cs.client.Session().Destroy(sessionID, nil)
	if err != nil {
		cs.logger.WithError(err).Warn("Failed to destroy session")
		return fmt.Errorf("failed to destroy session: %w", err)
	}
	
	cs.logger.WithFields(logrus.Fields{
		"lock_key":   lockKey,
		"session_id": sessionID,
	}).Info("Lock released successfully")
	
	return nil
}

// GenerateImageLockKey generates a consistent lock key for image builds
func (cs *ConsulStorage) GenerateImageLockKey(registryURL, imageName, branch string) string {
	// Sanitize components for use in lock key
	sanitizedRegistry := strings.ToLower(strings.ReplaceAll(registryURL, "/", "-"))
	sanitizedImage := strings.ToLower(strings.ReplaceAll(imageName, "/", "-"))
	sanitizedBranch := strings.ToLower(strings.ReplaceAll(branch, "/", "-"))
	
	// Create a consistent lock key that includes registry, image, and branch
	// This allows different branches to build concurrently but prevents
	// concurrent builds of the same image on the same branch
	return fmt.Sprintf("image-%s-%s-%s", sanitizedRegistry, sanitizedImage, sanitizedBranch)
}
