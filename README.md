# stalwart-dns

从文件批量把 DNS 记录写入 DNS 平台（支持 dry-run、失败自动回滚）。

## 功能

- 从 `config.json`（项目配置）读取记录并调用平台 API 创建解析（默认 DNSPod，可选 Cloudflare）
- `--dry-run` 仅打印计划，不触发任何 API 调用
- 事务语义：任意一条创建失败，会撤销本次已创建的记录（逆序删除）
- 内置 `dns.txt` 转换器：TSV → `config.json`，并可选输出 BIND zone 文件（更便于人工阅读）
 - 可选 `--upsert`：记录已存在时，更新为当前配置（谨慎使用）

## 配置文件（config.json）

`config.json` 格式如下：

```json
{
  "records": [
    { "type": "MX", "name": "example.com.", "contents": "10 mail.example.com." }
  ]
}
```

- `name`：建议使用 FQDN（末尾 `.` 可有可无）
- `contents`：不同记录类型的取值规则：
  - `MX`：`"<priority> <exchange>"`
  - `SRV`：`"<priority> <weight> <port> <target>"`
  - `TLSA`：`"<usage> <selector> <matching-type> <data>"`
  - `CNAME` / `TXT`：记录值本身

## 从 dns.txt 转换（推荐）

项目内置 `convert` 子命令，可把 `dns.txt`（TSV）转成 `config.json`，同时可选输出 BIND 兼容的 zone 文件：

```bash
go run ./cmd/stalwart-dns convert --input dns.txt --output config.json --zone zone.txt
```

`dns.txt` 输入格式（tab 分隔，表头可选）：

- 必需列：`Type`、`Name`、`Contents`
- 可选列：`TTL`、`Remark`
- TXT 内容按原样处理（可带或不带引号）

## 运行

### 0) 选择平台

默认使用 DNSPod；也可使用 Cloudflare：

- DNSPod：`--provider dnspod`（默认）
- Cloudflare：`--provider cloudflare`

### 1) 准备凭据

#### DNSPod

环境变量（推荐）：

- `DNSPOD_SECRET_ID`
- `DNSPOD_SECRET_KEY`

或使用参数：

- `--secret-id`
- `--secret-key`

#### Cloudflare

环境变量（推荐）：

- `CLOUDFLARE_API_TOKEN`

或使用参数：

- `--cf-token`
- `--cf-zone-id`（可选；不提供则按域名自动查询）

### 2) 初始化 config.json（可选）

如果你只有 `dns.txt`，可以直接生成 `config.json`（默认不覆盖已有文件；需要覆盖加 `--force`）：

```bash
go run ./cmd/stalwart-dns --init --dns-txt dns.txt --config config.json
```

### 3) Dry-run（不写入平台）

```bash
go run ./cmd/stalwart-dns --config config.json --dry-run
```

### 4) 写入平台

DNSPod：

```bash
go run ./cmd/stalwart-dns --config config.json
```

Cloudflare：

```bash
go run ./cmd/stalwart-dns --provider cloudflare --config config.json
```

可附加参数示例：

```bash
go run ./cmd/stalwart-dns --config config.json --record-line 默认 --region ap-guangzhou --upsert
```

## 注意事项

- `SecretId` 通常形如 `AKID...`（不是纯数字）。鉴权失败会导致一条记录都无法创建。
- 默认对“已存在的记录”会跳过（exists skip）。需要更新请加 `--upsert`。
- 平台 API 不一定支持所有记录类型。如果配置里包含不支持的类型（例如 DNSPod 的 TLSA），默认会在调用 API 前直接报错；需要忽略这些类型可加 `--skip-unsupported`。

