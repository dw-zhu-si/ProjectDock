package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"projectdock/internal/ai"
	"projectdock/internal/apiprobe"
	"projectdock/internal/config"
	"projectdock/internal/folders"
	"projectdock/internal/installer"
	"projectdock/internal/model"
	"projectdock/internal/ports"
	"projectdock/internal/projects"
	"projectdock/internal/sandboxbookmark"
	"projectdock/internal/store"
	"projectdock/internal/widget"
	webassets "projectdock/web"
)

type Server struct {
	store           *store.Store
	ports           *ports.Service
	projects        *projects.Service
	probe           *apiprobe.Service
	picker          folders.Picker
	directoryPicker folders.DirectoryPicker
	ai              *ai.Service
	github          installer.RepositoryInstaller
	bookmarks       *sandboxbookmark.Manager
	capabilities    Capabilities
	token           string
	version         string
	logger          *log.Logger
}

type Snapshot struct {
	GeneratedAt  time.Time               `json:"generatedAt"`
	Projects     []projects.ProjectView  `json:"projects"`
	Ports        []model.PortListener    `json:"ports"`
	PortPool     ports.PoolSnapshot      `json:"portPool"`
	Reservations []model.PortReservation `json:"reservations"`
	Audit        []model.AuditEvent      `json:"audit"`
	IgnoredCount int                     `json:"ignoredCount"`
	Capabilities Capabilities            `json:"capabilities"`
}

type Capabilities struct {
	AppStore         bool `json:"appStore"`
	PortMonitoring   bool `json:"portMonitoring"`
	ProjectLifecycle bool `json:"projectLifecycle"`
	FullDelete       bool `json:"fullDelete"`
	RegistrySync     bool `json:"registrySync"`
	GitHubInstall    bool `json:"githubInstall"`
	DirectoryAccess  bool `json:"directoryAccess"`
	APIProbe         bool `json:"apiProbe"`
}

type Options struct {
	AppStore bool
}

type importRequest struct {
	Paths  []string `json:"paths"`
	Source string   `json:"source,omitempty"`
}

type importItem struct {
	Path    string               `json:"path"`
	Status  string               `json:"status"`
	Message string               `json:"message,omitempty"`
	Result  *projects.SyncResult `json:"result,omitempty"`
}

type importReport struct {
	Imported int          `json:"imported"`
	Skipped  int          `json:"skipped"`
	Items    []importItem `json:"items"`
}

func New(st *store.Store, portService *ports.Service, projectService *projects.Service, probeService *apiprobe.Service, logger *log.Logger, appVersion ...string) (*Server, error) {
	return NewWithOptions(st, portService, projectService, probeService, logger, Options{}, appVersion...)
}

