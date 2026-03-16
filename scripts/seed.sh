#!/usr/bin/env bash
# Seed LocalStack with realistic demo data for lazyaws.
# Usage: ./scripts/seed.sh
# Requires: aws CLI configured to point at LocalStack, or run after `docker compose up`.

set -euo pipefail

AWS="aws --endpoint-url=http://localhost:4566 --region us-east-1 \
  --no-cli-pager \
  --output json"

echo "⏳ Waiting for LocalStack to be ready..."
until curl -sf http://localhost:4566/_localstack/health | grep -q '"s3": "available"'; do
  sleep 1
done
echo "✅ LocalStack ready"

# ── S3 ────────────────────────────────────────────────────────────────────────
echo "→ S3"
for bucket in my-app-assets my-data-lake my-backups; do
  $AWS s3api create-bucket --bucket "$bucket" >/dev/null
done

$AWS s3api put-bucket-versioning \
  --bucket my-app-assets \
  --versioning-configuration Status=Enabled >/dev/null

# Seed objects
echo '{"app":"lazyaws","version":"1.0.0"}' | $AWS s3 cp - s3://my-app-assets/config/app.json >/dev/null
echo 'user_id,name,email'                  | $AWS s3 cp - s3://my-data-lake/exports/users.csv >/dev/null
echo 'backup 2026-03-01'                   | $AWS s3 cp - s3://my-backups/db/2026-03-01.sql.gz >/dev/null
echo 'backup 2026-03-15'                   | $AWS s3 cp - s3://my-backups/db/2026-03-15.sql.gz >/dev/null

# ── Lambda ────────────────────────────────────────────────────────────────────
echo "→ Lambda"

# Minimal zip with a handler stub
TMP=$(mktemp -d)
for fn in api-handler data-processor auth-validator notification-sender; do
  cat > "$TMP/index.js" <<JS
exports.handler = async (event) => ({ statusCode: 200, body: JSON.stringify({ fn: "$fn" }) });
JS
  (cd "$TMP" && zip -q handler.zip index.js)
  $AWS lambda create-function \
    --function-name "$fn" \
    --runtime nodejs20.x \
    --role arn:aws:iam::000000000000:role/lambda-role \
    --handler index.handler \
    --zip-file fileb://"$TMP/handler.zip" \
    --environment "Variables={ENV=production,LOG_LEVEL=info}" \
    --timeout 30 \
    --memory-size 256 >/dev/null
done
rm -rf "$TMP"

# ── SQS ───────────────────────────────────────────────────────────────────────
echo "→ SQS"
$AWS sqs create-queue --queue-name orders-dlq >/dev/null
$AWS sqs create-queue --queue-name orders \
  --attributes '{"RedrivePolicy":"{\"deadLetterTargetArn\":\"arn:aws:sqs:us-east-1:000000000000:orders-dlq\",\"maxReceiveCount\":\"3\"}","VisibilityTimeout":"30"}' >/dev/null
$AWS sqs create-queue --queue-name notifications >/dev/null

# ── SNS ───────────────────────────────────────────────────────────────────────
echo "→ SNS"
ORDER_TOPIC=$($AWS sns create-topic --name order-events --query TopicArn --output text)
ALERT_TOPIC=$($AWS sns create-topic --name alerts      --query TopicArn --output text)

ORDERS_URL=$($AWS sqs get-queue-url --queue-name orders --query QueueUrl --output text)
ORDERS_ARN=$($AWS sqs get-queue-attributes \
  --queue-url "$ORDERS_URL" \
  --attribute-names QueueArn \
  --query Attributes.QueueArn --output text)

$AWS sns subscribe \
  --topic-arn "$ORDER_TOPIC" \
  --protocol sqs \
  --notification-endpoint "$ORDERS_ARN" >/dev/null

$AWS sns subscribe \
  --topic-arn "$ALERT_TOPIC" \
  --protocol email \
  --notification-endpoint ops@example.com >/dev/null

# ── DynamoDB ──────────────────────────────────────────────────────────────────
echo "→ DynamoDB"
$AWS dynamodb create-table \
  --table-name users \
  --attribute-definitions AttributeName=user_id,AttributeType=S \
  --key-schema AttributeName=user_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST >/dev/null

