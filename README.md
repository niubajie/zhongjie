# 房屋中介小程序 · 工程

这是《房屋中介小程序/技术方案-v1.5.md》的 M1 可运行实现。

当前已打通：

- Go + Gin + pgx 分层后端
- PostgreSQL 18 迁移与开发种子数据
- 开发态微信登录、JWT、手机号加密、姓氏/角色资料
- 地图聚合 / 房源详情 API
- 楼宇联想 + 房东发布房源 API（命中工业园区/写字楼才直接上架）
- 联系房东 API：确认后返回明文手机号/微信号（已授权时），写审计、7 天/每日限流
- 通知 outbox 与 30 天在租巡检（`cmd/worker` 每 6 小时生成待巡检记录）
- 房东文档导入：`.docx` / `.xlsx` 解析、楼宇匹配、逐行创建房源
- 小亿助手：会话、需求槽位、推荐排序、逐套解说、图片/视频/地图动作脚本
- 微信原生小程序地图首屏、登录/资料页、房源详情弹层、房东发布页、小亿页、点击拨打
- React 管理站：独立管理员 JWT、RBAC 角色、登录审计、真实统计与房源列表（只读）

## 目录

```text
房屋中介小程序/
├── api/openapi.yaml          M1 API 契约
├── server/                   Go API + 迁移 + 种子
├── miniprogram/              微信原生小程序（TypeScript）
├── admin-web/                React + Vite 管理站
├── docker-compose.yml        本地 PostgreSQL 18 + Redis
└── 技术方案-v1.5.md          方案定稿
```

## 本地启动

### 1. 数据库

```bash
cd 房屋中介小程序
docker compose -p housing up -d postgres redis
```

端口刻意避开常见的 5432/6379：

- PostgreSQL：`localhost:56000`，库/用户/密码都是 `housing`
- Redis：`localhost:56379`

### 2. 后端

```bash
cd server
cp .env.example .env   # 本地可改端口/密钥；.env 不进 Git
go run ./cmd/seed      # 迁移 + 种子数据
go run ./cmd/api
# 另开终端：
go run ./cmd/worker    # 30 天巡检 + 通知 outbox（30 秒轮询）
```

默认监听 `http://127.0.0.1:8080`。

`APP_ENV=dev` 时默认开启开发登录：

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s -X POST http://127.0.0.1:8080/v1/auth/wechat/login \
  -H 'Content-Type: application/json' \
  -d '{"devOpenid":"dev-tenant-1"}'
```

把返回的 token 用于后续请求：

```bash
curl -s http://127.0.0.1:8080/v1/me \
  -H "Authorization: Bearer <token>"
```

地图：

```bash
curl -s 'http://127.0.0.1:8080/v1/properties/map?cityCode=440300&swLat=22.45&swLng=113.85&neLat=22.60&neLng=114.15&zoom=13'
```

楼宇联想：

```bash
curl -s 'http://127.0.0.1:8080/v1/buildings/suggest?cityCode=440300&keyword=科技'
```

房东发布（先登录 `dev-landlord-1` 取 token）：

```bash
curl -s -X POST http://127.0.0.1:8080/v1/landlord/properties \
  -H "Authorization: Bearer <token>" \
  -H 'Content-Type: application/json' \
  -d '{"buildingUuid":"<楼宇uuid>","areaSqm":120,"rentCents":1680000,"layout":"3+1"}'
```

联系房东（租客需已绑定手机号）：

```bash
curl -s -X POST http://127.0.0.1:8080/v1/contact-requests \
  -H "Authorization: Bearer <tenant-token>" \
  -H 'Content-Type: application/json' \
  -d '{"propertyUuid":"<房源uuid>","consent":true,"note":"想周末看房"}'
```

快速回归：

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/smoke.ps1
```

### 3. 微信小程序

1. 用微信开发者工具打开 `miniprogram/`。
2. AppID 可先用测试号；`project.config.json` 当前是 `touristappid`。
3. 在“详情 → 本地设置”勾选“不校验合法域名”。
4. `miniprogram/app.ts` 里的 `apiBase` 默认是 `http://127.0.0.1:8080`；真机预览要改成电脑局域网 IP。
5. 登录页有“开发态：使用 13800138000”按钮；正式环境要关闭。

当前小程序包含：

