# lazyaws

A terminal UI for browsing AWS resources — inspired by [lazygit](https://github.com/jesseduffield/lazygit) and [lazydocker](https://github.com/jesseduffield/lazydocker).

Built for developers who live in the terminal and need fast, keyboard-driven access to AWS state without memorising CLI flags or context-switching to the console. Works equally well as a **local development companion against [LocalStack](https://www.localstack.cloud/)** and as a lightweight **read-only monitoring tool** against real AWS environments.

## Why

The AWS CLI is powerful but slow for exploratory workflows — finding a Lambda's env vars, checking a CloudFormation stack's outputs, or inspecting an SQS dead-letter queue involves multiple commands with long argument lists. `lazyaws` puts all of that in a three-panel TUI you can navigate in seconds.

It pairs naturally with LocalStack-based development: run `lazyaws -local` alongside your `docker compose up` to inspect the state of your emulated AWS environment in real time, the same way you'd use `lazydocker` to inspect containers.

## Usage

```bash
# Against your default AWS profile / region
go run .

# Against LocalStack (http://localhost:4566)
go run . -local

# Build a binary
go build -o lazyaws .
./lazyaws
```

AWS credentials are loaded from the standard chain (`AWS_*` environment variables, `~/.aws/credentials`, IAM instance role, etc.).

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
| IAM | Overview, Policies, Trust policy |
| Secrets Manager | Overview, Value, Versions |
| API Gateway (v1 + v2) | Overview, Routes/Resources, Stages |
| Route 53 | Overview, Records |
| ACM | Overview, Domains, Validation |

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

   Define a narrow interface over the SDK client (only the methods you actually call), implement `Provider`, and expose two constructors:
   - `New<Service>Provider(cfg aws.Config, local bool) *<Service>Provider` — for production use
   - `New<Service>ProviderWithClient(client <Service>API) *<Service>Provider` — for tests

   Use `KV()` for key-value detail output and `Table()` for tabular output (both in `format.go`).

2. **Register in `main.go`**

   Append `awspkg.New<Service>Provider(cfg, *local)` to the `providers` slice.

3. **Add the SDK dependency if needed**

   ```bash
   go get github.com/aws/aws-sdk-go-v2/service/<service>
   ```

4. **Write tests**

   Implement the narrow interface in your test file and use table-driven tests. See `s3_test.go` or `lambda_test.go` as reference.

### Example prompt for Claude

If you're using Claude Code, the following prompt will implement a new provider end-to-end:

```
Add a lazyaws provider for <ServiceName>.

List items using <ListAPI>, with ID=<id field> and Name=<name field>.

Tabs:
- Overview: show <field1>, <field2>, ... using KV()
- <TabName>: show <what> using Table() with columns <col1>, <col2>, ...

Follow the existing pattern in internal/aws/s3.go exactly:
- define a narrow <ServiceName>API interface
- implement Provider with New<ServiceName>Provider(cfg, local) and New<ServiceName>ProviderWithClient(client)
- register in main.go
- write table-driven tests in internal/aws/<service>_test.go
```