func NewWithOptions(st *store.Store, portService *ports.Service, projectService *projects.Service, probeService *apiprobe.Service, logger *log.Logger, options Options, appVersion ...string) (*Server, error) {
	token, err := sessionToken()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = log.Default()
	}
	version := "development"
	if len(appVersion) > 0 && strings.TrimSpace(appVersion[0]) != "" {
		version = strings.TrimSpace(appVersion[0])
	}
	capabilities := Capabilities{
		AppStore: options.AppStore, PortMonitoring: !options.AppStore,
		ProjectLifecycle: !options.AppStore, FullDelete: !options.AppStore,
		RegistrySync: !options.AppStore, GitHubInstall: true,
		DirectoryAccess: true, APIProbe: true,
	}
	var bookmarks *sandboxbookmark.Manager
	var github installer.RepositoryInstaller = installer.GitHubInstaller{}
	if options.AppStore {
		bookmarks, err = sandboxbookmark.New(st.Dir())
		if err != nil {
			return nil, err
		}
		github = installer.ZIPInstaller{}
	}
	return &Server{
		store: st, ports: portService, projects: projectService, probe: probeService,
		picker: folders.NewPicker(), directoryPicker: folders.NewDirectoryPicker(),
		ai: ai.NewService(st.Dir()), github: github, bookmarks: bookmarks,
		capabilities: capabilities, token: token, version: version, logger: logger,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /api/widget-snapshot", s.handleWidgetSnapshot)
	mux.HandleFunc("GET /api/projects", s.handleProjectsList)
	mux.HandleFunc("POST /api/projects", s.requireMutation(s.handleProjectUpsert))
	mux.HandleFunc("POST /api/projects/import", s.requireMutation(s.handleProjectImport))
	mux.HandleFunc("POST /api/projects/pick", s.requireMutation(s.handleProjectPick))
	mux.HandleFunc("POST /api/projects/scan", s.requireMutation(s.handleProjectScan))
	mux.HandleFunc("POST /api/directories/pick", s.requireMutation(s.handleDirectoryPick))
	mux.HandleFunc("POST /api/directories/authorize", s.requireMutation(s.handleDirectoryAuthorize))
	mux.HandleFunc("GET /api/settings/ai", s.handleAISettingsGet)
	mux.HandleFunc("PUT /api/settings/ai", s.requireMutation(s.handleAISettingsSave))
	mux.HandleFunc("POST /api/settings/ai/verify", s.requireMutation(s.handleAISettingsVerify))
	mux.HandleFunc("POST /api/github/install", s.requireMutation(s.handleGitHubInstall))
	mux.HandleFunc("POST /api/projects/sync-registry", s.requireMutation(s.handleProjectRegistrySync))
	mux.HandleFunc("PUT /api/projects/{id}", s.requireMutation(s.handleProjectUpsert))
	mux.HandleFunc("DELETE /api/projects/{id}", s.requireMutation(s.handleProjectDelete))
	mux.HandleFunc("POST /api/projects/{id}/delete", s.requireMutation(s.handleProjectDeleteChoice))
	mux.HandleFunc("POST /api/projects/{id}/start", s.requireMutation(s.handleProjectStart))
	mux.HandleFunc("POST /api/projects/{id}/stop", s.requireMutation(s.handleProjectStop))
	mux.HandleFunc("GET /api/projects/{id}/logs", s.handleProjectLogs)
	mux.HandleFunc("GET /api/ports", s.handlePorts)
	mux.HandleFunc("GET /api/ports/pool", s.handlePortPool)
	mux.HandleFunc("POST /api/ports/allocations", s.requireMutation(s.handleAllocationCreate))
	mux.HandleFunc("DELETE /api/ports/allocations/{port}", s.requireMutation(s.handleAllocationDelete))
	mux.HandleFunc("GET /api/reservations", s.handleReservations)
	mux.HandleFunc("POST /api/reservations", s.requireMutation(s.handleReservationCreate))
	mux.HandleFunc("DELETE /api/reservations/{port}", s.requireMutation(s.handleReservationDelete))
	mux.HandleFunc("POST /api/probe", s.requireMutation(s.handleProbe))
	mux.HandleFunc("GET /api/audit", s.handleAudit)

	public, err := fs.Sub(webassets.Files, ".")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(public))
	mux.Handle("/", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		fileServer.ServeHTTP(writer, request)
	}))
	return s.securityHeaders(s.hostGuard(mux))
}

func (s *Server) Serve(ctx context.Context, address string) error {
	if err := model.ValidateLoopbackListen(address); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("监听 ProjectDock: %w", err)
	}
	if s.capabilities.RegistrySync {
		go s.registrySyncLoop(ctx)
	}
	if s.bookmarks != nil {
		defer s.bookmarks.Close()
	}
	httpServer := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       25 * time.Second,
		WriteTimeout:      6 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	shutdownDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdownCtx)
		case <-shutdownDone:
		}
	}()
	err = httpServer.Serve(listener)
	close(shutdownDone)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) handleHealth(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"status": "ok", "service": "projectdock", "version": s.version, "time": time.Now(), "capabilities": s.capabilities,
	})
}

func (s *Server) handleSession(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, map[string]string{"token": s.token})
}

