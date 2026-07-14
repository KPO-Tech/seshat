package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/EngineerProjects/seshat/internal/automation"
	"github.com/EngineerProjects/seshat/internal/db"
)

// authKind describes how a request was authenticated.
type authKind int

const (
	authKindNone   authKind = iota
	authKindMaster          // SESHAT_AUTOMATION_API_KEY — full access incl. admin
	authKindUser            // sats_xxx user key — job CRUD for own owner_id only
)

type contextKey int

const (
	ctxAuthKind      contextKey = iota
	ctxResolvedOwner            // ownerID resolved from user key (overrides header)
)

// server holds the HTTP mux and the scheduler it proxies.
type server struct {
	mux       *http.ServeMux
	sched     *automation.JobScheduler
	db        *db.DB
	masterKey string // SESHAT_AUTOMATION_API_KEY; empty = no auth
	log       *log.Logger
}

func newServer(sched *automation.JobScheduler, database *db.DB, masterKey string) *server {
	s := &server{
		mux:       http.NewServeMux(),
		sched:     sched,
		db:        database,
		masterKey: masterKey,
		log:       log.New(log.Writer(), "[seshat-automation] ", log.LstdFlags),
	}
	s.routes()
	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *server) routes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Admin: API key management (master key only)
	s.mux.HandleFunc("POST /v1/admin/keys", s.adminAuth(s.handleCreateAPIKey))
	s.mux.HandleFunc("GET /v1/admin/keys", s.adminAuth(s.handleListAPIKeys))
	s.mux.HandleFunc("DELETE /v1/admin/keys/{id}", s.adminAuth(s.handleRevokeAPIKey))

	// Jobs: accessible by master key (seshat-ai) or user key (direct access)
	s.mux.HandleFunc("GET /v1/jobs", s.auth(s.handleListJobs))
	s.mux.HandleFunc("POST /v1/jobs", s.auth(s.handleCreateJob))
	s.mux.HandleFunc("GET /v1/jobs/{id}", s.auth(s.handleGetJob))
	s.mux.HandleFunc("PUT /v1/jobs/{id}", s.auth(s.handleUpdateJob))
	s.mux.HandleFunc("DELETE /v1/jobs/{id}", s.auth(s.handleDeleteJob))
	s.mux.HandleFunc("POST /v1/jobs/{id}/run", s.auth(s.handleRunNow))
	s.mux.HandleFunc("POST /v1/jobs/{id}/pause", s.auth(s.handlePauseJob))
	s.mux.HandleFunc("PUT /v1/jobs/{id}/pause", s.auth(s.handlePauseJob))
	s.mux.HandleFunc("POST /v1/jobs/{id}/resume", s.auth(s.handleResumeJob))
	s.mux.HandleFunc("PUT /v1/jobs/{id}/resume", s.auth(s.handleResumeJob))
	s.mux.HandleFunc("GET /v1/jobs/{id}/runs", s.auth(s.handleListRuns))
	s.mux.HandleFunc("GET /v1/runs/{id}", s.auth(s.handleGetRun))

	// Legacy routes (no /v1 prefix) — kept for backward compat with seshat-ai proxy
	s.mux.HandleFunc("GET /jobs", s.auth(s.handleListJobs))
	s.mux.HandleFunc("POST /jobs", s.auth(s.handleCreateJob))
	s.mux.HandleFunc("GET /jobs/{id}", s.auth(s.handleGetJob))
	s.mux.HandleFunc("PUT /jobs/{id}", s.auth(s.handleUpdateJob))
	s.mux.HandleFunc("DELETE /jobs/{id}", s.auth(s.handleDeleteJob))
	s.mux.HandleFunc("POST /jobs/{id}/run", s.auth(s.handleRunNow))
	s.mux.HandleFunc("POST /jobs/{id}/pause", s.auth(s.handlePauseJob))
	s.mux.HandleFunc("PUT /jobs/{id}/pause", s.auth(s.handlePauseJob))
	s.mux.HandleFunc("POST /jobs/{id}/resume", s.auth(s.handleResumeJob))
	s.mux.HandleFunc("PUT /jobs/{id}/resume", s.auth(s.handleResumeJob))
	s.mux.HandleFunc("GET /jobs/{id}/runs", s.auth(s.handleListRuns))
	s.mux.HandleFunc("GET /runs/{id}", s.auth(s.handleGetRun))
}

// ─── Middleware ────────────────────────────────────────────────────────────────

