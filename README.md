# Rockiscope

A Bluesky bot that posts daily horoscopes and win/loss predictions for the Colorado Rockies.

Born on July 5, 1991, the Rockies are a Cancer. Every day during baseball season, Rockiscope scrapes the daily Cancer horoscope, pulls live stats from the MLB API, runs everything through a prediction engine where the stars get the final say, and posts the result to Bluesky.

On off-days, it posts the horoscope alongside a season stats summary to keep the vibes going.

## How It Works

**Game Days** — 1 hour before first pitch:
- Fetches today's game from the MLB Stats API (opponent, venue, time, probable pitchers)
- Pulls season standings, head-to-head record, and starting pitcher stats
- Scrapes the daily Cancer horoscope from horoscope.com
- Runs the prediction engine (stats-weighted, horoscope-tipped)
- Posts to Bluesky

**Off Days** — 10 AM MST:
- Posts the daily Cancer horoscope with a season record summary

### The Prediction Engine

The prediction is a weighted score from real baseball data with the horoscope as the cosmic tiebreaker:

| Factor | Weight | Source |
|--------|--------|--------|
| Team win rate | 30% | MLB standings |
| Pitcher matchup | 30% | ERA + WHIP comparison |
| Head-to-head record | 15% | Season series vs opponent |
| Home/away split | 10% | Home vs away win rate |
| Streak momentum | 5% | Current W/L streak |
| Horoscope | 10% | SHA-256 hash of horoscope text |

Early in the season when sample sizes are small, the available factors are re-weighted and the stars hold more influence.

## Project Structure

```
rockiscope/
├── main.go                          # CLI entrypoint (subcommands)
├── internal/
│   ├── mlb/                         # MLB Stats API client
│   │   ├── client.go                # GameProvider interface + implementation
│   │   ├── client_test.go
│   │   └── types.go                 # Game, TeamRecord, PitcherStats, H2HRecord
│   ├── horoscope/                   # Horoscope.com scraper
│   │   ├── scraper.go               # Provider interface + implementation
│   │   └── scraper_test.go
│   ├── prediction/                  # Win prediction engine (pure logic, no I/O)
│   │   ├── engine.go
│   │   └── engine_test.go
│   ├── formatter/                   # Post text builder (game day + off day)
│   │   ├── post.go
│   │   └── post_test.go
│   ├── bluesky/                     # Bluesky AT Protocol client
│   │   ├── client.go                # Poster interface + implementation
│   │   └── client_test.go
│   └── scheduler/                   # Scheduling loop + force-run support
│       ├── scheduler.go
│       └── scheduler_test.go
├── testdata/                        # JSON/HTML fixtures for tests
├── install.sh                       # One-line Pi installer
├── rockiscope.service               # systemd unit file
└── .env.example                     # Credential template
```

Every external dependency (MLB API, horoscope.com, Bluesky) is behind an interface, making all business logic unit-testable with mocks.

## CLI

```
rockiscope <command>

Commands:
  run          Start the scheduler daemon (default)
  post         Force a post right now, skipping the schedule
  preview      Fetch all data and print the post without posting
  test-auth    Test Bluesky authentication
  test-mlb     Test MLB API connectivity and show today's game
  test-horo    Test horoscope scraper and show today's reading
  version      Print version
  help         Show help
```

### Examples

```bash
# Preview what would be posted today (no Bluesky creds needed)
rockiscope preview

# Test that your Bluesky credentials work
export BLUESKY_USERNAME=yourname.bsky.social
export BLUESKY_PASSWORD=your-app-password
rockiscope test-auth

# Test the MLB API and see today's game info
rockiscope test-mlb

# Force an immediate post (skips schedule, ignores dedup)
rockiscope post

# Start the daemon (normal operation)
rockiscope run
```

## Installation

### Prerequisites

