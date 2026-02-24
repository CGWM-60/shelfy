package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"cgwm/shelfy/internal/ai"
	"cgwm/shelfy/internal/auth"
	"cgwm/shelfy/internal/debrid"
	"cgwm/shelfy/internal/dlna"
	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/fileserver"
	"cgwm/shelfy/internal/infra/storage"
	"cgwm/shelfy/internal/media"
	"cgwm/shelfy/internal/repo"
	"cgwm/shelfy/internal/service"
	"cgwm/shelfy/internal/smb"
	"cgwm/shelfy/internal/stream"
	"cgwm/shelfy/internal/terminal"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

type Handler struct {
	svc      *service.App
	events   *events.Bus
	mediaDir string
	fs       storage.FileSystem
	fileUser string
	filePass string
	auth     *auth.Manager
	terminal terminal.Runner
}

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

func New(app *service.App, bus *events.Bus, mediaDir string, fs storage.FileSystem, fileUser, filePass string, authManager *auth.Manager) *Handler {
	if authManager == nil {
		authManager, _ = auth.NewManager(auth.Config{Enabled: false})
	}
	return &Handler{
		svc:      app,
		events:   bus,
		mediaDir: mediaDir,
		fs:       fs,
		fileUser: fileUser,
		filePass: filePass,
		auth:     authManager,
		terminal: terminal.NewLocalRunner(""),
	}
}

func (h *Handler) AuthManager() *auth.Manager {
	return h.auth
}

func (h *Handler) GetHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil || !h.auth.Enabled() {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled":       false,
			"authenticated": true,
			"username":      "",
		})
		return
	}
	username, ok := h.auth.AuthenticateRequest(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":       true,
		"authenticated": ok,
		"username":      username,
	})
}

func (h *Handler) PostAuthLogin(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil || !h.auth.Enabled() {
		writeError(w, http.StatusBadRequest, "auth disabled")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if !h.auth.CheckCredentials(strings.TrimSpace(req.Username), req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	cookie, err := h.auth.NewSessionCookie(strings.TrimSpace(req.Username), requestIsSecure(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cookie != nil {
		http.SetCookie(w, cookie)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"username":      strings.TrimSpace(req.Username),
	})
}

func (h *Handler) PostAuthLogout(w http.ResponseWriter, r *http.Request) {
	if h.auth != nil && h.auth.Enabled() {
		http.SetCookie(w, h.auth.ExpiredCookie(requestIsSecure(r)))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"done": true})
}

func (h *Handler) PostDownloads(w http.ResponseWriter, r *http.Request) {
	var req domain.DownloadAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	jobs, err := h.svc.Downloads.Add(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.maybeAutoIndexAI(r.Context(), "downloads_added")
	writeJSON(w, http.StatusCreated, jobs)
}

func (h *Handler) GetDownloads(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.svc.Downloads.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (h *Handler) PostDownloadStart(w http.ResponseWriter, r *http.Request) {
	h.withDownloadAction(w, r, h.svc.Downloads.Start)
}

func (h *Handler) PostDownloadPause(w http.ResponseWriter, r *http.Request) {
	h.withDownloadAction(w, r, h.svc.Downloads.Pause)
}

func (h *Handler) PostDownloadResume(w http.ResponseWriter, r *http.Request) {
	h.withDownloadAction(w, r, h.svc.Downloads.Resume)
}

func (h *Handler) DeleteDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := h.svc.Downloads.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) withDownloadAction(w http.ResponseWriter, r *http.Request, action func(context.Context, string) error) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := action(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	id, ch := h.events.Subscribe(64)
	defer h.events.Unsubscribe(id)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-ch:
			if err := events.WriteSSE(w, evt); err != nil {
				return
			}
		}
	}
}

func (h *Handler) GetTerminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "websocket upgrade required")
		return
	}
	defer conn.Close()

	if h.terminal == nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("terminal unavailable"))
		return
	}
	session, err := h.terminal.Open(r.Context())
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("cannot open terminal session"))
		return
	}
	defer session.Close()

	done := make(chan struct{}, 2)
	signalDone := func() {
		select {
		case done <- struct{}{}:
		default:
		}
	}

	go func() {
		buffer := make([]byte, 4096)
		for {
			n, readErr := session.Read(buffer)
			if n > 0 {
				if writeErr := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
					signalDone()
					return
				}
			}
			if readErr != nil {
				signalDone()
				return
			}
		}
	}()

	go func() {
		for {
			messageType, payload, readErr := conn.ReadMessage()
			if readErr != nil {
				signalDone()
				return
			}
			if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
				continue
			}
			if len(payload) == 0 {
				continue
			}
			if _, writeErr := session.Write(payload); writeErr != nil {
				signalDone()
				return
			}
		}
	}()

	select {
	case <-r.Context().Done():
	case <-done:
	}
}