// adminAuth requires the master key. Used for admin-only endpoints.
func (s *server) adminAuth(next http.HandlerFunc) http.HandlerFunc {
	if s.masterKey == "" {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != s.masterKey {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxAuthKind, authKindMaster)
		next(w, r.WithContext(ctx))
	}
}

// auth accepts either the master key (full access) or a valid sats_ user key.
// When a user key is used, the resolved owner_id is injected into the context
// and overrides the X-Seshat-User-ID header.
func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	if s.masterKey == "" && s.db == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

		// Master key → trusted seshat-ai caller, ownerID from header
		if s.masterKey != "" && token == s.masterKey {
			ctx := context.WithValue(r.Context(), ctxAuthKind, authKindMaster)
			next(w, r.WithContext(ctx))
			return
		}

		// User key → validate against DB, derive ownerID from key record
		if db.IsAutomationAPIKey(token) && s.db != nil {
			hash := db.HashAutomationAPIKey(token)
			keyRow, err := s.db.GetAutomationAPIKeyByHash(r.Context(), hash)
			if err != nil || keyRow == nil {
				jsonError(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ctxAuthKind, authKindUser)
			ctx = context.WithValue(ctx, ctxResolvedOwner, keyRow.OwnerID)
			next(w, r.WithContext(ctx))
			return
		}

		jsonError(w, "unauthorized", http.StatusUnauthorized)
	}
}

// ownerID returns the effective owner for this request.
// For user-key auth, the owner is resolved from the key record (not the header).
// For master-key auth, the owner comes from the X-Seshat-User-ID header.
func ownerID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxResolvedOwner).(string); ok && v != "" {
		return v
	}
	return r.Header.Get("X-Seshat-User-ID")
}

// isMasterAuth returns true if the request authenticated with the master key.
func isMasterAuth(r *http.Request) bool {
	k, _ := r.Context().Value(ctxAuthKind).(authKind)
	return k == authKindMaster
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

// ownerGuard returns 403 if the job's owner doesn't match the caller.
// Jobs with no owner (empty OwnerID) are accessible to all.
func ownerGuard(w http.ResponseWriter, job *automation.Job, caller string) bool {
	if job.OwnerID == "" || caller == "" || job.OwnerID == caller {
		return true
	}
	jsonError(w, "forbidden", http.StatusForbidden)
	return false
}

func (s *server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.sched.ListJobs(r.Context(), ownerID(r))
	if err != nil {
		s.internalError(w, err, "list jobs")
		return
	}
	if jobs == nil {
		jobs = []*automation.Job{}
	}
	jsonOK(w, jobs)
}

func (s *server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req jobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	job, err := req.toJob()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	job.OwnerID = ownerID(r)
	if err := s.sched.AddJob(r.Context(), job); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, job)
}

func (s *server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.sched.GetJob(r.Context(), id)
	if err != nil {
		s.internalError(w, err, "get job")
		return
	}
	if job == nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	if !ownerGuard(w, job, ownerID(r)) {
		return
	}
	jsonOK(w, job)
}

func (s *server) handleUpdateJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.sched.GetJob(r.Context(), id)
	if err != nil {
		s.internalError(w, err, "update job: fetch existing")
		return
	}
	if existing == nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	if !ownerGuard(w, existing, ownerID(r)) {
		return
	}
	var req jobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	updated, err := req.toJob()
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated.ID = existing.ID
	updated.OwnerID = existing.OwnerID
	updated.CreatedAt = existing.CreatedAt
	updated.Status = existing.Status
	if err := s.sched.UpdateJob(r.Context(), updated); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, updated)
}

