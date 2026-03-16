# lazyaws demo — generate with: vhs demo/demo.tape
# Prerequisites: docker compose up -d && ./scripts/seed.sh

Output demo/demo.gif

Set Shell "bash"
Set FontSize 14
Set FontFamily "JetBrains Mono"
Set Width 1200
Set Height 700
Set Theme "Tokyo Night"
Set Padding 16
Set PlaybackSpeed 1.0

# ── Boot the app ─────────────────────────────────────────────────────────────
Hide
Type "go build -o /tmp/lazyaws . && /tmp/lazyaws -local"
Show
Enter
Sleep 2s

# ── S3 is selected by default — browse buckets ───────────────────────────────
Sleep 1s

# Navigate down through the bucket list
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 600ms

# Move focus to the Items panel (right panel shows bucket objects)
Tab
Sleep 500ms

# Browse objects in the bucket
Down
Sleep 400ms
Down
Sleep 600ms

# Open detail panel and cycle detail tabs
Tab
Sleep 600ms

# Cycle through S3 tabs: Overview → Objects → Policy
]
Sleep 800ms
]
Sleep 800ms
[
Sleep 600ms

# ── Switch to Lambda ──────────────────────────────────────────────────────────
# Back to Resources panel
Shift+Tab
Shift+Tab
Sleep 400ms

# Navigate down to Lambda in the resources list
Down
Sleep 300ms

# Jump to items
Tab
Sleep 500ms

# Browse Lambda functions
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 500ms

# View detail
Tab
Sleep 600ms

# Cycle Lambda tabs: Overview → Env vars → Triggers
]
Sleep 800ms
]
Sleep 800ms
[
Sleep 600ms

# ── Switch to DynamoDB ────────────────────────────────────────────────────────
Shift+Tab
Shift+Tab
Sleep 400ms

Down
Down
Down
Down
Down
Down
Sleep 400ms

Tab
Sleep 500ms
Down
Sleep 400ms

Tab
Sleep 600ms
]
Sleep 800ms
[
Sleep 600ms

# ── Switch to Secrets Manager ─────────────────────────────────────────────────
Shift+Tab
Shift+Tab
Sleep 400ms

Down
Down
Down
Sleep 400ms

Tab
Sleep 500ms
Down
Sleep 300ms
Down
Sleep 500ms

Tab
Sleep 600ms
]
Sleep 800ms
]
Sleep 800ms

# ── Switch to CloudFormation ──────────────────────────────────────────────────
Shift+Tab
Shift+Tab
Sleep 400ms

Down
Down
Sleep 400ms

Tab
Sleep 500ms

Tab
Sleep 600ms
]
Sleep 800ms
]
Sleep 800ms
]
Sleep 800ms
[
Sleep 800ms

# ── Quit ──────────────────────────────────────────────────────────────────────
Sleep 1s
Type "q"
Sleep 500ms
