## Try it with LocalStack

Spin up a fully-seeded local AWS environment in two commands:

```bash
docker compose up -d
./scripts/seed.sh
lazyaws -local
```

This seeds S3 buckets, Lambda functions, SQS queues, SNS topics, DynamoDB tables, Secrets Manager secrets, a CloudFormation stack, CloudWatch log groups, Kinesis streams, an API Gateway HTTP API, and EventBridge rules — enough to explore every provider.

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