func (h *Handler) GetDebridProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Debrid.ListProviders())
}

func (h *Handler) PostDebridAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider  string `json:"provider"`
		Label     string `json:"label"`
		APIKey    string `json:"apiKey"`
		IsActive  bool   `json:"isActive"`
		IsDefault bool   `json:"isDefault"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Provider == "" || req.Label == "" {
		writeError(w, http.StatusBadRequest, "provider and label are required")
		return
	}
	account, err := h.svc.Debrid.CreateAccount(r.Context(), req.Provider, req.Label, req.APIKey, req.IsActive, req.IsDefault)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, account)
}

func (h *Handler) GetDebridAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.svc.Debrid.ListAccounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

func (h *Handler) PatchDebridAccount(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Label     *string `json:"label"`
		IsActive  *bool   `json:"isActive"`
		IsDefault *bool   `json:"isDefault"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	updated, err := h.svc.Debrid.PatchAccount(r.Context(), uint(id), debrid.AccountPatch{Label: req.Label, IsActive: req.IsActive, IsDefault: req.IsDefault})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteDebridAccount(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Debrid.DeleteAccount(r.Context(), uint(id)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PostDebridAuthStart(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	session, err := h.svc.Debrid.StartAuth(r.Context(), uint(id))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (h *Handler) PostDebridAuthPoll(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var session debrid.AuthSession
	if err := json.NewDecoder(r.Body).Decode(&session); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	done, err := h.svc.Debrid.PollAuth(r.Context(), uint(id), session)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"done": done})
}

func (h *Handler) PostDebridAuthPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if err := h.svc.Debrid.AuthWithPassword(r.Context(), uint(id), req.Username, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"done": true})
}

func (h *Handler) GetDebridStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	status, err := h.svc.Debrid.Status(r.Context(), uint(id))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (h *Handler) PostMediaScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Path == "" {
		req.Path = h.mediaDir
	}
	count, err := h.svc.Media.Scan(r.Context(), req.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.maybeAutoIndexAI(r.Context(), "media_scanned")
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

func (h *Handler) GetMedia(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Media.List(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("kind"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) GetMediaByID(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Media.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, repo.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	progress, _ := h.svc.Media.GetProgress(r.Context(), item.ID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"item": item, "progress": progress})
}

func (h *Handler) PostMediaProgress(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		PositionMs int64 `json:"positionMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.svc.Media.SaveProgress(r.Context(), id, req.PositionMs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetStream(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Media.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	file, err := h.fs.OpenFile(item.Path, os.O_RDONLY, 0)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rng, err := stream.ParseRangeHeader(r.Header.Get("Range"), info.Size())
	if err != nil {
		writeError(w, http.StatusRequestedRangeNotSatisfiable, err.Error())
		return
	}
	ctype := mime.TypeByExtension(strings.ToLower(pathExt(item.Path)))
	stream.WriteRangeHeaders(w, rng, info.Size(), ctype)

	if _, err := file.Seek(rng.Start, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = io.CopyN(w, file, rng.End-rng.Start+1)
}

func (h *Handler) GetSubtitles(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Media.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	base := strings.TrimSuffix(item.Path, pathExt(item.Path))
	candidates := []string{base + ".srt", base + ".vtt"}
	for _, candidate := range candidates {
		file, err := h.fs.OpenFile(candidate, os.O_RDONLY, 0)
		if err != nil {
			continue
		}
		defer file.Close()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, file)
		return
	}
	writeError(w, http.StatusNotFound, "subtitle not found")
}

func pathExt(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
	}
	return ""
}

func (h *Handler) PostAIIndex(w http.ResponseWriter, r *http.Request) {
	count, err := h.svc.AI.Index(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"indexed": count})
}

func (h *Handler) PostAISearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	results, err := h.svc.AI.Search(r.Context(), req.Query, req.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handler) PostAIAsk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Question   string `json:"question"`
		SearchOnly bool   `json:"searchOnly"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	answer, err := h.svc.AI.Ask(r.Context(), req.Question, req.SearchOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, answer)
}

func (h *Handler) GetAIStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.AI.Status())
}

