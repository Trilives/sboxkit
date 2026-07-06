# 模块化约束

本文约束 sboxkit 后续改动的文件规模、包边界与拆分规则，目标是避免单个文件持续膨胀，
让流程编排、系统操作、订阅转换、下载逻辑各自保持清晰。

约束对象是**整个仓库**，不只是 Go 源码：Web 面板前端（`internal/uiassets/assets/*`）、
文档（`docs/*.md`、根级 `*.md`）、脚本（`scripts/*`、`packaging/*`）、systemd 模板
（`internal/sysd/*.tmpl`）、测试夹具（`testdata/**`）同样受规模红线与拆分规则约束。
Go 特有的包边界与依赖方向见后半部分，非 Go 资产的约束见「非 Go 资产约束」一节。

## 规模红线

### Go 源码

| 对象 | 目标 | 软上限 | 硬上限 |
|---|---:|---:|---:|
| 普通 `.go` 文件 | 200-400 行 | 500 行 | 800 行 |
| 流程编排文件（`internal/flows/*.go`） | 150-300 行 | 400 行 | 600 行 |
| 单个函数 | 20-50 行 | 80 行 | 120 行 |
| 单个测试文件 | 150-300 行 | 500 行 | 800 行 |

### 非 Go 资产

| 对象 | 目标 | 软上限 | 硬上限 |
|---|---:|---:|---:|
| Web 前端脚本（`uiassets/assets/*.js`） | 200-350 行 | 500 行 | 700 行 |
| 样式表（`uiassets/assets/*.css`） | 200-400 行 | 550 行 | 750 行 |
| HTML 模板（`uiassets/assets/*.html`） | 80-150 行 | 250 行 | 400 行 |
| Markdown 文档（`docs/*.md`、根级 `*.md`） | 100-250 行 | 400 行 | 600 行 |
| Shell / 打包脚本（`scripts/*`、`packaging/*`） | 50-150 行 | 250 行 | 400 行 |
| systemd unit / 模板（`internal/sysd/*.tmpl`） | 一单元一文件，单一职责 | — | — |
| 测试夹具（`testdata/**`） | 小而聚焦，只覆盖被测分支 | — | — |

单个 JS 函数、CSS 规则块同样参照 Go 的「单个函数」红线精神：一个函数只做一件事，
超过 80 行应拆分；一段样式若同时服务两个不相关组件，应拆成两个命名清晰的规则块。

规则：

- 新增代码不得让文件越过硬上限。
- 修改已超过软上限的文件时，优先把新增逻辑放到同包新文件中。
- 如果一次改动让文件增加超过 120 行，必须说明为什么不拆分。
- 流程函数只负责串联步骤；具体判断、IO、转换、系统命令应下沉到同包 helper 或领域包。

## 拆分触发条件

遇到以下任一情况，应拆成新文件或新包：

- 文件里出现两个以上独立职责，例如“交互提示 + HTTP 下载 + systemd unit 渲染”。
- 一个流程函数连续出现三段以上可命名步骤。
- 同类 helper 超过 5 个，且只服务某个子主题。
- 测试需要大量构造夹具才能覆盖某段逻辑，说明该逻辑应抽成可测函数。
- 文件名已经无法准确概括新增代码。

拆分时优先使用同包多文件，而不是过早新建包。只有当新职责能被多个包复用，或依赖方向更清楚时，
才新建 `internal/<domain>` 包。

## 包边界

现有边界保持如下：

- `cmd/sboxkit`：只做 CLI 分发、版本输出、顶层子命令入口。
- `internal/flows`：交互流程编排，允许调用领域包，但不直接承载复杂转换或下载实现。
- `internal/subscription`：订阅拉取、来源识别、调用 converter 生成配置、active 切换。
- `internal/converter`：Clash / sing-box 原生 / base64 → sing-box 配置的协议转换。
- `internal/kernel`：sing-box 内核/geo 规则集下载、系统包种子接管、资源更新策略。
- `internal/sysd`：systemd unit、运行时暂存、服务/面板/自愈/定时器管理。
- `internal/config`：`customize.json` 默认值、字段元数据、读写兼容性。
- `internal/txn`：事务和回滚原语，不知道业务语义。
- `internal/tui`：通用交互组件，不依赖业务包。

依赖方向：

- `flows` 可以依赖领域包。
- 领域包不得依赖 `flows`。
- `tui`、`txn`、`paths`、`errs` 应保持底层工具属性，避免反向依赖业务。
- `sysd` 可以依赖 `paths/config/execx/jsonx`，不得依赖 `flows`。

