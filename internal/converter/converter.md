# Clash/Mihomo 节点转换为 sing-box 配置指南

## 1. 基本原则

Clash/Mihomo YAML 转换为 sing-box JSON 不是简单字段重命名。

推荐采用以下结构：

```text
Clash YAML
    ↓
解析与字段归一化
    ↓
NodeIR 中间结构
    ↓
协议能力检查
    ↓
对应 sing-box 版本的配置生成器
    ↓
sing-box check
```

不要在解析 YAML 时直接拼接 sing-box JSON。

---

## 2. 通用字段映射

| Clash/Mihomo     | sing-box         | 说明          |
| ---------------- | ---------------- | ----------- |
| `name`           | `tag`            | 必须保证唯一      |
| `server`         | `server`         | 保留域名，不要提前解析 |
| `port`           | `server_port`    | 转换为整数       |
| `udp: false`     | `network: tcp`   | 禁用 UDP      |
| `interface-name` | `bind_interface` | 依赖系统网卡      |
| `routing-mark`   | `routing_mark`   | 主要用于 Linux  |
| `tfo`            | `tcp_fast_open`  | 按源配置保留      |
| `mptcp`          | `tcp_multi_path` | 依赖内核支持      |
| `dialer-proxy`   | `detour`         | 必须检查引用和循环   |

注意：

```yaml
network: ws
```

在 Clash 中表示传输层类型。

在 sing-box 中，WebSocket 应写入：

```json
{
  "transport": {
    "type": "ws"
  }
}
```

sing-box 的 `network` 通常表示 `tcp`、`udp` 或两者。

---

## 3. TLS 转换

所有支持 TLS 的协议应共用一个 TLS 转换模块。

典型映射：

```yaml
tls: true
servername: example.com
skip-cert-verify: false
client-fingerprint: chrome
alpn:
  - h2
reality-opts:
  public-key: xxx
  short-id: xxx
```

转换为：

```json
{
  "tls": {
    "enabled": true,
    "server_name": "example.com",
    "insecure": false,
    "alpn": ["h2"],
    "utls": {
      "enabled": true,
      "fingerprint": "chrome"
    },
    "reality": {
      "enabled": true,
      "public_key": "xxx",
      "short_id": "xxx"
    }
  }
}
```

### 特别注意

Clash 的证书 `fingerprint` 与 sing-box 的公钥哈希字段不是同一概念，不可直接复制。

处理策略：

```text
检测到 certificate fingerprint
    → 不自动转换
    → 输出兼容性警告
    → 忽略或要求用户手动配置
```

---

## 4. 传输层转换

### WebSocket

```yaml
network: ws
ws-opts:
  path: /api
  headers:
    Host: example.com
```

转换为：

```json
{
  "transport": {
    "type": "ws",
    "path": "/api",
    "headers": {
      "Host": "example.com"
    }
  }
}
```

### gRPC

```yaml
network: grpc
grpc-opts:
  grpc-service-name: service
```

转换为：

```json
{
  "transport": {
    "type": "grpc",
    "service_name": "service"
  }
}
```

### 不应静默转换的传输类型

以下传输缺少直接兼容实现时，应拒绝节点：

* XHTTP
* mKCP
* Domain Socket
* 不兼容的 HTTPUpgrade 变体

禁止将不支持的传输自动降级为 TCP。

---

## 5. 协议处理要求

## Shadowsocks

基础映射：

```text
cipher   → method
password → password
```

需要额外处理：

* 检查加密方法是否被 sing-box 支持；
* 检查 SIP003 插件兼容性；
* 将插件参数序列化为字符串；
* 保留 UDP-over-TCP 版本。

支持程度较高的插件：

* `obfs`
* `v2ray-plugin`

其他插件应明确标记为不支持。

---

## VMess

主要字段：

```text
uuid                 → uuid
cipher               → security
alterId              → alter_id
packet-encoding      → packet_encoding
global-padding       → global_padding
authenticated-length → authenticated_length
```

注意：

* `alterId: 0` 表示 AEAD；
* transport 必须单独转换；
* TLS 和 Reality 必须单独转换；
* 不要自动开启 multiplex。

---

## VLESS

主要字段：

```text
uuid            → uuid
flow            → flow
packet-encoding → packet_encoding
reality-opts    → tls.reality
```

需要特别检查：

* `flow` 是否为支持值；
* XHTTP 节点应拒绝；
* 非空 `encryption` 字段不可静默丢弃；
* Vision 节点不要自动启用 multiplex。

---

## Trojan

Trojan 通常要求 TLS。

主要处理：

* password；
* SNI；
* ALPN；
* 证书校验；
* Reality；
* WebSocket、gRPC 等 transport。

以下 Mihomo 扩展通常不可直接转换：

```yaml
ss-opts:
  enabled: true
```

检测到 Trojan-Go Shadowsocks 加密扩展时，应拒绝节点。

---

## Hysteria2

主要映射：