func (h *Handler) GetAIReport(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.AI.Report())
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.svc.Settings.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) GetSystemStorage(w http.ResponseWriter, r *http.Request) {
	roots, err := h.managedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	knownUsage, err := h.computeKnownUsageByRoot(r.Context(), roots)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type rootStat struct {
		Path           string `json:"path"`
		Exists         bool   `json:"exists"`
		TotalBytes     uint64 `json:"totalBytes"`
		FreeBytes      uint64 `json:"freeBytes"`
		AvailableBytes uint64 `json:"availableBytes"`
		KnownUsedBytes int64  `json:"knownUsedBytes"`
	}
	rootStats := make([]rootStat, 0, len(roots))
	var totalBytes uint64
	var freeBytes uint64
	var availableBytes uint64
	var knownUsedTotal int64
	for _, root := range roots {
		stat := rootStat{
			Path:           root,
			KnownUsedBytes: knownUsage[root],
		}
		knownUsedTotal += stat.KnownUsedBytes
		if _, statErr := h.fs.Stat(root); statErr != nil {
			rootStats = append(rootStats, stat)
			continue
		}
		stat.Exists = true
		usage, usageErr := h.fs.StatFS(root)
		if usageErr == nil {
			stat.TotalBytes = usage.TotalBytes
			stat.FreeBytes = usage.FreeBytes
			stat.AvailableBytes = usage.AvailableBytes
		}
		rootStats = append(rootStats, stat)
		totalBytes += stat.TotalBytes
		freeBytes += stat.FreeBytes
		availableBytes += stat.AvailableBytes
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"roots":       rootStats,
		"totals": map[string]interface{}{
			"totalBytes":     totalBytes,
			"freeBytes":      freeBytes,
			"availableBytes": availableBytes,
			"knownUsedBytes": knownUsedTotal,
		},
	})
}

func (h *Handler) PostSystemSpeedtest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SizeMB int `json:"sizeMB"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	sizeMB := req.SizeMB
	if sizeMB <= 0 {
		sizeMB = 16
	}
	if sizeMB > 128 {
		sizeMB = 128
	}

	roots, err := h.managedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(roots) == 0 {
		writeError(w, http.StatusBadRequest, "no managed roots configured")
		return
	}
	targetRoot := roots[0]
	if err := h.fs.MkdirAll(targetRoot); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	tempPath := filepath.Join(targetRoot, ".shelfy-speedtest.tmp")
	file, err := h.fs.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o644)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer func() {
		_ = file.Close()
		_ = h.fs.Remove(tempPath)
	}()

	chunk := make([]byte, 1<<20)
	var written int64
	writeStart := time.Now()
	for i := 0; i < sizeMB; i++ {
		n, writeErr := file.Write(chunk)
		if writeErr != nil {
			writeError(w, http.StatusBadGateway, writeErr.Error())
			return
		}
		written += int64(n)
	}
	writeElapsed := time.Since(writeStart)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	readStart := time.Now()
	readBytes, err := io.Copy(io.Discard, io.LimitReader(file, written))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	readElapsed := time.Since(readStart)

	writeMbps := bytesPerSecondToMBps(written, writeElapsed)
	readMbps := bytesPerSecondToMBps(readBytes, readElapsed)
	totalElapsed := writeElapsed + readElapsed
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":       tempPath,
		"sampleMB":   sizeMB,
		"writeMBps":  writeMbps,
		"readMBps":   readMbps,
		"durationMs": totalElapsed.Milliseconds(),
	})
}

func (h *Handler) GetFSRoots(w http.ResponseWriter, r *http.Request) {
	roots, err := h.managedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"roots": roots})
}

func (h *Handler) GetFSList(w http.ResponseWriter, r *http.Request) {
	roots, err := h.managedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	path := r.URL.Query().Get("path")
	listing, err := h.svc.Files.ListManagedPath(r.Context(), roots, path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, listing)
}

func (h *Handler) GetFSFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if strings.TrimSpace(filePath) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	roots, err := h.managedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	content, err := h.svc.Files.ReadManagedFile(r.Context(), roots, filePath, 1<<20)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": filePath, "content": content})
}

func (h *Handler) PostFSMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	roots, err := h.managedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.svc.Files.CreateManagedDir(r.Context(), roots, req.Path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.rescanLibraries(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PutFSFile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	roots, err := h.managedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.svc.Files.WriteManagedFile(r.Context(), roots, req.Path, []byte(req.Content)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.rescanLibraries(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) PatchFSMove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FromPath string `json:"fromPath"`
		ToPath   string `json:"toPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.FromPath) == "" || strings.TrimSpace(req.ToPath) == "" {
		writeError(w, http.StatusBadRequest, "fromPath and toPath are required")
		return
	}
	roots, err := h.managedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.svc.Files.MoveManagedPath(r.Context(), roots, req.FromPath, req.ToPath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.rescanLibraries(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteFSItem(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if strings.TrimSpace(path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	recursive := false
	rawRecursive := r.URL.Query().Get("recursive")
	if rawRecursive != "" {
		parsed, err := strconv.ParseBool(rawRecursive)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid recursive flag")
			return
		}
		recursive = parsed
	}
	roots, err := h.managedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.svc.Files.DeleteManagedPath(r.Context(), roots, path, recursive); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.rescanLibraries(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) managedRoots(ctx context.Context) ([]string, error) {
	settings, err := h.svc.Settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, 1+len(settings.LibraryPaths))
	seen := map[string]struct{}{}
	addRoot := func(raw string) {
		cleaned := strings.TrimSpace(raw)
		if cleaned == "" {
			return
		}
		abs, absErr := filepath.Abs(filepath.Clean(cleaned))
		if absErr != nil {
			return
		}
		if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
			abs = resolved
		}
		if _, ok := seen[abs]; ok {
			return
		}
		seen[abs] = struct{}{}
		roots = append(roots, abs)
	}
	addRoot(settings.DownloadsPath)
	for _, root := range settings.LibraryPaths {
		addRoot(root)
	}
	sort.Strings(roots)
	return roots, nil
}

func (h *Handler) computeKnownUsageByRoot(ctx context.Context, roots []string) (map[string]int64, error) {
	usage := make(map[string]int64, len(roots))
	if len(roots) == 0 {
		return usage, nil
	}
	downloads, err := h.svc.Downloads.List(ctx)
	if err != nil {
		return nil, err
	}
	mediaItems, err := h.svc.Media.List(ctx, "", "")
	if err != nil {
		return nil, err
	}
	for _, root := range roots {
		usage[root] = 0
		for _, d := range downloads {
			if !pathWithinRoot(root, d.DestinationPath) {
				continue
			}
			size := d.SizeBytes
			if size <= 0 {
				size = d.DownloadedBytes
			}
			usage[root] += maxInt64(size, 0)
		}
		for _, item := range mediaItems {
			if !pathWithinRoot(root, item.Path) {
				continue
			}
			usage[root] += maxInt64(item.SizeBytes, 0)
		}
	}
	return usage, nil
}

func pathWithinRoot(root, target string) bool {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(target) == "" {
		return false
	}
	rootAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(target)))
	if err != nil {
		return false
	}
	if rootAbs == targetAbs {
		return true
	}
	prefix := rootAbs + string(filepath.Separator)
	return strings.HasPrefix(targetAbs, prefix)
}

