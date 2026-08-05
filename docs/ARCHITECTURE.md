# ProjectDock 架构与安全说明

## 0.10.1 本地化架构

- `web/js/i18n.js` 以简体中文界面键维护 217 组词条，11 个显式语言目录必须等长完整；`scripts/check-i18n.mjs` 在发布前检查语言集合、覆盖率和代表性译文。
- `auto` 和显式语言代码保存在 `localStorage`。界面启动时先解析系统语言与用户选择，再设置文档 `lang`、`dir` 和 `Intl` locale；静态 DOM 和后续动态渲染均走同一翻译函数。
- WebView 通过原生消息桥把选定语言发送给 Swift 宿主。宿主把选择写入 App Group 并请求 WidgetKit 刷新，因此 APP 菜单、系统反馈和组件与主界面保持一致。
- 阿拉伯语启用 RTL 页面方向；代码、路径、端口、URL、日志和表单技术值显式保持 LTR。该方向切换只影响呈现，不改变注册表和 API 数据。

## 0.9.3 版本握手与供应商防错

- 健康响应携带 `version`。0.9.3 CLI 在提交启停前校验服务版本，原生宿主也只连接与自身 `CFBundleShortVersionString` 一致的服务。
- 原生宿主以 `--parent-pid` 启动内嵌服务；服务核对真实父进程并每 500ms 检查宿主是否仍存在，宿主退出后取消 HTTP 服务上下文。
- AI 设置页面只保存用户选择的公开基础地址与模型名；供应商预设不读取其他应用的密钥。服务端再次执行兼容性校验，防止绕过前端把千问模型发送到 OpenAI 官方接口。
- 公证脚本只通过 `PROJECTDOCK_NOTARY_PROFILE` 引用钥匙串配置，不把 Apple ID、专用密码或 API 私钥写入仓库、命令模板或制品。

## 0.9.1 AI 可用性状态

- `internal/ai` 将非敏感验证状态与基础地址/模型一并保存在 `0600` 设置文件中；API 密钥仍只存在 macOS 钥匙串。
- `POST /api/settings/ai/verify` 使用当前模型执行最小 OpenAI 兼容对话请求。成功后才返回 `usable=true`；配置变化、鉴权失败或实际分析失败会清除可用状态。
- 上游错误只映射为本地友好类别，不保存或回显响应正文，避免供应商错误内容意外携带敏感信息。
- `POST /api/github/install` 在 URL 解析和 `git clone` 之前检查 `usable`，前端安装按钮同时禁用；服务端门禁是最终权威。

## 0.9.0 新增边界

- `internal/ai`：保存非敏感模型设置、通过 macOS 钥匙串读取密钥，并调用 OpenAI 兼容的 `/chat/completions`。仓库元数据和响应均有严格大小上限。
- `internal/installer`：解析并规范化 GitHub 仓库地址，以参数数组调用 `git clone --depth 1 -- ...`，不经过 shell。
- `internal/projects/scan.go`：有界本地目录扫描和可管理性判断，复用既有确定性启动方式检测。
- `projects.DeleteProject`：将登记删除和文件删除统一在生命周期锁内；彻底卸载先完成路径、运行态和项目名门禁，再删除文件与登记。

## 组件

```text
Codex / TRAE / Claude
        │ project sync / port check / allocate / unassign
        ▼
原子 JSON 注册表 ───────────────┐
        ▲        ▲              │
        │        │              ▼
三端项目注册表  文件夹选择/拖放  lsof 端口扫描
        │        │
        └────────┤
                 ▼
ProjectDock 本地服务 ── lsof 端口扫描
        │
        ├── 项目注册与受控进程组
        ├── 本地 API 调试器
        ├── 操作审计
        ├── 127.0.0.1 浏览器界面
        └── 脱敏组件快照 API
                    │
                    ▼
         原生 APP 数据桥 ── App Group 原子快照
                                    │
                                    ▼
                    WidgetKit 小号 / 中号 / 大号组件
```

## 项目同步与删除

