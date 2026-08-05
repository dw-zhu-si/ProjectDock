package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"projectdock/internal/apiprobe"
	"projectdock/internal/config"
	"projectdock/internal/model"
	"projectdock/internal/ports"
	"projectdock/internal/projects"
	"projectdock/internal/server"
	"projectdock/internal/store"
)

const version = "0.10.1"

var projectDockAPIBaseURL = "http://127.0.0.1:43110"

type cliError struct {
	code int
	err  error
}

func (e cliError) Error() string {
	return e.err.Error()
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		var exit cliError
		if errors.As(err, &exit) {
			fmt.Fprintln(os.Stderr, exit.err)
			os.Exit(exit.code)
		}
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	st := store.New(dataDir)
	scanner := ports.NewLsofScanner()
	portService := ports.NewService(st, scanner)
	projectService := projects.NewService(st, portService)

	switch args[0] {
	case "serve":
		return runServe(args[1:], st, portService, projectService)
	case "port":
		return runPort(args[1:], st, portService)
	case "project":
		return runProject(args[1:], projectService)
	case "doctor":
		return runDoctor(dataDir, scanner)
	case "_selftest-server":
		return runSelftestServer(args[1:])
	case "version", "--version", "-v":
		fmt.Println("projectctl", version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		return usageError()
	}
}

func runSelftestServer(args []string) error {
	flags := flag.NewFlagSet("_selftest-server", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:43219", "测试监听地址")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := model.ValidateLoopbackListen(*listen); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"ok","managedBy":"projectdock"}`))
	})
	server := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func runServe(args []string, st *store.Store, portService *ports.Service, projectService *projects.Service) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:43110", "本地监听地址")
	openBrowser := flags.Bool("open", true, "启动后打开浏览器")
	parentPID := flags.Int("parent-pid", 0, "可选宿主进程 PID；宿主退出时同步停止服务")
	appStore := flags.Bool("app-store", false, "启用 Mac App Store 沙盒能力边界")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := model.ValidateLoopbackListen(*listen); err != nil {
		return err
	}
	if err := st.Ensure(); err != nil {
		return err
	}
	if *parentPID < 0 {
		return errors.New("parent-pid 不能为负数")
	}
	if *parentPID > 0 && os.Getppid() != *parentPID {
		return errors.New("parent-pid 与实际宿主进程不匹配")
	}
	if *appStore {
		portService = ports.NewService(st, ports.DisabledScanner{})
		projectService = projects.NewService(st, portService)
	}
	app, err := server.NewWithOptions(st, portService, projectService, apiprobe.NewService(), log.Default(), server.Options{AppStore: *appStore}, version)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *parentPID > 0 {
		go monitorParent(ctx, *parentPID, stop)
	}
	url := "http://" + *listen
	fmt.Printf("ProjectDock %s 已启动：%s\n", version, url)
	fmt.Printf("数据目录：%s\n", st.Dir())
	if *openBrowser {
		go func() {
			time.Sleep(250 * time.Millisecond)
			if err := openURL(url); err != nil {
				log.Printf("打开浏览器失败: %v", err)
			}
		}()
	}
	return app.Serve(ctx, *listen)
}

func monitorParent(ctx context.Context, parentPID int, stop context.CancelFunc) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if syscall.Kill(parentPID, 0) != nil {
				stop()
				return
			}
		}
	}
}

