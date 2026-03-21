<p align="center">
  <img src="assets/logo.png" alt="lazyaws" width="600"/>
</p>

<p align="center">
  A terminal UI for managing AWS resources — inspired by
  <a href="https://github.com/jesseduffield/lazydocker">lazydocker</a>, 
  <a href="https://github.com/derailed/k9s">k9s</a> and 
  <a href="https://github.com/jesseduffield/lazygit">lazygit</a>.
</p>

<p align="center">
  <a href="https://github.com/bkneis/lazyaws/releases"><img src="https://img.shields.io/github/v/release/bkneis/lazyaws?color=FF9900&label=release" alt="release"></a>
  <a href="https://pkg.go.dev/github.com/bkneis/lazyaws"><img src="https://img.shields.io/badge/go-reference-FF9900" alt="go reference"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/bkneis/lazyaws?color=FF9900" alt="MIT license"></a>
  <a href="https://github.com/bkneis/lazyaws/actions"><img src="https://img.shields.io/github/actions/workflow/status/bkneis/lazyaws/test.yml?label=tests" alt="tests"></a>
  <a href="https://hub.docker.com/r/bkneis/lazyaws"><img src="https://img.shields.io/docker/pulls/bkneis/lazyaws" alt="Docker Pulls"></a>
</p>

---

<p align="center">
  <img src="demo2.gif" alt="lazyaws demo" width="900"/>
</p>

---

The aim of this project is to make it easier to navigate, observe and manage your infrastucture, whether in the wild or locally during development.

## Why

The AWS CLI is powerful but slow for exploratory workflows. Finding a Lambda's env vars, checking a CloudFormation stack's outputs, or inspecting an SQS queue means multiple commands with long argument lists.

`lazyaws` puts all of that in a three-panel TUI you can navigate in seconds — no flags to remember, no context-switching to the browser console and waiting for static assets to load.

It pairs naturally with **LocalStack-based development**: run `lazyaws -local` or `lazyaws -entrypoint-url=<aws control plane>` to inspect your emulated AWS environment in real time, the same way you'd use `lazydocker` to inspect containers.

My typicaly workflow while using AI tools like claude include providing verification loops such as restarting a localstack container and re deploying cloudformation templates to fix infrastructure issues. With lazyaws, you can easily observe this in real time while the agent is working. Or if an integration test fails, and leaves the resources deployed, it can be inspected quickly and easily without leaving the terminal.

## Install

**macOS / Linux**
```bash
brew install bkneis/lazyaws/lazyaws
```

**Linux packages**
```bash
# Debian/Ubuntu
sudo dpkg -i lazyaws_*.deb     # download .deb from GitHub Releases

# RPM-based
sudo rpm -i lazyaws_*.rpm      # download .rpm from GitHub Releases
```

**Windows**
```bash
scoop bucket add lazyaws https://github.com/bkneis/scoop-lazyaws
scoop install lazyaws
```

**Docker (all platforms)**
```bash
docker run --rm -it \
  -e AWS_ACCESS_KEY_ID \
  -e AWS_SECRET_ACCESS_KEY \
  -e AWS_DEFAULT_REGION \
  bkneis/lazyaws
```

For LocalStack, pass `--network host -e AWS_ENDPOINT_URL=http://localhost:4566`.

**go install / binary**
```bash
go install github.com/bkneis/lazyaws@latest
```

Or download a pre-built binary (Windows/macOS/Linux, amd64/arm64) from the [releases page](https://github.com/bkneis/lazyaws/releases).

## Usage

```bash
# Against your default AWS profile / region
lazyaws

# Against LocalStack (http://localhost:4566)
lazyaws -local
```

AWS credentials are loaded from the standard chain (`AWS_*` environment variables, `~/.aws/credentials`, IAM instance role, etc.).

## Cool Features

- Fast grep like search across your infrastucture using `/`
- Cloudwatch Logs Viewer
- S3 Explorer with ability to view text / json files and download items
- DynamoDB browser for easily inspecting JSON objects
- Actions menu (x) for interactive commands like uploading a file to s3, sending a message to SQS or SNS
- Cross resource linking, click underscored hyperlinks in resource lists to jump to that resouce
- Point it at any AWS control plane such as localstack using --entrypoint-url
- Completely clickable TUI, no need to learn keyboard shortcuts if you don't want to
- Connect to EC2, ECS and RDS instances right from your list (requires either AWS SSM or host needs to be addressable form the network)
- Single binary ~28mb that works across window, linux and mac 32/64bit
- Doesn't require aws cli to be installed or use any porcelin command processing, entirely built using go aws sdk and uses your local authentication configured

---

If you find this useful, consider giving it a ⭐ — it helps others discover the project.

---

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
| SSM | Overview, Value, History |
| ECS | Overivew | Services | Tasks |

## Contributing

I'd welcome any contrubtions from the community, if anyone wants to suggest/implement new features or integrate AWS services then please read CONTRUBTING.md and submit a PR :)

## License

[MIT](LICENSE) © bkneis