- 启动同步器只读取一个明确的 JSON 项目注册表，不递归扫描用户目录。
- 当前目录同步按规范化绝对路径幂等合并；项目 ID 由目录名和路径摘要稳定生成。
- 自动同步项目允许启动命令为空。启动服务会再次校验，空命令必须拒绝。
- 注册表保存 `syncMode`、`discoveredBy`、`lastSeenAt` 与可选项目卡路径。
- 删除项目会把规范化路径写入 `ignoredPaths`。后续自动同步返回“已忽略”，不会恢复项目。
- 用户显式选择或拖放同一路径时，视为手动恢复：解除忽略并重新登记，但仍不自动运行命令。

## macOS 文件夹入口

- 浏览器按钮通过受会话令牌保护的回环接口调用 `/usr/bin/osascript` 原生文件夹选择器。
- macOS 原生壳使用 `WKWebView` 承载同一界面，并作为 `NSDraggingDestination` 接收 Finder 文件 URL。
- 原生壳只把已经确认是目录的绝对路径传给页面；页面再调用同一个项目导入 API。
- 原生壳内嵌 `projectctl`，仅在控制台端口没有可用 ProjectDock 服务时启动自己的本地服务；退出时只终止自己启动的服务。
- 关闭主窗口不等于退出 APP；宿主继续维护本地服务和组件快照，用户通过菜单或 `Command-Q` 明确退出。

## 桌面组件数据边界

- `/api/widget-snapshot` 是只读回环接口，输出由服务端白名单模型重新构造，不复用完整 `/api/snapshot`。
- schema 2 快照只包含汇总计数、最多 6 个项目显示名/抽象状态和最多 8 个脱敏端口分配摘要；不序列化项目 ID、路径、命令、健康地址、PID、日志或接口调试数据。
- 宿主 APP 把脱敏 JSON 原子写入 `L4G2HAQ5B5.com.zhusi.projectdock` App Group；Widget 扩展不读取 `registry.json`。
- 宿主写入成功后请求 `WidgetCenter` 重载；扩展时间线仍按系统调度刷新，界面根据 `updatedAt` 标记数据是否过期。
- 组件链接只打开 `projectdock://open/projects` 或 `projectdock://refresh`，由宿主处理；组件自身没有项目启停和命令执行能力。

## 数据与并发

- 默认数据目录：`~/Library/Application Support/ProjectDock`。
- `PROJECTDOCK_HOME` 可把数据隔离到其他绝对路径。
- `registry.lock` 使用操作系统文件锁。
- `registry.json` 通过临时文件、`fsync` 和原子重命名保存，权限为 `0600`。
- `Project.Ports` 是无过期时间的持久端口分配；临时预留带过期时间，读取和写入时都会清理过期预留。

## 进程安全

- 启动命令只来自用户显式保存的项目记录。
- Web API 没有“执行任意命令”入口。
- 每次受管运行保存内存运行令牌、PID 和独立进程组。
- 停止时先发送 `SIGTERM`；5 秒未退出才对令牌与 PID 仍匹配的进程组发送 `SIGKILL`。
- ProjectDock 重启后不接管旧进程，也不会因端口相同而停止未知进程。

## Web 与接口安全

- 服务只允许绑定 `127.0.0.1`、`localhost` 或 `::1`。
- Host 头必须是回环主机，降低 DNS rebinding 风险。
- 每次启动生成随机会话令牌；所有写操作必须携带该令牌和 `application/json`。
- 页面设置 CSP、`X-Frame-Options: DENY`、同源资源和 no-referrer 策略。
- API 调试器仅允许回环 URL，禁止代理、跨回环重定向和账号信息 URL。
- Authorization、Cookie、代理认证、Host、Connection 等请求头不能由调试台设置。
- `Set-Cookie`、认证挑战和 token 类响应头会在界面中隐藏。

## 端口真值边界

仪表盘和 Widget 属于只读观察链路：一次快照只执行一次 `lsof`，并可在 2 秒内共享结果。`port check`、`allocate`、项目启动前复查及停止后的端口确认不使用观察缓存，始终重新扫描真实监听。

端口“持久分配”和“临时预留”都是跨工具协作锁，不是内核 socket 绑定。长时项目端口的正确流程始终是：

1. 扫描真实 TCP LISTEN。
2. 查询其他项目持久分配与有效临时预留。
3. 原子写入项目持久分配。
4. 项目启动前再次扫描真实监听并核对当前项目归属。
5. 启动后以真实监听与进程状态作为运行证据。
6. 项目停止后保留分配；只有显式取消分配或删除项目才释放归属。
