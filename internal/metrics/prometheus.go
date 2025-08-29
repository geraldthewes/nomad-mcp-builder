package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// Build phase metrics
	buildDuration prometheus.HistogramVec
	
	// Test phase metrics
	testDuration prometheus.HistogramVec
	
	// Publish phase metrics
	publishDuration prometheus.HistogramVec
	
	// Job metrics
	jobSuccessRate prometheus.GaugeVec
	concurrentJobs prometheus.Gauge
	totalJobs      prometheus.CounterVec
	
	// Resource usage metrics
	resourceUsage prometheus.GaugeVec
	
	// System metrics
	healthCheck prometheus.GaugeVec
	
	logger *logrus.Logger
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	m := &Metrics{
		buildDuration: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "build_duration_seconds",
				Help:    "Duration of build phase in seconds",
				Buckets: []float64{30, 60, 120, 300, 600, 1200, 1800, 3600}, // 30s to 1h
			},
			[]string{"status", "owner"},
		),
		
		testDuration: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "test_duration_seconds",
				Help:    "Duration of test phase in seconds",
				Buckets: []float64{10, 30, 60, 120, 300, 600, 900, 1800}, // 10s to 30m
			},
			[]string{"status", "owner"},
		),
		
		publishDuration: *prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "publish_duration_seconds",
				Help:    "Duration of publish phase in seconds",
				Buckets: []float64{5, 15, 30, 60, 120, 300}, // 5s to 5m
			},
			[]string{"status", "owner"},
		),
		
		jobSuccessRate: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "job_success_rate",
				Help: "Success rate of jobs over time window",
			},
			[]string{"window", "owner"},
		),
		
		concurrentJobs: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "concurrent_jobs_total",
				Help: "Current number of running jobs",
			},
		),
		
		totalJobs: *prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "total_jobs",
				Help: "Total number of jobs processed",
			},
			[]string{"status", "phase", "owner"},
		),
		
		resourceUsage: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "resource_usage",
				Help: "Resource usage metrics",
			},
			[]string{"resource_type", "job_id", "phase"},
		),
		
		healthCheck: *prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "health_check",
				Help: "Health status of services (1=healthy, 0=unhealthy)",
			},
			[]string{"service"},
		),
		
		logger: logrus.New(),
	}
	
	// Register all metrics
	prometheus.MustRegister(
		&m.buildDuration,
		&m.testDuration,
		&m.publishDuration,
		&m.jobSuccessRate,
		m.concurrentJobs,
		&m.totalJobs,
		&m.resourceUsage,
		&m.healthCheck,
	)
	
	return m
}

// RecordBuildDuration records the duration of a build phase
func (m *Metrics) RecordBuildDuration(duration time.Duration, status, owner string) {
	m.buildDuration.WithLabelValues(status, owner).Observe(duration.Seconds())
	m.logger.WithFields(logrus.Fields{
		"duration": duration.Seconds(),
		"status":   status,
		"owner":    owner,
		"phase":    "build",
	}).Debug("Recorded build duration metric")
}

// RecordTestDuration records the duration of a test phase
func (m *Metrics) RecordTestDuration(duration time.Duration, status, owner string) {
	m.testDuration.WithLabelValues(status, owner).Observe(duration.Seconds())
	m.logger.WithFields(logrus.Fields{
		"duration": duration.Seconds(),
		"status":   status,
		"owner":    owner,
		"phase":    "test",
	}).Debug("Recorded test duration metric")
}

// RecordPublishDuration records the duration of a publish phase
func (m *Metrics) RecordPublishDuration(duration time.Duration, status, owner string) {
	m.publishDuration.WithLabelValues(status, owner).Observe(duration.Seconds())
	m.logger.WithFields(logrus.Fields{
		"duration": duration.Seconds(),
		"status":   status,
		"owner":    owner,
		"phase":    "publish",
	}).Debug("Recorded publish duration metric")
}

// UpdateJobSuccessRate updates the job success rate for a time window
func (m *Metrics) UpdateJobSuccessRate(rate float64, window, owner string) {
	m.jobSuccessRate.WithLabelValues(window, owner).Set(rate)
	m.logger.WithFields(logrus.Fields{
		"rate":   rate,
		"window": window,
		"owner":  owner,
	}).Debug("Updated job success rate metric")
}