func maxInt64(v, floor int64) int64 {
	if v < floor {
		return floor
	}
	return v
}

func bytesPerSecondToMBps(bytes int64, elapsed time.Duration) float64 {
	if elapsed <= 0 || bytes <= 0 {
		return 0
	}
	mb := float64(bytes) / float64(1024*1024)
	return mb / elapsed.Seconds()
}

func (h *Handler) rescanLibraries(ctx context.Context) {
	settings, err := h.svc.Settings.Get(ctx)
	if err != nil {
		return
	}
	scannedAny := false
	for _, root := range settings.LibraryPaths {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if _, err := h.fs.Stat(root); err != nil {
			continue
		}
		if _, err := h.svc.Media.Scan(ctx, root); err == nil {
			scannedAny = true
		}
	}
	if scannedAny {
		h.maybeAutoIndexAI(ctx, "library_rescan")
	}
}

func (h *Handler) maybeAutoIndexAI(ctx context.Context, reason string) {
	settings, err := h.svc.Settings.Get(ctx)
	if err != nil || !settings.AIEnabled {
		return
	}
	indexCtx, cancel := timeoutContext(ctx, 30*time.Second)
	defer cancel()
	count, err := h.svc.AI.Index(indexCtx)
	if err != nil {
		h.events.Publish(events.Event{
			Type: "ai_index_failed",
			At:   time.Now(),
			Payload: map[string]interface{}{
				"reason": reason,
				"error":  err.Error(),
			},
		})
		return
	}
	h.events.Publish(events.Event{
		Type: "ai_indexed",
		At:   time.Now(),
		Payload: map[string]interface{}{
			"reason": reason,
			"count":  count,
		},
	})
}

