package bot

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"scribo/mode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// telegramMessageLimit is Telegram's hard cap, counted in UTF-16 units.
const telegramMessageLimit = 4096

// splitBudget is the source-text budget a chunk starts with. It leaves headroom for
// the expansion rendering adds; splitForFormat shrinks it further when that is not
// enough for the text at hand.
const splitBudget = 3900

// minSplitBudget stops the shrink loop. Below this the text is pathological enough
// that no budget helps, and the plain-text fallback in sendSuccessResponse takes over.
const minSplitBudget = 128

var (
	reFence       = regexp.MustCompile("(?s)```[a-zA-Z0-9_.+-]*\r?\n?(.*?)```")
	reInlineCode  = regexp.MustCompile("`([^`\n]+)`")
	reHeading     = regexp.MustCompile(`^ {0,3}#{1,6}[ \t]+(.*?)[ \t]*#*$`)
	reBullet      = regexp.MustCompile(`^(\s*)[-*+][ \t]+`)
	reLink        = regexp.MustCompile(`\[([^\]\n]*)\]\(([^)\s]+)\)`)
	reBold        = regexp.MustCompile(`\*\*([^\n]+?)\*\*`)
	reBoldAlt     = regexp.MustCompile(`__([^\n]+?)__`)
	reStrike      = regexp.MustCompile(`~~([^\n]+?)~~`)
	reItalic      = regexp.MustCompile(`(^|[^*\w])\*([^*\n]+?)\*`)
	reItalicAlt   = regexp.MustCompile(`(^|[^_\w])_([^_\n]+?)_`)
	rePlaceholder = regexp.MustCompile("\x00(\\d+)\x00")
)

// stash parks already-rendered fragments behind NUL-delimited markers so the later
// inline rules cannot reach inside them.
type stash struct {
	items []string
}

func (s *stash) put(v string) string {
	s.items = append(s.items, v)
	return fmt.Sprintf("\x00%d\x00", len(s.items)-1)
}

func (s *stash) restore(v string) string {
	if len(s.items) == 0 {
		return v
	}
	return rePlaceholder.ReplaceAllStringFunc(v, func(m string) string {
		idx, err := strconv.Atoi(strings.Trim(m, "\x00"))
		if err != nil || idx < 0 || idx >= len(s.items) {
			return m
		}
		return s.items[idx]
	})
}

// markdownToHTML converts the markdown subset models actually emit into the tag set
// Telegram accepts. MarkdownV2 was not an option: it demands that every '.', '-' and
// '!' be backslash-escaped, which the model has no reason to do, so unescaped prose
// fails to parse. Telegram HTML has no heading or list markup either, so headings
// become bold lines and bullets become a literal •.
func markdownToHTML(src string) string {
	// A NUL in the model output could otherwise forge a stash marker.
	src = strings.ReplaceAll(src, "\x00", "")

	var st stash

	// Code spans are stashed first so their contents survive verbatim.
	src = reFence.ReplaceAllStringFunc(src, func(m string) string {
		return st.put("<pre>" + html.EscapeString(reFence.FindStringSubmatch(m)[1]) + "</pre>")
	})
	src = reInlineCode.ReplaceAllStringFunc(src, func(m string) string {
		return st.put("<code>" + html.EscapeString(reInlineCode.FindStringSubmatch(m)[1]) + "</code>")
	})

	lines := strings.Split(src, "\n")
	for i, line := range lines {
		heading := false
		if m := reHeading.FindStringSubmatch(line); m != nil {
			line, heading = m[1], true
		} else {
			line = reBullet.ReplaceAllString(line, "$1• ")
		}

		line = html.EscapeString(line)
		line = reLink.ReplaceAllString(line, `<a href="$2">$1</a>`)
		// Bold before italic, or the two-star opener is eaten as an italic marker.
		line = reBold.ReplaceAllString(line, "<b>$1</b>")
		line = reBoldAlt.ReplaceAllString(line, "<b>$1</b>")
		line = reStrike.ReplaceAllString(line, "<s>$1</s>")
		line = reItalic.ReplaceAllString(line, "${1}<i>${2}</i>")
		line = reItalicAlt.ReplaceAllString(line, "${1}<i>${2}</i>")

		if heading && strings.TrimSpace(line) != "" {
			line = "<b>" + line + "</b>"
		}
		lines[i] = line
	}

	return st.restore(strings.Join(lines, "\n"))
}

// renderChunk turns one chunk of model output into the text Telegram should receive
// and the parse mode it needs; an empty parse mode means the text goes out verbatim.
func renderChunk(s string, f mode.Format) (string, string) {
	switch f {
	case mode.FormatMarkdown:
		return markdownToHTML(s), tgbotapi.ModeHTML
	case mode.FormatPlain:
		return s, ""
	default:
		return "<code>" + html.EscapeString(s) + "</code>", tgbotapi.ModeHTML
	}
}

// splitForFormat splits text so every chunk still fits Telegram's limit *after*
// rendering. Escaping and tag insertion expand the text by a factor that depends on
// its content — a chunk of nothing but '&' grows fivefold — so the budget is halved
// until the rendered result actually fits rather than trusting a fixed margin.
func splitForFormat(text string, f mode.Format) []string {
	budget := splitBudget
	for {
		chunks := splitMessage(text, budget)
		if budget <= minSplitBudget || rendersWithinLimit(chunks, f) {
			return chunks
		}
		budget /= 2
	}
}

func rendersWithinLimit(chunks []string, f mode.Format) bool {
	for _, c := range chunks {
		out, _ := renderChunk(c, f)
		if utf16Len(out) > telegramMessageLimit {
			return false
		}
	}
	return true
}
