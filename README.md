<p align="center">
  <img src="assets/logo.png" alt="lazyaws" width="600"/>
</p>

<p align="center">
  A terminal UI for browsing AWS resources — inspired by
  <a href="https://github.com/jesseduffield/lazygit">lazygit</a> and
  <a href="https://github.com/jesseduffield/lazydocker">lazydocker</a>.
</p>

<p align="center">
  <a href="https://github.com/bkneis/lazyaws/releases"><img src="https://img.shields.io/github/v/release/bkneis/lazyaws?color=FF9900&label=release" alt="release"></a>
  <a href="https://pkg.go.dev/github.com/bkneis/lazyaws"><img src="https://img.shields.io/badge/go-reference-FF9900" alt="go reference"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/bkneis/lazyaws?color=FF9900" alt="MIT license"></a>
  <a href="https://github.com/bkneis/lazyaws/actions"><img src="https://img.shields.io/github/actions/workflow/status/bkneis/lazyaws/test.yml?label=tests" alt="tests"></a>
</p>

---

<p align="center">
  <img src="demo/demo.gif" alt="lazyaws demo" width="900"/>
</p>

---

## Why

The AWS CLI is powerful but slow for exploratory workflows. Finding a Lambda's env vars, checking a CloudFormation stack's outputs, or inspecting an SQS dead-letter queue means multiple commands with long argument lists.

`lazyaws` puts all of that in a three-panel TUI you can navigate in seconds — no flags to remember, no context-switching to the browser console.

It pairs naturally with **LocalStack-based development**: run `lazyaws -local` alongside `docker compose up` to inspect your emulated AWS environment in real time, the same way you'd use `lazydocker` to inspect containers. It also works as a lightweight **read-only monitoring tool** against real AWS environments.

## Install

```bash
go install github.com/bkneis/lazyaws@latest
```

Or download a pre-built binary from the [releases page](https://github.com/bkneis/lazyaws/releases).

## Usage

```bash
# Against your default AWS profile / region
lazyaws

# Against LocalStack (http://localhost:4566)
lazyaws -local
```

AWS credentials are loaded from the standard chain (`AWS_*` environment variables, `~/.aws/credentials`, IAM instance role, etc.).

## Try it with LocalStack

Spin up a fully-seeded local AWS environment in two commands:

```bash
docker compose up -d
./scripts/seed.sh
lazyaws -local
```

This seeds S3 buckets, Lambda functions, SQS queues, SNS topics, DynamoDB tables, Secrets Manager secrets, a CloudFormation stack, CloudWatch log groups, Kinesis streams, an API Gateway HTTP API, and EventBridge rules — enough to explore every provider.

## Keybindings

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle focus between panels |
| `j` / `k` or `↓` / `↑` | Navigate lists |
| `[` / `]` | Previous / next detail tab |
| `r` | Refresh current resource list |
| `q` | Quit |

## Supported Services

| Service | Detail tabs |
|---------|-------------|
| S3 | Overview, Objects, Policy |
| Lambda | Overview, Env vars, Triggers |
| SNS | Overview, Subscriptions |
| SQS | Overview, Config, DLQ |
| CloudFormation | Overview, Resources, Outputs, Parameters |
| IAM Roles | Overview, Policies, Trust policy |
| IAM Policies | Overview, Document |
| Secrets Manager | Overview, Value, Versions |
| API Gateway (v1 + v2) | Overview, Routes/Resources, Stages |
| Route 53 | Overview, Records |
| ACM | Overview, Domains, Validation |
| DynamoDB | Overview, Items, Indexes |
| Kinesis | Overview, Shards |
| KMS | Overview, Aliases |
| Step Functions | Overview, Executions |
| CloudWatch | Overview, Metrics |
| CloudWatch Logs | Overview, Log streams |
| EventBridge | Overview, Rules |
| EC2 Instances | Overview, Tags |
| EC2 VPCs | Overview, Subnets |
| EC2 Security Groups | Overview, Rules |
| EC2 Volumes | Overview, Attachments |
| EC2 AMIs | Overview, Block devices |
| Elastic Load Balancers | Overview, Listeners, Target groups |
| Auto Scaling Groups | Overview, Instances |

## Contributing a New Service

Each AWS service is a self-contained file in `internal/aws/` that implements the `Provider` interface:

```go
type Provider interface {
    Name() string
    ListItems(ctx context.Context) ([]Item, error)
    GetDetail(ctx context.Context, item Item) (string, error)
    Tabs() []TabDef
}
```

### Steps

1. **Create `internal/aws/<service>.go`**

   Define a narrow interface over the SDK client, implement `Provider`, and expose two constructors:
   - `New<Service>Provider(cfg aws.Config, local bool) *<Service>Provider`
   - `New<Service>ProviderWithClient(client <Service>API) *<Service>Provider`

   Use `KV()` for key-value output and `Table()` for tabular output (both in `format.go`).

2. **Register in `main.go`**

   Append `awspkg.New<Service>Provider(cfg, *local)` to the `providers` slice.

3. **Add the SDK dependency if needed**

   ```bash
   go get github.com/aws/aws-sdk-go-v2/service/<service>
   ```

4. **Write tests**

   Implement the narrow interface directly in the test file and use table-driven tests. See `s3_test.go` or `lambda_test.go` as reference.

### Prompt for Claude Code

```
Add a lazyaws provider for <ServiceName>.

List items using <ListAPI>, with ID=<id field> and Name=<name field>.

Tabs:
- Overview: show <field1>, <field2>, ... using KV()
- <TabName>: show <what> using Table() with columns <col1>, <col2>, ...

Follow the existing pattern in internal/aws/s3.go exactly.
```

## License

[MIT](LICENSE) © bkneis
