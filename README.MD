![heart-rate-server](https://socialify.git.ci/xihan123/heart-rate-server/image?description=1&forks=1&issues=1&language=1&name=1&owner=1&pulls=1&stargazers=1&theme=Auto)

![Go](https://img.shields.io/badge/Go-1.27%2B-00ADD8.svg?logo=go&logoColor=white)
![SQLite](https://img.shields.io/badge/SQLite-003B57.svg?logo=sqlite&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-DC382D.svg?logo=redis&logoColor=white)
[![Docker Build and Push](https://github.com/xihan123/heart-rate-server/actions/workflows/docker-build-push.yml/badge.svg)](https://github.com/xihan123/heart-rate-server/actions/workflows/docker-build-push.yml)
[![License](https://img.shields.io/github/license/xihan123/heart-rate-server.svg)](LICENSE)

# heart-rate-server

一个用 Go 写的心率服务端，接收心率数据，提供查询接口和展示页面。账号存在 SQLite，近期心率记录存在 Redis。

可以配合 [HeartRateHook](https://github.com/xihan123/HeartRateHook) 上报数据，也可以把展示页面放进直播软件的浏览器源。

## 功能

- 注册、登录和 Cookie 自动续期
- 上报心率，校验时间戳及 1-250 BPM 范围
- 查询最新心率，提供可嵌入的展示页面
- 公开查看 UUID 与私密上报令牌分离

## 使用说明

### 示例服务

[![公开服务](https://img.shields.io/badge/Server-公开服务(全球CDN)-blue?style=for-the-badge)](https://public-heart-rate-api.xihan.website/)

注册登录就可以试用。

**示例服务只用于测试，没有接入机器人，也不保证稳定性，不要用于生产环境。**

### 注册和上报

1. 进入首页，点击「立即开始」。

![首页](https://github.com/user-attachments/assets/985e63da-0919-4871-8008-7ee77d904305)

1. 注册一个账号，再登录。不想注册的也可以用 `test:test123456` 测试。

![登录注册](https://github.com/user-attachments/assets/06b76b7c-03fb-4115-9681-8abc4908d1f2)

1. 复制「数据上报接口 (POST)」中的地址，填到上报设备。

![上报地址](https://github.com/user-attachments/assets/7f178fce-1a28-4342-b9eb-e83b1c3c320a)

使用 [HeartRateHook](https://github.com/xihan123/HeartRateHook) 时去掉地址末尾的 `receive_data`，保留 `/uuid/私密上报令牌/` 部分。不要换成公开查看 UUID，也不要把这个地址分享出去。截图中的地址只是示例，实际使用自己登录后拿到的地址。

### 展示心率

复制登录后显示的查看地址，在浏览器里打开，或填到直播软件的浏览器源中。查看地址可以公开分享，不包含上报令牌。

![心率展示](https://github.com/user-attachments/assets/aaefdb14-baf6-425c-be75-0d9b9ffaaaeb)

## 环境要求

- 源码运行：Go 1.27 或更高版本、C 编译器和 Redis。SQLite 驱动需要 CGO。
- Docker 运行：Docker 和可连接的 Redis，不需要在宿主机安装 Go。
- 浏览器登录需要 HTTPS，部署时在反向代理上配置。认证 Cookie 固定启用 `Secure`。

## 从源码运行

克隆项目，复制配置文件：

```sh
git clone https://github.com/xihan123/heart-rate-server.git
cd heart-rate-server
cp .env.example .env
```

编辑 `.env`，填写 Redis 地址和两个 Cookie 密钥。密钥分别用下面的命令生成：

```sh
openssl rand -hex 64  # COOKIE_HASH_KEY
openssl rand -hex 32  # COOKIE_BLOCK_KEY
```

服务不会自动读取 `.env`。Linux/macOS 在同一个 shell 中导入配置，再启动：

```sh
set -a
. ./.env
set +a
go mod download
go run .
```

Windows PowerShell：

```powershell
Get-Content .env | ForEach-Object {
    if ($_ -match '^\s*([A-Z_][A-Z0-9_]*)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
}
go mod download
go run .
```

密钥缺失、不是 Hex 或长度不对时，服务会在打开数据库前退出。

## Docker 部署

同样需要先复制并填写 `.env`，然后构建镜像：

```sh
docker build -t heart-rate-server .
mkdir -p data
```

镜像以 UID/GID `10001:10001` 运行。Linux 绑定挂载需要在启动前执行 `sudo chown -R 10001:10001 data`，并确保目录及已有数据库文件可写。SQLite 创建日志文件也需要目录写权限，不要改用 root 运行服务。

运行容器：

```sh
docker run -d -p 8080:8080 --env-file .env \
    --add-host host.docker.internal:host-gateway \
    -e REDIS_ADDR=host.docker.internal:6379 \
    -e DB_DSN=/data/heartrate.db -v "$PWD/data:/data" \
    --name heart-rate-server heart-rate-server
```

这个例子连接宿主机 Redis，其他部署方式请修改 `REDIS_ADDR`。数据库保存在挂载的 `data` 目录，重建容器时继续挂载同一个目录。

保留 `-e DB_DSN=/data/heartrate.db`：`.env.example` 中的相对路径会覆盖镜像默认值，导致容器尝试在不可写的 `/app` 下创建数据库。

## 环境变量

| 变量 | 用途 | 默认值 |
| ------ | ------ | -------- |
| `SERVER_PORT` | 监听端口 | `8080` |
| `DB_DSN` | SQLite 路径，支持相对或绝对路径 | 本地 `heartrate.db`，镜像 `/data/heartrate.db` |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `REDIS_PASSWORD` | Redis 密码 | 空 |
| `REDIS_DB` | Redis 数据库索引 | `0` |
| `BCRYPT_COST` | 密码哈希计算成本 | `10` |
| `COOKIE_HASH_KEY` | Cookie 签名密钥，64 字节 / 128 个 Hex 字符 | 必填 |
| `COOKIE_BLOCK_KEY` | Cookie 加密密钥，32 字节 / 64 个 Hex 字符 | 必填 |

## API

### 认证

| 端点 | 方法 | 用途 | 请求体 |
| ------ | ------ | ------ | -------- |
| `/register` | POST | 注册 | `{"username":"test","password":"test123456"}` |
| `/login` | POST | 登录 | 同上 |
| `/logout` | POST | 退出登录 | 无 |
| `/uuid` | GET | 获取查看 UUID 和上报令牌，需登录 | 无，返回 `data.uuid` 和 `data.report_token` |

### 心率数据

| 端点 | 方法 | 用途 |
| ------ | ------ | ------ |
| `/receive_data` | POST | 登录用户上报 |
| `/uuid/{report_token}/receive_data` | POST | 使用私密令牌上报，不能填公开查看 UUID |
| `/latest-heart-rate` | GET | 查询最新心率，需登录 |
| `/uuid/{uuid}/latest-heart-rate` | GET | 使用公开 UUID 查询最新心率 |

上报格式：

```json
{"data":{"heart_rate":72},"measured_at":1711700000000}
```

示例时间戳需要换成上报时的当前 Unix 毫秒时间。接口接受过去 10 分钟到未来 5 分钟的数据，心率范围为 1-250 BPM。

登录上报保留 10 分钟记录，令牌上报保留 30 分钟记录；每次上报都会清理窗口外的数据。查询接口只返回最新一条。

**从旧版本升级时先备份数据库。** 启动时会为已有账号生成独立上报令牌，公开查看 UUID 不变。升级后重新登录，把设备上的旧上报地址换成新地址；公开 UUID 不再接受上报。

### 页面

| 端点 | 方法 | 用途 |
| ------ | ------ | ------ |
| `/` | GET | 首页 |
| `/health` | GET | 登录注册页 |
| `/uuid/widget/view/{uuid}` | GET | 心率展示页，可嵌入直播软件 |

## 验证与构建

```sh
go test ./...
go test -race ./...
go vet ./...
node --test tests/widgets.test.cjs
go build .
```

Go 测试使用临时 SQLite 和内存 Redis，不需要外部 Redis，也不访问现有数据库。页面脚本测试需要 Node.js 18 或更高版本。

## 常见问题

- **登录后还是未登录**：检查是否使用 HTTPS，浏览器不会通过普通 HTTP 发送带 `Secure` 的认证 Cookie。
- **数据库打不开或提示只读**：检查 `DB_DSN`、挂载目录和已有文件权限，容器用户是 `10001:10001`。
- **旧上报地址返回 404**：重新登录获取私密上报令牌，公开查看 UUID 不能再用来上报。
- **展示页面一直是 `--`**：先确认设备有成功上报，再检查时间戳和 Redis 连接。没有数据时查询接口返回 404。

其他问题可以提 [Issue](https://github.com/xihan123/heart-rate-server/issues)，附上部署方式和报错信息。不要贴 Cookie 密钥或私密上报地址。

## 赞助

服务器由慈云数据赞助。

[![慈云数据](https://img.shields.io/badge/Powered_by-慈云数据-blue?style=for-the-badge&logo=aws)](https://www.zovps.com/)

![慈云数据](static/sponsors/ciyun.png)

## 许可证

本项目基于 [MIT License](LICENSE) 发布。