func (s *Server) handleSnapshot(writer http.ResponseWriter, request *http.Request) {
	listeners := []model.PortListener{}
	var err error
	if s.capabilities.PortMonitoring {
		listeners, err = s.ports.Observe(request.Context())
	}
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "snapshot_ports", err)
		return
	}
	projectViews, err := s.projects.ListWithListeners(request.Context(), listeners)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "snapshot_projects", err)
		return
	}
	if s.capabilities.ProjectLifecycle {
		projectViews = projects.VisibleInControlPanel(projectViews)
	}
	registry, err := s.store.Load(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "snapshot_registry", err)
		return
	}
	audit := newestAudit(registry.Audit, 100)
	portPool := ports.PoolSnapshot{From: 3000, To: 49999, Allocations: []ports.PortAllocation{}, Reservations: []model.PortReservation{}, UnassignedListeners: []model.PortListener{}, Suggestions: []int{}}
	reservations := []model.PortReservation{}
	if s.capabilities.PortMonitoring {
		portPool, err = s.ports.PoolWithListeners(request.Context(), listeners, 3000, 49999, 20)
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "snapshot_port_pool", err)
			return
		}
		reservations = registry.Reservations
	}
	writeJSON(writer, http.StatusOK, Snapshot{
		GeneratedAt: time.Now(), Projects: projectViews, Ports: listeners,
		PortPool: portPool, Reservations: reservations,
		Audit: audit, IgnoredCount: len(registry.IgnoredPaths), Capabilities: s.capabilities,
	})
}

func (s *Server) handleWidgetSnapshot(writer http.ResponseWriter, request *http.Request) {
	listeners := []model.PortListener{}
	var err error
	if s.capabilities.PortMonitoring {
		listeners, err = s.ports.Observe(request.Context())
	}
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "widget_ports", err)
		return
	}
	projectViews, err := s.projects.ListWithListeners(request.Context(), listeners)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "widget_projects", err)
		return
	}
	if s.capabilities.ProjectLifecycle {
		projectViews = projects.VisibleInControlPanel(projectViews)
	}
	registry, err := s.store.Load(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "widget_registry", err)
		return
	}
	allocations := []ports.PortAllocation{}
	reservations := []model.PortReservation{}
	if s.capabilities.PortMonitoring {
		allocations, err = s.ports.ListAllocationsWithListeners(request.Context(), listeners)
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "widget_allocations", err)
			return
		}
		reservations = registry.Reservations
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, widget.Build(projectViews, listeners, allocations, reservations, time.Now()))
}

func (s *Server) handleProjectsList(writer http.ResponseWriter, request *http.Request) {
	var result []projects.ProjectView
	var err error
	if s.capabilities.PortMonitoring {
		result, err = s.projects.List(request.Context())
	} else {
		result, err = s.projects.ListWithListeners(request.Context(), []model.PortListener{})
	}
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "projects_list", err)
		return
	}
	if s.capabilities.ProjectLifecycle {
		result = projects.VisibleInControlPanel(result)
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) handleProjectUpsert(writer http.ResponseWriter, request *http.Request) {
	var project model.Project
	if err := decodeJSON(request, &project); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_project", err)
		return
	}
	if id := request.PathValue("id"); id != "" {
		if project.ID != "" && project.ID != id {
			writeError(writer, http.StatusBadRequest, "project_id_mismatch", errors.New("路径和正文中的项目 ID 不一致"))
			return
		}
		project.ID = id
	}
	if s.capabilities.AppStore {
		if !s.pathAuthorized(project.Path) {
			writeError(writer, http.StatusForbidden, "directory_authorization_required", errors.New("请先用系统目录选择器授权该项目路径"))
			return
		}
		project.Ports = nil
		project.StartCommand = ""
		project.StopCommand = ""
	}
	saved, err := s.projects.Upsert(request.Context(), project)
	if err != nil {
		s.appendAudit(request.Context(), "project.upsert", "failed", project.ID, 0, map[string]any{"error": err.Error()})
		writeError(writer, http.StatusBadRequest, "project_save_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, saved)
}

func (s *Server) handleProjectImport(writer http.ResponseWriter, request *http.Request) {
	var input importRequest
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_import", err)
		return
	}
	if s.capabilities.AppStore {
		for _, path := range input.Paths {
			if !s.pathAuthorized(path) {
				writeError(writer, http.StatusForbidden, "directory_authorization_required", errors.New("导入路径不在用户已授权目录内"))
				return
			}
		}
	}
	report := s.importPaths(request.Context(), input.Paths, input.Source)
	status := http.StatusOK
	if report.Imported == 0 && report.Skipped > 0 {
		status = http.StatusBadRequest
	}
	writeJSON(writer, status, report)
}

