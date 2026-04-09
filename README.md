# ⚾ Rockiscope

A Bluesky bot that posts daily horoscopes and win/loss predictions for the Colorado Rockies — a team that has given its fans almost nothing but pain since 2018, yet here we are, building software for them.

Born on July 5, 1991, the Rockies are a Cancer. This tracks. Every game day, Rockiscope scrapes the daily Cancer horoscope, pulls live stats from the MLB API, runs everything through a prediction engine where the stars get the final say, and posts the result to Bluesky — one hour before first pitch.

On off-days, it posts the horoscope with a season stats summary because even when they're not playing, you're still thinking about them. 🔮

## 🧠 The Prediction Engine

Real stats provide the foundation, but the horoscope tips the scales. Honestly the horoscope is probably more reliable than the Rockies' bullpen most nights:

| Factor | Weight | What it uses |
|--------|--------|-------------|
| Team win rate | 30% | Season record from MLB standings |
| Pitcher matchup | 30% | ERA + WHIP: our starter vs theirs |
| Head-to-head | 15% | Season series record vs today's opponent |
| Home/away split | 10% | How we play at home vs on the road |
| Streak momentum | 5% | Current W/L streak |
| 🔮 Horoscope | 10% | The stars speak via SHA-256 hash of the daily reading |

Early in the season when data is thin, available factors get re-weighted and the stars hold more power. By August, when the Rockies are 20 games back as is tradition, the model has plenty of data to confidently predict more losses.

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
- 🔑 Prompt for your Bluesky [app password](https://bsky.app/settings/app-passwords)
- ✅ Test authentication
- ⚙️ Install and start a systemd service

That's it. It runs itself from there — wakes up at 5 AM MST, checks the schedule, posts an hour before game time, goes back to sleep. More consistent than the lineup, at least.

### Managing the service

```bash
sudo systemctl status rockiscope      # check status
sudo systemctl restart rockiscope     # restart
tail -f /home/pi/rockiscope/rockiscope.log  # logs
```

### Updating

```bash
# Build on your machine, copy to Pi, restart
GOOS=linux GOARCH=arm64 go build -o rockiscope
scp rockiscope pi@yourpi:/home/pi/rockiscope/
ssh pi@yourpi 'sudo systemctl restart rockiscope'
```

## 📊 Data Sources

- **MLB Stats API** — free, no auth, real-time game data + stats
- **Horoscope.com** — daily Cancer horoscope
- **Bluesky AT Protocol** — posting via XRPC API

## ✨ Sample Posts

Posts are threaded — game info first, full horoscope as a reply.

**Game Day** (1 hour before first pitch):
```
⚾ Rockies vs Houston Astros
🕐 1:10 PM MDT at Coors Field
🪖 Michael Lorenzen (9.00 ERA, 0-1)
📊 5-6 | vs HOU: 2-1 | W2

🔮 A slight celestial nudge toward a Rockies defeat (55%)
```
↳ reply:
```
♋ Today's Cancer horoscope:

Cancer, the energy around you is intense today. You may
feel pulled in multiple directions. Take a deep breath and
remember that the stars are aligned in your favor, even if
it doesn't feel like it right now.
```

**Off Day** (10 AM MST):
```
⚾ No Rockies game today.
📊 5-6 (.455) | Run Diff: -3 | L1
```
↳ reply:
```
♋ Today's Cancer horoscope:

Rest and reflection bring clarity today. The cosmos suggest
patience and self-care during this quiet moment.
```

---

Built with mass amounts of misplaced loyalty and a mass amount of Coors Banquet. 🏔️
