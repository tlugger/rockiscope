package formatter

import "fmt"

type FollowUp struct {
	Outcome  string
	Score    string
	Correct  bool
	Record   string
	Opponent string
	PostURI  string
	Seed     int // picks the message; 0 derives one from the score
}

var winCorrectMessages = []string{
	"✨ Called it. The stars cashed in and the Rockies didn't trip on the way to the bank.",
	"🔮 Prediction correct. Victory confirmed. I need to lie down.",
	"🌠 A cosmic W. Frame it. Laminate it. This may not happen again until June.",
	"⭐ The planets aligned and, for once, so did the infield.",
	"📈 Correct pick AND a win? Buy a lottery ticket. Buy two.",
	"🌙 The universe said win. The Rockies listened. Historic.",
	"👁️ I saw this in the void hours ago. The void was right.",
	"🍳 The stars cooked and the bullpen didn't burn it. Growth.",
	"✨ Fate wrote a win and nobody edited it out. Remarkable.",
	"🔭 Astronomy remains the most reliable part of this franchise.",
}

var winIncorrectMessages = []string{
	"🫠 The stars were wrong and I have never been happier to be wrong.",
	"🌤️ Forecast missed. Outcome delightful. Ignore the forecast.",
	"🤷 Astrology failed upward into a victory. We accept.",
	"🍀 Pure chaos overrode the cosmos. Chaos has a Rockies hat now.",
	"🌪️ Cosmic error. Scoreboard success. Filing this under miracles.",
	"😅 The stars didn't see that coming. Neither did the opponent.",
	"📉 Prediction busted. Vibes immaculate.",
	"🌈 The stars fumbled. The Rockies stumbled into glory anyway.",
	"🎲 The universe rolled a natural 20 by accident. Don't tell it.",
	"✨ Wrong pick, right outcome. Mercury is apologizing in the group chat.",
}

var lossCorrectMessages = []string{
	"📉 Called it. Pain was written in the stars, in pen.",
	"🌑 The horoscope warned us. We watched anyway. Every pitch.",
	"🔮 Cosmic accuracy remains tragically high.",
	"💀 Stars said loss. Rockies delivered loss. Teamwork.",
	"📊 Prediction correct. Morale incorrect.",
	"🌘 Even the universe saw that coming, and the universe is very far away.",
	"😐 The stars remain undefeated. Unlike the Rockies.",
	"📚 Another chapter of the prophecy, fulfilled with depressing accuracy.",
	"🌌 Fate remains a Rockies season ticket holder. Fate is suffering too.",
	"🧊 Cold, predictable, cosmically certified disappointment.",
}

var lossIncorrectMessages = []string{
	"💔 The stars believed. The bullpen had other ideas.",
	"🌪️ Cosmic sabotage detected. Opening an investigation.",
	"🫠 The prediction failed. So did several other things.",
	"🔧 The universe needs to rebuild its model. The Rockies need to rebuild everything else.",
	"📉 Wrong prediction, worse outcome. A rare double.",
	"🌑 Even astrology couldn't predict that collapse. And astrology predicts everything.",
	"🤦 The stars said win. The Rockies said, respectfully, no.",
	"⚠️ Catastrophic vibe mismatch. Mercury has been notified.",
	"🌘 Mercury clearly entered the bullpen around the seventh.",
	"📦 The packaging said hope. The contents were the eighth inning.",
}

func FormatFollowUp(f FollowUp) string {
	seed := f.Seed
	if seed == 0 {
		seed = seedOf(f.Score, f.Record)
	}
	var msgs []string
	switch {
	case f.Correct && f.Outcome == "Rockies W":
		msgs = winCorrectMessages
	case f.Correct:
		msgs = lossCorrectMessages
	case f.Outcome == "Rockies W":
		msgs = winIncorrectMessages
	default:
		msgs = lossIncorrectMessages
	}

	return Fit(fmt.Sprintf("%s\n\n🏁 %s\n📊 %s\n\n🎯 %s",
		pick(msgs, seed), f.Outcome, f.Score, f.Record), MaxPostLength)
}