func (s *Server) handleProjectPick(writer http.ResponseWriter, request *http.Request) {
	if s.capabilities.AppStore {
		writeError(writer, http.StatusNotImplemented, "native_picker_required", errors.New("商店版必须使用原生目录选择器"))
		return
	}
	var input struct{}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_picker_request", err)
		return
	}
	paths, err := s.picker.Pick(request.Context())
	if err != nil {
		writeError(writer, http.StatusBadRequest, "folder_pick_failed", err)
		return
	}
	report := s.importPaths(request.Context(), paths, "manual")
	if report.Imported == 0 {
		writeJSON(writer, http.StatusBadRequest, report)
		return
	}
	writeJSON(writer, http.StatusOK, report)
}

func (s *Server) handleDirectoryPick(writer http.ResponseWriter, request *http.Request) {
	if s.capabilities.AppStore {
		writeError(writer, http.StatusNotImplemented, "native_picker_required", errors.New("商店版必须使用原生目录选择器"))
		return
	}
	var input struct {
		Purpose string `json:"purpose"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_directory_picker", err)
		return
	}
	prompt := "选择 ProjectDock 要扫描的父目录"
	if input.Purpose == "install" {
		prompt = "选择 GitHub 项目的安装目录"
	}
	path, err := s.directoryPicker.PickOne(request.Context(), prompt)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "directory_pick_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"path": strings.TrimSuffix(path, string(filepath.Separator))})
}

func (s *Server) handleDirectoryAuthorize(writer http.ResponseWriter, request *http.Request) {
	if !s.capabilities.AppStore || s.bookmarks == nil {
		writeError(writer, http.StatusNotImplemented, "directory_authorization_unavailable", errors.New("当前版本不需要安全作用域书签"))
		return
	}
	var input struct {
		Bookmark string `json:"bookmark"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_directory_authorization", err)
		return
	}
	path, err := s.bookmarks.Authorize(input.Bookmark)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "directory_authorization_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"path": path})
}

func (s *Server) handleProjectScan(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Root string `json:"root"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_scan", err)
		return
	}
	if s.capabilities.AppStore && !s.pathAuthorized(input.Root) {
		writeError(writer, http.StatusForbidden, "directory_authorization_required", errors.New("请先用系统目录选择器授权扫描目录"))
		return
	}
	report, err := s.projects.Scan(request.Context(), input.Root)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "project_scan_failed", err)
		return
	}
	s.appendAudit(request.Context(), "project.scan", "success", "", 0, map[string]any{"root": report.Root, "candidates": len(report.Candidates), "truncated": report.Truncated})
	writeJSON(writer, http.StatusOK, report)
}

func (s *Server) handleAISettingsGet(writer http.ResponseWriter, request *http.Request) {
	settings, err := s.ai.Get(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "ai_settings_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, settings)
}

func (s *Server) handleAISettingsSave(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		BaseURL string `json:"baseUrl"`
		Model   string `json:"model"`
		APIKey  string `json:"apiKey"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_ai_settings", err)
		return
	}
	settings, err := s.ai.Save(request.Context(), ai.Settings{BaseURL: input.BaseURL, Model: input.Model}, input.APIKey)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "ai_settings_save_failed", err)
		return
	}
	s.appendAudit(request.Context(), "settings.ai", "success", "", 0, map[string]any{"baseUrl": settings.BaseURL, "model": settings.Model, "configured": settings.Configured})
	writeJSON(writer, http.StatusOK, settings)
}