func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	var req domain.AppSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	settings, err := h.svc.Settings.Save(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.svc.DLNA != nil {
		h.svc.DLNA.SetEnabled(settings.DLNAEnabled)
	}
	if h.svc.SMB != nil {
		if err := h.svc.SMB.ApplySettings(r.Context(), settings.SMBEnabled, smb.Config{
			ShareName: settings.SMBShareName,
			SharePath: settings.SMBSharePath,
		}); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *Handler) GetDLNADevices(w http.ResponseWriter, r *http.Request) {
	if h.svc.DLNA == nil {
		writeJSON(w, http.StatusOK, dlna.Snapshot{Enabled: false, Devices: []dlna.Device{}})
		return
	}
	writeJSON(w, http.StatusOK, h.svc.DLNA.Snapshot())
}

func (h *Handler) PostDLNAScan(w http.ResponseWriter, r *http.Request) {
	if h.svc.DLNA == nil {
		writeJSON(w, http.StatusOK, dlna.Snapshot{Enabled: false, Devices: []dlna.Device{}})
		return
	}
	ctx, cancel := timeoutContext(r.Context(), 2*time.Second)
	defer cancel()
	if _, err := h.svc.DLNA.Scan(ctx); err != nil && !errors.Is(err, dlna.ErrDisabled) {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.svc.DLNA.Snapshot())
}

func (h *Handler) GetDLNADebug(w http.ResponseWriter, r *http.Request) {
	if h.svc.DLNA == nil {
		writeJSON(w, http.StatusOK, dlna.DebugSnapshot{Enabled: false, RecentEvents: []dlna.DebugEvent{}})
		return
	}
	writeJSON(w, http.StatusOK, h.svc.DLNA.DebugSnapshot())
}

func (h *Handler) GetSMBStatus(w http.ResponseWriter, r *http.Request) {
	if h.svc.SMB == nil {
		writeJSON(w, http.StatusOK, smb.Status{Enabled: false, Running: false, Backend: "none", ShareName: "shelfy", Clients: []smb.Client{}})
		return
	}
	_ = h.svc.SMB.Refresh(r.Context())
	writeJSON(w, http.StatusOK, h.svc.SMB.Snapshot())
}

func (h *Handler) PostSMBRefresh(w http.ResponseWriter, r *http.Request) {
	if h.svc.SMB == nil {
		writeJSON(w, http.StatusOK, smb.Status{Enabled: false, Running: false, Backend: "none", ShareName: "shelfy", Clients: []smb.Client{}})
		return
	}
	if err := h.svc.SMB.Refresh(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.svc.SMB.Snapshot())
}

func (h *Handler) GetDLNADeviceDescription(w http.ResponseWriter, r *http.Request) {
	baseURL := requestBaseURL(r)
	hostID := dlnaHostIdentifier(r.Host)
	if h.svc.DLNA != nil {
		h.svc.DLNA.ObserveClient(r.RemoteAddr, r.UserAgent(), "dlna-device-description")
	}
	payload := dlna.BuildDeviceDescriptionXML(baseURL, "shelfy", hostID)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(payload))
}

func (h *Handler) GetDLNAContentDirectorySCPD(w http.ResponseWriter, r *http.Request) {
	if h.svc.DLNA != nil {
		h.svc.DLNA.ObserveClient(r.RemoteAddr, r.UserAgent(), "dlna-scpd")
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dlna.BuildContentDirectorySCPD()))
}

func (h *Handler) GetDLNAConnectionManagerSCPD(w http.ResponseWriter, r *http.Request) {
	if h.svc.DLNA != nil {
		h.svc.DLNA.ObserveClient(r.RemoteAddr, r.UserAgent(), "dlna-connman-scpd")
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dlna.BuildConnectionManagerSCPD()))
}

func (h *Handler) PostDLNAContentDirectoryControl(w http.ResponseWriter, r *http.Request) {
	if h.svc.DLNA != nil {
		h.svc.DLNA.ObserveClient(r.RemoteAddr, r.UserAgent(), "dlna-control")
	}
	if h.svc.DLNA != nil && !h.svc.DLNA.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "dlna disabled")
		return
	}
	if !dlna.IsBrowseAction(r.Header.Get("SOAPACTION")) {
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(dlna.BuildSOAPFault("401", "Invalid Action")))
		return
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	objectID, browseFlag, start, requested, err := dlna.ParseBrowseRequest(rawBody)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid browse request")
		return
	}

	baseURL := requestBaseURL(r)
	entries, listErr := h.svc.Files.List(r.Context())
	if listErr != nil {
		writeError(w, http.StatusInternalServerError, listErr.Error())
		return
	}
	roots, rootsErr := h.managedRoots(r.Context())
	if rootsErr != nil || len(roots) == 0 {
		roots = []string{h.mediaDir}
	}
	items, found := makeDLNABrowseItems(baseURL, entries, roots, objectID, browseFlag)
	if !found {
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(dlna.BuildSOAPFault("701", "No Such Object")))
		return
	}
	if strings.EqualFold(strings.TrimSpace(browseFlag), "BrowseMetadata") {
		start = 0
		requested = 0
	}

	response := dlna.BuildBrowseSOAP(items, start, requested)
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.Header().Set("EXT", "")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(response))
}