$AWS dynamodb create-table \
  --table-name sessions \
  --attribute-definitions AttributeName=session_id,AttributeType=S AttributeName=user_id,AttributeType=S \
  --key-schema AttributeName=session_id,KeyType=HASH \
  --global-secondary-indexes '[{"IndexName":"UserIndex","KeySchema":[{"AttributeName":"user_id","KeyType":"HASH"}],"Projection":{"ProjectionType":"ALL"}}]' \
  --billing-mode PAY_PER_REQUEST >/dev/null

$AWS dynamodb put-item --table-name users \
  --item '{"user_id":{"S":"u-001"},"name":{"S":"Alice"},"email":{"S":"alice@example.com"}}' >/dev/null
$AWS dynamodb put-item --table-name users \
  --item '{"user_id":{"S":"u-002"},"name":{"S":"Bob"},  "email":{"S":"bob@example.com"}}' >/dev/null

# ── Secrets Manager ───────────────────────────────────────────────────────────
echo "→ Secrets Manager"
$AWS secretsmanager create-secret \
  --name prod/db/password \
  --secret-string '{"username":"app","password":"s3cr3t!"}' >/dev/null
$AWS secretsmanager create-secret \
  --name prod/stripe/api-key \
  --secret-string 'sk_live_placeholder' >/dev/null
$AWS secretsmanager create-secret \
  --name prod/jwt/signing-key \
  --secret-string 'super-secret-jwt-key-256-bits' >/dev/null

# ── CloudWatch Logs ───────────────────────────────────────────────────────────
echo "→ CloudWatch Logs"
for fn in api-handler data-processor auth-validator notification-sender; do
  $AWS logs create-log-group --log-group-name "/aws/lambda/$fn" >/dev/null
  $AWS logs create-log-stream \
    --log-group-name "/aws/lambda/$fn" \
    --log-stream-name "2026/03/16/[\$LATEST]abc123" >/dev/null
done
$AWS logs create-log-group --log-group-name "/app/production" >/dev/null

# ── EventBridge ───────────────────────────────────────────────────────────────
echo "→ EventBridge"
$AWS events create-event-bus --name app-events >/dev/null
$AWS events put-rule \
  --name OrderCreatedRule \
  --event-bus-name app-events \
  --event-pattern '{"source":["app.orders"],"detail-type":["OrderCreated"]}' \
  --state ENABLED >/dev/null

# ── Kinesis ───────────────────────────────────────────────────────────────────
echo "→ Kinesis"
$AWS kinesis create-stream --stream-name clickstream --shard-count 2 >/dev/null
$AWS kinesis create-stream --stream-name audit-log   --shard-count 1 >/dev/null

# ── API Gateway (HTTP API v2) ─────────────────────────────────────────────────
echo "→ API Gateway"
API_ID=$($AWS apigatewayv2 create-api \
  --name "lazyaws-demo-api" \
  --protocol-type HTTP \
  --query ApiId --output text)

$AWS apigatewayv2 create-stage \
  --api-id "$API_ID" \
  --stage-name production \
  --auto-deploy >/dev/null

$AWS apigatewayv2 create-route \
  --api-id "$API_ID" \
  --route-key "GET /users" >/dev/null
$AWS apigatewayv2 create-route \
  --api-id "$API_ID" \
  --route-key "POST /orders" >/dev/null

# ── CloudFormation ────────────────────────────────────────────────────────────
echo "→ CloudFormation"
$AWS cloudformation create-stack \
  --stack-name app-infra \
  --template-body '{
    "AWSTemplateFormatVersion":"2010-09-09",
    "Description":"Demo app infrastructure",
    "Parameters":{
      "Env":{"Type":"String","Default":"production"}
    },
    "Resources":{
      "AppBucket":{
        "Type":"AWS::S3::Bucket",
        "Properties":{"BucketName":"cfn-app-bucket"}
      }
    },
    "Outputs":{
      "BucketName":{"Value":{"Ref":"AppBucket"},"Description":"App S3 bucket"}
    }
  }' \
  --parameters ParameterKey=Env,ParameterValue=production >/dev/null

echo ""
echo "✅ Seed complete. Run: go run . -local"
