# ProjectDock

ProjectDock 是个人独立开发的原生 macOS 本地项目控制台，与任何任职单位无关。APP 内置自己的 Go 控制服务、Web 界面和本地注册表，不依赖 Docker、Obsidian、TRAE、Codex、Claude 或其他后台系统才能启动和使用。

Codex、TRAE、Claude 可以通过随 APP 一起封装的 `projectctl` 主动登记项目；这些接入都是可选能力，不是 ProjectDock 的运行依赖。

## 当前能力

- ProjectDock 自有本机注册表；外部开发工具可通过 `projectctl project sync` 幂等登记当前目录。
- 自动识别 `.projectdock.json`、启动脚本、Makefile、Docker Compose 和 `package.json` 生命周期；只登记，不自动执行。
- 项目登记、文件夹选择、Finder 拖放、编辑、删除与按来源工具筛选。
- 粘贴标准 GitHub 仓库网址并选择安装父目录；AI 配置必须先通过真实连接验证，之后才允许浅克隆、分析技术栈和人工准备步骤并登记本地项目。
- AI 设置提供 OpenAI、阿里云百炼标准 API、百炼 Coding Plan、本机模型与自定义兼容接口预设；已知的供应商/模型错配会在发出请求前阻断。
- 扫描本地父目录并批量选择可管理项目；扫描有深度和数量上限，会忽略依赖、构建产物、隐藏目录与符号链接。
- 删除项目时可选择仅移除登记，或经项目名二次确认后彻底卸载精确项目目录；浅层路径、运行中项目和 ProjectDock 数据目录受保护。
- 关闭主窗口后继续后台运行并隐藏 Dock 图标；可选开机自动启动。
- 支持跟随系统以及简体中文、繁体中文、英语、日语、韩语、德语、法语、西班牙语、巴西葡萄牙语、俄语和阿拉伯语；语言选择会持久保存，并同步到原生菜单和桌面组件，阿拉伯语使用 RTL 布局。
- 项目页只显示路径可用且已接入明确启动方式的项目；登记待接入、归档和路径不可用记录不进入启停列表。
- 区分 ProjectDock 管理运行、外部运行和端口归属冲突。
- 受控进程启动、停止、PID、退出状态和运行日志；外部运行只执行已登记停止命令，不直接终止未知 PID。
- CLI 启停统一转交给正在运行的 ProjectDock 回环服务，桌面 APP 与 Codex/TRAE 命令共用同一个运行令牌和停止控制器。
- 可见的运行、项目和端口页每 5 秒观察本机 TCP LISTEN；API 页与审计页降低频率，窗口隐藏时暂停轮询并在恢复可见时立即刷新。
- 仪表盘和桌面组件在短时间内共享一次只读端口观察；端口检查、分配和启停仍强制执行新鲜扫描。
- 持久端口资源池：项目未启动时分配仍占用，显式取消后才释放；短时预留继续支持 TTL。
- GET、POST、PUT、PATCH、DELETE、HEAD 本地接口调试。
- 本地会话令牌、回环 Host、CSP、接口 SSRF 和敏感头门禁。
- 最近 500 条结构化操作审计。
- 原生 macOS 小号、中号、大号桌面组件，显示项目、运行、监听端口、持久分配和登记概况。
- 正式 macOS 应用图标，覆盖 APP、Dock 和组件库入口。
- 桌面组件只读取 App Group 中的脱敏快照，不接触项目路径、启动命令、PID、日志或接口详情。

## 快速开始

macOS Apple Silicon：

```bash
BIN="./outputs/ProjectDock-0.10.0/projectctl-darwin-arm64"

"$BIN" doctor
"$BIN" project add --id projectdock-console --name ProjectDock --path "$PWD" --source manual
"$BIN" port check 43110
"$BIN" port allocate 43110 --project projectdock-console --owner manual
"$BIN" serve --listen 127.0.0.1:43110
```

浏览器会自动打开 `http://127.0.0.1:43110`。停止服务不会释放持久分配；只有确认项目今后不再使用端口时执行：

```bash
"$BIN" port unassign 43110 --project projectdock-console
```

也可以解压并运行原生 APP：

```text
outputs/ProjectDock-0.10.0/ProjectDock-0.10.0-macos-arm64.zip
```

首次运行 APP 后，在 macOS 桌面组件库中搜索 `ProjectDock`，可添加小号、中号或大号组件。大号组件使用一个总览卡片和项目、端口两个信息面板，避免真实桌面宽度下数字和中文标签竖排。宿主 APP 每分钟更新一次脱敏快照并请求系统刷新；最终刷新时机仍由 WidgetKit 调度。

运行数据默认保存在 `~/Library/Application Support/ProjectDock`。测试或需要隔离数据时可设置：

```bash
PROJECTDOCK_HOME="/绝对路径/projectdock-data" "$BIN" serve
```

## 可选的开发工具接入

进入项目后先同步当前目录：

```bash
projectctl project sync --path "$PWD" --source codex
```

同步会发现明确的本地启动资料。需要人工指定时：

```bash
projectctl project add --id my-project --name 我的项目 --path "$PWD" \
  --workdir web --start "npm run dev" --stop "npm run stop"
```

项目启停前需要先打开 ProjectDock APP，或保持 `projectctl serve` 运行。随后 CLI 会把启停动作提交给这个长驻控制器：

```bash
projectctl project start my-project
projectctl project stop my-project
```

一次性 CLI 不再直接拉起无法在下一条命令中安全接管的长时进程。

再在启动监听前统一执行：

```bash
projectctl port check 5173 --project my-project
projectctl port allocate 5173 --project my-project --owner codex
projectctl port check 5173 --project my-project
```

项目停止时保留分配。只有项目不再使用该端口时执行：

```bash
projectctl port unassign 5173 --project my-project
```

## 状态边界

- macOS arm64 二进制已完成真实 `lsof`、项目启停、端口冲突、API 调用和浏览器验收。
- macOS 原生 `.app` 使用 WKWebView 接收 Finder 文件夹拖放；原生接收链和目录导入接口已验证，完整 Finder 人工拖放仍列为发布前人工验收项。
- macOS 小号、中号、大号桌面组件已在系统 WidgetKit 模拟器中使用真实 App Group 快照完成视觉验收；扩展已注册到本机组件库。
- APP 自身不调用 Docker；只有被管理项目明确登记 Docker Compose 启动命令时，ProjectDock 才会按用户操作执行那个项目自己的命令。
- ProjectDock 不默认探测 Obsidian 或三端注册表；只有显式设置 `PROJECTDOCK_PROJECTS_FILE` 时才启用可选注册表导入。
- 当前 0.10.0 APP、Widget 扩展和内置 CLI 使用 Developer ID 与 Apple 安全时间戳签名；Apple 公证已接受，票据已装订，`stapler validate` 与 Gatekeeper `spctl` 验证通过。
- 交付 ZIP 会移除 `__MACOSX/._*` AppleDouble，并在解包后执行深度严格验签。

架构与安全边界见 `docs/ARCHITECTURE.md`，参与开发前请阅读 `CONTRIBUTING.md` 与 `SECURITY.md`。
