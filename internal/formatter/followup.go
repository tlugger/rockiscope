package formatter

import (
	"fmt"
	"math/rand"
)

type FollowUp struct {
	Outcome   string
	Score    string
	Correct  bool
	Record   string
	Opponent string
	PostURI  string
}

var correctMessages = []string{
	"✨ The stars delivered. Rare W alignment.",
	"🔮 Mercury stayed out of the bullpen today.",
	"🌌 Astrology remains undefeated. The Rockies… TBD.",
	"👁️ I consulted the void and it whispered 'win.'",
	"🍳 The horoscope cooked. Finally.",
	"📈 Cosmic accuracy: 100%. Team batting: unrelated.",
	"🌠 Even the universe was surprised.",
	"⭐ Trust the stars. Never trust the bullpen.",
	"✨ The vibes were immaculate. For once.",
	"🌙 Planets aligned. Opponent declined.",
}

var incorrectMessages = []string{
	"🌑 Mercury entered the bullpen again.",
	"💔 The stars said 'maybe,' the Rockies said 'no.'",
	"⚠️ Cosmic interference detected.",
	"🔧 The universe is rebuilding too.",
	"🫠 Horoscope was right. Execution was optional.",
	"🧊 The vibes were there. The offense was not.",
	"🔮 Retrograde strikes again. So did the opponent.",
	"🤔 The stars did their part. Did we?",
	"🌘 Blame Mercury. Blame the bullpen. Same thing.",
	"🌌 Next prediction powered by denial.",
	"📉 Season outcome remains… astrologically consistent.",
}

func FormatFollowUp(f FollowUp) string {
	var msg string
	if f.Correct {
		msg = correctMessages[rand.Intn(len(correctMessages))]
	} else {
		msg = incorrectMessages[rand.Intn(len(incorrectMessages))]
	}

	return fmt.Sprintf("%s\n\n🏁 %s\n📊 %s\n\n🎯 %s",
		msg, f.Outcome, f.Score, f.Record)
}