func runPort(args []string, st *store.Store, service *ports.Service) error {
	if len(args) < 1 {
		return errors.New("用法: projectctl port <check|allocate|unassign|pool|reserve|release> [端口] [参数]")
	}
	action := args[0]
	if action == "pool" {
		flags := flag.NewFlagSet("port pool", flag.ContinueOnError)
		from := flags.Int("from", 3000, "搜索起始端口")
		to := flags.Int("to", 49999, "搜索结束端口")
		limit := flags.Int("limit", 20, "空闲建议数量")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		result, err := service.Pool(context.Background(), *from, *to, *limit)
		if err != nil {
			return err
		}
		printJSON(result)
		return nil
	}
	if len(args) < 2 {
		return errors.New("该端口动作必须提供端口号")
	}
	portNumber, err := strconv.Atoi(args[1])
	if err != nil {
		return errors.New("端口必须是数字")
	}
	switch action {
	case "check":
		flags := flag.NewFlagSet("port check", flag.ContinueOnError)
		projectID := flags.String("project", "", "允许复用自身预留的项目 ID")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		result, err := service.Check(context.Background(), portNumber, *projectID)
		if err != nil {
			return err
		}
		printJSON(result)
		if !result.Available {
			return cliError{code: 3, err: fmt.Errorf("端口 %d 不可用: %s", portNumber, result.Reason)}
		}
		return nil
	case "allocate":
		flags := flag.NewFlagSet("port allocate", flag.ContinueOnError)
		projectID := flags.String("project", "", "项目 ID")
		owner := flags.String("owner", "", "codex、trae、claude、manual 或 projectdock")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if *projectID == "" || *owner == "" {
			return errors.New("allocate 必须提供 --project 和 --owner")
		}
		allocation, err := service.Allocate(context.Background(), portNumber, *projectID, *owner)
		if err != nil {
			return err
		}
		appendAudit(st, "port.allocate", allocation.ProjectID, allocation.Port, map[string]any{"owner": strings.ToLower(*owner), "source": "cli"})
		printJSON(allocation)
		return nil
	case "unassign":
		flags := flag.NewFlagSet("port unassign", flag.ContinueOnError)
		projectID := flags.String("project", "", "项目 ID")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if *projectID == "" {
			return errors.New("unassign 必须提供 --project")
		}
		if err := service.Unassign(context.Background(), portNumber, *projectID); err != nil {
			return err
		}
		appendAudit(st, "port.unassign", *projectID, portNumber, map[string]any{"source": "cli"})
		printJSON(map[string]any{"unassigned": true, "port": portNumber, "projectId": *projectID})
		return nil
	case "reserve":
		flags := flag.NewFlagSet("port reserve", flag.ContinueOnError)
		projectID := flags.String("project", "", "项目 ID")
		owner := flags.String("owner", "", "codex、trae、claude 或 manual")
		ttl := flags.Duration("ttl", 4*time.Hour, "预留有效期")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if *projectID == "" || *owner == "" {
			return errors.New("reserve 必须提供 --project 和 --owner")
		}
		reservation, err := service.Reserve(context.Background(), model.PortReservation{
			Port: portNumber, ProjectID: *projectID, Owner: strings.ToLower(*owner),
			ExpiresAt: time.Now().Add(*ttl),
		})
		if err != nil {
			return err
		}
		appendAudit(st, "port.reserve", reservation.ProjectID, reservation.Port, map[string]any{"owner": reservation.Owner, "source": "cli"})
		printJSON(reservation)
		return nil
	case "release":
		flags := flag.NewFlagSet("port release", flag.ContinueOnError)
		projectID := flags.String("project", "", "项目 ID")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		if *projectID == "" {
			return errors.New("release 必须提供 --project")
		}
		if err := service.Release(context.Background(), portNumber, *projectID); err != nil {
			return err
		}
		appendAudit(st, "port.release", *projectID, portNumber, map[string]any{"source": "cli"})
		printJSON(map[string]any{"released": true, "port": portNumber, "projectId": *projectID})
		return nil
	default:
		return errors.New("端口动作必须是 check、allocate、unassign、pool、reserve 或 release")
	}
}