func (s *Server) handleAISettingsVerify(writer http.ResponseWriter, request *http.Request) {
	var input struct{}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_ai_verification", err)
		return
	}
	settings, err := s.ai.Verify(request.Context())
	if err != nil {
		s.appendAudit(request.Context(), "settings.ai.verify", "failed", "", 0, map[string]any{"status": settings.VerificationStatus, "error": err.Error()})
		writeError(writer, http.StatusBadGateway, "ai_verification_failed", err)
		return
	}
	s.appendAudit(request.Context(), "settings.ai.verify", "success", "", 0, map[string]any{"status": settings.VerificationStatus})
	writeJSON(writer, http.StatusOK, settings)
}

func (s *Server) handleGitHubInstall(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		URL         string `json:"url"`
		InstallRoot string `json:"installRoot"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_github_install", err)
		return
	}
	settings, err := s.ai.Get(request.Context())
	if err != nil || !settings.Configured {
		writeError(writer, http.StatusPreconditionFailed, "ai_configuration_required", errors.New("使用 GitHub 安装前，请先配置 AI 模型；远程接口还需要 API 密钥"))
		return
	}
	if !settings.Usable {
		writeError(writer, http.StatusPreconditionFailed, "ai_verification_required", errors.New("使用 GitHub 安装前，请先在 AI 设置中完成连接验证"))
		return
	}
	repository, err := installer.ParseGitHubURL(input.URL)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_github_url", err)
		return
	}
	installContext, cancel := context.WithTimeout(request.Context(), 5*time.Minute)
	defer cancel()
	if s.capabilities.AppStore && !s.pathAuthorized(input.InstallRoot) {
		writeError(writer, http.StatusForbidden, "directory_authorization_required", errors.New("请先用系统目录选择器授权安装目录"))
		return
	}
	destination, err := s.github.Install(installContext, repository, input.InstallRoot)
	if err != nil {
		s.appendAudit(request.Context(), "github.install", "failed", "", 0, map[string]any{"repository": repository.URL, "destination": destination, "error": err.Error()})
		writeError(writer, http.StatusBadRequest, "github_clone_failed", err)
		return
	}
	analysis, analysisErr := s.ai.AnalyzeRepository(installContext, repository.URL, installer.CollectMetadata(destination))
	result, syncErr := s.projects.SyncPath(request.Context(), projects.SyncInput{
		Path: destination, Name: repository.Name, Source: "other", DiscoveredBy: "github-install", Revive: true,
	})
	if syncErr != nil {
		s.appendAudit(request.Context(), "github.install", "failed", "", 0, map[string]any{"repository": repository.URL, "destination": destination, "error": syncErr.Error()})
		writeError(writer, http.StatusBadRequest, "github_import_failed", fmt.Errorf("仓库已克隆到 %s，但登记失败: %w", destination, syncErr))
		return
	}
	warning := ""
	if analysisErr != nil {
		warning = "仓库已安装并登记，但 AI 分析失败：" + analysisErr.Error()
	}
	s.appendAudit(request.Context(), "github.install", "success", result.Project.ID, 0, map[string]any{"repository": repository.URL, "destination": destination, "aiAnalyzed": analysisErr == nil})
	writeJSON(writer, http.StatusOK, map[string]any{"repository": repository, "destination": destination, "project": result.Project, "analysis": analysis, "warning": warning})
}

func (s *Server) handleProjectRegistrySync(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.RegistrySync, "registry_sync_unavailable", "Mac App Store 版不读取外部项目注册表") {
		return
	}
	var input struct{}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_sync_request", err)
		return
	}
	path := config.ProjectRegistryPath()
	if path == "" {
		writeError(writer, http.StatusNotFound, "project_registry_missing", errors.New("未找到三端项目注册表，可设置 PROJECTDOCK_PROJECTS_FILE"))
		return
	}
	report, err := s.projects.SyncRegistry(request.Context(), path)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "project_registry_sync_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, report)
}

func (s *Server) handleProjectDelete(writer http.ResponseWriter, request *http.Request) {
	if err := s.projects.Delete(request.Context(), request.PathValue("id")); err != nil {
		s.appendAudit(request.Context(), "project.delete", "failed", request.PathValue("id"), 0, map[string]any{"error": err.Error()})
		writeError(writer, http.StatusBadRequest, "project_delete_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleProjectDeleteChoice(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		RemoveFiles  bool   `json:"removeFiles"`
		Confirmation string `json:"confirmation"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_project_delete", err)
		return
	}
	if input.RemoveFiles && !s.requireCapability(writer, s.capabilities.FullDelete, "full_delete_unavailable", "Mac App Store 版只能移除登记，不会删除磁盘文件") {
		return
	}
	result, err := s.projects.DeleteProject(request.Context(), request.PathValue("id"), input.RemoveFiles, input.Confirmation)
	if err != nil {
		s.appendAudit(request.Context(), "project.delete", "failed", request.PathValue("id"), 0, map[string]any{"filesRequested": input.RemoveFiles, "error": err.Error()})
		writeError(writer, http.StatusBadRequest, "project_delete_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) handleProjectStart(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.ProjectLifecycle, "project_lifecycle_unavailable", "Mac App Store 版不执行任意 Shell 启动命令") {
		return
	}
	status, err := s.projects.Start(request.Context(), request.PathValue("id"))
	if err != nil {
		s.appendAudit(request.Context(), "project.start", "failed", request.PathValue("id"), 0, map[string]any{"error": err.Error()})
		writeError(writer, http.StatusConflict, "project_start_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (s *Server) handleProjectStop(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.ProjectLifecycle, "project_lifecycle_unavailable", "Mac App Store 版不执行任意 Shell 停止命令") {
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	status, err := s.projects.Stop(ctx, request.PathValue("id"))
	if err != nil {
		s.appendAudit(request.Context(), "project.stop", "failed", request.PathValue("id"), 0, map[string]any{"error": err.Error()})
		writeError(writer, http.StatusConflict, "project_stop_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (s *Server) handleProjectLogs(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.ProjectLifecycle, "project_lifecycle_unavailable", "Mac App Store 版没有受管进程日志") {
		return
	}
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	lines, err := s.projects.ReadLog(request.PathValue("id"), limit)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "project_logs_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"lines": lines})
}

func (s *Server) handlePorts(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.PortMonitoring, "port_monitoring_unavailable", "App Sandbox 不允许全系统端口监控") {
		return
	}
	listeners, err := s.ports.List(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "port_scan_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, listeners)
}

func (s *Server) handlePortPool(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.PortMonitoring, "port_monitoring_unavailable", "App Sandbox 不允许全系统端口监控") {
		return
	}
	from, _ := strconv.Atoi(request.URL.Query().Get("from"))
	to, _ := strconv.Atoi(request.URL.Query().Get("to"))
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	result, err := s.ports.Pool(request.Context(), from, to, limit)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "port_pool_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) handleAllocationCreate(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.PortMonitoring, "port_monitoring_unavailable", "Mac App Store 版不提供端口分配") {
		return
	}
	var input struct {
		Port      int    `json:"port"`
		ProjectID string `json:"projectId"`
		Owner     string `json:"owner"`
	}
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_allocation", err)
		return
	}
	saved, err := s.ports.Allocate(request.Context(), input.Port, input.ProjectID, input.Owner)
	if err != nil {
		s.appendAudit(request.Context(), "port.allocate", "failed", input.ProjectID, input.Port, map[string]any{"error": err.Error()})
		writeError(writer, http.StatusConflict, "allocation_failed", err)
		return
	}
	s.appendAudit(request.Context(), "port.allocate", "success", input.ProjectID, input.Port, map[string]any{"owner": input.Owner})
	writeJSON(writer, http.StatusOK, saved)
}

