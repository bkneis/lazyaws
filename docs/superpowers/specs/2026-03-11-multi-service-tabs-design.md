# lazyaws — Multi-Service Expansion & Tabbed Detail Pane

**Date:** 2026-03-11
**Status:** Approved

---

## Overview

Extend lazyaws to support 10 AWS services commonly used in SAM applications, and replace the static JSON detail pane with a tabbed detail view inspired by lazydocker. The tool remains read-only.

**Services:** S3, Lambda, SNS, SQS, CloudFormation, IAM Roles, Secrets Manager, API Gateway, Route53, ACM

---

## Architecture

### Provider Interface Extension

Add `Tabs() []TabDef` to the existing `Provider` interface. Each tab has a label and a lazy fetch function invoked on first selection.

`Item` gains a `Meta map[string]string` field for provider-specific context (e.g. API type, zone ID). Tab fetch functions receive the full `Item` and can read `Meta` to branch on type.

```go
type Item struct {
    ID   string
    Name string
    Meta map[string]string // provider-specific context, e.g. {"type": "REST"}
}

type TabDef struct {
    Label   string
    Fetch   func(ctx context.Context, item Item) (string, error)
}

type Provider interface {
    Name()      string
    ListItems(ctx context.Context) ([]Item, error)
    GetDetail(ctx context.Context, item Item) (string, error) // kept for compat
    Tabs()      []TabDef
}
```

Each provider file in `internal/aws/` defines its own tabs inline. The UI layer reads `Tabs()` once on provider selection and renders the tab bar in pane 3.

### Tab Behaviour

- `[` / `]` cycle tabs globally (lazydocker convention) — no need to focus pane 3
- Switching tabs triggers a lazy fetch if data not yet loaded for that tab
- Loading state: pane 3 body shows `... fetching`
- Tab state resets when a new item is selected in pane 2
- Sensitive values (secrets, passwords) masked as `••••••••••••••••` by default

### Async Loading Model

Tab fetches follow the same goroutine + `app.QueueUpdateDraw` pattern already used for `ListItems`. Each tab tracks its own load state: `idle | loading | loaded | error`.

---

## UI Layout

```
┌─────────────┬──────────────────────┬──────────────────────────────────┐
│ Resources   │ Items                │ [Overview]  Config  Tags         │
├─────────────┼──────────────────────┼──────────────────────────────────┤
│ > S3        │ > my-bucket-prod     │ Name:        my-bucket-prod      │
│   Lambda    │   my-bucket-staging  │ Region:      us-east-1           │
│   SNS       │   assets-cdn         │ Versioning:  Enabled             │
│   SQS       │                      │ Public:      All access blocked  │
│   CF        │                      │ Encryption:  SSE-S3              │
│   IAM       │                      │ Created:     2024-03-01          │
│   Secrets   │                      │                                  │
│   API GW    │                      │                                  │
│   Route53   │                      │                                  │
│   ACM       │                      │                                  │
└─────────────┴──────────────────────┴──────────────────────────────────┘
 Tab/S-Tab: panel    j/k: navigate    [/]: tab    r: refresh    q: quit
```

Tab bar format: active tab in `[brackets]`, inactive tabs plain, space-separated. Rendered as the first line of pane 3.

---

## Per-Service Tab Definitions

### S3 (3 tabs)

**Overview** — `GetBucketLocation`, `GetBucketVersioning`, `GetPublicAccessBlock`, `GetBucketEncryption`
```
  Region:       us-east-1
  Versioning:   Enabled
  Public:       All access blocked
  Encryption:   SSE-S3
  Created:      2024-03-01
```

**Objects** — `ListObjectsV2` (max 50)
```
  Key                              Size      Last Modified
  ────────────────────────────────────────────────────────
  images/hero.png                  2.3 MB    2024-11-01
  images/logo.svg                  14 KB     2024-09-12
  ...
  (showing 50 of 1,243 objects)
```