func runProject(args []string, service *projects.Service) error {
	if len(args) == 0 {
		return errors.New("用法: projectctl project <list|add|sync|sync-registry|remove|start|stop>")
	}
	switch args[0] {
	case "list":
		result, err := service.List(context.Background())
		if err != nil {
			return err
		}
		printJSON(result)
		return nil
	case "add":
		flags := flag.NewFlagSet("project add", flag.ContinueOnError)
		id := flags.String("id", "", "项目 ID")
		name := flags.String("name", "", "项目名称")
		path := flags.String("path", "", "项目绝对路径")
		source := flags.String("source", "manual", "来源工具")
		workdir := flags.String("workdir", "", "项目根目录内的相对工作目录")
		start := flags.String("start", "", "启动命令")
		stop := flags.String("stop", "", "安全停止命令")
		portsValue := flags.String("ports", "", "逗号分隔的计划端口")
		health := flags.String("health", "", "本地健康地址")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		parsedPorts, err := parsePorts(*portsValue)
		if err != nil {
			return err
		}
		project, err := service.Upsert(context.Background(), model.Project{
			ID: *id, Name: *name, Path: *path, Source: strings.ToLower(*source),
			SyncMode: "manual", DiscoveredBy: strings.ToLower(*source),
			WorkingDirectory: *workdir, StartCommand: *start, StopCommand: *stop,
			LaunchSource: "manual", Ports: parsedPorts, HealthURL: *health,
		})
		if err != nil {
			return err
		}
		printJSON(project)
		return nil
	case "sync":
		flags := flag.NewFlagSet("project sync", flag.ContinueOnError)
		path := flags.String("path", "", "项目绝对路径；默认当前目录")
		name := flags.String("name", "", "项目名称；默认目录名")
		source := flags.String("source", "other", "codex、trae、claude、manual、tri-agent 或 other")
		card := flags.String("card", "", "可选项目卡路径")
		revive := flags.Bool("revive", false, "显式解除删除忽略；仅用于用户手动恢复")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*path) == "" {
			current, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("读取当前目录: %w", err)
			}
			*path = current
		}
		result, err := service.SyncPath(context.Background(), projects.SyncInput{
			Path: *path, Name: *name, Source: strings.ToLower(*source),
			DiscoveredBy: strings.ToLower(*source), ProjectCard: *card, Revive: *revive,
		})
		if err != nil {
			if errors.Is(err, projects.ErrProjectIgnored) {
				return cliError{code: 4, err: err}
			}
			return err
		}
		printJSON(result)
		return nil
	case "sync-registry":
		flags := flag.NewFlagSet("project sync-registry", flag.ContinueOnError)
		path := flags.String("file", config.ProjectRegistryPath(), "三端项目注册表 JSON")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*path) == "" {
			return errors.New("未找到三端项目注册表，请使用 --file 或设置 PROJECTDOCK_PROJECTS_FILE")
		}
		report, err := service.SyncRegistry(context.Background(), *path)
		if err != nil {
			return err
		}
		printJSON(report)
		return nil
	case "remove":
		if len(args) != 2 {
			return errors.New("用法: projectctl project remove <项目ID>")
		}
		if err := service.Delete(context.Background(), args[1]); err != nil {
			return err
		}
		printJSON(map[string]any{"deleted": true, "projectId": args[1], "filesDeleted": false})
		return nil
	case "start", "stop":
		if len(args) != 2 {
			return fmt.Errorf("用法: projectctl project %s <项目ID>", args[0])
		}
		status, err := runProjectActionViaAPI(args[0], args[1])
		if err != nil {
			return err
		}
		printJSON(status)
		return nil
	default:
		return errors.New("项目动作必须是 list、add、sync、sync-registry、remove、start 或 stop")
	}
}

