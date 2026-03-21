#!/usr/bin/env bash
# Seed LocalStack with realistic demo data for lazyaws.
# Usage: ./scripts/seed.sh
# Requires: aws CLI and LocalStack running via docker compose.
# Safe to re-run — cleans up existing resources before recreating them.

set -euo pipefail

export AWS_DEFAULT_REGION=us-east-1

AWS="aws --endpoint-url=http://localhost:4567 --no-cli-pager --output json"

# echo "⏳ Waiting for LocalStack to be ready..."
# until curl -sf http://localhost:4567/_localstack/health | grep -q '"s3": "available"'; do
#   sleep 1
# done
# echo "✅ LocalStack ready"

# ── Teardown (idempotent — all errors suppressed) ─────────────────────────────
echo "🧹 Clearing previous state..."

# S3
for bucket in my-app-assets my-data-lake my-backups cfn-app-bucket; do
  $AWS s3 rb s3://"$bucket" --force 2>/dev/null || true
done

# Lambda
for fn in api-handler data-processor auth-validator notification-sender; do
  $AWS lambda delete-function --function-name "$fn" 2>/dev/null || true
done

# SQS
for q in orders orders-dlq notifications; do
  url=$($AWS sqs get-queue-url --queue-name "$q" --query QueueUrl --output text 2>/dev/null) || true
  [ -n "${url:-}" ] && $AWS sqs delete-queue --queue-url "$url" 2>/dev/null || true
done

# DynamoDB
for table in users sessions; do
  $AWS dynamodb delete-table --table-name "$table" 2>/dev/null || true
done

# Secrets Manager
for secret in prod/db/password prod/stripe/api-key prod/jwt/signing-key; do
  $AWS secretsmanager delete-secret --secret-id "$secret" \
    --force-delete-without-recovery 2>/dev/null || true
done

# CloudWatch Logs
for group in \
  /aws/lambda/api-handler /aws/lambda/data-processor \
  /aws/lambda/auth-validator /aws/lambda/notification-sender \
  /app/production; do
  $AWS logs delete-log-group --log-group-name "$group" 2>/dev/null || true
done

# EventBridge — teardown only, not seeded
$AWS events remove-targets --rule OrderCreatedRule \
  --event-bus-name app-events --ids "1" 2>/dev/null || true
$AWS events delete-rule --name OrderCreatedRule \
  --event-bus-name app-events 2>/dev/null || true
$AWS events delete-event-bus --name app-events 2>/dev/null || true

# Kinesis — teardown only, not seeded
for stream in clickstream audit-log; do
  $AWS kinesis delete-stream --stream-name "$stream" 2>/dev/null || true
done

# API Gateway v1 — create-rest-api is not idempotent
for id in $($AWS apigateway get-rest-apis \
    --query 'items[?name==`lazyaws-demo-api`].id' --output text 2>/dev/null || true); do
  $AWS apigateway delete-rest-api --rest-api-id "$id" 2>/dev/null || true
done

# CloudFormation
$AWS cloudformation delete-stack --stack-name app-infra 2>/dev/null || true

# IAM
for role in lambda-execution-role api-gateway-cloudwatch-role; do
  for arn in $($AWS iam list-attached-role-policies --role-name "$role" \
      --query 'AttachedPolicies[].PolicyArn' --output text 2>/dev/null || true); do
    $AWS iam detach-role-policy --role-name "$role" --policy-arn "$arn" 2>/dev/null || true
  done
  $AWS iam delete-role --role-name "$role" 2>/dev/null || true
done
POLICY_ARN=$($AWS iam list-policies --scope Local \
  --query "Policies[?PolicyName==\`lambda-s3-access\`].Arn" \
  --output text 2>/dev/null || true)
[ -n "${POLICY_ARN:-}" ] && $AWS iam delete-policy --policy-arn "$POLICY_ARN" 2>/dev/null || true

# Route53
for zone_id in $($AWS route53 list-hosted-zones \
    --query 'HostedZones[?Name==`example.com.`].Id' \
    --output text 2>/dev/null || true); do
  $AWS route53 change-resource-record-sets --hosted-zone-id "$zone_id" \
    --change-batch '{"Changes":[{"Action":"DELETE","ResourceRecordSet":
      {"Name":"api.example.com","Type":"A","TTL":300,
       "ResourceRecords":[{"Value":"1.2.3.4"}]}}]}' 2>/dev/null || true
  $AWS route53 delete-hosted-zone --id "$zone_id" 2>/dev/null || true
done

# ── S3 ────────────────────────────────────────────────────────────────────────
echo "→ S3"
for bucket in my-app-assets my-data-lake my-backups; do
  $AWS s3api create-bucket --bucket "$bucket" >/dev/null
done

$AWS s3api put-bucket-versioning \
  --bucket my-app-assets \
  --versioning-configuration Status=Enabled >/dev/null

echo '{"app":"lazyaws","version":"1.0.0"}' | $AWS s3 cp - s3://my-app-assets/config/app.json >/dev/null
echo 'user_id,name,email'                  | $AWS s3 cp - s3://my-data-lake/exports/users.csv >/dev/null
echo 'backup 2026-03-01'                   | $AWS s3 cp - s3://my-backups/db/2026-03-01.sql.gz >/dev/null
echo 'backup 2026-03-15'                   | $AWS s3 cp - s3://my-backups/db/2026-03-15.sql.gz >/dev/null