- A Bluesky account with an [app password](https://bsky.app/settings/app-passwords)
- For building from source: Go 1.21+

### Option 1: Install on Raspberry Pi (one-liner)

```bash
curl -sSL https://raw.githubusercontent.com/tlugger/rockiscope/main/install.sh | sudo bash
```

This will:
1. Detect your Pi's architecture (arm64/armv7)
2. Download the latest release binary
3. Prompt for your Bluesky credentials
4. Create the `.env` file with proper permissions
5. Install and start a systemd service

### Option 2: Build from Source

```bash
# Clone
git clone https://github.com/tlugger/rockiscope.git
cd rockiscope

# Run tests
go test ./...

# Build
go build -o rockiscope

# Cross-compile for Raspberry Pi
GOOS=linux GOARCH=arm64 go build -o rockiscope
```

### Option 3: Manual Setup on Any Linux Server

```bash
# Copy the binary to your server
scp rockiscope you@server:/opt/rockiscope/

# Create .env file
cat > /opt/rockiscope/.env << 'EOF'
BLUESKY_USERNAME=yourname.bsky.social
BLUESKY_PASSWORD=your-app-password
EOF
chmod 600 /opt/rockiscope/.env

# Test connectivity
cd /opt/rockiscope
source .env && export BLUESKY_USERNAME BLUESKY_PASSWORD
./rockiscope test-auth
./rockiscope test-mlb
./rockiscope test-horo
./rockiscope preview

# Install systemd service
sudo cp rockiscope.service /etc/systemd/system/
# Edit paths in the service file if not using /home/pi/rockiscope
sudo systemctl daemon-reload
sudo systemctl enable rockiscope
sudo systemctl start rockiscope
```

## Configuration

The only configuration is via environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `BLUESKY_USERNAME` | Yes | Your Bluesky handle (e.g. `yourname.bsky.social`) |
| `BLUESKY_PASSWORD` | Yes | A Bluesky [app password](https://bsky.app/settings/app-passwords) |

When running as a systemd service, these are loaded from the `.env` file via `EnvironmentFile`.

## Managing the Service

```bash
# Check status
sudo systemctl status rockiscope

# View logs
tail -f /home/pi/rockiscope/rockiscope.log

# Restart after an update
sudo systemctl restart rockiscope

# Stop
sudo systemctl stop rockiscope

# Disable (won't start on boot)
sudo systemctl disable rockiscope
```

## Updating

```bash
# On your dev machine
GOOS=linux GOARCH=arm64 go build -o rockiscope

# Copy to Pi
scp rockiscope pi@yourpi:/home/pi/rockiscope/

# Restart the service
ssh pi@yourpi 'sudo systemctl restart rockiscope'
```

## Running Tests

```bash
go test ./...
```

All tests use local fixture files (JSON and HTML) — no network calls required. The test suite covers:
- MLB API response parsing (schedule, standings, pitcher stats, H2H)
- Horoscope HTML scraping and text extraction
- Prediction engine (favorable, underdog, no-data, early-season, clamping)
- Post formatting (game day, off day, character limits)
- Bluesky auth and posting (mock HTTP server)
- Scheduler (game day timing, off day, dedup, sleep behavior)

## Data Sources

- **MLB Stats API** (`statsapi.mlb.com`) — free, no auth required
- **Horoscope.com** — daily Cancer horoscope, scraped HTML
- **Bluesky** — AT Protocol XRPC API for posting

## Sample Posts

**Game Day:**
```
Rockies vs Houston Astros
1:10 PM MDT at Coors Field
SP: Michael Lorenzen (9.00 ERA, 0-1)
Season: 5-6 | vs HOU: 2-1 | W2

Prediction: A slight celestial nudge toward a Rockies defeat (55%)

Cancer, the energy around you is intense today...
```

**Off Day:**
```
No Rockies game today.
Season: 5-6 (.455) | Run Diff: -3 | L1

Rest and reflection bring clarity today. The stars suggest patience...
```