func (s *Server) handleAllocationDelete(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.PortMonitoring, "port_monitoring_unavailable", "Mac App Store 版不提供端口分配") {
		return
	}
	portNumber, err := strconv.Atoi(request.PathValue("port"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_port", errors.New("端口必须是数字"))
		return
	}
	projectID := request.URL.Query().Get("project")
	if projectID == "" {
		writeError(writer, http.StatusBadRequest, "project_required", errors.New("取消分配时必须提供 project"))
		return
	}
	if err := s.ports.Unassign(request.Context(), portNumber, projectID); err != nil {
		s.appendAudit(request.Context(), "port.unassign", "failed", projectID, portNumber, map[string]any{"error": err.Error()})
		writeError(writer, http.StatusBadRequest, "allocation_delete_failed", err)
		return
	}
	s.appendAudit(request.Context(), "port.unassign", "success", projectID, portNumber, nil)
	writeJSON(writer, http.StatusOK, map[string]bool{"unassigned": true})
}

func (s *Server) handleReservations(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.PortMonitoring, "port_monitoring_unavailable", "Mac App Store 版不提供端口预留") {
		return
	}
	registry, err := s.store.Load(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "reservations_load_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, registry.Reservations)
}

func (s *Server) handleReservationCreate(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.PortMonitoring, "port_monitoring_unavailable", "Mac App Store 版不提供端口预留") {
		return
	}
	var reservation model.PortReservation
	if err := decodeJSON(request, &reservation); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_reservation", err)
		return
	}
	saved, err := s.ports.Reserve(request.Context(), reservation)
	if err != nil {
		s.appendAudit(request.Context(), "port.reserve", "failed", reservation.ProjectID, reservation.Port, map[string]any{"error": err.Error()})
		writeError(writer, http.StatusConflict, "reservation_failed", err)
		return
	}
	s.appendAudit(request.Context(), "port.reserve", "success", saved.ProjectID, saved.Port, map[string]any{"owner": saved.Owner})
	writeJSON(writer, http.StatusOK, saved)
}

