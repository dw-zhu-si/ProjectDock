# 参与 ProjectDock

感谢参与。提交前请先说明问题或目标，并保持改动聚焦、可回滚且不包含本机路径、凭证、私有项目内容或真实 API 密钥。

## 本地验证

```bash
go test ./...
go test -race ./...
go vet ./...
node scripts/check-i18n.mjs
node --check web/js/api.js
node --check web/js/app.js
node --check web/js/format.js
node --check web/js/i18n.js
node --check web/js/ui.js
xcodebuild -project ProjectDock.xcodeproj -scheme ProjectDock -configuration Release CODE_SIGNING_ALLOWED=NO build
```

新增界面文案时必须同步全部语言目录。涉及项目命令、删除、GitHub 安装、AI 请求或路径访问时，需要同时补充安全边界与失败测试。

提交贡献即表示你同意按 Apache License 2.0 提供该贡献。