func (h *Handler) PostDLNAConnectionManagerControl(w http.ResponseWriter, r *http.Request) {
	if h.svc.DLNA != nil {
		h.svc.DLNA.ObserveClient(r.RemoteAddr, r.UserAgent(), "dlna-connman-control")
	}
	if h.svc.DLNA != nil && !h.svc.DLNA.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "dlna disabled")
		return
	}
	soapAction := r.Header.Get("SOAPACTION")
	var response string
	switch {
	case dlna.IsConnectionManagerAction(soapAction, "GetProtocolInfo"):
		response = dlna.BuildGetProtocolInfoSOAP([]string{
			"http-get:*:video/mp4:*",
			"http-get:*:video/x-matroska:*",
			"http-get:*:audio/mpeg:*",
			"http-get:*:audio/flac:*",
			"http-get:*:image/jpeg:*",
			"http-get:*:image/png:*",
			"http-get:*:application/pdf:*",
		})
	case dlna.IsConnectionManagerAction(soapAction, "GetCurrentConnectionIDs"):
		response = dlna.BuildGetCurrentConnectionIDsSOAP()
	case dlna.IsConnectionManagerAction(soapAction, "GetCurrentConnectionInfo"):
		response = dlna.BuildGetCurrentConnectionInfoSOAP()
	default:
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(dlna.BuildSOAPFault("401", "Invalid Action")))
		return
	}
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.Header().Set("EXT", "")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(response))
}