func (s *Server) handleReservationDelete(writer http.ResponseWriter, request *http.Request) {
	if !s.requireCapability(writer, s.capabilities.PortMonitoring, "port_monitoring_unavailable", "Mac App Store 版不提供端口预留") {
		return
	}
	portNumber, err := strconv.Atoi(request.PathValue("port"))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_port", errors.New("端口必须是数字"))
		return
	}
	projectID := request.URL.Query().Get("project")
	if projectID == "" {
		writeError(writer, http.StatusBadRequest, "project_required", errors.New("释放预留时必须提供 project"))
		return
	}
	if err := s.ports.Release(request.Context(), portNumber, projectID); err != nil {
		s.appendAudit(request.Context(), "port.release", "failed", projectID, portNumber, map[string]any{"error": err.Error()})
		writeError(writer, http.StatusBadRequest, "reservation_release_failed", err)
		return
	}
	s.appendAudit(request.Context(), "port.release", "success", projectID, portNumber, nil)
	writeJSON(writer, http.StatusOK, map[string]bool{"released": true})
}

func (s *Server) handleProbe(writer http.ResponseWriter, request *http.Request) {
	var input apiprobe.Request
	if err := decodeJSON(request, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_probe", err)
		return
	}
	response, err := s.probe.Do(request.Context(), input)
	if err != nil {
		s.appendAudit(request.Context(), "api.probe", "failed", "", 0, map[string]any{"url": safeURL(input.URL), "error": err.Error()})
		writeError(writer, http.StatusBadGateway, "probe_failed", err)
		return
	}
	s.appendAudit(request.Context(), "api.probe", "success", "", 0, map[string]any{
		"url": safeURL(input.URL), "method": strings.ToUpper(input.Method), "status": response.Status, "durationMs": response.DurationMS,
	})
	writeJSON(writer, http.StatusOK, response)
}