# ── Lambda ────────────────────────────────────────────────────────────────────
echo "→ Lambda"
TMP=$(mktemp -d)
for fn in api-handler data-processor auth-validator notification-sender; do
  cat > "$TMP/index.js" <<JS
exports.handler = async (event) => ({ statusCode: 200, body: JSON.stringify({ fn: "$fn" }) });
JS
  (cd "$TMP" && zip -q handler.zip index.js)
  $AWS lambda create-function \
    --function-name "$fn" \
    --runtime nodejs20.x \
    --role arn:aws:iam::000000000000:role/lambda-execution-role \
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

# ── SNS (create-topic is idempotent — returns existing ARN) ───────────────────
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
  --item '{"user_id":{"S":"u-002"},"name":{"S":"Bob"},"email":{"S":"bob@example.com"}}' >/dev/null

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

# ── CloudWatch Logs (JSON events for JSON viewer manual testing) ───────────────
echo "→ CloudWatch Logs (JSON events)"
NOW=$(date +%s%3N)
for fn in api-handler data-processor; do
  STREAM="2026/03/16/[\$LATEST]abc123"
  $AWS logs put-log-events \
    --log-group-name "/aws/lambda/$fn" \
    --log-stream-name "$STREAM" \
    --log-events "[
      {\"timestamp\":$NOW,\"message\":\"START RequestId: abc-$fn Version: \$LATEST\"},
      {\"timestamp\":$((NOW+50)),\"message\":\"{\\\"level\\\":\\\"info\\\",\\\"requestId\\\":\\\"abc-$fn\\\",\\\"message\\\":\\\"Processing request\\\",\\\"path\\\":\\\"/api/users\\\",\\\"method\\\":\\\"GET\\\",\\\"duration\\\":42}\"},
      {\"timestamp\":$((NOW+100)),\"message\":\"{\\\"level\\\":\\\"warn\\\",\\\"requestId\\\":\\\"abc-$fn\\\",\\\"message\\\":\\\"Cache miss\\\",\\\"key\\\":\\\"users:list\\\",\\\"ttl\\\":300}\"},
      {\"timestamp\":$((NOW+150)),\"message\":\"{\\\"level\\\":\\\"error\\\",\\\"requestId\\\":\\\"abc-$fn\\\",\\\"message\\\":\\\"DB timeout\\\",\\\"error\\\":\\\"connection refused\\\",\\\"retries\\\":3,\\\"host\\\":\\\"db.internal:5432\\\"}\"},
      {\"timestamp\":$((NOW+200)),\"message\":\"END RequestId: abc-$fn\"},
      {\"timestamp\":$((NOW+250)),\"message\":\"REPORT RequestId: abc-$fn Duration: 150.00 ms Billed Duration: 200 ms Memory Size: 256 MB\"}
    ]"
  NOW=$((NOW+1000))
done

# ── API Gateway v1 (REST API) ─────────────────────────────────────────────────
echo "→ API Gateway (REST)"
API_ID=$($AWS apigateway create-rest-api \
  --name "lazyaws-demo-api" \
  --query 'id' --output text)

ROOT_ID=$($AWS apigateway get-resources \
  --rest-api-id "$API_ID" \
  --query 'items[0].id' --output text)

USERS_ID=$($AWS apigateway create-resource \
  --rest-api-id "$API_ID" \
  --parent-id "$ROOT_ID" \
  --path-part users \
  --query 'id' --output text)
$AWS apigateway put-method \
  --rest-api-id "$API_ID" \
  --resource-id "$USERS_ID" \
  --http-method GET \
  --authorization-type NONE >/dev/null

ORDERS_ID=$($AWS apigateway create-resource \
  --rest-api-id "$API_ID" \
  --parent-id "$ROOT_ID" \
  --path-part orders \
  --query 'id' --output text)
$AWS apigateway put-method \
  --rest-api-id "$API_ID" \
  --resource-id "$ORDERS_ID" \
  --http-method POST \
  --authorization-type NONE >/dev/null

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

# ── IAM ───────────────────────────────────────────────────────────────────────
echo "→ IAM"
$AWS iam create-role --role-name lambda-execution-role \
  --assume-role-policy-document '{"Version":"2012-10-17","Statement":[
    {"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},
     "Action":"sts:AssumeRole"}]}' >/dev/null
$AWS iam create-role --role-name api-gateway-cloudwatch-role \
  --assume-role-policy-document '{"Version":"2012-10-17","Statement":[
    {"Effect":"Allow","Principal":{"Service":"apigateway.amazonaws.com"},
     "Action":"sts:AssumeRole"}]}' >/dev/null
$AWS iam create-policy --policy-name lambda-s3-access \
  --policy-document '{"Version":"2012-10-17","Statement":[
    {"Effect":"Allow","Action":["s3:GetObject","s3:PutObject"],
     "Resource":"arn:aws:s3:::my-app-assets/*"}]}' >/dev/null

# ── Route53 ───────────────────────────────────────────────────────────────────
echo "→ Route53"
ZONE_ID=$($AWS route53 create-hosted-zone \
  --name example.com \
  --caller-reference "lazyaws-demo-$(date +%s)" \
  --query 'HostedZone.Id' --output text)
$AWS route53 change-resource-record-sets --hosted-zone-id "$ZONE_ID" \
  --change-batch '{"Changes":[{"Action":"CREATE","ResourceRecordSet":
    {"Name":"api.example.com","Type":"A","TTL":300,
     "ResourceRecords":[{"Value":"1.2.3.4"}]}}]}' >/dev/null

echo ""
echo "✅ Seed complete. Run: go run . -local"