**Policy** — `GetBucketPolicy`
```
  { raw JSON policy }
```

---

### Lambda (3 tabs)

**Overview** — `GetFunction`
```
  Runtime:      python3.12
  Memory:       512 MB
  Timeout:      30s
  Handler:      app.handler
  Code Size:    4.2 MB
  Last Mod:     2024-11-03 14:22
  Role:         arn:aws:iam::123456789:role/my-fn-role
  Description:  Processes order events from SQS
```

**Env** — `GetFunctionConfiguration` (Environment.Variables)
```
  DB_HOST     prod-db.cluster-abc.us-east-1.rds.amazonaws.com
  DB_PORT     5432
  LOG_LEVEL   INFO
  QUEUE_URL   https://sqs.us-east-1.amazonaws.com/123/orders
```

**Triggers** — `ListEventSourceMappings`
```
  Type          Source ARN                                    State
  ──────────────────────────────────────────────────────────────────
  SQS           arn:aws:sqs:us-east-1:123456789:order-queue  Enabled
  EventBridge   arn:aws:events:us-east-1:123:rule/nightly    Enabled
```

---

### SNS (2 tabs)

**Overview** — `GetTopicAttributes`
```
  ARN:           arn:aws:sns:us-east-1:123456789:order-events
  Type:          Standard
  Confirmed:     3
  Pending:       1
  Deleted:       0
  KMS Key:       (none)
```

**Subscriptions** — `ListSubscriptionsByTopic`
```
  Protocol  Endpoint                                          Status
  ──────────────────────────────────────────────────────────────────
  sqs       arn:aws:sqs:us-east-1:123456789:order-queue      Confirmed
  email     ops@example.com                                   Confirmed
  lambda    arn:aws:lambda:us-east-1:123:fn:notify            Confirmed
  sqs       arn:aws:sqs:us-east-1:123456789:order-dlq         Pending
```

---

### SQS (3 tabs)

**Overview** — `GetQueueAttributes` (ApproximateNumberOfMessages etc.)
```
  Type:       Standard
  Available:  42 messages
  In-flight:  3 messages
  Delayed:    0 messages
  ARN:        arn:aws:sqs:us-east-1:123456789:order-queue
```

**Config** — `GetQueueAttributes` (all attributes)
```
  Visibility Timeout:    30s
  Message Retention:     4 days
  Max Message Size:      256 KB
  Delivery Delay:        0s
  Receive Wait Time:     0s
  Encryption:            SSE-SQS
```

**DLQ** — `GetQueueAttributes` (RedrivePolicy)
```
  DLQ ARN:           arn:aws:sqs:us-east-1:123456789:order-dlq
  Max Receives:      3
  Messages in DLQ:   7
```
If no DLQ configured: show `(no dead-letter queue configured)`

---

### CloudFormation (4 tabs)

**Overview** — `DescribeStacks`
```
  Name:         my-app-prod
  Status:       UPDATE_COMPLETE
  Created:      2023-08-14 09:12
  Last Updated: 2024-11-03 14:55
  Description:  Main application stack — API + DB + Auth
```

**Resources** — `ListStackResources`
```
  Logical ID              Type                          Status
  ──────────────────────────────────────────────────────────────
  OrdersTable             AWS::DynamoDB::Table          CREATE_COMPLETE
  OrdersFunction          AWS::Lambda::Function         UPDATE_COMPLETE
  OrdersApi               AWS::ApiGateway::RestApi      UPDATE_COMPLETE
  ExecutionRole           AWS::IAM::Role                CREATE_COMPLETE
```

**Outputs** — `DescribeStacks` (Outputs field)
```
  ApiUrl          https://abc123.execute-api.us-east-1.amazonaws.com/prod
  UserPoolId      us-east-1_AbCdEfGhI
  TableName       my-app-orders-prod
  QueueUrl        https://sqs.us-east-1.amazonaws.com/123/order-queue
```

