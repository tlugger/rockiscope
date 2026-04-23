package formatter

import (
	"fmt"
	"math/rand"
)

type FollowUp struct {
	Outcome  string
	Score    string
	Correct  bool
	Record   string
	Opponent string
	PostURI  string
}

var winCorrectMessages = []string{
	"✨ Called it. The stars actually cashed in tonight.",
	"🔮 Horoscope nailed it. The Rockies… surprisingly cooperated.",
	"🌠 Cosmic W secured. Frame it — this may not happen again soon.",
	"⭐ Planets aligned and nobody tripped over first base.",
	"📈 Prediction correct AND a win? Buy a lottery ticket.",
	"🌙 The universe said win. The Rockies listened. For once.",
	"👁️ I saw this in the void hours ago.",
	"🍳 The stars cooked and the Rockies didn't burn it.",
	"✨ Even fate couldn't mess this one up.",
	"🔭 Astronomy remains the most reliable part of this season.",
}

var winIncorrectMessages = []string{
	"🫠 The stars were wrong, but hey — we'll take the win.",
	"🌤️ Forecast missed. Outcome appreciated.",
	"🤷‍♂️ Astrology failed upward into a victory.",
	"🍀 Pure luck override detected.",
	"🌪️ Cosmic error. Scoreboard success.",
	"😅 Didn't see that coming. Neither did the opponent.",
	"📉 Prediction busted. Vibes improved.",
	"🌈 Stars fumbled, Rockies stumbled into glory.",
	"🎲 The universe rolled a natural 20 by accident.",
	"✨ Wrong prediction, right outcome. No complaints.",
}

var lossCorrectMessages = []string{
	"📉 Called it. Pain was written in the stars.",
	"🌑 The horoscope warned us. We watched anyway.",
	"🔮 Cosmic accuracy remains tragically high.",
	"💀 Stars said loss. Rockies delivered loss.",
	"📊 Prediction correct. Morale incorrect.",
	"🌘 Even the universe saw that coming.",
	"😐 The stars remain undefeated. Unlike the Rockies.",
	"📚 Another chapter in the prophecy fulfilled.",
	"🌌 Fate remains a Rockies fan.",
	"🧊 Cold, predictable disappointment.",
}

var lossIncorrectMessages = []string{
	"💔 The stars tried. The bullpen had other ideas.",
	"🌪️ Cosmic sabotage detected.",
	"🫠 Prediction failed. So did everything else.",
	"🔧 Universe needs to rebuild its model.",
	"📉 Wrong prediction, worse outcome.",
	"🌑 Even astrology couldn't predict that collapse.",
	"🤦‍♂️ The stars said win. The Rockies said absolutely not.",
	"⚠️ Catastrophic vibe mismatch.",
	"🌘 Mercury clearly entered the bullpen mid-game.",
	"📦 Packaging said 'hope.' Contents were disappointment.",
}

func FormatFollowUp(f FollowUp) string {
	var msg string
	if f.Correct {
		switch f.Outcome {
		case "Rockies W":
			msg = winCorrectMessages[rand.Intn(len(winCorrectMessages))]
		default:
			msg = lossCorrectMessages[rand.Intn(len(lossCorrectMessages))]
		}

	} else {
		switch f.Outcome {
		case "Rockies W":
			msg = winIncorrectMessages[rand.Intn(len(winIncorrectMessages))]
		default:
			msg = lossIncorrectMessages[rand.Intn(len(lossIncorrectMessages))]
		}
	}

	return fmt.Sprintf("%s\n\n🏁 %s\n📊 %s\n\n🎯 %s",
		msg, f.Outcome, f.Score, f.Record)
}