## 非 Go 资产约束

Go 之外的资产同样有「边界」与「拆分触发条件」，只是维度不是包依赖，而是职责与文件角色。

### Web 面板前端（`internal/uiassets/assets`）

- **一文件一层**：`index.html` 只放结构与挂载点；`styles.css` 只放样式；`app.js` 放行为。
  不要把内联 `<style>` / `<script>` 塞回 HTML，也不要在 JS 里拼大段 HTML 字符串
  （现有代码用 `document.createElement` 构建 DOM，新增渲染继续沿用）。
- **`app.js` 内部按注释分区保持可读**：现有分区为 `helpers / API 层 / login / view 切换 /
  dashboard 数据 / 渲染 / actions / wiring`。当某一分区超过约 150 行、或出现第二块独立职责
  （例如「渲染」里又长出一套独立的图表逻辑）时，拆成 `app.<concern>.js` 并在 HTML 里按序引入，
  而不是让 `app.js` 越过软上限。
- **`styles.css` 按组件分块**：`:root` 变量、基础控件、appbar、group-card、node-card、toast
  各自成段。新增组件样式追加新段落，不要塞进已有组件的规则里。超过软上限时按组件拆成多份
  并在 HTML 里显式引入。
- 前端不得引入外部 CDN / 远程字体 / 远程脚本——面板是 `go:embed` 进二进制的自包含资源，
  所有资产必须内联或随包分发（与 Artifact 的 CSP 约束同理）。

### 文档（`docs/*.md`、根级 `*.md`）

- 一篇文档一个主题：架构决策进 `ARCHITECTURE.md`，模块化规则进本文，协议转换规则进
  `internal/converter/converter.md`。不要让单篇文档同时承载「架构 + 操作手册 + 变更日志」。
- 超过软上限时按小标题拆成多篇并相互链接，而不是无限追加章节。

### 脚本与打包（`scripts/*`、`packaging/*`、`internal/sysd/*.tmpl`）

- 一个脚本只做一件可命名的事（构建 / 发布 / 种子打包）；出现第二职责时拆成新脚本。
- systemd 模板保持一单元一文件，逻辑（安装 / 卸载 / 探针）留在 Go 侧，模板只做占位替换。

### 测试夹具（`testdata/**`）

- 夹具只包含覆盖目标分支所需的最小内容；需要大量夹具才能覆盖一段逻辑，说明该逻辑
  应抽成可直接单测的函数（与「拆分触发条件」最后一条一致）。
- 每个夹具的用途在对应测试里可追溯，不留孤儿夹具。

## 文件命名

按职责命名文件，避免 `utils.go`、`helpers.go` 成为垃圾桶。

推荐模式：

- `initflow.go`：初始化主流程和少量只读步骤 glue。
- `init_resources.go`：初始化资源判定、种子使用、下载兜底。
- `service.go`：主 systemd 服务。
- `resilience.go`：NetworkManager/watchdog 安装卸载。
- `healthcheck.go`：watchdog 探针实现。
- `selfupdate.go`：sboxkit 自更新（版本化目录 + 原子符号链接切换）。

测试文件跟随被测职责命名，例如 `init_resources_test.go`，不要把所有流程测试堆进
`initflow_test.go`。

## 修改流程

每次新增功能或改业务逻辑时执行：

1. 先看目标文件行数：`wc -l <目标文件>`（Go 用 `internal/<pkg>/*.go`，前端用
   `internal/uiassets/assets/*`）。
2. 如果目标文件超过软上限，先选择同职责的新文件（Go 同包新文件 / 前端 `app.<concern>.js`）
   承载新增职责，而不是继续堆进旧文件。
3. 为新 helper 写小范围单元测试；流程本身只保留少量集成式测试。前端行为改动用
   `tmux` 逐屏或浏览器手测记录一次。
4. Go 侧跑 `gofmt`、`go test ./...`、`go vet ./...`；前端改动确认面板可加载、无 CSP 违规。
5. 在评审 diff 时检查是否引入了新的“大文件吸附点”（含前端 `app.js` / `styles.css`）。

## 例外

以下情况可以临时超过软上限，但不能超过硬上限：

- 协议/格式兼容逻辑必须集中保持可读，例如同一转换表或字段元数据。
- release 前临时稳定化，后续必须单独开拆分任务。
- generated 或第三方嵌入内容，但生成源和用途必须明确。

例外应在 PR/提交说明中写清楚，并标注后续拆分边界。
