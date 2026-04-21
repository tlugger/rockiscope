# ⚾ Rockiscope

[![test](https://github.com/tlugger/rockiscope/actions/workflows/test.yml/badge.svg)](https://github.com/tlugger/rockiscope/actions/workflows/test.yml)
[![coverage](https://img.shields.io/badge/coverage-75%25-yellow)](.testcoverage.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/tlugger/rockiscope)](https://goreportcard.com/report/github.com/tlugger/rockiscope)
[![Release](https://img.shields.io/github/release/tlugger/rockiscope)](https://github.com/tlugger/rockiscope/releases/latest)

A Bluesky bot that posts daily horoscopes and win/loss predictions for the Colorado Rockies — a team that has given its fans almost nothing but pain since 2018, yet here we are, building software for them.

Born on July 5, 1991, the Rockies are a Cancer. Every game day, Rockiscope scrapes the daily Cancer horoscope, pulls live stats from the MLB API, runs everything through a prediction engine where the stars get the final say, and posts the result to Bluesky — one hour before first pitch.

On off-days, it posts the horoscope with a season stats summary because even when they're not playing, you're still thinking about them. 🔮

## 🧠 The Prediction Engine

Real stats provide the foundation, but the horoscope tips the scales. The engine weighs multiple factors and learns from its mistakes over time:

| Factor | Starting Weight | What it uses |
|--------|--------------|-------------|
| Team win rate | 30% | Season record from MLB standings |
| Pitcher matchup | 30% | ERA + WHIP: our starter vs theirs |
| Head-to-head | 15% | Season series record vs today's opponent |
| Home/away split | 10% | How we play at home vs on the road |
| Streak momentum | 5% | Current W/L streak |
| 🔮 Horoscope | 10% | The stars speak via SHA-256 hash of the daily reading |

### 🎯 Dynamic Weights

The prediction engine learns from its mistakes. After each game:
1. Records the prediction + actual result to `prediction_history.json`
2. Calculates which factors "pointed" the right direction over the last 10 games
3. Adjusts factor weights ±3% toward better-performing factors
4. Clamps weights to prevent any single factor from dominating (>45%) or disappearing (<2%)

Weights start at the values above but shift over time based on what's actually predicting correctly. On first run, the bot starts fresh — weights adjust as the season progresses. After 10+ games, the model begins dynamically optimizing.

### 🔄 Follow-up Posts

After each game, Rockiscope posts a follow-up reply with the result:
- **Correct prediction**: "The cosmos don't miss. ✨ Rockies W 4-3. Season: 7/10 correct"
- **Wrong**: ""Trust the stars. Never trust the bullpen. ⭐" Rockies L 7-3. Season: 6/10 correct"

This keeps the bot "real-time" and provides accountability for the predictions. Each follow-up includes the running season record.

## 📡 CLI

```
rockiscope <command>

  run          Start the daemon (default)
  post         Force a post now, skip the schedule
  preview      Print what would be posted, don't touch Bluesky
  test-auth    Verify Bluesky credentials
  test-mlb     Hit the MLB API and show today's game
  test-horo    Scrape today's horoscope
  version      Print version
```

## 🚀 Install

One command on a Raspberry Pi (or any Linux box):

```bash
curl -sSL https://raw.githubusercontent.com/tlugger/rockiscope/main/install.sh | sudo bash
```

This will:
- 📦 Download the latest release binary (or clone + build from source if no release exists)
- 📄 Create a `.env` file with placeholder Bluesky credentials
- ⚙️ Install and enable a systemd service

After the install, edit your credentials and start the service:

```bash
sudo nano /home/pi/rockiscope/.env    # add your Bluesky app password
sudo systemctl start rockiscope
```

Create an app password at [bsky.app/settings/app-passwords](https://bsky.app/settings/app-passwords).

To **update**, just re-run the same install command — it'll stop the service, download the latest binary, and restart.

### Managing the service

```bash
sudo systemctl status rockiscope      # check status
sudo systemctl restart rockiscope     # restart
tail -f /home/pi/rockiscope/rockiscope.log  # logs
```

## 📊 Data Sources

- **MLB Stats API** — free, no auth, real-time game data + stats
- **Horoscope.com** — daily Cancer horoscope
- **Bluesky AT Protocol** — posting via XRPC API

## ✨ Sample Posts

Each post includes the game info and prediction as text, with the full horoscope rendered as an attached image card — crescent moon, constellation, and all.

**Game Day** (1 hour before first pitch):
```
⚾ Rockies vs Houston Astros
🕐 1:10 PM MDT at Coors Field
🪖 Michael Lorenzen (9.00 ERA, 0-1)
📊 5-6 | vs HOU: 2-1 | W2

🔮 A slight celestial nudge toward a Rockies defeat (55%)
```
📎 Attached: horoscope image card

**Off Day** (10 AM MST):
```
⚾ No Rockies game today.
📊 5-6 (.455) | Run Diff: -3 | L1
```
📎 Attached: horoscope image card

---

Built with mass amounts of misplaced loyalty and a mass amount of Coors Banquet. 🏔️