func (h *Handler) HandleDLNAEvent(w http.ResponseWriter, r *http.Request) {
	if h.svc.DLNA != nil {
		h.svc.DLNA.ObserveClient(r.RemoteAddr, r.UserAgent(), "dlna-event")
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) GetDLNAMedia(w http.ResponseWriter, r *http.Request) {
	if h.svc.DLNA != nil {
		h.svc.DLNA.ObserveClient(r.RemoteAddr, r.UserAgent(), "dlna-media")
	}
	if h.svc.DLNA != nil && !h.svc.DLNA.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "dlna disabled")
		return
	}
	entry, err := h.svc.Files.Resolve(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	file, err := h.fs.OpenFile(entry.Path, os.O_RDONLY, 0)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rng, err := stream.ParseRangeHeader(r.Header.Get("Range"), info.Size())
	if err != nil {
		writeError(w, http.StatusRequestedRangeNotSatisfiable, err.Error())
		return
	}
	stream.WriteRangeHeaders(w, rng, info.Size(), entry.MimeType)
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("contentFeatures.dlna.org", "DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01500000000000000000000000000000")
	if _, err := file.Seek(rng.Start, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = io.CopyN(w, file, rng.End-rng.Start+1)
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwarded != "" {
		scheme = strings.Split(forwarded, ",")[0]
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		host = "localhost:8080"
	}
	return scheme + "://" + host
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if forwarded == "" {
		return false
	}
	return strings.EqualFold(strings.Split(forwarded, ",")[0], "https")
}

func dlnaHostIdentifier(host string) string {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		trimmed = "localhost"
	}
	h := fnv.New128a()
	_, _ = h.Write([]byte(trimmed))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func makeDLNAItems(baseURL string, entries []fileserver.Entry) []dlna.ContentItem {
	out := make([]dlna.ContentItem, 0, len(entries))
	for _, entry := range entries {
		class := "object.item"
		switch strings.ToLower(strings.TrimSpace(entry.Kind)) {
		case "video":
			class = "object.item.videoItem"
		case "audio":
			class = "object.item.audioItem.musicTrack"
		case "image":
			class = "object.item.imageItem.photo"
		}
		out = append(out, dlna.ContentItem{
			ID:       entry.ID,
			ParentID: "0",
			Title:    entry.Name,
			Class:    class,
			MimeType: entry.MimeType,
			Size:     entry.SizeBytes,
			URL:      strings.TrimRight(baseURL, "/") + "/dlna/media/" + entry.ID,
		})
	}
	return out
}

type dlnaBrowseIndex struct {
	Metadata map[string]dlna.ContentItem
	Children map[string][]dlna.ContentItem
}

func makeDLNABrowseItems(baseURL string, entries []fileserver.Entry, roots []string, objectID, browseFlag string) ([]dlna.ContentItem, bool) {
	index := buildDLNABrowseIndex(baseURL, entries, roots)
	targetID := strings.TrimSpace(objectID)
	if targetID == "" {
		targetID = "0"
	}
	if strings.EqualFold(strings.TrimSpace(browseFlag), "BrowseMetadata") {
		item, ok := index.Metadata[targetID]
		if !ok {
			return nil, false
		}
		return []dlna.ContentItem{item}, true
	}
	if children, ok := index.Children[targetID]; ok {
		out := append([]dlna.ContentItem{}, children...)
		return out, true
	}
	if _, ok := index.Metadata[targetID]; ok {
		return []dlna.ContentItem{}, true
	}
	return nil, false
}

func buildDLNABrowseIndex(baseURL string, entries []fileserver.Entry, roots []string) dlnaBrowseIndex {
	index := dlnaBrowseIndex{
		Metadata: map[string]dlna.ContentItem{},
		Children: map[string][]dlna.ContentItem{"0": {}},
	}
	addedByParent := map[string]map[string]struct{}{}
	ensureParent := func(parentID string) {
		if _, ok := index.Children[parentID]; !ok {
			index.Children[parentID] = []dlna.ContentItem{}
		}
		if _, ok := addedByParent[parentID]; !ok {
			addedByParent[parentID] = map[string]struct{}{}
		}
	}
	appendChild := func(parentID string, child dlna.ContentItem) {
		ensureParent(parentID)
		if _, exists := addedByParent[parentID][child.ID]; exists {
			return
		}
		index.Children[parentID] = append(index.Children[parentID], child)
		addedByParent[parentID][child.ID] = struct{}{}
	}

	index.Metadata["0"] = dlna.ContentItem{
		ID:          "0",
		ParentID:    "-1",
		Title:       "shelfy",
		Class:       "object.container.storageFolder",
		IsContainer: true,
	}

	normalizedRoots := normalizeDLNARoots(roots)
	if len(normalizedRoots) == 0 {
		for _, entry := range entries {
			if strings.TrimSpace(entry.Path) == "" {
				continue
			}
			normalizedRoots = append(normalizedRoots, filepath.Dir(entry.Path))
		}
		normalizedRoots = normalizeDLNARoots(normalizedRoots)
	}
	multiRoot := len(normalizedRoots) > 1
	if len(normalizedRoots) == 0 {
		normalizedRoots = []string{""}
	}

	for rootIdx, rootPath := range normalizedRoots {
		rootID := dlnaRootObjectID(rootIdx)
		parentID := "0"
		if multiRoot {
			title := filepath.Base(rootPath)
			if title == "" || title == "." || title == string(filepath.Separator) {
				title = rootPath
			}
			rootItem := dlna.ContentItem{
				ID:          rootID,
				ParentID:    "0",
				Title:       title,
				Class:       "object.container.storageFolder",
				IsContainer: true,
			}
			index.Metadata[rootID] = rootItem
			appendChild("0", rootItem)
			parentID = rootID
		}
		_ = parentID
		for _, entry := range entries {
			relParts, ok := dlnaRelativeParts(rootPath, entry.Path)
			if !ok || len(relParts) == 0 {
				continue
			}
			currentParent := "0"
			if multiRoot {
				currentParent = rootID
			}
			dirs := relParts[:len(relParts)-1]
			pathParts := make([]string, 0, len(dirs))
			for _, dir := range dirs {
				pathParts = append(pathParts, dir)
				containerID := dlnaDirObjectID(rootIdx, pathParts)
				if _, exists := index.Metadata[containerID]; !exists {
					container := dlna.ContentItem{
						ID:          containerID,
						ParentID:    currentParent,
						Title:       dir,
						Class:       "object.container.storageFolder",
						IsContainer: true,
					}
					index.Metadata[containerID] = container
					appendChild(currentParent, container)
				}
				currentParent = containerID
			}

			fileID := strings.TrimSpace(entry.ID)
			if fileID == "" {
				continue
			}
			item := dlna.ContentItem{
				ID:          fileID,
				ParentID:    currentParent,
				Title:       entry.Name,
				Class:       dlnaClassForKind(entry.Kind),
				MimeType:    entry.MimeType,
				Size:        entry.SizeBytes,
				URL:         strings.TrimRight(baseURL, "/") + "/dlna/media/" + fileID,
				IsContainer: false,
			}
			index.Metadata[fileID] = item
			appendChild(currentParent, item)
		}
	}

	for id, item := range index.Metadata {
		if !item.IsContainer {
			continue
		}
		item.ChildCount = len(index.Children[id])
		index.Metadata[id] = item
	}
	for parentID, children := range index.Children {
		for i := range children {
			if updated, ok := index.Metadata[children[i].ID]; ok {
				children[i] = updated
			}
		}
		sort.Slice(children, func(i, j int) bool {
			if children[i].IsContainer != children[j].IsContainer {
				return children[i].IsContainer
			}
			left := strings.ToLower(strings.TrimSpace(children[i].Title))
			right := strings.ToLower(strings.TrimSpace(children[j].Title))
			if left == right {
				return children[i].ID < children[j].ID
			}
			return left < right
		})
		index.Children[parentID] = children
	}
	return index
}

func normalizeDLNARoots(roots []string) []string {
	out := make([]string, 0, len(roots))
	seen := map[string]struct{}{}
	for _, raw := range roots {
		cleaned := strings.TrimSpace(raw)
		if cleaned == "" {
			continue
		}
		abs, err := filepath.Abs(filepath.Clean(cleaned))
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	sort.Strings(out)
	return out
}

func dlnaRelativeParts(rootPath, entryPath string) ([]string, bool) {
	entryAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(entryPath)))
	if err != nil {
		return nil, false
	}
	if resolved, err := filepath.EvalSymlinks(entryAbs); err == nil {
		entryAbs = resolved
	}
	if strings.TrimSpace(rootPath) == "" {
		base := filepath.Base(entryAbs)
		if base == "" || base == "." {
			return nil, false
		}
		return []string{base}, true
	}
	rootAbs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(rootPath)))
	if err != nil {
		return nil, false
	}
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = resolved
	}
	rel, err := filepath.Rel(rootAbs, entryAbs)
	if err != nil {
		return nil, false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || strings.HasPrefix(rel, "../") || rel == ".." {
		return nil, false
	}
	parts := make([]string, 0, 4)
	for _, part := range strings.Split(rel, "/") {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return nil, false
	}
	return parts, true
}

