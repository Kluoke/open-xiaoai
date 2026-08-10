# lx-go

一个用 Go（[goja](https://github.com/dop251/goja) + `goja_nodejs`）跑 [lx-music](https://github.com/lyswhut/lx-music-desktop) 自定义音源脚本的独立服务，对外暴露一套兼容 [pkg/music](../music) 期望的 "LX Sync Server" 风格 HTTP 接口。

## 核心设计：多音源自动加载 + 失败自动切换

把任意数量的 lx-music 自定义音源脚本（`.js` 文件）丢进 [`js/`](./js) 目录，启动时会：

1. 按**文件名排序**依次加载每一个 `.js` 文件，顺序即默认的音源优先级（越靠前越先被尝试）。
2. 从脚本头部注释里解析 `@name` 作为这个音源的名字（同时支持 `/*! ... */` 和 `/** ... */` 两种注释风格）。取不到 `@name` 时回退用文件名，并打日志提醒。
3. 每个脚本独立跑在自己的 goja 运行时里，互不影响：
   - 某个脚本加载失败（JS 语法错误、初始化时抛异常等）只会跳过它自己，打一条 `❌` 日志，其余脚本正常加载，服务不会被拖垮。
   - 两个脚本解析出同一个 `@name` 时视为冲突，只加载先加载的那个，后面同名的会被跳过并打日志（想用后面那个的话，改一下它的 `@name` 或者把冲突的文件移出 `js/` 目录）。
4. 启动日志会明确打印**已加载哪些音源、用的哪个文件、每个音源支持哪些平台/能力**，例如：

   ```
   ✅ [lx-go] 已加载音源: HYWmusic_beta_公益测试 (文件: HYWmusic_beta_公益测试.js)，支持平台: kg[musicUrl lyric pic], kw[musicUrl lyric pic], ...
   ✅ [lx-go] 已加载音源: 星海音乐源 (文件: xinghai-music-sourcev2.3.11.js)，支持平台: kg[musicUrl lyric pic], ...
   ✅ [lx-go] 已加载音源: 内置搜索源 (内置)，支持平台: kw[search], mg[search], wy[search]
   ```

5. **请求时按优先级挨个尝试，某个音源报错或者搜不到结果，自动切换下一个**，日志里会清楚地写明"当前用的是哪个音源"、"切到下一个了"：

   ```
   🔍 [music/search] 当前使用音源: HYWmusic_beta_公益测试，platform=wy keyword="稻香"
   ⚠️  [music/search] 音源 HYWmusic_beta_公益测试 未找到结果 (xxx)，自动切换下一个音源
   🔍 [music/search] 当前使用音源: 内置搜索源，platform=wy keyword="稻香"
   ✅ [music/search] 音源 内置搜索源 命中，platform=wy keyword="稻香"
   ```

   只有一个音源会在这次请求里真正命中（第一个成功返回结果的），不会同时打给所有音源。

6. 一个音源脚本是否支持某个平台的某个能力（`search` / `musicUrl`），完全以它自己在初始化时上报的 `send(EVENT_NAMES.inited, { sources: {...} })` 为准 —— 这是运行时读出来的，不是靠猜脚本代码。

### 启动自检：加载后先拿真实歌曲测一次，测不通不会进音源列表

光是脚本能跑起来（`inited` 事件正常）不代表它真的能用——很多音源脚本依赖的第三方接口本身可能已经失效。所以每个脚本加载成功之后，`lx-go` 会用固定的测试用例（周杰伦《稻香》）**真实发一次网络请求**验证它能不能用：

- 优先用 `search` 自检：脚本声明支持哪个平台的 `search`，就用"稻香"去搜那个平台，能搜到结果才算通过。
- 如果脚本压根不支持 `search`（目前大多数第三方音源脚本都是这样，只做 `musicUrl`），就退化成用一个已知有效的网易云歌曲 ID（《稻香》/周杰伦）测一次 `musicUrl`，能拿到直链才算通过。
- 两者都不支持的脚本（比如只做歌词/封面）没法用这个测试用例验证，直接放行加入。

**自检不通过的音源不会被加入音源列表**，并会打印清楚的错误原因，方便你判断是脚本本身的问题还是它依赖的上游接口挂了。实测日志（当前 `js/` 目录里的几个真实脚本 + 1 个内置搜索源，其中 lx-玉宁熙 因为上游接口对测试歌曲返回“ID 不存在”被自检刷掉）：

```
🧪 [lx-go] 正在自检音源: HYWmusic_beta_公益测试 (文件: HYWmusic_beta_公益测试.js)，测试用例: 稻香/周杰伦 ...
✅ [lx-go] HYWmusic_beta_公益测试: 自检通过: musicUrl(wy, 稻香) 返回了有效直链
✅ [lx-go] 已加载音源: HYWmusic_beta_公益测试 (文件: HYWmusic_beta_公益测试.js)，支持平台: ...

🧪 [lx-go] 正在自检音源: lx-玉宁熙-Pro (文件: lx-玉宁熙V1.2.2.js)，测试用例: 稻香/周杰伦 ...
❌ [lx-go] 自检失败: musicUrl(wy, 稻香) 报错: JS Promise 被拒绝: Error: 网易云歌曲ID不存在，未加入音源列表: lx-玉宁熙-Pro (文件: lx-玉宁熙V1.2.2.js)，请检查该脚本或其上游接口是否可用

🧪 [lx-go] 正在自检音源: 星海音乐源 (文件: xinghai-music-sourcev2.3.11.js)，测试用例: 稻香/周杰伦 ...
✅ [lx-go] 星海音乐源: 自检通过: musicUrl(wy, 稻香) 返回了有效直链

✅ [lx-go] 已加载音源: 内置搜索源 (内置)，自检通过: search(wy, "稻香") 有返回结果，支持平台: kw[search], mg[search], wy[search]
```

可以看到 "lx-玉宁熙-Pro" 这个脚本本身没跑出语法错误，但它请求的上游接口对这首测试歌曲返回了"网易云歌曲ID不存在"，自检直接判定它不可用，没有把它加进最终的音源列表——这正是自检要防住的情况（脚本能跑、但实际不可用）。

如果不想每次启动都发真实网络请求（比如离线开发/CI），可以设置环境变量跳过自检：

```bash
LX_GO_SKIP_SELFTEST=1 go run ./cmd/lxgo-server
```

### 内置搜索兜底

真实的 lx-music 第三方音源脚本几乎都只实现 `musicUrl`（拿到歌曲信息之后解析直链），搜索能力通常是客户端自带的，脚本本身不管。所以 `js/` 目录下的脚本大概率一个都不支持 `search`。

为了让 `GET /api/music/search` 真的能用，`lx-go` 内置了一个只做 `search` 的兜底音源（覆盖 `wy`/`kw`/`mg`，优先级最低），保证在没有专门搜索脚本的情况下也能搜到歌。如果你自己放进 `js/` 目录的某个脚本以后也实现了 `search`，它会被优先使用。

## HTTP 接口

### `GET /api/music/search`

音乐搜索，支持 `kw`、`kg`、`tx`、`wy`、`mg`（取决于当前加载的音源脚本里哪些声明了 `search` 能力）。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `name` | 是（或用 `q`） | 搜索关键词 |
| `source` | 是 | 平台代码：`wy`/`tx`/`kw`/`kg`/`mg` |
| `limit` | 否 | 返回条数，默认由脚本自行决定（一般 20） |

响应是一个裸数组（不是 `{list:[...]}` 包一层），每一项形如：

```json
[
  {
    "name": "稻香",
    "singer": "周杰伦",
    "albumName": "魔杰座",
    "id": "3357698666",
    "source": "wy",
    "interval": "03:43",
    "meta": { "wy": { "id": "3357698666" } }
  }
]
```

没有任何已加载音源为该 `source` 声明了 `search` 能力时返回 `404`；所有支持的音源都搜索失败/搜不到结果时返回 `500`。

### `POST /api/music/url`

获取音乐播放直链，支持 `kw`、`kg`、`tx`、`wy`、`mg`。

请求体：

```json
{
  "songInfo": { "id": "3357698666", "name": "稻香", "singer": "周杰伦", "source": "wy" },
  "quality": "320k"
}
```

`songInfo` 直接透传给音源脚本（一般就是 `GET /api/music/search` 返回的某一项），`songInfo.source` 决定去哪个平台解析；也可以用 `?source=` 查询参数指定。

响应：

```json
{ "url": "https://.../xxx.mp3", "type": "320k" }
```

某个音源解析失败会自动换下一个，全部失败时返回 `500`。

#### Header：`x-req-id`（可选，配合 SSE 进度追踪）

请求时带上 `x-req-id: <任意唯一字符串>`，就可以在解析过程中通过 `GET /api/music/progress?reqId=<同一个值>` 实时订阅"当前在尝试哪个音源、成功了还是失败了"。两个请求是并发发出的（一个 `POST /api/music/url` 一个 `GET .../progress`），`reqId` 是把它们串起来的唯一凭据。

### `GET /api/music/progress?reqId=xxx`

SSE（`text/event-stream`）接口，订阅同一个 `reqId` 的 `POST /api/music/url` 解析进度。每条事件形如：

```
data: {"stage":"trying","source":"星海音乐源","platform":"wy","message":"正在尝试音源 星海音乐源","done":false}

data: {"stage":"failed","source":"星海音乐源","platform":"wy","message":"xxx","done":false}

data: {"stage":"success","source":"内置搜索源","platform":"wy","message":"https://.../xxx.mp3","done":true}
```

`stage` 取值：`trying`（正在尝试某个音源）/`failed`（该音源失败，即将切下一个）/`success`（解析成功，附带最终直链）/`error`（全部音源都失败）。`done: true` 表示这是最后一条事件，连接会随之关闭。60 秒内没有任何事件也会自动断开。

### `GET /api/music/download`

代理下载音乐文件，支持自动注入 ID3 标签。

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `url` | 是 | `POST /api/music/url` 返回的直链 |
| `filename` | 否 | 保存的文件名，默认从 `url` 里猜 |
| `tag` | 否 | `1`（默认）注入 ID3 标签，`0` 原样转发不处理 |
| `name` | 否 | 写入 ID3 标题 |
| `singer` | 否 | 写入 ID3 艺术家 |
| `album` | 否 | 写入 ID3 专辑 |
| `pic` | 否 | 封面图片 URL，会被下载并作为 ID3 封面（`APIC`）内嵌到文件里 |

响应是音频文件的二进制流（`Content-Disposition: attachment`）。标签注入失败（比如封面下载失败）不会导致整次下载失败，会退化为不带该项标签继续。

### `GET /api/music/switch-source`

手动把"默认搜索音源"切换到下一个（按已加载音源的优先级顺序循环），并在响应和日志里打印切换前/切换后分别是哪个音源。

`GET /api/music/search` 每次请求都会把当前的默认搜索音源排到 fallback 顺序的最前面优先尝试（如果它不支持这次请求的平台则不受影响，还是按原顺序）。这个接口就是用来手动干预"优先用哪个搜索源"的——比如默认源最近老失败/被限流了，可以调一下这个接口切到下一个。

响应：

```json
{ "previous": "内置搜索源", "current": "其他搜索源", "pool": ["内置搜索源", "其他搜索源"] }
```

`pool` 是当前所有支持 `search` 的音源列表（按优先级顺序）。如果没有任何音源支持 `search`，返回 `404`。

> 早期只有内置搜索源支持 `search`，池子里只有一个成员；后来 `js/` 目录里加了"溯音音源""非常刀"这两个本身就支持 `search` 的脚本之后，池子里就有多个成员了，这个接口才真正在多个搜索源之间切换。具体池子里有哪些，以启动日志和 `GET /sources` 的实际输出为准。

## `/health`、`/sources`

诊断用：`/health` 返回当前加载了哪些音源；`/sources` 返回每个音源上报的 `inited.sources` 完整元信息（平台名称、支持的 action 列表）。

## 启动之后怎么测：搜索《稻香》全流程

```bash
go run ./cmd/lxgo-server
# 看到类似下面的日志说明启动成功（自检通过的音源才会出现在这里）：
#   ✅ [lx-go] 已加载音源: HYWmusic_beta_公益测试 ...
#   已加载 N 个音源，server started at :8080  （N 取决于 js/ 目录下有多少脚本自检通过）
```

**1. 搜索"稻香"**（`source=wy` 表示搜网易云平台）：

```bash
curl 'http://127.0.0.1:8080/api/music/search?name=稻香&source=wy'
```

返回一个裸数组，正常情况下第一条差不多长这样：

```json
[
  {
    "name": "稻香",
    "singer": "周杰伦-/Montagem",
    "albumName": "青花瓷",
    "id": 3357698666,
    "source": "wy",
    "interval": "65:48",
    "meta": { "picture": "http://...", "wy": { "id": 3357698666 } }
  }
]
```

**2. 拿着搜索结果里的 `id`/`name`/`singer` 去取播放直链**：

```bash
curl -X POST 'http://127.0.0.1:8080/api/music/url' \
  -H 'Content-Type: application/json' \
  -d '{"songInfo":{"id":"3357698666","name":"稻香","singer":"周杰伦","source":"wy"},"quality":"320k"}'
```

返回：

```json
{ "url": "https://.../xxx.mp3", "type": "320k" }
```

**3.（可选）带上 `x-req-id`，同时开一个终端订阅解析进度**：

```bash
# 终端 A：先订阅（会一直挂着直到收到 done:true 的事件或者超时）
curl -N 'http://127.0.0.1:8080/api/music/progress?reqId=test-1'

# 终端 B：发起取直链请求，带上同一个 reqId
curl -X POST 'http://127.0.0.1:8080/api/music/url' \
  -H 'Content-Type: application/json' \
  -H 'x-req-id: test-1' \
  -d '{"songInfo":{"id":"3357698666","name":"稻香","singer":"周杰伦","source":"wy"},"quality":"320k"}'
```

终端 A 会实时看到类似：

```
data: {"stage":"trying","source":"HYWmusic_beta_公益测试","platform":"wy","message":"正在尝试音源 HYWmusic_beta_公益测试","done":false}

data: {"stage":"success","source":"HYWmusic_beta_公益测试","platform":"wy","message":"https://.../xxx.mp3","done":true}
```

**4. 用第 2 步拿到的直链下载文件（顺便打上 ID3 标签）**：

```bash
curl -o 稻香.mp3 'http://127.0.0.1:8080/api/music/download?url=<第2步返回的url，需要做URL编码>&name=稻香&singer=周杰伦&album=魔杰座'
```

**5. 手动切换默认搜索音源**：

```bash
curl 'http://127.0.0.1:8080/api/music/switch-source'
# {"previous":"内置搜索源","current":"内置搜索源","pool":["内置搜索源"]}
```

**6. 看一下当前加载了哪些音源、每个音源支持什么**：

```bash
curl 'http://127.0.0.1:8080/health'
curl 'http://127.0.0.1:8080/sources'
```

## 接入 [pkg/music](../music)

`pkg/lx-go` 既可以当独立服务跑（见上面"启动之后怎么测"），也可以直接当 Go 库 import 到别的进程里用——`pkg/music` 用的就是后一种方式，不需要你手动另起一个 `lx-go` 进程/端口。

### 内嵌模式（推荐）：不用手动起进程

`pkg/music` 只要在配置里打开 `music.lx.embedded: true`，就会在自己进程内部直接调用这个包（`import "github.com/cxjava/open-xiaoai/pkg/lx-go"`），加载 `js/` 目录、跑自检、搜索/取直链全部是进程内函数调用，没有 HTTP、没有端口、没有回环网络：

```yaml
music:
  lx:
    enabled: true
    embedded: true
    embedded_js_dir: "../lx-go/js"   # 相对/绝对路径都行，指向你的音源脚本（可以多个）目录
    download: true
    source: "wy"
    quality: "128k"
```

具体是怎么接进去的：

- `pkg/music/go.mod` 用 `replace github.com/cxjava/open-xiaoai/pkg/lx-go => ../lx-go` 把这个包当本地模块依赖引进来。
- `pkg/music/lx_embedded.go` 里的 `EmbeddedLX` 类型包了一层 `lxgo.LoadSources` + `Registry.Search`/`Registry.ResolveURL`，实现了跟 `LXClient`（HTTP 版）完全一样的 `Resolve`/`Download` 接口，两者对 `pkg/music` 来说可以互换。
- `Module.New()` 会根据 `cfg.LX.Embedded` 自动选：`true` 就用 `EmbeddedLX`（本文件），否则退回 `LXClient`（走 `base_url` 打 HTTP，见下面"独立服务模式"）。

这套接法也是实测跑通过的：`pkg/music` 进程里直接调用 `EmbeddedLX.Resolve()`/`Download()`（不经过任何 HTTP/端口），完整走了一遍加载 6 个音源脚本、自检、搜索「周杰伦 稻香」、取直链、下载到本地文件的全流程，下载下来是真实可播放的 mp3。

`Registry.Search`/`Registry.ResolveURL` 这两个方法就是给这种"当库直接 import"的场景准备的：跟 `GET /api/music/search`/`POST /api/music/url` 这两个 HTTP handler 用的是同一套按优先级 fallback 的逻辑，只是没有 HTTP 层那些响应缓存和 SSE 进度推送——如果你要接的不是 `pkg/music` 而是别的 Go 程序，也可以直接照着 `lx_embedded.go` 这个写法抄。

### 独立服务模式：单独跑一个进程，走 HTTP

如果不想让 `pkg/music` 直接依赖这个 Go 包（比如两边要跑在不同机器上、或者接的不是 Go 程序），可以按老办法单独起一个 `lx-go` 进程，`pkg/music` 通过 `base_url` 走 HTTP 对接：

```bash
cd pkg/lx-go && go run ./cmd/lxgo-server   # 默认监听 :8080
```

```yaml
music:
  lx:
    enabled: true
    base_url: "http://127.0.0.1:8080"   # lx-go 默认监听 8080
    source: "wy"
    quality: "128k"
    download: true          # 可选：是否代理下载到本地再播放
    # username/password/user_token/frontend_auth 都不用填——
    # lx-go 本身不做鉴权，留空的话 pkg/music 也不会去调用 /api/user/login
```

`pkg/music` 里的 `LXClient`（`pkg/music/lx.go`）就是照着这个协议写的（`GET /api/music/search`、`POST /api/music/url`、`GET /api/music/download`），两边接口本来就对得上，用这种模式接入不需要改任何 Go 代码。这条路径同样实测跑通过：搜索「周杰伦 稻香」→ 取直链 → 代理下载到本地文件，全流程一次跑通，下载下来的文件是真实可播放的 mp3（7MB+）。

#### 端口需要改吗？

不需要特意改。`lx-go` 默认监听 `:8080`，`pkg/music` 自己的文件服务默认监听 `18080`（见 `pkg/music/config.go` 的 `HTTPConfig.Port`），两个端口本来就不冲突，可以就用默认值。如果你的环境 8080 已经被别的东西占了，启动时加 `-addr :其他端口` 就行（`go run ./cmd/lxgo-server -addr :9090`），然后同步改 `pkg/music` 配置里的 `base_url`。

（这条端口说明只适用于"独立服务模式"；用上面推荐的"内嵌模式"完全不涉及端口。）

### 联调时发现的一个坑，已经顺手修了

真实压测的时候发现：免费音源脚本解析出来的网易云 CDN 直链，实际有效期可能很短（实测一两分钟内就会被服务端返回 `auth failed - expired url` 拒绝）。而 `lx-go` 原来对 `/api/music/url` 的响应做了 10 分钟缓存（独立服务模式下才会命中这个缓存），会导致——如果 `pkg/music` 这边搜索/取直链和真正下载播放之间隔了几分钟，会用上一个早就已经过期的直链，直接下载失败。

已经把这个缓存时间从 10 分钟改成 20 秒（`pkg/lx-go/api.go` 里的 `urlCacheTTL`），只用来去重"短时间内重复点了好几次同一首歌"这种情况，不再长时间复用直链。**结论**：这属于免费公共音源接口本身的正常特性（链接短时效），不是 `lx-go` 或 `pkg/music` 的协议兼容性问题；`pkg/music` 那边"搜索完立刻取直链、取到直链立刻下载/播放"的调用方式本来就是紧跟着来的，不会受影响。内嵌模式完全不走这层缓存，天然不受影响。

## 给音源脚本提供的沙箱环境

为了尽量兼容真实的、未经改造的 lx-music 自定义音源脚本（包括经过混淆的），每个脚本运行时都能访问到：

- `globalThis.lx.request/send/on/EVENT_NAMES/env/version/currentScriptInfo/utils`（`lx-music` 自定义音源协议的标准接口）
- 全局 `Buffer`、`console`（通过 `goja_nodejs` 提供，行为对齐 Node）
- `lx.utils.buffer.from/bufToString`（utf-8/hex/base64 编解码）
- `lx.utils.crypto.md5/aesEncrypt/aesDecrypt`（部分音源解析网易云 `eapi` 等加密接口时需要用到）

## 开发/测试

```bash
go test ./...
```

`api_test.go` / `engine_test.go` / `registry_test.go` / `switch_source_test.go` 都是纯本地测试，不依赖网络。如果想验证真实的、放在 `js/` 目录下的音源脚本（包括启动自检的真实网络行为），可以自己写一个临时测试直接调用 `LoadSources("./js")`，观察日志输出即可；不想真的发网络请求时可以设置 `LX_GO_SKIP_SELFTEST=1` 跳过启动自检。