func (s *server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.sched.GetJob(r.Context(), id)
	if err != nil {
		s.internalError(w, err, "delete job: fetch existing")
		return
	}
	if existing == nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	if !ownerGuard(w, existing, ownerID(r)) {
		return
	}
	if err := s.sched.RemoveJob(r.Context(), id, ownerID(r)); err != nil {
		s.internalError(w, err, "delete job")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleRunNow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Verify ownership before triggering
	job, err := s.sched.GetJob(r.Context(), id)
	if err != nil || job == nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	if !ownerGuard(w, job, ownerID(r)) {
		return
	}
	run, err := s.sched.RunNow(r.Context(), id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	jsonOK(w, run)
}

func (s *server) handlePauseJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.sched.GetJob(r.Context(), id)
	if err != nil || job == nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	if !ownerGuard(w, job, ownerID(r)) {
		return
	}
	if err := s.sched.PauseJob(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated, _ := s.sched.GetJob(r.Context(), id)
	jsonOK(w, updated)
}

func (s *server) handleResumeJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.sched.GetJob(r.Context(), id)
	if err != nil || job == nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	if !ownerGuard(w, job, ownerID(r)) {
		return
	}
	if err := s.sched.ResumeJob(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated, _ := s.sched.GetJob(r.Context(), id)
	jsonOK(w, updated)
}

func (s *server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Verify ownership of the job before listing its runs
	job, err := s.sched.GetJob(r.Context(), id)
	if err != nil || job == nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	if !ownerGuard(w, job, ownerID(r)) {
		return
	}
	runs, err := s.sched.ListRuns(r.Context(), id, 50)
	if err != nil {
		s.internalError(w, err, "list runs")
		return
	}
	if runs == nil {
		runs = []*automation.JobRun{}
	}
	jsonOK(w, runs)
}

func (s *server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.sched.GetRun(r.Context(), id)
	if err != nil {
		s.internalError(w, err, "get run")
		return
	}
	if run == nil {
		jsonError(w, "run not found", http.StatusNotFound)
		return
	}
	// Verify ownership via the parent job
	job, _ := s.sched.GetJob(r.Context(), run.JobID)
	if job != nil && !ownerGuard(w, job, ownerID(r)) {
		return
	}
	jsonOK(w, run)
}

// ─── Request / response types ─────────────────────────────────────────────────

// jobRequest is the JSON shape for create and update.
type jobRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Trigger     triggerRequest `json:"trigger"`
	Agent       agentRequest   `json:"agent"`
	Task        string         `json:"task"`
}

type triggerRequest struct {
	Type     string `json:"type"` // "cron" | "interval" | "once"
	Cron     string `json:"cron,omitempty"`
	Interval string `json:"interval,omitempty"` // Go duration string, e.g. "24h"
	RunAt    string `json:"run_at,omitempty"`   // RFC3339
}

type agentRequest struct {
	Slug         string   `json:"slug,omitempty"`
	BaseType     string   `json:"base_type,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	Skills       []string `json:"skills,omitempty"`
	Model        string   `json:"model,omitempty"`
	MaxTurns     int      `json:"max_turns,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
}

func (req *jobRequest) toJob() (*automation.Job, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Task == "" {
		return nil, fmt.Errorf("task is required")
	}

	trigger, err := parseTrigger(req.Trigger)
	if err != nil {
		return nil, fmt.Errorf("trigger: %w", err)
	}

	return &automation.Job{
		Name:        req.Name,
		Description: req.Description,
		Trigger:     trigger,
		Agent: automation.AgentConfig{
			Slug:         req.Agent.Slug,
			BaseType:     req.Agent.BaseType,
			Tools:        req.Agent.Tools,
			Skills:       req.Agent.Skills,
			Model:        req.Agent.Model,
			MaxTurns:     req.Agent.MaxTurns,
			SystemPrompt: req.Agent.SystemPrompt,
		},
		Task: req.Task,
	}, nil
}

func parseTrigger(t triggerRequest) (automation.Trigger, error) {
	switch automation.TriggerType(t.Type) {
	case automation.TriggerTypeCron:
		if t.Cron == "" {
			return automation.Trigger{}, fmt.Errorf("cron expression required")
		}
		return automation.Trigger{Type: automation.TriggerTypeCron, Cron: t.Cron}, nil

	case automation.TriggerTypeInterval:
		if t.Interval == "" {
			return automation.Trigger{}, fmt.Errorf("interval duration required (e.g. \"24h\")")
		}
		d, err := time.ParseDuration(t.Interval)
		if err != nil {
			return automation.Trigger{}, fmt.Errorf("invalid interval %q: %w", t.Interval, err)
		}
		return automation.Trigger{Type: automation.TriggerTypeInterval, Interval: d}, nil

	case automation.TriggerTypeOnce:
		if t.RunAt == "" {
			return automation.Trigger{}, fmt.Errorf("run_at required for 'once' trigger")
		}
		at, err := time.Parse(time.RFC3339, t.RunAt)
		if err != nil {
			return automation.Trigger{}, fmt.Errorf("invalid run_at %q: %w", t.RunAt, err)
		}
		return automation.Trigger{Type: automation.TriggerTypeOnce, RunAt: &at}, nil

	default:
		return automation.Trigger{}, fmt.Errorf("unknown trigger type %q (valid: cron, interval, once)", t.Type)
	}
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// internalError logs the real error and returns a generic 500 to the caller.
// Use this for database/scheduler errors to avoid leaking schema details.
func (s *server) internalError(w http.ResponseWriter, err error, context string) {
	s.log.Printf("internal error [%s]: %v", context, err)
	jsonError(w, "internal server error", http.StatusInternalServerError)
}