**Parameters** — `DescribeStacks` (Parameters field)
```
  Env             prod
  AppName         my-app
  InstanceType    t3.micro
  LogRetention    30
```

---

### IAM Roles (3 tabs)

Scope: roles only (most relevant for SAM — Lambda execution roles, trust policies).
`ListItems` calls `ListRoles`.

**Overview** — `GetRole`
```
  Name:         order-processor-role
  ARN:          arn:aws:iam::123456789:role/order-processor-role
  Created:      2023-05-10
  Max Session:  1h
  Description:  Execution role for order processor Lambda
```

**Policies** — `ListAttachedRolePolicies` (managed) + `ListRolePolicies` (inline names only — no `GetRolePolicy` call; contents not shown).
```
  Type      Name
  ──────────────────────────────────────────────────────
  Managed   AWSLambdaBasicExecutionRole
  Managed   AmazonDynamoDBFullAccess
  Inline    AllowSQSRead
  Inline    AllowSecretsRead
```

**Trust** — `GetRole` (AssumeRolePolicyDocument)
```
  Principal:    lambda.amazonaws.com
  Action:       sts:AssumeRole
  Condition:    (none)
```

---

### Secrets Manager (3 tabs)

**Overview** — `DescribeSecret`
```
  ARN:           arn:aws:secretsmanager:us-east-1:123:secret/prod/db-AbCd
  Rotation:      Enabled (every 30 days)
  Last Rotated:  2024-10-15
  Last Accessed: 2024-11-03
  Last Changed:  2024-10-15
  KMS Key:       aws/secretsmanager
```

**Value** — `GetSecretValue`
If secret string is valid JSON: render as key-value table. Mask the **value** (not the key) as `••••••••••••••••` when the **key name** contains `password`, `secret`, `token`, or `key` (case-insensitive substring match). Nested JSON objects are not recursively expanded — render as `[object]`.
If plain string (not JSON): render raw without masking.

```
  db_host       prod-db.cluster-abc.us-east-1.rds.amazonaws.com
  db_port       5432
  db_name       orders
  db_user       app_user
  db_password   ••••••••••••••••
```

**Versions** — `ListSecretVersionIds`
```
  Version ID                            Staging Labels    Created
  ──────────────────────────────────────────────────────────────────
  a1b2c3d4-e5f6-7890-abcd-ef1234567890  AWSCURRENT        2024-10-15
  b2c3d4e5-f6a7-8901-bcde-f12345678901  AWSPREVIOUS       2024-09-15
```

---

### API Gateway (3 tabs)

`ListItems` calls `GetApis` (apigatewayv2, covers HTTP and WebSocket) and `GetRestApis` (apigateway, REST). Each `Item` stores `Meta["type"]` = `"HTTP"`, `"WEBSOCKET"`, or `"REST"` so tab fetch functions can branch correctly.

WebSocket APIs have the same tab structure as HTTP; route keys use WebSocket format (e.g. `$connect`, `$disconnect`, `$default`).

**Overview** — `GetApi` (apigatewayv2) if type=HTTP/WEBSOCKET, else `GetRestApi` (apigateway)
```
  API ID:    abc1234def
  Type:      HTTP API
  Endpoint:  https://abc1234def.execute-api.us-east-1.amazonaws.com
  Protocol:  HTTP
  Created:   2023-08-14
```

**Routes** — `GetRoutes` (apigatewayv2) if HTTP/WEBSOCKET; `GetResources` (apigateway, first page) if REST.
For REST, each resource's methods are shown as separate rows; integration type shown where available.
```
  Method  Route                    Integration
  ──────────────────────────────────────────────────
  GET     /users                   Lambda (list-users)
  POST    /users                   Lambda (create-user)
  GET     /users/{id}              Lambda (get-user)
  DELETE  /users/{id}              Lambda (delete-user)
  ANY     /health                  Lambda (health-check)
```