func (s *Server) handleAudit(writer http.ResponseWriter, request *http.Request) {
	registry, err := s.store.Load(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "audit_load_failed", err)
		return
	}
	writeJSON(writer, http.StatusOK, newestAudit(registry.Audit, 200))
}

func (s *Server) requireMutation(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-ProjectDock-Token") != s.token {
			writeError(writer, http.StatusForbidden, "mutation_token_required", errors.New("本地会话令牌缺失或已失效"))
			return
		}
		if request.Method == http.MethodPost || request.Method == http.MethodPut || request.Method == http.MethodPatch {
			contentType := request.Header.Get("Content-Type")
			if !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
				writeError(writer, http.StatusUnsupportedMediaType, "json_required", errors.New("写操作必须使用 application/json"))
				return
			}
		}
		next(writer, request)
	}
}

func (s *Server) requireCapability(writer http.ResponseWriter, enabled bool, code, message string) bool {
	if enabled {
		return true
	}
	writeError(writer, http.StatusNotImplemented, code, errors.New(message))
	return false
}

func (s *Server) pathAuthorized(path string) bool {
	return s.bookmarks != nil && s.bookmarks.Allows(path)
}

func (s *Server) hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		host := request.Host
		if parsedHost, _, err := net.SplitHostPort(host); err == nil {
			host = parsedHost
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			writeError(writer, http.StatusForbidden, "invalid_host", errors.New("ProjectDock 只接受本机回环 Host"))
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(writer, request)
	})
}

func decodeJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(request.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("解析 JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("请求正文只能包含一个 JSON 对象")
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, code string, err error) {
	writeJSON(writer, status, map[string]any{
		"error": map[string]string{"code": code, "message": err.Error()},
	})
}

func newestAudit(all []model.AuditEvent, limit int) []model.AuditEvent {
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	result := append(make([]model.AuditEvent, 0, len(all)), all...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func sessionToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("生成本地会话令牌: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func (s *Server) appendAudit(ctx context.Context, action, status, projectID string, port int, detail map[string]any) {
	value := make([]byte, 8)
	_, _ = rand.Read(value)
	_ = s.store.AppendAudit(ctx, model.AuditEvent{
		ID: hex.EncodeToString(value), Timestamp: time.Now(), Action: action, Status: status,
		ProjectID: projectID, Port: port, Detail: detail,
	})
}

func (s *Server) importPaths(ctx context.Context, paths []string, source string) importReport {
	report := importReport{Items: []importItem{}}
	if len(paths) == 0 {
		report.Skipped = 1
		report.Items = append(report.Items, importItem{Status: "skipped", Message: "没有收到文件夹路径"})
		return report
	}
	if len(paths) > 50 {
		paths = paths[:50]
	}
	if strings.TrimSpace(source) == "" {
		source = "manual"
	}
	for _, path := range paths {
		item := importItem{Path: path}
		result, err := s.projects.SyncPath(ctx, projects.SyncInput{
			Path: path, Source: source, DiscoveredBy: "folder-import", Revive: true,
		})
		if err != nil {
			item.Status = "skipped"
			item.Message = err.Error()
			report.Skipped++
		} else {
			item.Status = result.Action
			item.Result = &result
			report.Imported++
		}
		report.Items = append(report.Items, item)
	}
	return report
}

func (s *Server) registrySyncLoop(ctx context.Context) {
	path := config.ProjectRegistryPath()
	if path == "" {
		return
	}
	s.syncRegistryOnce(ctx, path)
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncRegistryOnce(ctx, path)
		}
	}
}

func (s *Server) syncRegistryOnce(ctx context.Context, path string) {
	report, err := s.projects.SyncRegistry(ctx, path)
	if err != nil {
		s.logger.Printf("自动同步项目注册表失败: %v", err)
		return
	}
	s.logger.Printf("自动同步项目注册表: 新增 %d，更新 %d，忽略 %d，跳过 %d", report.Added, report.Updated, report.Ignored, report.Skipped)
}

func safeURL(raw string) string {
	parts := strings.SplitN(raw, "?", 2)
	return parts[0]
}
