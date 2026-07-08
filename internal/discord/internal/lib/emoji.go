package lib

import "regexp"

var emojiPattern = regexp.MustCompile(":([a-z0-9_+-]+):")

// emojiMap translates Discord shortcodes like ":fire:" to Unicode.
var emojiMap = map[string]string{
	"outbox_tray":      "📤",
	"inbox_tray":       "📥",
	"skull":            "💀",
	"skull_crossbones": "☠️",
	"crossed_swords":   "⚔️",
	"warning":          "⚠️",
	"white_check_mark": "✅",
	"x":                "❌",
	"heart":            "❤️",
	"fire":             "🔥",
	"star":             "⭐",
	"tada":             "🎉",
	"wave":             "👋",
	"arrow_up":         "⬆️",
	"arrow_down":       "⬇️",
	"arrow_left":       "⬅️",
	"arrow_right":      "➡️",
	"moneybag":         "💰",
	"gem":              "💎",
	"scroll":           "📜",
	"map":              "🗺️",
	"house":            "🏠",
	"hammer":           "🔨",
	"pick":             "⛏️",
	"axe":              "🪓",
	"bow_and_arrow":    "🏹",
	"shield":           "🛡️",
	"crown":            "👑",
	"trident":          "🔱",
	"dagger":           "🗡️",
	"bomb":             "💣",
	"boomerang":        "🪃",
	"magic_wand":       "🪄",
	"crystal_ball":     "🔮",
	"book":             "📖",
	"books":            "📚",
	"scroll1":          "📜",
	"page_with_curl":   "📃",
	"memo":             "📝",
	"pencil":           "✏️",
	"pen":              "🖊️",
	"paintbrush":       "🖌️",
	"crayon":           "🖍️",
	"magnifying_glass": "🔍",
	"key":              "🔑",
	"lock":             "🔒",
	"unlock":           "🔓",
	"gear":             "⚙️",
	"tools":            "🛠️",
	"nut_and_bolt":     "🔩",
	"brick":            "🧱",
	"wood":             "🪵",
	"rock":             "🪨",
	"mine":             "⛏️",
	"tent":             "⛺",
	"campfire":         "🔥",
	"flashlight":       "🔦",
	"lantern":          "🏮",
	"candle":           "🕯️",
	"hourglass":        "⏳",
	"stopwatch":        "⏱️",
	"clock":            "🕐",
	"alarm_clock":      "⏰",
	"calendar":         "📅",
	"date":             "📅",
	"clock12":          "🕛",
	"clock1":           "🕐",
	"clock2":           "🕑",
	"clock3":           "🕒",
	"clock4":           "🕓",
	"clock5":           "🕔",
	"clock6":           "🕕",
	"clock7":           "🕖",
	"clock8":           "🕗",
	"clock9":           "🕘",
	"clock10":          "🕙",
	"clock11":          "🕚",
}

// ReplaceEmojis replaces Discord shortcode emojis with Unicode equivalents.
func ReplaceEmojis(text string) string {
	return emojiPattern.ReplaceAllStringFunc(text, func(match string) string {
		name := emojiPattern.FindStringSubmatch(match)[1]
		if emoji, ok := emojiMap[name]; ok {
			return emoji
		}
		return match
	})
}
