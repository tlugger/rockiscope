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
- **Correct prediction**: 
```
🌙 Planets aligned. Opponent declined.
📊 Rockies W | 4-3 | Season: 7/10 correct
```
- **Wrong**: 
```
🔮 Retrograde strikes again. So did the opponent.
📊 Rockies L | 7-3 | Season: 6/10 correct
```

## 🗓️ The Other Six Months

The Rockies' season ends in September. Rockiscope's doesn't. The bot reads MLB's season calendar and changes modes on its own:

| Phase | What it posts |
|-------|---------------|
| ⚾ **Regular season** | Everything above. Unchanged. |
| 🍼 **Postseason** | A **report card** thread for the season (accuracy, which signals worked, what the model learned, best call, worst miss). Then the bot **adopts a playoff team**, picking the roster whose players' birth charts are most compatible with a Cancer. It predicts the adopted team's games and replies with results. When they're eliminated, it re-adopts. When someone wins the World Series, it takes credit if it can. |
| 🔥 **Hot stove** | Only posts when something happens. Rockies trades, signings, waiver claims, DFAs and releases are checked at 10 AM and 5 PM, each with the player's zodiac sign and Rockies compatibility. Minor-league deals are ignored, and busy days roll up into a digest. Every Sunday there's an **Opening Day countdown**. |
| 🌵 **Spring training** | Predictions for spring games. *It doesn't count. Neither does this.* These picks are stored separately and never touch the real season's weights. The countdown goes daily for the final week. |

**Season rollover.** When spring training starts, the finished season's predictions, final weights and a stats summary are archived to `archive/<season>/`. The live history then resets with default weights and the bot announces the fresh start. If the bot was offline all spring, the rollover happens on Opening Day instead.

### 🛟 Built to survive a Raspberry Pi

- **Crash-safe state.** Every file is written atomically with a `.bak` fallback, so a power cut mid-write can't corrupt the season.
- **No double posts.** Every seasonal post has an idempotency key that's saved the moment it posts. A reboot mid-day, or mid-thread, picks up exactly where it left off.
- **No late predictions.** If the Pi was down through a pregame window, that game is skipped. Predicting after first pitch would be cheating.
- **Graceful degradation.** Failed API calls retry with backoff, then the bot tries again in 15 minutes. If MLB's calendar is unreachable it falls back to a cached copy, then to a month-based guess. If a horoscope or stat is missing, the post goes out without it.
- **Polite.** MLB requests are spaced out, the bot posts nothing between 10 PM and 8 AM except game results, and a daily post cap catches runaway bugs.
- **Clock-aware.** The bot waits for NTP before acting, since a Pi has no hardware clock, and timezone data is embedded in the binary.

## 📊 Dashboard

`rockiscope run` also serves an analytics dashboard on port 8086 (`ROCKISCOPE_PORT` to change it).

- **Every season, every phase.** Pick a season, then switch between ⚾ Regular Season, 🍼 Postseason, 🌵 Spring Training and 🔥 Hot Stove. Each prediction phase gets the same set:
  - **Cosmic highlights:** biggest upset, worst whiff, called shots, streaks.
  - **A game-by-game season calendar.**
  - **The Bot vs. the Pessimist:** would "always pick L" have beaten us? (In 2026 it did, by 5.) Postseason and spring race a coin flip instead.
  - **Stars vs. Science:** who was right when the horoscope disagreed with the stats.
  - **Confidence Is Decorative:** accuracy by the confidence phrase the posts used.
  - **Record by opponent**, and a game log with links to the Bluesky posts.
- **Regular season** adds **Trust in the Stars**, the weights rebuilt game by game by replaying the engine's updates, plus the factor-weights pie.
- **Postseason** adds the Adoption Agency (who we adopted, when they let us down) and the playoff birth-chart audit.
- **Hot Stove** shows every roster move posted, with signs and Rockies compatibility.
- **Finished seasons** get their report card. 📚 **All-time** compares seasons side by side.
- **A phase banner** shows what the bot is doing right now: the adopted team, the Opening Day countdown, or spring picks.

It's read-only: the dashboard reads the same crash-safe files the bot writes, including `archive/`. The JSON is available at `/api/status`, `/api/seasons` and `/api/season/{year}`.

## 📡 CLI

```
rockiscope <command>

  run          Start the daemon (default)
  post         Force a post now, skip the schedule
  preview      Print what would be posted, don't touch Bluesky or any files
  phase        Show the current mode (regular season, postseason, offseason, spring)
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

To **update**, re-run the same install command. It:

1. backs up your data (`prediction_history.json`, `season_state.json`, `archive/`) to `backups/`
2. downloads and checksum-verifies the new binary *before* touching the running bot
3. swaps it in, keeping the old one as `rockiscope.prev`
4. restarts and health-checks the service, **rolling back automatically** if the new version won't stay up

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