func runProjectActionViaAPI(action, projectID string) (model.RunStatus, error) {
	client := &http.Client{Timeout: 18 * time.Second}
	healthRequest, err := http.NewRequest(http.MethodGet, projectDockAPIBaseURL+"/api/health", nil)
	if err != nil {
		return model.RunStatus{}, err
	}
	healthResponse, err := client.Do(healthRequest)
	if err != nil {
		return model.RunStatus{}, errors.New("ProjectDock 桌面服务未运行；请先打开 ProjectDock APP，再执行项目启停")
	}
	var health struct {
		Service string `json:"service"`
		Version string `json:"version"`
	}
	if decodeErr := decodeAPIResponse(healthResponse, &health); decodeErr != nil || health.Service != "projectdock" {
		return model.RunStatus{}, errors.New("43110 端口不是可用的 ProjectDock 桌面服务")
	}
	if health.Version != version {
		return model.RunStatus{}, fmt.Errorf("ProjectDock CLI 与桌面服务版本不匹配（CLI %s，服务 %s）", version, health.Version)
	}

	sessionResponse, err := client.Get(projectDockAPIBaseURL + "/api/session")
	if err != nil {
		return model.RunStatus{}, fmt.Errorf("读取 ProjectDock 本地会话: %w", err)
	}
	var session struct {
		Token string `json:"token"`
	}
	if err := decodeAPIResponse(sessionResponse, &session); err != nil {
		return model.RunStatus{}, err
	}
	if session.Token == "" {
		return model.RunStatus{}, errors.New("ProjectDock 本地会话未返回操作令牌")
	}

	target := projectDockAPIBaseURL + "/api/projects/" + url.PathEscape(projectID) + "/" + action
	request, err := http.NewRequest(http.MethodPost, target, strings.NewReader("{}"))
	if err != nil {
		return model.RunStatus{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-ProjectDock-Token", session.Token)
	response, err := client.Do(request)
	if err != nil {
		return model.RunStatus{}, fmt.Errorf("调用 ProjectDock 项目%s接口: %w", map[string]string{"start": "启动", "stop": "停止"}[action], err)
	}
	var status model.RunStatus
	if err := decodeAPIResponse(response, &status); err != nil {
		return model.RunStatus{}, err
	}
	return status, nil
}

func decodeAPIResponse(response *http.Response, target any) error {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取 ProjectDock 本地响应: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &payload) == nil && payload.Error.Message != "" {
			return errors.New(payload.Error.Message)
		}
		return fmt.Errorf("ProjectDock 本地接口返回 HTTP %d", response.StatusCode)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("解析 ProjectDock 本地响应: %w", err)
	}
	return nil
}

func runDoctor(dataDir string, scanner *ports.LsofScanner) error {
	result := map[string]any{
		"version": version, "goos": runtime.GOOS, "goarch": runtime.GOARCH,
		"dataDir": dataDir, "lsofPath": scanner.Path,
	}
	if _, err := os.Stat(scanner.Path); err != nil {
		result["lsof"] = "missing"
		printJSON(result)
		return err
	}
	listeners, err := scanner.List(context.Background())
	if err != nil {
		result["lsof"] = "failed"
		printJSON(result)
		return err
	}
	result["lsof"] = "ok"
	result["listenerCount"] = len(listeners)
	printJSON(result)
	return nil
}

func openURL(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "linux":
		command = exec.Command("xdg-open", url)
	default:
		return errors.New("当前平台不支持自动打开浏览器")
	}
	return command.Start()
}

func parsePorts(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return []int{}, nil
	}
	parts := strings.Split(raw, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		portNumber, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || model.ValidatePort(portNumber) != nil {
			return nil, fmt.Errorf("无效端口 %q", part)
		}
		result = append(result, portNumber)
	}
	return result, nil
}

func appendAudit(st *store.Store, action, projectID string, port int, detail map[string]any) {
	_ = st.AppendAudit(context.Background(), model.AuditEvent{
		ID: fmt.Sprintf("cli-%d", time.Now().UnixNano()), Timestamp: time.Now(),
		Action: action, Status: "success", ProjectID: projectID, Port: port, Detail: detail,
	})
}

func printJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func usageError() error {
	printUsage()
	return cliError{code: 2, err: errors.New("缺少或无法识别的命令")}
}

func printUsage() {
	fmt.Println(`ProjectDock 本地项目总控台

用法:
  projectctl serve [--listen 127.0.0.1:43110] [--open=true]
  projectctl port check <端口> [--project 项目ID]
  projectctl port allocate <端口> --project 项目ID --owner codex|trae|claude|manual
  projectctl port unassign <端口> --project 项目ID
  projectctl port pool [--from 3000] [--to 49999] [--limit 20]
  projectctl port reserve <端口> --project 项目ID --owner codex|trae|claude|manual [--ttl 4h]
  projectctl port release <端口> --project 项目ID
  projectctl project list
  projectctl project add --id ID --name 名称 --path 绝对路径 [--workdir 相对目录] [--start 启动命令] [--stop 停止命令] [--source codex] [--ports 5173,3000]
  projectctl project sync [--path 绝对路径] --source codex|trae|claude
  projectctl project sync-registry [--file 三端项目注册表.json]
  projectctl project remove <项目ID>
  projectctl project start <项目ID>
  projectctl project stop <项目ID>
  projectctl doctor
  projectctl version`)
}