// IncrementConcurrentJobs increments the concurrent jobs counter
func (m *Metrics) IncrementConcurrentJobs() {
	m.concurrentJobs.Inc()
}

// DecrementConcurrentJobs decrements the concurrent jobs counter
func (m *Metrics) DecrementConcurrentJobs() {
	m.concurrentJobs.Dec()
}

// IncrementTotalJobs increments the total jobs counter
func (m *Metrics) IncrementTotalJobs(status, phase, owner string) {
	m.totalJobs.WithLabelValues(status, phase, owner).Inc()
	m.logger.WithFields(logrus.Fields{
		"status": status,
		"phase":  phase,
		"owner":  owner,
	}).Debug("Incremented total jobs metric")
}

// RecordResourceUsage records resource usage metrics
func (m *Metrics) RecordResourceUsage(resourceType, jobID, phase string, value float64) {
	m.resourceUsage.WithLabelValues(resourceType, jobID, phase).Set(value)
	m.logger.WithFields(logrus.Fields{
		"resource_type": resourceType,
		"job_id":        jobID,
		"phase":         phase,
		"value":         value,
	}).Debug("Recorded resource usage metric")
}

// UpdateHealthCheck updates the health status of a service
func (m *Metrics) UpdateHealthCheck(service string, healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	m.healthCheck.WithLabelValues(service).Set(value)
	m.logger.WithFields(logrus.Fields{
		"service": service,
		"healthy": healthy,
	}).Debug("Updated health check metric")
}

// GetConcurrentJobs returns the current number of concurrent jobs
func (m *Metrics) GetConcurrentJobs() float64 {
	metric := &dto.Metric{}
	m.concurrentJobs.Write(metric)
	return metric.GetGauge().GetValue()
}

// StartMetricsServer starts the Prometheus metrics HTTP server
func (m *Metrics) StartMetricsServer(port int) error {
	http.Handle("/metrics", promhttp.Handler())
	
	m.logger.WithField("port", port).Info("Starting Prometheus metrics server")
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

// MetricsMiddleware provides HTTP middleware to record request metrics
func (m *Metrics) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Wrap the response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
		
		next.ServeHTTP(wrapped, r)
		
		duration := time.Since(start)
		
		// Record HTTP request metrics (optional - can be added if needed)
		m.logger.WithFields(logrus.Fields{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status_code": wrapped.statusCode,
			"duration":    duration.Seconds(),
		}).Debug("HTTP request processed")
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// CalculateSuccessRate calculates success rate for a given time window
func (m *Metrics) CalculateSuccessRate(jobs []JobMetricData, window time.Duration) float64 {
	if len(jobs) == 0 {
		return 0.0
	}
	
	now := time.Now()
	windowStart := now.Add(-window)
	
	var total, successful int
	for _, job := range jobs {
		if job.Timestamp.After(windowStart) {
			total++
			if job.Status == "succeeded" {
				successful++
			}
		}
	}
	
	if total == 0 {
		return 0.0
	}
	
	return float64(successful) / float64(total)
}

// JobMetricData represents job data for metrics calculation
type JobMetricData struct {
	ID        string
	Status    string
	Owner     string
	Timestamp time.Time
}

// UpdateAllHealthChecks updates health status for all monitored services
func (m *Metrics) UpdateAllHealthChecks(services map[string]bool) {
	for service, healthy := range services {
		m.UpdateHealthCheck(service, healthy)
	}
}

// RecordJobPhaseCompletion records completion of a job phase with all relevant metrics
func (m *Metrics) RecordJobPhaseCompletion(phase string, duration time.Duration, status, owner string, resourceUsage map[string]float64, jobID string) {
	// Record duration based on phase
	switch phase {
	case "build":
		m.RecordBuildDuration(duration, status, owner)
	case "test":
		m.RecordTestDuration(duration, status, owner)
	case "publish":
		m.RecordPublishDuration(duration, status, owner)
	}
	
	// Record total jobs counter
	m.IncrementTotalJobs(status, phase, owner)
	
	// Record resource usage if provided
	for resourceType, value := range resourceUsage {
		m.RecordResourceUsage(resourceType, jobID, phase, value)
	}
	
	m.logger.WithFields(logrus.Fields{
		"phase":    phase,
		"duration": duration.Seconds(),
		"status":   status,
		"owner":    owner,
		"job_id":   jobID,
	}).Info("Recorded job phase completion metrics")
}