**Stages** — `GetStages` (apigatewayv2) if HTTP/WEBSOCKET; `GetStages` (apigateway) if REST.
```
  Stage     Deployment ID   Auto-Deploy   Last Deployed
  ──────────────────────────────────────────────────────
  prod      abc123          No            2024-11-03
  staging   def456          Yes           2024-11-05
```

---

### Route53 (2 tabs)

`ListItems` calls `ListHostedZones`.

**Overview** — `GetHostedZone`
```
  Zone:         example.com.
  Zone ID:      Z1234ABCDEFGHIJ
  Type:         Public
  Record Count: 12
  Comment:      Main hosted zone
```

**Records** — `ListResourceRecordSets`
```
  Name                       Type   TTL     Value
  ──────────────────────────────────────────────────────────────────
  example.com                A      -       ALIAS d1234.cloudfront.net
  www.example.com            CNAME  300     example.com
  api.example.com            A      -       ALIAS abc.execute-api.aws.com
  _dmarc.example.com         TXT    3600    "v=DMARC1; p=none; rua=..."
  mail.example.com           MX     3600    10 inbound-smtp.us-east-1...
```

---

### ACM (3 tabs)

`ListItems` calls `ListCertificates`.

**Overview** — `DescribeCertificate`
```
  Domain:     example.com
  Status:     Issued
  Type:       Amazon Issued
  Expires:    2025-11-03  (357 days)
  Issuer:     Amazon
  Key:        RSA-2048
  In Use By:  CloudFront (d1234.cloudfront.net)
              API Gateway (abc123.execute-api...)
```

**Domains** — `DescribeCertificate` (SubjectAlternativeNames)
```
  example.com          (primary)
  www.example.com
  api.example.com
  staging.example.com
```

**Validation** — `DescribeCertificate` (DomainValidationOptions)
```
  Method:  DNS

  Domain               Record Name                    Record Value
  ──────────────────────────────────────────────────────────────────────
  example.com          _abc123.example.com            _def456.acm-valid...
  www.example.com      _abc123.www.example.com        _def456.acm-valid...
```

---

## Keybindings

| Key         | Action                              |
|-------------|-------------------------------------|
| `Tab`       | Cycle focus forward across panes    |
| `Shift+Tab` | Cycle focus backward across panes   |
| `j` / `↓`   | Move down in pane 1 or pane 2       |
| `k` / `↑`   | Move up in pane 1 or pane 2         |
| `[`         | Previous tab in pane 3 (global)     |
| `]`         | Next tab in pane 3 (global)         |
| `r`         | Refresh current resource list       |
| `q`         | Quit                                |

---

## New AWS SDK Dependencies

| Service          | Go SDK Package                                    |
|------------------|---------------------------------------------------|
| SNS              | `github.com/aws/aws-sdk-go-v2/service/sns`        |
| SQS              | `github.com/aws/aws-sdk-go-v2/service/sqs`        |
| CloudFormation   | `github.com/aws/aws-sdk-go-v2/service/cloudformation` |
| IAM              | `github.com/aws/aws-sdk-go-v2/service/iam`        |
| Secrets Manager  | `github.com/aws/aws-sdk-go-v2/service/secretsmanager` |
| API Gateway v2   | `github.com/aws/aws-sdk-go-v2/service/apigatewayv2` |
| API Gateway v1   | `github.com/aws/aws-sdk-go-v2/service/apigateway` |
| Route53          | `github.com/aws/aws-sdk-go-v2/service/route53`    |
| ACM              | `github.com/aws/aws-sdk-go-v2/service/acm`        |

---

## Out of Scope

- Write operations / actions of any kind
- Region switching within the UI (loaded from env/AWS config)
- Pagination beyond first page for large lists (except Objects tab, capped at 50)
- Secret value unmasking toggle (future feature)
- IAM Users, Groups, Policies (Roles only for SAM relevance)