```text
password      → password
up            → up_mbps
down          → down_mbps
ports         → server_ports
hop-interval  → hop_interval
obfs          → obfs.type
obfs-password → obfs.password
```

注意：

* 带宽必须解析单位；
* `Kbps`、`Mbps`、`Gbps` 必须正确换算；
* `ports` 存在时，不应只保留单一 `port`；
* 用户名密码认证通常需要组合为单一 password；
* 新版本字段必须经过版本检查。

---

## TUIC

首先识别协议版本。

```text
TUIC v4：token
TUIC v5：uuid + password
```

推荐策略：

```text
TUIC v4 → 不支持
TUIC v5 → 转换
```

TUIC v5 主要映射：

```text
congestion-controller → congestion_control
udp-relay-mode        → udp_relay_mode
reduce-rtt            → zero_rtt_handshake
heartbeat-interval    → heartbeat
```

时间字段必须转换：

```yaml
heartbeat-interval: 10000
```

转换为：

```json
{
  "heartbeat": "10s"
}
```

不要将 `udp-relay-mode` 错误映射为 `udp_over_stream`。

---

## WireGuard

现代 sing-box 中应生成 WireGuard endpoint，而不是旧式 outbound。

转换时注意：

* `ip` 自动补 `/32`；
* `ipv6` 自动补 `/128`；
* 生成 `peers` 数组；
* 保留 `allowed_ips`；
* 保留 `reserved`；
* 保留 preshared key；
* 正确处理 keepalive；
* 区分普通 WireGuard 和 AmneziaWG。

AmneziaWG 专用字段不能直接转换为普通 WireGuard。

---

## 6. Multiplex 转换

可映射字段：

```text
smux.enabled         → multiplex.enabled
smux.protocol        → multiplex.protocol
smux.max-connections → multiplex.max_connections
smux.min-streams     → multiplex.min_streams
smux.max-streams     → multiplex.max_streams
smux.padding         → multiplex.padding
```

建议策略：

* 默认关闭；
* 仅在源节点显式配置时保留；
* 检查互斥字段；
* 不以“提高速度”为由自动开启；
* Vision、Reality 等组合单独验证。

---

## 7. DNS 与版本兼容

不要直接将：

```yaml
ip-version: ipv4
```

硬编码为某一个 sing-box 字段。

推荐在中间结构中保存：

```text
IPStrategy = IPv4Only
```

再由版本生成器决定使用：

* `domain_strategy`
* `domain_resolver`
* `route.default_domain_resolver`

建议按版本拆分生成器：

```text
generator/v1_13
generator/v1_14
```

不要在同一个生成器中堆积大量版本判断。

---

## 8. 应明确拒绝的节点

以下类型通常不存在可靠的直接转换：

* ShadowsocksR
* Mieru
* MASQUE
* OpenVPN
* TrustTunnel
* TUIC v4
* VLESS XHTTP
* 不支持的 Shadowsocks 插件
* AmneziaWG 专用扩展
* Mihomo 专用协议或传输

正确处理方式：

```text
保留原始节点信息
    ↓
从生成配置中排除
    ↓
输出明确的不支持原因
    ↓
显示成功转换数量
```

禁止静默丢弃字段或自动替换协议。

---

## 9. 验证流程

建议每次订阅更新都执行：

```text
1. YAML 语法检查
2. 节点字段归一化
3. 必填字段检查
4. 协议能力检查
5. detour 引用检查
6. detour 循环检查
7. 生成 sing-box JSON
8. sing-box format
9. sing-box check
10. 启动候选配置
11. 节点 TCP 测试
12. 节点 UDP 测试
13. 激活配置
14. 失败自动回滚
```

`sing-box check` 只能验证配置格式，不能验证：

* SNI 是否正确；
* Reality 公钥是否正确；
* WebSocket 路径是否正确；
* UDP 是否可用；
* DNS 是否可用；
* 测速地址是否可访问。

---

## 10. 推荐支持等级

| 等级          | 协议                                     |
| ----------- | -------------------------------------- |
| 完整支持        | HTTP、SOCKS、普通 Shadowsocks AEAD         |
| 结构化转换       | VMess、VLESS、Trojan                     |
| 专用转换器       | Hysteria2、TUIC v5                      |
| Endpoint 转换 | WireGuard                              |
| 条件支持        | AnyTLS、Snell、Hysteria v1、部分 SS 插件      |
| 明确不支持       | SSR、TUIC v4、XHTTP、Mieru、MASQUE、OpenVPN |

---

## 11. 实现优先级

优先完成以下模块：

1. TLS 字段归一化；
2. transport 独立转换；
3. Shadowsocks cipher 和插件检查；
4. TUIC 版本识别；
5. WireGuard endpoint 生成；
6. detour 引用与循环检查；
7. sing-box 版本化生成器；
8. 不支持节点报告；
9. 配置检查和自动回滚。

核心原则：

> 能准确转换时才生成；无法保持语义一致时，明确拒绝，不要生成看似合法但实际不可用的配置。