func dlnaRootObjectID(rootIdx int) string {
	return "root:" + strconv.Itoa(rootIdx)
}

func dlnaDirObjectID(rootIdx int, parts []string) string {
	if len(parts) == 0 {
		return dlnaRootObjectID(rootIdx)
	}
	return "dir:" + strconv.Itoa(rootIdx) + ":" + strings.Join(parts, "/")
}

func containerIDToPath(id string, rootIdx int) []string {
	prefix := "dir:" + strconv.Itoa(rootIdx) + ":"
	if !strings.HasPrefix(id, prefix) {
		return nil
	}
	rest := strings.TrimPrefix(id, prefix)
	if strings.TrimSpace(rest) == "" {
		return nil
	}
	out := make([]string, 0, 4)
	for _, part := range strings.Split(rest, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func dlnaClassForKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "video":
		return "object.item.videoItem"
	case "audio":
		return "object.item.audioItem.musicTrack"
	case "image":
		return "object.item.imageItem.photo"
	default:
		return "object.item"
	}
}

func (h *Handler) GetFiles(w http.ResponseWriter, r *http.Request) {
	if !fileserver.Authorized(r, h.fileUser, h.filePass) {
		w.Header().Set("WWW-Authenticate", `Basic realm="files"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	files, err := h.svc.Files.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		out = append(out, map[string]interface{}{
			"id":          file.ID,
			"name":        file.Name,
			"kind":        file.Kind,
			"path":        file.Path,
			"mimeType":    file.MimeType,
			"sizeBytes":   file.SizeBytes,
			"streamUrl":   "/files/" + file.ID + "/stream",
			"downloadUrl": "/files/" + file.ID + "/download",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) GetFileStream(w http.ResponseWriter, r *http.Request) {
	if !fileserver.Authorized(r, h.fileUser, h.filePass) {
		w.Header().Set("WWW-Authenticate", `Basic realm="files"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	entry, err := h.svc.Files.Resolve(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	file, err := h.fs.OpenFile(entry.Path, os.O_RDONLY, 0)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rng, err := stream.ParseRangeHeader(r.Header.Get("Range"), info.Size())
	if err != nil {
		writeError(w, http.StatusRequestedRangeNotSatisfiable, err.Error())
		return
	}
	stream.WriteRangeHeaders(w, rng, info.Size(), entry.MimeType)
	if _, err := file.Seek(rng.Start, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = io.CopyN(w, file, rng.End-rng.Start+1)
}

func (h *Handler) GetFileDownload(w http.ResponseWriter, r *http.Request) {
	if !fileserver.Authorized(r, h.fileUser, h.filePass) {
		w.Header().Set("WWW-Authenticate", `Basic realm="files"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	entry, err := h.svc.Files.Resolve(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	file, err := h.fs.OpenFile(entry.Path, os.O_RDONLY, 0)
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", entry.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileserver.BuildDownloadFilename(entry)))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

func decodeBody[T any](r *http.Request) (T, error) {
	var req T
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}

func timeoutContext(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
	if duration <= 0 {
		duration = 30 * time.Second
	}
	return context.WithTimeout(parent, duration)
}

var _ = fmt.Sprintf
var _ = ai.FakeEmbeddingProvider{}
var _ = media.Service{}
