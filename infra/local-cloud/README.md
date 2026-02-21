# FlowForge Cloud-Dev Stack

This stack provides a local cloud-capable dependency set for FlowForge:

- PostgreSQL 16
- Redis 7
- NATS (JetStream enabled)
- MinIO (S3-compatible object storage)

## Quick Start

From repo root:

```bash
./scripts/cloud_dev_stack.sh up
```

Check status:

```bash
./scripts/cloud_dev_stack.sh status
```

Tail logs:

```bash
./scripts/cloud_dev_stack.sh logs
```

Stop:

```bash
./scripts/cloud_dev_stack.sh down
```

Reset volumes:

```bash
./scripts/cloud_dev_stack.sh reset
```

## Ports

- Postgres: `127.0.0.1:15432`
- Redis: `127.0.0.1:16379`
- NATS client: `127.0.0.1:14222`
- NATS monitoring: `http://127.0.0.1:18222`
- MinIO API: `http://127.0.0.1:19000`
- MinIO console: `http://127.0.0.1:19001`

## Credentials / Bucket

Defaults are in `infra/local-cloud/.env.example`.
Create a local override file:

```bash
cp infra/local-cloud/.env.example infra/local-cloud/.env
```

The bootstrap creates the bucket:

- `${FF_MINIO_BUCKET}` (default: `flowforge-dev-logs`)
