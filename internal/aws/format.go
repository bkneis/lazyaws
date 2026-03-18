package aws

import (
	"fmt"
	"strings"
)

// KV renders key-value pairs as left-aligned columns with keys right-padded.
func KV(pairs [][2]string) string {
	maxKey := 0
	for _, p := range pairs {
		if len(p[0]) > maxKey {
			maxKey = len(p[0])
		}
	}
	var sb strings.Builder
	for _, p := range pairs {
		fmt.Fprintf(&sb, "  %s%-*s[-]  %s\n", ActiveTags.Header, maxKey+1, p[0]+":", p[1])
	}
	return sb.String()
}

// displayLen returns the visible character count of s, excluding tview tag markup
// (e.g. [red], ["region"]) and \x02...\x03 link markers.
func displayLen(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] == '\x02' {
			if j := strings.IndexByte(s[i+1:], '\x03'); j >= 0 {
				i += j + 2
				continue
			}
		}
		if s[i] == '[' {
			if j := strings.IndexByte(s[i+1:], ']'); j >= 0 {
				i += j + 2
				continue
			}
		}
		n++
		i++
	}
	return n
}

// Table renders a header row, a separator line, and data rows.
func Table(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = displayLen(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && displayLen(cell) > widths[i] {
				widths[i] = displayLen(cell)
			}
		}
	}

	padded := make([]string, len(headers))
	for i, h := range headers {
		padded[i] = fmt.Sprintf("%-*s", widths[i]+2, h)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "  %s%s[-]\n  ", ActiveTags.Header, strings.Join(padded, ""))
	for _, w := range widths {
		sb.WriteString(strings.Repeat("─", w) + "  ")
	}
	sb.WriteString("\n")
	for _, row := range rows {
		sb.WriteString("  ")
		for i, cell := range row {
			if i < len(widths) {
				sb.WriteString(cell)
				pad := widths[i] + 2 - displayLen(cell)
				if pad > 0 {
					sb.WriteString(strings.Repeat(" ", pad))
				}
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// tviewEscape escapes square brackets in raw text so tview does not interpret
// them as color/style tags when SetDynamicColors(true) is active.
func tviewEscape(s string) string {
	return strings.ReplaceAll(s, "[", "[[]")
}

// IsSensitiveKey returns true if the key name suggests a sensitive value
// (contains password, secret, token, or key — case-insensitive).
func IsSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	for _, s := range []string{"password", "secret", "token", "key"} {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// Link returns a styled cross-resource link. Navigation metadata is embedded
// as \x02provider:targetID\x03 (stripped before display, parsed for navigation).
func Link(label, providerName, targetID string) string {
	return "\x02" + providerName + ":" + targetID + "\x03" + ActiveTags.Link + label + "[-::-]"
}

// arnLastSegment returns the resource name from an ARN.
// Splits on ":" first, then on "/" to handle "role/MyRole" → "MyRole".
func arnLastSegment(arn string) string {
	parts := strings.Split(arn, ":")
	seg := parts[len(parts)-1]
	if idx := strings.LastIndex(seg, "/"); idx >= 0 {
		return seg[idx+1:]
	}
	return seg
}

// arnToSQSURL converts an SQS ARN to its queue URL form.
// arn:aws:sqs:{region}:{accountId}:{queueName} → https://sqs.{region}.amazonaws.com/{accountId}/{queueName}
func arnToSQSURL(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) != 6 {
		return arn
	}
	return fmt.Sprintf("https://sqs.%s.amazonaws.com/%s/%s", parts[3], parts[4], parts[5])
}

// parseLambdaFromIntegrationURI extracts the Lambda function name from an API Gateway integration URI.
// Handles both direct Lambda ARN and proxy URI format
// (arn:aws:apigateway:...:lambda:path/.../functions/{lambdaArn}/invocations).
func parseLambdaFromIntegrationURI(uri string) string {
	// Proxy format: contains "functions/" and "/invocations"
	if idx := strings.Index(uri, "functions/"); idx >= 0 {
		rest := uri[idx+len("functions/"):]
		if end := strings.Index(rest, "/invocations"); end >= 0 {
			rest = rest[:end]
		}
		// rest is the Lambda ARN — return last segment
		return arnLastSegment(rest)
	}
	// Direct Lambda ARN
	return arnLastSegment(uri)
}