- 地图首屏：全屏 `map`、缩放 3–16、拖动/缩放后查当前屏幕范围
- 楼宇聚合 marker：低 zoom 显示“X 套”，点击下钻
- 房源详情弹层：面积/租金/户型/楼层/图片数量
- “联系房东”二次确认框（真实联系接口在 M3 接入）
- 小亿欢迎动画/气泡（语音在 M4 接入）
- 登录 → 手机号 → 姓氏/角色资料流
- 房东发布页：楼宇联想选择、面积/月租校验、直接上架或转 `pending`
- 联系方式设置：手机号/微信号是否向确认联系的租客展示
- 文档导入页：从聊天记录选 `.docx` / `.xlsx` 上传并显示导入结果
- 小亿页：文字对话收集需求、竖向房源卡、逐段解说、图片/地图/视频场景动作、联系房东

### 4. 管理站

```bash
cd admin-web
npm install
npm run dev
```

打开 `http://127.0.0.1:5173`。

开发种子管理员：

```text
用户名：admin
密码：Admin123!
```

`cmd/seed` 只在非 `prod` 环境打印密码；生产用 `ADMIN_SEED_PASSWORD` 或管理端初始化流程。

当前管理站已接入：

- 独立管理员 JWT（issuer `housing-admin`，与租客 token 不通用）
- 角色字段与 `RequireAdmin` 中间件
- 登录成功写 `audit_logs`
- 真实统计 / 房源列表
- 管理站文字录入：解析楼宇/面积/租金，按房东手机号匹配后创建房源
- 导入任务、楼宇词典、联系记录、30 天巡检列表
- 房源上架/下架审核（写 `audit_logs`）
- TOTP 绑定、启用、登录校验
- token 只放内存，刷新页面需重新登录

尚未接入：HttpOnly 刷新会话、导入任务管理页面。部署公网前必须补齐。

## 环境变量

后端从环境变量读取，开发默认值见 `server/internal/config/config.go`。生产必须设置：

| 变量 | 说明 |
|---|---|
| `APP_ENV` | `dev` / `test` / `prod` |
| `DATABASE_URL` | PostgreSQL DSN |
| `JWT_SECRET` | 租客端 JWT，≥32 字符 |
| `ADMIN_JWT_SECRET` | 管理站 JWT，必须与租客端不同，≥32 字符 |
| `ADMIN_JWT_ACCESS_TTL` | 管理站 token 有效期，默认 `8h` |
| `WECHAT_APPID` / `WECHAT_SECRET` | 微信小程序凭证 |
| `WECHAT_SUBSCRIBE_TEMPLATE_ID` | 房东联系提醒订阅消息模板 ID；为空时 outbox 标记 `not_configured` |
| `WECHAT_SUBSCRIBE_PAGE` | 订阅消息跳转页面，默认 `pages/map/index` |
| `PII_ENC_KEY` | base64 编码的 32 字节 AES-GCM key |
| `PII_HASH_KEY` | 手机号 HMAC key |
| `DEV_LOGIN_ENABLED` | 生产必须为 `false` |
| `ADMIN_ORIGIN` | 管理站 CORS 来源 |

## 测试与构建

```bash
cd server
go vet ./...
go test ./...

cd ../miniprogram
npm run typecheck

cd ../admin-web
npm run build
```

当前环境没有 C 编译器，`go test -race` 无法启用；这是本机限制，不是代码通过。CI 环境需要补 `CGO_ENABLED=1` 和 gcc。

## 已知边界

- 后端当前用 `lat/lng` 范围查询，PostGIS 空间函数尚未接入（迁移可后续新增）。
- 图片只返回 `objectKey`，COS 签名 URL 尚未接入；小程序用占位图。
- 服务通知已写入 `notification_outbox`，但真实微信订阅消息发送还需要模板 ID/access_token；没有凭证时状态为 `not_configured`。
- 小亿当前是文字/规则版：没有接真实 ASR/TTS/LLM；音频、口型、Live2D 和真实视频播放器未接。
- 文档导入支持 `.docx` / `.xlsx` 文本字段；截图 OCR、Word/Excel 图片提取、导入任务管理页未接。
- 管理站录入目前只接文字；图片/视频录入需要 OCR/ASR 服务，先返回未配置。
- 管理站 token 在内存，刷新即退出；HttpOnly 刷新会话未接。
- 地图“3D/俯视”和微信订阅消息需要真机/真实小程序账号联调。
