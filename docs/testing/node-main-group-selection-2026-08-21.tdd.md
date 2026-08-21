# 节点切换主选择组 TDD 证据

## 来源与用户旅程

来源：`Records/Todo.md` 原最后一项（完成后已归档到 `Records/Done.md`）。

作为使用外部 sing-box 配置的用户，我希望“节点切换”和“固定节点”只操作识别出的主
selector；无法识别时可以直接补充关键词或留空退出，从而避免误切换到成员数较多但用途
不同的 selector。

## RED / GREEN

| 阶段 | 命令 | 结果 |
|---|---|---|
| RED：有效关键词必须先校验再保存 | `go test ./internal/flows -run 'Test(AddMainGroupKeyword\|PickGroup)' -count=1` | 编译失败：`undefined: addMainGroupKeyword` |
| GREEN：严格识别、关键词校验 | 同上 | PASS |
| RED：直接输入与空输入需要可测试交互边界 | `go test ./internal/flows -run 'Test(PromptMainGroupKeyword\|CollectMembers)' -count=1` | 编译失败：`undefined: promptMainGroupKeywordWith` |
| GREEN：直接输入、空输入、主组成员范围 | `go test ./internal/flows -run 'Test(PromptMainGroupKeyword\|CollectMembers\|AddMainGroupKeyword\|PickGroup)' -count=1` | PASS |

实际运行时设置了 `GOCACHE=/tmp/sboxkit-go-cache`，因为沙箱中的默认 Go 缓存只读。

## 测试规格

| # | 保证 | 测试 | 类型 | 结果 |
|---|---|---|---|---|
| 1 | `default_outbound` 精确命中主 selector | `TestPickGroupUsesConfiguredMainSelector` | 单元 | PASS |
| 2 | 主组缺失时不猜测成员最多的 selector | `TestPickGroupRequiresConfiguredMainSelector` | 单元 | PASS |
| 3 | forced 分组及无 selector 配置得到明确校验 | `TestPickGroupValidatesSelectorAndForcedGroup` | 单元 | PASS |
| 4 | 有序关键词可识别外部配置的主 selector | `TestPickGroupUsesMainGroupKeywordsWhenDefaultTagMissing` | 单元 | PASS |
| 5 | 只有能匹配当前 selector 的输入才会加入配置，且不修改输入对象 | `TestAddMainGroupKeywordRequiresAMatchingSelector` | 单元 | PASS |
| 6 | 直接输入被持久化；留空不写盘 | `TestPromptMainGroupKeywordSavesDirectInputAndAllowsBlank` | 集成 | PASS |
| 7 | 菜单候选只来自主 selector 的直接成员/子组 | `TestCollectMembersOnlyUsesMainSelectorChoices` | 单元 | PASS |
| 8 | 新字段默认值、JSON 往返和编辑器元数据有效 | `TestMainGroupKeywordsDefaultRoundTripAndEditorMetadata` | 集成 | PASS |
| 9 | 输入有效关键词后重新解析并返回新主组 | `TestResolveTargetGroupRechecksWithSavedKeyword` | 集成 | PASS |
| 10 | 留空/无效输入返回明确错误，交互错误原样传播 | `TestResolveTargetGroupReturnsMissingErrorWithoutMatchingKeyword`、`TestResolveTargetGroupUsesStoredGroupAndPropagatesPromptErrors` | 集成 | PASS |

## 验证

- `gofmt`：PASS。
- `go test ./... -count=1`：PASS。沙箱内首次运行因既有 `httptest` 不能监听临时端口而失败；授权后在正常权限下重跑通过。
- `go vet ./...`：PASS。
- `git diff --check`：PASS。
- TTY：在 PTY 中以临时 harness 调用实际 `tui.Ask`，输入 `节点选择`，得到 `RESULT=节点选择`。
- 非 TTY：设置 `NO_COLOR=1` 后以同一 harness 管道输入，得到 `RESULT=节点选择`。

`internal/flows/nodeselect_group.go` 的变更函数语句覆盖率为 87.5%–100%。包级覆盖率仍为
`internal/flows` 17.4%、`internal/config` 67.1%；低覆盖来自包内既有系统交互流程，不是本次
新增主组识别 helper。未创建 RED/GREEN checkpoint commit：工作区包含用户正在进行的
`todo/` → `Records/` 迁移，本次没有擅自暂存或提交这些混合改动。
