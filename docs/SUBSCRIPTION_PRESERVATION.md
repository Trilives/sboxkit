# 订阅必要配置保留规则

sboxkit 不只提取节点。订阅刷新和重新生成始终从保存的 `raw.*` 原文重新解析，
并按以下白名单保留会影响运行语义的配置。未列出的 Clash 专有字段不会被误抄到
sing-box JSON；原生 sing-box passthrough 则不受白名单限制。

## Clash / Mihomo 来源

| 源字段 | 处理方式 |
|---|---|
| `dns.enhanced-mode: fake-ip` | 启用 sing-box 1.12+ 的 `type: fakeip` DNS server |
| `dns.fake-ip-range` / `fake-ip-range6` | 映射为 `inet4_range` / `inet6_range` |
| `dns.fake-ip-filter` | 精确域名、`+.`/`*.` 后缀和普通通配符转换为优先于 fakeip catch-all 的 DNS 规则 |
| 根级 `hosts` | 转换为 `type: hosts` server 的 `predefined`，并置于普通 DNS 规则之前 |

`RULE-SET,` / `GEOSITE,` 形式的 fake-ip-filter 依赖 Mihomo rule-provider 语义，
不能直接搬到 sing-box，因此当前不转换；后续若支持，必须先解析对应 provider，
不能静默生成无效引用。

## sing-box 原生来源

### passthrough（`customize=false`）

整份 JSON 原样保留，包括 sboxkit 尚不认识的 `ntp`、`services`、`endpoints`、
自定义 route/DNS 字段和未来新增字段。只在缺失时补充既定的 Clash API 面板配置。

### 统一重建（`customize=true`）

节点重新进入 sboxkit 的完整生成管线，同时保留下列源语义：

- `type: fakeip` server 的地址范围；
- fakeip catch-all 之前的域名排除规则；
- `type: hosts` server 的 `predefined` 主机记录。

源 DNS server 标签不会直接复用，因为统一重建会创建自己的 DNS server；排除规则
会改写为指向 sboxkit 的真实 DNS server，避免留下悬空标签。

## 合并优先级

1. sboxkit 部署必需字段：mixed/TUN 入站、运行时路径、Clash API 面板。
2. 源订阅的必要语义：hosts、fake IP 范围与排除规则。
3. 用户 `customize.json` 中的 DNS/分流设置。
4. sboxkit 默认值。

生成配置必须通过随包 sing-box 的 `check`。当前使用 sing-box 1.12 引入的现代
FakeIP DNS server 格式；旧的 `dns.fakeip` 已在 1.14 移除，不能重新生成。

参考：[FakeIP DNS server](https://sing-box.sagernet.org/configuration/dns/server/fakeip/)、
[1.12 迁移指南](https://sing-box.sagernet.org/migration/)、
[DNS rule action](https://sing-box.sagernet.org/configuration/dns/rule_action/)。
