# 房屋中介小程序 · 工程

这是《房屋中介小程序/技术方案-v1.3.md》的 M1 可运行实现。

当前已打通：

- Go + Gin + pgx 分层后端
- PostgreSQL 18 迁移与开发种子数据
- 开发态微信登录、JWT、手机号加密、姓氏/角色资料
- 地图聚合 / 房源详情 API
- 楼宇联想 + 房东发布房源 API（命中工业园区/写字楼才直接上架）
- 联系房东 API：确认后返回明文手机号/微信号（已授权时），写审计、7 天/每日限流
- 微信原生小程序地图首屏、登录/资料页、房源详情弹层、房东发布页、点击拨打
- React 管理站只读骨架（管理鉴权未接，不能公网部署）

## 目录

```text
房屋中介小程序/
├── api/openapi.yaml          M1 API 契约
├── server/                   Go API + 迁移 + 种子
├── miniprogram/              微信原生小程序（TypeScript）
├── admin-web/                React + Vite 管理站骨架
├── docker-compose.yml        本地 PostgreSQL 18 + Redis
└── 技术方案-v1.3.md          方案定稿
```

## 本地启动

### 1. 数据库

```bash
cd 房屋中介小程序
docker compose -p housing up -d postgres redis
```

端口刻意避开常见的 5432/6379：

- PostgreSQL：`localhost:55432`，库/用户/密码都是 `housing`
- Redis：`localhost:56379`

### 2. 后端

```bash
cd server
cp .env.example .env   # 本地可改端口/密钥；.env 不进 Git
go run ./cmd/seed      # 迁移 + 种子数据
go run ./cmd/api
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

### 4. 管理站

```bash
cd admin-web
npm install
npm run dev
```

打开 `http://127.0.0.1:5173`。

当前管理站是**只读预览骨架**：没有管理端鉴权，不能部署公网。M2 会补 TOTP、RBAC、HttpOnly 会话和审计，再开放写操作。

## 环境变量

后端从环境变量读取，开发默认值见 `server/internal/config/config.go`。生产必须设置：

| 变量 | 说明 |
|---|---|
| `APP_ENV` | `dev` / `test` / `prod` |
| `DATABASE_URL` | PostgreSQL DSN |
| `JWT_SECRET` | ≥32 字符 |
| `WECHAT_APPID` / `WECHAT_SECRET` | 微信小程序凭证 |
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
- 服务通知尚未接入：联系请求会落库并返回明文联系方式，但 `notificationStatus=not_configured`，房东还收不到微信服务通知。
- 管理站鉴权、文档/截图导入、小亿语音/解说尚未实现。
- 地图“3D/俯视”和微信订阅消息需要真机/真实小程序账号联调。
